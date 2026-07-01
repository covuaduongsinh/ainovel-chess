package host

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/models"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

// recentSampleCap là kích thước cửa sổ trượt: chỉ giữ lại N lần gọi gần nhất của mỗi role (cacheRead, input)
// làm mẫu, dùng để so sánh "lũy kế vs N lần gần nhất" tỷ lệ hit trong cột trái, nhận diện "kéo lùi giai đoạn đầu" vs "hit thấp ổn định".
const recentSampleCap = 10

// UsageTracker lũy kế token LLM đầu vào/đầu ra và chi phí đô la của tất cả agent trong toàn phiên.
//
// Cơ chế hoạt động:
//   - Gọi Record(agentName, msg) mỗi khi callback OnMessage của agent được kích hoạt
//   - agentName ánh xạ sang role (architect_* gộp thành architect), tra ModelSet mô hình hiện tại role đó gắn
//   - Dùng models.DefaultRegistry tra giá mô hình, nhân theo 4 hạng: đầu vào không cache/đầu ra/đọc cache/ghi cache
//   - Khi registry không có mô hình này, lui về msg.Usage.Cost.Total (provider tự kèm, có thể là 0)
//   - Sau khi hot-switch mô hình (/model), các tin nhắn tiếp theo tự động tính theo mô hình mới, tin cũ giữ chi phí cũ
//
// Đồng thời duy trì chiều per-role (writer/editor/architect/coordinator):
//   - Dữ liệu hit lũy kế → hiệu quả tối ưu tổng thể
//   - Cửa sổ trượt N lần gần nhất → phân biệt kéo lùi giai đoạn đầu vs hit thấp ổn định
//   - Cờ CacheCapable → phân biệt "chưa bật" và "thực sự 0% hit"
//
// Thread-safe.
type UsageTracker struct {
	mu       sync.Mutex
	overall  agentTotals
	perAgent map[string]*agentTotals // key là tên role sau khi gộp agentRoleName
	perModel map[string]*agentTotals // key là provider/model; khi không rõ provider thì thoái lui thành model
	modelSet *bootstrap.ModelSet
	store    *storepkg.Store // có thể là nil (kịch bản test), khi nil tất cả phương thức lưu trữ im lặng noop

	// missingAssistantUsage lũy kế số lần "nhận tin nhắn assistant nhưng Usage là nil".
	// Thực tế chủ yếu xảy ra khi backend tương thích OpenAI tự xây không gửi
	// final usage chunk theo giao thức stream_options.include_usage của OpenAI ở cuối streaming —
	// partial.Usage luôn nil, tất cả trường lũy kế đều dừng ở 0. Bộ đếm giúp UI trực tiếp
	// báo cho người dùng "upstream không trả usage chứ không phải bên này hỏng",
	// thay vì đau đầu với code panel cache.
	missingAssistantUsage int
	loggedMissingUsage    bool // Cả phiên chỉ warn một lần, tránh tui.log bị tràn

	// saveCh được Record kích hoạt không chặn sau khi cộng dồn; autoSaveLoop lắng nghe và ghi đĩa theo debounce.
	// buffered=1: nhiều Record liên tiếp gộp thành một tín hiệu ghi đĩa; đầy thì bỏ, tick tiếp theo ghi cùng.
	saveCh chan struct{}

	// onCost được gọi ngoài lock sau mỗi lần ghi tài, mang theo chi phí lũy kế mới nhất (kiểm tra vượt ngưỡng BudgetSentinel).
	// Phải đặt qua SetOnCost trước khi Record song song bắt đầu, sau đó chỉ đọc.
	onCost func(total float64)

	// onMissingUsage được gọi một lần khi lần đầu phát hiện "tin nhắn assistant không có Usage" (cùng thời điểm với slog warn).
	// Khi ngân sách bật, điều này có nghĩa là vùng mù tính phí — chi phí luôn 0, ngân sách không bao giờ kích hoạt, phải báo người.
	onMissingUsage func()
}

// usageSample là mẫu hit của một lần OnMessage, chỉ ghi tử số và mẫu số tỷ lệ hit.
type usageSample struct {
	CacheRead int
	Input     int
}

// agentTotals là bộ đếm lũy kế của một agent.
//   - Saved là chênh lệch "nếu tính theo giá không cache" phản tính từ dữ liệu hit hiện tại
//   - CacheCapable chỉ được đặt true khi role này đã trải qua ít nhất một lần gọi "mô hình đã biết hỗ trợ cache"
//   - samples là ring buffer có độ dài cố định, N lần đầu append thẳng, sau đó luân phiên theo sampleIdx
type agentTotals struct {
	Input        int
	Output       int
	CacheRead    int
	CacheWrite   int
	Cost         float64
	Saved        float64
	CacheCapable bool
	samples      []usageSample
	sampleIdx    int
}

func NewUsageTracker(set *bootstrap.ModelSet, store *storepkg.Store) *UsageTracker {
	return &UsageTracker{
		modelSet: set,
		store:    store,
		perAgent: make(map[string]*agentTotals, 4),
		perModel: make(map[string]*agentTotals, 4),
		saveCh:   make(chan struct{}, 1),
	}
}

// Record phân phối một tin nhắn agent vào hai đường: lũy kế / chẩn đoán.
//
// Lũy kế chỉ xem Usage có tồn tại không — "tin nhắn nào mang Usage" là chi tiết
// lắp ráp của agentcore/litellm adapter (giao thức upstream đặt usage ở đỉnh response),
// sau này quy tắc lắp ráp thay đổi cũng không cần sửa đây.
// Chẩn đoán yêu cầu Role=Assistant và Content không rỗng, tránh AbortMsg / khôi phục ngoại lệ / tool /
// tin nhắn user làm ô nhiễm bộ đếm missingAssistantUsage.
func (t *UsageTracker) Record(agentName string, msg agentcore.AgentMessage) {
	if t == nil {
		return
	}
	m, ok := msg.(agentcore.Message)
	if !ok {
		return
	}
	if m.Usage == nil {
		if m.Role == agentcore.RoleAssistant && len(m.Content) > 0 {
			t.flagMissingUsage(agentName)
		}
		return
	}
	role := agentRoleName(agentName)
	provider, modelName := usageActualModel(m.Usage)
	t.accumulate(role, provider, modelName, *m.Usage)
}

func usageActualModel(u *agentcore.Usage) (provider, modelName string) {
	if u == nil {
		return "", ""
	}
	return strings.TrimSpace(u.Provider), strings.TrimSpace(u.Model)
}

// flagMissingUsage lũy kế một lần sự kiện "có vẻ là response LLM thật nhưng không lấy được usage",
// cả phiên chỉ ghi một lần warn log để tránh tui.log bị tràn.
func (t *UsageTracker) flagMissingUsage(agentName string) {
	t.mu.Lock()
	t.missingAssistantUsage++
	shouldLog := !t.loggedMissingUsage
	t.loggedMissingUsage = true
	t.mu.Unlock()
	if shouldLog {
		slog.Warn("Response LLM không mang dữ liệu usage, bảng cache/chi phí sẽ không có lũy kế — thường do upstream streaming không gửi final usage chunk theo giao thức OpenAI include_usage",
			"module", "usage", "agent", agentName)
		if t.onMissingUsage != nil {
			t.onMissingUsage()
		}
	}
	t.notifyDirty()
}

// SetOnMissingUsage đăng ký callback một lần cho "lần đầu phát hiện usage thiếu".
// Phải gọi một lần trong giai đoạn xây dựng Host, trước khi Record song song bắt đầu.
func (t *UsageTracker) SetOnMissingUsage(cb func()) {
	if t == nil {
		return
	}
	t.onMissingUsage = cb
}

// notifyDirty kích hoạt một tín hiệu ghi đĩa không chặn, autoSaveLoop thực sự ghi theo debounce.
// Kênh tín hiệu buffered=1: nhiều Record liên tiếp gộp thành một yêu cầu lưu là đủ.
func (t *UsageTracker) notifyDirty() {
	if t == nil || t.saveCh == nil {
		return
	}
	select {
	case t.saveCh <- struct{}{}:
	default:
	}
}

// accumulate lũy kế một tin nhắn có Usage vào ba bộ đếm: overall / per-role / per-model.
// provider/model rỗng nghĩa là "dùng ModelSet hiện tại lấy mô hình tương ứng với role" (đường thời gian thực);
// không rỗng nghĩa là "cưỡng chế tính giá theo mô hình chỉ định" (đường replay dùng _meta trong session jsonl).
// resolveCost thực thi ngoài lock (nó chỉ đọc modelSet/Registry), bên trong lock chỉ làm phép cộng.
func (t *UsageTracker) accumulate(role, provider, modelName string, u agentcore.Usage) {
	provider, modelName = t.effectiveModel(role, provider, modelName)
	cost, saved, capable := t.resolveCost(modelName, u)

	t.mu.Lock()
	addUsage(&t.overall, u, cost, saved, capable)

	per := t.perAgent[role]
	if per == nil {
		per = &agentTotals{}
		t.perAgent[role] = per
	}
	addUsage(per, u, cost, saved, capable)

	if key := modelUsageKey(provider, modelName); key != "" {
		perModel := t.perModel[key]
		if perModel == nil {
			perModel = &agentTotals{}
			t.perModel[key] = perModel
		}
		addUsage(perModel, u, cost, saved, capable)
	}
	total := t.overall.Cost
	t.mu.Unlock()

	t.notifyDirty()
	if t.onCost != nil {
		t.onCost(total)
	}
}

// SetOnCost đăng ký callback ghi tài (mang chi phí lũy kế mới nhất, gọi ngoài lock).
// Phải gọi một lần trong giai đoạn xây dựng Host, trước khi Record song song bắt đầu.
func (t *UsageTracker) SetOnCost(cb func(total float64)) {
	if t == nil {
		return
	}
	t.onCost = cb
}

func (t *UsageTracker) effectiveModel(role, provider, modelName string) (string, string) {
	provider = strings.TrimSpace(provider)
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		if t != nil && t.modelSet != nil {
			p, m, _ := t.modelSet.CurrentSelection(role)
			return p, m
		}
		return "", ""
	}
	if provider == "" && t != nil && t.modelSet != nil {
		p, m, _ := t.modelSet.CurrentSelection(role)
		if m == modelName {
			provider = p
		}
	}
	return provider, modelName
}

func modelUsageKey(provider, modelName string) string {
	provider = strings.TrimSpace(provider)
	modelName = strings.TrimSpace(modelName)
	switch {
	case modelName == "":
		return ""
	case provider == "":
		return modelName
	default:
		return provider + "/" + modelName
	}
}

// addUsage chồng token và chi phí của một lần gọi vào một bộ totals.
// Phải được gọi trong khi đang giữ UsageTracker.mu.
//
// CacheCapable ưu tiên phán định theo "thực tế": chỉ cần thấy CacheRead hoặc CacheWrite > 0
// là đã chứng minh upstream thực sự đã làm prompt caching. CacheReadCostPer1M của registry
// chỉ làm fallback, vì các mô hình backend tự xây (mimo-v2.5-pro / proxy nội địa v.v.)
// thường không có trong chỉ mục giá BerriAI/litellm, nhưng Usage thực tế hoàn toàn có dữ liệu cache,
// UI không nên nhầm phán là "chưa bật".
func addUsage(t *agentTotals, u agentcore.Usage, cost, saved float64, capable bool) {
	t.Input += u.Input
	t.Output += u.Output
	t.CacheRead += u.CacheRead
	t.CacheWrite += u.CacheWrite
	t.Cost += cost
	t.Saved += saved
	if capable || u.CacheRead > 0 || u.CacheWrite > 0 {
		t.CacheCapable = true
	}
	pushSample(t, u.CacheRead, u.Input)
}

// pushSample đẩy một mẫu vào ring buffer. N lần đầu là append thuần, sau đó luân phiên ghi đè.
func pushSample(t *agentTotals, cacheRead, input int) {
	s := usageSample{CacheRead: cacheRead, Input: input}
	if len(t.samples) < recentSampleCap {
		t.samples = append(t.samples, s)
		return
	}
	t.samples[t.sampleIdx] = s
	t.sampleIdx = (t.sampleIdx + 1) % recentSampleCap
}

// recentSums trả về tổng cacheRead và input trong cửa sổ trượt, làm tử số và mẫu số của "tỷ lệ hit N lần gần nhất".
// Dùng sum/sum thay vì "trung bình tỷ lệ từng lần" để tránh mẫu nhỏ (input=vài trăm token) khuếch đại nhiễu.
func recentSums(t *agentTotals) (cacheRead, input int) {
	for _, s := range t.samples {
		cacheRead += s.CacheRead
		input += s.Input
	}
	return cacheRead, input
}

// Totals trả về snapshot tổng lũy kế.
func (t *UsageTracker) Totals() (cost float64, input, output, cacheRead, cacheWrite int) {
	if t == nil {
		return 0, 0, 0, 0, 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.overall.Cost, t.overall.Input, t.overall.Output, t.overall.CacheRead, t.overall.CacheWrite
}

// SavedUSD trả về số đô la lũy kế tiết kiệm được nhờ cache hit.
func (t *UsageTracker) SavedUSD() float64 {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.overall.Saved
}

// OverallRecent trả về tổng cacheRead, tổng input và số mẫu trong cửa sổ trượt (≤ recentSampleCap lần).
func (t *UsageTracker) OverallRecent() (cacheRead, input, samples int) {
	if t == nil {
		return 0, 0, 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	r, in := recentSums(&t.overall)
	return r, in, len(t.overall.samples)
}

// OverallCacheCapable kiểm tra tổng thể có đã ít nhất một lần trải qua mô hình đã biết hỗ trợ cache không.
func (t *UsageTracker) OverallCacheCapable() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.overall.CacheCapable
}

// MissingAssistantUsage trả về số lần lũy kế "nhận tin nhắn assistant nhưng Usage là nil".
// Lớn hơn 0 thường nghĩa là upstream streaming không gửi final usage chunk của OpenAI,
// UI dựa vào đây hiển thị gợi ý thay vì nhầm nghĩ module cache bản thân bị hỏng.
func (t *UsageTracker) MissingAssistantUsage() int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.missingAssistantUsage
}

// ── Lưu trữ ──

// Snapshot sao chép trạng thái lũy kế hiện tại thành domain.UsageState có thể serialize.
// samples của cửa sổ trượt không vào snapshot — nó là cửa sổ chẩn đoán ngắn hạn, không có ý nghĩa qua tiến trình.
func (t *UsageTracker) Snapshot() domain.UsageState {
	if t == nil {
		return domain.UsageState{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	state := domain.UsageState{
		Schema:       domain.UsageSchemaVersion,
		UpdatedAt:    time.Now(),
		Overall:      totalsSnapshot(&t.overall),
		PerAgent:     make(map[string]domain.AgentUsageTotals, len(t.perAgent)),
		PerModel:     make(map[string]domain.AgentUsageTotals, len(t.perModel)),
		MissingUsage: t.missingAssistantUsage,
	}
	for role, v := range t.perAgent {
		state.PerAgent[role] = totalsSnapshot(v)
	}
	for model, v := range t.perModel {
		state.PerModel[model] = totalsSnapshot(v)
	}
	return state
}

// LoadFromStore đọc snapshot đã lưu từ store.Usage và điền ngược lại vào bộ nhớ. Trả về true nghĩa là
// đã load thành công một trạng thái không rỗng (schema khớp); false nghĩa là không có file hoặc không dùng được,
// caller nên tiếp tục đi đường session replay để điền lại một lần.
func (t *UsageTracker) LoadFromStore() (bool, error) {
	if t == nil || t.store == nil {
		return false, nil
	}
	state, err := t.store.Usage.Load()
	if err != nil {
		return false, err
	}
	if state == nil {
		return false, nil
	}
	t.applyState(*state)
	return true, nil
}

// SaveNow lập tức ghi snapshot hiện tại xuống đĩa. Cả đường autoSaveLoop / Close đều ghi qua nó.
func (t *UsageTracker) SaveNow() error {
	if t == nil || t.store == nil {
		return nil
	}
	return t.store.Usage.Save(t.Snapshot())
}

// StartAutoSave khởi một goroutine, lắng nghe saveCh + debounce ghi đĩa. Trước khi ctx done sẽ
// flush trạng thái chưa lưu lần cuối. Close kích hoạt flush + thoát thông qua cancel ctx.
func (t *UsageTracker) StartAutoSave(ctx context.Context) {
	if t == nil || t.store == nil {
		return
	}
	go t.autoSaveLoop(ctx)
}

// autoSaveLoop điều tiết tín hiệu dirty tần cao thành ghi đĩa mỗi 500ms.
//
// Ghi chú thiết kế: 500ms là giá trị kinh nghiệm — mỗi chương 1-2 LLM turn, ghi đĩa 1-2 lần hoàn toàn chấp nhận được;
// kể cả người dùng ctrl+C thoát thủ công không kịp kích hoạt timer, đường ctx cancel cũng flush một lần cuối.
// Crash thật sự (OS kill -9) sẽ mất lũy kế trong 0.5s gần nhất — session jsonl upstream vẫn là
// sự thật đầy đủ, lần khởi động tiếp sẽ replay từ sessions/ bù đắp chênh lệch.
func (t *UsageTracker) autoSaveLoop(ctx context.Context) {
	const debounce = 500 * time.Millisecond
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()

	var pending bool
	flush := func() {
		if err := t.SaveNow(); err != nil {
			slog.Warn("Ghi usage xuống đĩa thất bại", "module", "usage", "err", err)
		}
		pending = false
	}
	for {
		select {
		case <-ctx.Done():
			if pending {
				flush()
			}
			return
		case <-t.saveCh:
			if pending {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
			}
			timer.Reset(debounce)
			pending = true
		case <-timer.C:
			flush()
		}
	}
}

// applyState ghi snapshot đã lưu trở lại bộ nhớ. Chỉ được gọi lúc khởi động (sau LoadFromStore / replay),
// lúc này autoSaveLoop chưa khởi động / Record cũng không kích hoạt song song, có thể không giữ lock;
// nhưng giữ lại mu đề phòng test hoặc thứ tự gọi tương lai thay đổi gây ra đồng thời.
func (t *UsageTracker) applyState(state domain.UsageState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.overall = totalsFromState(state.Overall)
	if state.PerAgent == nil {
		t.perAgent = make(map[string]*agentTotals, 4)
	} else {
		t.perAgent = make(map[string]*agentTotals, len(state.PerAgent))
		for role, v := range state.PerAgent {
			tot := totalsFromState(v)
			t.perAgent[role] = &tot
		}
	}
	if state.PerModel == nil {
		t.perModel = make(map[string]*agentTotals, 4)
	} else {
		t.perModel = make(map[string]*agentTotals, len(state.PerModel))
		for model, v := range state.PerModel {
			tot := totalsFromState(v)
			t.perModel[model] = &tot
		}
	}
	t.missingAssistantUsage = state.MissingUsage
}

// totalsSnapshot sao chép agentTotals trong bộ nhớ thành domain.AgentUsageTotals có thể lưu trữ.
// Ring buffer samples cố ý không mang ra — xem chú thích UsageState.
func totalsSnapshot(t *agentTotals) domain.AgentUsageTotals {
	if t == nil {
		return domain.AgentUsageTotals{}
	}
	return domain.AgentUsageTotals{
		Input:        t.Input,
		Output:       t.Output,
		CacheRead:    t.CacheRead,
		CacheWrite:   t.CacheWrite,
		Cost:         t.Cost,
		Saved:        t.Saved,
		CacheCapable: t.CacheCapable,
	}
}

// totalsFromState khôi phục dạng đã lưu thành agentTotals trong bộ nhớ. samples để trống, sau khi khởi động lại
// tích lũy từ đầu từ 0, vài vòng Record sau là có thể khôi phục ngữ nghĩa "tỷ lệ hit N lần gần nhất".
func totalsFromState(s domain.AgentUsageTotals) agentTotals {
	return agentTotals{
		Input:        s.Input,
		Output:       s.Output,
		CacheRead:    s.CacheRead,
		CacheWrite:   s.CacheWrite,
		Cost:         s.Cost,
		Saved:        s.Saved,
		CacheCapable: s.CacheCapable,
	}
}

// AgentUsage là snapshot lũy kế dùng lượng của một agent (phơi ra cho UI).
type AgentUsage struct {
	Role            string
	Model           string
	Input           int
	Output          int
	CacheRead       int
	CacheWrite      int
	Cost            float64
	Saved           float64
	CacheCapable    bool
	RecentCacheRead int
	RecentInput     int
	RecentSamples   int
}

// PerAgent trả về lũy kế dùng lượng của từng role. Kết quả giảm dần theo CacheRead, role chưa tiêu thụ token thì bỏ qua.
func (t *UsageTracker) PerAgent() []AgentUsage {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]AgentUsage, 0, len(t.perAgent))
	for role, v := range t.perAgent {
		if v.Input == 0 && v.Output == 0 {
			continue
		}
		recentRead, recentInput := recentSums(v)
		out = append(out, AgentUsage{
			Role:            role,
			Input:           v.Input,
			Output:          v.Output,
			CacheRead:       v.CacheRead,
			CacheWrite:      v.CacheWrite,
			Cost:            v.Cost,
			Saved:           v.Saved,
			CacheCapable:    v.CacheCapable,
			RecentCacheRead: recentRead,
			RecentInput:     recentInput,
			RecentSamples:   len(v.samples),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CacheRead != out[j].CacheRead {
			return out[i].CacheRead > out[j].CacheRead
		}
		return out[i].Input > out[j].Input
	})
	return out
}

// PerModel trả về lũy kế dùng lượng của từng mô hình. Kết quả giảm dần theo chi phí, sau đó theo lượng đầu vào.
func (t *UsageTracker) PerModel() []AgentUsage {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]AgentUsage, 0, len(t.perModel))
	for model, v := range t.perModel {
		if v.Input == 0 && v.Output == 0 {
			continue
		}
		out = append(out, AgentUsage{
			Model:        model,
			Input:        v.Input,
			Output:       v.Output,
			CacheRead:    v.CacheRead,
			CacheWrite:   v.CacheWrite,
			Cost:         v.Cost,
			Saved:        v.Saved,
			CacheCapable: v.CacheCapable,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Cost != out[j].Cost {
			return out[i].Cost > out[j].Cost
		}
		return out[i].Input > out[j].Input
	})
	return out
}

// resolveCost đồng thời trả về cost / saved / capable của tin nhắn lần này.
//   - cost: registry hit thì nhân theo 4 hạng; không hit thì lui về cost tự kèm của provider
//   - saved: chỉ > 0 khi registry hit, CacheRead > 0, và InputCost > CacheReadCost
//   - capable: registry hit và mô hình đó có CacheReadCostPer1M > 0 → đã biết hỗ trợ prompt caching
//
// modelName ưu tiên dùng cái caller truyền vào (lúc replay đến từ _meta.model trong session jsonl).
func (t *UsageTracker) resolveCost(modelName string, u agentcore.Usage) (cost, saved float64, capable bool) {
	if entry, ok := models.DefaultRegistry().Resolve(modelName); ok {
		c := computeCost(u, *entry)
		s := computeSaved(u, *entry)
		canCache := entry.CacheReadCostPer1M > 0
		if c > 0 {
			return c, s, canCache
		}
	}
	if u.Cost != nil {
		return u.Cost.Total, 0, false
	}
	return 0, 0, false
}

// agentRoleName gộp tên subagent về tên role.
// architect_short/mid/long đều gộp về architect; các tên khác trả về nguyên.
func agentRoleName(agentName string) string {
	if strings.HasPrefix(agentName, "architect_") {
		return "architect"
	}
	return agentName
}

// computeCost tính chi phí đô la của lần gọi này theo đơn giá $/1M token.
//
// Tiền đề ngữ nghĩa (được tất cả provider của litellm đảm bảo thống nhất, xem điểm lắp ráp Usage
// của anthropic.go / bedrock.go / openai.go / gemini.go / compat.go):
//
//	u.Input  = tổng token đầu vào, **bao gồm** CacheRead; không bao gồm CacheWrite
//	u.Output = token đầu ra
//
// Do đó nonCachedInput = u.Input - u.CacheRead đúng với mọi provider.
// Nhánh phòng thủ giữ lại để đối phó trường hợp provider nào đó tương lai trả dữ liệu bẩn không bị crash.
func computeCost(u agentcore.Usage, e models.ModelEntry) float64 {
	nonCachedInput := u.Input - u.CacheRead
	if nonCachedInput < 0 {
		nonCachedInput = u.Input
	}
	c := 0.0
	c += float64(nonCachedInput) * e.InputCostPer1M / 1_000_000
	c += float64(u.Output) * e.OutputCostPer1M / 1_000_000
	c += float64(u.CacheRead) * e.CacheReadCostPer1M / 1_000_000
	c += float64(u.CacheWrite) * e.CacheWriteCostPer1M / 1_000_000
	return c
}

// computeSaved ước tính số đô la tiết kiệm được khi CacheRead hit so với "tính theo giá đầu vào thông thường".
// Lưu ý phụ phí CacheWrite không khấu trừ — nó thuộc khoản đầu tư cần thiết "mở đường cho hit tiếp theo",
// lợi ích thực lũy kế từ CacheRead tiếp theo thu hồi.
func computeSaved(u agentcore.Usage, e models.ModelEntry) float64 {
	if u.CacheRead <= 0 || e.InputCostPer1M <= 0 {
		return 0
	}
	delta := e.InputCostPer1M - e.CacheReadCostPer1M
	if delta <= 0 {
		return 0
	}
	return float64(u.CacheRead) * delta / 1_000_000
}

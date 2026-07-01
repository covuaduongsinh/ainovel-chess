package host

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/domain"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
	"sync/atomic"
)

// errorKind classifies a runtime error into a stable, short label for log
// filtering and alert routing. Returns "" when no special tag applies.
//
// err is the live error chain (may be nil after JSON serialization); msg is
// the rendered string fallback used when the chain has been flattened
// (e.g. inside sub-agent JSON results).
func errorKind(err error, msg string) string {
	if err != nil && errors.Is(err, agentcore.ErrProviderStreamIdle) {
		return "stream_idle"
	}
	if msg != "" && agentcore.IsStreamIdleMessage(msg) {
		return "stream_idle"
	}
	return ""
}

// Bộ đếm ID sự kiện tăng đơn điệu; kết hợp với timestamp tạo ID ổn định.
var eventIDCounter uint64

func nextEventID() string {
	return fmt.Sprintf("e%d", atomic.AddUint64(&eventIDCounter, 1))
}

// activeCall ghi lại ID, thời điểm bắt đầu và summary của một lần gọi đang tiến hành (TOOL / DISPATCH).
// summary được điền lại vào finish Event khi hoàn thành, đảm bảo replay (runtime queue) có thể khôi phục nội dung dòng.
type activeCall struct {
	id      string
	start   time.Time
	summary string
	depth   int
}

// observer đăng ký luồng sự kiện của coordinator và chiếu sang kênh xuất của Host.
// Đây là observer thuần túy, không tham gia bất kỳ quyết định điều khiển nào.
type observer struct {
	unsub   func()
	emitEv  func(Event)
	emitD   func(string)
	emitC   func()
	store   *storepkg.Store // Dùng để lưu runtime queue (ReplayQueue tiêu thụ)
	agents  map[string]*agentState
	agentMu sync.Mutex

	// aborting được Host đặt tại điểm vào Abort()/Close(), xóa tại Start/Resume/Continue.
	// Khi được đặt, tất cả sự kiện lỗi dẫn xuất từ context-cancel bị ức chế (vừa là mong muốn người dùng,
	// vừa tránh trùng lặp với sự kiện "người dùng tạm dừng thủ công"). Lỗi thực (không phải cancel) vẫn báo cáo bình thường.
	aborting atomic.Bool

	streamThinking        bool
	lastThinkingByAgent   map[string]string          // agent → văn bản thinking tích lũy gần nhất (dùng để trích delta tăng dần)
	dispatchStarts        map[string]*activeCall     // dispatched agent → lần gọi DISPATCH đang tiến hành
	currentDispatchTarget string                     // tên subagent đang thực thi (Args có thể rỗng khi handleToolEnd)
	toolStarts            map[string]*activeCall     // agent → lần gọi TOOL đang tiến hành
	streamExtractors      map[string]*agentExtractor // agent → bộ trích nội dung tham số JSON lần gọi công cụ hiện tại
	streamArgPrefixes     map[string]string          // agent/tool → tiền tố luồng tham số, dùng để nhận diện sớm thẻ nhẹ
	streamArgLabels       map[string]string          // agent/tool → tên hiển thị đã nhận diện sớm từ luồng tham số
	retryEvents           map[string]string          // retry scope → event ID, cập nhật tại chỗ cùng dòng (2/7)
	streamHasContent      bool                       // streamRound hiện tại có đã xuất nội dung nào chưa (để xác định có cần ngăn cách đoạn không)
	streamLastByte        byte                       // byte cuối cùng của lần xuất luồng gần nhất (dùng để bổ sung xuống dòng chính xác)
}

// agentExtractor ghi lại tên công cụ và instance bộ trích mà một agent đang trích xuất hiện tại.
// Tên công cụ dùng để phát hiện "lần gọi công cụ mới bắt đầu", tránh cache bị ô nhiễm bởi dư cặn vòng trước.
type agentExtractor struct {
	tool       string
	ext        *jsonFieldExtractor
	emittedAny bool // extractor này đã sản xuất nội dung nào chưa; dùng để bổ sung ngăn cách đoạn trước lần xuất đầu tiên
}

type agentState struct {
	name    string
	state   string
	tool    string
	summary string
	turn    int
	context AgentContextSnapshot
	updated time.Time
}

func newObserver(coordinator *agentcore.Agent, s *storepkg.Store, emitEv func(Event), emitD func(string), emitC func()) *observer {
	o := &observer{
		emitEv:              emitEv,
		emitD:               emitD,
		emitC:               emitC,
		store:               s,
		agents:              make(map[string]*agentState),
		lastThinkingByAgent: make(map[string]string),
		dispatchStarts:      make(map[string]*activeCall),
		toolStarts:          make(map[string]*activeCall),
		streamExtractors:    make(map[string]*agentExtractor),
		streamArgPrefixes:   make(map[string]string),
		streamArgLabels:     make(map[string]string),
		retryEvents:         make(map[string]string),
	}
	o.unsub = coordinator.Subscribe(o.handle)
	return o
}

func (o *observer) finalize() {
	o.agentMu.Lock()
	defer o.agentMu.Unlock()
	for _, a := range o.agents {
		a.state = "idle"
		a.tool = ""
	}
}

// setAborting được Host gọi tại các điểm chuyển vòng đời như Abort/Close/Start, điều khiển
// việc các sự kiện dẫn xuất kiểu "context canceled" có cần ức chế không (tránh trùng lặp với "người dùng tạm dừng thủ công").
func (o *observer) setAborting(v bool) { o.aborting.Store(v) }

func (o *observer) retryEventID(scope string, attempt int) string {
	if strings.TrimSpace(scope) == "" {
		scope = "coordinator"
	}
	if o.retryEvents == nil {
		o.retryEvents = make(map[string]string)
	}
	if attempt <= 1 || o.retryEvents[scope] == "" {
		o.retryEvents[scope] = nextEventID()
	}
	return o.retryEvents[scope]
}

// emitAndLog dùng cho trạng thái "bắt đầu" của sự kiện loại gọi: gửi cho TUI nhưng không ghi vào runtime queue,
// tránh replay bị trùng "bắt đầu một dòng, hoàn thành lại một dòng". slog được host.emitEvent ghi tập trung.
func (o *observer) emitAndLog(ev Event) {
	o.emitEv(ev)
}

// persistEvent ghi sự kiện vào runtime queue (slog được host.emitEvent ghi tập trung).
func (o *observer) persistEvent(ev Event) {
	if o.store == nil || o.store.Runtime == nil {
		return
	}
	priority := domain.RuntimePriorityBackground
	switch ev.Category {
	case "SYSTEM", "ERROR":
		priority = domain.RuntimePriorityControl
	}
	_, _ = o.store.Runtime.AppendQueue(domain.RuntimeQueueItem{
		Time:     ev.Time,
		Kind:     domain.RuntimeQueueUIEvent,
		Priority: priority,
		Category: ev.Category,
		Summary:  ev.Summary,
		Payload:  ev,
	})
}

func (o *observer) updateAgent(name string, fn func(*agentState)) {
	if name == "" {
		return
	}
	o.agentMu.Lock()
	defer o.agentMu.Unlock()
	a, ok := o.agents[name]
	if !ok {
		a = &agentState{name: name, state: "idle"}
		o.agents[name] = a
	}
	fn(a)
	a.updated = time.Now()
}

func (o *observer) agentSnapshots() []AgentSnapshot {
	o.agentMu.Lock()
	defer o.agentMu.Unlock()
	snaps := make([]AgentSnapshot, 0, len(o.agents))
	for _, a := range o.agents {
		snaps = append(snaps, AgentSnapshot{
			Name:      a.name,
			State:     a.state,
			Summary:   a.summary,
			Tool:      a.tool,
			Turn:      a.turn,
			Context:   a.context,
			UpdatedAt: a.updated,
		})
	}
	return snaps
}

func agentFromEvent(ev agentcore.Event) string {
	if ev.Progress != nil && ev.Progress.Agent != "" {
		return ev.Progress.Agent
	}
	return "coordinator"
}

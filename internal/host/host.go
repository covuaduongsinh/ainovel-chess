package host

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/voocel/agentcore"
	corecontext "github.com/voocel/agentcore/context"
	"github.com/voocel/ainovel-cli/assets"
	"github.com/voocel/ainovel-cli/internal/agents"
	"github.com/voocel/ainovel-cli/internal/agents/ctxpack"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host/exp"
	"github.com/voocel/ainovel-cli/internal/host/flow"
	"github.com/voocel/ainovel-cli/internal/host/imp"
	"github.com/voocel/ainovel-cli/internal/host/sim"
	modelreg "github.com/voocel/ainovel-cli/internal/models"
	"github.com/voocel/ainovel-cli/internal/notify"
	"github.com/voocel/ainovel-cli/internal/rules"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
	"github.com/voocel/ainovel-cli/internal/tools"
	"github.com/voocel/ainovel-cli/internal/userrules"
)

// Host là vỏ mỏng thời gian chạy.
// Trách nhiệm: khởi động/khôi phục/tiêm can thiệp/chiếu sự kiện/quản lý mô hình.
// Không đưa ra bất kỳ quyết định lịch trình nào, không tự chạy tiếp khi rảnh.
type Host struct {
	cfg               bootstrap.Config
	bundle            assets.Bundle
	store             *storepkg.Store
	models            *bootstrap.ModelSet
	coordinator       *agentcore.Agent
	coordinatorCtxMgr *corecontext.ContextEngine // Khi chuyển mô hình default/coordinator thì đồng bộ SetContextWindow + SetReserveTokens
	thinkingApplier   agents.ApplyThinking       // Khi /model điều chỉnh cường độ suy luận thì đồng bộ live agent (coordinator + subagent)
	askUser           *tools.AskUserTool
	writerRestore     *ctxpack.WriterRestorePack
	observer          *observer
	router            *flow.Dispatcher
	usage             *UsageTracker
	usageCancel       context.CancelFunc // Dừng autoSaveLoop và kích hoạt flush lần cuối
	budget            *BudgetSentinel    // Chính sách ngân sách; nil khi chưa bật (các phương thức an toàn với nil)
	budgetDetach      func()
	notifier          *notify.Notifier // Cảnh báo không người trực; nil khi chưa bật (Send an toàn với nil)

	events   chan Event
	streamCh chan string
	done     chan struct{}

	mu         sync.Mutex
	lifecycle  lifecycle
	cocreating bool // Chiếm dụng đồng sáng tác giai đoạn: chặn can thiệp đồng thời của import/simulate/continue trong cửa sổ paused
	closeOnce  sync.Once
}

type lifecycle string

const (
	lifecycleIdle      lifecycle = "idle"
	lifecycleRunning   lifecycle = "running"
	lifecyclePaused    lifecycle = "paused"
	lifecycleCompleted lifecycle = "completed"
)

// New tạo Host.
func New(cfg bootstrap.Config, bundle assets.Bundle) (*Host, error) {
	cfg.FillDefaults()
	if err := cfg.ValidateBase(); err != nil {
		return nil, err
	}
	slog.Info("Khởi động", "module", "boot", "provider", cfg.Provider, "model", cfg.ModelName, "output", cfg.OutputDir)

	// Khởi goroutine nền để làm mới metadata mô hình từ OpenRouter (cửa sổ/giá), cache đĩa 24h.
	modelreg.StartPricingRefresh(modelreg.DefaultRegistry(), bootstrap.DefaultConfigDir())

	store := storepkg.NewStore(cfg.OutputDir)
	if err := store.Init(); err != nil {
		return nil, fmt.Errorf("init store: %w", err)
	}

	models, err := bootstrap.NewModelSet(cfg)
	if err != nil {
		return nil, fmt.Errorf("create models: %w", err)
	}
	slog.Info("Mô hình sẵn sàng", "module", "boot", "summary", models.Summary())

	usage := NewUsageTracker(models, store)
	// Ưu tiên đọc meta/usage.json; các trường hợp sau đều đi qua sessions/*.jsonl để bù đắp một lần:
	//   - File không tồn tại (lần đầu nâng cấp lên phiên bản có persistence)
	//   - Schema version không khớp (bỏ định dạng cũ sau khi nâng cấp tương lai)
	//   - File tồn tại nhưng hỏng / lỗi IO (không để dữ liệu xấu làm lũy kế về zero vĩnh viễn)
	// Sau khi bù đắp xong lập tức SaveNow để cố định kết quả, lần khởi động tiếp theo Load trực tiếp.
	loaded, loadErr := usage.LoadFromStore()
	if loadErr != nil {
		slog.Warn("Tải usage thất bại, sẽ thử bù đắp từ sessions", "module", "usage", "err", loadErr)
	}
	if !loaded {
		if n, err := usage.ReplaySessions(cfg.OutputDir); err != nil {
			slog.Warn("usage replay thất bại", "module", "usage", "err", err)
		} else if n > 0 {
			slog.Info("usage bù đắp từ session hoàn tất", "module", "usage", "messages", n)
			if err := usage.SaveNow(); err != nil {
				slog.Warn("Lưu usage sau bù đắp thất bại", "module", "usage", "err", err)
			}
		}
	}
	usageCtx, usageCancel := context.WithCancel(context.Background())
	usage.StartAutoSave(usageCtx)

	var router *flow.Dispatcher
	var budget *BudgetSentinel
	coordinator, askUser, restore, coordinatorCtxMgr, applyThinking := agents.BuildCoordinator(cfg, store, models, bundle, usage.Record, func(string) {
		if budget != nil && budget.HandleBoundary() {
			return
		}
		if router != nil {
			router.Dispatch()
		}
	})
	store.Signals.ClearStaleSignals()

	h := &Host{
		cfg:               cfg,
		bundle:            bundle,
		store:             store,
		models:            models,
		coordinator:       coordinator,
		coordinatorCtxMgr: coordinatorCtxMgr,
		thinkingApplier:   applyThinking,
		askUser:           askUser,
		writerRestore:     restore,
		usage:             usage,
		usageCancel:       usageCancel,
		events:            make(chan Event, 100),
		streamCh:          make(chan string, 256),
		done:              make(chan struct{}, 4),
		lifecycle:         lifecycleIdle,
	}
	h.observer = newObserver(coordinator, store, h.emitEvent, h.emitDelta, h.emitClear)
	if cfg.Notify.IsEnabled() {
		h.notifier = notify.New(cfg.Notify.Command, cfg.Notify.Events)
	}
	// Sentinel ngân sách đăng ký sự kiện biên subagent để dừng; Dispatcher được kích hoạt đồng bộ bởi chuỗi thực thi công cụ,
	// không còn tranh quyền lần gọi model tiếp theo thông qua đăng ký sự kiện.
	if sentinel := NewBudgetSentinel(cfg.Budget,
		func() float64 { c, _, _, _, _ := usage.Totals(); return c },
		func(reason string) { h.abortWithEvent(reason, "error") },
		func(level, summary string) {
			h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: summary, Level: level})
			h.notifier.Send(notify.Notification{Kind: "budget", Level: level, Title: "ainovel: ngân sách", Body: summary})
		},
	); sentinel != nil {
		h.budget = sentinel
		budget = sentinel
		usage.SetOnCost(sentinel.OnCost)
		h.budgetDetach = coordinator.Subscribe(sentinel.HandleEvent)
		// Cảnh báo vùng mù tính phí: khi mô hình không báo usage thì chi phí luôn 0, ngân sách không bao giờ kích hoạt — cầu chì chưa nối phải báo người.
		usage.SetOnMissingUsage(func() {
			const blind = "Vùng mù ngân sách: mô hình không trả dữ liệu usage, thống kê chi phí bằng 0, ngưỡng ngân sách sẽ không kích hoạt (mô hình tùy chỉnh hãy xác nhận giá trong registry hoặc include_usage của upstream)"
			h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: blind, Level: "warn"})
			h.notifier.Send(notify.Notification{Kind: "budget", Level: "warn", Title: "ainovel: ngân sách", Body: blind})
		})
	}
	h.router = flow.NewDispatcher(coordinator, store)
	router = h.router
	// Cảnh báo lệnh lặp: telemetry thuần, khi treo máy thì "mô hình có thể đang xoay tại chỗ" đáng gọi người ngó qua.
	// Luồng sự kiện và notify phát theo cặp — notify chỉ là bản sao ngoài màn hình của sự kiện trong màn hình (kiến trúc §2.3).
	h.router.SetOnRepeat(func(agent, task string, n int) {
		body := fmt.Sprintf("Cùng một lệnh đã ra lệnh lần thứ %d (%s): %s", n, agent, task)
		h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: "Lệnh lặp: " + body, Level: "warn"})
		h.notifier.Send(notify.Notification{Kind: "repeat", Level: "warn", Title: "ainovel: lệnh lặp", Body: body})
	})

	if err := store.RunMeta.Init(cfg.Style, cfg.Provider, cfg.ModelName); err != nil {
		slog.Error("Khởi tạo metadata chạy thất bại", "module", "boot", "err", err)
	}

	return h, nil
}

// ── Vòng đời ──

// PrepareUserRules tạo snapshot quy tắc người dùng của cuốn sách này ở chế độ mới tạo (xác định từ phía khởi động, không qua Coordinator, không vào Run sáng tác chính).
//
// Tham số đầu vào là yêu cầu sáng tác **thô** của người dùng (chưa được BuildStartPrompt bao bọc) — chuẩn hóa cần quy tắc người dùng chính nó,
// không phải giàn giáo khởi động. Phải gọi một lần trước StartPrepared (cả hai đường tạo mới quick/cocreate đều đi qua đây).
//
// Chuẩn hóa thất bại chỉ hạ cấp không báo lỗi (đường tăng cường); chỉ khi snapshot không thể ghi xuống đĩa mới trả error dừng mở sách —
// các lần chạy tiếp theo sẽ không có nguồn sự thật ổn định (xem thiết kế §Thất bại và hạ cấp).
func (h *Host) PrepareUserRules(rawPrompt string) error {
	svc := userrules.NewService(h.store, h.models.Default, rules.DefaultOptions())
	snap, err := svc.Build(context.Background(), rawPrompt)
	if err != nil {
		return fmt.Errorf("ghi snapshot quy tắc người dùng thất bại, không thể tiếp tục: %w", err)
	}
	logUserRulesSnapshot(snap)
	return nil
}

// ensureUserRules lazily đảm bảo snapshot tồn tại (khi sách cũ không có snapshot thì tạo theo system_defaults + file rules).
// Đường khôi phục gọi, để sách cũ cũng lấy được kết quả chuẩn hóa của file rules.
func (h *Host) ensureUserRules() {
	svc := userrules.NewService(h.store, h.models.Default, rules.DefaultOptions())
	snap, err := svc.GetOrBuild(context.Background())
	if err != nil {
		slog.Warn("Đọc/tạo snapshot quy tắc người dùng thất bại, runtime sẽ dùng mặc định tích hợp", "module", "rules", "err", err)
		return
	}
	logUserRulesSnapshot(snap)
}

// logUserRulesSnapshot phản hồi khởi động: để người dùng thấy hệ thống hiểu quy tắc như thế nào (dùng lại log, không thêm cơ chế mới).
func logUserRulesSnapshot(snap *rules.Snapshot) {
	if snap == nil {
		return
	}
	words := "Chưa đặt"
	if w := snap.Structured.ChapterWords; w != nil {
		words = fmt.Sprintf("%d-%d", w.Min, w.Max)
	}
	slog.Info("Snapshot quy tắc người dùng",
		"module", "rules",
		"status", string(snap.Status),
		"nguon", snap.Sources,
		"so_tu_chuong", words,
		"cum_tu_cam", len(snap.Structured.ForbiddenPhrases),
		"tu_met_moi", len(snap.Structured.FatigueWords),
	)
	if snap.Status == rules.StatusDegraded {
		slog.Warn("Một số quy tắc không thể phân tích, đang chạy theo raw preferences (có thể tạo lại snapshot)",
			"module", "rules", "uncertain", snap.Uncertain)
	}
}

// StartPrepared bắt đầu sáng tác bằng prompt khởi động đã được sắp xếp xong.
func (h *Host) StartPrepared(promptText string) error {
	h.mu.Lock()
	if h.lifecycle == lifecycleRunning {
		h.mu.Unlock()
		return fmt.Errorf("already running")
	}
	if h.cocreating {
		h.mu.Unlock()
		return fmt.Errorf("đang đồng sáng tác giai đoạn, hãy kết thúc đồng sáng tác trước")
	}
	h.mu.Unlock()

	promptText = strings.TrimSpace(promptText)
	if promptText == "" {
		return fmt.Errorf("prompt is required")
	}
	if err := h.budget.Refuse(); err != nil {
		return err
	}
	if err := h.store.Checkpoints.Reset(); err != nil {
		return fmt.Errorf("reset checkpoints: %w", err)
	}
	if err := h.store.Progress.Init("", 0); err != nil {
		return fmt.Errorf("init progress: %w", err)
	}

	slog.Info("Bắt đầu sáng tác", "module", "host", "prompt_len", len(promptText))
	h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: "Bắt đầu sáng tác", Level: "info"})
	h.observer.setAborting(false)
	// Đặt lại theo dõi lặp và bật định tuyến trước, rồi mới khởi động Prompt, tránh sự kiện vòng đầu đến trước Enable
	h.router.ResetRepeat()
	h.router.Enable()
	if err := h.coordinator.Prompt(context.Background(), promptText); err != nil {
		return fmt.Errorf("prompt: %w", err)
	}
	// Chủ động dispatch lệnh đầu tiên: nếu đã vào giai đoạn viết (Phase=Writing), Host hạ lệnh ngay;
	// giai đoạn quy hoạch Route trả về nil, không có tác dụng phụ.
	h.router.Dispatch()

	h.mu.Lock()
	h.lifecycle = lifecycleRunning
	h.mu.Unlock()
	go h.waitDone()
	return nil
}

// Resume chế độ khôi phục: tạo resume prompt từ checkpoint + progress rồi khởi động.
func (h *Host) Resume() (string, error) {
	h.mu.Lock()
	if h.lifecycle == lifecycleRunning {
		h.mu.Unlock()
		return "", fmt.Errorf("already running")
	}
	if h.cocreating {
		h.mu.Unlock()
		return "", fmt.Errorf("đang đồng sáng tác giai đoạn, hãy kết thúc đồng sáng tác trước")
	}
	h.mu.Unlock()

	prompt, label, err := buildResumePrompt(h.store)
	if err != nil {
		return "", err
	}
	if label == "" {
		return "", nil // Chế độ tạo mới, không có khôi phục
	}
	if err := h.budget.Refuse(); err != nil {
		return "", err
	}

	slog.Info("Khôi phục sáng tác", "module", "host", "label", label)
	h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: "Khôi phục sáng tác: " + label, Level: "info"})
	for _, w := range h.store.CheckConsistency() {
		slog.Warn("Cảnh báo nhất quán", "module", "host", "detail", w)
		h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: "Cảnh báo nhất quán: " + w, Level: "warn"})
	}
	// Sách cũ không có snapshot thì tạo lazily (chuẩn hóa theo system_defaults + file rules); đã có thì đọc rẻ.
	h.ensureUserRules()
	h.refreshWriterRestore()
	h.observer.setAborting(false)
	h.router.ResetRepeat()
	h.router.Enable()
	if err := h.coordinator.Prompt(context.Background(), prompt); err != nil {
		return "", fmt.Errorf("resume prompt: %w", err)
	}
	// Chủ động dispatch lệnh đầu tiên, tránh Coordinator chỉ trả văn bản cho resume prompt khiến StopGuard chặn liên tục.
	h.router.Dispatch()

	h.mu.Lock()
	h.lifecycle = lifecycleRunning
	h.mu.Unlock()
	go h.waitDone()
	return label, nil
}

// interventionMsg gói văn bản người dùng thành thông điệp can thiệp mà Coordinator nhận diện được.
// Steer và Continue dùng chung một framing: lệnh người dùng ở cả hai lối vào đều mang tiền tố `[Người dùng can thiệp]`,
// mới kích hoạt ổn định việc phân loại can thiệp của coordinator.md. Nếu không, văn bản trần của Continue sẽ vòng qua
// quy tắc định tuyến, Coordinator mất điểm neo phân loại nên phân nhầm subagent (từng khiến "sửa chương đã viết" bị
// phân cho writer rồi đụng chốt edit_chapter).
func interventionMsg(text string) agentcore.Message {
	return agentcore.UserMsg("[Người dùng can thiệp] " + text)
}

// Continue tiếp tục với prompt chỉ định. Gọi khi người dùng nhập ở khung sau khi dừng máy.
func (h *Host) Continue(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("text is required")
	}
	h.mu.Lock()
	if h.cocreating {
		h.mu.Unlock()
		return fmt.Errorf("đang đồng sáng tác giai đoạn, hãy kết thúc đồng sáng tác trước")
	}
	running := h.lifecycle == lifecycleRunning
	h.mu.Unlock()

	h.emitEvent(Event{Time: time.Now(), Category: "USER", Summary: "[Tiếp tục] " + text, Level: "info"})

	if running {
		h.coordinator.FollowUp(interventionMsg(text))
		return nil
	}
	// Sau khi dừng máy → tiêm và tự động khôi phục (run khôi phục cũng chịu ràng buộc tiền đề ngân sách)
	if err := h.budget.Refuse(); err != nil {
		return err
	}
	h.refreshWriterRestore()
	h.observer.setAborting(false)
	_, err := h.coordinator.Inject(interventionMsg(text))
	if err != nil {
		return fmt.Errorf("inject: %w", err)
	}
	h.mu.Lock()
	h.lifecycle = lifecycleRunning
	h.mu.Unlock()
	go h.waitDone()
	return nil
}

// Steer gửi can thiệp của người dùng.
func (h *Host) Steer(text string) {
	h.mu.Lock()
	running := h.lifecycle == lifecycleRunning
	h.mu.Unlock()

	h.emitEvent(Event{Time: time.Now(), Category: "USER", Summary: "[Người dùng can thiệp] " + text, Level: "info"})

	msg := interventionMsg(text)
	if running {
		if _, err := h.coordinator.Inject(msg); err != nil {
			slog.Error("steer inject thất bại", "module", "host", "err", err)
		}
		return
	}
	// Dừng máy: lưu persistent để lần khởi động tiếp + phản hồi trạng thái hệ thống ("đã lưu" là thông báo hệ thống ngoài sự kiện USER)
	_ = h.store.RunMeta.SetPendingSteer(text)
	h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: "Can thiệp đã lưu, sẽ có hiệu lực lần khởi động tiếp theo", Level: "info"})
}

// Abort tạm dừng coordinator hiện tại.
func (h *Host) Abort() bool {
	return h.abortWithEvent("Người dùng tạm dừng sáng tác thủ công", "warn")
}

// abortWithEvent thực thi tạm dừng với sự kiện lý do được chỉ định. Dừng máy do ngân sách và tạm dừng thủ công dùng chung cơ chế dừng,
// chỉ khác văn bản sự kiện (dừng do ngân sách = lệnh Abort người dùng ký trước, ngữ nghĩa tương đương tạm dừng thủ công).
func (h *Host) abortWithEvent(summary, level string) bool {
	h.mu.Lock()
	running := h.lifecycle == lifecycleRunning
	if running {
		h.lifecycle = lifecyclePaused
	}
	h.mu.Unlock()
	if !running {
		return false
	}
	// Đặt cờ phải trước coordinator.Abort: cancel truyền lan sẽ lập tức gây sự kiện thất bại stream init / subagent,
	// observer dựa vào cờ này nhận diện là nhiễu phái sinh abort và chặn lại.
	h.observer.setAborting(true)
	h.coordinator.Abort()
	h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: summary, Level: level})
	return true
}

// Close kết thúc coordinator và đóng kênh sự kiện.
//
// Ngữ nghĩa persistence Usage: hủy autoSaveLoop trước (nó tự flush trạng thái dirty lần cuối),
// rồi bổ sung một SaveNow đồng bộ để kết thúc. Lỗ hổng đã biết: sau AbortSilent nếu vẫn có LLM
// in-flight quay về, OnMessage → Record sẽ cập nhật bộ nhớ nhưng **không được persist**. Phần
// "vài trăm token cuối" bị mất này sẽ được session jsonl replay tự động bù lại lần khởi động tiếp.
func (h *Host) Close() {
	h.observer.setAborting(true)
	h.coordinator.AbortSilent()
	if h.budgetDetach != nil {
		h.budgetDetach()
		h.budgetDetach = nil
	}
	if h.usageCancel != nil {
		h.usageCancel()
		h.usageCancel = nil
	}
	if err := h.usage.SaveNow(); err != nil {
		slog.Warn("Ghi usage trước khi thoát thất bại", "module", "usage", "err", err)
	}
	h.closeOnce.Do(func() {
		close(h.done)
		close(h.events)
		close(h.streamCh)
	})
}

// waitDone chờ coordinator dừng và phát sự kiện trạng thái cuối.
//
// Không tự chạy tiếp. Run kết thúc = Host vào trạng thái cuối:
//   - Phase=Complete  → đánh dấu completed, phát sự kiện "sáng tác hoàn thành"
//   - Khác            → đánh dấu idle, phát sự kiện "Coordinator dừng"
//
// Người dùng muốn tiếp tục sáng tác chỉ có hai đường: Continue thủ công (tiêm khi dừng máy) hoặc khởi động lại qua Resume.
// Xem docs/architecture.md §13.3, §8.3.
func (h *Host) waitDone() {
	h.coordinator.WaitForIdle()
	h.observer.finalize()

	h.mu.Lock()
	progress, _ := h.store.Progress.Load()
	if progress != nil && progress.Phase == domain.PhaseComplete {
		h.lifecycle = lifecycleCompleted
		summary := fmt.Sprintf("Sáng tác hoàn thành: %d chương %d chữ", len(progress.CompletedChapters), progress.TotalWordCount)
		h.mu.Unlock()
		slog.Info(summary, "module", "host")
		h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: summary, Level: "success"})
		h.notifier.Send(notify.Notification{
			Kind: "run_end", Level: "info", Title: "ainovel: sáng tác hoàn thành",
			Body: h.runEndBody(progress.NovelName, summary),
		})
	} else {
		wasRunning := h.lifecycle == lifecycleRunning
		if wasRunning {
			h.lifecycle = lifecycleIdle
		}
		completed := 0
		name := ""
		if progress != nil {
			completed = len(progress.CompletedChapters)
			name = progress.NovelName
		}
		h.mu.Unlock()
		if wasRunning {
			summary := fmt.Sprintf("Coordinator dừng (đã hoàn thành %d chương)", completed)
			slog.Warn(summary, "module", "host")
			h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: summary, Level: "warn"})
			h.notifier.Send(notify.Notification{
				Kind: "run_end", Level: "warn", Title: "ainovel: sáng tác dừng",
				Body: h.runEndBody(name, summary),
			})
		}
	}

	select {
	case h.done <- struct{}{}:
	default:
	}
}

// runEndBody lắp ráp nội dung thông báo run_end: tên sách + tóm tắt tiến độ + chi phí lũy kế.
func (h *Host) runEndBody(novelName, summary string) string {
	if name := strings.TrimSpace(novelName); name != "" {
		summary = "《" + name + "》" + summary
	}
	cost, _, _, _, _ := h.usage.Totals()
	if cost > 0 {
		summary += fmt.Sprintf(" · Chi phí $%.2f", cost)
	}
	return summary
}

// ── Kênh ──

// StreamClearSentinel gửi một tin duy nhất qua streamCh để báo hiệu "xóa round streaming hiện tại".
// Không dùng clearCh riêng nữa — hai kênh không có thứ tự khiến ✻ header thường rơi vào cuối round trước.
const StreamClearSentinel = "\x00\x00CLEAR\x00\x00"

func (h *Host) Events() <-chan Event        { return h.events }
func (h *Host) Stream() <-chan string       { return h.streamCh }
func (h *Host) Done() <-chan struct{}       { return h.done }
func (h *Host) Dir() string                 { return h.store.Dir() }
func (h *Host) AskUser() *tools.AskUserTool { return h.askUser }

// ── Phát sự kiện ──

func (h *Host) emitEvent(ev Event) {
	defer func() { recover() }()
	// Điểm vào slog duy nhất cho tất cả sự kiện. Cả sự kiện agentcore mà observer dịch
	// lẫn sự kiện SYSTEM tự phát của Host (Start/Abort/Resume...) đều ghi log ở đây,
	// tránh ESC abort và kết thúc ngoài không phân biệt được trên tui.log.
	if ev.Summary != "" || ev.Detail != "" {
		level := slog.LevelInfo
		switch ev.Level {
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
		// Log ghi đầy đủ Detail (để gỡ lỗi, không cắt); chỉ khi Detail rỗng mới lui về Summary.
		msg := ev.Detail
		if msg == "" {
			msg = ev.Summary
		}
		attrs := []any{"module", "event", "category", ev.Category, "agent", ev.Agent}
		if ev.Kind != "" {
			attrs = append(attrs, "kind", ev.Kind)
		}
		slog.Log(context.Background(), level, msg, attrs...)
	}
	select {
	case h.events <- ev:
	default:
		select {
		case <-h.events:
		default:
		}
		select {
		case h.events <- ev:
		default:
		}
	}
}

func (h *Host) emitDelta(delta string) {
	defer func() { recover() }()
	select {
	case h.streamCh <- delta:
	default:
		select {
		case <-h.streamCh:
		default:
		}
		select {
		case h.streamCh <- delta:
		default:
		}
	}
}

func (h *Host) emitClear() {
	// Đi qua streamCh bằng "sentinel", đảm bảo đến TUI theo thứ tự trên cùng một kênh với emitDelta.
	h.emitDelta(StreamClearSentinel)
}

// ── Snapshot (Tổng hợp trạng thái TUI) ──

func (h *Host) Snapshot() UISnapshot {
	h.mu.Lock()
	state := h.lifecycle
	provider, model, _ := h.models.CurrentSelection("default")
	h.mu.Unlock()

	// Phân giải động cửa sổ ngữ cảnh của mô hình hiện tại, Snapshot lần tiếp theo sau /model switch tự động phản ánh
	modelWindow, _ := h.cfg.ResolveContextWindow(model)
	cost, tokIn, tokOut, cacheRead, cacheWrite := h.usage.Totals()
	saved := h.usage.SavedUSD()
	overallCapable := h.usage.OverallCacheCapable()
	recentRead, recentInput, recentSamples := h.usage.OverallRecent()
	perAgent := h.usage.PerAgent()
	cacheStats := make([]AgentCacheStat, 0, len(perAgent))
	for _, a := range perAgent {
		cacheStats = append(cacheStats, AgentCacheStat{
			Role:            a.Role,
			Input:           a.Input,
			Output:          a.Output,
			CacheRead:       a.CacheRead,
			CacheWrite:      a.CacheWrite,
			Cost:            a.Cost,
			Saved:           a.Saved,
			CacheCapable:    a.CacheCapable,
			RecentCacheRead: a.RecentCacheRead,
			RecentInput:     a.RecentInput,
			RecentSamples:   a.RecentSamples,
		})
	}
	perModel := h.usage.PerModel()
	modelStats := make([]AgentCacheStat, 0, len(perModel))
	for _, a := range perModel {
		modelStats = append(modelStats, AgentCacheStat{
			Model:        a.Model,
			Input:        a.Input,
			Output:       a.Output,
			CacheRead:    a.CacheRead,
			CacheWrite:   a.CacheWrite,
			Cost:         a.Cost,
			Saved:        a.Saved,
			CacheCapable: a.CacheCapable,
		})
	}

	snap := UISnapshot{
		Provider:               provider,
		ModelName:              model,
		ModelContextWindow:     modelWindow,
		ThinkingLevel:          h.cfg.ResolveReasoningEffort("default"),
		Style:                  h.cfg.Style,
		RuntimeState:           string(state),
		IsRunning:              state == lifecycleRunning,
		TotalInputTokens:       tokIn,
		TotalOutputTokens:      tokOut,
		TotalCacheReadTokens:   cacheRead,
		TotalCacheWriteTokens:  cacheWrite,
		TotalCostUSD:           cost,
		TotalSavedUSD:          saved,
		BudgetLimitUSD:         h.budget.Limit(),
		OverallCacheCapable:    overallCapable,
		OverallRecentCacheRead: recentRead,
		OverallRecentInput:     recentInput,
		OverallRecentSamples:   recentSamples,
		CachePerAgent:          cacheStats,
		CachePerModel:          modelStats,
		MissingAssistantUsage:  h.usage.MissingAssistantUsage(),
	}

	progress, _ := h.store.Progress.Load()
	if progress != nil {
		snap.NovelName = strings.TrimSpace(progress.NovelName)
		snap.Phase = string(progress.Phase)
		snap.Flow = string(progress.Flow)
		snap.CurrentChapter = progress.CurrentChapter
		snap.TotalChapters = progress.TotalChapters
		snap.CompletedCount = len(progress.CompletedChapters)
		snap.TotalWordCount = progress.TotalWordCount
		snap.InProgressChapter = progress.InProgressChapter
		snap.PendingRewrites = progress.PendingRewrites
		snap.RewriteReason = progress.RewriteReason
		snap.Layered = progress.Layered
		if progress.CurrentVolume > 0 {
			snap.CurrentVolumeArc = fmt.Sprintf("Tập %d·Cung %d", progress.CurrentVolume, progress.CurrentArc)
		}
	}
	if snap.NovelName == "" {
		if premise, _ := h.store.Outline.LoadPremise(); premise != "" {
			snap.NovelName = domain.ExtractNovelNameFromPremise(premise)
		}
	}
	if meta, _ := h.store.RunMeta.Load(); meta != nil {
		snap.PendingSteer = meta.PendingSteer
	}

	snap.Agents = h.observer.agentSnapshots()
	h.fillContextStatus(&snap)
	snap.StatusLabel = deriveStatusLabel(snap)

	// Nhãn khôi phục
	if _, label, err := buildResumePrompt(h.store); err == nil && label != "" {
		snap.RecoveryLabel = label
	}

	h.fillDetails(&snap, progress)

	return snap
}

// fillContextStatus điền thông tin sức khỏe ngữ cảnh Coordinator.
func (h *Host) fillContextStatus(snap *UISnapshot) {
	if h.coordinator == nil {
		return
	}
	if usage := h.coordinator.BaselineContextUsage(); usage != nil {
		snap.ContextTokens = usage.Tokens
		snap.ContextWindow = usage.ContextWindow
		snap.ContextPercent = usage.Percent
	}
	if ctx := h.coordinator.ContextSnapshot(); ctx != nil {
		snap.ContextScope = ctx.Scope
		snap.ContextStrategy = ctx.LastStrategy
		snap.ContextActiveMessages = ctx.ActiveMessages
		snap.ContextSummaryCount = ctx.SummaryMessages
		snap.ContextCompactedCount = ctx.LastCompactedCount
		snap.ContextKeptCount = ctx.LastKeptCount
		if snap.ContextTokens == 0 {
			if ctx.BaselineUsage != nil {
				snap.ContextTokens = ctx.BaselineUsage.Tokens
				snap.ContextWindow = ctx.BaselineUsage.ContextWindow
				snap.ContextPercent = ctx.BaselineUsage.Percent
			} else if ctx.Usage != nil {
				snap.ContextTokens = ctx.Usage.Tokens
				snap.ContextWindow = ctx.Usage.ContextWindow
				snap.ContextPercent = ctx.Usage.Percent
			}
		}
	}
}

// fillDetails điền khu vực chi tiết: thiết định, nhân vật, commit/review/tóm tắt gần nhất.
func (h *Host) fillDetails(snap *UISnapshot, progress *domain.Progress) {
	if premise, _ := h.store.Outline.LoadPremise(); premise != "" {
		snap.Premise = truncate(premise, 80)
	}
	if outline, _ := h.store.Outline.LoadOutline(); len(outline) > 0 {
		for _, e := range outline {
			snap.Outline = append(snap.Outline, OutlineSnapshot{
				Chapter: e.Chapter, Title: e.Title, CoreEvent: e.CoreEvent,
			})
		}
	}
	if progress != nil && progress.Layered {
		if compass, _ := h.store.Outline.LoadCompass(); compass != nil {
			snap.CompassDirection = compass.EndingDirection
			snap.CompassScale = compass.EstimatedScale
		}
		if volumes, _ := h.store.Outline.LoadLayeredOutline(); len(volumes) > 0 {
			for _, v := range volumes {
				if v.Index > progress.CurrentVolume {
					snap.NextVolumeTitle = v.Title
					break
				}
			}
		}
	}
	if chars, _ := h.store.Characters.Load(); len(chars) > 0 {
		for _, c := range chars {
			label := c.Name
			if c.Role != "" {
				label += " (" + c.Role + ")"
			}
			snap.Characters = append(snap.Characters, label)
		}
	}
	if ledger, _ := h.store.Cast.Load(); len(ledger) > 0 {
		snap.SupportingCount = len(ledger)
		recent, _ := h.store.Cast.RecentActive(5)
		for _, e := range recent {
			label := e.Name
			if e.BriefRole != "" {
				label += " (" + e.BriefRole + ")"
			}
			snap.RecentSupporting = append(snap.RecentSupporting, label)
		}
	}
	if progress != nil && len(progress.CompletedChapters) > 0 {
		lastCh := progress.CompletedChapters[len(progress.CompletedChapters)-1]
		wc := progress.ChapterWordCounts[lastCh]
		snap.LastCommitSummary = fmt.Sprintf("Chương %d %d chữ", lastCh, wc)
	}
	currentCh := 1
	if progress != nil && len(progress.CompletedChapters) > 0 {
		currentCh = progress.CompletedChapters[len(progress.CompletedChapters)-1]
	}
	if review, err := h.store.World.LoadLastReview(currentCh); err == nil && review != nil {
		snap.LastReviewSummary = fmt.Sprintf("verdict=%s %d vấn đề", review.Verdict, len(review.Issues))
		if len(review.AffectedChapters) > 0 {
			snap.LastReviewSummary += fmt.Sprintf(" ảnh hưởng %v", review.AffectedChapters)
		}
	}
	if cp := h.store.Checkpoints.LatestGlobal(); cp != nil {
		snap.LastCheckpointName = fmt.Sprintf("%s.%s", cp.Scope, cp.Step)
	}
	if progress != nil {
		for i := len(progress.CompletedChapters) - 1; i >= 0 && len(snap.RecentSummaries) < 2; i-- {
			ch := progress.CompletedChapters[i]
			if summary, err := h.store.Summaries.LoadSummary(ch); err == nil && summary != nil {
				snap.RecentSummaries = append(snap.RecentSummaries,
					fmt.Sprintf("Chương %d: %s", ch, truncate(summary.Summary, 50)))
			}
		}
	}
}

func deriveStatusLabel(s UISnapshot) string {
	switch {
	case s.Phase == string(domain.PhaseComplete):
		return "COMPLETE"
	case s.Flow == string(domain.FlowReviewing):
		return "REVIEW"
	case s.Flow == string(domain.FlowRewriting) || s.Flow == string(domain.FlowPolishing):
		return "REWRITE"
	case s.RuntimeState == "running":
		return "RUNNING"
	default:
		return "READY"
	}
}

// ── Quản lý mô hình ──

func (h *Host) ConfiguredProviders() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	providers := make([]string, 0, len(h.cfg.Providers))
	for name := range h.cfg.Providers {
		providers = append(providers, name)
	}
	sort.Strings(providers)
	return providers
}

func (h *Host) ConfiguredModels(provider string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cfg.CandidateModels(provider)
}

func (h *Host) CurrentModelSelection(role string) (string, string, bool) {
	return h.models.CurrentSelection(role)
}

func (h *Host) SwitchModel(role, provider, model string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if provider == "" || model == "" {
		return fmt.Errorf("provider and model are required")
	}
	if err := h.models.Swap(role, provider, model); err != nil {
		return err
	}
	if role == "" || role == "default" {
		h.cfg.Provider = provider
		h.cfg.ModelName = model
	} else {
		if h.cfg.Roles == nil {
			h.cfg.Roles = make(map[string]bootstrap.RoleConfig)
		}
		rc := h.cfg.Roles[role]
		rc.Provider = provider
		rc.Model = model
		h.cfg.Roles[role] = rc
	}
	h.normalizeThinkingLocked(role)
	if path := bootstrap.DefaultConfigPath(); path != "" {
		if err := bootstrap.SaveConfig(path, h.cfg); err != nil {
			slog.Warn("Luu cau hinh that bai", "module", "host", "err", err)
		}
	}
	h.applyThinkingLocked(role)
	// Khi chuyen sang mo hinh chua dang ky, in mot dong warn, nhac nguoi dung dang dung du phong 128k — van dai de bi nen som.
	logRole := role
	if logRole == "" {
		logRole = "default"
	}
	window, source := h.cfg.ResolveContextWindow(model)
	bootstrap.LogContextWindowChoice(logRole, model, window, source)

	// Khi chuyen sang default/coordinator, dong bo cua so va reserve cua coordinator engine.
	// writer/architect/editor dung ContextManagerFactory tu xay lai theo mo hinh moi, khong can dong bo.
	// Khong dong bo se gay: khi chuyen 1M->128k coordinator engine van tinh threshold theo 1M,
	// tich luy messages vuot 128k se API loi; khi chuyen 128k->1M nguong bi ghim o 96k, lang phi ngu canh dai.
	//
	// Quan trong: phai dung models.CurrentSelection("coordinator") lay mo hinh "coordinator thuc su dang dung"
	// de tinh cua so -- khong dung truc tiep model dich chuyen. Khi nguoi dung cau hinh roles.coordinator mo hinh rieng,
	// chuyen default khong anh huong mo hinh thuc cua coordinator; dung cua so dich de SetContextWindow se sai
	// dat nguong coordinator sang gia tri khong lien quan (vi du: chuyen default sang mo hinh 1M se keo nguong
	// coordinator 200k len 891k, viet qua 200k se no API ngay).
	if h.coordinatorCtxMgr != nil && (role == "" || role == "default" || role == "coordinator") {
		_, coordinatorModel, _ := h.models.CurrentSelection("coordinator")
		coordinatorWindow, coordSource := h.cfg.ResolveContextWindow(coordinatorModel)
		h.coordinator.SetContextWindow(coordinatorWindow)
		h.coordinatorCtxMgr.SetContextWindow(coordinatorWindow)
		h.coordinatorCtxMgr.SetReserveTokens(bootstrap.CompactReserveTokens(coordinatorWindow))
		// Khi mo hinh thuc cua coordinator khac mo hinh dich chuyen (nguoi dung chuyen default nhung coordinator co role rieng),
		// dong LogContextWindowChoice o tren in cua so cua default, khong khop voi gia tri thuc te; bo sung mot dong.
		if coordinatorModel != model {
			bootstrap.LogContextWindowChoice("coordinator", coordinatorModel, coordinatorWindow, coordSource)
		}
	}

	h.emitEvent(Event{
		Time:     time.Now(),
		Category: "SYSTEM",
		Summary:  fmt.Sprintf("Da chuyen mo hinh: %s -> %s/%s", role, provider, model),
		Level:    "info",
	})
	return nil
}

// concreteThinkingRoles la cac role cu the co the ap dung cuong douy luan (tuong ung voi routing cua agents.ApplyThinking).
// Khi goi default thi ap dung lai lan luot theo ResolveReasoningEffort cua tung role.
var concreteThinkingRoles = []string{"coordinator", "architect", "writer", "editor"}

// CurrentThinking tra ve chuoi cuong douy luan hien tai cua mot role (de panel /model dong bo gia tri hien tai).
func (h *Host) CurrentThinking(role string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cfg.ResolveReasoningEffort(strings.ToLower(strings.TrimSpace(role)))
}

func (h *Host) AvailableThinking(role string) []agentcore.ThinkingLevel {
	h.mu.Lock()
	model := h.models.ForRole(strings.ToLower(strings.TrimSpace(role)))
	h.mu.Unlock()
	return agents.AvailableThinkingForModel(model)
}

func (h *Host) normalizeThinkingLocked(role string) agentcore.ThinkingLevel {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" || role == "default" {
		parsed, _ := agents.ParseThinkingLevel(h.cfg.ReasoningEffort)
		for _, r := range concreteThinkingRoles {
			resolved, ok := agents.ResolveThinkingForModel(h.models.ForRole(r), parsed)
			if !ok || resolved != parsed {
				h.cfg.ReasoningEffort = string(resolved)
				return resolved
			}
		}
		h.cfg.ReasoningEffort = string(parsed)
		return parsed
	}

	_, hasRoleThinking := h.cfg.Roles[role]
	hasRoleThinking = hasRoleThinking && h.cfg.Roles[role].ReasoningEffort != ""
	parsed, _ := agents.ParseThinkingLevel(h.cfg.ResolveReasoningEffort(role))
	resolved, _ := agents.ResolveThinkingForModel(h.models.ForRole(role), parsed)
	if !hasRoleThinking {
		if resolved != parsed {
			h.cfg.ReasoningEffort = string(resolved)
		}
		return resolved
	}
	if h.cfg.Roles == nil {
		h.cfg.Roles = make(map[string]bootstrap.RoleConfig)
	}
	rc := h.cfg.Roles[role]
	rc.ReasoningEffort = string(resolved)
	h.cfg.Roles[role] = rc
	return resolved
}

func (h *Host) applyThinkingLocked(role string) {
	if h.thinkingApplier == nil {
		return
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" || role == "default" {
		for _, r := range concreteThinkingRoles {
			lv, _ := agents.ParseThinkingLevel(h.cfg.ResolveReasoningEffort(r))
			h.thinkingApplier(r, lv)
		}
		return
	}
	lv, _ := agents.ParseThinkingLevel(h.cfg.ResolveReasoningEffort(role))
	h.thinkingApplier(role, lv)
}

// SetRoleThinking dat cuong douy luan cho mot role (hoac default): kiem tra->luu tru->dong bo live agent->su kien.
// Co cau tuong tu SwitchModel; doc lap voi lua chon mo hinh, co the dieu chinh rieng biet. level rong = khong ghi de (ke thua).
func (h *Host) SetRoleThinking(role, level string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	parsed, err := agents.ParseThinkingLevel(level)
	if err != nil {
		return err
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" || role == "default" {
		for _, r := range concreteThinkingRoles {
			if resolved, ok := agents.ResolveThinkingForModel(h.models.ForRole(r), parsed); !ok || resolved != parsed {
				parsed = resolved
				break
			}
		}
	} else {
		parsed, _ = agents.ResolveThinkingForModel(h.models.ForRole(role), parsed)
	}
	// Luu tru: role cu the ghi vao Roles[role].ReasoningEffort, default/"" ghi vao ReasoningEffort cap cao nhat.
	if role == "" || role == "default" {
		h.cfg.ReasoningEffort = string(parsed)
	} else {
		if h.cfg.Roles == nil {
			h.cfg.Roles = make(map[string]bootstrap.RoleConfig)
		}
		rc := h.cfg.Roles[role]
		rc.ReasoningEffort = string(parsed)
		h.cfg.Roles[role] = rc
	}
	if path := bootstrap.DefaultConfigPath(); path != "" {
		if err := bootstrap.SaveConfig(path, h.cfg); err != nil {
			slog.Warn("Luu cau hinh that bai", "module", "host", "err", err)
		}
	}

	// Dong bo live: role cu the ap dung truc tiep; default thi duyet qua cac role cu the ap dung lai theo ResolveReasoningEffort
	// (role da bi ghi de cap role giu nguyen, chua bi ghi de thi ap dung default moi).
	h.applyThinkingLocked(role)

	logRole := role
	if logRole == "" {
		logRole = "default"
	}
	shown := string(parsed)
	if shown == "" {
		shown = "Mac dinh (ke thua)"
	}
	h.emitEvent(Event{
		Time:     time.Now(),
		Category: "SYSTEM",
		Summary:  fmt.Sprintf("Da chuyen cuong douy luan: %s -> %s", logRole, shown),
		Level:    "info",
	})
	return nil
}

// ── Phát lại sự kiện ──

func (h *Host) ReplayQueue(afterSeq int64) ([]domain.RuntimeQueueItem, error) {
	if h.store == nil || h.store.Runtime == nil {
		return nil, nil
	}
	return h.store.Runtime.LoadQueueAfter(afterSeq)
}

// ── Đồng sáng tác ──

// CoCreateStream khoi dong lanh dong sang tao: lam ro yeu cau tu dau, tao ra chi thi sang tac cho ca cuon sach.
func (h *Host) CoCreateStream(ctx context.Context, history []CoCreateMessage, onProgress func(kind, text string)) (CoCreateReply, error) {
	return coCreateStream(ctx, h.models, h.store.Sessions, coCreateSystemPrompt, history, onProgress)
}

// StageCoCreateStream dong sang tao giai doan: lap ke hoach huong di tiep theo dua tren noi dung da viet.
// System prompt = prompt giai doan + tom tat trang thai truyen hien tai, de tro ly biet "da viet gi roi".
func (h *Host) StageCoCreateStream(ctx context.Context, history []CoCreateMessage, onProgress func(kind, text string)) (CoCreateReply, error) {
	return coCreateStream(ctx, h.models, h.store.Sessions, stageSystemPrompt(h.store), history, onProgress)
}

// stagePlanPrefix gói "brief hướng đi tiếp theo" do đồng sáng tác sinh ra thành một can thiệp hoạch định giai đoạn,
// giao Coordinator phán định. Chỉ dán dấu sự thật [Hoạch định giai đoạn] + phát biểu trung tính, không viết chết
// "làm sao triển khai" — định tuyến cụ thể (compass / architect / save_user_rules) giao cho tiêu chí "Hoạch định
// giai đoạn" của coordinator.md, tránh tạo nguồn sự thật thứ hai với prompt, cũng không chặn yêu cầu phong cách đi
// qua user_rules (giữ "phán định phân loại thuộc về LLM"). Continue rồi xếp chồng thêm tiền tố [Người dùng can thiệp].
const stagePlanPrefix = "[Hoạch định giai đoạn] Tôi tạm dừng sáng tác, cùng trợ lý đồng sáng tác rà soát hướng đi tiếp theo dưới đây, hãy theo phân loại can thiệp của bạn mà phán định cách triển khai, rồi tiếp tục sáng tác. Hướng đi tiếp theo như sau:\n\n"

// PauseForCoCreate vao dong sang tao giai doan: dat co chiem dung dong sang tao, neu dang chay thi tam dung coordinator luon.
// Tra ve false neu khong the vao (ca sach da hoan thanh hoac da trong dong sang tao), caller bo qua la duoc.
// Co chiem dung chan cac can thiep dong thoi import/simulate/start/resume/continue trong cua so dong sang tao--
// sau khi tam dung khi dang chay lifecycle=paused, mutex ==running hien tai mat hieu luc, dung co nay bu vao;
// Da dung (idle/paused) cung cho phep vao, sau khi lap ke hoach xong co the chay tiep qua Continue.
func (h *Host) PauseForCoCreate() bool {
	h.mu.Lock()
	if h.cocreating || h.lifecycle == lifecycleCompleted {
		h.mu.Unlock()
		return false
	}
	h.cocreating = true
	running := h.lifecycle == lifecycleRunning
	h.mu.Unlock()

	// Khi dang chay, tai su dung abortWithEvent dung may (running->paused + setAborting + Abort + su kien), dong tu voi
	// tam dung thu cong, khong sao chep lai; Da dung (idle/paused) chi dat co, lap ke hoach xong chay tiep qua Continue.
	if running {
		h.abortWithEvent("Vao dong sang tao giai doan, sang tac da tam dung", "info")
	} else {
		h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: "Vao dong sang tao giai doan", Level: "info"})
	}
	return true
}

// ResumeFromCoCreate ket thuc dong sang tao giai doan: nap huong di tiep theo do dong sang tao tao ra lam can thiep va khoi phuc sang tac.
// Sau khi xoa co chiem dung, tai su dung duong nap can thiep dung may cua Continue (bi rang buoc tien ngan sach).
// Ghi chu: khi draft rong thi tra ve som, khong xoa co la co y (dong sang tao chua ket thuc); Guard canStart() phia TUI
// dung cung tieu chi "khong rong" nhu o day, dam bao duong nay khong the truy cap duoc, cocreating se khong bi ro.
func (h *Host) ResumeFromCoCreate(draft string) error {
	draft = strings.TrimSpace(draft)
	if draft == "" {
		return fmt.Errorf("draft is required")
	}
	h.mu.Lock()
	if !h.cocreating {
		h.mu.Unlock()
		return fmt.Errorf("not in co-create")
	}
	h.cocreating = false
	h.mu.Unlock()

	// Abort cua PauseForCoCreate la bat dong bo: truoc khi khoi phuc doi run cu hoi tu, tro ve tien de "dung may that su"
	// nhat quan voi Continue sau khi tam dung thu cong, tranh steer lenh tiep tuc vao run cu dang thoat.
	// Khi vao dong sang tao o trang thai khong chay (chua Abort) coordinator da idle, WaitForIdle tra ve ngay.
	h.coordinator.WaitForIdle()

	h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: "Dong sang tao giai doan hoan tat, da nap huong di tiep theo va khoi phuc sang tac", Level: "info"})
	return h.Continue(stagePlanPrefix + draft)
}

// CancelCoCreate huy bo dong sang tao giai doan: xoa co chiem dung, giu trang thai tam dung (nguoi dung co the tiep tuc o o nhap hoac khoi dong lai Resume).
func (h *Host) CancelCoCreate() {
	h.mu.Lock()
	if !h.cocreating {
		h.mu.Unlock()
		return
	}
	h.cocreating = false
	h.mu.Unlock()
	h.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Summary: "Da thoat dong sang tao giai doan, sang tac van tam dung (co the tiep tuc o o nhap)", Level: "info"})
}

// ── Cong cu ──

func (h *Host) refreshWriterRestore() {
	if h.writerRestore != nil {
		h.writerRestore.Refresh(h.store)
	}
}

func truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

// ImportFrom khoi dong mot lan nhap tieu thuyet tu ben ngoai suy nguoc: phan tach -> suy nguoc foundation -> phan tich tung chuong luu dia.
// Xung dot voi Coordinator; sau khi nhap xong caller co the goi Resume() tiep tuc viet ngay.
// Kenh su kien tra ve duoc imp.Run dong, caller chiu trach nhiem tieu thu (day thi huy bo de tranh chan goroutine phan tich).
func (h *Host) ImportFrom(ctx context.Context, opts imp.Options) (<-chan imp.Event, error) {
	if err := h.guardExclusive("nhap"); err != nil {
		return nil, err
	}

	deps := imp.Deps{
		Store:      h.store,
		CommitTool: tools.NewCommitChapterTool(h.store),
		LLM:        h.models.ForRole("architect"),
		Prompts: imp.Prompts{
			Foundation: h.bundle.Prompts.ImportFoundation,
			Analyzer:   h.bundle.Prompts.ImportAnalyzer,
		},
	}
	return imp.Run(ctx, deps, opts)
}

// Simulate doc thu muc simulate va tao hoac cap nhat tang dan ho so phuong phap viet.
func (h *Host) Simulate(ctx context.Context) (<-chan sim.Event, error) {
	if err := h.guardExclusive("tao ho so phuong phap viet"); err != nil {
		return nil, err
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working dir: %w", err)
	}
	deps := sim.Deps{
		Store: h.store,
		LLM:   h.models.ForRole("architect"),
		Prompts: sim.Prompts{
			Source: h.bundle.Prompts.SimulationSource,
			Merge:  h.bundle.Prompts.SimulationMerge,
		},
	}
	return sim.Run(ctx, deps, sim.Options{SourceDir: filepath.Join(wd, "simulate")})
}

// ImportSimulationProfile nhap ho so phuong phap viet da tao truoc do.
func (h *Host) ImportSimulationProfile(ctx context.Context, path string) (<-chan sim.Event, error) {
	if err := h.guardExclusive("nhap ho so phuong phap viet"); err != nil {
		return nil, err
	}
	return sim.RunImport(ctx, h.store, path)
}

// guardExclusive kiem tra chiem dung doc quyen: khi coordinator dang chay hoac trong cua so dong sang tao giai doan
// thi tu choi cac diem vao co the ghi de trang thai (import/simulate). Bu vao khe dong thoi chi kiem tra ==running trong khi paused.
func (h *Host) guardExclusive(action string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case h.lifecycle == lifecycleRunning:
		return fmt.Errorf("coordinator dang chay, vui long tam dung truoc khi %s", action)
	case h.cocreating:
		return fmt.Errorf("dong sang tao giai doan dang dien ra, vui long ket thuc dong sang tao truoc khi %s", action)
	}
	return nil
}

// Export xuat cac chuong da hoan thanh ra file ben ngoai (hien tai chi ho tro TXT).
//
// Khac voi ImportFrom: xuat la thao tac chi doc (khong thay doi Progress / Checkpoint),
// do do **khong yeu cau Coordinator ranh**--co the xuat "san pham hien tai" bat cu luc nao trong khi dang viet.
// Chi doc snapshot nhat quan cua Progress.CompletedChapters + ban thao cuoi chuong + de cuong + premise.
func (h *Host) Export(ctx context.Context, opts exp.Options) (*exp.Result, error) {
	return exp.Run(ctx, exp.Deps{Store: h.store}, opts)
}

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/voocel/agentcore"
	corecontext "github.com/voocel/agentcore/context"
	"github.com/voocel/agentcore/llm"
	"github.com/voocel/agentcore/subagent"
	"github.com/voocel/ainovel-cli/assets"
	"github.com/voocel/ainovel-cli/internal/agents/ctxpack"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host/reminder"
	"github.com/voocel/ainovel-cli/internal/rules"
	"github.com/voocel/ainovel-cli/internal/store"
	"github.com/voocel/ainovel-cli/internal/tools"
	"github.com/voocel/ainovel-cli/internal/userrules"
)

// agentToRole chuẩn hóa tên subagent thành tên role mà ModelSet nhận ra.
// architect_short / architect_long cùng dùng chung một cấu hình role architect.
// Đồng nghĩa với host.agentRoleName, vì build và host không phụ thuộc nhau nên mỗi bên giữ một bản.
func agentToRole(name string) string {
	if strings.HasPrefix(name, "architect_") {
		return "architect"
	}
	return name
}

// subagentMaxRetries là giới hạn retry LLM thống nhất cho tất cả SubAgentConfig và Coordinator.
// Chiến lược backoff: lũy thừa (bị giới hạn bởi maxDelay), ưu tiên tuân theo server Retry-After.
// Kết hợp với ToolsAreIdempotent=true để các lỗi retryable như stream-idle / 503 / rung mạng ngắn
// có thể retry tại lớp subagent thay vì ném toàn bộ subagent về coordinator để dispatch lại.
// Quy tắc sắt 1 của dự án đảm bảo công cụ ghi đi theo checkpoint+digest idempotent, retry là an toàn.
const subagentMaxRetries = 7

// UsageRecorder là callback dùng lượng tùy chọn của BuildCoordinator; chữ ký giống OnMessage,
// được gọi mỗi tin nhắn agent, do lớp Host chịu trách nhiệm tổng hợp. nil nghĩa là không theo dõi.
type UsageRecorder func(agentName string, msg agentcore.AgentMessage)

// FlowBoundaryHook runs synchronously after a Coordinator tool that advances
// the durable story state succeeds. Host uses it to queue the next flow
// instruction before the Coordinator gets another LLM turn.
type FlowBoundaryHook func(toolName string)

// ApplyThinking áp dụng cường độ suy luận của một vai trò cụ thể vào live agent (dùng cho điều chỉnh /model runtime).
// coordinator → Agent.SetThinkingLevel; architect → hai subagent architect_*;
// writer/editor → subagent tương ứng. level rỗng = giữ mặc định model/provider. Tên role khác bị bỏ qua.
type ApplyThinking func(role string, level agentcore.ThinkingLevel)

// ParseThinkingLevel chuyển đổi chuỗi cấu hình thành agentcore.ThinkingLevel.
// "" hợp lệ (= không ghi đè/kế thừa); các giá trị còn lại phải là off/low/medium/high/xhigh/max,
// nếu không thì trả về error (khi khởi động hạ cấp thành rỗng và warn, runtime thì hiển thị error cho người dùng).
func ParseThinkingLevel(s string) (agentcore.ThinkingLevel, error) {
	lv := agentcore.NormalizeThinkingLevel(agentcore.ThinkingLevel(s))
	switch lv {
	case "", agentcore.ThinkingOff, agentcore.ThinkingLow, agentcore.ThinkingMedium,
		agentcore.ThinkingHigh, agentcore.ThinkingXHigh, agentcore.ThinkingMax:
		return lv, nil
	default:
		return "", fmt.Errorf("cường độ suy luận không hợp lệ %q (có thể chọn: off/low/medium/high/xhigh/max)", s)
	}
}

func ResolveThinkingForModel(model agentcore.ChatModel, level agentcore.ThinkingLevel) (agentcore.ThinkingLevel, bool) {
	return llm.ThinkingPolicyFor(model).Resolve(level)
}

func AvailableThinkingForModel(model agentcore.ChatModel) []agentcore.ThinkingLevel {
	return llm.ThinkingPolicyFor(model).Available
}

// roleThinking phân tích cường độ suy luận có hiệu lực của một vai trò; giá trị không hợp lệ hạ cấp thành rỗng (không ghi đè) và warn.
func roleThinking(cfg bootstrap.Config, role string) agentcore.ThinkingLevel {
	lv, err := ParseThinkingLevel(cfg.ResolveReasoningEffort(role))
	if err != nil {
		slog.Warn("Bỏ qua cấu hình cường độ suy luận không hợp lệ", "module", "agent", "role", role, "err", err)
		return ""
	}
	return lv
}

func resolvedRoleThinking(model agentcore.ChatModel, cfg bootstrap.Config, role string) agentcore.ThinkingLevel {
	resolved, _ := ResolveThinkingForModel(model, roleThinking(cfg, role))
	return resolved
}

// BuildCoordinator lắp ráp Coordinator Agent và các SubAgent của nó.
// Trả về Agent, AskUserTool, WriterRestorePack, tham chiếu ContextEngine của Coordinator,
// và closure ApplyThinking —— lớp Host khi chuyển /model cần gọi trực tiếp SetContextWindow +
// SetReserveTokens để liên động cửa sổ mô hình mới (writer/architect/editor đi theo ContextManagerFactory
// tự động xây lại, không cần ref; chỉ coordinator thường trú mới cần), và qua ApplyThinking liên động
// cường độ suy luận các vai trò. Lớp Host lấy luồng sự kiện qua Agent.Subscribe, không cần callback emit nữa.
func BuildCoordinator(
	cfg bootstrap.Config,
	store *store.Store,
	models *bootstrap.ModelSet,
	bundle assets.Bundle,
	recordUsage UsageRecorder,
	onFlowBoundary FlowBoundaryHook,
) (*agentcore.Agent, *tools.AskUserTool, *ctxpack.WriterRestorePack, *corecontext.ContextEngine, ApplyThinking) {
	// Công cụ dùng chung
	contextTool := tools.NewContextTool(store, bundle.References, cfg.Style)
	// Dịch vụ quy tắc người dùng: chuẩn hóa các nguồn → hợp nhất xác định → ghi snapshot của sách này xuống disk.
	// Công cụ save_user_rules của Coordinator tái sử dụng nó để cập nhật trong khi chạy; chuẩn hóa dùng mô hình Default (nhất quán với phía Host mở sách).
	userRulesSvc := userrules.NewService(store, models.Default, rules.DefaultOptions())
	readChapter := tools.NewReadChapterTool(store)
	askUser := tools.NewAskUserTool()

	architectTools := []agentcore.Tool{
		contextTool,
		tools.NewSaveFoundationTool(store),
	}
	writerTools := []agentcore.Tool{
		contextTool,
		readChapter,
		tools.NewPlanChapterTool(store),
		tools.NewDraftChapterTool(store),
		tools.NewEditChapterTool(store),
		tools.NewCheckConsistencyTool(store),
		tools.NewCommitChapterTool(store),
	}
	editorTools := []agentcore.Tool{
		contextTool,
		readChapter,
		tools.NewSaveReviewTool(store),
		tools.NewSaveArcSummaryTool(store),
		tools.NewSaveVolumeSummaryTool(store),
	}

	// Provider failover chỉ ghi log, không thông báo cho host
	reportFailover := func(ev bootstrap.FailoverEvent) {
		slog.Warn("chuyển đổi provider",
			"module", "agent",
			"role", ev.Role,
			"reason", ev.Reason,
			"from", fmt.Sprintf("%s/%s", ev.FromProvider, ev.FromModel),
			"to", fmt.Sprintf("%s/%s", ev.ToProvider, ev.ToModel),
			"err", ev.Err,
		)
	}

	architectModel := models.ForRoleWithFailover("architect", reportFailover)
	writerModel := models.ForRoleWithFailover("writer", reportFailover)
	editorModel := models.ForRoleWithFailover("editor", reportFailover)
	coordinatorModel := models.ForRoleWithFailover("coordinator", reportFailover)

	// ContextManager của Coordinator được tạo một lần khi xây Agent, phân giải theo mô hình khởi động.
	// Khi chạy /model chuyển sang mô hình cửa sổ nhỏ hơn, khuyến nghị người dùng cấu hình context_window rõ ràng làm dự phòng.
	_, coordinatorModelName, _ := models.CurrentSelection("coordinator")
	coordinatorContextWindow, coordinatorSource := cfg.ResolveContextWindow(coordinatorModelName)
	// ContextManager của Writer được tái tạo mỗi lần gọi factory, cửa sổ theo dõi động khi swap mô hình (xem factory bên dưới).
	_, writerModelName, _ := models.CurrentSelection("writer")
	writerContextWindow, writerSource := cfg.ResolveContextWindow(writerModelName)
	bootstrap.LogContextWindowChoice("coordinator", coordinatorModelName, coordinatorContextWindow, coordinatorSource)
	bootstrap.LogContextWindowChoice("writer", writerModelName, writerContextWindow, writerSource)

	// modelLookup khi ghi session sẽ đính kèm _meta:{provider,model} vào mỗi tin nhắn assistant,
	// cho phép replay không còn phụ thuộc "ModelSet hiện tại" để suy ngược cost lịch sử, chuyển mô hình trong khi chạy vẫn tính chính xác.
	modelLookup := func(agentName string) (string, string) {
		role := agentToRole(agentName)
		provider, name, _ := models.CurrentSelection(role)
		return provider, name
	}
	baseOnMsg := store.Sessions.SubAgentLogger(modelLookup)
	onMsg := func(agentName, task string, msg agentcore.AgentMessage) {
		baseOnMsg(agentName, task, msg)
		if recordUsage != nil {
			recordUsage(agentName, msg)
		}
	}
	baseCoordinatorLog := store.Sessions.CoordinatorLogger(modelLookup)
	coordinatorOnMessage := func(msg agentcore.AgentMessage) {
		baseCoordinatorLog(msg)
		if recordUsage != nil {
			recordUsage("coordinator", msg)
		}
	}

	architectStopGuardFactory := func(_, _ string) agentcore.StopGuard {
		return reminder.NewArchitectStopGuard(store)
	}
	architectThinking, _ := ResolveThinkingForModel(architectModel, roleThinking(cfg, "architect"))
	architectShort := subagent.Config{
		Name:               "architect_short",
		Description:        "Nhà hoạch định truyện ngắn: tạo thiết lập súc tích và đại cương phẳng cho câu chuyện đơn tập, đơn xung đột, mật độ cao",
		Model:              architectModel,
		SystemPrompt:       bundle.Prompts.ArchitectShort,
		Tools:              architectTools,
		MaxTurns:           15,
		MaxRetries:         subagentMaxRetries,
		ThinkingLevel:      architectThinking,
		ToolsAreIdempotent: true,
		OnMessage:          onMsg,
		StopAfterToolResult: func(toolName string, result json.RawMessage) bool {
			r := decodeSaveFoundationResult(toolName, result)
			return r.Type == "outline" && r.FoundationReady
		},
		StopGuardFactory: architectStopGuardFactory,
	}
	architectLong := subagent.Config{
		Name:                "architect_long",
		Description:         "Nhà hoạch định truyện dài: tạo thiết lập phân lớp và đại cương tập-arc cho câu chuyện serial có thể mở rộng liên tục",
		Model:               architectModel,
		SystemPrompt:        bundle.Prompts.ArchitectLong,
		Tools:               architectTools,
		MaxTurns:            20,
		MaxRetries:          subagentMaxRetries,
		ThinkingLevel:       architectThinking,
		ToolsAreIdempotent:  true,
		OnMessage:           onMsg,
		StopAfterToolResult: architectLongShouldStopAfterToolResult,
		StopGuardFactory:    architectStopGuardFactory,
	}

	writerPrompt := bundle.Prompts.Writer
	if style, ok := bundle.Styles[cfg.Style]; ok {
		writerPrompt += "\n\n" + style
	}

	restore := &ctxpack.WriterRestorePack{}
	restore.Refresh(store)

	writer := subagent.Config{
		Name:               "writer",
		Description:        "Người sáng tác: tự chủ hoàn thành lên ý tưởng, viết, tự đánh giá và nộp một chương",
		Model:              writerModel,
		SystemPrompt:       writerPrompt,
		Tools:              writerTools,
		MaxTurns:           30,
		MaxRetries:         subagentMaxRetries,
		ThinkingLevel:      resolvedRoleThinking(writerModel, cfg, "writer"),
		ToolsAreIdempotent: true,
		StopAfterTools:     []string{"commit_chapter"},
		OnMessage:          onMsg,
		StopGuardFactory: func(_, _ string) agentcore.StopGuard {
			return reminder.NewWriterStopGuard(store)
		},
		ContextManagerFactory: func(model agentcore.ChatModel) agentcore.ContextManager {
			// Mỗi lần gọi subagent(writer) sẽ được tái tạo, đọc tên mô hình mới nhất từ runModel hiện tại.
			// Sau khi /model chuyển writer, chương tiếp theo tự động dùng cửa sổ mới.
			window, _ := cfg.ResolveContextWindow(bootstrap.ModelName(model))
			return newContextManager(contextManagerConfig{
				Model:            model,
				ContextWindow:    window,
				ReserveTokens:    bootstrap.CompactReserveTokens(window),
				KeepRecentTokens: 20000,
				Agent:            "writer",
				ToolMicrocompact: &corecontext.ToolResultMicrocompactConfig{
					IdleThreshold: 5 * time.Minute,
				},
				ExtraStrategies: []corecontext.Strategy{
					ctxpack.NewStoreSummaryCompact(ctxpack.StoreSummaryCompactConfig{
						Store:            store,
						KeepRecentTokens: 20000,
					}),
				},
				Summary: &corecontext.FullSummaryConfig{
					PostSummaryHooks:    []corecontext.PostSummaryHook{restore.Hook()},
					SystemPrompt:        ctxpack.WriterSummarySystemPrompt,
					SummaryPrompt:       ctxpack.WriterSummaryPrompt,
					UpdateSummaryPrompt: ctxpack.WriterUpdateSummaryPrompt,
					TurnPrefixPrompt:    ctxpack.WriterTurnPrefixPrompt,
				},
			})
		},
	}

	editor := subagent.Config{
		Name:               "editor",
		Description:        "Người thẩm định: đọc văn gốc, phát hiện vấn đề từ hai góc độ cấu trúc và thẩm mỹ",
		Model:              editorModel,
		SystemPrompt:       bundle.Prompts.Editor,
		Tools:              editorTools,
		MaxTurns:           20,
		MaxRetries:         subagentMaxRetries,
		ThinkingLevel:      resolvedRoleThinking(editorModel, cfg, "editor"),
		ToolsAreIdempotent: true,
		OnMessage:          onMsg,
		// Chỉ dừng khi trúng sản phẩm đầu ra cuối cùng loại tóm tắt; save_review không còn dừng cứng ——
		// StopAfterTool thoát sẽ bỏ qua StopGuard (agentcore loop.go), nếu save_review dừng cứng,
		// editor "được dispatch để tạo arc summary nhưng đánh giá trước" sẽ bị cắt tại save_review, không đến được save_arc_summary.
		// Việc kết thúc tác vụ đánh giá/tóm tắt chuyển sang NewEditorStopGuard nhận biết tác vụ để kiểm soát.
		StopAfterToolResult: func(toolName string, _ json.RawMessage) bool {
			return toolName == "save_arc_summary" || toolName == "save_volume_summary"
		},
		StopGuardFactory: func(_, task string) agentcore.StopGuard {
			return reminder.NewEditorStopGuard(store, task)
		},
	}

	subagentTool := subagent.New(architectShort, architectLong, writer, editor)

	coordinatorEngine := newContextManager(contextManagerConfig{
		Model:            coordinatorModel,
		ContextWindow:    coordinatorContextWindow,
		ReserveTokens:    bootstrap.CompactReserveTokens(coordinatorContextWindow),
		KeepRecentTokens: 30000,
		Agent:            "coordinator",
		CommitOnProject:  true,
	})

	agent := agentcore.NewAgent(
		agentcore.WithModel(coordinatorModel),
		agentcore.WithSystemPrompt(bundle.Prompts.Coordinator),
		agentcore.WithTools(subagentTool, contextTool, tools.NewSaveUserRulesTool(userRulesSvc), tools.NewReopenBookTool(store)),
		agentcore.WithMaxTurns(100_000),
		agentcore.WithOnMessage(coordinatorOnMessage),
		agentcore.WithToolsAreIdempotent(true),
		// subagent là kênh chính của luồng; lỗi thực sự nên trả về rõ ràng cho Host, không phải vô hiệu hóa công cụ vĩnh viễn trong một lần run.
		agentcore.WithMaxToolErrors(0),
		agentcore.WithMaxRetries(subagentMaxRetries),
		agentcore.WithContextManager(coordinatorEngine),
		agentcore.WithStopGuard(reminder.NewStopGuard(store, nil)),
		agentcore.WithMiddlewares(flowBoundaryMiddleware(onFlowBoundary)),
		// Chặn cứng việc dispatch subagent khi phase=complete, ngăn Writer vào vòng lặp vô hạn.
		agentcore.WithToolGate(combineToolGates(
			completePhaseGate(store),
			writerExpandedChapterGate(store),
		)),
	)
	// Cường độ suy luận của Coordinator: áp dụng kết quả phân giải vô điều kiện. Khi chưa cấu hình thì rỗng
	// (không gửi thinking, dùng mặc định provider), nhất quán với các subagent (Config.ThinkingLevel mặc định rỗng) ——
	// tránh ghi đè mặc định agentcore ThinkingLow mà buộc tất cả provider phải gửi low (bao gồm GLM/Ollama sẽ bị bắt buộc bật thinking).
	coordinatorThinking, _ := ResolveThinkingForModel(models.ForRole("coordinator"), roleThinking(cfg, "coordinator"))
	agent.SetThinkingLevel(coordinatorThinking)

	// Liên động cường độ suy luận các vai trò trong khi chạy: coordinator qua Agent, subagent qua subagentTool override.
	applyThinking := func(role string, level agentcore.ThinkingLevel) {
		switch role {
		case "coordinator":
			level, _ = ResolveThinkingForModel(models.ForRole("coordinator"), level)
			agent.SetThinkingLevel(level)
		case "architect":
			level, _ = ResolveThinkingForModel(models.ForRole("architect"), level)
			subagentTool.SetThinkingLevel("architect_short", level)
			subagentTool.SetThinkingLevel("architect_long", level)
		case "writer", "editor":
			level, _ = ResolveThinkingForModel(models.ForRole(role), level)
			subagentTool.SetThinkingLevel(role, level)
		}
	}

	return agent, askUser, restore, coordinatorEngine, applyThinking
}

func flowBoundaryMiddleware(onBoundary FlowBoundaryHook) agentcore.ToolMiddleware {
	return func(ctx context.Context, call agentcore.ToolCall, next agentcore.ToolExecuteFunc) (json.RawMessage, error) {
		out, err := next(ctx, call.Args)
		if err == nil && onBoundary != nil && isFlowBoundaryTool(call.Name) {
			onBoundary(call.Name)
		}
		return out, err
	}
}

func isFlowBoundaryTool(name string) bool {
	return name == "subagent" || name == "reopen_book"
}

// completePhaseGate trả về một ToolGate: từ chối tất cả dispatch subagent khi phase=complete.
// Ngăn Coordinator LLM vẫn gọi Writer/Architect sau khi sách hoàn thành dẫn đến vòng lặp vô hạn.
func completePhaseGate(st *store.Store) agentcore.ToolGate {
	return func(_ context.Context, req agentcore.GateRequest) (*agentcore.GateDecision, error) {
		if req.Call.Name != "subagent" {
			return nil, nil
		}
		// fail-open: khi Load lỗi hoặc progress rỗng thì cho phép qua hết, không để lỗi đọc tức thời khóa cứng dispatch bình thường.
		// Chi phí duy nhất là deadlock có thể tái hiện khi đọc thất bại đúng lúc ở giai đoạn complete (xác suất rất thấp, chấp nhận được).
		progress, _ := st.Progress.Load()
		if progress != nil && progress.Phase == domain.PhaseComplete {
			return &agentcore.GateDecision{
				Allowed: false,
				Reason:  "Toàn bộ tác phẩm đã hoàn thành (phase=complete), không thể dispatch subagent trực tiếp. Nếu người dùng muốn làm lại các chương đã viết, hãy gọi reopen_book(chapters=[...]) để mở lại sách vào trạng thái làm lại trước (sau đó sẽ tự động dispatch writer để viết lại); nếu người dùng muốn thêm cốt truyện, hãy thông báo cần tạo dự án mới.",
			}, nil
		}
		return nil, nil
	}
}

func combineToolGates(gates ...agentcore.ToolGate) agentcore.ToolGate {
	return func(ctx context.Context, req agentcore.GateRequest) (*agentcore.GateDecision, error) {
		for _, gate := range gates {
			if gate == nil {
				continue
			}
			decision, err := gate(ctx, req)
			if err != nil {
				return nil, err
			}
			if decision != nil && !decision.Allowed {
				return decision, nil
			}
		}
		return nil, nil
	}
}

func writerExpandedChapterGate(st *store.Store) agentcore.ToolGate {
	return func(_ context.Context, req agentcore.GateRequest) (*agentcore.GateDecision, error) {
		if req.Call.Name != "subagent" {
			return nil, nil
		}
		var args struct {
			Agent string `json:"agent"`
			Task  string `json:"task"`
		}
		if err := json.Unmarshal(req.Call.Args, &args); err != nil || args.Agent != "writer" {
			return nil, nil
		}
		chapter := chapterFromTask(args.Task)
		if chapter <= 0 {
			chapter = writerFallbackChapter(st)
		}
		if chapter <= 0 {
			return nil, nil
		}
		if err := tools.EnsureChapterExpanded(st, chapter); err != nil {
			return &agentcore.GateDecision{
				Allowed: false,
				Reason:  err.Error() + ". Hãy dispatch architect_long thay thế, gọi save_foundation(type=expand_arc) để mở rộng arc tiếp theo, hoặc type=append_volume để thêm và mở rộng tập tiếp theo rồi mới dispatch writer.",
			}, nil
		}
		return nil, nil
	}
}

func writerFallbackChapter(st *store.Store) int {
	if st == nil {
		return 0
	}
	progress, err := st.Progress.Load()
	if err != nil || progress == nil {
		return 0
	}
	if len(progress.PendingRewrites) > 0 {
		return progress.PendingRewrites[0]
	}
	return progress.NextChapter()
}

var chapterTaskRe = regexp.MustCompile(`第\s*(\d+)\s*章`)

func chapterFromTask(task string) int {
	m := chapterTaskRe.FindStringSubmatch(task)
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

type saveFoundationResult struct {
	Type            string `json:"type"`
	FoundationReady bool   `json:"foundation_ready"`
}

func decodeSaveFoundationResult(toolName string, result json.RawMessage) saveFoundationResult {
	if toolName != "save_foundation" {
		return saveFoundationResult{}
	}
	var r saveFoundationResult
	_ = json.Unmarshal(result, &r)
	return r
}

func architectLongShouldStopAfterToolResult(toolName string, result json.RawMessage) bool {
	r := decodeSaveFoundationResult(toolName, result)
	switch r.Type {
	case "expand_arc", "complete_book":
		return true
	default:
		return false
	}
}

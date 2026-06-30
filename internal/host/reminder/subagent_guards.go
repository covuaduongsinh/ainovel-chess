package reminder

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/store"
)

// subagentMaxConsecutiveBlocks chặn liên tiếp N lần thì nâng lên thành kết thúc, tránh mô hình yếu lặp chết.
const subagentMaxConsecutiveBlocks = 3

// hardStopReasons là các lý do từ chối trả lời phía provider không thể khôi phục bằng thông điệp thúc giục. Tiêm
// "phải commit" với chúng vô hiệu, ngược lại mỗi lần lại tốn token của một lời gọi LLM trọn vẹn,
// và cuối cùng sau khi escalate khiến coordinator phân lại cả SubAgent, cộng dồn lãng phí gấp nhiều lần
// (thực đo ch02 đụng safety thì một lần viết chương sinh 3 lần phân lại 17 lời gọi LLM, tỉ lệ trúng
// tụt từ 50% xuống 2.8%).
//
// Lưu ý StopReasonError / StopReasonAborted không cần liệt kê: agentcore khi nhận hai stop reason này ở
// loop.go thì kết thúc run thẳng, không hề gọi StopGuard.
// Ở đây chỉ liệt kê các ngữ nghĩa từ chối trả lời của provider thực sự đi đến StopGuard.
var hardStopReasons = map[agentcore.StopReason]struct{}{
	"safety":         {},
	"content_filter": {},
}

// newCheckpointDeltaGuard dựng một StopGuard:
// nếu sau baseline mà chưa xuất hiện checkpoint của step chỉ định thì từ chối end_turn.
// baseline do bên gọi nắm bắt tại thời điểm factory, bảo đảm ngữ nghĩa per-run đúng.
func newCheckpointDeltaGuard(st *store.Store, agentName string, requiredSteps []string, blockMsg string) agentcore.StopGuard {
	var baseline int64
	if cp := st.Checkpoints.LatestGlobal(); cp != nil {
		baseline = cp.Seq
	}
	need := make(map[string]struct{}, len(requiredSteps))
	for _, s := range requiredSteps {
		need[s] = struct{}{}
	}
	var consecutive atomic.Int32
	return func(_ context.Context, info agentcore.StopInfo) agentcore.StopDecision {
		// Lỗi không khôi phục được: nâng cấp thẳng, không phí một lần thúc giục.
		if _, hard := hardStopReasons[info.Message.StopReason]; hard {
			slog.Error("subagent stop_guard phát hiện dừng máy không khôi phục được, nâng cấp ngay",
				"module", "host.reminder", "agent", agentName,
				"turn", info.TurnIndex, "stop_reason", info.Message.StopReason)
			return agentcore.StopDecision{Allow: false, Escalate: true}
		}
		// Quét ngược: checkpoint mới ở đuôi, gặp <= baseline là break.
		all := st.Checkpoints.All()
		for i := len(all) - 1; i >= 0; i-- {
			cp := all[i]
			if cp.Seq <= baseline {
				break
			}
			if _, ok := need[cp.Step]; ok {
				consecutive.Store(0)
				return agentcore.StopDecision{Allow: true}
			}
		}
		n := consecutive.Add(1)
		if n > subagentMaxConsecutiveBlocks {
			slog.Error("subagent stop_guard chặn liên tiếp quá hạn, nâng lên thành kết thúc",
				"module", "host.reminder", "agent", agentName, "turn", info.TurnIndex, "consecutive", n)
			return agentcore.StopDecision{Allow: false, Escalate: true}
		}
		slog.Warn("subagent stop_guard chặn end_turn",
			"module", "host.reminder", "agent", agentName, "turn", info.TurnIndex, "consecutive", n)
		return agentcore.StopDecision{Allow: false, InjectMessage: blockMsg}
	}
}

// NewWriterStopGuard yêu cầu writer lượt này ít nhất sinh một lần commit_chapter thành công.
func NewWriterStopGuard(st *store.Store) agentcore.StopGuard {
	return newCheckpointDeltaGuard(st, "writer",
		[]string{"commit"},
		"Bạn phải gọi commit_chapter nộp chương này rồi mới được kết thúc. draft_chapter chỉ lưu bản nháp, không tính là hoàn thành.",
	)
}

// NewArchitectStopGuard yêu cầu architect lượt này ít nhất lưu xuống một lần save_foundation.
func NewArchitectStopGuard(st *store.Store) agentcore.StopGuard {
	return newCheckpointDeltaGuard(st, "architect",
		[]string{
			"premise", "outline", "layered_outline", "characters", "world_rules",
			"expand_arc", "append_volume", "update_compass", "complete_book",
		},
		"Bạn phải gọi save_foundation lưu sản phẩm xuống rồi mới được kết thúc. Chỉ xuất văn bản Markdown/JSON đồng nghĩa với mất dữ liệu.",
	)
}

// NewEditorStopGuard yêu cầu editor lượt này lưu xuống sản phẩm khớp với "nhiệm vụ" rồi mới được kết thúc.
//
// Nhận biết nhiệm vụ: khi được phân đi sinh tóm tắt, chỉ save_review (phúc tra) không tính là hoàn thành — phải
// sinh tóm tắt tương ứng. Nếu không, editor "được phân sinh tóm tắt cung mà lại phúc tra trước" sẽ thỏa mãn tiêu
// chí lỏng cũ rồi kết thúc sớm, tóm tắt cung không bao giờ lưu xuống (phối với dispatcher khử trùng câm tịt từng
// gây vòng lặp chết cung bộ khung giữa tập, xem chi tiết outline-exhaustion-livelock). Thoát StopAfterTool sẽ vòng
// qua StopGuard (loop.go), nên build.go đồng bộ chuyển save_review ra khỏi hard stop, để sau khi phúc tra còn đi
// tiếp đến công cụ tóm tắt, rồi do guard này chốt chặn thu lại.
func NewEditorStopGuard(st *store.Store, task string) agentcore.StopGuard {
	switch {
	case strings.Contains(task, "save_volume_summary") || strings.Contains(task, "tóm tắt tập"):
		return newCheckpointDeltaGuard(st, "editor", []string{"volume_summary"},
			"Nhiệm vụ lần này là sinh tóm tắt tập: bạn phải gọi save_volume_summary lưu xuống rồi mới được kết thúc, save_review phúc tra không tính là hoàn thành.")
	case strings.Contains(task, "save_arc_summary") || strings.Contains(task, "tóm tắt cung"):
		return newCheckpointDeltaGuard(st, "editor", []string{"arc_summary"},
			"Nhiệm vụ lần này là sinh tóm tắt cung: bạn phải gọi save_arc_summary lưu xuống rồi mới được kết thúc, save_review phúc tra không tính là hoàn thành.")
	default:
		// Thẩm định hoặc nhiệm vụ tạm thời: bất kỳ thẩm định/tóm tắt nào lưu xuống là được (giữ hành vi lỏng sẵn có).
		return newCheckpointDeltaGuard(st, "editor",
			[]string{"review", "arc_summary", "volume_summary"},
			"Bạn phải gọi một trong save_review / save_arc_summary / save_volume_summary lưu kết quả xuống rồi mới được kết thúc.")
	}
}

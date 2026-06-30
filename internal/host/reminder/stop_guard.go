package reminder

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host/flow"
	"github.com/voocel/ainovel-cli/internal/store"
)

// StopGuard là phòng tuyến cuối cùng "vật lý không thể dừng máy".
// Khi LLM cố end_turn:
//   - Progress.Phase = Complete → cho qua
//   - ngược lại tiêm user message, để agent tiếp tục turn kế
//   - chặn liên tiếp quá maxConsecutive lần → Escalate kết thúc run (cho thấy prompt/reminder hỏng nặng)
//
// Guard tự duy trì bộ đếm consecutive block; một khi cho qua hoặc tiêm thành công thì đặt lại về 0.
// Cái thực sự dẫn dắt hành vi Coordinator là Reminder + Prompt, StopGuard chỉ là lưới đỡ.
const maxConsecutiveBlocks = 5

// NewStopGuard dựng StopGuard chuyên cho Coordinator.
// onBlock tùy chọn, khác nil thì mỗi lần chặn gọi một lần, dùng để kiểm toán.
func NewStopGuard(st *store.Store, onBlock func(reason string, consecutive int32)) agentcore.StopGuard {
	var consecutive atomic.Int32
	var lastBlockTurn atomic.Int64 // TurnIndex của lần block trước; -1 nghĩa là chưa block lần nào
	lastBlockTurn.Store(-1)
	return func(_ context.Context, info agentcore.StopInfo) agentcore.StopDecision {
		progress, _ := st.Progress.Load()
		if progress != nil && progress.Phase == domain.PhaseComplete {
			consecutive.Store(0)
			lastBlockTurn.Store(-1)
			return agentcore.StopDecision{Allow: true}
		}
		// Chỉ "bị chặn liên tiếp ở turn liền kề" mới cộng dồn bộ đếm; ngược lại coi là lượt mới (LLM đã làm
		// tool call đạt tiến triển, hoặc người dùng tiêm / resume khiến TurnIndex chảy ngược), đặt lại bộ đếm.
		last := lastBlockTurn.Load()
		if last < 0 || int64(info.TurnIndex) != last+1 {
			consecutive.Store(0)
		}
		lastBlockTurn.Store(int64(info.TurnIndex))
		n := consecutive.Add(1)
		if n > maxConsecutiveBlocks {
			slog.Error("stop_guard chặn liên tiếp quá hạn, nâng lên thành kết thúc",
				"module", "host.reminder", "turn", info.TurnIndex, "consecutive", n)
			if onBlock != nil {
				onBlock("escalated", n)
			}
			return agentcore.StopDecision{Allow: false, Escalate: true}
		}
		inject := blockMessage(st, progress)
		if progress != nil && len(progress.PendingRewrites) > 0 {
			inject = fmt.Sprintf("Cấm kết thúc đối thoại. Hàng đợi chờ viết lại chưa xong: %v, hãy gọi writer xử lý ngay.", progress.PendingRewrites)
		}
		slog.Warn("stop_guard chặn end_turn",
			"module", "host.reminder", "turn", info.TurnIndex, "consecutive", n)
		if onBlock != nil {
			onBlock("blocked", n)
		}
		return agentcore.StopDecision{Allow: false, InjectMessage: inject}
	}
}

func blockMessage(st *store.Store, progress *domain.Progress) string {
	if progress != nil && flow.Route(flow.LoadState(st)) != nil {
		return "Cấm kết thúc đối thoại. Phase chưa Complete; hãy chờ và thực thi lệnh `[Host ra lệnh]` mà Host ra, đừng tự gọi novel_context hay subagent."
	}
	return "Cấm kết thúc đối thoại. Phase chưa Complete, và hiện không có lệnh định tuyến của Host; đây là tình huống Coordinator phán định, hãy theo quy tắc phán định của coordinator.md tiếp tục xử lý, đừng chờ suông lệnh Host."
}

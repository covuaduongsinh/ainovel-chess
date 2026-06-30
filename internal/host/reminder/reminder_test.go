package reminder

import (
	"context"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	return s
}

func TestStopGuard_AllowsStopOnlyWhenComplete(t *testing.T) {
	s := newTestStore(t)
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("init progress: %v", err)
	}

	guard := NewStopGuard(s, nil)

	// Chưa Complete: phải chặn + tiêm
	decision := guard(context.Background(), agentcore.StopInfo{TurnIndex: 1})
	if decision.Allow {
		t.Fatal("stop must be blocked before Phase=Complete")
	}
	if decision.InjectMessage == "" {
		t.Fatal("inject message required when blocking")
	}

	// Chuyển Complete: cho qua
	if err := s.Progress.UpdatePhase(domain.PhaseComplete); err != nil {
		t.Fatalf("update phase: %v", err)
	}
	decision = guard(context.Background(), agentcore.StopInfo{TurnIndex: 2})
	if !decision.Allow {
		t.Fatal("stop must be allowed when Phase=Complete")
	}
}

func TestStopGuard_EscalatesAfterTooManyConsecutiveBlocks(t *testing.T) {
	s := newTestStore(t)
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("init progress: %v", err)
	}

	var blocks []string
	guard := NewStopGuard(s, func(reason string, _ int32) {
		blocks = append(blocks, reason)
	})

	for i := 0; i < maxConsecutiveBlocks; i++ {
		decision := guard(context.Background(), agentcore.StopInfo{TurnIndex: i})
		if decision.Escalate {
			t.Fatalf("escalated too early at iteration %d", i)
		}
	}
	decision := guard(context.Background(), agentcore.StopInfo{TurnIndex: maxConsecutiveBlocks})
	if !decision.Escalate {
		t.Fatalf("expected escalate after %d consecutive blocks", maxConsecutiveBlocks+1)
	}
	if len(blocks) != maxConsecutiveBlocks+1 {
		t.Fatalf("audit callback called %d times, want %d", len(blocks), maxConsecutiveBlocks+1)
	}
	if blocks[len(blocks)-1] != "escalated" {
		t.Fatalf("last audit reason should be 'escalated', got %q", blocks[len(blocks)-1])
	}
}

func TestStopGuard_DefaultBlockMessageWaitsForHost(t *testing.T) {
	s := newTestStore(t)
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("init progress: %v", err)
	}
	if err := s.Progress.UpdatePhase(domain.PhaseWriting); err != nil {
		t.Fatalf("update phase: %v", err)
	}

	decision := NewStopGuard(s, nil)(context.Background(), agentcore.StopInfo{TurnIndex: 1})
	if !strings.Contains(decision.InjectMessage, "[Host ra lệnh]") {
		t.Fatalf("inject message should point to Host instruction, got %q", decision.InjectMessage)
	}
	for _, forbidden := range []string{"phán định", "coordinator.md"} {
		if strings.Contains(decision.InjectMessage, forbidden) {
			t.Fatalf("inject message should not suggest freelance action %q: %q", forbidden, decision.InjectMessage)
		}
	}
}

func TestStopGuard_DefaultBlockMessageAllowsCoordinatorJudgmentWhenNoRoute(t *testing.T) {
	s := newTestStore(t)
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("init progress: %v", err)
	}

	decision := NewStopGuard(s, nil)(context.Background(), agentcore.StopInfo{TurnIndex: 1})
	if strings.Contains(decision.InjectMessage, "[Host ra lệnh]") {
		t.Fatalf("no-route inject should not tell coordinator to wait for Host, got %q", decision.InjectMessage)
	}
	if !strings.Contains(decision.InjectMessage, "phán định") {
		t.Fatalf("no-route inject should mention coordinator judgment, got %q", decision.InjectMessage)
	}
}

// TestSubAgentGuard_HardStopReasonEscalatesImmediately kiểm chứng: khi mô hình trả về
// safety / content_filter — loại từ chối trả lời phía provider không khôi phục được — thì StopGuard
// của subagent phải Escalate ngay thay vì tiêm thông điệp thúc giục.
//
// Bối cảnh lịch sử: thực đo hy3-preview:free khi viết chương 2 từ chối liên tiếp 8 lần stop_reason='safety';
// logic cũ tiêm lặp "phải commit", mô hình tiếp tục safety, gom đủ 3 lần block mới escalate, sau đó
// coordinator lại phân lại writer tổng cộng 3 lần. Mỗi lần phân lại là một SubAgent mới → tiền tố cache khởi động
// nguội toàn bộ. Sau khi sửa, lần safety đầu tiên escalate ngay, coordinator nhìn thông điệp lỗi của LLM thấy
// không khôi phục được, nghiêng về đổi đường thay vì phân lại.
//
// Lưu ý chỉ test safety / content_filter: StopReasonError / StopReasonAborted đi nhánh agentcore loop.go kết thúc
// run thẳng, không hề gọi StopGuard, liệt kê vào ngược lại đưa vào code chết.
func TestSubAgentGuard_HardStopReasonEscalatesImmediately(t *testing.T) {
	cases := []agentcore.StopReason{
		agentcore.StopReason("safety"),
		agentcore.StopReason("content_filter"),
	}
	for _, sr := range cases {
		t.Run(string(sr), func(t *testing.T) {
			s := newTestStore(t)
			guard := NewWriterStopGuard(s)
			info := agentcore.StopInfo{
				TurnIndex: 1,
				Message:   agentcore.Message{StopReason: sr},
			}
			d := guard(context.Background(), info)
			if !d.Escalate {
				t.Fatalf("stop_reason=%q must escalate immediately, got %#v", sr, d)
			}
			if d.InjectMessage != "" {
				t.Fatalf("stop_reason=%q must not inject any message, got %q", sr, d.InjectMessage)
			}
		})
	}
}

// TestSubAgentGuard_NormalStopStillBlocks bảo đảm hành vi chặn với stop_reason bình thường
// không bị ảnh hưởng bởi lối tắt lỗi cứng — khi LLM tự dừng mà chưa commit thì vẫn phải thúc.
func TestSubAgentGuard_NormalStopStillBlocks(t *testing.T) {
	s := newTestStore(t)
	guard := NewWriterStopGuard(s)
	info := agentcore.StopInfo{
		TurnIndex: 1,
		Message:   agentcore.Message{StopReason: agentcore.StopReasonStop},
	}
	d := guard(context.Background(), info)
	if d.Escalate {
		t.Fatal("normal stop must not escalate on first block")
	}
	if d.Allow {
		t.Fatal("normal stop must be blocked when no commit checkpoint exists")
	}
	if d.InjectMessage == "" {
		t.Fatal("normal stop must inject a follow-up message")
	}
}

// TestStopGuard_NonConsecutiveTurnResetsCounter kiểm chứng: khi TurnIndex giữa hai lần block không liền kề
// (giữa chừng LLM làm tool call hoặc người dùng resume) thì bộ đếm consecutive được đặt lại.
func TestStopGuard_NonConsecutiveTurnResetsCounter(t *testing.T) {
	s := newTestStore(t)
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("init progress: %v", err)
	}

	guard := NewStopGuard(s, nil)

	for i := 0; i < maxConsecutiveBlocks; i++ {
		if d := guard(context.Background(), agentcore.StopInfo{TurnIndex: i}); d.Escalate {
			t.Fatalf("escalated too early at iteration %d", i)
		}
	}

	d := guard(context.Background(), agentcore.StopInfo{TurnIndex: maxConsecutiveBlocks + 10})
	if d.Escalate {
		t.Fatal("non-consecutive block must NOT escalate; counter should have been reset")
	}
	if d.Allow {
		t.Fatal("stop must still be blocked when Phase != Complete")
	}

	d = guard(context.Background(), agentcore.StopInfo{TurnIndex: 1})
	if d.Escalate {
		t.Fatal("resume (TurnIndex backflow) must NOT escalate")
	}
}

// TestEditorStopGuard_TaskAware kiểm chứng nhận biết nhiệm vụ: khi được phân sinh tóm tắt cung, chỉ save_review
// (phúc tra) không tính là hoàn thành, phải sinh arc_summary mới cho qua — chặn điểm khởi đầu Defect C của vòng
// lặp chết cung bộ khung giữa tập.
func TestEditorStopGuard_TaskAware(t *testing.T) {
	normalStop := agentcore.StopInfo{TurnIndex: 1, Message: agentcore.Message{StopReason: agentcore.StopReasonStop}}

	// Nhiệm vụ tóm tắt + chỉ lưu review → phải chặn (review không thỏa yêu cầu arc_summary).
	t.Run("summary task blocks on review only", func(t *testing.T) {
		s := newTestStore(t)
		guard := NewEditorStopGuard(s, "Sinh tóm tắt cung 1 của tập 5 (save_arc_summary)")
		if _, err := s.Checkpoints.Append(domain.ArcScope(5, 1), "review", "reviews/v05a01.json", "d1"); err != nil {
			t.Fatalf("append review: %v", err)
		}
		if d := guard(context.Background(), normalStop); d.Allow {
			t.Fatal("summary task must NOT be satisfied by a review checkpoint")
		}
	})

	// Nhiệm vụ tóm tắt + đã lưu arc_summary → cho qua.
	t.Run("summary task allows on arc_summary", func(t *testing.T) {
		s := newTestStore(t)
		guard := NewEditorStopGuard(s, "Sinh tóm tắt cung 1 của tập 5 (save_arc_summary)")
		if _, err := s.Checkpoints.Append(domain.ArcScope(5, 1), "arc_summary", "summaries/arc-v05a01.json", "d1"); err != nil {
			t.Fatalf("append arc_summary: %v", err)
		}
		if d := guard(context.Background(), normalStop); !d.Allow {
			t.Fatal("summary task must be satisfied by an arc_summary checkpoint")
		}
	})

	// Nhiệm vụ thẩm định + đã lưu review → cho qua (hành vi lỏng mặc định không đổi).
	t.Run("review task allows on review", func(t *testing.T) {
		s := newTestStore(t)
		guard := NewEditorStopGuard(s, "Thẩm định cấp cung cho cung 1 của tập 5 (scope=arc)")
		if _, err := s.Checkpoints.Append(domain.ArcScope(5, 1), "review", "reviews/v05a01.json", "d1"); err != nil {
			t.Fatalf("append review: %v", err)
		}
		if d := guard(context.Background(), normalStop); !d.Allow {
			t.Fatal("review task must be satisfied by a review checkpoint")
		}
	})
}

package web

import (
	"context"
	"net/http"
	"strings"

	"github.com/voocel/ainovel-cli/internal/entry/startup"
)

// handleStart đi theo đường khởi động nhất quán với headless quick:
// PrepareQuick → PrepareDossier → PrepareUserRules (tạo snapshot quy tắc sách này một cách xác định) → StartPrepared.
//
// Ba bước sau chạy BẤT ĐỒNG BỘ như một job, không nằm trong request. Lý do: PrepareDossier và
// PrepareUserRules mỗi bước là một cuộc gọi LLM tuần tự (với provider dạng CLI, mỗi cuộc gọi vài chục
// giây), còn StartPrepared thì chạy hẳn lượt đầu của Coordinator. Làm đồng bộ thì trình duyệt phải giữ
// một request HTTP mở suốt nhiều phút, không có phản hồi gì — và khi nó bỏ cuộc, người dùng chỉ nhận
// được "Failed to fetch" trong khi máy chủ vẫn đang chạy tiếp. TUI vốn đã chạy đúng ba bước này ngoài
// luồng giao diện (tea.Cmd); web nay soi gương theo.
//
// Chỉ phần kiểm tra rẻ tiền (decode, prompt rỗng, PrepareQuick) ở lại trong request, để đầu vào sai
// vẫn báo lỗi tức thì như cũ.
func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	var req startRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		writeErr(w, http.StatusBadRequest, errMsg("vui lòng nhập yêu cầu sáng tác"))
		return
	}
	plan, err := startup.PrepareQuick(startup.Request{
		Mode:        startup.ModeQuick,
		UserPrompt:  prompt,
		OutputDir:   s.eng.Dir(),
		Interactive: true,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	// Chặn sớm khi AI đang chạy. StartPrepared vẫn tự canh (nguồn sự thật), nhưng nó nằm SAU hai
	// bước chuẩn hóa: không có cửa này thì mỗi cú bấm lặp đốt hai cuộc gọi LLM rồi mới bị từ chối,
	// và tệ hơn là ghi đè hồ sơ nhân vật + snapshot quy tắc của cuốn sách đang viết dở.
	if s.eng.Snapshot().IsRunning {
		writeErr(w, http.StatusConflict, errMsg("AI đang chạy — hãy dừng lượt hiện tại trước khi bắt đầu lại"))
		return
	}
	id, ctx := s.jobs.start()
	go s.runStart(ctx, id, plan, req)
	writeOK(w, map[string]any{"id": id})
}

// runStart chạy ba bước nặng của khởi động và tường thuật từng bước qua SSE.
// ctx đến từ jobRegistry: nút "Dừng" của hộp thoại tiến trình hủy được cuộc gọi LLM đang chờ.
func (s *Server) runStart(ctx context.Context, id string, plan startup.Plan, req startRequest) {
	defer s.jobs.finish(id)

	step := func(stage, msg string, cur int) {
		s.emitProgress(progressDTO{Job: "start", ID: id, Stage: stage, Current: cur, Total: 3, Message: msg})
	}
	fail := func(stage string, err error) {
		s.emitProgress(progressDTO{Job: "start", ID: id, Stage: stage, Message: "Không khởi động được", Error: err.Error(), Done: true})
	}

	// Hai bước chuẩn hóa GIÁNG CẤP khi cuộc gọi LLM hỏng hoặc bị hủy (giữ văn bản gốc) chứ không trả
	// lỗi — nên phải tự kiểm tra ctx sau mỗi bước, nếu không bấm "Dừng" vẫn khởi động Coordinator.
	stopped := func() bool {
		if ctx.Err() == nil {
			return false
		}
		s.emitProgress(progressDTO{Job: "start", ID: id, Stage: "cancel", Message: "Đã dừng theo yêu cầu", Done: true})
		return true
	}

	step("dossier", "Chuẩn hóa hồ sơ nhân vật có thật…", 1)
	if err := s.eng.PrepareDossier(ctx, strings.TrimSpace(req.Subject), req.SourceText, req.Grounding); err != nil {
		fail("dossier", err)
		return
	}
	if stopped() {
		return
	}
	step("rules", "Chuẩn hóa quy tắc sáng tác…", 2)
	if err := s.eng.PrepareUserRules(ctx, plan.RawPrompt); err != nil {
		fail("rules", err)
		return
	}
	if stopped() {
		return
	}
	step("start", "Khởi động Coordinator…", 3)
	if err := s.eng.StartPrepared(plan.StartPrompt); err != nil {
		fail("start", err)
		return
	}
	s.emitProgress(progressDTO{Job: "start", ID: id, Stage: "done", Current: 3, Total: 3, Message: "Đã bắt đầu sáng tác", Done: true})
}

func (s *Server) handleResume(w http.ResponseWriter, _ *http.Request) {
	label, err := s.eng.Resume()
	if err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeOK(w, map[string]any{"label": label})
}

func (s *Server) handleSteer(w http.ResponseWriter, r *http.Request) {
	var req textRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeErr(w, http.StatusBadRequest, errMsg("nội dung can thiệp trống"))
		return
	}
	s.eng.Steer(req.Text)
	writeOK(w, nil)
}

func (s *Server) handleContinue(w http.ResponseWriter, r *http.Request) {
	var req textRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.eng.Continue(req.Text); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeOK(w, nil)
}

func (s *Server) handleAbort(w http.ResponseWriter, _ *http.Request) {
	aborted := s.eng.Abort()
	writeOK(w, map[string]any{"aborted": aborted})
}

func (s *Server) handleAnswer(w http.ResponseWriter, r *http.Request) {
	var req answerRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.ask.resolve(req.ID, req.Answers, req.Notes); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeOK(w, nil)
}

// errMsg là constructor lỗi nhẹ (tránh dùng fmt.Errorf khắp nơi).
type errMsg string

func (e errMsg) Error() string { return string(e) }

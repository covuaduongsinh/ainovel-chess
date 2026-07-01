package web

import (
	"net/http"
	"strings"

	"github.com/voocel/ainovel-cli/internal/entry/startup"
)

// handleStart đi theo đường khởi động nhất quán với headless quick:
// PrepareQuick → PrepareUserRules (tạo snapshot quy tắc sách này một cách xác định) → StartPrepared.
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
	if err := s.eng.PrepareDossier(strings.TrimSpace(req.Subject), req.SourceText, req.Grounding); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.eng.PrepareUserRules(plan.RawPrompt); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.eng.StartPrepared(plan.StartPrompt); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeOK(w, nil)
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

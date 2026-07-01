package web

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/voocel/ainovel-cli/internal/host/imp"
	"github.com/voocel/ainovel-cli/internal/host/sim"
)

// jobRegistry quản lý các tác vụ nền có thể hủy (import / simulate).
// import/simulate đã loại trừ lẫn nhau ở tầng host, ở đây chỉ cần hỗ trợ "hủy cái đang chạy".
type jobRegistry struct {
	mu   sync.Mutex
	jobs map[string]context.CancelFunc
	seq  int
}

func newJobRegistry() *jobRegistry {
	return &jobRegistry{jobs: make(map[string]context.CancelFunc)}
}

func (jr *jobRegistry) start() (string, context.Context) {
	jr.mu.Lock()
	defer jr.mu.Unlock()
	jr.seq++
	id := "job-" + strconv.Itoa(jr.seq)
	ctx, cancel := context.WithCancel(context.Background())
	jr.jobs[id] = cancel
	return id, ctx
}

func (jr *jobRegistry) finish(id string) {
	jr.mu.Lock()
	if cancel, ok := jr.jobs[id]; ok {
		cancel()
		delete(jr.jobs, id)
	}
	jr.mu.Unlock()
}

func (jr *jobRegistry) cancel(id string) error {
	jr.mu.Lock()
	cancel, ok := jr.jobs[id]
	jr.mu.Unlock()
	if !ok {
		return fmt.Errorf("không tìm thấy tác vụ %q", id)
	}
	cancel()
	return nil
}

// progressDTO là phép chiếu thống nhất của tiến trình import/simulate.
type progressDTO struct {
	Job     string `json:"job"`
	ID      string `json:"id"`
	Stage   string `json:"stage"`
	Current int    `json:"current"`
	Total   int    `json:"total"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
	Done    bool   `json:"done"`
}

func (s *Server) emitProgress(p progressDTO) {
	s.hub.emit(sseMessage{Type: msgProgress, Data: p}, true)
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	var req importRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		writeErr(w, http.StatusBadRequest, errMsg("vui lòng nhập đường dẫn file truyện"))
		return
	}
	id, ctx := s.jobs.start()
	ch, err := s.eng.ImportFrom(ctx, imp.Options{SourcePath: req.Path, ResumeFrom: req.From})
	if err != nil {
		s.jobs.finish(id)
		writeErr(w, http.StatusConflict, err)
		return
	}
	go func() {
		defer s.jobs.finish(id)
		for ev := range ch {
			done := ev.Stage == imp.StageDone || ev.Stage == imp.StageError
			s.emitProgress(progressDTO{
				Job: "import", ID: id, Stage: string(ev.Stage),
				Current: ev.Current, Total: ev.Total, Message: ev.Message,
				Error: errString(ev.Err), Done: done,
			})
		}
		// Nhập thành công → tự động tiếp tục viết (nhất quán với hành vi TUI).
		if _, err := s.eng.Resume(); err != nil {
			slog.Warn("Tự động tiếp tục viết sau import thất bại", "module", "web", "err", err)
		}
	}()
	writeOK(w, map[string]any{"id": id})
}

func (s *Server) handleSimulate(w http.ResponseWriter, _ *http.Request) {
	id, ctx := s.jobs.start()
	ch, err := s.eng.Simulate(ctx)
	if err != nil {
		s.jobs.finish(id)
		writeErr(w, http.StatusConflict, err)
		return
	}
	go s.streamSim(id, "simulate", ch)
	writeOK(w, map[string]any{"id": id})
}

func (s *Server) handleImportSim(w http.ResponseWriter, r *http.Request) {
	var req textRequest // Text = đường dẫn file json hồ sơ
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeErr(w, http.StatusBadRequest, errMsg("vui lòng nhập đường dẫn file hồ sơ phỏng tác"))
		return
	}
	id, ctx := s.jobs.start()
	ch, err := s.eng.ImportSimulationProfile(ctx, req.Text)
	if err != nil {
		s.jobs.finish(id)
		writeErr(w, http.StatusConflict, err)
		return
	}
	go s.streamSim(id, "importsim", ch)
	writeOK(w, map[string]any{"id": id})
}

func (s *Server) streamSim(id, job string, ch <-chan sim.Event) {
	defer s.jobs.finish(id)
	for ev := range ch {
		done := ev.Stage == sim.StageDone || ev.Stage == sim.StageError
		s.emitProgress(progressDTO{
			Job: job, ID: id, Stage: string(ev.Stage),
			Current: ev.Current, Total: ev.Total, Message: ev.Message,
			Error: errString(ev.Err), Done: done,
		})
	}
}

func (s *Server) handleJobCancel(w http.ResponseWriter, r *http.Request) {
	var req textRequest // Text = id tác vụ
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.jobs.cancel(req.Text); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeOK(w, nil)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

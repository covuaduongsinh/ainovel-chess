package web

import (
	"encoding/json"
	"io/fs"
	"net/http"

	"github.com/voocel/ainovel-cli/assets"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/host"
)

// Server giữ engine, SSE hub và các bridge, dịch yêu cầu HTTP thành lời gọi phương thức host.
type Server struct {
	eng     *host.Host
	hub     *hub
	mux     *http.ServeMux
	cfg     bootstrap.Config
	bundle  assets.Bundle
	version string

	ask      *askUserBridge
	cocreate *coCreateBridge
	jobs     *jobRegistry // tác vụ nền có thể hủy như import/simulate
}

func newServer(eng *host.Host, cfg bootstrap.Config, bundle assets.Bundle, version string) *Server {
	s := &Server{
		eng:     eng,
		hub:     newHub(eng),
		mux:     http.NewServeMux(),
		cfg:     cfg,
		bundle:  bundle,
		version: version,
		jobs:    newJobRegistry(),
	}
	s.ask = newAskUserBridge(s.hub)
	s.cocreate = newCoCreateBridge(s, s.hub)
	eng.AskUser().SetHandler(s.ask.handler)
	s.routes()
	return s
}

func (s *Server) routes() {
	// Tài nguyên tĩnh frontend (đường dẫn gốc).
	sub, _ := fs.Sub(staticFS, "static")
	s.mux.Handle("/", http.FileServer(http.FS(sub)))

	// Kênh quan sát
	s.mux.HandleFunc("GET /api/stream", s.handleStream)
	s.mux.HandleFunc("GET /api/snapshot", s.handleSnapshot)
	s.mux.HandleFunc("GET /api/meta", s.handleMeta)

	// Vòng đời sáng tác
	s.mux.HandleFunc("POST /api/start", s.handleStart)
	s.mux.HandleFunc("POST /api/resume", s.handleResume)
	s.mux.HandleFunc("POST /api/steer", s.handleSteer)
	s.mux.HandleFunc("POST /api/continue", s.handleContinue)
	s.mux.HandleFunc("POST /api/abort", s.handleAbort)
	s.mux.HandleFunc("POST /api/answer", s.handleAnswer)
	s.mux.HandleFunc("POST /api/dossier/draft", s.handleDossierDraft)

	// Mô hình / mức độ suy luận
	s.mux.HandleFunc("GET /api/models", s.handleModels)
	s.mux.HandleFunc("POST /api/model", s.handleSwitchModel)
	s.mux.HandleFunc("POST /api/model/auto", s.handleModelAuto)
	s.mux.HandleFunc("GET /api/thinking", s.handleThinking)
	s.mux.HandleFunc("POST /api/thinking", s.handleSetThinking)

	// Lệnh
	s.mux.HandleFunc("POST /api/export", s.handleExport)
	s.mux.HandleFunc("POST /api/diag", s.handleDiag)
	s.mux.HandleFunc("POST /api/import", s.handleImport)
	s.mux.HandleFunc("POST /api/simulate", s.handleSimulate)
	s.mux.HandleFunc("POST /api/importsim", s.handleImportSim)
	s.mux.HandleFunc("POST /api/job/cancel", s.handleJobCancel)

	// Cộng tác sáng tác
	s.mux.HandleFunc("POST /api/cocreate/open", s.handleCoCreateOpen)
	s.mux.HandleFunc("POST /api/cocreate/send", s.handleCoCreateSend)
	s.mux.HandleFunc("POST /api/cocreate/start", s.handleCoCreateStart)
	s.mux.HandleFunc("POST /api/cocreate/cancel", s.handleCoCreateCancel)
}

// ── Tiện ích HTTP ──

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeOK(w http.ResponseWriter, v any) {
	if v == nil {
		v = map[string]any{"ok": true}
	}
	writeJSON(w, http.StatusOK, v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	msg := "lỗi không xác định"
	if err != nil {
		msg = err.Error()
	}
	writeJSON(w, code, map[string]any{"error": msg})
}

func decodeBody(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

// metaResponse là thông tin tĩnh được kéo khi frontend khởi động.
type metaResponse struct {
	Version string `json:"version"`
	Dir     string `json:"dir"`
}

func (s *Server) handleMeta(w http.ResponseWriter, _ *http.Request) {
	writeOK(w, metaResponse{Version: s.version, Dir: s.eng.Dir()})
}

func (s *Server) handleSnapshot(w http.ResponseWriter, _ *http.Request) {
	writeOK(w, s.eng.Snapshot())
}

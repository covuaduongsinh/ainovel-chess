package web

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"runtime/debug"

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

	sm *sessionManager // quản lý vòng đời phiên dự án (để đổi/đóng dự án)
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

// handler trả về http.Handler đối ngoại của server: mux định tuyến được bọc bởi
// middleware recover. Trước đây session.go gắn thẳng s.mux, nên một panic trong bất kỳ
// handler nào sẽ do net/http bắt và đóng phắt kết nối — trình duyệt chỉ thấy "mạng lỗi",
// không có thông điệp, còn log thì không có ngữ cảnh route.
func (s *Server) handler() http.Handler {
	return recoverMiddleware(s.mux)
}

// headerTracker theo dõi việc header đã được gửi chưa, để middleware biết còn kịp
// trả về JSON lỗi hay không (SSE/tải file đã stream thì chỉ ghi log rồi ngắt).
type headerTracker struct {
	http.ResponseWriter
	wrote bool
}

func (t *headerTracker) WriteHeader(code int) {
	t.wrote = true
	t.ResponseWriter.WriteHeader(code)
}

func (t *headerTracker) Write(b []byte) (int, error) {
	t.wrote = true
	return t.ResponseWriter.Write(b)
}

// Flush giữ nguyên khả năng streaming của SSE (handleStream ép kiểu http.Flusher).
func (t *headerTracker) Flush() {
	if f, ok := t.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tw := &headerTracker{ResponseWriter: w}
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			// http.ErrAbortHandler là quy ước "chủ động bỏ request", không phải lỗi thật.
			if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(rec)
			}
			slog.Error("panic trong handler web",
				"module", "web", "method", r.Method, "path", r.URL.Path,
				"panic", rec, "stack", string(debug.Stack()))
			if !tw.wrote {
				writeJSON(tw, http.StatusInternalServerError, map[string]any{
					"error": "lỗi nội bộ máy chủ (đã ghi log)",
				})
			}
		}()
		next.ServeHTTP(tw, r)
	})
}

func (s *Server) routes() {
	// Tài nguyên tĩnh frontend (đường dẫn gốc).
	sub, _ := fs.Sub(staticFS, "static")
	s.mux.Handle("/", http.FileServer(http.FS(sub)))

	// Kênh quan sát
	s.mux.HandleFunc("GET /api/stream", s.handleStream)
	s.mux.HandleFunc("GET /api/snapshot", s.handleSnapshot)
	s.mux.HandleFunc("GET /api/meta", s.handleMeta)

	// Đổi dự án: rời workbench hiện tại về màn chọn dự án
	s.mux.HandleFunc("POST /api/projects/leave", s.handleLeave)

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
	s.mux.HandleFunc("POST /api/adapt", s.handleAdapt)
	s.mux.HandleFunc("POST /api/audiobook", s.handleAudiobook)
	s.mux.HandleFunc("POST /api/comic", s.handleComic)
	s.mux.HandleFunc("GET /api/comic/presets", s.handleComicPresets)
	s.mux.HandleFunc("GET /api/comic/config", s.handleComicConfig)
	s.mux.HandleFunc("POST /api/comic/config", s.handleComicConfigSave)
	s.mux.HandleFunc("POST /api/comic/test-image", s.handleComicTestImage)
	s.mux.HandleFunc("GET /api/comic/estimate", s.handleComicEstimate)
	s.mux.HandleFunc("POST /api/job/cancel", s.handleJobCancel)

	// Sách nói (Vbee TTS)
	s.mux.HandleFunc("GET /api/vbee/config", s.handleVbeeConfig)
	s.mux.HandleFunc("POST /api/vbee/config", s.handleVbeeConfigSave)
	s.mux.HandleFunc("GET /api/vbee/voices", s.handleVbeeVoices)
	s.mux.HandleFunc("POST /api/vbee/preview", s.handleVbeePreview)

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
	// ExportBase là đường dẫn base gợi ý cho hộp thoại xuất: {thư-mục-dự-án}/{tên-thư-mục}
	// (không hậu tố; file trùng tên thư mục chứa). Frontend điền sẵn vào ô đường dẫn.
	ExportBase string `json:"exportBase"`
}

func (s *Server) handleMeta(w http.ResponseWriter, _ *http.Request) {
	dir := s.eng.Dir()
	writeOK(w, metaResponse{
		Version:    s.version,
		Dir:        dir,
		ExportBase: filepath.Join(dir, filepath.Base(dir)),
	})
}

func (s *Server) handleSnapshot(w http.ResponseWriter, _ *http.Request) {
	writeOK(w, s.eng.Snapshot())
}

// handleLeave rời dự án hiện tại về màn chọn dự án. Nếu engine đang chạy thì chặn (409) và yêu cầu
// người dùng tạm dừng trước — không tự Abort để tránh cắt ngang lượt viết đang diễn ra.
func (s *Server) handleLeave(w http.ResponseWriter, _ *http.Request) {
	if s.sm == nil {
		writeErr(w, http.StatusInternalServerError, errMsg("không có trình quản lý phiên"))
		return
	}
	if s.eng.Snapshot().IsRunning {
		writeErr(w, http.StatusConflict, errMsg("AI đang chạy — hãy Tạm dừng trước khi đổi dự án"))
		return
	}
	// Swap sang picker (đóng chính Server này) trước khi trả lời, để trình duyệt reload chắc chắn
	// gặp màn chọn dự án chứ không trúng lúc đang teardown.
	s.sm.showPicker()
	writeOK(w, map[string]any{"ok": true})
}

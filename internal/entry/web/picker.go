package web

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/voocel/ainovel-cli/internal/store"
)

// projectCreateRequest là thân yêu cầu tạo dự án mới (form màn chọn dự án).
type projectCreateRequest struct {
	Name string `json:"name"`
}

// projectOpenRequest là thân yêu cầu mở một dự án hiện có.
type projectOpenRequest struct {
	Dir string `json:"dir"`
}

// newPickerMux dựng mux cho màn chọn dự án (phục vụ khi chưa mở dự án nào), theo đúng tiền lệ
// newSetupMux: trang tĩnh + API liệt kê / tạo / mở. Mở/tạo thành công sẽ tự swap sang workbench
// (do sm.open thực hiện), trình duyệt chỉ cần reload.
func newPickerMux(sm *sessionManager) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, err := staticFS.ReadFile("static/projects.html")
		if err != nil {
			http.Error(w, "trang chọn dự án bị thiếu", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})

	mux.HandleFunc("GET /styles.css", func(w http.ResponseWriter, _ *http.Request) {
		data, _ := staticFS.ReadFile("static/styles.css")
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(data)
	})

	mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, _ *http.Request) {
		list, err := store.List(sm.root)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		if list == nil {
			list = []store.ProjectSummary{}
		}
		writeOK(w, map[string]any{"root": sm.root, "projects": list})
	})

	mux.HandleFunc("POST /api/projects", func(w http.ResponseWriter, r *http.Request) {
		var req projectCreateRequest
		if err := decodeBody(r, &req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			writeErr(w, http.StatusBadRequest, errMsg("vui lòng nhập tên dự án"))
			return
		}
		slug := store.Slugify(req.Name)
		dir := store.UniqueDir(sm.root, slug)
		if err := sm.open(dir); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeOK(w, map[string]any{"dir": dir})
	})

	mux.HandleFunc("POST /api/projects/open", func(w http.ResponseWriter, r *http.Request) {
		var req projectOpenRequest
		if err := decodeBody(r, &req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		dir, ok := resolveUnderRoot(sm.root, req.Dir)
		if !ok {
			writeErr(w, http.StatusBadRequest, errMsg("đường dẫn dự án không hợp lệ"))
			return
		}
		if err := sm.open(dir); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeOK(w, map[string]any{"dir": dir})
	})

	return mux
}

// resolveUnderRoot làm sạch dir và xác nhận nó nằm bên trong root (chống path traversal).
// Trả về đường dẫn đã làm sạch và true nếu hợp lệ.
func resolveUnderRoot(root, dir string) (string, bool) {
	cleaned := filepath.Clean(dir)
	rootClean := filepath.Clean(root)
	rel, err := filepath.Rel(rootClean, cleaned)
	if err != nil {
		return "", false
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return cleaned, true
}

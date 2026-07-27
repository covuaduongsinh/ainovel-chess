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

// projectOpenRequest là thân yêu cầu mở một dự án hiện có. Cũng dùng cho lưu trữ / bỏ lưu trữ,
// vì cả ba thao tác chỉ cần đúng một tham số là thư mục dự án.
type projectOpenRequest struct {
	Dir string `json:"dir"`
}

// projectRenameRequest là thân yêu cầu đổi tên dự án (đổi cả tên sách lẫn tên thư mục).
type projectRenameRequest struct {
	Dir  string `json:"dir"`
	Name string `json:"name"`
}

// projectDeleteRequest là thân yêu cầu xóa vĩnh viễn một dự án. Confirm phải khớp tên sách hoặc
// tên thư mục — hàng rào phía máy chủ để một request lạc không thể xóa mất cả cuốn sách.
type projectDeleteRequest struct {
	Dir     string `json:"dir"`
	Confirm string `json:"confirm"`
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

	mux.HandleFunc("GET /projects.css", func(w http.ResponseWriter, _ *http.Request) {
		data, _ := staticFS.ReadFile("static/projects.css")
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(data)
	})

	mux.HandleFunc("GET /projects.js", func(w http.ResponseWriter, _ *http.Request) {
		data, _ := staticFS.ReadFile("static/projects.js")
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = w.Write(data)
	})

	mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, _ *http.Request) {
		list, archived, err := listProjects(sm.root)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeOK(w, map[string]any{"root": sm.root, "projects": list, "archived": archived})
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

	mux.HandleFunc("POST /api/projects/rename", func(w http.ResponseWriter, r *http.Request) {
		var req projectRenameRequest
		if err := decodeBody(r, &req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		dir, ok := resolveUnderRoot(sm.root, req.Dir)
		if !ok {
			writeErr(w, http.StatusBadRequest, errMsg("đường dẫn dự án không hợp lệ"))
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			writeErr(w, http.StatusBadRequest, errMsg("vui lòng nhập tên dự án"))
			return
		}
		newDir, err := store.Rename(dir, req.Name)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeOK(w, map[string]any{"dir": newDir})
	})

	mux.HandleFunc("POST /api/projects/archive", func(w http.ResponseWriter, r *http.Request) {
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
		newDir, err := store.Archive(sm.root, dir)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeOK(w, map[string]any{"dir": newDir})
	})

	mux.HandleFunc("POST /api/projects/unarchive", func(w http.ResponseWriter, r *http.Request) {
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
		newDir, err := store.Unarchive(sm.root, dir)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeOK(w, map[string]any{"dir": newDir})
	})

	mux.HandleFunc("POST /api/projects/delete", func(w http.ResponseWriter, r *http.Request) {
		var req projectDeleteRequest
		if err := decodeBody(r, &req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		dir, ok := resolveUnderRoot(sm.root, req.Dir)
		if !ok {
			writeErr(w, http.StatusBadRequest, errMsg("đường dẫn dự án không hợp lệ"))
			return
		}
		if err := confirmDeletion(sm.root, dir, req.Confirm); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := store.Delete(dir); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeOK(w, map[string]any{"dir": dir})
	})

	return mux
}

// listProjects đọc danh sách dự án đang dùng và dự án trong kho lưu trữ, chuẩn hóa nil thành
// mảng rỗng để phía JS không phải phòng thủ.
func listProjects(root string) ([]store.ProjectSummary, []store.ProjectSummary, error) {
	list, err := store.List(root)
	if err != nil {
		return nil, nil, err
	}
	archived, err := store.ListArchived(root)
	if err != nil {
		return nil, nil, err
	}
	if list == nil {
		list = []store.ProjectSummary{}
	}
	if archived == nil {
		archived = []store.ProjectSummary{}
	}
	return list, archived, nil
}

// confirmDeletion yêu cầu chuỗi xác nhận khớp tên sách hoặc tên thư mục của đúng dự án sắp xóa.
// Xóa là thao tác không hoàn tác được (mất cả cuốn sách), nên ngoài hộp thoại phía trình duyệt,
// máy chủ tự kiểm tra lại một lần nữa.
func confirmDeletion(root, dir, confirm string) error {
	confirm = strings.TrimSpace(confirm)
	if confirm == "" {
		return errMsg("vui lòng gõ đúng tên dự án để xác nhận xóa")
	}
	list, archived, err := listProjects(root)
	if err != nil {
		return err
	}
	for _, p := range append(list, archived...) {
		if filepath.Clean(p.Dir) != dir {
			continue
		}
		if strings.EqualFold(confirm, strings.TrimSpace(p.Name)) || strings.EqualFold(confirm, p.Slug) {
			return nil
		}
		return errMsg("tên xác nhận không khớp với dự án cần xóa")
	}
	return errMsg("không tìm thấy dự án cần xóa")
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

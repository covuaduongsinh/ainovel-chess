package web

import (
	"net/http"
	"strings"
)

// handleDossierDraft để AI soạn nháp hồ sơ nhân vật thật từ tên chủ thể.
// Trả về bản nháp (JSON) + bản Markdown dễ đọc để người dùng duyệt/sửa trên giao diện.
// Không ghi store: hồ sơ chính thức chỉ được lưu khi bắt đầu sáng tác (handleStart → PrepareDossier).
func (s *Server) handleDossierDraft(w http.ResponseWriter, r *http.Request) {
	var req dossierDraftRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	subject := strings.TrimSpace(req.Subject)
	if subject == "" {
		writeErr(w, http.StatusBadRequest, errMsg("vui lòng nhập tên nhân vật có thật"))
		return
	}
	d, err := s.eng.DraftDossier(subject)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeOK(w, map[string]any{
		"dossier":    d,
		"markdown":   d.RenderMarkdown(),
		"disclaimer": d.Disclaimer,
	})
}

package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/vbee"
)

// vbeeConfigDTO là hình chiếu cấu hình Vbee ra giao diện. AppID/AccessToken luôn được
// CHE khi trả về.
type vbeeConfigDTO struct {
	Configured   bool    `json:"configured"`
	AppID        string  `json:"appId"`
	AccessToken  string  `json:"accessToken"`
	VoiceCode    string  `json:"voiceCode"`
	BaseURL      string  `json:"baseUrl"`
	VoicesURL    string  `json:"voicesUrl"`
	WebhookURL   string  `json:"webhookUrl"`
	Speed        float64 `json:"speed"`
	Bitrate      int     `json:"bitrate"`
	SampleRate   int     `json:"sampleRate"`
	OutputFormat string  `json:"outputFormat"`
}

// maskedMarker là dấu hiệu cho biết giá trị gửi lên chính là giá trị đã che mà giao
// diện nhận được trước đó — nghĩa là người dùng KHÔNG đổi trường này.
const maskedMarker = "****"

// clearMarker cho phép xóa hẳn một thông tin xác thực mà không phải sửa tay config.json.
const clearMarker = "-"

func (s *Server) handleVbeeConfig(w http.ResponseWriter, r *http.Request) {
	writeOK(w, toVbeeDTO(s.eng.VbeeConfig()))
}

func toVbeeDTO(v bootstrap.VbeeConfig) vbeeConfigDTO {
	return vbeeConfigDTO{
		Configured:   v.Configured(),
		AppID:        bootstrap.MaskSecret(v.AppID),
		AccessToken:  bootstrap.MaskSecret(v.AccessToken),
		VoiceCode:    v.VoiceCode,
		BaseURL:      v.BaseURL,
		VoicesURL:    v.VoicesURL,
		WebhookURL:   v.WebhookURL,
		Speed:        v.Speed,
		Bitrate:      v.Bitrate,
		SampleRate:   v.SampleRate,
		OutputFormat: v.OutputFormat,
	}
}

func (s *Server) handleVbeeConfigSave(w http.ResponseWriter, r *http.Request) {
	var req vbeeConfigDTO
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cur := s.eng.VbeeConfig()

	next := bootstrap.VbeeConfig{
		AppID:        mergeSecret(cur.AppID, req.AppID),
		AccessToken:  mergeSecret(cur.AccessToken, req.AccessToken),
		VoiceCode:    strings.TrimSpace(req.VoiceCode),
		BaseURL:      strings.TrimSpace(req.BaseURL),
		VoicesURL:    strings.TrimSpace(req.VoicesURL),
		WebhookURL:   strings.TrimSpace(req.WebhookURL),
		Speed:        req.Speed,
		Bitrate:      req.Bitrate,
		SampleRate:   req.SampleRate,
		OutputFormat: strings.TrimSpace(req.OutputFormat),
	}
	if err := s.eng.SetVbeeConfig(next); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeOK(w, toVbeeDTO(s.eng.VbeeConfig()))
}

// mergeSecret quyết định giá trị bí mật mới từ những gì giao diện gửi lên.
//
// Máy chủ KHÔNG giữ trạng thái phiên, nên quy ước là: giá trị chứa "****" chính là giá
// trị đã che mà trình duyệt vừa hiển thị → giữ nguyên bí mật đang lưu. Gõ giá trị mới
// thì thay. Gõ đúng ký tự "-" thì xóa hẳn.
func mergeSecret(current, incoming string) string {
	incoming = strings.TrimSpace(incoming)
	switch {
	case incoming == "":
		return current
	case incoming == clearMarker:
		return ""
	case strings.Contains(incoming, maskedMarker):
		return current
	}
	return incoming
}

func (s *Server) handleVbeeVoices(w http.ResponseWriter, r *http.Request) {
	q := vbee.VoiceQuery{
		Ownership:    strings.TrimSpace(r.URL.Query().Get("ownership")),
		LanguageCode: strings.TrimSpace(r.URL.Query().Get("lang")),
		Gender:       strings.TrimSpace(r.URL.Query().Get("gender")),
	}
	// Vbee coi voiceOwnership là tham số BẮT BUỘC: thiếu nó thì trả 400 chứ không
	// phải trả về mọi nhóm. Điền mặc định để lời gọi không tham số vẫn dùng được.
	if q.Ownership == "" {
		q.Ownership = "VBEE"
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil {
		q.Limit = n
	}
	voices, err := s.eng.VbeeVoices(r.Context(), q)
	if err != nil {
		// Endpoint danh sách giọng nằm ở host khác endpoint TTS nên có thể hỏng riêng.
		// Giao diện sẽ thoái hóa thành ô nhập mã giọng tự do — không chặn việc tạo sách nói.
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeOK(w, map[string]any{"voices": voices})
}

// vbeePreviewRequest là body cho POST /api/vbee/preview.
type vbeePreviewRequest struct {
	Voice string `json:"voice"`
	Text  string `json:"text"`
}

// handleVbeePreview tổng hợp đồng bộ một câu ngắn và trả thẳng byte audio.
// Dùng để kiểm tra thông tin xác thực trước khi chạy cả cuốn sách: chế độ sync không
// cần webhookUrl và chỉ tốn vài chục ký tự tín dụng.
func (s *Server) handleVbeePreview(w http.ResponseWriter, r *http.Request) {
	var req vbeePreviewRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	data, err := s.eng.VbeePreview(r.Context(), strings.TrimSpace(req.Voice), strings.TrimSpace(req.Text))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

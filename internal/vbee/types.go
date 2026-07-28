// Package vbee là client HTTP tối thiểu cho API chuyển văn bản thành giọng nói của
// Vbee (https://api-docs.vbee.vn). Gói này CHỈ lo giao thức: không biết gì về store,
// chương truyện hay tiến trình — mọi chính sách chờ/thử lại nằm ở internal/host/tts.
//
// Hai chế độ:
//   - async (Batch): trả requestId, phải poll GET /v1/tts/requests/{id}; tối đa 100.000 ký tự.
//   - sync (Realtime): trả thẳng byte audio; tối đa 300 ký tự, dùng để nghe thử.
package vbee

const (
	// DefaultBaseURL là endpoint TTS.
	DefaultBaseURL = "https://api.vbee.vn"
	// DefaultVoicesURL là endpoint danh sách giọng — CHÚ Ý: Vbee đặt API này ở host
	// KHÁC với endpoint TTS.
	DefaultVoicesURL = "https://vbee.vn"
	// DefaultWebhookURL là URL giữ chỗ. Tài liệu Vbee ghi webhookUrl là bắt buộc với
	// mode async, nhưng ứng dụng chạy localhost nên không có URL công khai — ta gửi
	// URL giữ chỗ rồi chủ động poll trạng thái.
	DefaultWebhookURL = "https://example.com/vbee-callback"

	// ModeAsync trả requestId, phải poll.
	ModeAsync = "async"
	// ModeSync trả thẳng byte audio.
	ModeSync = "sync"

	StatusProcessing = "PROCESSING"
	StatusCompleted  = "COMPLETED"
	StatusFailed     = "FAILED"

	// MaxAsyncChars là giới hạn ký tự cho một yêu cầu async.
	MaxAsyncChars = 100000
	// MaxSyncChars là giới hạn ký tự cho một yêu cầu sync.
	MaxSyncChars = 300
)

// SpeechRequest là thân yêu cầu POST /v1/tts.
//
// LƯU Ý: thân yêu cầu dùng camelCase, còn API danh sách giọng lại trả snake_case —
// đây là bất nhất của phía Vbee, không phải nhầm lẫn khi khai báo struct.
type SpeechRequest struct {
	Text              string       `json:"text"`
	Mode              string       `json:"mode"`
	VoiceCode         string       `json:"voiceCode"`
	WebhookURL        string       `json:"webhookUrl,omitempty"`
	OutputFormat      string       `json:"outputFormat,omitempty"`      // mp3 (mặc định) | wav
	Bitrate           int          `json:"bitrate,omitempty"`           // 8|16|32|64|128
	Speed             float64      `json:"speed,omitempty"`             // 0.25–1.9
	SampleRate        int          `json:"sampleRate,omitempty"`        // 8000..48000
	EmphasisIntensity int          `json:"emphasisIntensity,omitempty"` // 0–100, bước 10
	ClientPause       *ClientPause `json:"clientPause,omitempty"`
}

// ClientPause điều chỉnh độ dài các khoảng nghỉ khi đọc (đơn vị: giây).
type ClientPause struct {
	MajorBreak     float64 `json:"majorBreak,omitempty"`     // dấu chấm phẩy; mặc định 0.3
	MediumBreak    float64 `json:"mediumBreak,omitempty"`    // dấu phẩy; mặc định 0.25
	ParagraphBreak float64 `json:"paragraphBreak,omitempty"` // xuống dòng; mặc định 0.6
	SentenceBreak  float64 `json:"sentenceBreak,omitempty"`  // dấu chấm; mặc định 0.45
}

// RequestStatus là phản hồi GET /v1/tts/requests/{requestId}.
//
// AudioLink HẾT HẠN SAU 3 PHÚT kể từ lúc trạng thái chuyển COMPLETED — phải tải ngay.
// Tệp âm thanh vẫn nằm trên máy chủ Vbee 3 ngày, nên có thể xin link mới bằng cách
// gọi lại chính endpoint này.
type RequestStatus struct {
	RequestID string `json:"requestId"`
	Status    string `json:"status"`
	AudioLink string `json:"audioLink"`
}

// Voice là một giọng đọc khả dụng.
type Voice struct {
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	LanguageCode string  `json:"language_code"`
	Gender       string  `json:"gender"`
	Demo         string  `json:"demo"`          // mp3 nghe thử MIỄN PHÍ (không trừ tín dụng)
	CreditFactor float64 `json:"credit_factor"` // hệ số trừ tín dụng, >1 = giọng đắt hơn
}

// VoiceQuery là bộ lọc cho GET /api/public/v1/voices.
type VoiceQuery struct {
	Ownership    string // VBEE | COMMUNITY | PERSONAL
	LanguageCode string // vd "vi-VN"
	Gender       string // male | female
	Limit        int    // 1–100, mặc định 20
	Cursor       string
}

// VoicePage là một trang kết quả đã bóc khỏi phong bì {result:{...},status:1}.
type VoicePage struct {
	Voices      []Voice
	HasNextPage bool
	NextCursor  string
}

// submitResponse là phản hồi POST /v1/tts ở mode async.
type submitResponse struct {
	RequestID string `json:"requestId"`
	Status    string `json:"status"`
}

// voicesEnvelope là phong bì của API danh sách giọng.
type voicesEnvelope struct {
	Result struct {
		Pagination struct {
			HasNextPage bool   `json:"has_next_page"`
			NextCursor  string `json:"next_cursor"`
		} `json:"pagination"`
		Voices []Voice `json:"voices"`
	} `json:"result"`
	Status int `json:"status"`
}

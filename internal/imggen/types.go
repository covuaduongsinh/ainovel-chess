// Package imggen là client HTTP thuần cho API sinh ảnh của Google Gemini.
//
// Giống internal/vbee: gói vendor ĐỘC LẬP, chỉ dùng stdlib, không phụ thuộc gì trong repo,
// test bằng httptest.Server. Bên tiêu thụ (internal/host/comic) tự bọc nó lại — giữ đúng
// hướng phụ thuộc mà vbee/tts đã lập.
//
// # Vì sao phải viết mới thay vì dùng litellm
//
// litellm v1.8.3 (đã có trong repo) chỉ nhận ảnh VÀO: Response.Blocks chỉ có TextBlock,
// ToolUseBlock, ReasoningBlock — không có block ảnh, nên byte ảnh trả về không có chỗ chứa.
// Provider Gemini của nó cũng không bao giờ đặt responseModalities.
//
// # Hai phương ngữ dây
//
// Google đang có hai bề mặt API song song cho việc sinh ảnh và tài liệu đã gắn nhãn đường
// generateContent là "Legacy" mà chưa công bố ngày khai tử. Vì vậy phần DỰNG/ĐỌC gói tin
// được tách sau interface dialect, đổi được bằng cấu hình mà không đụng phần chính sách
// (thử lại, backoff, hạn mức).
package imggen

import (
	"net/http"
	"time"
)

const (
	// DefaultBaseURL là endpoint công khai của Gemini API.
	DefaultBaseURL = "https://generativelanguage.googleapis.com"

	// DefaultModel cân bằng giá và chất lượng cho tranh truyện.
	DefaultModel = "gemini-2.5-flash-image"

	// MaxRefImages là trần ảnh tham chiếu ta tự đặt. Tài liệu Google cho tới 14 ảnh nhưng
	// ngân sách riêng cho "ảnh nhân vật" chỉ khoảng 4 — vượt quá thì mô hình bắt đầu TRỘN
	// đặc điểm các nhân vật vào nhau, đúng thứ ta đang cố tránh.
	MaxRefImages = 4

	// MaxRefBytes là trần cho MỘT ảnh tham chiếu (giữ tổng gói tin dưới hạn ~20 MB).
	MaxRefBytes = 4 << 20

	// defaultTimeout cho mỗi lần sinh ảnh. Model dòng 3.x có bước "thinking" nên một ảnh
	// có thể mất hơn một phút.
	defaultTimeout = 3 * time.Minute

	maxErrorBody = 8 << 10
)

// KnownModels liệt kê các model sinh ảnh để giao diện gợi ý.
// Giá tham khảo (bậc Standard) ghi kèm để người dùng thấy ngay đánh đổi.
var KnownModels = [][2]string{
	{"gemini-2.5-flash-image", "Nano Banana — $0,039/ảnh"},
	{"gemini-3.1-flash-lite-image", "Nano Banana 2 Lite — $0,034/ảnh (1K)"},
	{"gemini-3.1-flash-image", "Nano Banana 2 — $0,067 (1K) / $0,101 (2K)"},
	{"gemini-3-pro-image", "Nano Banana Pro — $0,134/ảnh"},
}

// Options cấu hình client.
type Options struct {
	APIKey  string
	BaseURL string // rỗng = DefaultBaseURL
	Model   string // rỗng = DefaultModel
	Dialect string // "generatecontent" (mặc định) | "interactions"

	// KeyInQuery gửi khoá qua query param ?key= thay vì header x-goog-api-key.
	// Mặc định dùng header: khoá không lọt vào URL nên không dính log proxy và không lộ
	// trong thông báo lỗi.
	KeyInQuery bool

	// Client là khe tiêm cho test; rỗng thì tự dựng với defaultTimeout.
	Client *http.Client
}

// RefImage là một ảnh tham chiếu gửi kèm — cơ chế giữ nhất quán nhân vật.
type RefImage struct {
	MimeType string // image/png | image/jpeg | image/webp
	Data     []byte // byte THÔ; client tự mã hoá base64
	Label    string // nhãn đi kèm, vd "Reference: Tí Tốt"
}

// Request là một yêu cầu sinh ảnh.
type Request struct {
	Prompt      string // tiếng Anh, đã tiêm token phong cách + mô tả chuẩn nhân vật
	Negative    string // tiếng Anh; Gemini KHÔNG có trường riêng nên được nối vào cuối Prompt
	Refs        []RefImage
	AspectRatio string   // "4:3", "3:2", "16:9"… rỗng = không gửi
	ImageSize   string   // "1K" | "2K" | "4K"; rỗng = không gửi
	Model       string   // ghi đè Options.Model cho riêng lần gọi này
	Thinking    string   // "minimal" (mặc định) | "high"
}

// Result là ảnh trả về.
type Result struct {
	Image        []byte // byte ảnh đã giải base64
	MimeType     string
	Text         string // phần chữ mô hình trả kèm (nếu có) — hữu ích khi gỡ lỗi
	FinishReason string
	Model        string
	TotalTokens  int
}

// --- cấu trúc gói tin của phương ngữ generateContent ---

type genRequest struct {
	Contents         []genContent `json:"contents"`
	GenerationConfig *genConfig   `json:"generationConfig,omitempty"`
}

type genContent struct {
	Role  string    `json:"role,omitempty"`
	Parts []genPart `json:"parts"`
}

type genPart struct {
	Text       string   `json:"text,omitempty"`
	InlineData *genBlob `json:"inlineData,omitempty"`
}

// genBlob dùng base64 CHUẨN, KHÔNG có tiền tố "data:image/png;base64,".
type genBlob struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type genConfig struct {
	ResponseModalities []string        `json:"responseModalities,omitempty"`
	ImageConfig        *genImageConfig `json:"imageConfig,omitempty"`
	ThinkingConfig     *genThinking    `json:"thinkingConfig,omitempty"`
}

type genImageConfig struct {
	AspectRatio string `json:"aspectRatio,omitempty"`
	ImageSize   string `json:"imageSize,omitempty"`
}

type genThinking struct {
	ThinkingLevel string `json:"thinkingLevel,omitempty"`
}

type genResponse struct {
	Candidates []struct {
		Content struct {
			Role  string    `json:"role"`
			Parts []genPart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	UsageMetadata struct {
		TotalTokenCount int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

// errorEnvelope là phong bì lỗi chuẩn google.rpc.Status.
type errorEnvelope struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
		Details []struct {
			Type       string `json:"@type"`
			RetryDelay string `json:"retryDelay"`
		} `json:"details"`
	} `json:"error"`
}

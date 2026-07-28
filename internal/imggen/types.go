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
	"strings"
	"time"
)

const (
	// DefaultBaseURL là endpoint công khai của Gemini API.
	DefaultBaseURL = "https://generativelanguage.googleapis.com"

	// DefaultModel là model rẻ nhất cho bản nháp/đọc màn hình.
	//
	// Chọn bản "lite" thay vì gemini-2.5-flash-image ($0,039) là cố ý: cả hai đều chỉ cho ra
	// ~1 megapixel, nhưng bản lite rẻ hơn 14% VÀ trung thực về việc chỉ làm được 1K (bản 2.5
	// nhận tham số imageSize="2K" rồi lặng lẽ trả về 1K, người dùng vẫn trả tiền đủ).
	// Muốn IN 300 DPI thì phải đổi sang ModelFlashImage — xem ModelCaps.
	DefaultModel = "gemini-3.1-flash-lite-image"

	ModelFlashLite = "gemini-3.1-flash-lite-image"
	ModelFlash     = "gemini-3.1-flash-image"
	ModelPro       = "gemini-3-pro-image"
	ModelLegacy25  = "gemini-2.5-flash-image"

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
	{ModelFlashLite, "Nano Banana 2 Lite — $0,034/ảnh · CHỈ 1K (rẻ nhất, hợp bản nháp)"},
	{ModelLegacy25, "Nano Banana — $0,039/ảnh · thực tế chỉ ~1K (bản cũ)"},
	{ModelFlash, "Nano Banana 2 — $0,067 (1K) / $0,101 (2K) · IN ĐƯỢC"},
	{ModelPro, "Nano Banana Pro — $0,134/ảnh · 1K–4K, đẹp nhất"},
}

// ModelCaps mô tả độ phân giải một model THẬT SỰ hỗ trợ, kèm giá tham khảo mỗi ảnh.
//
// Bảng này tồn tại vì API nhận tham số `imageSize` rồi có thể LẶNG LẼ bỏ qua: yêu cầu 2K,
// nhận về 1K, và vẫn bị tính tiền đủ (lỗi đã biết: googleapis/js-genai #1461). Không có bảng
// này thì khoản lãng phí đó vô hình.
type ModelCaps struct {
	Sizes      []string           // theo thứ tự tăng dần: "512", "1K", "2K", "4K"
	USDPerSize map[string]float64 // giá tham khảo bậc Standard
	Legacy     bool
	Note       string // ghi chú tiếng Việt hiện lên UI
}

var modelCaps = map[string]ModelCaps{
	ModelFlashLite: {
		Sizes:      []string{"1K"},
		USDPerSize: map[string]float64{"1K": 0.0336},
		Note:       "chỉ làm được 1K — hợp bản nháp và bản đọc màn hình",
	},
	ModelLegacy25: {
		Sizes:      []string{"1K"},
		USDPerSize: map[string]float64{"1K": 0.039},
		Legacy:     true,
		Note:       "bản cũ; nhận tham số 2K nhưng thực tế vẫn trả ~1K",
	},
	ModelFlash: {
		Sizes:      []string{"512", "1K", "2K", "4K"},
		USDPerSize: map[string]float64{"512": 0.045, "1K": 0.067, "2K": 0.101, "4K": 0.151},
		Note:       "in 300 DPI được (đặt 2K)",
	},
	ModelPro: {
		Sizes:      []string{"1K", "2K", "4K"},
		USDPerSize: map[string]float64{"1K": 0.134, "2K": 0.134, "4K": 0.24},
		Note:       "chất lượng cao nhất, đắt nhất",
	},
}

// CapsFor tra năng lực một model. ok=false với model lạ — khi đó KHÔNG suy đoán gì cả,
// cứ gửi tham số đi và để máy chủ quyết định.
func CapsFor(model string) (ModelCaps, bool) {
	c, ok := modelCaps[strings.TrimSpace(model)]
	return c, ok
}

// SupportsSize cho biết model có thật sự làm được độ phân giải này không.
// Model lạ luôn trả true (không chặn thứ mình không biết).
func SupportsSize(model, size string) bool {
	size = normalizeSize(size)
	if size == "" {
		return true
	}
	caps, ok := CapsFor(model)
	if !ok {
		return true
	}
	for _, s := range caps.Sizes {
		if s == size {
			return true
		}
	}
	return false
}

// PriceFor trả giá tham khảo mỗi ảnh; ok=false nếu không tra được.
func PriceFor(model, size string) (float64, bool) {
	caps, ok := CapsFor(model)
	if !ok {
		return 0, false
	}
	size = normalizeSize(size)
	if size == "" && len(caps.Sizes) > 0 {
		size = caps.Sizes[0]
	}
	p, ok := caps.USDPerSize[size]
	return p, ok
}

// normalizeSize chuẩn hoá cách viết ("1k" → "1K"), rỗng giữ rỗng.
func normalizeSize(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch s {
	case "", "512", "1K", "2K", "4K":
		return s
	}
	return s
}

// megapixelFor quy đổi nhãn độ phân giải sang số megapixel tối thiểu mong đợi.
// Dùng để đối chiếu với ảnh THẬT trả về.
func megapixelFor(size string) float64 {
	switch normalizeSize(size) {
	case "512":
		return 0.20
	case "1K":
		return 0.80
	case "2K":
		return 3.20
	case "4K":
		return 12.0
	}
	return 0
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

	// Width/Height là kích thước THẬT đo được từ byte ảnh, không phải kích thước đã yêu cầu.
	Width, Height int

	// Warning là cảnh báo tiếng Việt cho người dùng khi kết quả không đúng thứ đã đặt hàng
	// (ví dụ xin 2K mà nhận 1K). Rỗng = không có gì bất thường.
	//
	// Đây là lý do trường này tồn tại: API vẫn trả HTTP 200 và vẫn tính tiền đủ khi nó lờ đi
	// tham số imageSize, nên nếu không đối chiếu thì khoản lãng phí đó hoàn toàn vô hình.
	Warning string
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

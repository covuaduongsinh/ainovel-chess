package comic

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/voocel/ainovel-cli/internal/imggen"
)

// geminiSource bọc client Gemini lại thành ImageSource.
//
// Đây là toàn bộ phần "cắm điện" của giai đoạn 2: bộ dàn trang, bộ tính bố cục và bộ đóng
// gói không biết gì về nó và không phải sửa một dòng nào.
//
// Giữ adapter ở ĐÂY chứ không cho internal/imggen tự thoả ImageSource là cố ý, theo đúng
// lệ vbee/tts: gói client nhà cung cấp không biết gì về gói tiêu thụ.
type geminiSource struct {
	client *imggen.Client

	// maxRateLimitStrikes: 429 vừa có nghĩa "quá nhanh, chờ chút" (thử lại được) vừa có
	// nghĩa "cạn hạn mức ngày / chưa bật thanh toán" (vô vọng). Tầng transport không phân
	// biệt được, nên đếm số 429 LIÊN TIẾP ở đây rồi nâng thành lỗi chí tử — nếu không thì
	// một cuốn sách sẽ ngồi thử lại hàng nghìn lần vô ích.
	maxRateLimitStrikes int

	mu      sync.Mutex
	strikes int
	fatal   error
}

// NewGeminiSource dựng ImageSource từ client đã cấu hình.
func NewGeminiSource(c *imggen.Client) ImageSource {
	return &geminiSource{client: c, maxRateLimitStrikes: 5}
}

// ErrFatalImageSource báo hiệu không nên gọi sinh ảnh tiếp trong lần chạy này.
var ErrFatalImageSource = errors.New("nguồn sinh ảnh hỏng, dừng sinh ảnh cho lần chạy này")

func (g *geminiSource) Panel(ctx context.Context, req PanelRequest) (*PanelImage, error) {
	g.mu.Lock()
	if g.fatal != nil {
		err := g.fatal
		g.mu.Unlock()
		return nil, err
	}
	g.mu.Unlock()

	refs := make([]imggen.RefImage, 0, len(req.Refs))
	for _, r := range req.Refs {
		refs = append(refs, imggen.RefImage{MimeType: r.MimeType, Data: r.Data, Label: r.Label})
	}

	res, err := g.client.Generate(ctx, imggen.Request{
		Prompt:      req.Prompt,
		Negative:    req.Negative,
		Refs:        refs,
		AspectRatio: req.Aspect,
		ImageSize:   req.Size,
	})
	if err != nil {
		g.note(err)
		return nil, err
	}
	g.reset()
	return &PanelImage{
		Data: res.Image, MimeType: res.MimeType,
		Width: res.Width, Height: res.Height, Warning: res.Warning,
	}, nil
}

// note cập nhật trạng thái sau một lỗi: khoá sai thì chí tử ngay, 429 thì đếm.
func (g *geminiSource) note(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Sai khoá / bị từ chối: thử tiếp là vô nghĩa và chỉ làm chậm.
	if errors.Is(err, imggen.ErrUnauthorized) {
		g.fatal = fmt.Errorf("%w: %v", ErrFatalImageSource, err)
		return
	}
	if imggen.IsRetryable(err) {
		g.strikes++
		if g.strikes >= g.maxRateLimitStrikes {
			g.fatal = fmt.Errorf("%w: %d lần liên tiếp bị giới hạn/lỗi máy chủ — nhiều khả năng đã cạn hạn mức hoặc chưa bật thanh toán (%v)",
				ErrFatalImageSource, g.strikes, err)
		}
		return
	}
	// Lỗi mềm (bị chặn nội dung, không có ảnh…): không tính là hỏng nguồn.
	g.strikes = 0
}

func (g *geminiSource) reset() {
	g.mu.Lock()
	g.strikes = 0
	g.mu.Unlock()
}

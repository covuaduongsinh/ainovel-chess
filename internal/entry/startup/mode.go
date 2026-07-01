package startup

import "fmt"

// startup layer chứa phối hợp khởi động -- trước khi vào Engine.
// Quy ước phân tầng:
// 1. entry/tui, entry/headless là điểm vào (host entry);
// 2. startup phụ trách các chiến lược khởi động nhanh/đồng sáng tác/tiếp tục;
// 3. orchestrator.Engine chỉ phụ trách thực thi phiên chính thức, không phụ trách chuẩn bị trước chế độ.

// Mode biểu thị loại chiến lược khởi động trước khi vào Engine.
type Mode string

const (
	// ModeQuick dùng thẳng đầu vào người dùng làm điểm khởi đầu sáng tác.
	ModeQuick Mode = "quick"
	// ModeCoCreate làm nhiều vòng làm rõ trước, rồi tạo bản thảo sáng tác để vào Engine.
	ModeCoCreate Mode = "cocreate"
	// ModeContinueFromNovel lắp ráp ngữ cảnh dựa trên nội dung tiểu thuyết đã có rồi tiếp tục viết.
	ModeContinueFromNovel Mode = "continue_from_novel"
)

// Request mô tả đầu vào gốc tầng entry gửi lên tầng chiến lược khởi động.
// Host entry thu thập đầu vào người dùng trước, rồi startup sắp xếp thành kế hoạch có thể vào Engine.
type Request struct {
	Mode        Mode
	UserPrompt  string
	NovelPath   string
	OutputDir   string
	Interactive bool
}

// Plan mô tả kết quả tầng chiến lược khởi động tạo ra.
// Host entry không nên tự ghép prompt khởi động chính thức, mà nên tiêu thụ Plan rồi mới điều khiển Engine.
type Plan struct {
	Mode        Mode
	DisplayName string
	StartPrompt string // prompt khởi động Coordinator đã đóng gói (sản phẩm của BuildStartPrompt)
	RawPrompt   string // yêu cầu sáng tác gốc của người dùng (chưa đóng gói); dùng để chuẩn hóa quy tắc người dùng, resume mode là rỗng
	ResumeOnly  bool
}

// ErrNotImplemented đánh dấu chiến lược giữ chỗ chưa triển khai.
var ErrNotImplemented = fmt.Errorf("startup mode not implemented")

// PrepareContinueFromNovel là điểm giữ chỗ thống nhất cho tiếp tục viết dựa trên tiểu thuyết đã có.
// TUI/headless trong tương lai đều nên sắp xếp đầu vào thành Request trước, rồi từ đây tạo ra Plan có thể vào Engine.
func PrepareContinueFromNovel(req Request) (Plan, error) {
	return Plan{}, fmt.Errorf("%w: %s", ErrNotImplemented, ModeContinueFromNovel)
}

package tui

import "github.com/charmbracelet/lipgloss"

// Bảng màu chủ đề -- sắc thái ấm, phong cách sách cổ
// AdaptiveColor: Light = giá trị nền sáng, Dark = giá trị nền tối
//
// Nguyên tắc thiết kế: bản Light giữ nguyên (đã tinh chỉnh tốt trên nền sáng); bản Dark
// đồng loạt tăng ~25% lightness và nhích saturate so với Light, đảm bảo đủ độ tương phản
// trên nền tối (colorDim trước đây #6b6355 gần như vô hình trên nền đen #1c1c1c, đường
// phân cách và văn bản phụ biến mất hoàn toàn).
//
// colorAccent2 nền tối đổi từ #7a9e7e sang xanh ngọc #5fb8a3, tách biệt với "xanh lành mạnh"
// của colorSuccess -- hai màu trước đây giống nhau hoàn toàn, khiến màu biểu tượng architect
// agent và cảm giác vui mừng "tỉ lệ khớp cao" bị nhầm lẫn.
// bodyTextColor là chiến lược foreground cho "văn bản trung tính":
//   - Terminal tối → NoColor, kế thừa foreground mặc định của terminal, tránh ép #e8e0d0 trắng
//     ngà vào chủ đề nền ấm/lạnh do người dùng tự cấu hình (thực tế màu mặc định nền tối đọc tốt hơn).
//   - Terminal sáng → dùng bản Light của colorText (nâu đậm #3d3529), giữ phong cách ấm;
//     màu đen mặc định trên nền sáng quá cứng, nâu đậm đã điều chỉnh nhìn mềm hơn trên nền sáng.
//
// AdaptiveColor bắt buộc cả hai đầu đều có giá trị màu, không có tùy chọn "không màu",
// nên đây phán một lần khi khởi động, sau đó toàn bộ giá trị tổng quan/nội dung chương/
// mô tả lệnh v.v. đều tham chiếu bodyTextColor thống nhất.
var bodyTextColor lipgloss.TerminalColor = func() lipgloss.TerminalColor {
	if lipgloss.HasDarkBackground() {
		return lipgloss.NoColor{}
	}
	return lipgloss.Color("#3d3529")
}()

var (
	colorText    = lipgloss.AdaptiveColor{Light: "#3d3529", Dark: "#e8e0d0"}
	colorDim     = lipgloss.AdaptiveColor{Light: "#8a7e6b", Dark: "#8a8175"}
	colorMuted   = lipgloss.AdaptiveColor{Light: "#7a7060", Dark: "#b8b09c"}
	colorAccent  = lipgloss.AdaptiveColor{Light: "#b8860b", Dark: "#e5b449"}
	colorAccent2 = lipgloss.AdaptiveColor{Light: "#3d7a42", Dark: "#5fb8a3"}
	colorRunning = lipgloss.AdaptiveColor{Light: "#6f8641", Dark: "#b5d075"}
	colorSuccess = lipgloss.AdaptiveColor{Light: "#3d7a42", Dark: "#7ec488"}
	colorError   = lipgloss.AdaptiveColor{Light: "#b5433a", Dark: "#e07060"}
	colorReview  = lipgloss.AdaptiveColor{Light: "#b07530", Dark: "#e09b5a"}
	colorContext = lipgloss.AdaptiveColor{Light: "#6b5a9e", Dark: "#a890d8"}
	colorTool    = lipgloss.AdaptiveColor{Light: "#3a7a8a", Dark: "#7ec5d8"}
)

// Bản đồ màu theo trạng thái
var statusColors = map[string]lipgloss.AdaptiveColor{
	"READY":    colorDim,
	"PAUSING":  colorAccent,
	"PAUSED":   colorAccent,
	"RUNNING":  colorRunning,
	"REVIEW":   colorReview,
	"REWRITE":  colorReview,
	"COMPLETE": colorSuccess,
	"ERROR":    colorError,
}

// Hiển thị trạng thái: biểu tượng + nhãn. Phù hợp với chủ đề ấm tổng thể, tránh khối màu đặc gây nhức mắt.
// icon của RUNNING để trống, được spinner frame điền động, để cảm giác chuyển động hòa vào chỉ thị trạng thái.
var statusDisplay = map[string]struct {
	icon  string
	label string
}{
	"READY":    {"○", "Sẵn sàng"},
	"RUNNING":  {"", "Đang chạy"},
	"REVIEW":   {"◆", "Xét duyệt"},
	"REWRITE":  {"◆", "Làm lại"},
	"COMPLETE": {"●", "Hoàn thành"},
	"PAUSED":   {"⏸", "Tạm dừng"},
	"PAUSING":  {"⏸", "Đang tạm dừng"},
	"ERROR":    {"✕", "Lỗi"},
}

// Bản đồ màu theo danh mục sự kiện
var categoryColors = map[string]lipgloss.AdaptiveColor{
	"DISPATCH": colorAccent,
	"DONE":     colorSuccess,
	"TOOL":     colorTool,
	"SYSTEM":   colorAccent,
	"USER":     colorAccent2,
	"REVIEW":   colorReview,
	"CHECK":    colorSuccess,
	"ERROR":    colorError,
	"AGENT":    colorMuted,
	"CONTEXT":  colorContext,
	"COMPACT":  colorContext,
}

// Style cơ bản
var (
	baseBorder = lipgloss.RoundedBorder()

	topBarStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Padding(0, 1)

	statusIconStyle = lipgloss.NewStyle().
			Bold(true)

	statusLabelStyle = lipgloss.NewStyle().
				Foreground(colorText)

	panelTitleStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	fieldLabelStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Width(10)

	// fieldValueStyle / cardContentStyle dùng bodyTextColor -- giá trị vùng tổng quan (trạng thái chạy,
	// số chương đã hoàn thành, số từ v.v.), mục dàn ý, danh sách nhân vật, tóm tắt chương v.v. là "văn bản
	// trung tính": trên nền tối theo màu mặc định terminal (tránh ép trắng ngà vào chủ đề người dùng),
	// trên nền sáng dùng nâu đậm để giữ phong cách ấm.
	// Các phần tử mang ngữ nghĩa mạnh (tiêu đề, giá trị nổi bật, trạng thái, lỗi, màu tỉ lệ khớp v.v.)
	// vẫn dùng colorAccent / colorError và các màu chủ đề khác.
	fieldValueStyle = lipgloss.NewStyle().Foreground(bodyTextColor)

	highlightValueStyle = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true)

	contextUsageMetaStyle = lipgloss.NewStyle().
				Foreground(colorDim)

	cardTitleStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true)

	cardContentStyle = lipgloss.NewStyle().Foreground(bodyTextColor)
)

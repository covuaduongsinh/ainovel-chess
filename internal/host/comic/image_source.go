package comic

import "context"

// ImageSource là KHỚP NỐI giữa giai đoạn 1 và giai đoạn 2.
//
// Giai đoạn 1 không có bản cài nào: Deps.Img == nil, mọi khung dùng ảnh giữ chỗ và các
// product refsheet/panelart tự bỏ qua (kèm Event thông báo, KHÔNG phải lỗi).
// Giai đoạn 2 thêm internal/imggen (client HTTP thuần cho Gemini) và một bản bọc mỏng
// thoả giao diện này — không phần nào khác của gói phải sửa.
//
// Đặt giao diện ở ĐÂY chứ không ở internal/imggen là cố ý, theo đúng lệ sẵn có của repo:
// internal/vbee là client nhà cung cấp thuần, không biết gì về internal/host/tts;
// chính host/tts mới là nơi bọc nó lại. Giữ nguyên hướng phụ thuộc đó.
type ImageSource interface {
	// Panel sinh MỘT ảnh. Trả lỗi bọc ErrPanelSkipped nếu khung này nên bỏ qua mềm
	// (bị bộ lọc nội dung chặn chẳng hạn) thay vì làm hỏng cả lần chạy.
	Panel(ctx context.Context, req PanelRequest) (*PanelImage, error)
}

// PanelRequest là yêu cầu sinh một ảnh khung.
type PanelRequest struct {
	Prompt   string // tiếng Anh, đã tiêm style token + canonical prompt nhân vật
	Negative string // tiếng Anh

	// Refs là ẢNH THAM CHIẾU (model sheet nhân vật) — cơ chế giữ nhất quán nhân vật
	// qua hàng nghìn khung. Đây là lý do chọn Gemini thay vì bộ sinh ảnh chỉ nhận chữ.
	Refs []RefImage

	Aspect string // "4:3", "3:2", "16:9"...
	Size   string // "1K" | "2K"
}

// RefImage là một ảnh tham chiếu gửi kèm yêu cầu.
type RefImage struct {
	MimeType string // image/png | image/jpeg
	Data     []byte // byte THÔ (bản cài tự mã hoá base64 nếu giao thức cần)
	Label    string // nhãn đi kèm, vd "Reference: Tí Tốt"
}

// PanelImage là ảnh trả về.
type PanelImage struct {
	Data     []byte // byte ảnh đã giải mã (PNG hoặc JPEG)
	MimeType string
}

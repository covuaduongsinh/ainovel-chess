// Package comicdraw dựng một TRANG TRUYỆN TRANH thành ảnh raster (PNG) và ảnh vector (SVG)
// từ mô tả bố cục + kịch bản chữ.
//
// Đây là gói LÁ thuần đồ hoạ: không biết gì về store, LLM, sự kiện hay cấu hình. Nhờ vậy
// test của nó chạy được mà không cần dựng dự án giả, và host/comic chỉ việc dịch mô hình
// nghiệp vụ sang các kiểu ở đây.
//
// # Ba hệ toạ độ
//
//  1. Vật lý — Millimeter, chỉ xuất hiện trong PageSpec.
//  2. Chuẩn hoá — Rect{X,Y,W,H} trong khoảng 0..1 TƯƠNG ĐỐI VỚI KHUNG THÀNH PHẨM (trim).
//     Mọi thứ tầng trên sinh ra đều nằm ở hệ này, nhờ đó một kịch bản dựng được cả A4 lẫn
//     B5 mà không phải sinh lại.
//  3. Thiết bị — pixel float64 ở 300 DPI, gốc trên-trái, ĐÃ TÍNH CẢ TRÀN LỀ.
//
// # Vì sao không cần thư viện 2D
//
// x/image/vector chỉ TÔ, không có stroker. Ba mẹo thay thế cả một thư viện 2D:
//   - Nét viền: sinh hình theo tham số có đối số inflate rồi tô hai lần (màu nét ở bản
//     phình ra, màu ruột ở bản gốc). Khử răng cưa hoàn hảo ở cả hai mép.
//   - Hợp hình: phát nhiều contour đóng vào CÙNG một Path, non-zero winding tự hợp nhất.
//   - Cắt ảnh theo hình: Path.Mask() cho ra image.Alpha, dùng với draw.DrawMask.
package comicdraw

import (
	"image"
	"image/color"
)

// Millimeter là đơn vị vật lý, chỉ dùng ở PageSpec.
type Millimeter float64

// Point là một điểm chuẩn hoá 0..1 trong khung thành phẩm.
type Point struct{ X, Y float64 }

// Rect là một hình chữ nhật chuẩn hoá 0..1 trong khung thành phẩm.
type Rect struct{ X, Y, W, H float64 }

// PageSpec mô tả khổ trang vật lý.
type PageSpec struct {
	TrimW, TrimH Millimeter // khổ thành phẩm
	Bleed        Millimeter // tràn lề (mỗi cạnh)
	SafeMargin   Millimeter // lề an toàn — không đặt nội dung quan trọng ra ngoài
	Gutter       Millimeter // rãnh giữa các khung
	DPI          int
	Background   color.RGBA
}

var (
	white = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	black = color.RGBA{0x1A, 0x1A, 0x1A, 0xFF}
)

// SpecA4 là khổ A4 dọc ở 300 DPI: trim 2480×3508 px, cả tràn lề 2551×3579 px.
func SpecA4() PageSpec {
	return PageSpec{TrimW: 210, TrimH: 297, Bleed: 3, SafeMargin: 5, Gutter: 4, DPI: 300, Background: white}
}

// SpecB5 là khổ B5 ISO dọc ở 300 DPI.
func SpecB5() PageSpec {
	return PageSpec{TrimW: 176, TrimH: 250, Bleed: 3, SafeMargin: 5, Gutter: 4, DPI: 300, Background: white}
}

// SpecFor tra khổ trang theo tên ("a4" | "b5"); không rõ thì trả A4.
func SpecFor(name string) PageSpec {
	if name == "b5" || name == "B5" {
		return SpecB5()
	}
	return SpecA4()
}

// MM đổi milimét sang pixel theo DPI của trang.
func (s PageSpec) MM(v Millimeter) float64 {
	return float64(v) / 25.4 * float64(s.DPI)
}

// TrimPxW/TrimPxH là kích thước khung thành phẩm tính bằng pixel.
func (s PageSpec) TrimPxW() int { return int(s.MM(s.TrimW) + 0.5) }
func (s PageSpec) TrimPxH() int { return int(s.MM(s.TrimH) + 0.5) }

// PxW/PxH là kích thước toàn trang KỂ CẢ tràn lề — đây là kích thước ảnh xuất ra để in.
func (s PageSpec) PxW() int { return int(s.MM(s.TrimW+2*s.Bleed) + 0.5) }
func (s PageSpec) PxH() int { return int(s.MM(s.TrimH+2*s.Bleed) + 0.5) }

// BleedPx là độ dày tràn lề tính bằng pixel.
func (s PageSpec) BleedPx() float64 { return s.MM(s.Bleed) }

// Dev đổi một Rect chuẩn hoá sang pixel thiết bị (đã cộng dịch chuyển tràn lề).
func (s PageSpec) Dev(r Rect) (x, y, w, h float64) {
	b := s.BleedPx()
	tw, th := s.MM(s.TrimW), s.MM(s.TrimH)
	return b + r.X*tw, b + r.Y*th, r.W * tw, r.H * th
}

// DevPoint đổi một Point chuẩn hoá sang pixel thiết bị.
func (s PageSpec) DevPoint(p Point) (x, y float64) {
	b := s.BleedPx()
	return b + p.X*s.MM(s.TrimW), b + p.Y*s.MM(s.TrimH)
}

// PanelShape là hình dạng khung tranh.
type PanelShape uint8

const (
	ShapeRect    PanelShape = iota // chữ nhật vuông góc
	ShapeRounded                   // chữ nhật bo góc
)

// Border là nét viền khung.
type Border struct {
	Width Millimeter
	Color color.RGBA
}

// Placeholder là nội dung vẽ khi khung CHƯA có ảnh (giai đoạn 1, hoặc ảnh bị thiếu).
//
// Cố ý đặt trong gói này và kích hoạt bởi Panel.Art == nil: nhờ vậy tầng trên không cần
// biết ảnh là thật hay giữ chỗ, và giai đoạn 2 chỉ việc điền Art mà không sửa gì ở đây.
type Placeholder struct {
	Label string // "T3·K2"
	Note  string // mô tả khung bằng tiếng Việt — để chấm bố cục và typography thật
	Tint  color.RGBA
}

// Panel là một khung tranh trên trang.
type Panel struct {
	ID          string
	Rect        Rect
	Shape       PanelShape
	Corner      Millimeter
	FullBleed   bool  // tràn ra tận mép bleed ở những cạnh chạm khung trim
	Border      Border
	Art         image.Image // nil ⇒ vẽ Placeholder
	ArtFocus    Point       // tâm cắt khi cover-fit; zero value ⇒ {0.5, 0.5}
	Placeholder Placeholder

	// ArtHref là đường dẫn ảnh dùng cho bản SVG (<image xlink:href>). Rỗng ⇒ SVG vẽ ô màu
	// giữ chỗ. Tách khỏi Art vì bản raster cần ảnh đã giải mã, còn SVG chỉ cần tham chiếu.
	ArtHref string
}

// BalloonKind là loại bong bóng.
type BalloonKind uint8

const (
	BalloonSpeech  BalloonKind = iota // ellipse — lời thoại
	BalloonThought                    // mây — độc thoại nội tâm
	BalloonShout                      // gai — hét
	BalloonWhisper                    // ellipse nét đứt — thì thầm
	BalloonBox                        // hộp bo góc — thuyết minh trong khung
)

// ParseBalloonKind đổi khoá tiếng Việt sang BalloonKind.
func ParseBalloonKind(s string) BalloonKind {
	switch s {
	case "doc-thoai":
		return BalloonThought
	case "het":
		return BalloonShout
	case "thi-tham":
		return BalloonWhisper
	case "thuyet-minh":
		return BalloonBox
	default:
		return BalloonSpeech
	}
}

// Balloon là một bong bóng thoại đã được định vị.
type Balloon struct {
	ID   string
	Kind BalloonKind

	// Anchor là VÙNG cho phép đặt bong bóng (chuẩn hoá, trong khung trim).
	// Bộ dựng tự co giãn bong bóng bên trong vùng này cho vừa chữ.
	Anchor Rect

	// ClipTo giới hạn bong bóng không tràn khỏi một khung; zero value = không giới hạn.
	ClipTo *Rect

	// Tail là đích của đuôi (miệng người nói). Zero value ⇒ không vẽ đuôi.
	Tail      Point
	HasTail   bool
	TailWidth Millimeter

	Text  string // tiếng Việt; bộ dựng tự chuẩn hoá NFC
	Style TextStyle

	Fill, Stroke color.RGBA
	StrokeWidth  Millimeter
	PadH, PadV   Millimeter
}

// SFX là chữ tượng thanh.
type SFX struct {
	At       Point
	Text     string
	Style    TextStyle
	Rotation float64    // radian
	Outline  Millimeter // độ dày viền ngoài
	Fill     color.RGBA
	Stroke   color.RGBA
}

// Page là toàn bộ mô tả một trang, đủ để dựng ra PNG và SVG giống hệt nhau.
type Page struct {
	Index      int
	Spec       PageSpec
	Panels     []Panel
	Balloons   []Balloon // vẽ SAU mọi khung, theo đúng thứ tự đọc
	SFX        []SFX
	PageNumber string
	Seed       int64 // quyết định mọi jitter ⇒ dựng lại cho ra kết quả y hệt
}

// Package comic là "khả năng ngang" chuyển thể một dự án sách đã hoàn thành thành
// TRANG TRUYỆN TRANH hoàn chỉnh, xuất bản được: kịch bản trang→khung, bố cục trang,
// kịch bản chữ (bong bóng thoại), prompt sinh ảnh, trang đã dàn (PNG + SVG) và các
// gói xuất bản (PDF 300 DPI / CBZ / EPUB3 fixed-layout).
//
// Giống imp/sim/adapt/tts: đây là tác vụ nhiều bước chạy dài, chỉ ĐỌC dữ liệu có sẵn
// trong store rồi GHI output ra thư mục ngoài (mặc định {novelDir}/truyen-tranh/).
// Không đụng tới Coordinator/Phase/Flow — host bọc bằng guardExclusive để không chạy chồng.
//
// Khác adapt (/video) ở chỗ: adapt chỉ sinh NHIÊN LIỆU (prompt, kịch bản) cho người khác
// dựng video; comic dựng ra SẢN PHẨM CUỐI luôn — trang truyện tranh đã ghép ảnh, đã vẽ
// bong bóng và lồng chữ tiếng Việt, đã đóng gói in được.
package comic

import (
	"context"
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/comicdraw"
	"github.com/voocel/ainovel-cli/internal/store"
)

// LLMChat là giao diện tối thiểu để gọi mô hình đồng bộ (giống adapt.LLMChat).
type LLMChat interface {
	Generate(ctx context.Context, messages []agentcore.Message, tools []agentcore.ToolSpec, opts ...agentcore.CallOption) (*agentcore.LLMResponse, error)
}

// Deps là phụ thuộc chạy do host tiêm vào.
type Deps struct {
	Store   *store.Store
	LLM     LLMChat
	Prompts Prompts
	Fonts   *comicdraw.FontSet

	// Img là nguồn sinh ảnh khung. GIAI ĐOẠN 1 để nil ⇒ mọi khung dùng ảnh giữ chỗ.
	// Giai đoạn 2 tiêm một bản cài bọc client Gemini vào đây; không phần nào khác phải sửa.
	Img ImageSource
}

// Prompts gom các system prompt cho những bước cần LLM (bước render-only không dùng).
type Prompts struct {
	Style     string
	Character string
	Script    string
}

// Product là một loại sản phẩm truyện tranh.
type Product string

const (
	ProductStyle       Product = "style"       // art direction truyện tranh (LLM)
	ProductCharacter   Product = "character"   // model sheet nhân vật (LLM)
	ProductScript      Product = "script"      // kịch bản trang→khung + kịch bản chữ (LLM)
	ProductLayout      Product = "layout"      // hình học bố cục trang (thuần Go)
	ProductPanelPrompt Product = "panelprompt" // bảng prompt khung (render-only)
	ProductRefSheet    Product = "refsheet"    // ẢNH model sheet nhân vật [GĐ2 — cần Deps.Img]
	ProductPanelArt    Product = "panelart"    // ẢNH từng khung        [GĐ2 — cần Deps.Img]
	ProductPage        Product = "page"        // dàn trang + lồng chữ → PNG/SVG
	ProductPublish     Product = "publish"     // đóng gói PDF/CBZ/EPUB
)

// DefaultOrder trả về thứ tự chạy khi người dùng chọn "all".
// Các bước cấp-sách chạy trước để dựng "style bible"; bước sinh ảnh nằm giữa; dàn trang
// và đóng gói chạy cuối. Ở giai đoạn 1, refsheet/panelart tự bỏ qua khi Deps.Img == nil.
func DefaultOrder() []Product {
	return []Product{
		ProductStyle,
		ProductCharacter,
		ProductRefSheet,
		ProductScript,
		ProductLayout,
		ProductPanelPrompt,
		ProductPanelArt,
		ProductPage,
		ProductPublish,
	}
}

// isKnownProduct kiểm tra một product có được hỗ trợ không.
func isKnownProduct(p Product) bool {
	switch p {
	case ProductStyle, ProductCharacter, ProductScript, ProductLayout,
		ProductPanelPrompt, ProductRefSheet, ProductPanelArt, ProductPage, ProductPublish:
		return true
	}
	return false
}

// isBookLevel phân loại product cấp-sách (chạy 1 lần) vs cấp-chương.
func isBookLevel(p Product) bool {
	switch p {
	case ProductStyle, ProductCharacter, ProductRefSheet:
		return true
	}
	return false
}

// needsImageSource cho biết product có bắt buộc Deps.Img không (bước của giai đoạn 2).
func needsImageSource(p Product) bool {
	return p == ProductRefSheet || p == ProductPanelArt
}

// Format là một định dạng đóng gói xuất bản.
type Format string

const (
	FormatPDF  Format = "pdf"  // in ấn 300 DPI, có tràn lề
	FormatCBZ  Format = "cbz"  // truyện tranh số
	FormatEPUB Format = "epub" // EPUB3 fixed-layout
	FormatPNG  Format = "png"  // PNG từng trang (luôn có sẵn từ bước page)
	FormatSVG  Format = "svg"  // SVG vector sửa tay được
)

// DefaultFormats trả về bộ định dạng mặc định khi người dùng không chỉ định.
func DefaultFormats() []Format { return []Format{FormatPDF, FormatCBZ, FormatEPUB} }

func isKnownFormat(f Format) bool {
	switch f {
	case FormatPDF, FormatCBZ, FormatEPUB, FormatPNG, FormatSVG:
		return true
	}
	return false
}

// Options là tham số điều khiển, dùng chung cho cả Web và TUI (mirror).
type Options struct {
	Products []Product // rỗng = DefaultOrder()
	From, To int       // phạm vi chương (0/0 = toàn bộ đã hoàn thành)
	OutDir   string    // rỗng = {novelDir}/truyen-tranh/

	// StylePreset là khoá trong StylePresets ("thieu-nhi", "manga"...). Rỗng = "thieu-nhi".
	// StyleHint là chữ tự do người dùng nhập, ghép THÊM vào preset chứ không thay thế.
	StylePreset string
	StyleHint   string

	PageSize  string   // "a4" (mặc định) | "b5"
	Formats   []Format // rỗng = DefaultFormats()
	Overwrite bool     // ghi đè file đã có; false = bỏ qua (resume incremental)

	// MaxImages là cầu dao chi phí: trần số ảnh sinh trong MỘT lần chạy (0 = không giới hạn).
	// Chỉ có tác dụng ở giai đoạn 2. Sinh ảnh là bước TỐN TIỀN duy nhất của tính năng này.
	MaxImages int

	// ImageSize truyền xuống nguồn sinh ảnh: "1K" (đọc màn hình) | "2K" (in được).
	// Rỗng = "2K" khi có FormatPDF trong Formats, ngược lại "1K".
	ImageSize string
}

// Stage là giai đoạn tiến trình, chiếu ra UI.
type Stage string

const (
	StageContext     Stage = "context"
	StageStyle       Stage = "style"
	StageCharacter   Stage = "character"
	StageRefSheet    Stage = "refsheet"
	StageScript      Stage = "script"
	StageLayout      Stage = "layout"
	StagePanelPrompt Stage = "panelprompt"
	StagePanelArt    Stage = "panelart"
	StagePage        Stage = "page"
	StagePublish     Stage = "publish"
	StageDone        Stage = "done"
	StageError       Stage = "error"
)

// Event là một mốc tiến trình gửi qua kênh.
type Event struct {
	Time    time.Time
	Stage   Stage
	Product Product
	Current int
	Total   int
	Message string // tiếng Việt có dấu
	Err     error
}

// Output mô tả một file đã ghi.
type Output struct {
	Product Product `json:"product"`
	Path    string  `json:"path"`
	Bytes   int     `json:"bytes"`
}

package comic

// Các struct dưới đây vừa là schema output LLM (bọc trong <output>), vừa là dạng lưu
// JSON trên đĩa. Quy ước song ngữ giống adapt/model.go: *_prompt viết TIẾNG ANH (đưa vào
// bộ sinh ảnh); mô tả/nhãn/lời thoại viết TIẾNG VIỆT có dấu.

// StyleResult — art direction cấp sách cho bản truyện tranh.
type StyleResult struct {
	Overall     string   `json:"overall"`      // phong cách tổng thể (VI)
	Palette     []string `json:"palette"`      // bảng màu chủ đạo (VI)
	LineArt     string   `json:"line_art"`     // đặc tả nét vẽ (VI)
	Lettering   string   `json:"lettering"`    // quy ước bong bóng & chữ (VI)
	StyleTokens []string `json:"style_tokens"` // token EN chèn vào MỌI prompt khung
	Negative    []string `json:"negative"`     // negative token EN dùng chung
	Locations   []Locale `json:"locations"`    // bối cảnh chính
}

type Locale struct {
	Name        string `json:"name"`         // tên bối cảnh (VI)
	Description string `json:"description"`  // mô tả (VI)
	ImagePrompt string `json:"image_prompt"` // prompt sinh ảnh (EN)
}

// CharacterSheet — model sheet một nhân vật, dùng cho cả prompt lẫn ảnh tham chiếu.
type CharacterSheet struct {
	Name            string   `json:"name"`
	Role            string   `json:"role"`             // vai trò trong truyện (VI)
	Appearance      string   `json:"appearance"`       // ngoại hình (VI)
	Wardrobe        string   `json:"wardrobe"`         // trang phục (VI)
	Palette         []string `json:"palette"`          // bảng màu riêng
	CanonicalPrompt string   `json:"canonical_prompt"` // mô tả CỐ ĐỊNH (EN) — chèn nguyên văn vào mọi khung
	SheetPrompt     string   `json:"sheet_prompt"`     // prompt sinh ẢNH model sheet (EN)
	NegativePrompt  string   `json:"negative_prompt"`  // negative riêng (EN)
}

// ScriptResult — kịch bản truyện tranh một chương.
type ScriptResult struct {
	Chapter int        `json:"chapter"`
	Title   string     `json:"title"`
	Pages   []PageSpec `json:"pages"`
}

// PageSpec — một trang truyện tranh.
type PageSpec struct {
	PageNo int     `json:"page_no"`
	Beat   string  `json:"beat"`             // nhịp/vai trò của trang (VI)
	Panels []Panel `json:"panels"`
	Cliff  string  `json:"cliff,omitempty"`  // cú hích trước khi lật trang (VI)
	Spread bool    `json:"spread,omitempty"` // trang đôi cho cao trào
}

// Panel — một khung tranh. Size/RowBreak là NGỮ NGHĨA do LLM quyết;
// hình học x,y,w,h do layout.go tính xác định từ đó (xem docs/comic.md).
type Panel struct {
	PanelNo        int       `json:"panel_no"` // thứ tự đọc: trái→phải, trên→dưới
	Size           string    `json:"size"`     // nho | vua | lon | tran-trang
	RowBreak       bool      `json:"row_break"`
	Description    string    `json:"description"` // diễn biến trong khung (VI)
	Shot           string    `json:"shot"`        // toan | trung | can | dac-ta
	Characters     []string  `json:"characters"`  // tên nhân vật xuất hiện → tra model sheet
	ImagePrompt    string    `json:"image_prompt"`
	NegativePrompt string    `json:"negative_prompt,omitempty"`
	ReserveFor     string    `json:"reserve_for,omitempty"` // vùng chừa chỗ đặt bong bóng
	Balloons       []Balloon `json:"balloons,omitempty"`
	SFX            []SFX     `json:"sfx,omitempty"`
}

// Balloon — một bong bóng thoại / ô thuyết minh trong khung.
type Balloon struct {
	Order   int    `json:"order"`   // thứ tự đọc trong khung
	Kind    string `json:"kind"`    // thoai | doc-thoai | het | thi-tham | thuyet-minh
	Speaker string `json:"speaker"` // rỗng nếu là ô thuyết minh
	Text    string `json:"text"`    // lời thoại TIẾNG VIỆT CÓ DẤU
	Anchor  string `json:"anchor"`  // tren-trai | tren-giua | tren-phai | duoi-trai | ...
	TailTo  string `json:"tail_to,omitempty"`
}

// SFX — chữ tượng thanh.
type SFX struct {
	Text   string `json:"text"`   // RẦM! · VÙ · TÍCH TẮC
	Anchor string `json:"anchor"`
	Scale  string `json:"scale"` // nho | vua | lon
}

// PanelPrompt là một dòng trong bảng prompt khung — HỢP ĐỒNG giữa giai đoạn 1 và 2.
// Giai đoạn 1 ghi tệp này ra prompts/panels.json KỂ CẢ khi ảnh chưa tồn tại; giai đoạn 2
// chỉ việc đọc nó, gọi API và đặt tệp vào đúng ArtFile. Người dùng cũng có thể tự vẽ rồi
// thả ảnh vào đúng đường dẫn đó — bộ dàn trang không phân biệt.
type PanelPrompt struct {
	Chapter  int      `json:"chapter"`
	Page     int      `json:"page"`
	Panel    int      `json:"panel"`
	ArtFile  string   `json:"art_file"` // đường dẫn TƯƠNG ĐỐI trong outDir
	Prompt   string   `json:"prompt_en"`
	Negative string   `json:"negative_en"`
	Refs     []string `json:"refs,omitempty"` // đường dẫn tương đối tới ảnh model sheet
	Aspect   string   `json:"aspect_ratio"`
	Size     string   `json:"image_size"`
}

// panelSizeWeight quy đổi ngữ nghĩa kích thước khung sang trọng số bố cục.
// Đây là bảng tra DUY NHẤT quyết định khung to nhỏ — sửa ở đây là đổi nhịp cả cuốn.
//
// Các giá trị được chọn để ăn khớp với rowBudget=3.0 trong layout.go, cho ra đúng những
// tổ hợp hàng quen thuộc của truyện tranh:
//
//	nho+nho+nho = 3.0 ✓   vua+vua = 3.0 ✓   lon+nho = 3.0 ✓   vua+nho = 2.5 ✓
//	lon+vua = 3.5 ✗ (tách hàng)             lon+lon = 4.0 ✗ (tách hàng)
//
// Đừng nâng "lon" lên 2.5: khi đó lon+nho = 3.5 sẽ bị tách hàng, mà cặp khung lớn cạnh
// khung nhỏ lại là bố cục phổ biến nhất trong truyện tranh.
func panelSizeWeight(size string) float64 {
	switch size {
	case "tran-trang":
		return 0 // 0 = chiếm trọn trang, layout.go xử lý riêng
	case "lon":
		return 2.0
	case "vua":
		return 1.5
	case "nho":
		return 1.0
	default:
		return 1.5 // không rõ thì coi như vừa
	}
}

// normalizeBalloonKind ép loại bong bóng về một trong các giá trị hợp lệ.
func normalizeBalloonKind(kind string) string {
	switch kind {
	case "thoai", "doc-thoai", "het", "thi-tham", "thuyet-minh":
		return kind
	default:
		return "thoai"
	}
}

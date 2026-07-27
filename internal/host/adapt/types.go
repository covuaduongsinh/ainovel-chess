// Package adapt là "khả năng ngang" chuyển thể một dự án sách đã hoàn thành
// thành bộ sản phẩm phục vụ sản xuất video: kịch bản, phân cảnh, thiết kế nhân
// vật/đạo cụ, concept art direction, chỉ đạo animation và các bảng prompt.
//
// Giống imp/sim: đây là tác vụ LLM-nặng nhiều bước, chỉ ĐỌC dữ liệu có sẵn trong
// store rồi GHI output ra thư mục ngoài (mặc định {novelDir}/video/). Không đụng
// tới Coordinator/Phase/Flow — host bọc bằng guardExclusive để không chạy chồng.
package adapt

import (
	"context"
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/store"
)

// LLMChat là giao diện tối thiểu để gọi mô hình đồng bộ (giống sim.LLMChat).
type LLMChat interface {
	Generate(ctx context.Context, messages []agentcore.Message, tools []agentcore.ToolSpec, opts ...agentcore.CallOption) (*agentcore.LLMResponse, error)
}

// Deps là phụ thuộc chạy do host tiêm vào.
type Deps struct {
	Store   *store.Store
	LLM     LLMChat
	Prompts Prompts
}

// Prompts gom 6 system prompt cho các product cần LLM (product render-only không dùng).
type Prompts struct {
	Concept     string
	Character   string
	Prop        string
	Consistency string
	Screenplay  string
	Storyboard  string
}

// Product là một loại sản phẩm chuyển thể.
type Product string

const (
	ProductConcept     Product = "concept"     // art direction toàn cục
	ProductCharacter   Product = "character"   // thiết kế nhân vật
	ProductProp        Product = "prop"        // đạo cụ
	ProductConsistency Product = "consistency" // bảng nhất quán (khóa token trực quan)
	ProductScreenplay  Product = "screenplay"  // kịch bản
	ProductStoryboard  Product = "storyboard"  // phân cảnh / shot list
	ProductAnimation   Product = "animation"   // chỉ đạo animation (render-only)
	ProductImagePrompt Product = "imageprompt" // bảng prompt ảnh (render-only)
	ProductVideoPrompt Product = "videoprompt" // bảng prompt video (render-only)
)

// DefaultOrder trả về thứ tự chạy khi người dùng chọn "all".
// Các bước hình ảnh chạy trước để tạo "style bible", storyboard tiêm token chuẩn
// từ consistency-bible vào từng prompt; các bản render phẳng chạy cuối.
func DefaultOrder() []Product {
	return []Product{
		ProductConcept,
		ProductCharacter,
		ProductProp,
		ProductConsistency,
		ProductScreenplay,
		ProductStoryboard,
		ProductAnimation,
		ProductImagePrompt,
		ProductVideoPrompt,
	}
}

// isKnownProduct kiểm tra một product có được hỗ trợ không.
func isKnownProduct(p Product) bool {
	switch p {
	case ProductConcept, ProductCharacter, ProductProp, ProductConsistency,
		ProductScreenplay, ProductStoryboard, ProductAnimation, ProductImagePrompt, ProductVideoPrompt:
		return true
	}
	return false
}

// Grouping quyết định thứ tự xử lý + đầu ra đóng gói.
type Grouping string

const (
	// GroupByChapter (mặc định): chạy cấp-sách 1 lần rồi duyệt tập→chương, mỗi chương làm
	// trọn các loại cấp-chương và ghi đóng gói lồng video/tap-{VV}/chuong-{NN}/ — tập trước
	// hoàn chỉnh trước, dễ dựng video từng tập.
	GroupByChapter Grouping = "chapter"
	// GroupByProduct (tương thích cũ): mỗi loại sản phẩm quét toàn bộ chương; không đóng gói lồng.
	GroupByProduct Grouping = "product"
)

// Options là tham số điều khiển, dùng chung cho cả Web và TUI (mirror).
type Options struct {
	Products  []Product // rỗng = DefaultOrder()
	Grouping  Grouping  // rỗng = GroupByChapter
	From, To  int       // phạm vi chương (0/0 = toàn bộ đã hoàn thành); chỉ dùng cho screenplay/storyboard/animation
	OutDir    string    // rỗng = {novelDir}/video/
	Overwrite bool      // ghi đè file đã tồn tại; false = bỏ qua (resume incremental)
	StyleHint string    // gợi ý phong cách hình ảnh người dùng nhập
}

// isBookLevel phân loại product cấp-sách (chạy 1 lần) vs cấp-chương.
func isBookLevel(p Product) bool {
	switch p {
	case ProductConcept, ProductCharacter, ProductProp, ProductConsistency:
		return true
	}
	return false
}

// normalizeGrouping trả về Grouping hiệu lực (mặc định GroupByChapter).
func normalizeGrouping(g Grouping) Grouping {
	if g == GroupByProduct {
		return GroupByProduct
	}
	return GroupByChapter
}

// Stage là giai đoạn tiến trình, chiếu ra UI.
type Stage string

const (
	StageContext     Stage = "context"
	StageConcept     Stage = "concept"
	StageCharacter   Stage = "character"
	StageProp        Stage = "prop"
	StageConsistency Stage = "consistency"
	StageScreenplay  Stage = "screenplay"
	StageStoryboard  Stage = "storyboard"
	StageAnimation   Stage = "animation"
	StageImagePrompt Stage = "imageprompt"
	StageVideoPrompt Stage = "videoprompt"
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

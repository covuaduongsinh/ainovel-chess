// Package rules triển khai lớp đầu vào sở thích người dùng (Policy): chuẩn hóa và hợp nhất các quy tắc viết từ nhiều nguồn thành
// ảnh chụp của sách (xem snapshot.go), lúc chạy được novel_context tiêm vào và commit_chapter kiểm tra cơ học.
//
// Rule là loại sự thật thứ tư, ngang hàng với Progress / Checkpoint / Artifact, nhưng tính chất ngược lại:
// ba loại trước là đầu ra hệ thống, Rule là đầu vào ý định người dùng được lưu trữ lâu dài.
//
// Ràng buộc thiết kế (không thể thỏa hiệp):
//   - Công cụ chỉ trả về sự thật, không trả chỉ thị (Violation là sự thật, editor quyết định có kích hoạt viết lại không)
//   - Không thêm đường verdict mới (tái dùng PendingRewrites)
//   - Không thêm trường mức độ nghiêm khắc (severity được ánh xạ cố định theo loại quy tắc, editor tự phán quyết ngữ nghĩa)
//   - Không động đến Flow Router (rule không tham gia định tuyến)
package rules

// SourceKind đánh dấu nguồn tệp quy tắc, chỉ dùng để tạo nhãn nguồn (ví dụ global:my-style.md).
type SourceKind int

const (
	// SourceGlobal — sở thích toàn cục của người dùng (tất cả .md trong thư mục ~/.ainovel/rules/, hợp nhất theo thứ tự từ điển tên tệp), dùng chung cho nhiều sách.
	SourceGlobal SourceKind = iota
	// SourceProject — quy tắc của sách này (tất cả .md trong thư mục ./.ainovel/rules/, hợp nhất theo thứ tự từ điển tên tệp), ưu tiên cao nhất.
	SourceProject
)

// String trả về tên có thể đọc được của nguồn, dùng làm tiền tố nhãn nguồn.
func (k SourceKind) String() string {
	switch k {
	case SourceGlobal:
		return "global"
	case SourceProject:
		return "project"
	default:
		return "unknown"
	}
}

// WordRange biểu thị phạm vi số chữ cho phép của chương; nil nghĩa là chưa khai báo.
type WordRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// Structured chứa các trường quy tắc có cấu trúc có thể kiểm tra cơ học (kết quả ứng viên/hợp nhất sau khi chuẩn hóa từng nguồn).
type Structured struct {
	Genre            string         `json:"genre,omitempty"`
	ChapterWords     *WordRange     `json:"chapter_words,omitempty"`
	ForbiddenChars   []string       `json:"forbidden_chars,omitempty"`
	ForbiddenPhrases []string       `json:"forbidden_phrases,omitempty"`
	FatigueWords     map[string]int `json:"fatigue_words,omitempty"`
}

// IsEmpty dùng để xác định xem có hoàn toàn không có quy tắc có cấu trúc nào không; checker có thể bỏ qua dựa vào đây.
func (s Structured) IsEmpty() bool {
	return s.Genre == "" &&
		s.ChapterWords == nil &&
		len(s.ForbiddenChars) == 0 &&
		len(s.ForbiddenPhrases) == 0 &&
		len(s.FatigueWords) == 0
}

// Severity đánh dấu mức độ nghiêm trọng của Violation.
// Ánh xạ cố định (người dùng không thể cấu hình):
//
//	forbidden_chars xuất hiện        -> Error
//	forbidden_phrases xuất hiện      -> Error
//	fatigue_words vượt ngưỡng        -> Warning
//	chapter_words độ lệch < 20%      -> Warning
//	chapter_words độ lệch >= 20%     -> Error
type Severity string

const (
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// ChapterWordsDeviationThreshold xác định ngưỡng nâng độ lệch chapter_words lên error (20%).
const ChapterWordsDeviationThreshold = 0.20

// Violation là đầu ra của checker: tuyên bố sự thật về việc chương này vi phạm một quy tắc cơ học nào đó.
//
// Lưu ý: commit_chapter chuyển tiếp violations vào JSON trả về, không chặn commit;
// editor khi thẩm định sẽ ánh xạ những sự thật này vào bảy chiều hiện có (aesthetic/pacing/character/consistency),
// để LLM tự quyết định có nâng verdict kích hoạt polish/rewrite không.
type Violation struct {
	Rule      string   `json:"rule"`                // forbidden_chars / forbidden_phrases / fatigue_words / chapter_words
	Target    string   `json:"target,omitempty"`    // đối tượng vi phạm cụ thể (từ/ký tự nào); chapter_words để trống
	Limit     any      `json:"limit,omitempty"`     // ngưỡng; fatigue_words=int / chapter_words="3000-6000" / forbidden_*=trống
	Actual    any      `json:"actual"`              // giá trị thực tế; fatigue_words/forbidden_*=số lần xuất hiện / chapter_words=số chữ chương này
	Deviation float64  `json:"deviation,omitempty"` // tỷ lệ độ lệch chapter_words (0~1), các quy tắc khác để trống
	Severity  Severity `json:"severity"`            // error / warning
}

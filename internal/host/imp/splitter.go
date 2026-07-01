package imp

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/voocel/ainovel-cli/internal/utils"
)

// Regex mặc định để nhận diện tiêu đề chương. Bao phủ các dạng tiêu đề phổ biến trong tiếng Trung
// (第N章/回/话/卷/节/幕、卷N、序章/楔子/尾声/番外/外传, v.v.) và tiếng Anh
// (Chapter N, Prologue, Epilogue), tương thích với tiền tố Markdown (# / ##),
// tiền tố「正文 第N章」của định dạng txt từ trang Qidian, cũng như tiêu đề được bao bởi【】〖〗.
//
// Các named group: nhóm phụ đề được ưu tiên hơn nhóm từ khóa (khi trích xuất sẽ thử theo thứ tự priority):
//   - cn    phụ đề chương đánh số (văn bản sau 第X章/回/话/卷/节/幕)
//   - vol   phụ đề quyển độc lập (văn bản sau 卷X)
//   - sp    phụ đề đơn vị đặc biệt (văn bản sau 序章/楔子/尾声/番外)
//   - en    phụ đề chương tiếng Anh (văn bản sau Chapter X / Prologue / Epilogue)
//   - spkw  từ khóa đơn vị đặc biệt (dùng làm tiêu đề khi không có phụ đề, ví dụ「楔子」「番外」)
//   - enkw  từ khóa đơn vị đặc biệt tiếng Anh (dùng làm tiêu đề khi không có phụ đề, ví dụ「Prologue」)

// ws là nội dung của lớp ký tự: khoảng trắng ASCII + khoảng trắng toàn góc (full-width).
// \s trong Go RE2 chỉ khớp khoảng trắng ASCII, trong khi tiêu đề theo kiểu Trung Quốc
// thường dùng U+3000 làm dấu phân cách (ví dụ「第一章　风起」).
const ws = `\s\x{3000}`

// cnNum là tập các ký tự số có thể dùng cho đánh số chương: chữ số Ả Rập / toàn góc / chữ số Trung Quốc thường / chữ số Trung Quốc hoa phồn thể (壹贰叁…萬).
const cnNum = `零〇○Ｏ０一二三四五六七八九十百千万两壹贰貳叁參肆伍陆陸柒捌玖拾佰仟萬兩\d`

// sub là mẫu bắt phụ đề: lấy đến cuối dòng nhưng không nuốt ký tự đóng ngoặc bên phải (】〗), để dành cho dấu ngoặc đóng tùy chọn ở cuối.
const sub = `[^】〗\n]*`

var defaultChapterRegex = regexp.MustCompile(
	`(?im)^#{0,2}[` + ws + `]*(?:正文[` + ws + `]*)?[【〖]?[` + ws + `]*(?:` +
		`第\s*(?:[` + cnNum + `]+)\s*(?:章|回|话|卷|节|幕)` +
		`(?:[:：．\.` + ws + `]+(?P<cn>` + sub + `))?` +
		`|` +
		`卷\s*(?:[` + cnNum + `]+)` +
		`(?:[:：．\.` + ws + `]+(?P<vol>` + sub + `))?` +
		`|` +
		`(?P<spkw>序章|序幕|楔子|引子|前言|序言|尾声|终章|后记|番外|外传)` +
		`(?:[:：．\.` + ws + `]+(?P<sp>` + sub + `))?` +
		`|` +
		`(?:Chapter\s+(?:\d+|[IVXLCDM]+)|(?P<enkw>Prologue|Epilogue))` +
		`(?:[:：．\.` + ws + `]+(?P<en>` + sub + `))?` +
		`)[` + ws + `]*[】〗]?[` + ws + `]*$`,
)

// SplitFile phân tách một tệp văn bản đơn thành danh sách các chương.
func SplitFile(path string) ([]Chapter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read source: %w", err)
	}
	text := utils.DecodeText(data)
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("source file is empty: %s", path)
	}
	return splitText(text, defaultChapterRegex), nil
}

// splitText là phiên bản hàm thuần túy để phân tách, tiện cho unit test.
func splitText(text string, pattern *regexp.Regexp) []Chapter {
	lines := strings.Split(text, "\n")
	type marker struct {
		line  int
		title string
	}
	var marks []marker
	for i, ln := range lines {
		if loc := pattern.FindStringSubmatchIndex(ln); loc != nil {
			marks = append(marks, marker{line: i, title: extractTitle(ln, pattern, loc, len(marks)+1)})
		}
	}
	if len(marks) == 0 {
		return nil
	}

	chapters := make([]Chapter, 0, len(marks))
	for i, m := range marks {
		end := len(lines)
		if i+1 < len(marks) {
			end = marks[i+1].line
		}
		body := strings.Join(lines[m.line+1:end], "\n")
		body = stripTrailingNoise(body)
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		chapters = append(chapters, Chapter{Title: m.title, Content: body})
	}
	return chapters
}

// extractTitle trích xuất tiêu đề chương từ dòng khớp; ưu tiên lấy named capture group, nếu không có thì dùng số thứ tự chương làm dự phòng.
func extractTitle(line string, pattern *regexp.Regexp, loc []int, fallbackNum int) string {
	subnames := pattern.SubexpNames()
	priority := []string{"cn", "vol", "sp", "en", "spkw", "enkw"}
	for _, name := range priority {
		idx := pattern.SubexpIndex(name)
		if idx <= 0 {
			continue
		}
		if loc[2*idx] < 0 {
			continue
		}
		if t := strings.TrimSpace(line[loc[2*idx]:loc[2*idx+1]]); t != "" {
			return t
		}
	}
	// Dự phòng: lấy nhóm bắt đầu tiên không rỗng (phòng thủ, các named group của regex mặc định đã bao phủ mọi nhánh)
	for i := 1; i < len(subnames); i++ {
		if loc[2*i] < 0 {
			continue
		}
		if t := strings.TrimSpace(line[loc[2*i]:loc[2*i+1]]); t != "" {
			return t
		}
	}
	return fmt.Sprintf("Chuong %d", fallbackNum)
}

// stripTrailingNoise loại bỏ các đuôi nhiễu phổ biến (ví dụ: license trailer của Project Gutenberg).
var trailerRe = regexp.MustCompile(`(?im)^\s*Project Gutenberg(?:\(TM\)|™)?[\s\S]*$`)

func stripTrailingNoise(content string) string {
	if loc := trailerRe.FindStringIndex(content); loc != nil {
		return strings.TrimRight(content[:loc[0]], " \t\n")
	}
	return content
}

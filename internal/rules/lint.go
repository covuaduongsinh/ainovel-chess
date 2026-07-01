package rules

import (
	"regexp"
	"strings"
)

// Lint kiểm tra đường đáy sản phẩm tích hợp sẵn: quét cơ chế sót trong nội dung, không liên quan đến quy tắc người dùng, luôn chạy khi commit.
// Cùng hợp đồng với Check — chỉ trả về sự thật (nguyên tắc sắt một), không chặn luồng, do thẩm định/người dùng phán quyết.
//
// Hiện tại ba loại (đều từ lỗi thực chứng của bản chạy dài):
//   - markdown_residue: nội dung còn sót ** in đậm, dòng tiêu đề # ngoài dòng đầu (khi xuất txt sẽ lộ ký hiệu)
//   - non_cjk_fragments: đoạn ký tự Latin liên tiếp (mô hình trộn ngôn ngữ, ví dụ nội dung tiếng Việt lẫn "pattern")
func Lint(text string) []Violation {
	var vs []Violation
	vs = appendMarkdownResidue(vs, text)
	vs = appendNonCJKFragments(vs, text)
	return vs
}

func appendMarkdownResidue(vs []Violation, text string) []Violation {
	if n := strings.Count(text, "**"); n > 0 {
		vs = append(vs, Violation{
			Rule:     "markdown_residue",
			Target:   "**",
			Actual:   n,
			Severity: SeverityWarning,
		})
	}
	headings := 0
	seenContent := false
	for line := range strings.SplitSeq(text, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		// Dòng # tiêu đề ở dòng không rỗng đầu tiên là định dạng hợp lệ của tệp chương (không cố định theo số dòng, chấp nhận dòng trống đầu)
		first := !seenContent
		seenContent = true
		if !first && strings.HasPrefix(t, "#") {
			headings++
		}
	}
	if headings > 0 {
		vs = append(vs, Violation{
			Rule:     "markdown_residue",
			Target:   "#",
			Actual:   headings,
			Severity: SeverityWarning,
		})
	}
	return vs
}

var latinFragmentRe = regexp.MustCompile(`[A-Za-z]{2,}`)

// appendNonCJKFragments báo cáo tổng số lần và ví dụ không trùng lặp của các đoạn ký tự Latin.
// Tiếng Anh hợp lệ trong đề tài hiện đại (tên thương hiệu/viết tắt) cũng bị khớp — sự thật mức warning, do thẩm định phán quyết theo đề tài.
func appendNonCJKFragments(vs []Violation, text string) []Violation {
	matches := latinFragmentRe.FindAllString(text, -1)
	if len(matches) == 0 {
		return vs
	}
	seen := make(map[string]struct{})
	var examples []string
	for _, m := range matches {
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		if len(examples) < 3 {
			examples = append(examples, m)
		}
	}
	return append(vs, Violation{
		Rule:     "non_cjk_fragments",
		Target:   strings.Join(examples, "、"),
		Actual:   len(matches),
		Severity: SeverityWarning,
	})
}

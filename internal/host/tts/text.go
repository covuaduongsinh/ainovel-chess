package tts

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Bộ biến đổi Markdown → văn bản đọc được.
//
// Nguyên tắc: chỉ gỡ ký hiệu định dạng, KHÔNG diễn giải lại nội dung. Vbee đã tự bung
// số Ả Rập, ngày tháng và tiền tệ; ta bung thêm sẽ đọc đôi. Số La Mã cũng cố ý không
// chuẩn hóa vì normalizer sẽ phá chữ "I", "V", "X" đứng một mình trong thoại và tên riêng.

var (
	reCodeFence = regexp.MustCompile("(?s)```.*?```")
	reHeading   = regexp.MustCompile(`^\s*#{1,6}\s+(.*)$`)
	// RE2 không có backreference nên phải liệt kê từng ký tự thay vì (\1\s*){2,}.
	reHRule      = regexp.MustCompile(`^\s*(-[ \t]*){3,}$|^\s*(\*[ \t]*){3,}$|^\s*(_[ \t]*){3,}$`)
	reQuote      = regexp.MustCompile(`^\s*>\s?`)
	reBullet     = regexp.MustCompile(`^\s*[-*+]\s+`)
	reImage      = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	reLink       = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	reCodeSpan   = regexp.MustCompile("`([^`]+)`")
	reStrong     = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reStrongU    = regexp.MustCompile(`__(.+?)__`)
	reStrike     = regexp.MustCompile(`~~(.+?)~~`)
	reEm         = regexp.MustCompile(`\*(.+?)\*`)
	reEmUnder    = regexp.MustCompile(`(^|[^\p{L}\p{N}_])_([^_\n]+)_($|[^\p{L}\p{N}_])`)
	reHTMLTag    = regexp.MustCompile(`</?[A-Za-z][^>]*>`)
	reMDEscape   = regexp.MustCompile(`\\([\\` + "`" + `*_{}\[\]()#+\-.!~>])`)
	reSpaces     = regexp.MustCompile(`[ \t]+`)
	reBlankLines = regexp.MustCompile(`\n{3,}`)
)

// reChuong khớp dòng mở đầu dạng "Chương 12", không phân biệt hoa thường.
var reChuong = regexp.MustCompile(`(?i)^\s*chương\s+(\d+)\s*([:：.\-–—]\s*)?(.*)$`)

// CleanChapter biến nội dung Markdown của một chương thành văn bản thuần để đọc.
//
// chapter là số chương toàn cục, outlineTitle là tiêu đề lấy từ dàn ý (có thể rỗng).
// Kết quả luôn mở đầu bằng một "lời dẫn" nói rõ đang đọc chương mấy, vì phần lớn tệp
// chương trong repo này vào thẳng văn xuôi, không có dòng tiêu đề nào.
func CleanChapter(chapter int, outlineTitle, md string, sceneBreak string) string {
	md = strings.ReplaceAll(md, "\r\n", "\n")
	md = strings.ReplaceAll(md, "\r", "\n")
	md = reCodeFence.ReplaceAllString(md, "")

	var (
		out         []string
		headingText string
		seenBody    bool
	)
	for _, line := range strings.Split(md, "\n") {
		switch {
		case reHRule.MatchString(line):
			// Đường kẻ ngang = ranh giới cảnh. Ngắt đoạn, hoặc đọc câu chuyển cảnh.
			out = append(out, "")
			if s := strings.TrimSpace(sceneBreak); s != "" {
				out = append(out, ensureSentenceEnd(s))
			}
			continue
		case reHeading.MatchString(line):
			text := strings.TrimSpace(reHeading.FindStringSubmatch(line)[1])
			if !seenBody && headingText == "" {
				// Tiêu đề đầu tệp: cất lại để dựng lời dẫn, không đọc thành câu riêng.
				headingText = text
				continue
			}
			// Tiêu đề giữa chương là một câu nên được ĐỌC, không im lặng bỏ đi.
			if text != "" {
				out = append(out, ensureSentenceEnd(text))
				seenBody = true
			}
			continue
		}

		line = reQuote.ReplaceAllString(line, "")
		line = reBullet.ReplaceAllString(line, "")
		line = cleanInline(line)
		line = strings.TrimSpace(reSpaces.ReplaceAllString(line, " "))
		if line != "" {
			seenBody = true
		}
		out = append(out, line)
	}

	body := reBlankLines.ReplaceAllString(strings.Join(out, "\n"), "\n\n")
	body = strings.Trim(body, "\n")

	lead, body := chapterLeadIn(chapter, outlineTitle, headingText, body)
	if body == "" {
		if lead == "" {
			return ""
		}
		return lead
	}
	if lead == "" {
		return body
	}
	return lead + "\n\n" + body
}

// cleanInline gỡ các ký hiệu inline. Thứ tự quan trọng: ảnh trước liên kết, ký hiệu
// dài trước ký hiệu ngắn (** trước *), nếu không sẽ ăn nhầm.
func cleanInline(s string) string {
	s = reImage.ReplaceAllString(s, "")
	s = reLink.ReplaceAllString(s, "$1")
	s = reCodeSpan.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, "`", "")
	s = reStrong.ReplaceAllString(s, "$1")
	s = reStrongU.ReplaceAllString(s, "$1")
	s = reStrike.ReplaceAllString(s, "$1")
	s = reEm.ReplaceAllString(s, "$1")
	// Nghiêng bằng gạch dưới: chỉ khi hai gạch nằm ở ranh giới không-phải-chữ,
	// để tên_có_gạch_dưới và mã định danh không bị xé.
	s = reEmUnder.ReplaceAllString(s, "$1$2$3")
	s = reHTMLTag.ReplaceAllString(s, "")
	s = reMDEscape.ReplaceAllString(s, "$1")
	return stripPictographs(s)
}

// stripPictographs bỏ emoji và ký hiệu trang trí — Vbee sẽ đọc thành tên ký hiệu,
// nghe rất lạ giữa văn xuôi.
//
// CỐ Ý GIỮ \p{Pd} (gạch ngang các loại): dấu "—" là ký hiệu mở lời thoại xuyên suốt
// bản thảo tiếng Việt trong repo này, và Vbee đọc nó thành một nhịp nghỉ.
func stripPictographs(s string) string {
	if !strings.ContainsFunc(s, isPictograph) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isPictograph(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isPictograph(r rune) bool {
	switch {
	case r >= 0x2190 && r <= 0x21FF: // mũi tên
		return true
	case r >= 0x2600 && r <= 0x27BF: // ký hiệu linh tinh + dingbat
		return true
	case r >= 0x2B00 && r <= 0x2BFF:
		return true
	case r >= 0xFE00 && r <= 0xFE0F: // bộ chọn biến thể
		return true
	case r >= 0x1F000 && r <= 0x1FAFF:
		return true
	}
	return false
}

// chapterLeadIn dựng câu mở đầu và trả về (lời dẫn, phần thân đã bỏ dòng tiêu đề nếu có).
//
// Ba dạng tiêu đề gặp trong repo — "# Chương 1: X", "## Chương 3: X" và "Chương 1: X"
// trần — đều quy về một dạng đọc thống nhất. Phần lớn chương KHÔNG có tiêu đề nào,
// khi đó lời dẫn được dựng từ tiêu đề trong dàn ý.
func chapterLeadIn(chapter int, outlineTitle, headingText, body string) (string, string) {
	// Dạng 3: dòng đầu của thân là "Chương N: ..." trần.
	if firstLine, rest, ok := splitFirstLine(body); ok && reChuong.MatchString(firstLine) {
		return formatLead(chapter, reChuong.FindStringSubmatch(firstLine)[3], outlineTitle), rest
	}
	// Dạng 1 & 2: tiêu đề ATX đầu tệp đã được cất vào headingText.
	if headingText != "" {
		if m := reChuong.FindStringSubmatch(headingText); m != nil {
			return formatLead(chapter, m[3], outlineTitle), body
		}
		// Tiêu đề ATX không chứa chữ "Chương" (vd "# Đêm quân cờ thức giấc").
		return formatLead(chapter, headingText, outlineTitle), body
	}
	// Không có tiêu đề nào — trường hợp phổ biến nhất.
	return formatLead(chapter, "", outlineTitle), body
}

// formatLead ghép "Chương N. Tên chương." Tên lấy theo thứ tự ưu tiên: tên đọc được
// từ chính tệp chương, rồi tới tiêu đề trong dàn ý.
func formatLead(chapter int, titleFromFile, outlineTitle string) string {
	title := strings.TrimSpace(titleFromFile)
	if title == "" {
		title = strings.TrimSpace(outlineTitle)
	}
	if chapter <= 0 {
		if title == "" {
			return ""
		}
		return ensureSentenceEnd(title)
	}
	if title == "" {
		return fmt.Sprintf("Chương %d.", chapter)
	}
	return fmt.Sprintf("Chương %d. %s", chapter, ensureSentenceEnd(title))
}

// splitFirstLine tách dòng không rỗng đầu tiên khỏi phần còn lại.
func splitFirstLine(s string) (string, string, bool) {
	s = strings.TrimLeft(s, "\n")
	if s == "" {
		return "", "", false
	}
	idx := strings.Index(s, "\n")
	if idx < 0 {
		return s, "", true
	}
	return s[:idx], strings.TrimLeft(s[idx+1:], "\n"), true
}

// ensureSentenceEnd thêm dấu chấm nếu chuỗi chưa kết bằng dấu kết câu, để Vbee ngắt nhịp.
func ensureSentenceEnd(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	r := []rune(s)
	switch r[len(r)-1] {
	case '.', '!', '?', '…', ':', ';', '”', '"', '»', '’':
		return s
	}
	return s + "."
}

// SplitForTTS chia văn bản thành các phần không vượt maxChars RUNE.
//
// Đếm theo rune chứ không theo byte: dấu tiếng Việt chiếm 2–3 byte nên đếm byte sẽ
// chia sớm khoảng 2,5 lần so với cách Vbee tính.
//
// Việc chia là TẤT ĐỊNH theo văn bản, nên chạy lại với Overwrite=false sinh đúng tên
// phần cũ và resume đúng chỗ.
func SplitForTTS(text string, maxChars int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxChars <= 0 {
		maxChars = DefaultMaxChars
	}
	if runeLen(text) <= maxChars {
		return []string{text}
	}

	var parts []string
	var cur strings.Builder
	curLen := 0

	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			parts = append(parts, s)
		}
		cur.Reset()
		curLen = 0
	}
	// addBlock nối một khối vào phần hiện tại, chốt phần khi vượt ngưỡng.
	addBlock := func(block, sep string) {
		n := runeLen(block)
		sepLen := runeLen(sep)
		if curLen > 0 && curLen+sepLen+n > maxChars {
			flush()
		}
		if curLen > 0 {
			cur.WriteString(sep)
			curLen += sepLen
		}
		cur.WriteString(block)
		curLen += n
	}

	for _, para := range strings.Split(text, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		if runeLen(para) <= maxChars {
			addBlock(para, "\n\n")
			continue
		}
		// Đoạn văn tự nó đã quá dài → xuống cấp tách theo câu.
		for _, sent := range splitSentences(para) {
			if runeLen(sent) <= maxChars {
				addBlock(sent, " ")
				continue
			}
			// Một câu vẫn quá dài → cắt cứng ở ranh giới khoảng trắng.
			for _, chunk := range hardWrap(sent, maxChars) {
				flush()
				parts = append(parts, chunk)
			}
		}
	}
	flush()
	return parts
}

// splitSentences tách một đoạn thành các câu. Ranh giới là [.!?…] kèm dấu đóng
// ngoặc/nháy bám ngay sau, rồi tới khoảng trắng.
//
// BỎ QUA ranh giới nằm trong cặp nháy chưa đóng — thoại tiếng Việt trong repo này dùng
// nháy cong “...” chứa đầy dấu chấm, cắt ở đó sẽ xé câu thoại làm đôi.
func splitSentences(para string) []string {
	runes := []rune(para)
	var (
		sents   []string
		start   int
		inCurly bool // đang trong “ ... ”
		inPlain bool // đang trong " ... "
	)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '“':
			inCurly = true
			continue
		case '”':
			inCurly = false
			continue
		case '"':
			inPlain = !inPlain
			continue
		}
		if !isSentenceEnd(runes[i]) {
			continue
		}
		// Nuốt các dấu đóng bám ngay sau dấu kết câu, ĐỒNG THỜI cập nhật trạng thái
		// nháy trên bản sao. Nhờ vậy dấu chấm cuối lời thoại — “...đây.” — vẫn được
		// nhận là ranh giới câu, dù về mặt kỹ thuật nó nằm trong cặp nháy.
		j := i + 1
		tmpCurly, tmpPlain := inCurly, inPlain
		for j < len(runes) && isClosingMark(runes[j]) {
			switch runes[j] {
			case '”':
				tmpCurly = false
			case '"':
				tmpPlain = !tmpPlain
			}
			j++
		}
		// Còn nằm trong cặp nháy chưa đóng thì đây là dấu chấm GIỮA lời thoại, bỏ qua.
		if tmpCurly || tmpPlain {
			continue
		}
		// Phải có khoảng trắng (hoặc hết chuỗi) thì mới coi là ranh giới thật.
		if j < len(runes) && !unicode.IsSpace(runes[j]) {
			continue
		}
		if s := strings.TrimSpace(string(runes[start:j])); s != "" {
			sents = append(sents, s)
		}
		start = j
		inCurly, inPlain = tmpCurly, tmpPlain
		i = j - 1
	}
	if s := strings.TrimSpace(string(runes[start:])); s != "" {
		sents = append(sents, s)
	}
	return sents
}

func isSentenceEnd(r rune) bool {
	switch r {
	case '.', '!', '?', '…':
		return true
	}
	return false
}

func isClosingMark(r rune) bool {
	switch r {
	case '"', '\'', '”', '’', ')', ']', '»', '.', '!', '?', '…':
		return true
	}
	return false
}

// hardWrap cắt cứng một chuỗi quá dài tại ranh giới khoảng trắng gần nhất TRƯỚC
// maxChars; nếu cả đoạn không có khoảng trắng nào thì cắt đúng maxChars rune.
func hardWrap(s string, maxChars int) []string {
	runes := []rune(s)
	var out []string
	for len(runes) > maxChars {
		cut := maxChars
		for cut > 0 && !unicode.IsSpace(runes[cut]) {
			cut--
		}
		if cut == 0 {
			cut = maxChars
		}
		if chunk := strings.TrimSpace(string(runes[:cut])); chunk != "" {
			out = append(out, chunk)
		}
		runes = runes[cut:]
	}
	if chunk := strings.TrimSpace(string(runes)); chunk != "" {
		out = append(out, chunk)
	}
	return out
}

func runeLen(s string) int { return len([]rune(s)) }

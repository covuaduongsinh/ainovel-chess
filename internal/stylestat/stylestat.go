// Package stylestat thực hiện thống kê phong cách toàn bộ tác phẩm từ nội dung đã viết, chỉ xuất ra số liệu thực tế.
//
// Động lực: Cửa sổ đánh giá trong arc (~10 chương) tự nhiên mù quáng với các mẫu cố định ở cấp độ toàn tác phẩm ——
// lỗi câu tic trung bình vài chục lần/chương, cấu trúc kết chương đồng nhất, lặp lại xuyên chương —— mỗi chương
// xem riêng đều "bình thường", chỉ thống kê toàn tác phẩm mới lộ ra. Thống kê giao cho code
// (xác định, không ảo giác), phán xét giao cho LLM (editor phán theo số, writer tự tránh dựa vào đó).
package stylestat

import (
	"regexp"
	"sort"
	"strings"
)

// minChapters số chương tối thiểu để tạo thống kê — ít hơn thì mẫu quá nhỏ, tần số không có ý nghĩa.
const minChapters = 5

// phraseWindow khai thác cụm từ động chỉ xét N chương gần nhất: writer cần tránh "cửa miệng hiện tại".
const phraseWindow = 20

// Input đầu vào thống kê. Chapters theo thứ tự tăng dần của số chương; Stopwords là danh từ riêng như tên nhân vật,
// bỏ qua khi khai thác cụm từ động (tên nhân vật xuất hiện tự nhiên với tần số cao, không phải vấn đề phong cách).
type Input struct {
	Chapters  []string
	Titles    []string
	Stopwords []string
}

// Stats kết quả thống kê phong cách toàn tác phẩm. Tất cả các trường đều là đếm thực tế, không chứa bất kỳ phán xét hay chỉ thị nào.
type Stats struct {
	Chapters          int            `json:"chapters"`
	Patterns          []PatternStat  `json:"patterns,omitempty"`
	TopPhrases        []PhraseStat   `json:"top_phrases,omitempty"`
	RepeatedSentences []SentenceStat `json:"repeated_sentences,omitempty"`
	Ending            EndingStat     `json:"ending"`
	OpeningTimeRate   float64        `json:"opening_time_rate"`
	TitleFormats      *TitleStat     `json:"title_formats,omitempty"`
}

// PatternStat số đếm toàn tác phẩm của lớp mẫu câu cố định (lỗi tic văn phong AI phổ biến).
type PatternStat struct {
	Name       string  `json:"name"`
	Total      int     `json:"total"`
	PerChapter float64 `json:"per_chapter"`
}

// PhraseStat cụm từ tần số cao được khai thác trong phraseWindow chương gần nhất.
type PhraseStat struct {
	Text  string `json:"text"`
	Count int    `json:"count"`
}

// SentenceStat câu dài lặp lại từng chữ xuyên chương (bằng chứng trực tiếp của lặp lại mô tả).
type SentenceStat struct {
	Text     string `json:"text"`
	Chapters int    `json:"chapters"`
	Count    int    `json:"count"`
}

// EndingStat phân bố hình thái dòng cuối chương. Kết ngắn tự nó hợp lệ, nhưng đồng nhất toàn tác phẩm mới là vấn đề.
type EndingStat struct {
	ShortRatio  float64 `json:"short_ratio"`
	MedianRunes int     `json:"median_runes"`
}

// TitleStat số đếm dùng lẫn tiền tố "Chương N" trong tiêu đề chương (dùng lẫn = dấu vết cơ chế lộ ra trong sản phẩm).
type TitleStat struct {
	WithPrefix    int `json:"with_prefix"`
	WithoutPrefix int `json:"without_prefix"`
}

// patternDefs các mẫu câu phong cách AI phổ biến. Đếm là gần đúng (regex không phân tích cú pháp),
// mục đích là so sánh baseline dọc theo chính tác phẩm, độ chính xác tuyệt đối không quan trọng.
var patternDefs = []struct {
	name string
	re   *regexp.Regexp
}{
	{"Câu chỉnh sửa 'không phải… mà là…'", regexp.MustCompile(`不是[^。！？\n]{1,24}?[，、]?(?:而)?是`)},
	{"Lượng từ thời gian 'X hơi thở/X thoáng'", regexp.MustCompile(`[一两二三四五六七八九十几数半][息瞬]`)},
	{"So sánh 'như một/như thể/tựa như/tựa'", regexp.MustCompile(`像一|仿佛|如同|宛如`)},
	{"Nhịp im lặng 'im lặng/không nói/không quay đầu'", regexp.MustCompile(`沉默了|没有说话|没有回头`)},
}

var (
	sentenceSplit = regexp.MustCompile(`[。！？\n]+`)
	openingTimeRe = regexp.MustCompile(`夜|清晨|黎明|天亮|醒来|晨光|一整夜`)
	titlePrefixRe = regexp.MustCompile(`^#{0,2}\s*第[零〇一二三四五六七八九十百千万\d]+章`)
)

// shortEndingRunes dòng cuối không vượt quá số ký tự này được tính là "kết ngắn".
const shortEndingRunes = 30

// Compute tính thống kê phong cách toàn tác phẩm; trả về nil khi số chương không đủ.
func Compute(in Input) *Stats {
	n := len(in.Chapters)
	if n < minChapters {
		return nil
	}
	all := strings.Join(in.Chapters, "\n")

	s := &Stats{Chapters: n}
	for _, def := range patternDefs {
		total := len(def.re.FindAllStringIndex(all, -1))
		if total == 0 {
			continue
		}
		s.Patterns = append(s.Patterns, PatternStat{
			Name:       def.name,
			Total:      total,
			PerChapter: round1(float64(total) / float64(n)),
		})
	}
	s.TopPhrases = minePhrases(recentWindow(in.Chapters), in.Stopwords)
	s.RepeatedSentences = repeatedSentences(in.Chapters)
	s.Ending = endingShape(in.Chapters)
	s.OpeningTimeRate = openingTimeRate(in.Chapters)
	s.TitleFormats = titleFormats(in.Titles)
	return s
}

func recentWindow(chapters []string) []string {
	if len(chapters) <= phraseWindow {
		return chapters
	}
	return chapters[len(chapters)-phraseWindow:]
}

// minePhrases khai thác cụm từ tần số cao 3-6 ký tự trong cửa sổ.
// Lọc: chứa dấu câu/khoảng trắng, hư từ ở đầu/cuối, trúng danh từ riêng; loại trùng: bỏ những cụm là chuỗi con của cụm đã chọn.
func minePhrases(chapters []string, stopwords []string) []PhraseStat {
	text := strings.Join(chapters, "\n")
	runes := []rune(text)
	threshold := max(8, len(chapters)/2)

	counts := make(map[string]int)
	for size := 3; size <= 6; size++ {
		for i := 0; i+size <= len(runes); i++ {
			gram := runes[i : i+size]
			if !validGram(gram) {
				continue
			}
			counts[string(gram)]++
		}
	}

	stopGrams := stopwordBigrams(stopwords)
	type cand struct {
		text  string
		count int
	}
	var cands []cand
	for g, c := range counts {
		if c < threshold || hitStopword(g, stopGrams) {
			continue
		}
		cands = append(cands, cand{g, c})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].count != cands[j].count {
			return cands[i].count > cands[j].count
		}
		// Cùng tần số ưu tiên cụm dài hơn (thông tin nhiều hơn), sau đó sắp theo thứ tự từ điển để ổn định
		if len(cands[i].text) != len(cands[j].text) {
			return len(cands[i].text) > len(cands[j].text)
		}
		return cands[i].text < cands[j].text
	})

	var out []PhraseStat
	for _, c := range cands {
		if len(out) >= 8 {
			break
		}
		dup := false
		for _, picked := range out {
			if strings.Contains(picked.Text, c.text) || strings.Contains(c.text, picked.Text) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, PhraseStat{Text: c.text, Count: c.count})
		}
	}
	return out
}

// gramEdgeStop các n-gram có hư từ/đại từ này ở đầu hoặc cuối không phải cụm từ phong cách, bỏ qua.
const gramEdgeStop = "的了着是在和与就也都还又把被他她它我你这那"

func validGram(gram []rune) bool {
	for _, r := range gram {
		if r < 0x4E00 || r > 0x9FFF { // chỉ đoạn thuần Hán tự
			return false
		}
	}
	if strings.ContainsRune(gramEdgeStop, gram[0]) || strings.ContainsRune(gramEdgeStop, gram[len(gram)-1]) {
		return false
	}
	return true
}

// stopwordBigrams tách danh từ riêng thành các đoạn 2 ký tự: tên nhân vật thường xuất hiện dạng rút gọn trong văn
// ("九渊" trong "九渊负手"), khớp theo tên đầy đủ sẽ bị sót. Thà lọc nghiêm hơn —— thiếu một sự kiện cụm từ
// không sao, tên nhân vật lọt vào danh sách cửa miệng mới là nhiễu.
func stopwordBigrams(stopwords []string) []string {
	var grams []string
	for _, w := range stopwords {
		runes := []rune(strings.TrimSpace(w))
		if len(runes) < 2 {
			continue
		}
		for i := 0; i+2 <= len(runes); i++ {
			grams = append(grams, string(runes[i:i+2]))
		}
	}
	return grams
}

func hitStopword(gram string, stopGrams []string) bool {
	for _, g := range stopGrams {
		if strings.Contains(gram, g) {
			return true
		}
	}
	return false
}

// repeatedSentences tìm các câu ≥12 ký tự lặp lại từng chữ xuyên ≥3 chương, lấy top 5 theo số lần.
func repeatedSentences(chapters []string) []SentenceStat {
	type rec struct {
		count    int
		chapters map[int]struct{}
	}
	seen := make(map[string]*rec)
	for ci, text := range chapters {
		for _, sent := range sentenceSplit.Split(text, -1) {
			// Bóc dấu ngoặc bao quanh rồi gộp: cùng một câu thoại có/không có dấu ngoặc mở không nên tính là hai câu khác
			sent = strings.Trim(strings.TrimSpace(sent), `"""‘’「」『』`)
			if len([]rune(sent)) < 12 {
				continue
			}
			r := seen[sent]
			if r == nil {
				r = &rec{chapters: make(map[int]struct{})}
				seen[sent] = r
			}
			r.count++
			r.chapters[ci] = struct{}{}
		}
	}

	var out []SentenceStat
	for sent, r := range seen {
		if len(r.chapters) < 3 {
			continue
		}
		out = append(out, SentenceStat{Text: truncateRunes(sent, 40), Chapters: len(r.chapters), Count: r.count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Text < out[j].Text
	})
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

func endingShape(chapters []string) EndingStat {
	var lengths []int
	short := 0
	for _, text := range chapters {
		line := lastNonEmptyLine(text)
		if line == "" {
			continue
		}
		n := len([]rune(line))
		lengths = append(lengths, n)
		if n <= shortEndingRunes {
			short++
		}
	}
	if len(lengths) == 0 {
		return EndingStat{}
	}
	sort.Ints(lengths)
	return EndingStat{
		ShortRatio:  round2(float64(short) / float64(len(lengths))),
		MedianRunes: lengths[len(lengths)/2],
	}
}

func openingTimeRate(chapters []string) float64 {
	hit := 0
	for _, text := range chapters {
		if openingTimeRe.MatchString(firstParagraph(text)) {
			hit++
		}
	}
	return round2(float64(hit) / float64(len(chapters)))
}

func titleFormats(titles []string) *TitleStat {
	if len(titles) == 0 {
		return nil
	}
	t := &TitleStat{}
	for _, title := range titles {
		if strings.TrimSpace(title) == "" {
			continue
		}
		if titlePrefixRe.MatchString(title) {
			t.WithPrefix++
		} else {
			t.WithoutPrefix++
		}
	}
	// Chỉ khi dùng lẫn mới đáng báo cáo; định dạng thống nhất không phải vấn đề theo nghĩa thực tế
	if t.WithPrefix == 0 || t.WithoutPrefix == 0 {
		return nil
	}
	return t
}

func lastNonEmptyLine(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

// firstParagraph lấy dòng đầu tiên không rỗng và không phải tiêu đề Markdown (dòng đầu file chương thường là tiêu đề #).
func firstParagraph(text string) string {
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line
	}
	return ""
}

func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }
func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }

package rules

import (
	"fmt"
	"maps"
	"strings"
)

// Snapshot là ảnh chụp quy tắc người dùng sau khi chuẩn hóa của sách này (meta/user_rules.json).
//
// Đây là nguồn sự thật duy nhất lúc chạy: được tạo bằng cách chuẩn hóa và hợp nhất từ các nguồn khi mở/nhập/làm mới sách,
// sau đó novel_context tiêm vào và commit_chapter kiểm tra đều chỉ đọc bản này, không đọc lại tệp rules nhiều lần (tránh trôi dạt và phân kỳ giữa hai đầu đọc).
//
// Chỉ có Structured + Preferences được tiêm vào mô hình (xem Payload); Version / Status / Sources /
// Uncertain là metadata vận hành và chẩn đoán, không đưa vào working_memory.user_rules.
type Snapshot struct {
	Version     int        `json:"version"`
	Status      Status     `json:"status"`
	Structured  Structured `json:"structured"`
	Preferences string     `json:"preferences"`
	Sources     []string   `json:"sources"`
	Uncertain   []string   `json:"uncertain"`
}

// Status đánh dấu xem chuẩn hóa ảnh chụp có hoàn toàn thành công không.
type Status string

const (
	// StatusReady tất cả nguồn đều chuẩn hóa thành công.
	StatusReady Status = "ready"
	// StatusDegraded ít nhất một nguồn chuẩn hóa thất bại, đã giáng cấp thành raw preferences (xem chi tiết ở Uncertain / log).
	StatusDegraded Status = "degraded"
)

// SnapshotVersion là phiên bản schema ảnh chụp hiện tại, tiện cho di chuyển trong tương lai.
const SnapshotVersion = 1

// Candidate là kết quả ứng viên sau khi chuẩn hóa một nguồn đơn lẻ.
//
// Các nguồn được sắp xếp theo thứ tự ưu tiên thấp→cao rồi chuyển cho BuildSnapshot hợp nhất có tính xác định.
// LLM chỉ chịu trách nhiệm chuyển ngôn ngữ tự nhiên của một nguồn duy nhất thành Structured/Preferences ứng viên;
// ưu tiên và ghi đè trường do BuildSnapshot (Go) phán quyết.
type Candidate struct {
	Source      string     // nhãn nguồn có thể đọc được, đưa vào Snapshot.Sources (ví dụ system_defaults / startup_prompt / global:my.md)
	Structured  Structured // trường có cấu trúc ứng viên của nguồn này
	Preferences string     // nội dung sở thích ngôn ngữ tự nhiên của nguồn này
	Uncertain   []string   // các mục nguồn này cố ý không đề xuất vào structured + lý do (chẩn đoán)
	Degraded    bool       // nguồn này chuẩn hóa thất bại, đã giáng cấp thành raw preferences
}

// Payload trả về dạng được tiêm vào working_memory.user_rules: chỉ lộ structured + preferences.
// Dù cả hai đều rỗng cũng trả về cấu trúc ổn định, tránh LLM thấy user_rules=null đi vào nhánh bất thường.
func (s Snapshot) Payload() map[string]any {
	return map[string]any{
		"structured":  s.Structured,
		"preferences": s.Preferences,
	}
}

// BuildSnapshot hợp nhất có tính xác định các ứng viên đã được sắp xếp theo ưu tiên (thấp→cao) thành ảnh chụp.
//
// Quy tắc hợp nhất (tất cả xác định ở phía Go, không giao cho LLM):
//   - structured: ghi đè theo trường, nguồn ưu tiên cao ghi đè nguồn ưu tiên thấp; fatigue_words được cộng dồn theo từ
//   - preferences: không ghi đè, nối theo thứ tự nguồn (ưu tiên cao ở sau), kèm tiêu đề nguồn
//   - giá trị rỗng/bằng không xem là trường thiếu, không ghi đè giá trị hiện có (sanitizeStructured)
//   - bất kỳ nguồn nào Degraded → ảnh chụp status=degraded
func BuildSnapshot(cands []Candidate) Snapshot {
	snap := Snapshot{
		Version: SnapshotVersion,
		Status:  StatusReady,
		Sources: make([]string, 0, len(cands)),
	}
	var prefs []string
	for _, c := range cands {
		s := sanitizeStructured(c.Structured)
		if s.Genre != "" {
			snap.Structured.Genre = s.Genre
		}
		if s.ChapterWords != nil {
			snap.Structured.ChapterWords = s.ChapterWords
		}
		if len(s.ForbiddenChars) > 0 {
			snap.Structured.ForbiddenChars = s.ForbiddenChars
		}
		if len(s.ForbiddenPhrases) > 0 {
			snap.Structured.ForbiddenPhrases = s.ForbiddenPhrases
		}
		if len(s.FatigueWords) > 0 {
			snap.Structured.FatigueWords = mergeFatigueWords(snap.Structured.FatigueWords, s.FatigueWords)
		}

		if p := strings.TrimSpace(c.Preferences); p != "" {
			if src := strings.TrimSpace(c.Source); src != "" {
				prefs = append(prefs, fmt.Sprintf("## [%s]\n\n%s", src, p))
			} else {
				prefs = append(prefs, p)
			}
		}
		if src := strings.TrimSpace(c.Source); src != "" {
			snap.Sources = append(snap.Sources, src)
		}
		snap.Uncertain = append(snap.Uncertain, c.Uncertain...)
		if c.Degraded {
			snap.Status = StatusDegraded
		}
	}
	snap.Preferences = strings.Join(prefs, "\n\n")
	return snap
}

// OverlaySnapshot chồng một ứng viên ưu tiên cao lên ảnh chụp hiện có (ứng viên thắng).
//
// Dùng cho save_user_rules lúc chạy: không chuẩn hóa lại tất cả nguồn, chỉ ghi đè quy tắc mới vào ảnh chụp hiện tại —
// structured ghi đè theo trường, preferences nối thêm một đoạn, sources/uncertain tích lũy, giáng cấp lan truyền.
func OverlaySnapshot(base Snapshot, cand Candidate) Snapshot {
	out := base
	out.Version = SnapshotVersion
	s := sanitizeStructured(cand.Structured)
	if s.Genre != "" {
		out.Structured.Genre = s.Genre
	}
	if s.ChapterWords != nil {
		out.Structured.ChapterWords = s.ChapterWords
	}
	if len(s.ForbiddenChars) > 0 {
		out.Structured.ForbiddenChars = s.ForbiddenChars
	}
	if len(s.ForbiddenPhrases) > 0 {
		out.Structured.ForbiddenPhrases = s.ForbiddenPhrases
	}
	if len(s.FatigueWords) > 0 {
		out.Structured.FatigueWords = mergeFatigueWords(cloneFatigue(out.Structured.FatigueWords), s.FatigueWords)
	}
	if p := strings.TrimSpace(cand.Preferences); p != "" {
		section := p
		if src := strings.TrimSpace(cand.Source); src != "" {
			section = fmt.Sprintf("## [%s]\n\n%s", src, p)
		}
		if strings.TrimSpace(out.Preferences) == "" {
			out.Preferences = section
		} else {
			out.Preferences = out.Preferences + "\n\n" + section
		}
	}
	if src := strings.TrimSpace(cand.Source); src != "" {
		out.Sources = append(append([]string{}, out.Sources...), src)
	}
	if len(cand.Uncertain) > 0 {
		out.Uncertain = append(append([]string{}, out.Uncertain...), cand.Uncertain...)
	}
	if cand.Degraded {
		out.Status = StatusDegraded
	}
	return out
}

// mergeFatigueWords cộng dồn ngưỡng từ mòn theo từng từ, src ghi đè ngưỡng cùng từ trong dst (ưu tiên gần nhất).
// Cho phép người dùng chỉ cần thêm ít từ mòn mà không phải liệt kê lại đường đáy tích hợp sẵn.
func mergeFatigueWords(dst, src map[string]int) map[string]int {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]int, len(src))
	}
	maps.Copy(dst, src)
	return dst
}

func cloneFatigue(m map[string]int) map[string]int {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]int, len(m))
	maps.Copy(out, m)
	return out
}

// SystemDefaults là baseline cơ học tích hợp trong code (nguồn ưu tiên thấp nhất), không qua chuẩn hóa LLM.
//
// Số liệu di từ front matter của assets/rules/default.md cũ. Bản tiếng Việt của ForbiddenPhrases/FatigueWords
// dưới đây là BẢN NHÁP SƠ BỘ thay cho danh sách tiếng Trung gốc (vốn dò câu sáo/từ đệm tiếng Trung như
// "không khỏi"/"một tia"/"im lặng hồi lâu", không khớp văn tiếng Việt). Trọng số mang tính ước lệ; CẦN DUYỆT và tinh chỉnh bằng
// ngữ liệu tiếng Việt thật (giống cách danh sách gốc được chắt từ một lần chạy dài 196 chương).
func SystemDefaults() Candidate {
	return Candidate{
		Source: "system_defaults",
		Structured: Structured{
			ChapterWords: &WordRange{Min: 3000, Max: 6000},
			// Câu sáo AI là chuỗi cố định; checker khớp chuỗi con theo mặt chữ, các mẫu có biến (không phải X mà là Y) quy về tầng ngữ nghĩa.
			ForbiddenPhrases: []string{"ở một mức độ nào đó", "đáng chú ý là", "không hiểu vì sao", "lẫn lộn đủ vị"},
			FatigueWords: map[string]int{
				"không khỏi": 1, "bỗng nhiên": 1, "tựa như": 2, "phảng phất": 2, "dường như": 1,
				"một tia": 2, "một thoáng": 2, "một làn": 2, "bất giác": 1, "như một": 3,
				"im lặng hồi lâu": 2, "không nói gì": 2,
			},
		},
	}
}

// sanitizeStructured thực thi "giá trị rỗng/bằng không = trường thiếu": bộ chuẩn hóa có thể thải ra genre:"", chapter_words.min:0
// dạng placeholder như vậy (thực chứng từ prototype), phải xem như chưa khai báo, tránh làm ô nhiễm quá trình hợp nhất và kiểm tra cơ học.
func sanitizeStructured(s Structured) Structured {
	out := Structured{}
	if g := strings.TrimSpace(s.Genre); g != "" {
		out.Genre = g
	}
	out.ChapterWords = sanitizeWordRange(s.ChapterWords)
	out.ForbiddenChars = nonEmptyStrings(s.ForbiddenChars)
	out.ForbiddenPhrases = nonEmptyStrings(s.ForbiddenPhrases)
	out.FatigueWords = sanitizeFatigueWords(s.FatigueWords)
	return out
}

// sanitizeWordRange xử lý giá trị bằng không và khoảng không hợp lệ: min/max đều bằng 0 nghĩa là không ràng buộc (loại bỏ);
// một bên bằng 0 là hợp lệ (checker xem 0 là "không giới hạn phía đó"); min>max>0 không hợp lệ, loại bỏ toàn bộ đoạn.
func sanitizeWordRange(r *WordRange) *WordRange {
	if r == nil {
		return nil
	}
	min, max := r.Min, r.Max
	if min < 0 {
		min = 0
	}
	if max < 0 {
		max = 0
	}
	if min == 0 && max == 0 {
		return nil
	}
	if max > 0 && min > max {
		return nil
	}
	return &WordRange{Min: min, Max: max}
}

func nonEmptyStrings(in []string) []string {
	var out []string
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func sanitizeFatigueWords(m map[string]int) map[string]int {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]int, len(m))
	for w, n := range m {
		if w = strings.TrimSpace(w); w == "" || n <= 0 {
			continue
		}
		out[w] = n
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

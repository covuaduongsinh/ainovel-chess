package rules

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Check kiểm tra cơ học nội dung chương theo quy tắc có cấu trúc, trả về danh sách vi phạm thực tế.
//
// Hợp đồng thiết kế:
//   - Chỉ trả về sự thật, không đưa ra lệnh (nguyên tắc sắt một)
//   - Không chặn luồng của bất kỳ bên gọi nào
//   - severity được ánh xạ cố định theo loại quy tắc (xem bảng chú thích trong types.go)
//
// Tham số:
//   - text: nội dung chương (bản nháp hoặc bản cuối đều được)
//   - wordCount: số chữ của chương (đếm theo rune). Khi <0, checker tự tính để tránh bên gọi quét O(n) lặp lại.
//   - s: quy tắc có cấu trúc sau khi hợp nhất; khi IsEmpty thì trả về nil ngay.
func Check(text string, wordCount int, s Structured) []Violation {
	if s.IsEmpty() {
		return nil
	}
	if wordCount < 0 {
		wordCount = utf8.RuneCountInString(text)
	}

	var violations []Violation
	violations = appendForbiddenChars(violations, text, s.ForbiddenChars)
	violations = appendForbiddenPhrases(violations, text, s.ForbiddenPhrases)
	violations = appendFatigueWords(violations, text, s.FatigueWords)
	violations = appendChapterWords(violations, wordCount, s.ChapterWords)
	return violations
}

// forbidden_chars: xuất hiện ≥1 lần là error.
// Cùng một quy tắc chỉ tạo ra một violation, actual là số lần xuất hiện.
func appendForbiddenChars(vs []Violation, text string, list []string) []Violation {
	for _, ch := range list {
		if ch == "" {
			continue
		}
		n := strings.Count(text, ch)
		if n == 0 {
			continue
		}
		vs = append(vs, Violation{
			Rule:     "forbidden_chars",
			Target:   ch,
			Actual:   n,
			Severity: SeverityError,
		})
	}
	return vs
}

// forbidden_phrases: xuất hiện ≥1 lần là error; hành vi giống forbidden_chars, chỉ khác tên rule.
func appendForbiddenPhrases(vs []Violation, text string, list []string) []Violation {
	for _, ph := range list {
		if ph == "" {
			continue
		}
		n := strings.Count(text, ph)
		if n == 0 {
			continue
		}
		vs = append(vs, Violation{
			Rule:     "forbidden_phrases",
			Target:   ph,
			Actual:   n,
			Severity: SeverityError,
		})
	}
	return vs
}

// fatigue_words: vi phạm khi số lần xuất hiện trong chương vượt ngưỡng, mức warning.
// Không tích lũy qua các chương — vấn đề xuyên chương để chẩn đoán sau.
func appendFatigueWords(vs []Violation, text string, m map[string]int) []Violation {
	for word, limit := range m {
		if word == "" || limit <= 0 {
			continue
		}
		n := strings.Count(text, word)
		if n <= limit {
			continue
		}
		vs = append(vs, Violation{
			Rule:     "fatigue_words",
			Target:   word,
			Limit:    limit,
			Actual:   n,
			Severity: SeverityWarning,
		})
	}
	return vs
}

// chapter_words: độ lệch số chữ.
// Độ lệch < 20%: warning; độ lệch ≥ 20%: error.
// Công thức độ lệch: khi dưới min dùng (min-actual)/min; khi trên max dùng (actual-max)/max.
func appendChapterWords(vs []Violation, wordCount int, rng *WordRange) []Violation {
	if rng == nil {
		return vs
	}
	var deviation float64
	switch {
	case wordCount < rng.Min:
		if rng.Min == 0 {
			return vs
		}
		deviation = float64(rng.Min-wordCount) / float64(rng.Min)
	case wordCount > rng.Max:
		if rng.Max == 0 {
			return vs
		}
		deviation = float64(wordCount-rng.Max) / float64(rng.Max)
	default:
		return vs // trong phạm vi
	}

	severity := SeverityWarning
	if deviation >= ChapterWordsDeviationThreshold {
		severity = SeverityError
	}
	vs = append(vs, Violation{
		Rule:      "chapter_words",
		Limit:     fmt.Sprintf("%d-%d", rng.Min, rng.Max),
		Actual:    wordCount,
		Deviation: deviation,
		Severity:  severity,
	})
	return vs
}

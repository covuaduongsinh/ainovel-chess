package rules

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// findViolation tìm vi phạm đầu tiên trong kết quả theo rule + target.
func findViolation(vs []Violation, rule, target string) *Violation {
	for i := range vs {
		if vs[i].Rule == rule && vs[i].Target == target {
			return &vs[i]
		}
	}
	return nil
}

func TestCheck_EmptyStructured(t *testing.T) {
	vs := Check("nội dung bất kỳ", -1, Structured{})
	if vs != nil {
		t.Errorf("empty structured should return nil, got %+v", vs)
	}
}

func TestCheck_ForbiddenChars(t *testing.T) {
	text := "Anh cười——rồi thở dài——rồi rời đi."
	vs := Check(text, -1, Structured{
		ForbiddenChars: []string{"——"},
	})
	v := findViolation(vs, "forbidden_chars", "——")
	if v == nil {
		t.Fatal("expected forbidden_chars violation")
	}
	if v.Severity != SeverityError {
		t.Errorf("severity=%s, want error", v.Severity)
	}
	if v.Actual != 2 {
		t.Errorf("actual=%v, want 2", v.Actual)
	}
}

func TestCheck_ForbiddenCharsNotPresent(t *testing.T) {
	vs := Check("văn bản thường không vi phạm", -1, Structured{
		ForbiddenChars: []string{"——"},
	})
	if len(vs) != 0 {
		t.Errorf("expected no violations, got %+v", vs)
	}
}

func TestCheck_ForbiddenPhrases(t *testing.T) {
	text := "không phải……mà là sự thật bị che giấu. Đây là thăm dò động lực cốt lõi."
	vs := Check(text, -1, Structured{
		ForbiddenPhrases: []string{"không phải……mà là", "động lực cốt lõi"},
	})
	if len(vs) != 2 {
		t.Errorf("expected 2 violations, got %d: %+v", len(vs), vs)
	}
	for _, v := range vs {
		if v.Severity != SeverityError {
			t.Errorf("severity=%s, want error", v.Severity)
		}
	}
}

func TestCheck_FatigueWordsUnderLimit(t *testing.T) {
	text := "Anh ta không khỏi bật cười."
	vs := Check(text, -1, Structured{
		FatigueWords: map[string]int{"không khỏi": 1},
	})
	if len(vs) != 0 {
		t.Errorf("under limit should not violate, got %+v", vs)
	}
}

func TestCheck_FatigueWordsAtLimit(t *testing.T) {
	// limit=1, actual=1 → không vi phạm
	text := "Anh ta không khỏi bật cười."
	vs := Check(text, -1, Structured{
		FatigueWords: map[string]int{"không khỏi": 1},
	})
	if len(vs) != 0 {
		t.Errorf("at limit should not violate (limit 1 actual 1), got %+v", vs)
	}
}

func TestCheck_FatigueWordsOverLimit(t *testing.T) {
	// limit=1, actual=3 → warning
	text := "Anh ta không khỏi cười, rồi không khỏi nhăn mày, cuối cùng không khỏi rời đi."
	vs := Check(text, -1, Structured{
		FatigueWords: map[string]int{"không khỏi": 1},
	})
	v := findViolation(vs, "fatigue_words", "không khỏi")
	if v == nil {
		t.Fatal("expected fatigue_words violation")
	}
	if v.Severity != SeverityWarning {
		t.Errorf("severity=%s, want warning", v.Severity)
	}
	if v.Limit != 1 {
		t.Errorf("limit=%v, want 1", v.Limit)
	}
	if v.Actual != 3 {
		t.Errorf("actual=%v, want 3", v.Actual)
	}
}

// Kiểm thử biên số chữ
// Phạm vi 3000-6000:
//   actual 3000 → trong phạm vi → no violation
//   actual 2999 → deviation ≈ 0.033% → warning
//   actual 2401 → deviation = 599/3000 ≈ 19.97% → warning
//   actual 2400 → deviation = 600/3000 = 20% → error (>= threshold)
//   actual 6001 → deviation ≈ 0.017% → warning
//   actual 7199 → deviation ≈ 19.98% → warning
//   actual 7200 → deviation = 1200/6000 = 20% → error

func TestCheck_ChapterWordsInRange(t *testing.T) {
	rng := &WordRange{Min: 3000, Max: 6000}
	vs := Check("", 4000, Structured{ChapterWords: rng})
	if len(vs) != 0 {
		t.Errorf("in range should yield no violation, got %+v", vs)
	}
	// giá trị biên
	vs = Check("", 3000, Structured{ChapterWords: rng})
	if len(vs) != 0 {
		t.Errorf("at min should be in range, got %+v", vs)
	}
	vs = Check("", 6000, Structured{ChapterWords: rng})
	if len(vs) != 0 {
		t.Errorf("at max should be in range, got %+v", vs)
	}
}

func TestCheck_ChapterWordsSlightlyBelow(t *testing.T) {
	// actual 2401 → deviation = 599/3000 = 0.1996... < 20% → warning
	rng := &WordRange{Min: 3000, Max: 6000}
	vs := Check("", 2401, Structured{ChapterWords: rng})
	if len(vs) != 1 || vs[0].Rule != "chapter_words" {
		t.Fatalf("expected 1 chapter_words violation, got %+v", vs)
	}
	if vs[0].Severity != SeverityWarning {
		t.Errorf("severity=%s, want warning at <20%%", vs[0].Severity)
	}
	if vs[0].Deviation >= ChapterWordsDeviationThreshold {
		t.Errorf("deviation=%f should be < %f", vs[0].Deviation, ChapterWordsDeviationThreshold)
	}
}

func TestCheck_ChapterWordsAtThreshold(t *testing.T) {
	// actual 2400 → deviation = 600/3000 = 0.2 == 20% → error (>= threshold)
	rng := &WordRange{Min: 3000, Max: 6000}
	vs := Check("", 2400, Structured{ChapterWords: rng})
	if len(vs) != 1 || vs[0].Severity != SeverityError {
		t.Errorf("expected error at 20%% threshold, got %+v", vs)
	}
}

func TestCheck_ChapterWordsAboveMax(t *testing.T) {
	// actual 7200 → deviation = 1200/6000 = 0.2 == 20% → error
	rng := &WordRange{Min: 3000, Max: 6000}
	vs := Check("", 7200, Structured{ChapterWords: rng})
	if len(vs) != 1 || vs[0].Severity != SeverityError {
		t.Errorf("expected error at 20%% above max, got %+v", vs)
	}
	if vs[0].Actual != 7200 {
		t.Errorf("actual=%v, want 7200", vs[0].Actual)
	}
}

func TestCheck_ChapterWordsSlightlyAbove(t *testing.T) {
	// actual 7199 → deviation = 1199/6000 ≈ 0.19983 < 20% → warning
	rng := &WordRange{Min: 3000, Max: 6000}
	vs := Check("", 7199, Structured{ChapterWords: rng})
	if len(vs) != 1 || vs[0].Severity != SeverityWarning {
		t.Errorf("expected warning slightly above max, got %+v", vs)
	}
}

func TestCheck_AutoWordCount(t *testing.T) {
	// khi wordCount = -1, checker tự tính
	text := strings.Repeat("a", 2500) // 2500 ký tự (auto-đếm)
	rng := &WordRange{Min: 3000, Max: 6000}
	vs := Check(text, -1, Structured{ChapterWords: rng})
	if len(vs) != 1 || vs[0].Rule != "chapter_words" {
		t.Fatalf("expected 1 chapter_words violation, got %+v", vs)
	}
	if vs[0].Actual != 2500 {
		t.Errorf("auto wordCount=%v, want 2500", vs[0].Actual)
	}
	if vs[0].Actual != utf8.RuneCountInString(text) {
		t.Errorf("auto count mismatch: %v vs rune count %d", vs[0].Actual, utf8.RuneCountInString(text))
	}
}

func TestCheck_MultipleRulesAtOnce(t *testing.T) {
	text := "không khỏi——rồi không khỏi——rồi rời đi."
	rng := &WordRange{Min: 3000, Max: 6000}
	s := Structured{
		ChapterWords:   rng,
		ForbiddenChars: []string{"——"},
		FatigueWords:   map[string]int{"không khỏi": 1},
	}
	vs := Check(text, 10, s)

	// nên kích hoạt cả ba loại cùng lúc: forbidden_chars + fatigue_words + chapter_words
	rules := map[string]bool{}
	for _, v := range vs {
		rules[v.Rule] = true
	}
	if !rules["forbidden_chars"] || !rules["fatigue_words"] || !rules["chapter_words"] {
		t.Errorf("expected all three rules triggered, got %+v", rules)
	}
}

func TestCheck_FatigueZeroLimitSkipped(t *testing.T) {
	// limit=0 là giá trị không hợp lệ, nên bỏ qua toàn bộ quy tắc (parser cũng lọc, đây là phòng thủ)
	text := "không khỏi không khỏi không khỏi"
	vs := Check(text, -1, Structured{
		FatigueWords: map[string]int{"không khỏi": 0},
	})
	if len(vs) != 0 {
		t.Errorf("limit=0 should be skipped, got %+v", vs)
	}
}

func TestCheck_EmptyTargetsSkipped(t *testing.T) {
	// mục tiêu chuỗi rỗng không nên gây ra false positive
	vs := Check("văn bản bất kỳ", -1, Structured{
		ForbiddenChars:   []string{""},
		ForbiddenPhrases: []string{""},
		FatigueWords:     map[string]int{"": 1},
	})
	if len(vs) != 0 {
		t.Errorf("empty targets should be skipped, got %+v", vs)
	}
}

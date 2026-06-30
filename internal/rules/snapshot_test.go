package rules

import (
	"strings"
	"testing"
)

func TestBuildSnapshot_FieldOverridePrecedence(t *testing.T) {
	// thấp→cao: defaults đặt 3000-6000, project ghi đè thành 1200-1600; ưu tiên cao thắng.
	snap := BuildSnapshot([]Candidate{
		{Source: "system_defaults", Structured: Structured{ChapterWords: &WordRange{Min: 3000, Max: 6000}}},
		{Source: "project:a.md", Structured: Structured{ChapterWords: &WordRange{Min: 1200, Max: 1600}}},
	})
	if snap.Structured.ChapterWords == nil || snap.Structured.ChapterWords.Min != 1200 || snap.Structured.ChapterWords.Max != 1600 {
		t.Fatalf("kỳ vọng project ghi đè defaults, nhận %+v", snap.Structured.ChapterWords)
	}
	if snap.Status != StatusReady {
		t.Fatalf("kỳ vọng ready, nhận %s", snap.Status)
	}
	if snap.Version != SnapshotVersion {
		t.Fatalf("version phải là %d, nhận %d", SnapshotVersion, snap.Version)
	}
}

func TestBuildSnapshot_EmptyAndZeroAreAbsent(t *testing.T) {
	// Bộ chuẩn hóa nhả placeholder: genre:"", chapter_words{0,0}, phần tử chuỗi rỗng — đều phải coi là thiếu, không ghi đè giá trị thật ưu tiên thấp.
	snap := BuildSnapshot([]Candidate{
		{Source: "system_defaults", Structured: Structured{
			Genre:        "tu tiên",
			ChapterWords: &WordRange{Min: 3000, Max: 6000},
		}},
		{Source: "startup_prompt", Structured: Structured{
			Genre:            "",                        // placeholder chuỗi rỗng → không ghi đè
			ChapterWords:     &WordRange{Min: 0, Max: 0}, // giá trị 0 → không ghi đè
			ForbiddenPhrases: []string{"", "  "},         // toàn rỗng → loại bỏ
		}},
	})
	if snap.Structured.Genre != "tu tiên" {
		t.Fatalf("genre rỗng không nên ghi đè, kỳ vọng tu tiên, nhận %q", snap.Structured.Genre)
	}
	if snap.Structured.ChapterWords == nil || snap.Structured.ChapterWords.Min != 3000 {
		t.Fatalf("chapter_words giá trị 0 không nên ghi đè, nhận %+v", snap.Structured.ChapterWords)
	}
	if len(snap.Structured.ForbiddenPhrases) != 0 {
		t.Fatalf("forbidden_phrases toàn rỗng phải bị loại bỏ, nhận %v", snap.Structured.ForbiddenPhrases)
	}
}

func TestBuildSnapshot_UpperBoundOnly(t *testing.T) {
	// "mỗi chương đừng quá 2500 chữ" → {min:0, max:2500}, min:0 hợp lệ nghĩa là không có cận dưới.
	snap := BuildSnapshot([]Candidate{
		{Source: "startup_prompt", Structured: Structured{ChapterWords: &WordRange{Min: 0, Max: 2500}}},
	})
	if snap.Structured.ChapterWords == nil || snap.Structured.ChapterWords.Max != 2500 {
		t.Fatalf("chỉ-cận-trên phải được giữ, nhận %+v", snap.Structured.ChapterWords)
	}
}

func TestBuildSnapshot_InvalidRangeDropped(t *testing.T) {
	snap := BuildSnapshot([]Candidate{
		{Source: "x", Structured: Structured{ChapterWords: &WordRange{Min: 5000, Max: 1000}}},
	})
	if snap.Structured.ChapterWords != nil {
		t.Fatalf("khoảng min>max bất hợp lệ phải bị loại bỏ, nhận %+v", snap.Structured.ChapterWords)
	}
}

func TestBuildSnapshot_PreferencesPrecedenceOrder(t *testing.T) {
	snap := BuildSnapshot([]Candidate{
		{Source: "global:g.md", Preferences: "sở thích toàn cục"},
		{Source: "project:p.md", Preferences: "sở thích dự án"},
	})
	gi := strings.Index(snap.Preferences, "sở thích toàn cục")
	pi := strings.Index(snap.Preferences, "sở thích dự án")
	if gi < 0 || pi < 0 || gi > pi {
		t.Fatalf("preferences phải ghép theo ưu tiên thấp→cao (dự án ở sau), nhận:\n%s", snap.Preferences)
	}
	if !strings.Contains(snap.Preferences, "## [global:g.md]") {
		t.Fatalf("preferences phải kèm tiêu đề nguồn, nhận:\n%s", snap.Preferences)
	}
}

func TestBuildSnapshot_FatigueWordsMergeByWord(t *testing.T) {
	snap := BuildSnapshot([]Candidate{
		{Source: "system_defaults", Structured: Structured{FatigueWords: map[string]int{"bỗng nhiên": 1, "tựa như": 2}}},
		{Source: "project:p.md", Structured: Structured{FatigueWords: map[string]int{"tựa như": 5}}},
	})
	if snap.Structured.FatigueWords["bỗng nhiên"] != 1 {
		t.Fatalf("bỗng nhiên phải giữ ngưỡng defaults 1, nhận %d", snap.Structured.FatigueWords["bỗng nhiên"])
	}
	if snap.Structured.FatigueWords["tựa như"] != 5 {
		t.Fatalf("tựa như phải bị project ghi đè thành 5, nhận %d", snap.Structured.FatigueWords["tựa như"])
	}
}

func TestBuildSnapshot_DegradedPropagates(t *testing.T) {
	snap := BuildSnapshot([]Candidate{
		{Source: "system_defaults", Structured: Structured{ChapterWords: &WordRange{Min: 3000, Max: 6000}}},
		{Source: "project:bad.md", Preferences: "văn gốc giáng cấp", Degraded: true},
	})
	if snap.Status != StatusDegraded {
		t.Fatalf("bất kỳ nguồn nào giáng cấp thì status=degraded, nhận %s", snap.Status)
	}
	// Nguồn giáng cấp vẫn vào dưới dạng raw preferences, không chặn; structured của các nguồn khác như thường.
	if snap.Structured.ChapterWords == nil {
		t.Fatalf("giáng cấp không nên ảnh hưởng structured của nguồn khác")
	}
	if !strings.Contains(snap.Preferences, "văn gốc giáng cấp") {
		t.Fatalf("nguồn giáng cấp phải được giữ làm raw preferences")
	}
}

func TestSystemDefaults_MatchesLegacyDefaultMD(t *testing.T) {
	d := SystemDefaults().Structured
	if d.ChapterWords == nil || d.ChapterWords.Min != 3000 || d.ChapterWords.Max != 6000 {
		t.Fatalf("số chữ mặc định phải là 3000-6000, nhận %+v", d.ChapterWords)
	}
	if len(d.ForbiddenPhrases) != 4 {
		t.Fatalf("cụm cấm mặc định phải có 4 mục, nhận %d", len(d.ForbiddenPhrases))
	}
	if len(d.FatigueWords) != 12 {
		t.Fatalf("từ lặp mòn mặc định phải có 12 mục, nhận %d", len(d.FatigueWords))
	}
}

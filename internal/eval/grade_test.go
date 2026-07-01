package eval

import (
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/diag"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
	"github.com/voocel/ainovel-cli/internal/stylestat"
)

// writerSmokeCase là một smoke case chương đầu tiên writer điển hình, dùng để kiểm thử cổng kiểm soát.
func writerSmokeCase() Case {
	c := Case{
		ID:          "writer_first_chapter",
		Category:    "smoke",
		Role:        "writer",
		Prompt:      "viết một tiểu thuyết tiên hiệp",
		MaxChapters: 1,
		Expect: Expect{
			Phase:                "writing",
			MinCompletedChapters: 1,
			RequiredCheckpoints:  []string{"chapter:1:plan", "chapter:1:draft", "chapter:1:commit"},
			NoPending:            []string{"pending_commit", "pending_steer"},
		},
	}
	_ = c.Validate() // điền max_severity mặc định
	return c
}

// cleanCollected tạo ra kết quả thu thập "một chương hoàn thành bình thường" (không có findings, không có tín hiệu còn sót, hợp đồng đầy đủ).
func cleanCollected() Collected {
	return Collected{
		Dir:      "/fake",
		Report:   diag.Report{Stats: diag.Stats{CompletedChapters: 1, TotalChapters: 1, Phase: "writing", Flow: "writing"}},
		Progress: &domain.Progress{Phase: domain.PhaseWriting, CompletedChapters: []int{1}},
		Checkpoints: []domain.Checkpoint{
			{Scope: domain.ChapterScope(1), Step: "plan"},
			{Scope: domain.ChapterScope(1), Step: "draft"},
			{Scope: domain.ChapterScope(1), Step: "commit"},
		},
		Pending: map[string]bool{},
	}
}

func TestGradePassesCleanRun(t *testing.T) {
	r := Grade(writerSmokeCase(), cleanCollected())
	if r.Outcome != Pass {
		t.Fatalf("kỳ vọng PASS, nhận được %s; hard fails=%v", r.Outcome, r.HardFails)
	}
	if len(r.Passed) == 0 {
		t.Fatal("kỳ vọng có bản ghi hợp đồng đã qua")
	}
}

// Giả định cốt lõi: writer bỏ qua commit phải bị chặn.
func TestGradeCatchesMissingCommit(t *testing.T) {
	col := cleanCollected()
	col.Checkpoints = col.Checkpoints[:2] // bỏ commit
	r := Grade(writerSmokeCase(), col)
	if r.Outcome != Fail {
		t.Fatalf("thiếu commit phải FAIL, nhận được %s", r.Outcome)
	}
	if !hasIssue(r.HardFails, "contract:checkpoint", "chapter:1:commit") {
		t.Fatalf("phải báo cáo thiếu chapter:1:commit, thực tế %+v", r.HardFails)
	}
}

// Giả định cốt lõi: tín hiệu pending còn sót phải bị chặn.
func TestGradeCatchesPendingResidual(t *testing.T) {
	col := cleanCollected()
	col.Pending["pending_commit"] = true
	r := Grade(writerSmokeCase(), col)
	if r.Outcome != Fail {
		t.Fatalf("tín hiệu pending còn sót phải FAIL, nhận được %s", r.Outcome)
	}
	if !hasIssue(r.HardFails, "contract:no_pending", "pending_commit") {
		t.Fatalf("phải báo cáo pending_commit còn sót, thực tế %+v", r.HardFails)
	}
}

// Giả định cốt lõi: phase không khớp phải bị chặn.
func TestGradeCatchesPhaseMismatch(t *testing.T) {
	col := cleanCollected()
	col.Progress.Phase = domain.PhaseOutline // chưa vào writing
	r := Grade(writerSmokeCase(), col)
	if r.Outcome != Fail {
		t.Fatalf("phase không khớp phải FAIL, nhận được %s", r.Outcome)
	}
	if !hasIssue(r.HardFails, "contract:phase", "outline") {
		t.Fatalf("phải báo cáo phase không khớp, thực tế %+v", r.HardFails)
	}
}

func TestGradeMinChaptersNotMet(t *testing.T) {
	col := cleanCollected()
	col.Report.Stats.CompletedChapters = 0
	r := Grade(writerSmokeCase(), col)
	if r.Outcome != Fail {
		t.Fatalf("không đạt min_completed_chapters phải FAIL, nhận được %s", r.Outcome)
	}
}

// critical finding kích hoạt hard fail; warning finding chỉ WARN (max_severity=warning mặc định).
func TestGradeFindingSeverity(t *testing.T) {
	crit := cleanCollected()
	crit.Report.Findings = []diag.Finding{{Rule: "PhaseFlowMismatch", Severity: diag.SevCritical, Title: "Lỗi máy trạng thái"}}
	if r := Grade(writerSmokeCase(), crit); r.Outcome != Fail {
		t.Fatalf("critical finding phải FAIL, nhận được %s", r.Outcome)
	}

	warn := cleanCollected()
	warn.Report.Findings = []diag.Finding{{Rule: "RewritePendingPressure", Severity: diag.SevWarning, Title: "Tồn đọng viết lại"}}
	r := Grade(writerSmokeCase(), warn)
	if r.Outcome != Warn {
		t.Fatalf("warning finding phải WARN, nhận được %s", r.Outcome)
	}

	// info finding là Note thông tin, không nên đẩy case sạch thành WARN.
	info := cleanCollected()
	info.Report.Findings = []diag.Finding{{Rule: "GhostCharacter", Severity: diag.SevInfo, Title: "Nhân vật vắng mặt lâu dài"}}
	ri := Grade(writerSmokeCase(), info)
	if ri.Outcome != Pass {
		t.Fatalf("info finding không được thay đổi cổng kiểm soát, kỳ vọng PASS, nhận được %s", ri.Outcome)
	}
	if len(ri.Notes) != 1 {
		t.Fatalf("info finding phải vào Notes, nhận được %d mục", len(ri.Notes))
	}
}

func TestGradeRuntimeErrorFails(t *testing.T) {
	col := cleanCollected()
	col.RuntimeErr = "stream EOF"
	r := Grade(writerSmokeCase(), col)
	if r.Outcome != Fail {
		t.Fatalf("lỗi runtime phải FAIL, nhận được %s", r.Outcome)
	}
}

// Artifact phụ thuộc hợp đồng bị đọc lỗi không được false pass, phải hard fail (fail-loud).
func TestGradeLoadErrorFails(t *testing.T) {
	col := cleanCollected()
	col.LoadErrors = []string{"pending_commit: unexpected end of JSON input"}
	r := Grade(writerSmokeCase(), col)
	if r.Outcome != Fail {
		t.Fatalf("đọc artifact thất bại phải FAIL, nhận được %s", r.Outcome)
	}
	if !hasIssue(r.HardFails, "load", "pending_commit") {
		t.Fatalf("phải báo cáo load thất bại, thực tế %+v", r.HardFails)
	}
}

func TestGradeDeltaStylestatWarnAndBlock(t *testing.T) {
	base := cleanResult()
	base.Metrics.Stylestat = &stylestat.Stats{
		Patterns: []stylestat.PatternStat{{Name: "p", PerChapter: 1}},
		Ending:   stylestat.EndingStat{ShortRatio: 0.2},
	}
	variant := cleanResult()
	variant.Metrics.Stylestat = &stylestat.Stats{
		Patterns:          []stylestat.PatternStat{{Name: "p", PerChapter: 2}},
		RepeatedSentences: []stylestat.SentenceStat{{Text: "câu lặp", Chapters: 3, Count: 3}},
		Ending:            stylestat.EndingStat{ShortRatio: 0.5},
	}

	c := writerSmokeCase()
	c.Gate.StylestatRegression = "warn"
	d := GradeDelta(c, base, variant)
	if d.Outcome != Warn {
		t.Fatalf("hồi quy stylestat mặc định phải WARN, nhận được %s", d.Outcome)
	}
	if !hasIssue(d.Warnings, "delta:stylestat", "chỉ số phong cách hồi quy") {
		t.Fatalf("phải báo cáo stylestat warning, thực tế %+v", d.Warnings)
	}

	c.Gate.StylestatRegression = "block"
	d = GradeDelta(c, base, variant)
	if d.Outcome != Fail {
		t.Fatalf("stylestat block phải FAIL, nhận được %s", d.Outcome)
	}
	if !hasIssue(d.HardFails, "delta:stylestat", "chỉ số phong cách hồi quy") {
		t.Fatalf("phải báo cáo stylestat hard fail, thực tế %+v", d.HardFails)
	}
}

func TestGradeDeltaTitleMixedUsesMinorityCount(t *testing.T) {
	base := cleanResult()
	base.Metrics.Stylestat = &stylestat.Stats{
		TitleFormats: &stylestat.TitleStat{WithPrefix: 2, WithoutPrefix: 3},
	}
	variant := cleanResult()
	variant.Metrics.Stylestat = &stylestat.Stats{
		TitleFormats: &stylestat.TitleStat{WithPrefix: 2, WithoutPrefix: 5},
	}

	d := GradeDelta(writerSmokeCase(), base, variant)
	if d.Metrics.Stylestat == nil {
		t.Fatal("phải tạo ra stylestat delta")
	}
	if d.Metrics.Stylestat.TitleMixedDelta != 0 {
		t.Fatalf("khi định dạng thiểu số không tăng không được báo nhầm hồi quy tiêu đề lẫn lộn, nhận được %+d", d.Metrics.Stylestat.TitleMixedDelta)
	}
	if d.Outcome != Pass {
		t.Fatalf("chỉ tăng số lượng tiêu đề đa số không được thay đổi cổng kiểm soát, nhận được %s issues=%+v", d.Outcome, d.Warnings)
	}
}

func TestGradeDeltaCostAndToolCallThresholds(t *testing.T) {
	base := cleanResult()
	base.Metrics.ToolCalls = 10
	base.Metrics.Usage = UsageMetrics{UsageRecorded: true, CostUSD: 1, Input: 100, Output: 100}
	variant := cleanResult()
	variant.Metrics.ToolCalls = 14
	variant.Metrics.Usage = UsageMetrics{UsageRecorded: true, CostUSD: 1.4, Input: 150, Output: 140}

	c := writerSmokeCase()
	c.Gate.MaxToolCallDeltaRatio = float64Ptr(0.3)
	c.Gate.MaxCostDeltaRatio = float64Ptr(0.3)
	d := GradeDelta(c, base, variant)
	if d.Outcome != Warn {
		t.Fatalf("chi phí/tool_calls vượt ngưỡng phải WARN, nhận được %s", d.Outcome)
	}
	if !hasIssue(d.Warnings, "delta:tool_calls", "vượt quá ngưỡng") {
		t.Fatalf("phải báo cáo hồi quy tool_calls, thực tế %+v", d.Warnings)
	}
	if !hasIssue(d.Warnings, "delta:cost", "vượt quá ngưỡng") {
		t.Fatalf("phải báo cáo hồi quy cost, thực tế %+v", d.Warnings)
	}
}

func TestGradeDeltaInsufficientStylestatIsNote(t *testing.T) {
	d := GradeDelta(writerSmokeCase(), cleanResult(), cleanResult())
	if d.Outcome != Pass {
		t.Fatalf("mẫu không đủ không được thay đổi cổng kiểm soát, nhận được %s", d.Outcome)
	}
	if !hasIssue(d.Notes, "stylestat", "mẫu không đủ") {
		t.Fatalf("phải ghi lại note mẫu không đủ stylestat, thực tế %+v", d.Notes)
	}
}

func TestParseCheckpointSpec(t *testing.T) {
	cases := []struct {
		spec  string
		kind  domain.ScopeKind
		step  string
		valid bool
	}{
		{"chapter:1:commit", domain.ScopeChapter, "commit", true},
		{"arc:1:2:arc_summary", domain.ScopeArc, "arc_summary", true},
		{"volume:3:volume_summary", domain.ScopeVolume, "volume_summary", true},
		{"global:layered_outline", domain.ScopeGlobal, "layered_outline", true},
		{"chapter:commit", "", "", false},
		{"bogus:1:x", "", "", false},
	}
	for _, tc := range cases {
		scope, step, err := parseCheckpointSpec(tc.spec)
		if tc.valid && err != nil {
			t.Errorf("%s: kỳ vọng phân tích thành công, nhận được %v", tc.spec, err)
		}
		if !tc.valid {
			if err == nil {
				t.Errorf("%s: kỳ vọng phân tích thất bại", tc.spec)
			}
			continue
		}
		if scope.Kind != tc.kind || step != tc.step {
			t.Errorf("%s: phân tích thành kind=%s step=%s", tc.spec, scope.Kind, step)
		}
	}
}

func cleanResult() Result {
	r := Grade(writerSmokeCase(), cleanCollected())
	r.Metrics.TotalWords = 1000
	return r
}

// TestCollectReadsCheckpoints xác minh đường dẫn đọc store thực: sau khi ghi checkpoint, Collect có thể khớp hợp đồng.
func TestCollectReadsCheckpoints(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if _, err := s.Checkpoints.Append(domain.ChapterScope(1), "commit", "chapters/01.md", "d1"); err != nil {
		t.Fatalf("append checkpoint: %v", err)
	}

	col := Collect(dir, nil)
	ok, err := col.HasCheckpoint("chapter:1:commit")
	if err != nil || !ok {
		t.Fatalf("phải khớp chapter:1:commit, ok=%v err=%v", ok, err)
	}
	if miss, _ := col.HasCheckpoint("chapter:2:commit"); miss {
		t.Fatal("không được khớp chapter:2:commit không tồn tại")
	}
	if col.Pending["pending_commit"] {
		t.Fatal("thư mục sạch không được có pending_commit còn sót")
	}
}

func hasIssue(issues []Issue, source, detailContains string) bool {
	for _, it := range issues {
		if it.Source == source && strings.Contains(it.Detail, detailContains) {
			return true
		}
	}
	return false
}

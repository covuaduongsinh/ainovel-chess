package ctxpack

import (
	"context"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
	corecontext "github.com/voocel/agentcore/context"
	"github.com/voocel/ainovel-cli/internal/domain"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

func TestStoreSummaryCompactApplyUsesPersistentStoreData(t *testing.T) {
	s := seededWriterStore(t)
	strategy := NewStoreSummaryCompact(StoreSummaryCompactConfig{
		Store:              s,
		KeepRecentTokens:   80,
		SummaryTokenBudget: 2000,
	})

	msgs := []agentcore.AgentMessage{
		agentcore.UserMsg(strings.Repeat("ngữ cảnh cũ", 200)),
		agentcore.Message{
			Role:    agentcore.RoleAssistant,
			Content: []agentcore.ContentBlock{agentcore.TextBlock(strings.Repeat("phản hồi cũ", 200))},
		},
		agentcore.UserMsg("Tiếp tục viết chương ba, chú ý nối tiếp kết chương hai."),
		agentcore.Message{
			Role:    agentcore.RoleAssistant,
			Content: []agentcore.ContentBlock{agentcore.TextBlock("Nhận lệnh, tôi sẽ sắp xếp lại cảnh hiện tại trước.")},
		},
	}

	out, result, err := strategy.Apply(context.Background(), msgs, msgs, corecontext.Budget{
		Tokens:    corecontext.EstimateTotal(msgs),
		Window:    128,
		Threshold: 32,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !result.Applied {
		t.Fatal("expected store summary strategy to apply")
	}
	if result.Name != storeSummaryStrategyName {
		t.Fatalf("unexpected strategy name: %q", result.Name)
	}
	if len(out) < 2 {
		t.Fatalf("expected summary + kept messages, got %d", len(out))
	}
	summary, ok := out[0].(corecontext.ContextSummary)
	if !ok {
		t.Fatalf("expected ContextSummary, got %T", out[0])
	}
	if !strings.Contains(summary.Summary, "Tóm tắt chương gần đây") {
		t.Fatalf("expected persistent summaries in checkpoint, got %q", summary.Summary)
	}
	if !strings.Contains(summary.Summary, "Kế hoạch chương hiện tại") {
		t.Fatalf("expected chapter plan in checkpoint, got %q", summary.Summary)
	}
	if !strings.Contains(summary.Summary, "Phục bút đang hoạt động") {
		t.Fatalf("expected foreshadow data in checkpoint, got %q", summary.Summary)
	}
	if !strings.Contains(summary.Summary, "Vấn đề thẩm định chờ sửa") {
		t.Fatalf("expected pending review section in checkpoint, got %q", summary.Summary)
	}
	if !strings.Contains(summary.Summary, "manh mối kho cần tích áp thêm một nhịp") {
		t.Fatalf("expected pending review details in checkpoint, got %q", summary.Summary)
	}
	if result.Info == nil || result.Info.CompactedCount <= 0 {
		t.Fatalf("expected compaction info, got %+v", result.Info)
	}
}

func TestStoreSummaryCompactApplyFallsBackWhenStoreDataInsufficient(t *testing.T) {
	dir := t.TempDir()
	s := storepkg.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Save(&domain.Progress{
		Phase:             domain.PhaseWriting,
		CurrentChapter:    1,
		TotalChapters:     3,
		CompletedChapters: nil,
	}); err != nil {
		t.Fatalf("Save progress: %v", err)
	}

	strategy := NewStoreSummaryCompact(StoreSummaryCompactConfig{Store: s, KeepRecentTokens: 20})
	msgs := []agentcore.AgentMessage{
		agentcore.UserMsg(strings.Repeat("ngữ cảnh cũ", 40)),
		agentcore.Message{
			Role:    agentcore.RoleAssistant,
			Content: []agentcore.ContentBlock{agentcore.TextBlock(strings.Repeat("phản hồi cũ", 40))},
		},
	}

	out, result, err := strategy.Apply(context.Background(), msgs, msgs, corecontext.Budget{
		Tokens:    corecontext.EstimateTotal(msgs),
		Window:    64,
		Threshold: 16,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Applied {
		t.Fatal("expected no-op when persistent memory is insufficient")
	}
	if len(out) != len(msgs) {
		t.Fatalf("expected messages unchanged, got %d", len(out))
	}
}

func TestWriterRestorePackRefreshReusesStoreBuilder(t *testing.T) {
	s := seededWriterStore(t)
	pack := &WriterRestorePack{}
	pack.Refresh(s)

	msg, ok := pack.buildMessage(restoreBudgetTokens)
	if !ok {
		t.Fatal("expected restore pack message")
	}
	text := msg.TextContent()
	if !strings.Contains(text, "<post-compact-context>") {
		t.Fatalf("expected wrapped restore context, got %q", text)
	}
	if !strings.Contains(text, "Vấn đề thẩm định chờ sửa") {
		t.Fatalf("expected pending review section, got %q", text)
	}
	if !strings.Contains(text, "Kế hoạch chương hiện tại") {
		t.Fatalf("expected chapter plan section, got %q", text)
	}
}

func seededWriterStore(t *testing.T) *storepkg.Store {
	t.Helper()

	s := storepkg.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Save(&domain.Progress{
		Phase:             domain.PhaseWriting,
		CurrentChapter:    3,
		TotalChapters:     6,
		CompletedChapters: []int{1, 2},
		Flow:              domain.FlowWriting,
	}); err != nil {
		t.Fatalf("Save progress: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "Chương 1", CoreEvent: "mở màn"},
		{Chapter: 2, Title: "Chương 2", CoreEvent: "xung đột leo thang"},
		{Chapter: 3, Title: "Chương 3", CoreEvent: "truy tìm manh mối", Scenes: []string{"nhân vật chính điều tra vụ mất tích", "phát hiện manh mối kho cũ"}},
	}); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}
	if err := s.Drafts.SaveChapterPlan(domain.ChapterPlan{
		Chapter:    3,
		Title:      "Chương 3",
		Goal:       "thúc đẩy điều tra vụ mất tích",
		Conflict:   "nhân vật chính và đồng đội bất đồng hướng điều tra",
		Hook:       "phát hiện băng ghi âm nghi ngờ trong kho",
		EmotionArc: "từ nghi ngờ đến căng thẳng",
	}); err != nil {
		t.Fatalf("SaveChapterPlan: %v", err)
	}
	if err := s.Summaries.SaveSummary(domain.ChapterSummary{
		Chapter:    1,
		Summary:    "nhân vật chính nhận nhiệm vụ, phát hiện vụ mất tích không đơn giản.",
		Characters: []string{"Lâm Lam", "Chu Sách"},
		KeyEvents:  []string{"nhiệm vụ được thành lập"},
	}); err != nil {
		t.Fatalf("SaveSummary 1: %v", err)
	}
	if err := s.Summaries.SaveSummary(domain.ChapterSummary{
		Chapter:    2,
		Summary:    "hai người truy tìm bến cũ, manh mối chỉ đến kho bỏ hoang.",
		Characters: []string{"Lâm Lam", "Chu Sách", "chú Trần"},
		KeyEvents:  []string{"xung đột bến cũ", "manh mối kho xuất hiện"},
	}); err != nil {
		t.Fatalf("SaveSummary 2: %v", err)
	}
	if err := s.World.SaveForeshadowLedger([]domain.ForeshadowEntry{
		{ID: "tape", Description: "băng ghi âm mà người mất tích để lại", PlantedAt: 2, Status: "planted"},
	}); err != nil {
		t.Fatalf("SaveForeshadowLedger: %v", err)
	}
	if err := s.World.SaveTimeline([]domain.TimelineEvent{
		{Chapter: 2, Time: "ban đêm", Event: "đối đầu tại bến cũ", Characters: []string{"Lâm Lam", "Chu Sách"}},
	}); err != nil {
		t.Fatalf("SaveTimeline: %v", err)
	}
	if err := s.World.SaveStyleRules(domain.WritingStyleRules{
		Prose:  []string{"câu hơi ngắn, giữ cảm giác áp bức"},
		Taboos: []string{"tránh giải thích trực tiếp bí ẩn"},
	}); err != nil {
		t.Fatalf("SaveStyleRules: %v", err)
	}
	if err := s.World.SaveReview(domain.ReviewEntry{
		Chapter: 2,
		Scope:   "chapter",
		Verdict: "polish",
		Summary: "Kết thúc Chương 2 dẫn dắt hơi vội, cần bổ sung thêm một nhịp cảm giác áp bức trước kho.",
		Issues: []domain.ConsistencyIssue{
			{
				Type:        "pacing",
				Severity:    "warning",
				Description: "manh mối kho xuất hiện quá nhanh, tích áp huyền bí chưa đủ.",
				Suggestion:  "thêm một đoạn do dự và miêu tả áp bức môi trường trước khi vào kho.",
			},
		},
		ContractMisses: []string{"móc câu cuối chương chưa đủ mạnh"},
	}); err != nil {
		t.Fatalf("Save chapter review: %v", err)
	}
	if err := s.World.SaveReview(domain.ReviewEntry{
		Chapter: 2,
		Scope:   "global",
		Verdict: "polish",
		Summary: "Tiết tấu cuối Chương 2 hơi nhanh, manh mối kho cần tích áp thêm một nhịp.",
	}); err != nil {
		t.Fatalf("SaveReview: %v", err)
	}
	return s
}

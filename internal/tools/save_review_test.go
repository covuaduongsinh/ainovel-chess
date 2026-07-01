package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

func TestSaveReviewPersistsContractAssessment(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 10); err != nil {
		t.Fatalf("Progress.Init: %v", err)
	}
	if err := s.Progress.MarkChapterComplete(3, 3000, "", ""); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter":           3,
		"scope":             "chapter",
		"dimensions":        []map[string]any{{"dimension": "consistency", "score": 85, "verdict": "pass", "comment": "Ve co ban nhat quan"}, {"dimension": "character", "score": 82, "verdict": "pass", "comment": "Nhan vat on dinh"}, {"dimension": "pacing", "score": 78, "verdict": "warning", "comment": "Hoi cham"}, {"dimension": "continuity", "score": 84, "verdict": "pass", "comment": "Mach lac"}, {"dimension": "foreshadow", "score": 80, "verdict": "pass", "comment": "Binh thuong"}, {"dimension": "hook", "score": 76, "verdict": "warning", "comment": "Moc cau binh thuong"}, {"dimension": "aesthetic", "score": 81, "verdict": "pass", "comment": "Ngon ngu co ban on"}},
		"issues":            []map[string]any{},
		"contract_status":   "partial",
		"contract_misses":   []string{"Chua dat ro loi moi thu thach noi mon"},
		"contract_notes":    "Duong chinh da day tien, nhung hang muc day tien thu hai trong contract chua thuc hien.",
		"verdict":           "polish",
		"summary":           "Chuong nay co ban hoan thanh muc tieu, nhung contract van con thieu sot.",
		"affected_chapters": []int{3},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	review, err := s.World.LoadReview(3)
	if err != nil {
		t.Fatalf("LoadReview: %v", err)
	}
	if review == nil {
		t.Fatal("expected review saved, got nil")
	}
	if review.ContractStatus != "partial" {
		t.Fatalf("unexpected contract status: %q", review.ContractStatus)
	}
	if len(review.ContractMisses) != 1 || review.ContractMisses[0] != "Chua dat ro loi moi thu thach noi mon" {
		t.Fatalf("unexpected contract misses: %+v", review.ContractMisses)
	}
	if review.Dimension("aesthetic") == nil {
		t.Fatalf("expected aesthetic dimension persisted, got %+v", review.Dimensions)
	}
}

func TestSaveReviewRejectsMissingDimensions(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 10); err != nil {
		t.Fatalf("Progress.Init: %v", err)
	}
	if err := s.Progress.MarkChapterComplete(3, 3000, "", ""); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter":    3,
		"scope":      "chapter",
		"dimensions": []map[string]any{{"dimension": "consistency", "score": 85, "verdict": "pass", "comment": "Ve co ban nhat quan"}},
		"issues":     []map[string]any{},
		"verdict":    "accept",
		"summary":    "ok",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil || !strings.Contains(err.Error(), "dimensions must contain exactly") {
		t.Fatalf("expected dimensions validation error, got %v", err)
	}
}

func TestSaveReviewRejectsDimensionWithoutComment(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 10); err != nil {
		t.Fatalf("Progress.Init: %v", err)
	}
	if err := s.Progress.MarkChapterComplete(3, 3000, "", ""); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter": 3,
		"scope":   "chapter",
		"dimensions": []map[string]any{
			{"dimension": "consistency", "score": 85, "comment": "Ve co ban nhat quan"},
			{"dimension": "character", "score": 82, "comment": "Nhan vat on dinh"},
			{"dimension": "pacing", "score": 78},
			{"dimension": "continuity", "score": 84, "comment": "Mach lac"},
			{"dimension": "foreshadow", "score": 80, "comment": "Binh thuong"},
			{"dimension": "hook", "score": 76, "comment": "Moc cau binh thuong"},
			{"dimension": "aesthetic", "score": 81, "comment": "Ngon ngu co ban on"},
		},
		"issues":  []map[string]any{},
		"verdict": "accept",
		"summary": "ok",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil || !strings.Contains(err.Error(), "dimension comment is required: pacing") {
		t.Fatalf("expected dimension comment validation error, got %v", err)
	}
}

func TestSaveReviewRejectsUnfinishedAffectedChapter(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 80); err != nil {
		t.Fatalf("Progress.Init: %v", err)
	}
	for ch := 1; ch <= 58; ch++ {
		if err := s.Progress.MarkChapterComplete(ch, 3000, "", ""); err != nil {
			t.Fatalf("MarkChapterComplete(%d): %v", ch, err)
		}
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter": 58,
		"scope":   "chapter",
		"dimensions": []map[string]any{
			{"dimension": "consistency", "score": 85, "comment": "Ve co ban nhat quan"},
			{"dimension": "character", "score": 82, "comment": "Nhan vat on dinh"},
			{"dimension": "pacing", "score": 58, "comment": "Nhip do can viet lai"},
			{"dimension": "continuity", "score": 84, "comment": "Mach lac"},
			{"dimension": "foreshadow", "score": 80, "comment": "Binh thuong"},
			{"dimension": "hook", "score": 76, "comment": "Moc cau binh thuong"},
			{"dimension": "aesthetic", "score": 81, "comment": "Ngon ngu co ban on"},
		},
		"issues":            []map[string]any{},
		"contract_status":   "partial",
		"verdict":           "polish",
		"summary":           "Can danh bong chuong 58, khong the cho chuong chua hoan thanh vao hang doi.",
		"affected_chapters": []int{65},
		"contract_misses":   []string{"Nhip do vuot qua trach nhiem cua chuong nay"},
		"contract_notes":    "Chi nen xu ly cac chuong da hoan thanh.",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil || !strings.Contains(err.Error(), "pending_rewrites") {
		t.Fatalf("expected unfinished affected chapter rejection, got %v", err)
	}
	review, err := s.World.LoadReview(58)
	if err != nil {
		t.Fatalf("LoadReview: %v", err)
	}
	if review != nil {
		t.Fatalf("review should not be saved when pending rewrite validation fails: %+v", review)
	}
	p, _ := s.Progress.Load()
	if p.Flow != domain.FlowWriting && p.Flow != "" {
		t.Fatalf("flow should not enter rewrite/polish, got %s", p.Flow)
	}
	if len(p.PendingRewrites) != 0 {
		t.Fatalf("pending_rewrites should remain empty, got %v", p.PendingRewrites)
	}
}

// TestSaveReviewDerivesVerdictFromScore xac minh: verdict duoc suy xuat tat dinh tu score, neu mo hinh
// cho verdict khong nhat quan (vi du score=85 nhung dien warning) se khong bao loi ma bi ghi de thanh gia tri dung (pass).
// Phong ngua hoi quy: xung dot score/verdict tu mo hinh yeu da tung lam save_review that bai lien tuc.
func TestSaveReviewDerivesVerdictFromScore(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 10); err != nil {
		t.Fatalf("Progress.Init: %v", err)
	}
	if err := s.Progress.MarkChapterComplete(3, 3000, "", ""); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter": 3,
		"scope":   "chapter",
		"dimensions": []map[string]any{
			{"dimension": "consistency", "score": 85, "verdict": "pass", "comment": "Nhat quan"},
			{"dimension": "character", "score": 82, "comment": "On dinh"}, // bo qua verdict
			{"dimension": "pacing", "score": 78, "verdict": "warning", "comment": "Hoi cham"},
			{"dimension": "continuity", "score": 84, "verdict": "pass", "comment": "Mach lac"},
			{"dimension": "foreshadow", "score": 80, "verdict": "pass", "comment": "Binh thuong"},
			{"dimension": "hook", "score": 76, "verdict": "warning", "comment": "Moc cau binh thuong"},
			{"dimension": "aesthetic", "score": 85, "verdict": "warning", "comment": "Ngon ngu on"}, // khong nhat quan: 85 nhung lai dien warning
		},
		"issues":  []map[string]any{},
		"verdict": "accept",
		"summary": "ok",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute should succeed (verdict auto-derived), got %v", err)
	}

	review, err := s.World.LoadReview(3)
	if err != nil || review == nil {
		t.Fatalf("LoadReview: %v", err)
	}
	// 85 → pass (ghi de warning tu mo hinh); 82 bo qua → pass.
	if d := review.Dimension("aesthetic"); d == nil || d.Verdict != "pass" {
		t.Fatalf("aesthetic verdict should be derived to pass, got %+v", d)
	}
	if d := review.Dimension("character"); d == nil || d.Verdict != "pass" {
		t.Fatalf("character verdict should be derived to pass, got %+v", d)
	}
}

func TestSaveReviewRejectsMissingAffectedChaptersForRewrite(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter": 3,
		"scope":   "chapter",
		"dimensions": []map[string]any{
			{"dimension": "consistency", "score": 85, "verdict": "pass", "comment": "Ve co ban nhat quan"},
			{"dimension": "character", "score": 82, "verdict": "pass", "comment": "Nhan vat on dinh"},
			{"dimension": "pacing", "score": 78, "verdict": "warning", "comment": "Hoi cham"},
			{"dimension": "continuity", "score": 84, "verdict": "pass", "comment": "Mach lac"},
			{"dimension": "foreshadow", "score": 80, "verdict": "pass", "comment": "Binh thuong"},
			{"dimension": "hook", "score": 76, "verdict": "warning", "comment": "Moc cau binh thuong"},
			{"dimension": "aesthetic", "score": 81, "verdict": "pass", "comment": "Ngon ngu co ban on"},
		},
		"issues":  []map[string]any{},
		"verdict": "rewrite",
		"summary": "Can viet lai",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil || !strings.Contains(err.Error(), "affected_chapters is required") {
		t.Fatalf("expected affected_chapters validation error, got %v", err)
	}
}

func TestSaveReviewRejectsIssueWithoutEvidence(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter": 3,
		"scope":   "chapter",
		"dimensions": []map[string]any{
			{"dimension": "consistency", "score": 85, "verdict": "pass", "comment": "Ve co ban nhat quan"},
			{"dimension": "character", "score": 82, "verdict": "pass", "comment": "Nhan vat on dinh"},
			{"dimension": "pacing", "score": 78, "verdict": "warning", "comment": "Hoi cham"},
			{"dimension": "continuity", "score": 84, "verdict": "pass", "comment": "Mach lac"},
			{"dimension": "foreshadow", "score": 80, "verdict": "pass", "comment": "Binh thuong"},
			{"dimension": "hook", "score": 76, "verdict": "warning", "comment": "Moc cau binh thuong"},
			{"dimension": "aesthetic", "score": 81, "verdict": "pass", "comment": "Ngon ngu co ban on"},
		},
		"issues": []map[string]any{
			{"type": "hook", "severity": "warning", "description": "Moc cau cuoi chuong yeu"},
		},
		"verdict":           "polish",
		"summary":           "Can tang cuong moc cau.",
		"affected_chapters": []int{3},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil || !strings.Contains(err.Error(), "issue evidence is required") {
		t.Fatalf("expected issue evidence validation error, got %v", err)
	}
}

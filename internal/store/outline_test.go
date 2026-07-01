package store

import (
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
)

func setupLayered(t *testing.T, volumes []domain.VolumeOutline) *Store {
	t.Helper()
	s := NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.Outline.SaveLayeredOutline(volumes); err != nil {
		t.Fatalf("SaveLayeredOutline: %v", err)
	}
	if err := s.Progress.SetLayered(true); err != nil {
		t.Fatalf("SetLayered: %v", err)
	}
	return s
}

func TestCheckArcBoundaryNeedsNewVolume(t *testing.T) {
	// Chỉ có 1 tập 1 cung 1 chương, và không phải Final → nên kích hoạt NeedsNewVolume
	s := setupLayered(t, []domain.VolumeOutline{{
		Index: 1, Title: "Tập một", Theme: "Khởi đầu",
		Arcs: []domain.ArcOutline{{
			Index: 1, Title: "Cung đầu", Goal: "Mục tiêu",
			Chapters: []domain.OutlineEntry{{Title: "Chương một", CoreEvent: "Mở màn", Hook: "Tiếp tục"}},
		}},
	}})

	b, err := s.Outline.CheckArcBoundary(1) // Chương 1 = chương cuối của cung/tập
	if err != nil {
		t.Fatalf("CheckArcBoundary: %v", err)
	}
	if b == nil {
		t.Fatal("expected boundary, got nil")
	}
	if !b.IsArcEnd || !b.IsVolumeEnd {
		t.Fatalf("expected arc+volume end, got arc=%v vol=%v", b.IsArcEnd, b.IsVolumeEnd)
	}
	if !b.NeedsNewVolume {
		t.Fatal("expected NeedsNewVolume=true")
	}
	if b.NextVolume != 0 || b.NextArc != 0 {
		t.Fatalf("expected no next, got vol=%d arc=%d", b.NextVolume, b.NextArc)
	}
}

func TestCheckArcBoundaryLastVolumeRequiresDecision(t *testing.T) {
	// Chương cuối của tập duy nhất → kích hoạt NeedsNewVolume, để Router cho kiến trúc sư chọn một trong hai:
	// append_volume tiếp tục viết / complete_book kết thúc.
	s := setupLayered(t, []domain.VolumeOutline{{
		Index: 1, Title: "Tập duy nhất", Theme: "Chủ đề",
		Arcs: []domain.ArcOutline{{
			Index: 1, Title: "Cung duy nhất", Goal: "Thu kết",
			Chapters: []domain.OutlineEntry{{Title: "Chương cuối", CoreEvent: "Kết cục", Hook: "Không"}},
		}},
	}})

	b, err := s.Outline.CheckArcBoundary(1)
	if err != nil {
		t.Fatalf("CheckArcBoundary: %v", err)
	}
	if !b.NeedsNewVolume {
		t.Fatal("expected NeedsNewVolume=true at last expanded chapter")
	}
	if b.HasNextArc() {
		t.Fatal("expected no next arc")
	}
}

func TestCheckArcBoundaryNextArcInSameVolume(t *testing.T) {
	// 2 cung: kết thúc cung 1 nên chỉ sang cung 2, không kích hoạt NeedsNewVolume
	s := setupLayered(t, []domain.VolumeOutline{{
		Index: 1, Title: "Tập một", Theme: "Khởi đầu",
		Arcs: []domain.ArcOutline{
			{Index: 1, Title: "Cung đầu", Goal: "Mục tiêu", Chapters: []domain.OutlineEntry{{Title: "Chương một", CoreEvent: "Sự kiện", Hook: "Móc thu hút"}}},
			{Index: 2, Title: "Cung hai", Goal: "Mục tiêu 2", EstimatedChapters: 10},
		},
	}})

	b, err := s.Outline.CheckArcBoundary(1)
	if err != nil {
		t.Fatalf("CheckArcBoundary: %v", err)
	}
	if !b.IsArcEnd {
		t.Fatal("expected arc end")
	}
	if b.IsVolumeEnd {
		t.Fatal("expected not volume end (second arc exists)")
	}
	if b.NeedsNewVolume {
		t.Fatal("expected NeedsNewVolume=false")
	}
	if b.NextVolume != 1 || b.NextArc != 2 {
		t.Fatalf("expected next vol=1 arc=2, got vol=%d arc=%d", b.NextVolume, b.NextArc)
	}
	if !b.NeedsExpansion {
		t.Fatal("expected NeedsExpansion=true for skeleton arc")
	}
}

func TestAppendVolumeValidation(t *testing.T) {
	s := setupLayered(t, []domain.VolumeOutline{{
		Index: 1, Title: "Tập một", Theme: "Khởi đầu",
		Arcs: []domain.ArcOutline{{
			Index: 1, Title: "Cung đầu", Goal: "Mục tiêu",
			Chapters: []domain.OutlineEntry{{Title: "Chương", CoreEvent: "Sự kiện", Hook: "Móc thu hút"}},
		}},
	}})

	validVol := domain.VolumeOutline{
		Index: 2, Title: "Tập hai", Theme: "Nâng cấp",
		Arcs: []domain.ArcOutline{{
			Index: 1, Title: "Cung một", Goal: "Mục tiêu",
			Chapters: []domain.OutlineEntry{{Title: "Chương mới", CoreEvent: "Tiến triển", Hook: "Móc thu hút"}},
		}},
	}

	// Thêm bình thường nên thành công
	if err := s.AppendVolume(validVol); err != nil {
		t.Fatalf("AppendVolume valid: %v", err)
	}

	// Index không tăng dần → thất bại
	if err := s.AppendVolume(domain.VolumeOutline{
		Index: 1, Title: "Trùng lặp", Theme: "x",
		Arcs: []domain.ArcOutline{{Index: 1, Title: "Cung", Goal: "g", Chapters: []domain.OutlineEntry{{Title: "ch", CoreEvent: "e", Hook: "h"}}}},
	}); err == nil {
		t.Fatal("expected error for non-increasing index")
	}

	// Không có cung → thất bại
	if err := s.AppendVolume(domain.VolumeOutline{Index: 3, Title: "Rỗng", Theme: "x"}); err == nil {
		t.Fatal("expected error for volume with no arcs")
	}

	// Cung đầu tiên không có chương → thất bại
	if err := s.AppendVolume(domain.VolumeOutline{
		Index: 3, Title: "Khung xương", Theme: "x",
		Arcs: []domain.ArcOutline{{Index: 1, Title: "Cung", Goal: "g", EstimatedChapters: 10}},
	}); err == nil {
		t.Fatal("expected error for first arc without chapters")
	}
}

// Lưu ý: ngữ nghĩa từ chối append dựa trên tập Final trước đây đã được đưa xuống lớp save_foundation (từ chối Phase=Complete),
// xem save_foundation_test.go::TestSaveFoundationAppendVolumeRejectsAfterComplete.
// Lớp store chỉ giữ lại các kiểm tra cấu trúc (Index tăng dần / cung đầu có chương, v.v.).

func TestSaveAndLoadCompass(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// direction rỗng nên thất bại
	if err := s.Outline.SaveCompass(domain.StoryCompass{EstimatedScale: "3 tập"}); err == nil {
		t.Fatal("expected error for empty ending_direction")
	}

	// Lưu bình thường
	compass := domain.StoryCompass{
		EndingDirection: "Nhân vật chính đối mặt với lựa chọn cuối cùng",
		OpenThreads:     []string{"Manh mối A", "Quan hệ B"},
		EstimatedScale:  "Ước tính 4-6 tập",
		LastUpdated:     12,
	}
	if err := s.Outline.SaveCompass(compass); err != nil {
		t.Fatalf("SaveCompass: %v", err)
	}

	loaded, err := s.Outline.LoadCompass()
	if err != nil {
		t.Fatalf("LoadCompass: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected compass, got nil")
	}
	if loaded.EndingDirection != "Nhân vật chính đối mặt với lựa chọn cuối cùng" {
		t.Fatalf("expected direction %q, got %q", "Nhân vật chính đối mặt với lựa chọn cuối cùng", loaded.EndingDirection)
	}
	if len(loaded.OpenThreads) != 2 {
		t.Fatalf("expected 2 threads, got %d", len(loaded.OpenThreads))
	}
}

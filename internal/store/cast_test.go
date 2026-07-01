package store

import (
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
)

func newCastTestStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return s
}

func TestCastMergeAppearances_NewEntries(t *testing.T) {
	s := newCastTestStore(t)
	intros := []domain.CastIntro{{Name: "Lao Chu", BriefRole: "chu-quan-tro"}}
	if err := s.Cast.MergeAppearances(5, []string{"Lao Chu", "A Van"}, intros, nil); err != nil {
		t.Fatalf("MergeAppearances: %v", err)
	}

	entries, err := s.Cast.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.FirstSeenChapter != 5 || e.LastSeenChapter != 5 || e.AppearanceCount != 1 {
			t.Errorf("entry %s: unexpected appearance fields %+v", e.Name, e)
		}
		if e.Name == "Lao Chu" && e.BriefRole != "chu-quan-tro" {
			t.Errorf("expected BriefRole chu-quan-tro for Lao Chu, got %q", e.BriefRole)
		}
		if e.Name == "A Van" && e.BriefRole != "" {
			t.Errorf("A Van khong co intro, BriefRole phai la rong, nhan duoc %q", e.BriefRole)
		}
	}
}

func TestCastMergeAppearances_AccumulatesOnRepeat(t *testing.T) {
	s := newCastTestStore(t)
	if err := s.Cast.MergeAppearances(5, []string{"Lao Chu"}, nil, nil); err != nil {
		t.Fatalf("first merge: %v", err)
	}
	if err := s.Cast.MergeAppearances(8, []string{"Lao Chu"}, nil, nil); err != nil {
		t.Fatalf("second merge: %v", err)
	}

	entries, _ := s.Cast.Load()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.FirstSeenChapter != 5 || e.LastSeenChapter != 8 || e.AppearanceCount != 2 {
		t.Fatalf("expected first=5,last=8,count=2; got %+v", e)
	}
	if len(e.AppearanceChapters) != 2 || e.AppearanceChapters[0] != 5 || e.AppearanceChapters[1] != 8 {
		t.Errorf("AppearanceChapters wrong: %v", e.AppearanceChapters)
	}
}

func TestCastMergeAppearances_IsIdempotent(t *testing.T) {
	s := newCastTestStore(t)
	if err := s.Cast.MergeAppearances(5, []string{"Lao Chu"}, nil, nil); err != nil {
		t.Fatalf("first merge: %v", err)
	}
	// Commit cùng chương bị gọi nhiều lần (phục hồi sau sự cố hoặc viết lại)
	if err := s.Cast.MergeAppearances(5, []string{"Lao Chu"}, nil, nil); err != nil {
		t.Fatalf("second merge: %v", err)
	}

	entries, _ := s.Cast.Load()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].AppearanceCount != 1 {
		t.Errorf("expected AppearanceCount=1 after duplicate, got %d", entries[0].AppearanceCount)
	}
}

func TestCastMergeAppearances_FiltersCoreCharacters(t *testing.T) {
	s := newCastTestStore(t)
	core := map[string]bool{"Lam Moc": true, "Li Thanh Nghien": true}
	if err := s.Cast.MergeAppearances(3, []string{"Lam Moc", "Li Thanh Nghien", "Lao Chu"}, nil, core); err != nil {
		t.Fatalf("MergeAppearances: %v", err)
	}

	entries, _ := s.Cast.Load()
	if len(entries) != 1 || entries[0].Name != "Lao Chu" {
		t.Fatalf("expected only Lao Chu in ledger, got %+v", entries)
	}
}

func TestCastMergeAppearances_BackfillsBriefRole(t *testing.T) {
	s := newCastTestStore(t)
	// Chương 5 giới thiệu Lao Chu nhưng Writer quên điền brief_role
	if err := s.Cast.MergeAppearances(5, []string{"Lao Chu"}, nil, nil); err != nil {
		t.Fatalf("first merge: %v", err)
	}
	// Chương 8 xuất hiện lại, Writer lần này bổ sung brief_role
	intros := []domain.CastIntro{{Name: "Lao Chu", BriefRole: "chu-quan-tro"}}
	if err := s.Cast.MergeAppearances(8, []string{"Lao Chu"}, intros, nil); err != nil {
		t.Fatalf("second merge: %v", err)
	}

	entries, _ := s.Cast.Load()
	if entries[0].BriefRole != "chu-quan-tro" {
		t.Errorf("expected BriefRole chu-quan-tro backfilled, got %q", entries[0].BriefRole)
	}
}

func TestCastMergeAppearances_NoOverwriteBriefRole(t *testing.T) {
	s := newCastTestStore(t)
	// Chương 5 xác định BriefRole=chu-quan-tro
	if err := s.Cast.MergeAppearances(5,
		[]string{"Lao Chu"},
		[]domain.CastIntro{{Name: "Lao Chu", BriefRole: "chu-quan-tro"}},
		nil,
	); err != nil {
		t.Fatalf("first merge: %v", err)
	}
	// Chương 8 Writer truyền nhầm BriefRole khác (không nên ghi đè)
	if err := s.Cast.MergeAppearances(8,
		[]string{"Lao Chu"},
		[]domain.CastIntro{{Name: "Lao Chu", BriefRole: "tay-danh-bac"}},
		nil,
	); err != nil {
		t.Fatalf("second merge: %v", err)
	}

	entries, _ := s.Cast.Load()
	if entries[0].BriefRole != "chu-quan-tro" {
		t.Errorf("expected BriefRole NOT overwritten, got %q", entries[0].BriefRole)
	}
}

func TestCastRecentActive_OrdersByLastSeen(t *testing.T) {
	s := newCastTestStore(t)
	_ = s.Cast.MergeAppearances(3, []string{"A"}, nil, nil)
	_ = s.Cast.MergeAppearances(10, []string{"B"}, nil, nil)
	_ = s.Cast.MergeAppearances(7, []string{"C"}, nil, nil)

	recent, err := s.Cast.RecentActive(2)
	if err != nil {
		t.Fatalf("RecentActive: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("expected 2, got %d", len(recent))
	}
	if recent[0].Name != "B" || recent[1].Name != "C" {
		t.Errorf("expected order B, C; got %s, %s", recent[0].Name, recent[1].Name)
	}
}

func TestCastRecentActive_SkipsPromoted(t *testing.T) {
	s := newCastTestStore(t)
	if err := s.Cast.Save([]domain.CastEntry{
		{Name: "da-thang-cap-nhan-vat-chinh", LastSeenChapter: 20, AppearanceCount: 8, Promoted: true},
		{Name: "nhan-vat-phu-hoat-dong", LastSeenChapter: 18, AppearanceCount: 3},
		{Name: "nhan-vat-phu-khac", LastSeenChapter: 15, AppearanceCount: 2},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	recent, err := s.Cast.RecentActive(10)
	if err != nil {
		t.Fatalf("RecentActive: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("expected 2 (Promoted excluded), got %d: %+v", len(recent), recent)
	}
	for _, e := range recent {
		if e.Promoted {
			t.Errorf("Promoted entry leaked into RecentActive: %+v", e)
		}
	}
	if recent[0].Name != "nhan-vat-phu-hoat-dong" {
		t.Errorf("expected first=nhan-vat-phu-hoat-dong, got %s", recent[0].Name)
	}
}

func TestCastMergeAppearances_NoOpOnEmpty(t *testing.T) {
	s := newCastTestStore(t)
	if err := s.Cast.MergeAppearances(5, nil, nil, nil); err != nil {
		t.Fatalf("MergeAppearances empty: %v", err)
	}
	if err := s.Cast.MergeAppearances(0, []string{"Lao Chu"}, nil, nil); err != nil {
		t.Fatalf("MergeAppearances chapter=0: %v", err)
	}
	entries, _ := s.Cast.Load()
	if len(entries) != 0 {
		t.Errorf("expected empty ledger, got %d entries", len(entries))
	}
}

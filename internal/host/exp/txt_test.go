package exp

import (
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
)

func TestStripChapterTitleHeader(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		title string
		want  string
	}{
		{"plain body untouched", "Hắn nhìn ra ngoài cửa sổ.", "Người về đêm mưa", "Hắn nhìn ra ngoài cửa sổ."},
		{"strip h1 chapter header", "# 第 1 章  Người về đêm mưa\n\nHắn nhìn ra ngoài cửa sổ.", "Người về đêm mưa", "Hắn nhìn ra ngoài cửa sổ."},
		{"strip h2 with chapter token", "## 第二章\n\nHắn nhìn ra ngoài cửa sổ.", "", "Hắn nhìn ra ngoài cửa sổ."},
		{"keep body even if no header", "Câu đầu tiên của nội dung.\nCâu thứ hai.", "", "Câu đầu tiên của nội dung.\nCâu thứ hai."},
		{"do not strip non-chapter heading", "# Lời mở đầu\nHắn nhìn ra ngoài cửa sổ.", "Phù sinh thôn biên", "# Lời mở đầu\nHắn nhìn ra ngoài cửa sổ."},
		{"single line header only", "# 第 1 章", "", ""},
		// writer viet ten chuong thuan tuy lam tieu de vao dong dau -> trung lap voi tieu de thong nhat cua trinh xuat, nen loai bo
		{"strip h1 matching chapter title", "# Phù sinh thôn biên\n\nTrời chưa sáng.", "Phù sinh thôn biên", "Trời chưa sáng."},
		// Dong dau h1 nhung noi dung khong bang tieu de chuong nay -> coi la noi dung, giu lai
		{"keep h1 not matching title", "# Tiêu đề khác\nNội dung.", "Phù sinh thôn biên", "# Tiêu đề khác\nNội dung."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := stripChapterTitleHeader(c.in, c.title)
			if got != c.want {
				t.Fatalf("stripChapterTitleHeader\nin   = %q\ntitle= %q\nwant = %q\ngot  = %q", c.in, c.title, c.want, got)
			}
		})
	}
}

func TestBuildTitleIndex(t *testing.T) {
	outline := []domain.OutlineEntry{
		{Chapter: 1, Title: "Người về đêm mưa"},
		{Chapter: 2, Title: ""}, // Tieu de rong nen bi loc bo
		{Chapter: 3, Title: "Bình minh"},
	}
	idx := buildTitleIndex(outline)
	if got := idx[1]; got != "Người về đêm mưa" {
		t.Errorf("ch1 title: got %q want Người về đêm mưa", got)
	}
	if _, ok := idx[2]; ok {
		t.Errorf("ch2 should be absent (empty title)")
	}
	if got := idx[3]; got != "Bình minh" {
		t.Errorf("ch3 title: got %q want Bình minh", got)
	}
}

func TestBuildLocations(t *testing.T) {
	volumes := []domain.VolumeOutline{
		{Index: 1, Title: "Khởi nguyên", Arcs: []domain.ArcOutline{
			{Index: 1, Title: "Thiếu niên lần đầu xuất hiện", Chapters: []domain.OutlineEntry{{}, {}}}, // 2 chuong
			{Index: 2, Title: "Thử thách tông môn", Chapters: []domain.OutlineEntry{{}}},              // 1 chuong
		}},
		{Index: 2, Title: "Trỗi dậy", Arcs: []domain.ArcOutline{
			{Index: 1, Title: "Trận đầu", Chapters: []domain.OutlineEntry{{}}},
		}},
	}
	locs := buildLocations(volumes)

	// Chi kiem tra thuoc tap: cung khong vao location, nhung lop cung van tham gia tinh so chuong toan cuc.
	if loc := locs[1]; !loc.IsFirstOfVolume || loc.VolumeIdx != 1 {
		t.Errorf("ch1 should be first of volume 1: %+v", loc)
	}
	if loc := locs[2]; loc.IsFirstOfVolume || loc.VolumeIdx != 1 {
		t.Errorf("ch2 should be volume 1, not first: %+v", loc)
	}
	// ch3 la chuong dau cua cung 2, nhung van nam trong tap 1 -> khong phai dau tap.
	if loc := locs[3]; loc.IsFirstOfVolume || loc.VolumeIdx != 1 {
		t.Errorf("ch3 (arc 2, same volume) should not be first of volume: %+v", loc)
	}
	if loc := locs[4]; !loc.IsFirstOfVolume || loc.VolumeIdx != 2 {
		t.Errorf("ch4 should start volume 2: %+v", loc)
	}
}

func TestRenderTXT_TitleAndChapter(t *testing.T) {
	got := renderTXT(
		"Ánh Chớp",
		[]int{1, 2},
		chapterTitleIndex{1: "Người về đêm mưa", 2: "Bình minh"},
		nil,
		map[int]string{
			1: "# 第 1 章 Người về đêm mưa\n\nHắn nhìn ra ngoài cửa sổ.",
			2: "Cô ấy mở cửa.",
		},
	)
	if !strings.HasPrefix(got, "《Ánh Chớp》\n\n") {
		t.Errorf("missing book title at start:\n%s", got)
	}
	// premise khong vao xuat: sau ten sach nen la chuong ngay, khong chen bat ky tien de nao
	if !strings.Contains(got, "Chuong 1  Người về đêm mưa") {
		t.Errorf("missing ch1 header")
	}
	if !strings.Contains(got, "Hắn nhìn ra ngoài cửa sổ.") {
		t.Errorf("missing ch1 body")
	}
	if strings.Contains(got, "# 第 1 章") {
		t.Errorf("body markdown header not stripped:\n%s", got)
	}
	if !strings.Contains(got, "Chuong 2  Bình minh") {
		t.Errorf("missing ch2 header")
	}
}

func TestRenderTXT_EmptyNovelNameNoTitleLine(t *testing.T) {
	got := renderTXT(
		"",
		[]int{1},
		chapterTitleIndex{1: "Người về đêm mưa"},
		nil,
		map[int]string{1: "Nội dung."},
	)
	if strings.Contains(got, "《") {
		t.Errorf("should not contain book title brackets: %s", got)
	}
	if !strings.HasPrefix(got, "Chuong 1  Người về đêm mưa") {
		t.Errorf("expect chapter header at very start: %s", got)
	}
}

// TestRenderTXT_LayeredVolume Kiem tra de cuong phan tang chi chen phan cach tap o dau tap, phan cach cung khong bao gio xuat hien
// (issue #27: dinh dang la "Ten sach -> phan cach tap -> noi dung chuong").
func TestRenderTXT_LayeredVolume(t *testing.T) {
	locs := map[int]chapterLocation{
		1: {VolumeIdx: 1, VolumeTitle: "Khởi nguyên", IsFirstOfVolume: true},
		2: {VolumeIdx: 1, VolumeTitle: "Khởi nguyên"},
	}
	got := renderTXT(
		"X", []int{1, 2},
		chapterTitleIndex{1: "A", 2: "B"},
		locs,
		map[int]string{1: "Nội dung một.", 2: "Nội dung hai."},
	)
	if !strings.Contains(got, "Tap 1  Khởi nguyên") {
		t.Errorf("missing volume header: %s", got)
	}
	if strings.Contains(got, "弧") {
		t.Errorf("arc divider should never appear: %s", got)
	}
	// Tieu de tap chi xuat hien mot lan truoc chuong dau
	if strings.Count(got, "Tap 1") != 1 {
		t.Errorf("volume header should appear exactly once: %s", got)
	}
}

func TestRenderTXT_ChapterWithoutTitleFallsBackToNumberOnly(t *testing.T) {
	got := renderTXT(
		"", []int{5},
		chapterTitleIndex{}, // Khong co tieu de
		nil,
		map[int]string{5: "Nội dung."},
	)
	if !strings.Contains(got, "Chuong 5\n\n") {
		t.Errorf("expect 'Chuong 5' fallback header: %s", got)
	}
}

package exp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

// newTestStore Xay dung mot store toi gian tren t.TempDir(), da ghi ban thao cuoi 1..n chuong va progress.
func newTestStore(t *testing.T, novelName string, completed []int) (*store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	if err := s.Progress.Init(novelName, len(completed)); err != nil {
		t.Fatalf("init progress: %v", err)
	}
	for _, ch := range completed {
		if err := s.Drafts.SaveFinalChapter(ch, fmt.Sprintf("Nội dung ch %d.", ch)); err != nil {
			t.Fatalf("save chapter %d: %v", ch, err)
		}
		if err := s.Progress.StartChapter(ch); err != nil {
			t.Fatalf("start chapter %d: %v", ch, err)
		}
		if err := s.Progress.MarkChapterComplete(ch, 5, "cliff", "main"); err != nil {
			t.Fatalf("mark complete %d: %v", ch, err)
		}
	}
	return s, dir
}

func TestRun_HappyPath_DefaultsToNovelDir(t *testing.T) {
	s, dir := newTestStore(t, "Ánh Chớp", []int{1, 2, 3})
	if err := s.Outline.SavePremise("Câu chuyện ánh sáng và bóng tối."); err != nil {
		t.Fatalf("save premise: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "Người về đêm mưa"},
		{Chapter: 2, Title: "Bình minh"},
		{Chapter: 3, Title: "Tàn tro"},
	}); err != nil {
		t.Fatalf("save outline: %v", err)
	}

	res, err := Run(context.Background(), Deps{Store: s}, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Chapters != 3 {
		t.Errorf("Chapters = %d, want 3", res.Chapters)
	}
	if res.Path != filepath.Join(dir, "Ánh Chớp.txt") {
		t.Errorf("Path = %q, want default {dir}/Ánh Chớp.txt", res.Path)
	}
	data, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := string(data)
	for _, want := range []string{"《Ánh Chớp》", "Chuong 1  Người về đêm mưa", "Chuong 3  Tàn tro"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q\nfull:\n%s", want, text)
		}
	}
	// premise khong vao xuat (ban do sang tac, khong phai noi dung doc gia)
	if strings.Contains(text, "Câu chuyện ánh sáng và bóng tối.") {
		t.Errorf("premise must not appear in export:\n%s", text)
	}
}

// TestRun_PremiseNotExported Kiem tra dau cuoi: premise.md co ton tai cung khong vao xuat, ten sach giu lai (issue #27).
func TestRun_PremiseNotExported(t *testing.T) {
	s, _ := newTestStore(t, "Ánh Chớp", []int{1})
	if err := s.Outline.SavePremise("# Ánh Chớp\n## Đối tượng đọc giả\nBản vẽ sáng tác không nên xuất hiện."); err != nil {
		t.Fatalf("save premise: %v", err)
	}
	res, err := Run(context.Background(), Deps{Store: s}, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	data, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "Bản vẽ sáng tác không nên xuất hiện.") || strings.Contains(text, "đối tượng đọc giả") {
		t.Errorf("premise must not be exported, got:\n%s", text)
	}
	if !strings.Contains(text, "《Ánh Chớp》") {
		t.Errorf("book title should remain: %s", text)
	}
}

func TestRun_NoCompletedChapters(t *testing.T) {
	s, _ := newTestStore(t, "X", nil)
	_, err := Run(context.Background(), Deps{Store: s}, Options{})
	if err == nil {
		t.Fatal("expect error when no completed chapters")
	}
}

func TestRun_ExistingFile_NoOverwrite(t *testing.T) {
	s, dir := newTestStore(t, "X", []int{1})
	target := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(target, []byte("preexisting"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	_, err := Run(context.Background(), Deps{Store: s}, Options{OutPath: target})
	if err == nil {
		t.Fatal("expect error when target exists and !Overwrite")
	}
	if !strings.Contains(err.Error(), "da ton tai") {
		t.Errorf("unexpected error: %v", err)
	}

	// Them Overwrite nen thanh cong
	res, err := Run(context.Background(), Deps{Store: s}, Options{OutPath: target, Overwrite: true})
	if err != nil {
		t.Fatalf("Overwrite Run: %v", err)
	}
	if res.Path != target {
		t.Errorf("Path = %q want %q", res.Path, target)
	}
	data, _ := os.ReadFile(target)
	if string(data) == "preexisting" {
		t.Error("file not overwritten")
	}
}

func TestRun_RangeWithSkipped(t *testing.T) {
	s, _ := newTestStore(t, "X", []int{1, 2, 3})
	res, err := Run(context.Background(), Deps{Store: s}, Options{From: 2, To: 5, Overwrite: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Chapters != 2 {
		t.Errorf("Chapters = %d want 2 (only 2,3 completed in range 2..5)", res.Chapters)
	}
	if got := res.Skipped; len(got) != 2 || got[0] != 4 || got[1] != 5 {
		t.Errorf("Skipped = %v want [4 5]", got)
	}
}

func TestRun_FromGreaterThanTo(t *testing.T) {
	s, _ := newTestStore(t, "X", []int{1, 2})
	_, err := Run(context.Background(), Deps{Store: s}, Options{From: 5, To: 2})
	if err == nil {
		t.Fatal("expect error for invalid range")
	}
}

func TestRun_UnsupportedFormat(t *testing.T) {
	s, _ := newTestStore(t, "X", []int{1})
	_, err := Run(context.Background(), Deps{Store: s}, Options{Format: Format("pdf")})
	if err == nil {
		t.Fatal("expect error for unsupported format")
	}
}

func TestRun_FallbackFileNameWhenNovelNameEmpty(t *testing.T) {
	s, dir := newTestStore(t, "", []int{1})
	res, err := Run(context.Background(), Deps{Store: s}, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	wantBase := filepath.Base(dir) + ".txt"
	if filepath.Base(res.Path) != wantBase {
		t.Errorf("Path base = %q want %q (fallback to dir name)", filepath.Base(res.Path), wantBase)
	}
}

func TestInferFormat(t *testing.T) {
	cases := []struct {
		in      string
		want    Format
		wantErr bool
	}{
		{"", FormatTXT, false},
		{"book.txt", FormatTXT, false},
		{"book.TXT", FormatTXT, false},
		{"book.epub", FormatEPUB, false},
		{"book.EPUB", FormatEPUB, false},
		{"/abs/path/x.epub", FormatEPUB, false},
		{"book", FormatTXT, false}, // Khong hau to dung TXT
		{"book.dat", "", true},
		{"book.pdf", "", true},
	}
	for _, c := range cases {
		got, err := inferFormat(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("inferFormat(%q) want error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("inferFormat(%q): unexpected err: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("inferFormat(%q) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestRun_EPUB_FromExtension(t *testing.T) {
	s, dir := newTestStore(t, "Ánh Chớp", []int{1})
	if err := s.Outline.SavePremise("Ánh sáng và bóng tối."); err != nil {
		t.Fatalf("save premise: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{{Chapter: 1, Title: "Đêm mưa"}}); err != nil {
		t.Fatalf("save outline: %v", err)
	}

	target := filepath.Join(dir, "out.epub")
	res, err := Run(context.Background(), Deps{Store: s}, Options{OutPath: target})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Path != target {
		t.Errorf("Path = %q want %q", res.Path, target)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// EPUB la zip, 4 byte dau la header PK
	if len(data) < 4 || string(data[:2]) != "PK" {
		t.Errorf("output does not look like a zip: %x", data[:min(8, len(data))])
	}
}

func TestRun_DefaultPathFollowsFormat(t *testing.T) {
	s, dir := newTestStore(t, "Ánh Chớp", []int{1})
	res, err := Run(context.Background(), Deps{Store: s}, Options{Format: FormatEPUB})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := filepath.Join(dir, "Ánh Chớp.epub")
	if res.Path != want {
		t.Errorf("Path = %q want %q", res.Path, want)
	}
}

func TestRun_UnknownExtension(t *testing.T) {
	s, _ := newTestStore(t, "X", []int{1})
	_, err := Run(context.Background(), Deps{Store: s}, Options{OutPath: "/tmp/foo.dat"})
	if err == nil {
		t.Fatal("expect error for unknown extension")
	}
	if !strings.Contains(err.Error(), "phan mo rong") {
		t.Errorf("error should mention extension: %v", err)
	}
}

func TestSanitizeFileName(t *testing.T) {
	cases := map[string]string{
		"":                     "novel",
		"   ":                  "novel",
		"normal":               "normal",
		"a/b":                  "a_b",
		"a\\b":                 "a_b",
		"a:b*c?\"d<e>f|g\x00h": "a_b_c__d_e_f_g_h",
	}
	for in, want := range cases {
		if got := sanitizeFileName(in); got != want {
			t.Errorf("sanitizeFileName(%q) = %q want %q", in, got, want)
		}
	}
}

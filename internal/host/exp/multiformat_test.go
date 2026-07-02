package exp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// TestRun_MultiFormat_WritesThreeFiles kiểm tra Formats non-empty ghi ra đủ base.md/.txt/.epub.
func TestRun_MultiFormat_WritesThreeFiles(t *testing.T) {
	s, dir := newTestStore(t, "Ánh Chớp", []int{1, 2})
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "Mở đầu"},
		{Chapter: 2, Title: "Kết"},
	}); err != nil {
		t.Fatalf("save outline: %v", err)
	}

	base := filepath.Join(dir, "tac-pham")
	res, err := Run(context.Background(), Deps{Store: s}, Options{
		OutPath: base,
		Formats: DefaultFormats(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Outputs) != 3 {
		t.Fatalf("Outputs = %d, want 3", len(res.Outputs))
	}
	wantExt := map[Format]string{FormatMD: ".md", FormatTXT: ".txt", FormatEPUB: ".epub"}
	for _, o := range res.Outputs {
		if o.Path != base+wantExt[o.Format] {
			t.Errorf("Path = %q, want %q", o.Path, base+wantExt[o.Format])
		}
		if _, err := os.Stat(o.Path); err != nil {
			t.Errorf("file %s không tồn tại: %v", o.Path, err)
		}
	}
	if res.Path != res.Outputs[0].Path {
		t.Errorf("Path = %q, want first output %q", res.Path, res.Outputs[0].Path)
	}
}

// TestRun_MD_Content kiểm tra renderMD sinh tiêu đề ATX và nội dung chương.
func TestRun_MD_Content(t *testing.T) {
	s, dir := newTestStore(t, "Ánh Chớp", []int{1})
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "Người về đêm mưa"},
	}); err != nil {
		t.Fatalf("save outline: %v", err)
	}

	target := filepath.Join(dir, "sach.md")
	res, err := Run(context.Background(), Deps{Store: s}, Options{OutPath: target})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Outputs) != 1 || res.Outputs[0].Format != FormatMD {
		t.Fatalf("Outputs = %+v, want single MD", res.Outputs)
	}
	if res.Path != target {
		t.Errorf("Path = %q want %q", res.Path, target)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := string(data)
	for _, want := range []string{"# Ánh Chớp", "## Chuong 1 — Người về đêm mưa"} {
		if !strings.Contains(text, want) {
			t.Errorf("MD thiếu %q\n---\n%s", want, text)
		}
	}
}

// TestFormatsForPath kiểm tra hậu tố nhận biết -> 1 định dạng; không hậu tố -> 3 định dạng.
func TestFormatsForPath(t *testing.T) {
	cases := []struct {
		path string
		want []Format
	}{
		{"", DefaultFormats()},
		{"D:\\truyen\\tac-pham", DefaultFormats()},
		{"tac-pham.epub", []Format{FormatEPUB}},
		{"tac-pham.txt", []Format{FormatTXT}},
		{"tac-pham.md", []Format{FormatMD}},
	}
	for _, c := range cases {
		got := FormatsForPath(c.path)
		if len(got) != len(c.want) {
			t.Errorf("FormatsForPath(%q) = %v, want %v", c.path, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("FormatsForPath(%q)[%d] = %q, want %q", c.path, i, got[i], c.want[i])
			}
		}
	}
}

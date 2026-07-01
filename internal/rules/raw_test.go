package rules

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRawFileSources_ScansAllMarkdownInOrder kiểm tra tất cả .md trong thư mục đều được quét,
// trả về theo thứ tự từ điển tên tệp; tệp không phải .md bị bỏ qua; văn bản gốc được giữ nguyên.
func TestRawFileSources_ScansAllMarkdownInOrder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("b.md", "# Sở thích B")
	write("a.md", "# Sở thích A")
	write("ignore.txt", "not a rule")
	write("empty.md", "   ") // tệp trống nên bị bỏ qua

	srcs := RawFileSources(LoadOptions{HomeRulesDir: dir})
	if len(srcs) != 2 {
		t.Fatalf("nên quét được 2 nguồn a.md / b.md (.txt và tệp trống bị bỏ qua), got %d: %+v", len(srcs), srcs)
	}
	// thứ tự từ điển: a trước b sau
	if srcs[0].Label != "global:a.md" || srcs[1].Label != "global:b.md" {
		t.Errorf("nên trả về theo thứ tự từ điển, got %q, %q", srcs[0].Label, srcs[1].Label)
	}
	for _, s := range srcs {
		if s.Kind != SourceGlobal {
			t.Errorf("nguồn HomeRulesDir nên là SourceGlobal, got %v", s.Kind)
		}
	}
}

// TestRawFileSources_DirMissing kiểm tra khi thư mục không tồn tại thì bỏ qua lặng lẽ (trả về nil).
func TestRawFileSources_DirMissing(t *testing.T) {
	srcs := RawFileSources(LoadOptions{HomeRulesDir: filepath.Join(t.TempDir(), "nope")})
	if len(srcs) != 0 {
		t.Errorf("thư mục thiếu nên trả về 0 nguồn, got %d", len(srcs))
	}
	if len(RawFileSources(LoadOptions{})) != 0 {
		t.Error("LoadOptions rỗng nên trả về 0 nguồn")
	}
}

// TestRawFileSources_IgnoresHiddenAndSubdirs khóa cứng: tệp ẩn/tạm của trình soạn thảo (bắt đầu bằng .) bị bỏ qua,
// không đệ quy vào thư mục con — ngăn nội dung nhị phân tệp bẩn bị tiêm vào LLM như văn bản sở thích.
func TestRawFileSources_IgnoresHiddenAndSubdirs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rules")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real.md"), []byte("# real"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dirty := range []string{"._real.md", ".#lock.md", ".hidden.md"} {
		if err := os.WriteFile(filepath.Join(dir, dirty), []byte("\x00binary garbage\x00"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "nested.md"), []byte("# nested"), 0o644); err != nil {
		t.Fatal(err)
	}

	srcs := RawFileSources(LoadOptions{HomeRulesDir: dir})
	if len(srcs) != 1 || srcs[0].Label != "global:real.md" {
		t.Fatalf("nên chỉ quét được real.md (bỏ qua ẩn/bẩn/thư mục con), got %+v", srcs)
	}
}

// TestRawFileSources_GlobalThenProject kiểm tra nguồn toàn cục đứng trước, nguồn dự án đứng sau.
func TestRawFileSources_GlobalThenProject(t *testing.T) {
	base := t.TempDir()
	global := filepath.Join(base, "global")
	project := filepath.Join(base, "project")
	for _, d := range []string{global, project} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(global, "g.md"), []byte("# Toàn cục"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "p.md"), []byte("# Sách này"), 0o644); err != nil {
		t.Fatal(err)
	}

	srcs := RawFileSources(LoadOptions{HomeRulesDir: global, ProjectRulesDir: project})
	if len(srcs) != 2 || srcs[0].Kind != SourceGlobal || srcs[1].Kind != SourceProject {
		t.Fatalf("nên toàn cục trước rồi dự án sau, got %+v", srcs)
	}
}

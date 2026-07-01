package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnsureRulesDirAt kiểm tra chuẩn bị thư mục + README.txt: ghi hướng dẫn, luôn ghi đè bằng mẫu mới nhất,
// và README.txt (không phải .md) không bị quét thành quy tắc.
func TestEnsureRulesDirAt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rules")
	if err := ensureRulesDirAt(dir); err != nil {
		t.Fatal(err)
	}
	readme := filepath.Join(dir, "README.txt")
	data, err := os.ReadFile(readme)
	if err != nil {
		t.Fatalf("README.txt should be written: %v", err)
	}
	// Sau khi bỏ YAML, hướng dẫn chuyển sang "ngôn ngữ thường + tự động chuẩn hóa", không dạy front matter nữa.
	if !strings.Contains(string(data), "chuẩn hóa") {
		t.Errorf("README.txt nên nói rõ ngôn ngữ tự nhiên sẽ được chuẩn hóa, got %q", data)
	}
	if strings.Contains(string(data), "front matter") {
		t.Errorf("README.txt không nên dạy YAML front matter nữa, got %q", data)
	}

	// Luôn ghi đè bằng mẫu mới nhất: nội dung cũ lỗi thời được làm mới khi ensure lại
	if err := os.WriteFile(readme, []byte("Nội dung cũ lỗi thời"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureRulesDirAt(dir); err != nil {
		t.Fatal(err)
	}
	if again, _ := os.ReadFile(readme); string(again) != homeRulesReadme {
		t.Errorf("README.txt should be refreshed to latest template, got %q", again)
	}

	// README.txt không bị xem là quy tắc (quét chỉ nhận .md)
	if srcs := RawFileSources(LoadOptions{HomeRulesDir: dir}); len(srcs) != 0 {
		t.Errorf("README.txt must not be scanned as a rule, got %d sources", len(srcs))
	}
}

// TestDefaultProjectRulesDir khóa cứng thư mục quy tắc cấp dự án đối xứng toàn cục: ./.ainovel/rules/.
func TestDefaultProjectRulesDir(t *testing.T) {
	proj := filepath.Join("/tmp", "demo-book")
	want := filepath.Join(proj, ".ainovel", "rules")
	if got := DefaultProjectRulesDir(proj); got != want {
		t.Errorf("DefaultProjectRulesDir=%q, want %q", got, want)
	}
	if got := DefaultProjectRulesDir(""); got != "" {
		t.Errorf("gốc dự án rỗng nên trả về chuỗi rỗng, got %q", got)
	}
}

// TestDefaultOptions_ScansProjectRulesFromDotAinovel kiểm tra end-to-end:
// DefaultOptions nạp ./.ainovel/rules/ trong cwd vào nguồn SourceProject.
func TestDefaultOptions_ScansProjectRulesFromDotAinovel(t *testing.T) {
	proj := t.TempDir()
	t.Chdir(proj)
	rulesDir := filepath.Join(proj, ".ainovel", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "book.md"), []byte("# Sở thích sách này\nMỗi chương 4000 chữ"), 0o644); err != nil {
		t.Fatal(err)
	}

	srcs := RawFileSources(DefaultOptions())
	var got *RawSource
	for i := range srcs {
		if srcs[i].Kind == SourceProject {
			got = &srcs[i]
		}
	}
	if got == nil {
		t.Fatalf("nên quét được nguồn quy tắc dự án từ ./.ainovel/rules/, got %+v", srcs)
	}
	if !strings.Contains(got.Text, "Sở thích sách này") {
		t.Errorf("văn bản gốc quy tắc dự án nên được trả về nguyên vẹn, got %q", got.Text)
	}
	if got.Label != "project:book.md" {
		t.Errorf("nhãn nguồn nên là project:book.md, got %q", got.Label)
	}
}

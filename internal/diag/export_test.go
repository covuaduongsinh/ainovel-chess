package diag

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/store"
)

// sentinel là một đoạn "nội dung tiểu thuyết" tuyệt đối không được xuất hiện trong export.
const sentinel = "dem-tuyet-nhan-vat-chinh-vach-tran-am-muu-kinh-thien-cua-phan-dien-day-la-noi-dung-bi-mat"

// writeSession ghi một số tin nhắn theo định dạng sessions/*.jsonl vào thư mục output tạm.
func writeSession(t *testing.T, rel string, msgs []agentcore.Message) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "meta", "sessions", rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var b strings.Builder
	for _, m := range msgs {
		data, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return dir
}

func commitCall(chapterRaw string) agentcore.Message {
	args := json.RawMessage(`{"chapter":` + chapterRaw + `,"content":"` + sentinel + sentinel + `"}`)
	return agentcore.Message{
		Role:    agentcore.RoleAssistant,
		Content: []agentcore.ContentBlock{agentcore.ToolCallBlock(agentcore.ToolCall{Name: "commit_chapter", Args: args})},
	}
}

func errResult(msg string) agentcore.Message {
	return agentcore.Message{
		Role:     agentcore.RoleTool,
		Content:  []agentcore.ContentBlock{agentcore.TextBlock(msg)},
		Metadata: map[string]any{"is_error": true},
	}
}

// TestExport_DeathLoopShape tái hiện end-to-end issue #34: model stringify hóa chapter của
// commit_chapter gây vòng lặp xác thực. Kiểm tra export có phát hiện được, và nội dung tiểu thuyết không lọt ra.
func TestExport_DeathLoopShape(t *testing.T) {
	var msgs []agentcore.Message
	// Một đoạn văn bản lập kế hoạch coordinator thuần túy (<4KB, bỏ qua session_compact), phải bị khử nhận dạng.
	msgs = append(msgs, agentcore.Message{
		Role:    agentcore.RoleAssistant,
		Content: []agentcore.ContentBlock{agentcore.TextBlock(sentinel)},
	})
	// 14 vòng commit_chapter(chapter:"7") + InputValidationError.
	for range 14 {
		msgs = append(msgs, commitCall(`"7"`))
		msgs = append(msgs, errResult("InputValidationError: chapter must be int"))
	}

	dir := writeSession(t, "coordinator.jsonl", msgs)
	s := store.NewStore(dir)
	rep, rc := Diagnose(s)
	out := string(RenderExport(rep, rc))

	if strings.Contains(out, sentinel) {
		t.Fatalf("nội dung tiểu thuyết lọt ra! Export chứa sentinel:\n%s", out)
	}
	if !strings.Contains(out, `chapter: "7"`) {
		t.Errorf("thiếu tín hiệu lỗi kiểu chapter: \"7\" (nguyên nhân gốc #34)\n%s", out)
	}
	if !strings.Contains(out, "InputValidationError") {
		t.Errorf("chuỗi lỗi chưa được giữ lại\n%s", out)
	}
	if !strings.Contains(out, "×14") {
		t.Errorf("tổng hợp lặp chưa liệt kê ×14\n%s", out)
	}
	// Phase 2: phát hiện runtime phải phân loại vòng lặp này thành RepeatedToolError mức critical.
	if !strings.Contains(out, "Cong cu lien tuc bao cung mot loi") {
		t.Errorf("phat hien runtime chua tao ra RepeatedToolError\n%s", out)
	}
	if !strings.Contains(out, "[critical]") {
		t.Errorf("14 lần lặp phải được nâng lên critical\n%s", out)
	}
}

// TestExport_NumberVsStringArg chứng minh projection vô hướng và chuỗi có thể phân biệt kiểu:
// chapter:7 (số) giữ nguyên là 7, chapter:"7" (chuỗi) giữ nguyên là "7".
func TestExport_NumberVsStringArg(t *testing.T) {
	intDir := writeSession(t, "coordinator.jsonl", []agentcore.Message{commitCall(`7`)})
	si := store.NewStore(intDir)
	repInt, rcInt := Diagnose(si)
	outInt := string(RenderExport(repInt, rcInt))
	if !strings.Contains(outInt, "chapter: 7") || strings.Contains(outInt, `chapter: "7"`) {
		t.Errorf("tham số số phải render thành chapter: 7 (không có dấu ngoặc kép)\n%s", outInt)
	}
}

// TestProjectValue_ProseArgRedacted bảo vệ ranh giới khử nhận dạng: giá trị ngắn kiểu identifier giữ lại,
// giá trị ngắn chứa tiếng Trung/khoảng trắng (như dispatch task, chapter title) đều bị khử nhận dạng.
func TestProjectValue_ProseArgRedacted(t *testing.T) {
	keep := map[string]string{
		`"7"`:       `"7"`,       // số bị stringify (#34 signal)
		`"premise"`: `"premise"`, // enum
		`"writer"`:  `"writer"`,  // tên vai trò
		`7`:         `7`,         // vô hướng số
		`true`:      `true`,      // vô hướng bool
	}
	for in, want := range keep {
		if got := projectValue([]byte(in)); got != want {
			t.Errorf("phải giữ lại %s: got %q want %q", in, got, want)
		}
	}
	// Chua tieng Viet / khoang trang → phai khu nhan dang, va khong chua van ban goc.
	prose := []string{`"Chuong 7 Su that dem tuyet"`, `"Dem tuyet ke giet nguoi"`, `"Nhan vat chinh vach tran am muu"`}
	for _, in := range prose {
		got := projectValue([]byte(in))
		if !strings.HasPrefix(got, "<redacted") {
			t.Errorf("gia tri ngan co khoang trang phai bi khu nhan dang: %s -> %q", in, got)
		}
		if strings.Contains(got, "dem tuyet") || strings.Contains(got, "nhan vat") {
			t.Errorf("sau khi khu nhan dang van chua noi dung goc: %s -> %q", in, got)
		}
	}
}

// TestWriteExport_WritesFile chứng minh đường dẫn hàm thuần túy: không phụ thuộc TUI, ghi vào đường dẫn tương đối cố định.
func TestWriteExport_WritesFile(t *testing.T) {
	dir := writeSession(t, "coordinator.jsonl", []agentcore.Message{commitCall(`"7"`), errResult("boom")})
	s := store.NewStore(dir)

	rep, rc := Diagnose(s)
	path, err := WriteExport(s, rep, rc)
	if err != nil {
		t.Fatalf("WriteExport: %v", err)
	}
	if want := filepath.Join(dir, filepath.FromSlash(ExportRelPath)); path != want {
		t.Errorf("đường dẫn sai: got %s want %s", path, want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc lại: %v", err)
	}
	if !strings.Contains(string(data), "diag-export") {
		t.Errorf("nội dung file bất thường\n%s", data)
	}
	if strings.Contains(string(data), sentinel) {
		t.Errorf("file đã ghi chứa nội dung tiểu thuyết")
	}
}

// TestRedactMessage_DupSha chứng minh cùng một đoạn văn bản xuất hiện nhiều lần tạo ra cùng sha (tín hiệu vòng lặp).
func TestRedactMessage_DupSha(t *testing.T) {
	a := redactMessage("coordinator", agentcore.Message{
		Role:    agentcore.RoleAssistant,
		Content: []agentcore.ContentBlock{agentcore.TextBlock(sentinel)},
	})
	b := redactMessage("coordinator", agentcore.Message{
		Role:    agentcore.RoleAssistant,
		Content: []agentcore.ContentBlock{agentcore.TextBlock(sentinel)},
	})
	if a.TextSha == "" || a.TextSha != b.TextSha {
		t.Errorf("cùng nội dung phải cho cùng sha: %q vs %q", a.TextSha, b.TextSha)
	}
	if a.Redacted != 1 {
		t.Errorf("phải khử nhận dạng 1 khối văn bản, got %d", a.Redacted)
	}
}

package imp

import (
	"context"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
	"github.com/voocel/ainovel-cli/internal/tools"
)

const validAnalyzerEnvelope = `=== SUMMARY ===
Sau khi nhận tin tố cáo ẩn danh, Lâm Vãn phát hiện tại kho lưu trữ rằng tất cả nạn nhân mất tích đều họ Trần, và tìm được địa chỉ tổ trạch của gia tộc họ Trần bên cạnh vật tế lễ.

=== CHARACTERS ===
["Lâm Vãn","quản lý kho lưu trữ"]

=== KEY_EVENTS ===
["Lâm Vãn nhận được thư ẩn danh","phát hiện điểm chung họ Trần tại kho lưu trữ","tìm được địa chỉ tổ trạch"]

=== TIMELINE ===
[
  {"time":"chiều tối","event":"Lâm Vãn nhận thư ẩn danh","characters":["Lâm Vãn"]},
  {"time":"ngày hôm sau","event":"thăm kho lưu trữ","characters":["Lâm Vãn","quản lý kho lưu trữ"]}
]

=== FORESHADOW ===
[
  {"id":"hk-chen-family","action":"plant","description":"mối liên hệ giữa gia tộc họ Trần và vụ mất tích liên hoàn"}
]

=== RELATIONSHIPS ===
[]

=== STATE_CHANGES ===
[
  {"entity":"Lâm Vãn","field":"location","old_value":"tòa soạn","new_value":"kho lưu trữ","reason":"truy theo dấu vết"}
]

=== HOOK_TYPE ===
mystery

=== DOMINANT_STRAND ===
quest
`

func TestParseAnalyzer_Valid(t *testing.T) {
	got, err := parseAnalyzerOutput(validAnalyzerEnvelope)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.HookType != "mystery" || got.DominantStrand != "quest" {
		t.Errorf("hook/strand: %+v", got)
	}
	if len(got.Characters) != 2 || len(got.KeyEvents) != 3 {
		t.Errorf("counts: %+v", got)
	}
	if len(got.ForeshadowUpdates) != 1 || got.ForeshadowUpdates[0].ID != "hk-chen-family" {
		t.Errorf("foreshadow: %+v", got.ForeshadowUpdates)
	}
	if len(got.TimelineEvents) != 2 {
		t.Errorf("timeline: %+v", got.TimelineEvents)
	}
	if len(got.RelationshipChanges) != 0 {
		t.Errorf("relationships should be empty: %+v", got.RelationshipChanges)
	}
	if len(got.StateChanges) != 1 || got.StateChanges[0].Field != "location" {
		t.Errorf("state changes: %+v", got.StateChanges)
	}
}

func TestParseAnalyzer_RejectsInvalidHookType(t *testing.T) {
	bad := strings.Replace(validAnalyzerEnvelope, "mystery", "weird", 1)
	if _, err := parseAnalyzerOutput(bad); err == nil ||
		!strings.Contains(err.Error(), "invalid hook_type") {
		t.Fatalf("want hook_type error, got %v", err)
	}
}

func TestParseAnalyzer_RejectsPlantWithoutDescription(t *testing.T) {
	bad := strings.Replace(
		validAnalyzerEnvelope,
		`{"id":"hk-chen-family","action":"plant","description":"mối liên hệ giữa gia tộc họ Trần và vụ mất tích liên hoàn"}`,
		`{"id":"hk-chen-family","action":"plant"}`,
		1,
	)
	if _, err := parseAnalyzerOutput(bad); err == nil ||
		!strings.Contains(err.Error(), "requires description") {
		t.Fatalf("want plant-without-desc error, got %v", err)
	}
}

func TestParseAnalyzer_MissingRequiredTag(t *testing.T) {
	bad := strings.Replace(validAnalyzerEnvelope, "=== HOOK_TYPE ===\nmystery\n", "", 1)
	if _, err := parseAnalyzerOutput(bad); err == nil ||
		!strings.Contains(err.Error(), "missing required tags") {
		t.Fatalf("want missing-tag error, got %v", err)
	}
}

func TestPersistChapter_FullPipeline(t *testing.T) {
	dir := t.TempDir()
	st := store.NewStore(dir)
	if err := st.Progress.Init("ch-test", 2); err != nil {
		t.Fatal(err)
	}

	// Chuan bi foundation: su dung ReverseFoundation+PersistFoundation de mo phong Phase 2 da hoan tat
	fr := mustParse(t, validEnvelope, 2)
	if err := PersistFoundation(context.Background(), st, domain.PlanningTierShort, fr); err != nil {
		t.Fatal(err)
	}

	a, err := parseAnalyzerOutput(validAnalyzerEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	commitTool := tools.NewCommitChapterTool(st)
	body := "Lâm Vãn mở phong bì ẩn danh, phát hiện một hàng chữ nguệch ngoạc...\n\n(nội dung lược bỏ, >500 ký tự để qua kiểm tra LoadChapterContent)"
	body = strings.Repeat(body, 10) // Gop du so ky tu

	if err := PersistChapter(context.Background(), st, commitTool, 1, "Lần Đầu Gặp Gỡ", body, a); err != nil {
		t.Fatalf("PersistChapter: %v", err)
	}

	prog, _ := st.Progress.Load()
	if len(prog.CompletedChapters) != 1 || prog.CompletedChapters[0] != 1 {
		t.Errorf("completed chapters wrong: %+v", prog.CompletedChapters)
	}

	hooks, err := st.World.LoadForeshadowLedger()
	if err != nil {
		t.Fatalf("load hooks: %v", err)
	}
	if len(hooks) != 1 || hooks[0].ID != "hk-chen-family" {
		t.Errorf("foreshadow not persisted: %+v", hooks)
	}

	// De xuat lan hai cung chuong phai la idempotent (commit_chapter.IsChapterCompleted ngan lai)
	if err := PersistChapter(context.Background(), st, commitTool, 1, "Lần Đầu Gặp Gỡ", body, a); err != nil {
		t.Errorf("re-import should be idempotent, got: %v", err)
	}
	prog2, _ := st.Progress.Load()
	if len(prog2.CompletedChapters) != 1 {
		t.Errorf("re-import duplicated completion: %+v", prog2.CompletedChapters)
	}
}

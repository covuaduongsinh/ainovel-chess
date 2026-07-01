package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

func TestCollectReadsStyleUsageAndToolCalls(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	progress := &domain.Progress{
		Phase:             domain.PhaseWriting,
		Flow:              domain.FlowWriting,
		TotalChapters:     5,
		CompletedChapters: []int{3, 1, 5, 2, 4},
	}
	if err := s.Progress.Save(progress); err != nil {
		t.Fatalf("save progress: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "第一章 起风"},
		{Chapter: 2, Title: "破局"},
		{Chapter: 3, Title: "第三章 入局"},
		{Chapter: 4, Title: "追问"},
		{Chapter: 5, Title: "第五章 回声"},
	}); err != nil {
		t.Fatalf("save outline: %v", err)
	}
	if err := s.Characters.Save([]domain.Character{{Name: "Lâm Mặc", Aliases: []string{"Dược đồng"}}}); err != nil {
		t.Fatalf("save characters: %v", err)
	}
	for ch := 1; ch <= 5; ch++ {
		text := "# Tiêu đề\n\nKhông phải gió đã ngừng, mà là tất cả mọi người đều nín thở. Sau một khoảnh khắc im lặng, Lâm Mặc siết chặt túi thuốc."
		if err := s.Drafts.SaveFinalChapter(ch, text); err != nil {
			t.Fatalf("save chapter %d: %v", ch, err)
		}
	}
	if err := s.Usage.Save(domain.UsageState{
		Overall:      domain.AgentUsageTotals{Input: 100, Output: 40, Cost: 0.12},
		MissingUsage: 1,
	}); err != nil {
		t.Fatalf("save usage: %v", err)
	}
	writeSessionLine(t, dir, "meta/sessions/coordinator.jsonl", agentcore.Message{
		Role: agentcore.RoleAssistant,
		Content: []agentcore.ContentBlock{
			agentcore.ToolCallBlock(agentcore.ToolCall{Name: "dispatch"}),
			agentcore.ToolCallBlock(agentcore.ToolCall{Name: "novel_context"}),
		},
	})

	col := Collect(dir, nil)
	if len(col.LoadErrors) != 0 {
		t.Fatalf("không được có lỗi đọc: %v", col.LoadErrors)
	}
	if col.Style.Status != "ok" || col.Style.Stats == nil {
		t.Fatalf("stylestat phải tính được, nhận được status=%s stats=%v", col.Style.Status, col.Style.Stats)
	}
	if col.Style.Stats.TitleFormats == nil {
		t.Fatal("stylestat phải phát hiện tiêu đề dùng lẫn lộn")
	}
	if !col.Usage.UsageRecorded || col.Usage.Input != 100 || col.Usage.Output != 40 || col.Usage.CostUSD != 0.12 {
		t.Fatalf("đọc usage không chính xác: %+v", col.Usage)
	}
	if col.ToolCalls != 2 {
		t.Fatalf("tool calls = %d want 2", col.ToolCalls)
	}
}

func TestCollectStyleInsufficientSample(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Progress.Save(&domain.Progress{CompletedChapters: []int{1}}); err != nil {
		t.Fatalf("save progress: %v", err)
	}
	if err := s.Drafts.SaveFinalChapter(1, "Chỉ có một chương"); err != nil {
		t.Fatalf("save chapter: %v", err)
	}
	col := Collect(dir, nil)
	if col.Style.Status != "insufficient_sample" {
		t.Fatalf("mẫu một chương phải là insufficient_sample, nhận được %s", col.Style.Status)
	}
}

func TestCollectFailsLoudWhenCompletedChapterMissing(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Progress.Save(&domain.Progress{CompletedChapters: []int{1}}); err != nil {
		t.Fatalf("save progress: %v", err)
	}
	col := Collect(dir, nil)
	if !containsString(col.LoadErrors, "progress đánh dấu đã hoàn thành nhưng bản thảo cuối rỗng") {
		t.Fatalf("thiếu bản thảo cuối phải vào LoadErrors, thực tế %v", col.LoadErrors)
	}
}

func TestChapterTitleUsesLayeredEntryChapter(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Outline.SaveLayeredOutline([]domain.VolumeOutline{
		{
			Index: 1,
			Arcs: []domain.ArcOutline{
				{Index: 1}, // arc chưa mở rộng không được làm vị trí chương tiếp theo bị trôi
				{
					Index: 2,
					Chapters: []domain.OutlineEntry{
						{Chapter: 7, Title: "Chương 7 Tiêu đề thật"},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save layered outline: %v", err)
	}

	got := chapterTitle(s, 7, "# Tiêu đề dự phòng từ nội dung\n\nNội dung", func(string, error) {})
	if got != "Chương 7 Tiêu đề thật" {
		t.Fatalf("phải khớp tiêu đề phân tầng theo entry.Chapter, nhận được %q", got)
	}
}

func containsString(items []string, sub string) bool {
	for _, item := range items {
		if strings.Contains(item, sub) {
			return true
		}
	}
	return false
}

func writeSessionLine(t *testing.T, root, rel string, msg agentcore.Message) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir session: %v", err)
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}
}

package adapt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

// fakeLLM trả JSON cố định theo marker trong system prompt.
type fakeLLM struct{ calls int }

func (f *fakeLLM) Generate(_ context.Context, messages []agentcore.Message, _ []agentcore.ToolSpec, _ ...agentcore.CallOption) (*agentcore.LLMResponse, error) {
	f.calls++
	sys := messages[0].TextContent()
	var payload string
	switch {
	case strings.Contains(sys, "CONCEPT"):
		payload = `{"style":{"overall":"tối giản","style_tokens":["cinematic"]},"locations":[{"name":"Tửu lâu","image_prompt":"a tavern"}]}`
	case strings.Contains(sys, "CHARACTER"):
		payload = `{"name":"","appearance":"cao gầy","key_art_prompt":"a tall lean man","negative_prompt":"blurry"}`
	case strings.Contains(sys, "PROP"):
		payload = `{"props":[{"name":"Kiếm cổ","image_prompt":"an ancient sword"}]}`
	case strings.Contains(sys, "CONSISTENCY"):
		payload = `{"style_tokens":["cinematic"],"characters":[{"name":"Lâm","canonical_prompt":"a tall lean man"}]}`
	case strings.Contains(sys, "SCREENPLAY"):
		payload = `{"chapter":0,"title":"x","markdown":"## CẢNH 1\n\nNỘI. – TỬU LÂU – ĐÊM\n\nLâm bước vào.\n"}`
	case strings.Contains(sys, "STORYBOARD"):
		payload = `{"scenes":[{"scene_id":"1","heading":"NỘI. – TỬU LÂU – ĐÊM","summary":"Lâm vào quán","shots":[{"shot_id":"1","description":"Lâm bước vào","camera_angle":"trung","movement":"tĩnh","duration_sec":4,"image_prompt":"a tall lean man enters a tavern","video_prompt":"slow push in","negative_prompt":"blurry"}]}]}`
	default:
		payload = `{}`
	}
	text := "<output>\n" + payload + "\n</output>"
	return &agentcore.LLMResponse{
		Message: agentcore.Message{
			Role:    agentcore.RoleAssistant,
			Content: []agentcore.ContentBlock{agentcore.TextBlock(text)},
		},
	}, nil
}

// tolerantTempDir tạo thư mục tạm với dọn dẹp khoan dung (retry), tránh lỗi
// "directory not empty" của RemoveAll trên Windows khi file vừa được fsync/rename.
func tolerantTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "adapt-test-*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() {
		for i := 0; i < 10; i++ {
			if err := os.RemoveAll(dir); err == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
	return dir
}

func newTestStore(t *testing.T, completed []int) (*store.Store, string) {
	t.Helper()
	dir := tolerantTempDir(t)
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	if err := s.Progress.Init("Ánh Chớp", len(completed)); err != nil {
		t.Fatalf("init progress: %v", err)
	}
	for _, ch := range completed {
		if err := s.Drafts.SaveFinalChapter(ch, fmt.Sprintf("Nội dung chương %d. Lâm bước vào tửu lâu.", ch)); err != nil {
			t.Fatalf("save chapter: %v", err)
		}
		if err := s.Progress.StartChapter(ch); err != nil {
			t.Fatalf("start chapter: %v", err)
		}
		if err := s.Progress.MarkChapterComplete(ch, 8, "cliff", "main"); err != nil {
			t.Fatalf("mark complete: %v", err)
		}
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "Mở đầu", Scenes: []string{"Vào quán"}},
		{Chapter: 2, Title: "Kết", Scenes: []string{"Rời đi"}},
	}); err != nil {
		t.Fatalf("save outline: %v", err)
	}
	if err := s.Characters.Save([]domain.Character{
		{Name: "Lâm", Role: "chính", Description: "thiếu niên", Tier: "core"},
		{Name: "Phụ", Role: "phụ", Tier: "decorative"},
	}); err != nil {
		t.Fatalf("save characters: %v", err)
	}
	return s, dir
}

func testPrompts() Prompts {
	return Prompts{
		Concept:     "CONCEPT",
		Character:   "CHARACTER",
		Prop:        "PROP",
		Consistency: "CONSISTENCY",
		Screenplay:  "SCREENPLAY",
		Storyboard:  "STORYBOARD",
	}
}

func drain(ch <-chan Event) []Event {
	var out []Event
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}

func lastStage(events []Event) Stage {
	if len(events) == 0 {
		return ""
	}
	return events[len(events)-1].Stage
}

func TestRun_All_WritesExpectedFiles(t *testing.T) {
	s, dir := newTestStore(t, []int{1, 2})
	ch, err := Run(context.Background(), Deps{Store: s, LLM: &fakeLLM{}, Prompts: testPrompts()}, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	events := drain(ch)
	if lastStage(events) != StageDone {
		t.Fatalf("stage cuối = %q, muốn done; events=%+v", lastStage(events), events)
	}
	want := []string{
		"video/concept/art-direction.json",
		"video/concept/art-direction.md",
		"video/characters/lâm.json",
		"video/characters/characters.json",
		"video/props/props.json",
		"video/consistency-bible.json",
		"video/screenplay/01.md",
		"video/screenplay/02.md",
		"video/storyboard/01.json",
		"video/storyboard/01.md",
		"video/animation/01.md",
		"video/prompts/image-prompts.md",
		"video/prompts/video-prompts.md",
	}
	for _, rel := range want {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("thiếu file %s: %v", rel, err)
		}
	}
	// Nhân vật decorative không được thiết kế.
	if _, err := os.Stat(filepath.Join(dir, "video/characters/phụ.json")); err == nil {
		t.Errorf("nhân vật decorative không nên được thiết kế")
	}
}

func TestRun_SingleProduct_Screenplay_Range(t *testing.T) {
	s, dir := newTestStore(t, []int{1, 2})
	ch, err := Run(context.Background(), Deps{Store: s, LLM: &fakeLLM{}, Prompts: testPrompts()},
		Options{Products: []Product{ProductScreenplay}, To: 1})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	drain(ch)
	if _, err := os.Stat(filepath.Join(dir, "video/screenplay/01.md")); err != nil {
		t.Errorf("thiếu screenplay chương 1: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "video/screenplay/02.md")); err == nil {
		t.Errorf("chương 2 ngoài phạm vi to=1, không nên tạo")
	}
}

func TestRun_NoCompletedChapters_Errors(t *testing.T) {
	dir := tolerantTempDir(t)
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := s.Progress.Init("Trống", 0); err != nil {
		t.Fatalf("init progress: %v", err)
	}
	ch, err := Run(context.Background(), Deps{Store: s, LLM: &fakeLLM{}, Prompts: testPrompts()}, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	events := drain(ch)
	if lastStage(events) != StageError {
		t.Fatalf("muốn stage error khi chưa có chương, nhận %q", lastStage(events))
	}
}

func TestRun_DerivedWithoutStoryboard_SoftGuard(t *testing.T) {
	s, dir := newTestStore(t, []int{1})
	ch, err := Run(context.Background(), Deps{Store: s, LLM: &fakeLLM{}, Prompts: testPrompts()},
		Options{Products: []Product{ProductImagePrompt}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	events := drain(ch)
	if lastStage(events) != StageDone {
		t.Fatalf("guard mềm: vẫn nên kết thúc done, nhận %q", lastStage(events))
	}
	if _, err := os.Stat(filepath.Join(dir, "video/prompts/image-prompts.md")); err == nil {
		t.Errorf("không có storyboard thì không nên tạo bảng prompt ảnh")
	}
}

func TestRun_Overwrite(t *testing.T) {
	s, dir := newTestStore(t, []int{1})
	deps := Deps{Store: s, LLM: &fakeLLM{}, Prompts: testPrompts()}

	if ch, err := Run(context.Background(), deps, Options{Products: []Product{ProductConcept}}); err != nil {
		t.Fatalf("Run 1: %v", err)
	} else {
		drain(ch)
	}
	target := filepath.Join(dir, "video/concept/art-direction.json")
	if err := os.WriteFile(target, []byte("SENTINEL"), 0o644); err != nil {
		t.Fatalf("ghi sentinel: %v", err)
	}
	// Không overwrite → giữ nguyên sentinel.
	if ch, err := Run(context.Background(), deps, Options{Products: []Product{ProductConcept}}); err != nil {
		t.Fatalf("Run 2: %v", err)
	} else {
		drain(ch)
	}
	if data, _ := os.ReadFile(target); string(data) != "SENTINEL" {
		t.Errorf("không overwrite nhưng file bị ghi đè")
	}
	// Overwrite=true → ghi lại.
	if ch, err := Run(context.Background(), deps, Options{Products: []Product{ProductConcept}, Overwrite: true}); err != nil {
		t.Fatalf("Run 3: %v", err)
	} else {
		drain(ch)
	}
	if data, _ := os.ReadFile(target); string(data) == "SENTINEL" {
		t.Errorf("overwrite=true nhưng file không được ghi lại")
	}
}

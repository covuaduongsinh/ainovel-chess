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

func TestRun_ByChapter_WritesBundleAndKeepsFlatLayout(t *testing.T) {
	s, dir := newTestStore(t, []int{1, 2})
	// Options{} rỗng → mặc định GroupByChapter.
	ch, err := Run(context.Background(), Deps{Store: s, LLM: &fakeLLM{}, Prompts: testPrompts()}, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if lastStage(drain(ch)) != StageDone {
		t.Fatalf("muốn stage done")
	}
	// Đóng gói lồng theo tập/chương (sách không phân tầng → tap-01).
	bundle := []string{
		"video/tap-01/chuong-001/kich-ban.md",
		"video/tap-01/chuong-001/phan-canh.json",
		"video/tap-01/chuong-001/phan-canh.md",
		"video/tap-01/chuong-001/animation.md",
		"video/tap-01/chuong-001/prompt-anh.md",
		"video/tap-01/chuong-001/prompt-video.md",
		"video/tap-01/chuong-002/kich-ban.md",
		"video/tap-01/_tap-01.md",
	}
	for _, rel := range bundle {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("thiếu file đóng gói %s: %v", rel, err)
		}
	}
	// Vẫn giữ bố cục theo loại cũ.
	flat := []string{"video/screenplay/01.md", "video/storyboard/01.json", "video/animation/01.md", "video/prompts/image-prompts.md"}
	for _, rel := range flat {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("thiếu file theo loại %s: %v", rel, err)
		}
	}
}

func TestRun_ByChapter_CompletesChapterBeforeNext(t *testing.T) {
	s, _ := newTestStore(t, []int{1, 2})
	ch, err := Run(context.Background(), Deps{Store: s, LLM: &fakeLLM{}, Prompts: testPrompts()},
		Options{Products: []Product{ProductScreenplay, ProductStoryboard}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	events := drain(ch)
	lastCh1, firstCh2 := -1, -1
	for i, ev := range events {
		if ev.Stage != StageScreenplay && ev.Stage != StageStoryboard {
			continue
		}
		if strings.Contains(ev.Message, "Chương 1") {
			lastCh1 = i
		}
		if strings.Contains(ev.Message, "Chương 2") && firstCh2 < 0 {
			firstCh2 = i
		}
	}
	if lastCh1 < 0 || firstCh2 < 0 {
		t.Fatalf("không thấy sự kiện của cả hai chương: lastCh1=%d firstCh2=%d", lastCh1, firstCh2)
	}
	if lastCh1 > firstCh2 {
		t.Errorf("chương 1 chưa làm trọn đã sang chương 2 (lastCh1=%d > firstCh2=%d)", lastCh1, firstCh2)
	}
}

func TestRun_ByProduct_NoBundle(t *testing.T) {
	s, dir := newTestStore(t, []int{1, 2})
	ch, err := Run(context.Background(), Deps{Store: s, LLM: &fakeLLM{}, Prompts: testPrompts()},
		Options{Grouping: GroupByProduct})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	drain(ch)
	if _, err := os.Stat(filepath.Join(dir, "video/tap-01")); err == nil {
		t.Errorf("chế độ gói-theo-loại không nên tạo thư mục đóng gói lồng")
	}
	// Bố cục theo loại vẫn có.
	if _, err := os.Stat(filepath.Join(dir, "video/screenplay/01.md")); err != nil {
		t.Errorf("thiếu screenplay theo loại: %v", err)
	}
}

func TestVolumeGroupsForRange_Layered(t *testing.T) {
	b := &storyBible{
		Layered:    true,
		MaxChapter: 3,
		Completed:  map[int]bool{1: true, 2: true, 3: true},
		Volumes: []domain.VolumeOutline{
			{Index: 1, Title: "Tập một", Arcs: []domain.ArcOutline{{Index: 1, Chapters: []domain.OutlineEntry{{}, {}}}}},
			{Index: 2, Title: "Tập hai", Arcs: []domain.ArcOutline{{Index: 1, Chapters: []domain.OutlineEntry{{}}}}},
		},
	}
	groups, skipped := b.volumeGroupsForRange(0, 0)
	if len(skipped) != 0 {
		t.Fatalf("skipped = %v, muốn rỗng", skipped)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %d, muốn 2", len(groups))
	}
	if groups[0].Index != 1 || len(groups[0].Chapters) != 2 || groups[0].Chapters[0] != 1 || groups[0].Chapters[1] != 2 {
		t.Errorf("nhóm tập 1 sai: %+v", groups[0])
	}
	if groups[1].Index != 2 || len(groups[1].Chapters) != 1 || groups[1].Chapters[0] != 3 {
		t.Errorf("nhóm tập 2 sai: %+v", groups[1])
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

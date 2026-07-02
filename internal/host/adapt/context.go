package adapt

import (
	"encoding/json"
	"fmt"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

// maxChapterRunes giới hạn rune của văn bản một chương đưa vào prompt (tránh vượt cửa sổ).
const maxChapterRunes = 24000

// storyBible là ngữ cảnh chung chỉ đọc, dựng một lần rồi tiêm vào mọi bước.
type storyBible struct {
	NovelName  string
	Premise    string
	Outline    []domain.OutlineEntry // đã trải phẳng theo số chương toàn cục
	Layered    bool
	Volumes    []domain.VolumeOutline
	Characters []domain.Character
	Snapshots  []domain.CharacterSnapshot
	WorldRules []domain.WorldRule
	Timeline   []domain.TimelineEvent
	StyleHint  string

	Completed  map[int]bool
	MaxChapter int
}

// loadStoryBible đọc toàn bộ ngữ cảnh cần thiết từ store (chỉ đọc).
func loadStoryBible(st *store.Store, styleHint string) (*storyBible, error) {
	progress, err := st.Progress.Load()
	if err != nil {
		return nil, fmt.Errorf("đọc tiến độ: %w", err)
	}
	if progress == nil || len(progress.CompletedChapters) == 0 {
		return nil, fmt.Errorf("dự án chưa có chương nào hoàn thành để chuyển thể")
	}

	b := &storyBible{
		NovelName:  progress.NovelName,
		Layered:    progress.Layered,
		StyleHint:  styleHint,
		Completed:  make(map[int]bool, len(progress.CompletedChapters)),
	}
	for _, ch := range progress.CompletedChapters {
		b.Completed[ch] = true
		if ch > b.MaxChapter {
			b.MaxChapter = ch
		}
	}

	if premise, err := st.Outline.LoadPremise(); err == nil {
		b.Premise = premise
	}

	if progress.Layered {
		if volumes, err := st.Outline.LoadLayeredOutline(); err == nil && len(volumes) > 0 {
			b.Volumes = volumes
			b.Outline = domain.FlattenOutline(volumes)
		}
	}
	if len(b.Outline) == 0 {
		if outline, err := st.Outline.LoadOutline(); err == nil {
			b.Outline = outline
		}
	}

	if chars, err := st.Characters.Load(); err == nil {
		b.Characters = chars
	}
	if snaps, err := st.Characters.LoadLatestSnapshots(); err == nil {
		b.Snapshots = snaps
	}
	if rules, err := st.World.LoadWorldRules(); err == nil {
		b.WorldRules = rules
	}
	if tl, err := st.World.LoadTimeline(); err == nil {
		b.Timeline = tl
	}

	return b, nil
}

// outlineFor tìm mục dàn ý của một chương (rỗng nếu không có).
func (b *storyBible) outlineFor(chapter int) domain.OutlineEntry {
	for _, e := range b.Outline {
		if e.Chapter == chapter {
			return e
		}
	}
	return domain.OutlineEntry{Chapter: chapter}
}

// completedChaptersInRange trả về danh sách chương đã hoàn thành trong [from,to]
// (0/0 = toàn bộ), cùng danh sách chương bị bỏ qua vì chưa hoàn thành.
func (b *storyBible) completedChaptersInRange(from, to int) (chapters, skipped []int) {
	if from <= 0 {
		from = 1
	}
	if to <= 0 || to > b.MaxChapter {
		to = b.MaxChapter
	}
	for ch := from; ch <= to; ch++ {
		if b.Completed[ch] {
			chapters = append(chapters, ch)
		} else {
			skipped = append(skipped, ch)
		}
	}
	return chapters, skipped
}

// styleContext gói phần "style bible" dùng chung cho prompt (premise + thế giới + gợi ý phong cách).
func (b *storyBible) styleContext() map[string]any {
	return map[string]any{
		"novel_name":  b.NovelName,
		"premise":     compact(b.Premise, 8000),
		"world_rules": b.WorldRules,
		"style_hint":  b.StyleHint,
	}
}

// charactersPayload gói hồ sơ nhân vật + snapshot mới nhất.
func (b *storyBible) charactersPayload() []map[string]any {
	snapByName := make(map[string]domain.CharacterSnapshot, len(b.Snapshots))
	for _, s := range b.Snapshots {
		snapByName[s.Name] = s
	}
	out := make([]map[string]any, 0, len(b.Characters))
	for _, c := range b.Characters {
		item := map[string]any{
			"name":        c.Name,
			"aliases":     c.Aliases,
			"role":        c.Role,
			"description": c.Description,
			"arc":         c.Arc,
			"traits":      c.Traits,
			"tier":        c.Tier,
		}
		if s, ok := snapByName[c.Name]; ok {
			item["latest_status"] = s.Status
			item["motivation"] = s.Motivation
			item["relations"] = s.Relations
		}
		out = append(out, item)
	}
	return out
}

// compact cắt ngắn chuỗi theo rune, giữ đầu và đuôi (giống sim.compactSourceContent).
func compact(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	head := maxRunes * 3 / 4
	tail := maxRunes - head
	return string(runes[:head]) + "\n\n[...lược bớt...]\n\n" + string(runes[len(runes)-tail:])
}

// jsonPayload tạo user prompt từ một payload JSON có thụt lề (giống sim.buildSourceUserPrompt).
func jsonPayload(intro string, payload any) string {
	data, _ := json.MarshalIndent(payload, "", "  ")
	return intro + "\n\n" + string(data)
}

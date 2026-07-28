package comic

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host/adapt"
	"github.com/voocel/ainovel-cli/internal/store"
)

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

	Preset    StylePreset
	StyleHint string

	Completed  map[int]bool
	MaxChapter int

	// Di sản từ /video — nạp lại thay vì sinh lại (xem ingestVideo).
	VideoBible      *adapt.ConsistencyBible
	VideoConcept    *adapt.ConceptResult
	VideoCharacters []adapt.CharacterDesign
	videoDir        string
}

// loadStoryBible đọc toàn bộ ngữ cảnh cần thiết từ store (chỉ đọc).
func loadStoryBible(st *store.Store, opts Options) (*storyBible, error) {
	progress, err := st.Progress.Load()
	if err != nil {
		return nil, fmt.Errorf("đọc tiến độ: %w", err)
	}
	if progress == nil || len(progress.CompletedChapters) == 0 {
		return nil, fmt.Errorf("dự án chưa có chương nào hoàn thành để làm truyện tranh")
	}

	b := &storyBible{
		NovelName: progress.NovelName,
		Layered:   progress.Layered,
		Preset:    resolvePreset(opts.StylePreset),
		StyleHint: opts.StyleHint,
		Completed: make(map[int]bool, len(progress.CompletedChapters)),
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

	b.videoDir = filepath.Join(st.Dir(), "video")
	b.ingestVideo()
	return b, nil
}

// ingestVideo nạp lại các tài sản do /video sinh ra nếu có (best-effort, im lặng khi thiếu).
//
// Đây là điểm khiến truyện tranh và video chia sẻ cùng "ADN thị giác": consistency-bible
// đã khoá sẵn prompt chuẩn từng nhân vật + style token chung, việc sinh lại vừa tốn tiền
// vừa cho ra mô tả khác đi — nhân vật sẽ không còn giống bản video.
func (b *storyBible) ingestVideo() {
	var cb adapt.ConsistencyBible
	if readJSON(filepath.Join(b.videoDir, "consistency-bible.json"), &cb) && len(cb.Characters) > 0 {
		b.VideoBible = &cb
	}
	var concept adapt.ConceptResult
	if readJSON(filepath.Join(b.videoDir, "concept", "art-direction.json"), &concept) {
		b.VideoConcept = &concept
	}
	var chars []adapt.CharacterDesign
	if readJSON(filepath.Join(b.videoDir, "characters", "characters.json"), &chars) && len(chars) > 0 {
		b.VideoCharacters = chars
	}
}

// hasVideoAssets cho biết có di sản /video để tái dùng không (dùng để báo tiến trình).
func (b *storyBible) hasVideoAssets() bool {
	return b.VideoBible != nil || b.VideoConcept != nil || len(b.VideoCharacters) > 0
}

// loadStoryboard nạp phân cảnh /video của một chương (nguồn tốt nhất cho bước kịch bản).
func (b *storyBible) loadStoryboard(ch int) *adapt.StoryboardResult {
	var sb adapt.StoryboardResult
	if readJSON(filepath.Join(b.videoDir, fmt.Sprintf("storyboard/%02d.json", ch)), &sb) && len(sb.Scenes) > 0 {
		return &sb
	}
	return nil
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

// completedChaptersInRange trả về chương đã hoàn thành trong [from,to] (0/0 = toàn bộ),
// cùng danh sách chương bị bỏ qua vì chưa hoàn thành.
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

// chapterLoc định vị một chương trong cấu trúc phân tầng.
type chapterLoc struct {
	VolumeIndex int
	VolumeTitle string
}

// chapterLocations dựng map chương toàn cục → tập, đánh số tuần tự giống FlattenOutline.
func (b *storyBible) chapterLocations() map[int]chapterLoc {
	m := make(map[int]chapterLoc)
	if !b.Layered || len(b.Volumes) == 0 {
		return m
	}
	ch := 1
	for _, v := range b.Volumes {
		for _, a := range v.Arcs {
			for range a.Chapters {
				m[ch] = chapterLoc{VolumeIndex: v.Index, VolumeTitle: v.Title}
				ch++
			}
		}
	}
	return m
}

// volumeGroup gom các chương (số toàn cục) thuộc cùng một tập.
type volumeGroup struct {
	Index    int
	Title    string
	Chapters []int
}

// volumeGroupsForRange nhóm chương ĐÃ HOÀN THÀNH trong [from,to] theo tập, giữ thứ tự tăng
// dần để tập trước hoàn chỉnh trước. Chương không tra được tập → gom vào tập 1.
func (b *storyBible) volumeGroupsForRange(from, to int) ([]volumeGroup, []int) {
	chapters, skipped := b.completedChaptersInRange(from, to)
	locs := b.chapterLocations()
	pos := map[int]int{}
	var groups []volumeGroup
	for _, ch := range chapters {
		vi, vt := 1, ""
		if loc, ok := locs[ch]; ok {
			vi, vt = loc.VolumeIndex, loc.VolumeTitle
		}
		if p, seen := pos[vi]; seen {
			groups[p].Chapters = append(groups[p].Chapters, ch)
			continue
		}
		pos[vi] = len(groups)
		groups = append(groups, volumeGroup{Index: vi, Title: vt, Chapters: []int{ch}})
	}
	return groups, skipped
}

// styleContext gói phần nền dùng chung cho mọi prompt.
func (b *storyBible) styleContext() map[string]any {
	ctx := map[string]any{
		"novel_name":   b.NovelName,
		"premise":      compact(b.Premise, 8000),
		"world_rules":  b.WorldRules,
		"style_preset": b.Preset.Label,
		"style_tokens": b.Preset.Tokens,
		"negative":     b.Preset.Negative,
		"style_hint":   b.StyleHint,
	}
	// Di sản /video được ưu tiên: nếu đã có style token khoá sẵn thì dùng lại.
	if b.VideoConcept != nil && len(b.VideoConcept.Style.StyleTokens) > 0 {
		ctx["video_style_tokens"] = b.VideoConcept.Style.StyleTokens
		ctx["video_palette"] = b.VideoConcept.Style.Palette
	}
	return ctx
}

// charactersPayload gói hồ sơ nhân vật + snapshot mới nhất + prompt chuẩn từ /video.
func (b *storyBible) charactersPayload() []map[string]any {
	snapByName := make(map[string]domain.CharacterSnapshot, len(b.Snapshots))
	for _, s := range b.Snapshots {
		snapByName[s.Name] = s
	}
	canonical := map[string]string{}
	if b.VideoBible != nil {
		for _, t := range b.VideoBible.Characters {
			canonical[t.Name] = t.CanonicalPrompt
		}
	}
	designByName := map[string]adapt.CharacterDesign{}
	for _, d := range b.VideoCharacters {
		designByName[d.Name] = d
	}

	out := make([]map[string]any, 0, len(b.Characters))
	for _, c := range b.Characters {
		item := map[string]any{
			"name":        c.Name,
			"aliases":     c.Aliases,
			"role":        c.Role,
			"description": c.Description,
			"traits":      c.Traits,
			"tier":        c.Tier,
		}
		if s, ok := snapByName[c.Name]; ok {
			item["latest_status"] = s.Status
			item["motivation"] = s.Motivation
		}
		if p, ok := canonical[c.Name]; ok && p != "" {
			item["canonical_prompt_da_khoa"] = p
		}
		if d, ok := designByName[c.Name]; ok {
			item["appearance_tu_video"] = d.Appearance
			item["wardrobe_tu_video"] = d.Wardrobe
			item["palette_tu_video"] = d.Palette
		}
		out = append(out, item)
	}
	return out
}

// readJSON đọc + giải mã một tệp JSON (best-effort; false khi thiếu hoặc hỏng).
func readJSON(path string, out any) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, out) == nil
}

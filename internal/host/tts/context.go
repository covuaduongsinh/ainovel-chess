package tts

import (
	"fmt"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

// bookIndex là bản rút gọn của adapt.storyBible: chỉ giữ những gì TTS cần (tên sách,
// tập, tiêu đề chương, tập hợp chương đã hoàn thành). Cố ý KHÔNG dùng chung với
// adapt/context.go — xem adapt/io.go:14 về quy ước "gói năng lực tự chứa" của repo.
// Nếu sửa quy tắc nhóm theo tập ở đây, nhớ đối chiếu adapt/context.go:124-169.
type bookIndex struct {
	NovelName string
	Layered   bool
	Volumes   []domain.VolumeOutline
	Outline   []domain.OutlineEntry // đã trải phẳng theo số chương toàn cục

	Completed  map[int]bool
	MaxChapter int
}

// loadBookIndex đọc ngữ cảnh tối thiểu từ store (chỉ đọc).
func loadBookIndex(st *store.Store) (*bookIndex, error) {
	progress, err := st.Progress.Load()
	if err != nil {
		return nil, fmt.Errorf("đọc tiến độ: %w", err)
	}
	if progress == nil || len(progress.CompletedChapters) == 0 {
		return nil, fmt.Errorf("dự án chưa có chương nào hoàn thành để tạo sách nói")
	}

	b := &bookIndex{
		NovelName: progress.NovelName,
		Layered:   progress.Layered,
		Completed: make(map[int]bool, len(progress.CompletedChapters)),
	}
	for _, ch := range progress.CompletedChapters {
		b.Completed[ch] = true
		if ch > b.MaxChapter {
			b.MaxChapter = ch
		}
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
	return b, nil
}

// titleFor trả về tiêu đề chương lấy từ dàn ý (rỗng nếu không có).
func (b *bookIndex) titleFor(chapter int) string {
	for _, e := range b.Outline {
		if e.Chapter == chapter {
			return e.Title
		}
	}
	return ""
}

// completedChaptersInRange trả về danh sách chương đã hoàn thành trong [from,to]
// (0/0 = toàn bộ), cùng danh sách chương bị bỏ qua vì chưa hoàn thành.
func (b *bookIndex) completedChaptersInRange(from, to int) (chapters, skipped []int) {
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

// volumeGroup gom các chương (số toàn cục) thuộc cùng một tập.
type volumeGroup struct {
	Index    int
	Title    string
	Chapters []int
}

// volumeGroupsForRange nhóm các chương ĐÃ HOÀN THÀNH trong [from,to] theo tập, giữ
// thứ tự tăng dần. Chương không tra được tập → gom vào tập 1 (sách không phân tầng).
// Trả kèm danh sách chương bị bỏ qua vì chưa hoàn thành.
func (b *bookIndex) volumeGroupsForRange(from, to int) ([]volumeGroup, []int) {
	chapters, skipped := b.completedChaptersInRange(from, to)
	locs := b.chapterVolumes()
	pos := map[int]int{}
	var groups []volumeGroup
	for _, ch := range chapters {
		vi, vt := 1, ""
		if loc, ok := locs[ch]; ok {
			vi, vt = loc.index, loc.title
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

type volumeLoc struct {
	index int
	title string
}

// chapterVolumes dựng map chương toàn cục → tập từ dàn ý phân tầng, đánh số tuần tự
// giống domain.FlattenOutline. Sách không phân tầng → map rỗng.
func (b *bookIndex) chapterVolumes() map[int]volumeLoc {
	m := make(map[int]volumeLoc)
	if !b.Layered || len(b.Volumes) == 0 {
		return m
	}
	ch := 1
	for _, v := range b.Volumes {
		for _, a := range v.Arcs {
			for range a.Chapters {
				m[ch] = volumeLoc{index: v.Index, title: v.Title}
				ch++
			}
		}
	}
	return m
}

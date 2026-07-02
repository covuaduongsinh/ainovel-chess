package adapt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// runStoryboard chẻ mỗi chương thành cảnh → shots (mỗi chương 1 lời gọi LLM).
// Ưu tiên đọc kịch bản đã sinh (screenplay/{NN}.md) làm đầu vào; nếu chưa có thì
// dùng văn xuôi + OutlineEntry.Scenes. Tiêm visualBible để giữ nhất quán hình ảnh.
// Fail-soft per-chapter.
func runStoryboard(ctx context.Context, rc *runCtx) error {
	chapters, skipped := rc.bible.completedChaptersInRange(rc.opts.From, rc.opts.To)
	if len(chapters) == 0 {
		rc.emit(Event{Stage: StageStoryboard, Message: "Không có chương hoàn thành nào trong phạm vi"})
		return nil
	}
	if len(skipped) > 0 {
		rc.emit(Event{Stage: StageStoryboard, Message: fmt.Sprintf("Bỏ qua %d chương chưa hoàn thành trong phạm vi", len(skipped))})
	}

	visual := rc.visualBible()
	total := len(chapters)
	done := 0
	for i, ch := range chapters {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry := rc.bible.outlineFor(ch)
		rc.emit(Event{Stage: StageStoryboard, Current: i + 1, Total: total, Message: fmt.Sprintf("Phân cảnh chương %d — %s", ch, entry.Title)})

		source, kind := rc.storyboardSource(ch)
		if strings.TrimSpace(source) == "" {
			rc.emit(Event{Stage: StageStoryboard, Current: i + 1, Total: total, Message: fmt.Sprintf("Bỏ qua chương %d (không có nội dung)", ch)})
			continue
		}
		payload := map[string]any{
			"chapter":       ch,
			"title":         entry.Title,
			"outline":       entry,
			"source_kind":   kind,
			"source":        compact(source, maxChapterRunes),
			"visual_bible":  visual,
			"style_hint":    rc.bible.StyleHint,
		}
		var result StoryboardResult
		if err := generateJSON(ctx, rc.deps.LLM, rc.deps.Prompts.Storyboard,
			jsonPayload("Chẻ chương sau thành cảnh và shot, kèm prompt sinh ảnh/video song ngữ. Trả JSON theo schema trong <output>.", payload),
			&result); err != nil {
			rc.emit(Event{Stage: StageStoryboard, Current: i + 1, Total: total, Message: fmt.Sprintf("Bỏ qua chương %d (lỗi sinh)", ch), Err: err})
			continue
		}
		if len(result.Scenes) == 0 {
			rc.emit(Event{Stage: StageStoryboard, Current: i + 1, Total: total, Message: fmt.Sprintf("Bỏ qua chương %d (phân cảnh rỗng)", ch)})
			continue
		}
		result.Chapter = ch
		if strings.TrimSpace(result.Title) == "" {
			result.Title = entry.Title
		}
		if _, err := rc.writeJSON(ProductStoryboard, fmt.Sprintf("storyboard/%02d.json", ch), result); err != nil {
			return err
		}
		if _, err := rc.write(ProductStoryboard, fmt.Sprintf("storyboard/%02d.md", ch), []byte(renderStoryboardMarkdown(result))); err != nil {
			return err
		}
		done++
	}
	rc.emit(Event{Stage: StageStoryboard, Current: total, Total: total, Message: fmt.Sprintf("Đã phân cảnh %d/%d chương", done, total)})
	return nil
}

// storyboardSource chọn nguồn tốt nhất cho phân cảnh: kịch bản đã sinh > văn xuôi gốc.
func (rc *runCtx) storyboardSource(ch int) (content, kind string) {
	spPath := filepath.Join(rc.outDir, fmt.Sprintf("screenplay/%02d.md", ch))
	if data, err := os.ReadFile(spPath); err == nil && strings.TrimSpace(string(data)) != "" {
		return string(data), "screenplay"
	}
	text, err := rc.deps.Store.Drafts.LoadChapterText(ch)
	if err != nil {
		return "", "prose"
	}
	return text, "prose"
}

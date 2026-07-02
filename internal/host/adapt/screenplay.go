package adapt

import (
	"context"
	"fmt"
	"strings"
)

// runScreenplay chuyển văn xuôi từng chương → kịch bản (mỗi chương 1 lời gọi LLM).
// Fail-soft per-chapter: một chương lỗi thì bỏ qua, tiếp tục các chương còn lại.
func runScreenplay(ctx context.Context, rc *runCtx) error {
	chapters, skipped := rc.bible.completedChaptersInRange(rc.opts.From, rc.opts.To)
	if len(chapters) == 0 {
		rc.emit(Event{Stage: StageScreenplay, Message: "Không có chương hoàn thành nào trong phạm vi"})
		return nil
	}
	if len(skipped) > 0 {
		rc.emit(Event{Stage: StageScreenplay, Message: fmt.Sprintf("Bỏ qua %d chương chưa hoàn thành trong phạm vi", len(skipped))})
	}

	total := len(chapters)
	done := 0
	for i, ch := range chapters {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry := rc.bible.outlineFor(ch)
		rc.emit(Event{Stage: StageScreenplay, Current: i + 1, Total: total, Message: fmt.Sprintf("Viết kịch bản chương %d — %s", ch, entry.Title)})

		text, err := rc.deps.Store.Drafts.LoadChapterText(ch)
		if err != nil {
			rc.emit(Event{Stage: StageScreenplay, Current: i + 1, Total: total, Message: fmt.Sprintf("Bỏ qua chương %d (đọc lỗi)", ch), Err: err})
			continue
		}
		if strings.TrimSpace(text) == "" {
			rc.emit(Event{Stage: StageScreenplay, Current: i + 1, Total: total, Message: fmt.Sprintf("Bỏ qua chương %d (nội dung rỗng)", ch)})
			continue
		}

		payload := map[string]any{
			"chapter":    ch,
			"title":      entry.Title,
			"outline":    entry,
			"prose":      compact(text, maxChapterRunes),
			"characters": rc.bible.charactersPayload(),
			"style_hint": rc.bible.StyleHint,
		}
		var result ScreenplayResult
		if err := generateJSON(ctx, rc.deps.LLM, rc.deps.Prompts.Screenplay,
			jsonPayload("Chuyển văn xuôi chương sau thành kịch bản chuẩn (scene heading, action, lời thoại). Trả JSON {chapter,title,markdown} trong <output>.", payload),
			&result); err != nil {
			rc.emit(Event{Stage: StageScreenplay, Current: i + 1, Total: total, Message: fmt.Sprintf("Bỏ qua chương %d (lỗi sinh)", ch), Err: err})
			continue
		}
		if strings.TrimSpace(result.Markdown) == "" {
			rc.emit(Event{Stage: StageScreenplay, Current: i + 1, Total: total, Message: fmt.Sprintf("Bỏ qua chương %d (kịch bản rỗng)", ch)})
			continue
		}
		md := renderScreenplayMarkdown(ch, entry.Title, result)
		if _, err := rc.write(ProductScreenplay, fmt.Sprintf("screenplay/%02d.md", ch), []byte(md)); err != nil {
			return err
		}
		done++
	}
	rc.emit(Event{Stage: StageScreenplay, Current: total, Total: total, Message: fmt.Sprintf("Đã viết kịch bản %d/%d chương", done, total)})
	return nil
}

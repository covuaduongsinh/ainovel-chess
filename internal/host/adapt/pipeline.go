package adapt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// runChapterPipeline chạy các product cấp-chương theo thứ tự tập→chương: mỗi chương làm
// trọn kịch bản → phân cảnh → animation rồi ghi đóng gói lồng ngay; hết tập ghi mục lục
// tập. Nhờ vậy tập trước hoàn chỉnh trước, dễ dựng video từng tập. Fail-soft per-chapter
// (đã xử lý trong screenplayChapter/storyboardChapter); chỉ trả lỗi khi lỗi ghi tệp/hủy.
func runChapterPipeline(ctx context.Context, rc *runCtx, sel map[Product]bool) error {
	wantScreenplay := sel[ProductScreenplay]
	wantStoryboard := sel[ProductStoryboard]
	wantAnimation := sel[ProductAnimation]
	anyChapterProduct := wantScreenplay || wantStoryboard || wantAnimation || sel[ProductImagePrompt] || sel[ProductVideoPrompt]
	if !anyChapterProduct {
		return nil
	}

	groups, skipped := rc.bible.volumeGroupsForRange(rc.opts.From, rc.opts.To)
	if len(groups) == 0 {
		rc.emit(Event{Stage: StageContext, Message: "Không có chương hoàn thành nào trong phạm vi"})
		return nil
	}
	if len(skipped) > 0 {
		rc.emit(Event{Stage: StageContext, Message: fmt.Sprintf("Bỏ qua %d chương chưa hoàn thành trong phạm vi", len(skipped))})
	}

	visual := rc.visualBible()
	total := 0
	for _, g := range groups {
		total += len(g.Chapters)
	}

	idx := 0
	for _, g := range groups {
		for _, ch := range g.Chapters {
			if err := ctx.Err(); err != nil {
				return err
			}
			idx++
			entry := rc.bible.outlineFor(ch)
			label := fmt.Sprintf("Tập %d · Chương %d — %s", g.Index, ch, entry.Title)

			var sp *ScreenplayResult
			if wantScreenplay {
				rc.emit(Event{Stage: StageScreenplay, Current: idx, Total: total, Message: label + ": kịch bản"})
				r, err := screenplayChapter(ctx, rc, ch)
				if err != nil {
					return err
				}
				sp = r
			}

			var sb *StoryboardResult
			if wantStoryboard {
				rc.emit(Event{Stage: StageStoryboard, Current: idx, Total: total, Message: label + ": phân cảnh"})
				r, err := storyboardChapter(ctx, rc, ch, visual)
				if err != nil {
					return err
				}
				sb = r
			}

			// Animation cấp-loại (animation/{NN}.md): cần storyboard; nạp lại nếu không sinh lần này.
			if wantAnimation {
				asb := sb
				if asb == nil {
					asb = rc.loadStoryboardForChapter(ch)
				}
				if asb != nil {
					if _, err := rc.write(ProductAnimation, fmt.Sprintf("animation/%02d.md", ch), []byte(renderAnimationMarkdown(*asb))); err != nil {
						return err
					}
				}
			}

			// Đóng gói lồng cho chương (writeChapterBundle tự nạp lại từ đĩa nếu sp/sb nil).
			if err := writeChapterBundle(rc, ch, g.Index, sp, sb); err != nil {
				return err
			}
		}
		if err := writeVolumeIndex(rc, g); err != nil {
			return err
		}
		rc.emit(Event{Stage: StageStoryboard, Message: fmt.Sprintf("Hoàn tất Tập %d (%d chương)", g.Index, len(g.Chapters))})
	}

	// Bảng prompt tổng cả sách (giữ bố cục theo loại cũ) nếu được chọn.
	if sel[ProductImagePrompt] {
		if err := runImagePrompt(ctx, rc); err != nil {
			return err
		}
	}
	if sel[ProductVideoPrompt] {
		if err := runVideoPrompt(ctx, rc); err != nil {
			return err
		}
	}
	return nil
}

// loadStoryboardForChapter nạp storyboard/{NN}.json đã ghi (best-effort).
func (rc *runCtx) loadStoryboardForChapter(ch int) *StoryboardResult {
	var s StoryboardResult
	if rc.loadArtifact(fmt.Sprintf("storyboard/%02d.json", ch), &s) && len(s.Scenes) > 0 {
		if s.Chapter == 0 {
			s.Chapter = ch
		}
		return &s
	}
	return nil
}

// readOut đọc một tệp đã ghi trong outDir (best-effort).
func (rc *runCtx) readOut(rel string) ([]byte, bool) {
	data, err := os.ReadFile(filepath.Join(rc.outDir, rel))
	if err != nil {
		return nil, false
	}
	return data, true
}

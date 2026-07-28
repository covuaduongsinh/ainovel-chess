package comic

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// runRefSheet sinh ẢNH model sheet cho từng nhân vật.
//
// Đây là tầng thứ hai của chiến lược giữ nhất quán nhân vật: mỗi nhân vật được vẽ MỘT lần
// ở dạng turnaround trên nền trơn, rồi chính ảnh đó được truyền làm ảnh tham chiếu cho mọi
// khung có nhân vật đó. Không có bước này thì mỗi khung là một lần mô hình tự tưởng tượng
// lại diện mạo, và nhân vật sẽ trôi dạt dần qua các chương.
func runRefSheet(ctx context.Context, rc *runCtx) error {
	if rc.deps.Img == nil {
		return nil
	}
	chars := rc.ensureCharacters()
	if len(chars) == 0 {
		rc.emit(Event{Stage: StageRefSheet, Product: ProductRefSheet,
			Message: "Chưa có model sheet nhân vật — chạy bước character trước"})
		return nil
	}

	total := len(chars)
	made := 0
	for i, c := range chars {
		if err := ctx.Err(); err != nil {
			return err
		}
		rel := "nhan-vat/" + slug(c.Name) + ".png"
		if !rc.opts.Overwrite && exists(rc.path(rel)) {
			continue
		}
		if rc.opts.MaxImages > 0 && rc.imagesMade >= rc.opts.MaxImages {
			rc.emit(Event{Stage: StageRefSheet,
				Message: fmt.Sprintf("Đã chạm trần %d ảnh cho lần chạy này — dừng sinh ảnh", rc.opts.MaxImages)})
			return nil
		}

		rc.emit(Event{Stage: StageRefSheet, Product: ProductRefSheet, Current: i + 1, Total: total,
			Message: "Vẽ model sheet: " + c.Name})

		img, err := rc.deps.Img.Panel(ctx, PanelRequest{
			Prompt:   rc.buildSheetPrompt(c),
			Negative: rc.negativeTokens(c.NegativePrompt),
			Aspect:   "3:2", // turnaround nằm ngang
			Size:     rc.imageSize(),
		})
		if err != nil {
			// Nguồn hỏng hẳn thì dừng luôn, đừng thử tiếp cho các nhân vật còn lại.
			if errors.Is(err, ErrFatalImageSource) {
				rc.emit(Event{Stage: StageRefSheet, Message: "Dừng sinh ảnh", Err: err})
				return nil
			}
			rc.emit(Event{Stage: StageRefSheet,
				Message: fmt.Sprintf("Bỏ qua model sheet %s: %v", c.Name, err)})
			continue
		}
		if err := rc.writeAlways(ProductRefSheet, rel, img.Data); err != nil {
			return err
		}
		rc.imagesMade++
		made++
	}
	rc.emit(Event{Stage: StageRefSheet, Product: ProductRefSheet, Current: total, Total: total,
		Message: fmt.Sprintf("Đã vẽ %d model sheet nhân vật", made)})
	return nil
}

// buildSheetPrompt ghép prompt vẽ model sheet: mô tả chuẩn + yêu cầu bố cục turnaround.
func (rc *runCtx) buildSheetPrompt(c CharacterSheet) string {
	parts := []string{}
	if s := strings.TrimSpace(c.SheetPrompt); s != "" {
		parts = append(parts, s)
	}
	// canonical_prompt luôn phải có mặt: nó mới là thứ khoá diện mạo.
	if s := strings.TrimSpace(c.CanonicalPrompt); s != "" {
		parts = append(parts, s)
	}
	parts = append(parts,
		"character model sheet, full body turnaround: front view, three-quarter view, side view, and back view, "+
			"plus a row of three facial expressions",
		"evenly lit, plain flat neutral background, no scenery, no props, characters standing in a neutral pose")
	if tk := rc.styleTokens(); len(tk) > 0 {
		parts = append(parts, strings.Join(tk, ", "))
	}
	return strings.Join(filterEmpty(parts), ", ")
}

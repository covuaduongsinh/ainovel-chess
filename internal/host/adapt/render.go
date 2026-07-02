package adapt

import (
	"context"
	"fmt"
	"strings"
)

// ─────────────────────────── Render Markdown ───────────────────────────

func renderConceptMarkdown(novel string, c ConceptResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Concept / Art Direction — %s\n\n", titleOr(novel))
	b.WriteString("## Phong cách tổng thể\n\n")
	writeLine(&b, c.Style.Overall)
	writeField(&b, "Ánh sáng", c.Style.Lighting)
	writeField(&b, "Ngôn ngữ máy quay", c.Style.CameraLanguage)
	writeList(&b, "Bảng màu", c.Style.Palette)
	writeList(&b, "Style tokens (EN)", c.Style.StyleTokens)
	writeList(&b, "Tham chiếu", c.Style.References)
	if len(c.Locations) > 0 {
		b.WriteString("\n## Địa điểm chính\n")
		for _, l := range c.Locations {
			fmt.Fprintf(&b, "\n### %s\n\n", titleOr(l.Name))
			writeLine(&b, l.Description)
			writeField(&b, "Không khí", l.Mood)
			writePrompt(&b, "Image prompt", l.ImagePrompt)
		}
	}
	return b.String()
}

func renderCharactersMarkdown(novel string, designs []CharacterDesign) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Thiết kế nhân vật — %s\n", titleOr(novel))
	for _, d := range designs {
		fmt.Fprintf(&b, "\n## %s\n\n", titleOr(d.Name))
		writeField(&b, "Ngoại hình", d.Appearance)
		writeField(&b, "Trang phục", d.Wardrobe)
		writeList(&b, "Bảng màu", d.Palette)
		writePrompt(&b, "Key-art prompt", d.KeyArtPrompt)
		writePrompt(&b, "Turnaround prompt", d.TurnaroundPrompt)
		writePrompt(&b, "Negative prompt", d.NegativePrompt)
	}
	return b.String()
}

func renderPropsMarkdown(novel string, p PropResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Đạo cụ — %s\n", titleOr(novel))
	for _, d := range p.Props {
		fmt.Fprintf(&b, "\n## %s\n\n", titleOr(d.Name))
		writeField(&b, "Mô tả", d.Description)
		writeField(&b, "Vai trò", d.Significance)
		writePrompt(&b, "Image prompt", d.ImagePrompt)
		writePrompt(&b, "Negative prompt", d.NegativePrompt)
	}
	return b.String()
}

func renderConsistencyMarkdown(novel string, cb ConsistencyBible) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Bảng nhất quán trực quan — %s\n\n", titleOr(novel))
	writeList(&b, "Style tokens chung (EN)", cb.StyleTokens)
	writeConsistencyGroup(&b, "Nhân vật", cb.Characters)
	writeConsistencyGroup(&b, "Đạo cụ", cb.Props)
	writeConsistencyGroup(&b, "Địa điểm", cb.Locations)
	if strings.TrimSpace(cb.Notes) != "" {
		b.WriteString("\n## Ghi chú\n\n")
		writeLine(&b, cb.Notes)
	}
	return b.String()
}

func writeConsistencyGroup(b *strings.Builder, title string, tokens []ConsistencyToken) {
	if len(tokens) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## %s\n", title)
	for _, t := range tokens {
		fmt.Fprintf(b, "\n### %s\n\n", titleOr(t.Name))
		writePrompt(b, "Canonical prompt", t.CanonicalPrompt)
		writeField(b, "Seed", t.SeedHint)
		writeField(b, "Ghi chú", t.Notes)
	}
}

func renderScreenplayMarkdown(ch int, title string, s ScreenplayResult) string {
	head := fmt.Sprintf("# Chương %d — %s\n\n", ch, titleOr(title))
	body := strings.TrimSpace(s.Markdown)
	return head + body + "\n"
}

func renderStoryboardMarkdown(s StoryboardResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Phân cảnh — Chương %d — %s\n", s.Chapter, titleOr(s.Title))
	for _, sc := range s.Scenes {
		fmt.Fprintf(&b, "\n## Cảnh %s — %s\n\n", scOr(sc.SceneID), titleOr(sc.Heading))
		writeLine(&b, sc.Summary)
		for _, sh := range sc.Shots {
			fmt.Fprintf(&b, "\n### Shot %s", scOr(sh.ShotID))
			if sh.DurationSec > 0 {
				fmt.Fprintf(&b, " · %ds", sh.DurationSec)
			}
			b.WriteString("\n\n")
			writeField(&b, "Diễn biến", sh.Description)
			writeField(&b, "Góc máy", sh.CameraAngle)
			writeField(&b, "Chuyển động", sh.Movement)
			writeField(&b, "Lời thoại", sh.Dialogue)
			writePrompt(&b, "Image prompt", sh.ImagePrompt)
			writePrompt(&b, "Video prompt", sh.VideoPrompt)
			writePrompt(&b, "Negative prompt", sh.NegativePrompt)
		}
	}
	return b.String()
}

// ─────────────────────── Render-only products ───────────────────────

// runAnimation dựng bảng chỉ đạo animation từ storyboard đã sinh (không gọi LLM).
func runAnimation(ctx context.Context, rc *runCtx) error {
	boards := rc.loadStoryboards()
	if len(boards) == 0 {
		rc.emit(Event{Stage: StageAnimation, Message: "Chưa có storyboard — hãy chạy 'storyboard' trước"})
		return nil
	}
	done := 0
	for _, s := range boards {
		if err := ctx.Err(); err != nil {
			return err
		}
		var b strings.Builder
		fmt.Fprintf(&b, "# Chỉ đạo animation — Chương %d — %s\n", s.Chapter, titleOr(s.Title))
		for _, sc := range s.Scenes {
			fmt.Fprintf(&b, "\n## Cảnh %s — %s\n\n", scOr(sc.SceneID), titleOr(sc.Heading))
			for _, sh := range sc.Shots {
				fmt.Fprintf(&b, "- **Shot %s**", scOr(sh.ShotID))
				if sh.DurationSec > 0 {
					fmt.Fprintf(&b, " (%ds)", sh.DurationSec)
				}
				b.WriteString(": ")
				b.WriteString(joinNonEmpty(" · ", sh.Movement, sh.CameraAngle))
				if strings.TrimSpace(sh.VideoPrompt) != "" {
					fmt.Fprintf(&b, "\n  - Motion: %s", sh.VideoPrompt)
				}
				b.WriteString("\n")
			}
		}
		if _, err := rc.write(ProductAnimation, fmt.Sprintf("animation/%02d.md", s.Chapter), []byte(b.String())); err != nil {
			return err
		}
		done++
	}
	rc.emit(Event{Stage: StageAnimation, Current: done, Total: len(boards), Message: fmt.Sprintf("Đã tạo chỉ đạo animation cho %d chương", done)})
	return nil
}

// runImagePrompt tổng hợp bảng prompt ảnh phẳng (không gọi LLM).
func runImagePrompt(ctx context.Context, rc *runCtx) error {
	boards := rc.loadStoryboards()
	if len(boards) == 0 {
		rc.emit(Event{Stage: StageImagePrompt, Message: "Chưa có storyboard — hãy chạy 'storyboard' trước"})
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Bảng prompt ảnh — %s\n", titleOr(rc.bible.NovelName))
	if c := rc.ensureConcept(); c != nil {
		for _, l := range c.Locations {
			writePromptRow(&b, "Địa điểm: "+l.Name, l.ImagePrompt, "")
		}
	}
	for _, d := range rc.ensureCharacters() {
		writePromptRow(&b, "Nhân vật: "+d.Name, d.KeyArtPrompt, d.NegativePrompt)
	}
	for _, s := range boards {
		fmt.Fprintf(&b, "\n## Chương %d — %s\n", s.Chapter, titleOr(s.Title))
		for _, sc := range s.Scenes {
			for _, sh := range sc.Shots {
				writePromptRow(&b, fmt.Sprintf("C%s·S%s", scOr(sc.SceneID), scOr(sh.ShotID)), sh.ImagePrompt, sh.NegativePrompt)
			}
		}
	}
	if _, err := rc.write(ProductImagePrompt, "prompts/image-prompts.md", []byte(b.String())); err != nil {
		return err
	}
	rc.emit(Event{Stage: StageImagePrompt, Current: 1, Total: 1, Message: "Đã tạo bảng prompt ảnh"})
	return nil
}

// runVideoPrompt tổng hợp bảng prompt video phẳng (không gọi LLM).
func runVideoPrompt(ctx context.Context, rc *runCtx) error {
	boards := rc.loadStoryboards()
	if len(boards) == 0 {
		rc.emit(Event{Stage: StageVideoPrompt, Message: "Chưa có storyboard — hãy chạy 'storyboard' trước"})
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Bảng prompt video — %s\n", titleOr(rc.bible.NovelName))
	for _, s := range boards {
		fmt.Fprintf(&b, "\n## Chương %d — %s\n", s.Chapter, titleOr(s.Title))
		for _, sc := range s.Scenes {
			for _, sh := range sc.Shots {
				label := fmt.Sprintf("C%s·S%s", scOr(sc.SceneID), scOr(sh.ShotID))
				if sh.DurationSec > 0 {
					label += fmt.Sprintf(" (%ds)", sh.DurationSec)
				}
				writePromptRow(&b, label, sh.VideoPrompt, sh.NegativePrompt)
			}
		}
	}
	if _, err := rc.write(ProductVideoPrompt, "prompts/video-prompts.md", []byte(b.String())); err != nil {
		return err
	}
	rc.emit(Event{Stage: StageVideoPrompt, Current: 1, Total: 1, Message: "Đã tạo bảng prompt video"})
	return nil
}

// loadStoryboards đọc các file storyboard/{NN}.json cho chương hoàn thành trong phạm vi.
func (rc *runCtx) loadStoryboards() []StoryboardResult {
	chapters, _ := rc.bible.completedChaptersInRange(rc.opts.From, rc.opts.To)
	var out []StoryboardResult
	for _, ch := range chapters {
		var s StoryboardResult
		if rc.loadArtifact(fmt.Sprintf("storyboard/%02d.json", ch), &s) && len(s.Scenes) > 0 {
			if s.Chapter == 0 {
				s.Chapter = ch
			}
			out = append(out, s)
		}
	}
	return out
}

// ─────────────────────────── tiện ích render ───────────────────────────

func titleOr(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(chưa đặt)"
	}
	return s
}

func scOr(s string) string {
	if strings.TrimSpace(s) == "" {
		return "?"
	}
	return s
}

func writeLine(b *strings.Builder, s string) {
	if strings.TrimSpace(s) == "" {
		return
	}
	b.WriteString(strings.TrimSpace(s))
	b.WriteString("\n")
}

func writeField(b *strings.Builder, label, val string) {
	if strings.TrimSpace(val) == "" {
		return
	}
	fmt.Fprintf(b, "- **%s:** %s\n", label, strings.TrimSpace(val))
}

func writeList(b *strings.Builder, label string, items []string) {
	items = nonEmpty(items)
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "- **%s:** %s\n", label, strings.Join(items, ", "))
}

func writePrompt(b *strings.Builder, label, prompt string) {
	if strings.TrimSpace(prompt) == "" {
		return
	}
	fmt.Fprintf(b, "- **%s:**\n\n  ```\n  %s\n  ```\n", label, strings.TrimSpace(prompt))
}

func writePromptRow(b *strings.Builder, label, prompt, negative string) {
	if strings.TrimSpace(prompt) == "" {
		return
	}
	fmt.Fprintf(b, "\n- **%s**\n\n  ```\n  %s\n  ```\n", label, strings.TrimSpace(prompt))
	if strings.TrimSpace(negative) != "" {
		fmt.Fprintf(b, "  Negative: `%s`\n", strings.TrimSpace(negative))
	}
}

func nonEmpty(items []string) []string {
	out := items[:0:0]
	for _, s := range items {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func joinNonEmpty(sep string, parts ...string) string {
	return strings.Join(nonEmpty(parts), sep)
}

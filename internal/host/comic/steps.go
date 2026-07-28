package comic

import (
	"context"
	"fmt"
	"strings"
)

// Ba bước cần LLM: style (1 lần) → character (1 lần) → script (mỗi chương 1 lần).
// Mọi bước còn lại của đường ống đều là thuần Go, không tốn token.

// runStyle dựng định hướng mỹ thuật cấp sách.
func runStyle(ctx context.Context, rc *runCtx) error {
	if !rc.opts.Overwrite && exists(rc.path("style/art-direction.json")) {
		rc.emit(Event{Stage: StageStyle, Product: ProductStyle, Message: "Đã có định hướng mỹ thuật, bỏ qua"})
		return nil
	}
	rc.emit(Event{Stage: StageStyle, Product: ProductStyle, Message: "Đang xác lập định hướng mỹ thuật..."})

	payload := rc.bible.styleContext()
	payload["characters"] = rc.bible.charactersPayload()

	var res StyleResult
	if err := generateJSON(ctx, rc.deps.LLM, rc.deps.Prompts.Style,
		jsonPayload("Xác lập định hướng mỹ thuật cho bản truyện tranh của cuốn sách sau.", payload),
		&res); err != nil {
		return fmt.Errorf("sinh định hướng mỹ thuật: %w", err)
	}
	if len(res.StyleTokens) == 0 {
		// Không có token phong cách thì mọi khung sẽ trôi dạt — lấy tạm từ preset.
		res.StyleTokens = strings.Split(rc.bible.Preset.Tokens, ", ")
	}
	rc.style = &res

	if _, err := rc.writeJSON(ProductStyle, "style/art-direction.json", res); err != nil {
		return err
	}
	if _, err := rc.write(ProductStyle, "style/art-direction.md", []byte(renderStyleMarkdown(rc.bible.NovelName, res))); err != nil {
		return err
	}
	rc.emit(Event{Stage: StageStyle, Product: ProductStyle, Message: fmt.Sprintf("Đã khoá %d token phong cách", len(res.StyleTokens))})
	return nil
}

// runCharacter lập model sheet cho các nhân vật cốt lõi.
func runCharacter(ctx context.Context, rc *runCtx) error {
	if !rc.opts.Overwrite && exists(rc.path("nhan-vat/characters.json")) {
		rc.emit(Event{Stage: StageCharacter, Product: ProductCharacter, Message: "Đã có model sheet nhân vật, bỏ qua"})
		return nil
	}
	rc.emit(Event{Stage: StageCharacter, Product: ProductCharacter, Message: "Đang lập model sheet nhân vật..."})

	payload := map[string]any{
		"novel_name":   rc.bible.NovelName,
		"premise":      compact(rc.bible.Premise, 4000),
		"characters":   rc.bible.charactersPayload(),
		"style_tokens": rc.styleTokens(),
		"style_preset": rc.bible.Preset.Label,
		"style_hint":   rc.bible.StyleHint,
	}

	var res struct {
		Characters []CharacterSheet `json:"characters"`
	}
	if err := generateJSON(ctx, rc.deps.LLM, rc.deps.Prompts.Character,
		jsonPayload("Lập model sheet truyện tranh cho các nhân vật cốt lõi của cuốn sách sau.", payload),
		&res); err != nil {
		return fmt.Errorf("sinh model sheet nhân vật: %w", err)
	}
	if len(res.Characters) == 0 {
		return fmt.Errorf("model sheet nhân vật rỗng")
	}
	rc.characters = res.Characters

	if _, err := rc.writeJSON(ProductCharacter, "nhan-vat/characters.json", res.Characters); err != nil {
		return err
	}
	if _, err := rc.write(ProductCharacter, "nhan-vat/characters.md",
		[]byte(renderCharactersMarkdown(rc.bible.NovelName, res.Characters))); err != nil {
		return err
	}
	// Mỗi nhân vật một tệp riêng — tiện tra cứu và tiện thay thủ công.
	for _, c := range res.Characters {
		if _, err := rc.writeJSON(ProductCharacter, "nhan-vat/"+slug(c.Name)+".json", c); err != nil {
			return err
		}
	}
	rc.emit(Event{Stage: StageCharacter, Product: ProductCharacter,
		Message: fmt.Sprintf("Đã lập model sheet cho %d nhân vật", len(res.Characters))})
	return nil
}

// scriptChapter dựng kịch bản truyện tranh cho MỘT chương (1 lời gọi LLM).
// Trả (*result, nil) khi thành công; (nil, nil) khi bỏ qua mềm; (nil, err) khi lỗi ghi tệp.
func scriptChapter(ctx context.Context, rc *runCtx, ch int) (*ScriptResult, error) {
	rel := fmt.Sprintf("kich-ban/%02d.json", ch)
	if !rc.opts.Overwrite && exists(rc.path(rel)) {
		var cached ScriptResult
		if rc.loadArtifact(rel, &cached) && len(cached.Pages) > 0 {
			return &cached, nil
		}
	}

	entry := rc.bible.outlineFor(ch)
	source, kind := rc.scriptSource(ch)
	if strings.TrimSpace(source) == "" {
		rc.emit(Event{Stage: StageScript, Message: fmt.Sprintf("Bỏ qua chương %d (không có nội dung)", ch)})
		return nil, nil
	}

	payload := map[string]any{
		"chapter":      ch,
		"title":        entry.Title,
		"outline":      entry,
		"source_kind":  kind,
		"source":       compact(source, maxChapterRunes),
		"characters":          rc.characterPromptTable(),
		"style_tokens":        rc.styleTokens(),
		"style_preset":        rc.bible.Preset.Label,
		"style_hint":          rc.bible.StyleHint,
		"max_panels_per_page": rc.maxPanelsPerPage(),
	}
	// Phân cảnh video nếu có — gộp shot thành trang rẻ hơn nhiều so với chẻ lại chương.
	if sb := rc.bible.loadStoryboard(ch); sb != nil {
		payload["storyboard"] = sb
	}

	var res ScriptResult
	if err := generateJSON(ctx, rc.deps.LLM, rc.deps.Prompts.Script,
		jsonPayload("Chuyển chương sau thành kịch bản truyện tranh chia theo trang và khung.", payload),
		&res); err != nil {
		rc.emit(Event{Stage: StageScript, Message: fmt.Sprintf("Bỏ qua chương %d (lỗi sinh)", ch), Err: err})
		return nil, nil
	}
	if len(res.Pages) == 0 {
		rc.emit(Event{Stage: StageScript, Message: fmt.Sprintf("Bỏ qua chương %d (kịch bản rỗng)", ch)})
		return nil, nil
	}
	res.Chapter = ch
	if strings.TrimSpace(res.Title) == "" {
		res.Title = entry.Title
	}
	normalizeScript(&res)

	if _, err := rc.writeJSON(ProductScript, rel, res); err != nil {
		return nil, err
	}
	if _, err := rc.write(ProductScript, fmt.Sprintf("kich-ban/%02d.md", ch),
		[]byte(renderScriptMarkdown(res))); err != nil {
		return nil, err
	}
	return &res, nil
}

// scriptSource chọn nguồn tốt nhất cho kịch bản: kịch bản phim của /video > văn xuôi gốc.
func (rc *runCtx) scriptSource(ch int) (content, kind string) {
	if data, ok := rc.bible.readVideo(fmt.Sprintf("screenplay/%02d.md", ch)); ok {
		return string(data), "screenplay"
	}
	text, err := rc.deps.Store.Drafts.LoadChapterText(ch)
	if err != nil {
		return "", "prose"
	}
	return text, "prose"
}

// normalizeScript vá các trường LLM hay bỏ sót, để bước tính bố cục luôn có dữ liệu hợp lệ.
func normalizeScript(s *ScriptResult) {
	for pi := range s.Pages {
		p := &s.Pages[pi]
		if p.PageNo == 0 {
			p.PageNo = pi + 1
		}
		for ki := range p.Panels {
			k := &p.Panels[ki]
			if k.PanelNo == 0 {
				k.PanelNo = ki + 1
			}
			if panelSizeWeight(k.Size) == 0 && k.Size != "tran-trang" {
				k.Size = "vua"
			}
			for bi := range k.Balloons {
				b := &k.Balloons[bi]
				b.Kind = normalizeBalloonKind(b.Kind)
				if b.Order == 0 && bi > 0 {
					b.Order = bi
				}
			}
		}
	}
}

// characterPromptTable gói (tên → canonical prompt) để tiêm vào prompt kịch bản.
func (rc *runCtx) characterPromptTable() []map[string]string {
	out := make([]map[string]string, 0, len(rc.ensureCharacters()))
	for _, c := range rc.ensureCharacters() {
		out = append(out, map[string]string{
			"name":       c.Name,
			"role":       c.Role,
			"appearance": c.Appearance,
		})
	}
	return out
}

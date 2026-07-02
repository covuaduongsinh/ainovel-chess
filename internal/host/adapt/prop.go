package adapt

import "context"

// runProp sinh bảng đạo cụ chủ chốt (1 lời gọi LLM cho toàn bộ).
func runProp(ctx context.Context, rc *runCtx) error {
	rc.emit(Event{Stage: StageProp, Message: "Đang dựng bảng đạo cụ..."})
	payload := map[string]any{
		"style_context": rc.styleForVisual(),
		"premise":       compact(rc.bible.Premise, 6000),
		"world_rules":   rc.bible.WorldRules,
		"characters":    rc.bible.charactersPayload(),
	}
	var result PropResult
	if err := generateJSON(ctx, rc.deps.LLM, rc.deps.Prompts.Prop,
		jsonPayload("Liệt kê và thiết kế các đạo cụ chủ chốt xuất hiện trong truyện. Chỉ trả về JSON trong <output>.", payload),
		&result); err != nil {
		return err
	}
	rc.props = &result
	if _, err := rc.writeJSON(ProductProp, "props/props.json", result); err != nil {
		return err
	}
	if _, err := rc.write(ProductProp, "props/props.md", []byte(renderPropsMarkdown(rc.bible.NovelName, result))); err != nil {
		return err
	}
	rc.emit(Event{Stage: StageProp, Current: 1, Total: 1, Message: "Đã tạo bảng đạo cụ (" + itoa(len(result.Props)) + " mục)"})
	return nil
}

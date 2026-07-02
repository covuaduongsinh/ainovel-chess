package adapt

import "context"

// runConcept sinh art direction toàn cục (1 lời gọi LLM), làm "style bible" nền.
func runConcept(ctx context.Context, rc *runCtx) error {
	rc.emit(Event{Stage: StageConcept, Message: "Đang dựng concept / art direction..."})
	payload := map[string]any{
		"style_context": rc.bible.styleContext(),
		"characters":    rc.bible.charactersPayload(),
	}
	var result ConceptResult
	if err := generateJSON(ctx, rc.deps.LLM, rc.deps.Prompts.Concept,
		jsonPayload("Dựng art direction (phong cách hình ảnh + địa điểm chính) cho dự án phim từ dữ liệu sau. Chỉ trả về JSON trong <output>.", payload),
		&result); err != nil {
		return err
	}
	rc.concept = &result
	if _, err := rc.writeJSON(ProductConcept, "concept/art-direction.json", result); err != nil {
		return err
	}
	if _, err := rc.write(ProductConcept, "concept/art-direction.md", []byte(renderConceptMarkdown(rc.bible.NovelName, result))); err != nil {
		return err
	}
	rc.emit(Event{Stage: StageConcept, Current: 1, Total: 1, Message: "Đã tạo concept / art direction"})
	return nil
}

package adapt

import "context"

// runConsistency tổng hợp concept + character + prop thành "consistency bible":
// khóa token trực quan chuẩn (mô tả cố định) cho mỗi nhân vật/đạo cụ/địa điểm,
// để tiêm vào mọi prompt storyboard giữ nhất quán xuyên suốt.
func runConsistency(ctx context.Context, rc *runCtx) error {
	rc.emit(Event{Stage: StageConsistency, Message: "Đang tổng hợp bảng nhất quán..."})
	payload := map[string]any{
		"style_hint": rc.bible.StyleHint,
		"concept":    rc.ensureConcept(),
		"characters": rc.ensureCharacters(),
	}
	if p := rc.ensureProps(); p != nil {
		payload["props"] = p.Props
	}
	var result ConsistencyBible
	if err := generateJSON(ctx, rc.deps.LLM, rc.deps.Prompts.Consistency,
		jsonPayload("Tổng hợp các thiết kế sau thành bảng nhất quán trực quan (canonical prompt cố định cho mỗi thực thể). Chỉ trả về JSON trong <output>.", payload),
		&result); err != nil {
		return err
	}
	rc.consistency = &result
	if _, err := rc.writeJSON(ProductConsistency, "consistency-bible.json", result); err != nil {
		return err
	}
	if _, err := rc.write(ProductConsistency, "consistency-bible.md", []byte(renderConsistencyMarkdown(rc.bible.NovelName, result))); err != nil {
		return err
	}
	rc.emit(Event{Stage: StageConsistency, Current: 1, Total: 1, Message: "Đã tạo bảng nhất quán"})
	return nil
}

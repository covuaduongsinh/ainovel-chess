package adapt

import (
	"context"
	"strings"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// runCharacter sinh thiết kế trực quan cho từng nhân vật core/important (mỗi nhân vật 1 lời gọi).
// Fail-soft: một nhân vật lỗi thì bỏ qua và tiếp tục, không dừng cả bước.
func runCharacter(ctx context.Context, rc *runCtx) error {
	targets := designTargets(rc.bible.Characters)
	if len(targets) == 0 {
		rc.emit(Event{Stage: StageCharacter, Message: "Không có nhân vật core/important để thiết kế"})
		return nil
	}

	snapByName := make(map[string]domain.CharacterSnapshot, len(rc.bible.Snapshots))
	for _, s := range rc.bible.Snapshots {
		snapByName[s.Name] = s
	}
	styleCtx := rc.styleForVisual()

	designs := make([]CharacterDesign, 0, len(targets))
	total := len(targets)
	for i, c := range targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		rc.emit(Event{Stage: StageCharacter, Current: i + 1, Total: total, Message: "Thiết kế nhân vật: " + c.Name})
		payload := map[string]any{
			"character":     characterPayload(c, snapByName[c.Name]),
			"style_context": styleCtx,
		}
		var d CharacterDesign
		if err := generateJSON(ctx, rc.deps.LLM, rc.deps.Prompts.Character,
			jsonPayload("Thiết kế trực quan cho nhân vật sau, bám sát mô tả và phong cách. Chỉ trả về JSON trong <output>.", payload),
			&d); err != nil {
			rc.emit(Event{Stage: StageCharacter, Current: i + 1, Total: total, Message: "Bỏ qua nhân vật " + c.Name + " (lỗi)", Err: err})
			continue
		}
		if strings.TrimSpace(d.Name) == "" {
			d.Name = c.Name
		}
		designs = append(designs, d)
		if _, err := rc.writeJSON(ProductCharacter, "characters/"+slug(c.Name)+".json", d); err != nil {
			return err
		}
	}

	rc.characters = designs
	if _, err := rc.writeJSON(ProductCharacter, "characters/characters.json", designs); err != nil {
		return err
	}
	if _, err := rc.write(ProductCharacter, "characters/characters.md", []byte(renderCharactersMarkdown(rc.bible.NovelName, designs))); err != nil {
		return err
	}
	rc.emit(Event{Stage: StageCharacter, Current: total, Total: total, Message: "Đã thiết kế " + itoa(len(designs)) + " nhân vật"})
	return nil
}

// designTargets lọc nhân vật đáng thiết kế (core/important; tier rỗng coi như important).
func designTargets(chars []domain.Character) []domain.Character {
	var out []domain.Character
	for _, c := range chars {
		switch strings.ToLower(strings.TrimSpace(c.Tier)) {
		case "core", "important", "":
			out = append(out, c)
		}
	}
	return out
}

func characterPayload(c domain.Character, snap domain.CharacterSnapshot) map[string]any {
	item := map[string]any{
		"name":        c.Name,
		"aliases":     c.Aliases,
		"role":        c.Role,
		"description": c.Description,
		"arc":         c.Arc,
		"traits":      c.Traits,
		"tier":        c.Tier,
	}
	if snap.Name != "" {
		item["latest_status"] = snap.Status
		item["motivation"] = snap.Motivation
		item["relations"] = snap.Relations
	}
	return item
}

// styleForVisual trả về ngữ cảnh phong cách để tiêm vào bước thiết kế hình ảnh
// (ưu tiên concept đã sinh; nếu chưa có thì dùng premise + gợi ý phong cách).
func (rc *runCtx) styleForVisual() map[string]any {
	if c := rc.ensureConcept(); c != nil {
		return map[string]any{
			"overall":      c.Style.Overall,
			"palette":      c.Style.Palette,
			"lighting":     c.Style.Lighting,
			"style_tokens": c.Style.StyleTokens,
			"style_hint":   rc.bible.StyleHint,
		}
	}
	return map[string]any{
		"premise":    compact(rc.bible.Premise, 4000),
		"style_hint": rc.bible.StyleHint,
	}
}

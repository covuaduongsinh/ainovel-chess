package assets

import (
	"embed"
	"fmt"
	"strings"

	"github.com/voocel/ainovel-cli/internal/tools"
)

//go:embed prompts/*.md
var promptsFS embed.FS

//go:embed references
var referencesFS embed.FS

//go:embed styles/*.md
var stylesFS embed.FS

// Prompts là tập hợp prompt được nhúng.
type Prompts struct {
	Coordinator      string
	ArchitectShort   string
	ArchitectLong    string
	Writer           string
	Editor           string
	ImportFoundation string
	ImportAnalyzer   string
	SimulationSource string
	SimulationMerge  string
}

// Bundle là tập hợp tài nguyên tĩnh cần cho việc chạy.
type Bundle struct {
	References tools.References
	Prompts    Prompts
	Styles     map[string]string
}

// Load trả về tập tài nguyên tương ứng với phong cách chỉ định.
func Load(style string) Bundle {
	return Bundle{
		References: loadReferences(style),
		Prompts:    loadPrompts(),
		Styles:     loadStyles(),
	}
}

func loadReferences(style string) tools.References {
	if style == "" {
		style = "default"
	}
	refs := tools.References{
		ChapterGuide:      mustRead(referencesFS, "references/chapter-guide.md"),
		HookTechniques:    mustRead(referencesFS, "references/hook-techniques.md"),
		QualityChecklist:  mustRead(referencesFS, "references/quality-checklist.md"),
		OutlineTemplate:   mustRead(referencesFS, "references/outline-template.md"),
		CharacterTemplate: mustRead(referencesFS, "references/character-template.md"),
		ChapterTemplate:   mustRead(referencesFS, "references/chapter-template.md"),
		Consistency:       mustRead(referencesFS, "references/consistency.md"),
		ContentExpansion:  mustRead(referencesFS, "references/content-expansion.md"),
		DialogueWriting:   mustRead(referencesFS, "references/dialogue-writing.md"),
		LongformPlanning:  mustRead(referencesFS, "references/longform-planning.md"),
		Differentiation:   mustRead(referencesFS, "references/differentiation.md"),
		AntiAITone:        mustRead(referencesFS, "references/anti-ai-tone.md"),
	}
	if style != "" && style != "default" {
		genreDir := "references/genres/" + style + "/"
		if data, err := referencesFS.ReadFile(genreDir + "style-references.md"); err == nil {
			refs.StyleReference = string(data)
		}
		if data, err := referencesFS.ReadFile(genreDir + "arc-templates.md"); err == nil {
			refs.ArcTemplates = string(data)
		}
	}
	return refs
}

func loadPrompts() Prompts {
	return Prompts{
		Coordinator:      WithSimulationGuidance(mustRead(promptsFS, "prompts/coordinator.md"), "coordinator"),
		ArchitectShort:   WithSimulationGuidance(mustRead(promptsFS, "prompts/architect-short.md"), "architect"),
		ArchitectLong:    WithSimulationGuidance(mustRead(promptsFS, "prompts/architect-long.md"), "architect"),
		Writer:           WithSimulationGuidance(mustRead(promptsFS, "prompts/writer.md"), "writer"),
		Editor:           WithSimulationGuidance(mustRead(promptsFS, "prompts/editor.md"), "editor"),
		ImportFoundation: mustRead(promptsFS, "prompts/import-foundation.md"),
		ImportAnalyzer:   mustRead(promptsFS, "prompts/import-chapter-analyzer.md"),
		SimulationSource: mustRead(promptsFS, "prompts/simulation-source.md"),
		SimulationMerge:  mustRead(promptsFS, "prompts/simulation-merge.md"),
	}
}

// WithSimulationGuidance bổ sung chỉ dẫn chân dung phỏng tác vào prompt cốt lõi. Xuất ra để các
// cảnh ngoài như eval dùng lại khi override variant, bảo đảm prompt sau khi override tương đương
// baseline mà Load sinh ra (cùng đường bao gói).
func WithSimulationGuidance(prompt, role string) string {
	return prompt + "\n\n" + strings.ReplaceAll(simulationGuidance, "{{role}}", role)
}

// OverridePrompt dùng raw để override prompt vai trò tương ứng với tệp prompt chỉ định trong bundle,
// và đi qua đúng bao gói WithSimulationGuidance giống hệt Load — khi eval làm A/B chỉ cần gọi nó,
// không phải sao chép logic bao gói, nếu không baseline có hậu tố chân dung phỏng tác còn variant thì
// không, A/B sẽ không tương đương. file là tên tệp prompt.
func (b *Bundle) OverridePrompt(file, raw string) error {
	role, ok := promptRole[file]
	if !ok {
		return fmt.Errorf("tệp prompt không hỗ trợ override: %s (chỉ prompt cốt lõi mới override được)", file)
	}
	wrapped := WithSimulationGuidance(raw, role)
	switch file {
	case "coordinator.md":
		b.Prompts.Coordinator = wrapped
	case "architect-short.md":
		b.Prompts.ArchitectShort = wrapped
	case "architect-long.md":
		b.Prompts.ArchitectLong = wrapped
	case "writer.md":
		b.Prompts.Writer = wrapped
	case "editor.md":
		b.Prompts.Editor = wrapped
	}
	return nil
}

// promptRole ánh xạ tên tệp prompt cốt lõi sang placeholder vai trò của simulation guidance.
var promptRole = map[string]string{
	"coordinator.md":     "coordinator",
	"architect-short.md": "architect",
	"architect-long.md":  "architect",
	"writer.md":          "writer",
	"editor.md":          "editor",
}

const simulationGuidance = `## Chân dung phỏng tác

Khi novel_context trả về simulation_profile, bắt buộc coi nó là ràng buộc hướng phỏng tác của tác phẩm hiện tại. {{role}} nên đọc trong đó các phần style, lexicon, plot_design, hook_design, pacing_density, reader_engagement và role_guidance.

Nguyên tắc sử dụng: tham khảo cấu trúc, nhịp, móc câu, cách phóng thích thông tin và thủ pháp cuốn hút độc giả; đừng sao chép câu văn, nhân vật, địa danh, thiết định riêng hay tình tiết cố định của nguyên văn. Nếu simulation_profile xung đột với yêu cầu rõ ràng của người dùng, ưu tiên tuân theo yêu cầu của người dùng.`

func loadStyles() map[string]string {
	styles := make(map[string]string)
	entries, err := stylesFS.ReadDir("styles")
	if err != nil {
		return styles
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		data, err := stylesFS.ReadFile("styles/" + e.Name())
		if err != nil {
			continue
		}
		styles[name] = string(data)
	}
	return styles
}

func mustRead(fs embed.FS, path string) string {
	data, err := fs.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("embed read %s: %v", path, err))
	}
	return string(data)
}

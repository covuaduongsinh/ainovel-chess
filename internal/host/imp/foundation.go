package imp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

// FoundationResult là sản phẩm có cấu trúc từ quá trình suy ngược Foundation.
type FoundationResult struct {
	Premise    string                 // Chuỗi Markdown
	Characters []domain.Character     // Hồ sơ nhân vật
	WorldRules []domain.WorldRule     // Quy tắc thế giới
	Volumes    []domain.VolumeOutline // Đại cương phân tầng: nhập nội dung làm quyển đầu tiên (có thể tiếp tục, có thể mở rộng)
	Compass    *domain.StoryCompass   // Điểm neo định hướng tiếp tục viết (ending_direction / open_threads / estimated_scale)
}

// LLMChat là dependency tối thiểu của gói imp đối với ChatModel: chỉ cần một lần tạo văn bản thông thường.
// Tách thành interface độc lập để dễ inject mock trong unit test, tránh phụ thuộc trực tiếp vào agentcore client.
type LLMChat interface {
	Generate(ctx context.Context, messages []agentcore.Message, tools []agentcore.ToolSpec, opts ...agentcore.CallOption) (*agentcore.LLMResponse, error)
}

// ReverseFoundation dùng một lần gọi LLM để suy ngược foundation từ nội dung các chương đã phân tách.
// Không gọi save_foundation, là hàm thuần túy; việc lưu trữ do phía gọi quyết định.
func ReverseFoundation(ctx context.Context, llm LLMChat, systemPrompt string, chapters []Chapter) (*FoundationResult, error) {
	if len(chapters) == 0 {
		return nil, fmt.Errorf("no chapters to analyze")
	}
	if llm == nil {
		return nil, fmt.Errorf("llm is nil")
	}

	system := strings.ReplaceAll(systemPrompt, "${chapter_count}", fmt.Sprintf("%d", len(chapters)))
	user := buildFoundationUserPrompt(chapters)

	resp, err := llm.Generate(ctx, []agentcore.Message{
		agentcore.SystemMsg(system),
		agentcore.UserMsg(user),
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("llm generate: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("llm returned nil response")
	}

	return parseFoundationOutput(resp.Message.TextContent(), len(chapters))
}

// buildFoundationUserPrompt xây dựng user prompt: ghép tuần tự tất cả các chương,
// thêm anchor số chương để LLM dễ tham chiếu.
func buildFoundationUserPrompt(chapters []Chapter) string {
	var sb strings.Builder
	sb.WriteString("Dưới đây là ")
	fmt.Fprintf(&sb, "%d", len(chapters))
	sb.WriteString(" chương nội dung đã hoàn thành. Hãy suy ngược foundation theo đúng system prompt và xuất ra năm đoạn === TAG ===.\n\n")
	for i, ch := range chapters {
		fmt.Fprintf(&sb, "## Chuong %d: %s\n\n", i+1, ch.Title)
		sb.WriteString(ch.Content)
		sb.WriteString("\n\n---\n\n")
	}
	return sb.String()
}

// parseFoundationOutput phân tích envelope trong đầu ra của LLM và kiểm tra các ràng buộc quan trọng.
func parseFoundationOutput(text string, expectChapters int) (*FoundationResult, error) {
	env := parseTaggedEnvelope(text)
	if env == nil {
		return nil, fmt.Errorf("no === TAG === envelope found in LLM output")
	}
	if err := requireTags(env, "PREMISE", "CHARACTERS", "WORLD_RULES", "LAYERED_OUTLINE", "COMPASS"); err != nil {
		return nil, err
	}

	premise := stripFences(env["PREMISE"])
	if !strings.HasPrefix(strings.TrimLeft(premise, " \t\n"), "#") {
		return nil, fmt.Errorf("premise must start with a Markdown heading line (# tên truyện)")
	}

	var characters []domain.Character
	if err := decodeJSON("characters", env["CHARACTERS"], &characters); err != nil {
		return nil, err
	}
	if len(characters) == 0 {
		return nil, fmt.Errorf("characters array is empty")
	}

	var worldRules []domain.WorldRule
	if err := decodeJSON("world_rules", env["WORLD_RULES"], &worldRules); err != nil {
		return nil, err
	}

	var volumes []domain.VolumeOutline
	if err := decodeJSON("layered_outline", env["LAYERED_OUTLINE"], &volumes); err != nil {
		return nil, err
	}
	// Đại cương nhập phải trải phẳng đủ N chương thực (FlattenOutline chỉ đếm chương thực, không tính arc khung),
	// nếu không khi commit từng chương sẽ có chương nằm ngoài phạm vi đại cương và bị guard từ chối.
	if got := len(domain.FlattenOutline(volumes)); got != expectChapters {
		return nil, fmt.Errorf("layered outline chapter count mismatch: got %d, want %d", got, expectChapters)
	}

	var compass domain.StoryCompass
	if err := decodeJSON("compass", env["COMPASS"], &compass); err != nil {
		return nil, err
	}

	return &FoundationResult{
		Premise:    premise,
		Characters: characters,
		WorldRules: worldRules,
		Volumes:    volumes,
		Compass:    &compass,
	}, nil
}

// PersistFoundation Ghi ket qua suy nguoc vao Store, thu tu nhat quan voi Architect long prompt:
// premise -> characters -> world_rules -> layered_outline -> compass. Nhap noi dung lam tap dau tien
// thanh de cuong phan tang, de sach nhap co the duoc tiep tuc viet va mo rong. Moi buoc deu kich hoat cung logic luu disk cua save_foundation.
//
// Khong goi truc tiep SaveFoundationTool vi day la phuc hoi tat dinh, khong can di qua dieu phoi cong cu LLM.
// Nhung giu cung hieu ung phu voi SaveFoundationTool: tien hanh phase, them checkpoint.
func PersistFoundation(ctx context.Context, st *store.Store, scale domain.PlanningTier, fr *FoundationResult) error {
	if fr == nil {
		return fmt.Errorf("nil foundation result")
	}
	if err := st.RunMeta.SetPlanningTier(scale); err != nil {
		return fmt.Errorf("save planning tier: %w", err)
	}

	// 1. premise
	if err := st.Outline.SavePremise(fr.Premise); err != nil {
		return fmt.Errorf("save premise: %w", err)
	}
	if name := domain.ExtractNovelNameFromPremise(fr.Premise); name != "" {
		_ = st.Progress.SetNovelName(name)
	}
	_ = st.Progress.UpdatePhase(domain.PhasePremise)
	if _, err := st.Checkpoints.AppendArtifact(domain.GlobalScope(), "premise", "premise.md"); err != nil {
		return fmt.Errorf("checkpoint premise: %w", err)
	}

	// 2. characters
	if err := st.Characters.Save(fr.Characters); err != nil {
		return fmt.Errorf("save characters: %w", err)
	}
	if _, err := st.Checkpoints.AppendArtifact(domain.GlobalScope(), "characters", "characters.json"); err != nil {
		return fmt.Errorf("checkpoint characters: %w", err)
	}

	// 3. world_rules
	if err := st.World.SaveWorldRules(fr.WorldRules); err != nil {
		return fmt.Errorf("save world_rules: %w", err)
	}
	if _, err := st.Checkpoints.AppendArtifact(domain.GlobalScope(), "world_rules", "world_rules.json"); err != nil {
		return fmt.Errorf("checkpoint world_rules: %w", err)
	}

	// 4. layered outline (Nhap noi dung lam tap dau tien -> che do phan tang, co the tiep tuc viet va mo rong)
	if err := st.Outline.SaveLayeredOutline(fr.Volumes); err != nil {
		return fmt.Errorf("save layered outline: %w", err)
	}
	if err := st.Outline.SaveOutline(domain.FlattenOutline(fr.Volumes)); err != nil {
		return fmt.Errorf("save flattened outline: %w", err)
	}
	_ = st.Progress.UpdatePhase(domain.PhaseOutline)
	_ = st.Progress.SetTotalChapters(domain.TotalChapters(fr.Volumes))
	_ = st.Progress.SetLayered(true)
	if len(fr.Volumes) > 0 && len(fr.Volumes[0].Arcs) > 0 {
		_ = st.Progress.UpdateVolumeArc(fr.Volumes[0].Index, fr.Volumes[0].Arcs[0].Index)
	}
	if _, err := st.Checkpoints.AppendArtifact(domain.GlobalScope(), "layered_outline", "layered_outline.json"); err != nil {
		return fmt.Errorf("checkpoint layered outline: %w", err)
	}

	// 5. compass (Diem neo huong di tiep tuc viet): de layeredBookComplete phan xet dua vao open_threads,
	//    tranh bi coi la da ket thuc ngay khi nhap; cung cung cap co so ve huong di/do dai khi tiep tuc viet.
	if err := st.Outline.SaveCompass(*fr.Compass); err != nil {
		return fmt.Errorf("save compass: %w", err)
	}
	if _, err := st.Checkpoints.AppendArtifact(domain.GlobalScope(), "compass", "meta/compass.json"); err != nil {
		return fmt.Errorf("checkpoint compass: %w", err)
	}

	// 6. foundation hoan chinh -> tien den giai doan writing (nhat quan voi logic cuoi cua save_foundation)
	if len(st.FoundationMissing()) == 0 {
		if p, _ := st.Progress.Load(); p != nil &&
			p.Phase != domain.PhaseWriting && p.Phase != domain.PhaseComplete {
			_ = st.Progress.UpdatePhase(domain.PhaseWriting)
		}
	}
	return nil
}

// decodeJSON Phan tich JSON (mang hoac doi tuong) va kem nhan the, tien cho viec debug.
func decodeJSON(label, body string, out any) error {
	body = stripFences(body)
	if body == "" {
		return fmt.Errorf("%s body is empty", label)
	}
	if err := json.Unmarshal([]byte(body), out); err != nil {
		return fmt.Errorf("parse %s JSON: %w", label, err)
	}
	return nil
}

// stripFences Bo hang rao code ``` o dau cuoi (gom ca nhan ngon ngu), LLM doi khi tu y boc them mot lop.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	if j := strings.LastIndex(s, "```"); j >= 0 {
		s = s[:j]
	}
	return strings.TrimSpace(s)
}

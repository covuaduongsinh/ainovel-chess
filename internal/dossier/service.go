// Package dossier là tầng dịch vụ tạo "hồ sơ nhân vật thật" (mỏ neo sự thật) cho chế độ
// viết bám sát nhân vật có thật:
//   - NormalizeSource: biến tư liệu người dùng dán thành danh sách dữ kiện có cấu trúc.
//   - DraftFromSubject: để AI soạn nháp hồ sơ từ kiến thức mô hình (người dùng duyệt/sửa trước khi viết).
//
// Đây là đường tăng cường, không phải điều kiện tiên quyết: khi mô hình không khả dụng hoặc
// phân tích thất bại, dịch vụ giáng cấp (giữ nguyên tư liệu thô) chứ không chặn sáng tác.
// Bản nháp AI dựa vào trí nhớ mô hình nên gắn Disclaimer "cần kiểm chứng" — người dùng phải duyệt.
package dossier

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/voocel/agentcore"
	"github.com/voocel/agentcore/llm"
	"github.com/voocel/ainovel-cli/internal/domain"
)

const (
	dossierMaxTokens   = 8192
	dossierMaxAttempts = 3
	draftDisclaimer    = "Bản nháp do AI soạn từ trí nhớ mô hình, CÓ THỂ SAI. Hãy kiểm chứng và chỉnh sửa trước khi dùng."
)

// Service tạo/chuẩn hóa hồ sơ nhân vật thật qua một lần gọi LLM.
type Service struct {
	model    agentcore.ChatModel
	thinking agentcore.ThinkingLevel
}

// NewService tạo dịch vụ. model nên là mô hình mặc định (mạnh) như userrules dùng.
// Khi model là nil, mọi đường đều giáng cấp (giữ tư liệu thô / trả hồ sơ rỗng có ghi chú).
func NewService(model agentcore.ChatModel) *Service {
	thinking := agentcore.ThinkingAuto
	if model != nil {
		thinking, _ = llm.ThinkingPolicyFor(model).Resolve(agentcore.ThinkingOff)
	}
	return &Service{model: model, thinking: thinking}
}

// NormalizeSource biến tư liệu người dùng dán thành hồ sơ có cấu trúc.
// Không chặn sáng tác: khi mô hình nil hoặc phân tích thất bại, giữ nguyên tư liệu thô làm một dữ kiện.
func (s *Service) NormalizeSource(ctx context.Context, subject, source string) (domain.Dossier, error) {
	subject = strings.TrimSpace(subject)
	source = strings.TrimSpace(source)
	base := domain.Dossier{Subject: subject, Fidelity: domain.FidelityAnchored, RawSource: source}

	if source == "" {
		// Không có tư liệu: chỉ neo cái tên thật. Nếu có bật grounding mà bỏ trống tư liệu,
		// người dùng thường đã bấm "Soạn hồ sơ" nên nhánh này hiếm; vẫn giữ chủ thể làm mỏ neo.
		if subject != "" {
			base.Facts = []domain.DossierFact{{Category: "tiểu sử", Fact: "Nhân vật chính dựa trên nhân vật có thật tên " + subject + "; giữ đúng tên thật.", MustHold: true}}
		}
		return base, nil
	}
	if s == nil || s.model == nil {
		base.Facts = degradedFacts(source)
		return base, nil
	}

	facts, _, ok := s.generate(ctx, dossierNormalizeSystemPrompt, subject+"\n\n"+source)
	if !ok || len(facts) == 0 {
		base.Facts = degradedFacts(source)
		return base, nil
	}
	base.Facts = facts
	return base, nil
}

// DraftFromSubject để AI soạn nháp hồ sơ từ kiến thức mô hình. Luôn gắn Draft=true + Disclaimer.
func (s *Service) DraftFromSubject(ctx context.Context, subject string) (domain.Dossier, error) {
	subject = strings.TrimSpace(subject)
	d := domain.Dossier{Subject: subject, Fidelity: domain.FidelityAnchored, Draft: true, Disclaimer: draftDisclaimer}
	if subject == "" {
		d.Disclaimer = "Hãy nhập tên nhân vật có thật trước khi soạn nháp."
		return d, nil
	}
	if s == nil || s.model == nil {
		d.Disclaimer = "Mô hình không khả dụng — hãy tự nhập tư liệu nhân vật. " + draftDisclaimer
		return d, nil
	}
	facts, disclaimer, ok := s.generate(ctx, dossierDraftSystemPrompt, subject)
	if !ok {
		d.Disclaimer = "Không soạn được bản nháp tự động — hãy tự nhập tư liệu. " + draftDisclaimer
		return d, nil
	}
	d.Facts = facts
	if strings.TrimSpace(disclaimer) != "" {
		d.Disclaimer = strings.TrimSpace(disclaimer) + " " + draftDisclaimer
	}
	return d, nil
}

// generate gọi mô hình với thử lại có giới hạn, trả về facts + disclaimer đã phân tích.
func (s *Service) generate(ctx context.Context, systemPrompt, userText string) ([]domain.DossierFact, string, bool) {
	messages := []agentcore.Message{
		{Role: agentcore.RoleSystem, Content: []agentcore.ContentBlock{agentcore.TextBlock(systemPrompt)}},
		{Role: agentcore.RoleUser, Content: []agentcore.ContentBlock{agentcore.TextBlock(userText)}},
	}
	var lastErr string
	for attempt := 1; attempt <= dossierMaxAttempts; attempt++ {
		resp, err := s.model.Generate(ctx, messages, nil,
			agentcore.WithThinking(s.thinking),
			agentcore.WithMaxTokens(dossierMaxTokens))
		switch {
		case err != nil:
			lastErr = err.Error()
		case resp == nil:
			lastErr = "mô hình trả về phản hồi rỗng"
		default:
			raw := resp.Message.TextContent()
			if out, ok := parseDossierJSON(raw); ok {
				return out.toFacts(), out.Disclaimer, true
			}
			lastErr = "trả về JSON không hợp lệ"
			messages = append(messages,
				agentcore.Message{Role: agentcore.RoleAssistant, Content: []agentcore.ContentBlock{agentcore.TextBlock(raw)}},
				agentcore.Message{Role: agentcore.RoleUser, Content: []agentcore.ContentBlock{agentcore.TextBlock(dossierRetryHint)}},
			)
		}
		slog.Warn("Soạn/chuẩn hóa hồ sơ nhân vật thất bại",
			"module", "dossier", "attempt", attempt, "err", lastErr)
		if ctx.Err() != nil {
			break
		}
	}
	return nil, "", false
}

// degradedFacts giữ nguyên tư liệu thô làm một dữ kiện tham khảo khi không chuẩn hóa được.
func degradedFacts(source string) []domain.DossierFact {
	return []domain.DossierFact{{Category: "tư liệu thô", Fact: source, MustHold: false}}
}

// dossierOutput là dạng JSON theo quy ước của service.
type dossierOutput struct {
	Facts []struct {
		Category string `json:"category"`
		Fact     string `json:"fact"`
		MustHold bool   `json:"must_hold"`
	} `json:"facts"`
	Disclaimer string `json:"disclaimer"`
}

func (o dossierOutput) toFacts() []domain.DossierFact {
	out := make([]domain.DossierFact, 0, len(o.Facts))
	for _, f := range o.Facts {
		fact := strings.TrimSpace(f.Fact)
		if fact == "" {
			continue
		}
		out = append(out, domain.DossierFact{
			Category: strings.TrimSpace(f.Category),
			Fact:     fact,
			MustHold: f.MustHold,
		})
	}
	return out
}

func parseDossierJSON(raw string) (dossierOutput, bool) {
	s := extractJSON(raw)
	if s == "" {
		return dossierOutput{}, false
	}
	var out dossierOutput
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return dossierOutput{}, false
	}
	return out, true
}

// extractJSON gỡ hàng rào ```json và lấy từ { đầu tiên đến } cuối cùng.
func extractJSON(raw string) string {
	s := strings.TrimSpace(raw)
	if after, ok := strings.CutPrefix(s, "```"); ok {
		s = after
		s = strings.TrimPrefix(s, "json")
		s = strings.TrimPrefix(s, "JSON")
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < 0 || end < start {
		return ""
	}
	return s[start : end+1]
}

const dossierNormalizeSystemPrompt = `Bạn là «bộ chuẩn hóa hồ sơ nhân vật thật» của hệ thống viết truyện thiếu nhi bằng AI. Người dùng cung cấp TÊN một nhân vật/chủ thể có thật và một đoạn TƯ LIỆU. Hãy trích xuất tư liệu thành các dữ kiện có cấu trúc. Chỉ xuất một đối tượng JSON, không kèm giải thích.

JSON: { "facts": [ {"category": "...", "fact": "...", "must_hold": true|false} ], "disclaimer": "" }

- category chọn trong: tiểu sử | mốc thời gian | thành tựu | tính cách | bối cảnh | chuyên môn.
- fact: một câu ngắn gọn, trung thực với tư liệu; KHÔNG bịa thêm ngoài tư liệu.
- must_hold = true cho dữ kiện lõi cần giữ đúng (tên thật, mốc thời gian, thành tựu quan trọng, đặc điểm tính cách nổi bật, bối cảnh thời đại). must_hold = false cho chi tiết phụ/tham khảo.
- Giữ giọng phù hợp trẻ mầm non–tiểu học; tránh chi tiết nặng nề, bạo lực.
- disclaimer để trống ("") vì đây là tư liệu do người dùng cấp.`

const dossierDraftSystemPrompt = `Bạn là «trợ lý soạn hồ sơ nhân vật thật» cho hệ thống viết truyện thiếu nhi bằng AI. Người dùng đưa TÊN một nhân vật/chủ thể có thật. Hãy soạn một bản NHÁP các dữ kiện có thật mà bạn biết về nhân vật đó, để người dùng kiểm chứng và chỉnh sửa. Chỉ xuất một đối tượng JSON, không kèm giải thích.

JSON: { "facts": [ {"category": "...", "fact": "...", "must_hold": true|false} ], "disclaimer": "..." }

- category chọn trong: tiểu sử | mốc thời gian | thành tựu | tính cách | bối cảnh | chuyên môn.
- fact: một câu ngắn gọn. must_hold = true cho các dữ kiện lõi, kiểm chứng được (tên thật, năm sinh/mất nếu chắc, thành tựu chính, bối cảnh thời đại, lĩnh vực chuyên môn). must_hold = false cho chi tiết bạn KHÔNG chắc.
- QUAN TRỌNG: nếu không chắc một dữ kiện, hãy đặt must_hold = false và ghi rõ trong disclaimer những điểm cần người dùng kiểm chứng. Thà bỏ sót còn hơn bịa.
- Giữ giọng phù hợp trẻ mầm non–tiểu học.
- disclaimer: nêu ngắn gọn những điểm chưa chắc chắn cần kiểm chứng (một câu).`

const dossierRetryHint = "Phản hồi trên không phân tích được thành JSON. Chỉ xuất đúng một đối tượng JSON gồm hai trường facts và disclaimer, không kèm văn bản giải thích hay hàng rào code."

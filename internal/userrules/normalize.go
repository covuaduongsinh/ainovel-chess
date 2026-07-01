// Package userrules là tầng dịch vụ chuẩn hóa quy tắc người dùng: chuyển các quy tắc ngôn ngữ tự nhiên từ nhiều nguồn
// qua một lần gọi LLM thành các trường có cấu trúc ứng viên, rồi rules.BuildSnapshot hợp nhất có tính xác định thành ảnh chụp của sách.
//
// Trách nhiệm theo tầng:
//   - package rules: dữ liệu thuần + hợp nhất có tính xác định (Snapshot / Candidate / BuildSnapshot / SystemDefaults)
//   - package này: chuẩn hóa LLM + điều phối + lưu xuống đĩa (phụ thuộc agentcore + store + rules)
//
// Chuẩn hóa là đường tăng cường, không phải điều kiện tiên quyết của sáng tác chính: bất kỳ nguồn nào thất bại đều giáng cấp thành raw preferences, sáng tác chính phải tiếp tục.
package userrules

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/voocel/agentcore"
	"github.com/voocel/agentcore/llm"
	"github.com/voocel/ainovel-cli/internal/rules"
)

// normalizeMaxTokens giới hạn đầu ra cho mỗi lần chuẩn hóa (token suy nghĩ và đầu ra JSON dùng chung ngân sách này).
// JSON chuẩn hóa bản thân rất nhỏ (thường <1k), phần lớn ở đây dành cho ngân sách suy nghĩ của "mô hình suy luận không tắt được tư duy" —
// để hẹp quá, suy nghĩ sẽ chèn JSON dẫn đến cắt ngắn và lỗi phân tích. max_tokens là giới hạn trên không phải lượng tính phí, tăng lên không tốn thêm chi phí.
const normalizeMaxTokens = 8192

// normalizeMaxAttempts tổng số lần thử chuẩn hóa (tối đa thử lại 2 lần rồi giáng cấp, không thử lại vô hạn, xem thiết kế §thất bại và giáng cấp).
// Đầu ra LLM có tính ngẫu nhiên, thử lại sau lỗi phân tích thường lấy được JSON hợp lệ; dao động mạng tức thời tương tự.
const normalizeMaxAttempts = 3

// Normalizer chuẩn hóa quy tắc ngôn ngữ tự nhiên của một nguồn đơn lẻ thành rules.Candidate (một lần gọi LLM).
type Normalizer struct {
	model    agentcore.ChatModel
	thinking agentcore.ThinkingLevel // chuẩn hóa là trích xuất cơ học, tắt tư duy được thì tắt (xem NewNormalizer)
}

// NewNormalizer tạo bộ chuẩn hóa với một ChatModel. Chuẩn hóa là công cụ khởi động một lần,
// nên truyền mô hình mạnh hơn (như mô hình mặc định của ModelSet), không cần theo mô hình yếu dùng khi viết.
//
// Chuẩn hóa là trích xuất cơ học, không cần suy luận: tắt tư duy được thì tắt (dành max_tokens cho JSON, tiết kiệm latency và chi phí).
// Dùng Resolve(off) của chiến lược tư duy mô hình — hỗ trợ tắt thì tắt, không hỗ trợ (các mô hình luôn suy nghĩ như dòng o)
// thì dùng ThinkingAuto (mặc định provider), ngân sách tư duy của normalizeMaxTokens sẽ tránh cắt ngắn.
func NewNormalizer(model agentcore.ChatModel) *Normalizer {
	thinking := agentcore.ThinkingAuto
	if model != nil {
		thinking, _ = llm.ThinkingPolicyFor(model).Resolve(agentcore.ThinkingOff)
	}
	return &Normalizer{model: model, thinking: thinking}
}

// Normalize chuẩn hóa một nguồn. Không bao giờ trả về error — khi thất bại trả về Candidate giáng cấp
// (văn bản gốc làm raw preferences, không tạo structured), bên gọi tiếp tục hợp nhất.
func (n *Normalizer) Normalize(ctx context.Context, source, text string) rules.Candidate {
	text = strings.TrimSpace(text)
	if text == "" {
		return rules.Candidate{Source: source}
	}
	if n == nil || n.model == nil {
		return degraded(source, text)
	}

	messages := []agentcore.Message{
		{Role: agentcore.RoleSystem, Content: []agentcore.ContentBlock{agentcore.TextBlock(normalizerSystemPrompt)}},
		{Role: agentcore.RoleUser, Content: []agentcore.ContentBlock{agentcore.TextBlock(text)}},
	}

	// Giáng cấp sau khi thử lại có giới hạn: lỗi kỹ thuật (mạng/mô hình/JSON không hợp lệ) vào log, không vào ảnh chụp,
	// ảnh chụp chỉ giữ status=degraded + ghi chú nguồn (xem thiết kế §thất bại và giáng cấp / §phản hồi lại).
	var lastErr string
	for attempt := 1; attempt <= normalizeMaxAttempts; attempt++ {
		resp, err := n.model.Generate(ctx, messages, nil,
			agentcore.WithThinking(n.thinking),
			agentcore.WithMaxTokens(normalizeMaxTokens))
		switch {
		case err != nil:
			lastErr = err.Error()
		case resp == nil:
			lastErr = "mô hình trả về phản hồi rỗng"
		default:
			raw := resp.Message.TextContent()
			if out, ok := parseNormalizerJSON(raw); ok {
				return rules.Candidate{
					Source:      source,
					Structured:  out.Structured,
					Preferences: strings.TrimSpace(out.Preferences),
					Uncertain:   coerceUncertain(out.Uncertain),
				}
			}
			lastErr = "trả về JSON không hợp lệ"
			// Thử lại có phản hồi: đưa đầu ra không hợp lệ lần trước và gợi ý sửa vào cuộc hội thoại,
			// để vòng tiếp theo có thể tạo lại JSON có mục tiêu dựa trên lỗi, thay vì thử lại mù.
			// Chỉ có ý nghĩa với "định dạng xấu" — hai nhánh lỗi mạng / phản hồi rỗng
			// không có đầu ra lần trước để phản hồi, vẫn là thử lại mù.
			messages = append(messages,
				agentcore.Message{Role: agentcore.RoleAssistant, Content: []agentcore.ContentBlock{agentcore.TextBlock(raw)}},
				agentcore.Message{Role: agentcore.RoleUser, Content: []agentcore.ContentBlock{agentcore.TextBlock(normalizerRetryHint)}},
			)
		}
		slog.Warn("Chuẩn hóa quy tắc thất bại",
			"module", "rules", "source", source, "attempt", attempt, "err", lastErr)
		if ctx.Err() != nil {
			break // ctx bị hủy thì thử lại cũng sẽ thất bại, giáng cấp ngay
		}
	}
	return degraded(source, text)
}

// degraded tạo một ứng viên giáng cấp: khi chuẩn hóa thất bại, lấy văn bản gốc làm sở thích phong cách, không trích xuất bất kỳ quy tắc cơ học nào.
// uncertain ghi chú nguồn (tiện phản hồi lại "nguồn nào không phân tích được"), nhưng không chứa chi tiết lỗi kỹ thuật — lỗi kỹ thuật chỉ vào log.
func degraded(source, text string) rules.Candidate {
	return rules.Candidate{
		Source:      source,
		Preferences: text,
		Uncertain:   []string{source + ": chuẩn hóa thất bại, đã xử lý văn bản gốc như sở thích phong cách (không trích xuất quy tắc cơ học)"},
		Degraded:    true,
	}
}

// normalizerOutput là dạng JSON theo quy ước của bộ chuẩn hóa.
// Structured tái dùng trực tiếp rules.Structured (hình dạng JSON nhất quán); Uncertain dùng RawMessage để chấp nhận
// nhiều dạng mô hình trả về (string / []string / [{item,reason}], thực chứng prototype đều xuất hiện).
type normalizerOutput struct {
	Structured  rules.Structured `json:"structured"`
	Preferences string           `json:"preferences"`
	Uncertain   json.RawMessage  `json:"uncertain"`
}

func parseNormalizerJSON(raw string) (normalizerOutput, bool) {
	s := extractJSON(raw)
	if s == "" {
		return normalizerOutput{}, false
	}
	var out normalizerOutput
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return normalizerOutput{}, false
	}
	return out, true
}

// extractJSON trích xuất đối tượng JSON từ phản hồi mô hình: gỡ bỏ hàng rào ```json, lấy từ { đầu tiên đến } cuối cùng.
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

// coerceUncertain thống nhất uncertain mô hình trả về thành []string, chấp nhận ba dạng string / []string / []object.
func coerceUncertain(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return nonEmpty(list)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s = strings.TrimSpace(s); s != "" {
			return []string{s}
		}
		return nil
	}
	var objs []map[string]any
	if err := json.Unmarshal(raw, &objs); err == nil {
		var out []string
		for _, o := range objs {
			if str := stringifyUncertainObj(o); str != "" {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

func stringifyUncertainObj(o map[string]any) string {
	item, _ := o["item"].(string)
	if item == "" {
		item, _ = o["field"].(string)
	}
	reason, _ := o["reason"].(string)
	switch {
	case item != "" && reason != "":
		return item + "：" + reason
	case item != "":
		return item
	case reason != "":
		return reason
	default:
		b, _ := json.Marshal(o)
		return string(b)
	}
}

func nonEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// normalizerSystemPrompt là prompt hệ thống của bộ chuẩn hóa.
// Đã xác nhận đề xuất bảo thủ đúng (10/10) với 10 ví dụ thực tế (bao gồm bẫy phát minh ngưỡng).
const normalizerSystemPrompt = `Bạn là «bộ chuẩn hóa quy tắc» của hệ thống viết tiểu thuyết AI. Bạn đọc các quy tắc viết lâu dài từ một nguồn của người dùng (ngôn ngữ tự nhiên) và trích xuất thành dạng có cấu trúc. Chỉ xuất một đối tượng JSON, không có bất kỳ văn bản giải thích nào.

Đầu ra JSON có ba trường: structured / preferences / uncertain.

structured chỉ cho phép các trường sau (không có trường nào khác):
- genre: chuỗi (thể loại)
- chapter_words: {min: số nguyên, max: số nguyên} (khoảng số chữ mỗi chương)
- forbidden_chars: [chuỗi] (ký tự không được xuất hiện)
- forbidden_phrases: [chuỗi] (cụm từ không được xuất hiện, khớp chính xác theo nghĩa đen)
- fatigue_words: {từ: số nguyên} (từ mòn → giới hạn số lần xuất hiện mỗi chương)

【Đề xuất bảo thủ — quan trọng nhất】
- Chỉ ghi vào structured khi người dùng nói rõ ràng, không mơ hồ.
- forbidden_chars/forbidden_phrases là mức error: chỉ đề xuất khi có lệnh cấm rõ ràng như "không được có X / cấm X / đừng viết X".
- fatigue_words: chỉ đề xuất khi đồng thời có "từ cụ thể" và "ngưỡng số lần cụ thể"; "ít dùng X / đừng hay dùng X" mà không có số thì đặt vào preferences, tuyệt đối không tự đặt ra ngưỡng.
- chapter_words: chỉ đề xuất khi có khoảng / giới hạn trên / giới hạn dưới / số chữ mục tiêu cụ thể; "ngắn thôi / nhịp nhanh hơn" thì đặt vào preferences.
- Những gì không thể kiểm tra cơ học, không có ngưỡng rõ ràng, phụ thuộc ngữ cảnh — đều đặt vào preferences.
- Nguyên tắc: thà bỏ sót vào structured còn hơn đề xuất sai (vì sẽ báo sai mỗi chương).

preferences: sở thích phong cách / nhân vật / thẩm mỹ bằng ngôn ngữ tự nhiên, một đoạn văn có thể đọc được.
uncertain: các mục bạn cố ý không đề xuất vào structured + lý do (mảng chuỗi).`

// normalizerRetryHint được thêm vào cho mô hình khi đầu ra chuẩn hóa không thể phân tích thành JSON, hướng dẫn tạo lại có mục tiêu
// (thử lại có phản hồi, xem nhánh "trả về JSON không hợp lệ" trong Normalize).
const normalizerRetryHint = "Phản hồi trên không thể phân tích thành JSON. Vui lòng chỉ xuất đúng một đối tượng JSON, chứa ba trường structured / preferences / uncertain, không có bất kỳ văn bản giải thích hay hàng rào code nào."

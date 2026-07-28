package comic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/voocel/agentcore"
)

// maxOutputTokens là trần token cho mỗi lời gọi LLM. Kịch bản truyện tranh một chương
// gồm nhiều trang × nhiều khung × lời thoại nên cần rộng hơn adapt (8192).
const maxOutputTokens = 16384

// maxChapterRunes giới hạn rune văn bản một chương đưa vào prompt (tránh vượt cửa sổ).
const maxChapterRunes = 24000

// generateJSON gọi LLM đồng bộ, bóc khối <output>...</output> rồi unmarshal vào out.
// Bản sao của adapt/parse.go — giữ riêng theo lệ "mỗi khả năng ngang tự chứa".
func generateJSON(ctx context.Context, llm LLMChat, systemPrompt, userPrompt string, out any) error {
	if strings.TrimSpace(systemPrompt) == "" {
		return fmt.Errorf("thiếu system prompt")
	}
	resp, err := llm.Generate(ctx, []agentcore.Message{
		agentcore.SystemMsg(systemPrompt),
		agentcore.UserMsg(userPrompt),
	}, nil, agentcore.WithMaxTokens(maxOutputTokens))
	if err != nil {
		return fmt.Errorf("gọi LLM thất bại: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("LLM trả về rỗng")
	}
	body := extractOutput(resp.Message.TextContent())
	if err := parseJSONPayload(body, out); err != nil {
		return fmt.Errorf("phân tích JSON thất bại: %w", err)
	}
	return nil
}

// extractOutput lấy nội dung giữa <output> và </output>. Thiếu thẻ đóng thì lấy tới hết
// (khoan dung với output bị cắt cụt); không có thẻ mở thì trả nguyên văn.
func extractOutput(text string) string {
	const open, closeTag = "<output>", "</output>"
	lo := strings.Index(text, open)
	if lo < 0 {
		return text
	}
	rest := text[lo+len(open):]
	if hi := strings.Index(rest, closeTag); hi >= 0 {
		return rest[:hi]
	}
	return rest
}

// parseJSONPayload bóc JSON object từ text (gỡ rào ```).
func parseJSONPayload(text string, out any) error {
	body := strings.TrimSpace(text)
	if strings.HasPrefix(body, "```") {
		lines := strings.Split(body, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
			if n := len(lines); n > 0 && strings.HasPrefix(strings.TrimSpace(lines[n-1]), "```") {
				lines = lines[:n-1]
			}
			body = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	start := strings.Index(body, "{")
	end := strings.LastIndex(body, "}")
	if start < 0 || end < start {
		return fmt.Errorf("không tìm thấy JSON object trong phản hồi")
	}
	if err := json.Unmarshal([]byte(body[start:end+1]), out); err != nil {
		return fmt.Errorf("giải mã JSON: %w", err)
	}
	return nil
}

// jsonPayload gói hướng dẫn + payload JSON thành user prompt.
func jsonPayload(instruction string, payload any) string {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return instruction
	}
	return instruction + "\n\n```json\n" + string(data) + "\n```"
}

// compact cắt ngắn chuỗi theo rune, GIỮ CẢ ĐẦU VÀ ĐUÔI (giống adapt.compact).
// Giữ đuôi là có chủ đích: đoạn kết chương thường chứa cú hích, mất nó thì kịch bản
// truyện tranh sẽ hụt mất cao trào của trang cuối.
func compact(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	head := maxRunes * 3 / 4
	tail := maxRunes - head
	return string(runes[:head]) + "\n\n[...lược bớt...]\n\n" + string(runes[len(runes)-tail:])
}

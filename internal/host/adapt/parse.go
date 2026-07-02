package adapt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/voocel/agentcore"
)

// maxOutputTokens là trần token cho mỗi lời gọi LLM (chương/thiết kế có thể dài).
const maxOutputTokens = 8192

// generateJSON gọi LLM đồng bộ với system+user prompt, bóc khối <output>...</output>
// (hoặc fallback JSON object đầu tiên) và unmarshal vào out.
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

// extractOutput lấy nội dung giữa <output> và </output>. Nếu thiếu thẻ đóng thì
// lấy từ sau thẻ mở tới hết (khoan dung với output bị cắt cụt); không có thẻ mở
// thì trả nguyên văn cho parseJSONPayload tự dò JSON.
func extractOutput(text string) string {
	const open, close = "<output>", "</output>"
	lo := strings.Index(text, open)
	if lo < 0 {
		return text
	}
	rest := text[lo+len(open):]
	if hi := strings.Index(rest, close); hi >= 0 {
		return rest[:hi]
	}
	return rest
}

// parseJSONPayload bóc JSON object từ text (gỡ rào ```), giống sim/parser.go.
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

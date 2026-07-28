package comic

import (
	"context"
	"encoding/json"
	"errors"
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
	raw := body[start : end+1]

	err := json.Unmarshal([]byte(raw), out)
	if err == nil {
		return nil
	}
	// Thử sửa các lỗi cú pháp LLM hay mắc rồi giải mã lại.
	if fixed := repairJSON(raw); fixed != raw {
		if err2 := json.Unmarshal([]byte(fixed), out); err2 == nil {
			return nil
		}
	}
	return fmt.Errorf("giải mã JSON: %w%s", err, jsonErrContext(raw, err))
}

// repairJSON sửa các lỗi cú pháp mà LLM hay mắc khi sinh JSON dài.
//
// Hiện xử lý DẤU PHẨY THỪA trước `}` hoặc `]` — lỗi phổ biến nhất, và cũng là lỗi đã gặp
// thật khi sinh model sheet nhân vật (Go báo:
// "invalid character '}' looking for beginning of object key string").
//
// Bắt buộc phải bám trạng thái chuỗi: lời thoại tiếng Việt đầy dấu phẩy, cắt nhầm một dấu
// phẩy bên trong chuỗi là hỏng nội dung chứ không phải sửa cú pháp.
func repairJSON(s string) string {
	out := make([]byte, 0, len(s))
	inStr, esc := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case esc:
			esc = false
		case inStr && c == '\\':
			esc = true
		case c == '"':
			inStr = !inStr
		case !inStr && c == ',':
			// Nhìn tới ký tự không phải khoảng trắng kế tiếp; nếu là } hoặc ] thì bỏ dấu phẩy.
			j := i + 1
			for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r') {
				j++
			}
			if j < len(s) && (s[j] == '}' || s[j] == ']') {
				continue // nuốt dấu phẩy thừa
			}
		}
		out = append(out, c)
	}
	return string(out)
}

// jsonErrContext trích đoạn quanh vị trí lỗi để thông báo nói rõ hỏng ở đâu.
// Không có đoạn trích thì lỗi JSON của LLM gần như không thể chẩn đoán từ log.
func jsonErrContext(raw string, err error) string {
	var se *json.SyntaxError
	if !errors.As(err, &se) {
		return ""
	}
	off := int(se.Offset)
	lo, hi := off-60, off+60
	if lo < 0 {
		lo = 0
	}
	if hi > len(raw) {
		hi = len(raw)
	}
	if lo >= hi {
		return ""
	}
	return fmt.Sprintf(" (quanh vị trí %d: …%s…)", off, strings.ReplaceAll(raw[lo:hi], "\n", " "))
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

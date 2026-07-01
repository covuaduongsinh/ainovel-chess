package diag

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/store"
)

// SkelEvent là khung hành vi của một tin nhắn phiên sau khi khử nhận dạng: giữ lại tín hiệu cấu trúc (vai trò / tool /
// lỗi / dấu vân tay lặp), tất cả văn bản tự do (nội dung, prompt, suy nghĩ) đều bị khử nhận dạng. Đây là một
// lớp projection chặt hơn store.compactMessage — cái sau nén theo thể tích (>4KB), còn đây không quan tâm thể tích,
// bất kỳ văn bản nào cũng không được lọt ra.
type SkelEvent struct {
	Agent    string     // phiên nguồn: coordinator / writer-ch07 ...
	Role     string     // assistant / tool / user
	Tools    []SkelTool // các lời gọi tool trong tin nhắn này
	ErrClass string     // role=tool và is_error: dòng đầu của lỗi (chuỗi lỗi framework, không chứa nội dung)
	TextSha  string     // hash ngắn của nội dung đã khử nhận dạng; cùng sha = lặp tạo cùng đoạn (tín hiệu vòng lặp)
	Redacted int        // số khối văn bản/suy nghĩ đã khử nhận dạng trong tin nhắn này (dùng để tự kiểm tra)
}

// SkelTool là projection đã khử nhận dạng của một lần gọi tool.
type SkelTool struct {
	Name     string            // tên tool (tín hiệu cấu trúc, không chứa nội dung)
	Args     map[string]string // key → giá trị vô hướng gốc / chuỗi ngắn có dấu ngoặc / "<redacted len sha>"
	Invalid  bool              // ArgsInvalid: tham số model gửi không thể phân tích (#34 signal)
	ParseErr string            // ArgsParseError: nguyên nhân phân tích thất bại
}

// redactMessage project một agentcore.Message thành khung hành vi.
func redactMessage(agent string, m agentcore.Message) SkelEvent {
	ev := SkelEvent{Agent: agent, Role: string(m.Role)}
	isErr, _ := m.Metadata["is_error"].(bool)

	var text strings.Builder
	for _, b := range m.Content {
		switch b.Type {
		case agentcore.ContentText:
			// Kết quả lỗi tool giữ lại dòng đầu: đây là chuỗi lỗi của chúng ta (ví dụ InputValidationError),
			// không chứa nội dung, và là chìa khóa để định vị vòng lặp. Các văn bản còn lại đều bị khử nhận dạng.
			if m.Role == agentcore.RoleTool && isErr && ev.ErrClass == "" {
				ev.ErrClass = firstLine(b.Text, 160)
				continue
			}
			if strings.TrimSpace(b.Text) != "" {
				text.WriteString(b.Text)
				ev.Redacted++
			}
		case agentcore.ContentThinking:
			if strings.TrimSpace(b.Thinking) != "" {
				text.WriteString(b.Thinking)
				ev.Redacted++
			}
		case agentcore.ContentToolCall:
			if b.ToolCall != nil {
				ev.Tools = append(ev.Tools, redactToolCall(b.ToolCall))
			}
		}
	}
	if t := text.String(); t != "" {
		ev.TextSha = shortHash(t)
	}
	return ev
}

// redactToolCall project một lần gọi tool: tên tool + tham số (giá trị đã khử nhận dạng) + đánh dấu lỗi phân tích.
func redactToolCall(tc *agentcore.ToolCall) SkelTool {
	return SkelTool{
		Name:     tc.Name,
		Args:     redactArgs(tc.Args),
		Invalid:  tc.ArgsInvalid,
		ParseErr: tc.ArgsParseError,
	}
}

// redactArgs project đối tượng tham số tool thành key → giá trị đã khử nhận dạng. Tham số không phải đối tượng trả về nil
// (ArgsInvalid/ParseErr đã được ghi riêng trong SkelTool).
func redactArgs(raw json.RawMessage) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = projectValue(v)
	}
	return out
}

// projectValue project giá trị tham số đơn lẻ theo kiểu JSON:
//   - Vô hướng (số / bool / null): giá trị gốc là tín hiệu cấu trúc, giữ nguyên (chapter: 7)
//   - Chuỗi ngắn kiểu identifier: giữ với dấu ngoặc kép, lộ kiểu (chapter: "7" ← tín hiệu stringify số của #34)
//   - Chuỗi chứa tiếng Trung / khoảng trắng / văn bản dài, đối tượng, mảng: khử nhận dạng thành <redacted …> (nội dung không lọt ra)
//   - Đã là placeholder [session_compact: …]: an toàn và có thông tin, giữ nguyên
func projectValue(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return ""
	}
	switch s[0] {
	case '"':
		var str string
		if err := json.Unmarshal(raw, &str); err != nil {
			return redactPlaceholder(s)
		}
		if strings.HasPrefix(str, store.CompactTag) {
			return str
		}
		// Chỉ giữ lại giá trị ngắn "giống identifier/số/enum" (chapter:"7", type:"premise", agent:"writer");
		// bất kỳ chuỗi nào chứa tiếng Trung, khoảng trắng hoặc ký hiệu khác đều coi là nội dung, đều bị khử nhận dạng.
		if utf8.RuneCountInString(str) <= 32 && isStructuralToken(str) {
			return strconv.Quote(str)
		}
		return redactPlaceholder(str)
	case '{':
		return fmt.Sprintf("<redacted object len=%d>", len(raw))
	case '[':
		return fmt.Sprintf("<redacted array len=%d>", len(raw))
	default:
		return s
	}
}

// isStructuralToken kiểm tra chuỗi có "giống identifier" không — ASCII thuần chữ cái / số / `_-.:/`,
// không có khoảng trắng, không có tiếng Trung. Dùng để phân biệt tín hiệu cấu trúc (giữ lại) và đoạn nội dung (khử nhận dạng).
func isStructuralToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.' || r == ':' || r == '/':
		default:
			return false
		}
	}
	return true
}

func redactPlaceholder(s string) string {
	return fmt.Sprintf("<redacted len=%d sha=%s>", utf8.RuneCountInString(s), shortHash(s))
}

// shortHash lấy hash ngắn của văn bản; chỉ dùng để phán định "có phải cùng một đoạn văn bản xuất hiện lặp lại không", không phải mục đích mã hóa.
func shortHash(s string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return fmt.Sprintf("%08x", h.Sum32())
}

// firstLine lấy dòng đầu và cắt ngắn theo rune, dùng cho tóm tắt chuỗi lỗi.
func firstLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\n\r"); i >= 0 {
		s = s[:i]
	}
	if utf8.RuneCountInString(s) > max {
		r := []rune(s)
		s = string(r[:max]) + "…"
	}
	return s
}

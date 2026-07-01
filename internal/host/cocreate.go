package host

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/store"
)

// Đồng sáng tác khởi động lạnh: làm rõ yêu cầu từ đầu, tạo ra lệnh sáng tác cho cả cuốn sách.
const coCreateSystemPrompt = `Bạn là trợ lý đồng sáng tác tiểu thuyết. Nhiệm vụ của bạn không phải trực tiếp bắt đầu viết tiểu thuyết, mà là thông qua nhiều vòng hội thoại ngắn giúp người dùng làm rõ yêu cầu sáng tác, đồng thời liên tục tổng hợp ra một đoạn lệnh sáng tác có thể giao trực tiếp cho engine sáng tác.

Mỗi vòng phản hồi phải xuất theo đúng định dạng XML sau, gồm bốn thẻ xuất hiện theo thứ tự, mỗi thẻ phải có thẻ mở và đóng đúng:

<reply>
Phản hồi tự nhiên cho người dùng: trả lời input của người dùng trước, rồi đặt tối đa 1-2 câu hỏi quan trọng nhất hiện tại. Nếu thông tin đã đủ để bắt đầu sáng tác, hãy nói với người dùng có thể nhấn Ctrl+S để bắt đầu.
</reply>

<draft>
Bản nháp lệnh sáng tác đầy đủ hiện tại, dùng Markdown: bắt đầu trực tiếp từ tiêu đề cấp hai, ví dụ "## Chủ đề", "## Yếu tố chính", "## Thông tin cần làm rõ"; dùng bullet để liệt kê các điểm. Mỗi vòng phải **tích lũy cập nhật** trên các kết luận đã có, hấp thụ ý định mới nhất của người dùng; dù vòng này không có gì mới cũng phải viết lại toàn bộ bản nháp nguyên vẹn — không được bỏ qua, không được viết "(giữ nguyên vòng trước)" hay kiểu placeholder tương tự.
</draft>
` + coCreateProtocolTail

// Đồng sáng tác giai đoạn: tiểu thuyết đã viết được một phần, lên kế hoạch hướng đi "giai đoạn tiếp theo". Bên gọi cần
// nối tóm tắt trạng thái câu chuyện hiện tại vào sau prompt này (đoạn "## Trạng thái câu chuyện hiện tại"),
// để mô hình lên kế hoạch dựa trên nội dung đã viết.
const stageCoCreateSystemPrompt = `Bạn là trợ lý "đồng sáng tác giai đoạn" của tiểu thuyết. Cuốn tiểu thuyết này đã viết được một phần (xem tiến độ ở phần "Trạng thái câu chuyện hiện tại" bên dưới). Người dùng tạm dừng lại, muốn cùng bạn lên kế hoạch hướng đi "giai đoạn tiếp theo" rồi tiếp tục sáng tác.

Nhiệm vụ của bạn không phải tiếp tục viết chính văn, mà là thông qua nhiều vòng hội thoại ngắn giúp người dùng nghĩ rõ đoạn sau (vài chương tới / cung tiếp theo / tập tiếp theo) sẽ đi theo hướng nào, đồng thời liên tục tổng hợp ra một "brief hướng đi tiếp theo" để engine sáng tác dựa vào tiếp tục.

Nguyên tắc bất biến: mọi gợi ý phải nhất quán với cốt truyện, nhân vật, phục bút đã xảy ra trong "Trạng thái câu chuyện hiện tại", tuyệt đối không lật ngược hoặc bỏ qua nội dung đã viết; chỉ lên kế hoạch "tiếp theo sẽ đi thế nào", không thiết kế lại toàn bộ cuốn sách.

Mỗi vòng phản hồi phải xuất theo đúng định dạng XML sau, gồm bốn thẻ xuất hiện theo thứ tự, mỗi thẻ phải có thẻ mở và đóng đúng:

<reply>
Phản hồi tự nhiên cho người dùng: trả lời input của người dùng trước, rồi đặt tối đa 1-2 câu hỏi quan trọng nhất hiện tại. Nếu hướng đi tiếp theo đã đủ rõ ràng, hãy nói với người dùng có thể nhấn Ctrl+S để giao hướng đi cho engine sáng tác tiếp tục.
</reply>

<draft>
"Brief hướng đi tiếp theo" đầy đủ hiện tại, dùng Markdown: bắt đầu trực tiếp từ tiêu đề cấp hai, ví dụ "## Hướng đi tiếp theo", "## Bước ngoặt chính", "## Phục bút cần thu", "## Nhịp điệu và độ dài"; dùng bullet để liệt kê các điểm. Mỗi vòng phải **tích lũy cập nhật** trên các kết luận đã có, hấp thụ ý định mới nhất của người dùng; dù vòng này không có gì mới cũng phải viết lại toàn bộ brief nguyên vẹn — không được bỏ qua, không được viết "(giữ nguyên vòng trước)" hay kiểu placeholder tương tự.
</draft>
` + coCreateProtocolTail

// coCreateProtocolTail là phần đuôi giao thức xuất dùng chung cho cả hai chế độ đồng sáng tác (<ready> / <suggestions> + quy chuẩn xuất).
// Hai chế độ chỉ khác nhau ở ngữ cảnh mở đầu và ngữ nghĩa của <draft>, giao thức hoàn toàn giống nhau.
const coCreateProtocolTail = `
<ready>false</ready>

<suggestions>
1-3 "câu người dùng có thể muốn nói tiếp theo", mỗi dòng một câu bắt đầu bằng "- ". Đây là gợi ý khi người dùng bí ý,
nhấn phím số để điền vào ô nhập liệu, người dùng có thể chỉnh sửa rồi gửi.

Yêu cầu:
- Đặt từ góc nhìn người dùng, như người dùng đang nói với bạn, không viết thành câu hỏi ngược của trợ lý.
- Mỗi câu không quá 25 chữ, đa dạng hóa câu từ, tránh rập khuôn.
- Đưa ra xu hướng / lựa chọn / bổ sung ý định, không dùng một câu viết hộ toàn bộ thiết định cho người dùng.
</suggestions>

Quy chuẩn xuất:
- Bắt buộc dùng bốn thẻ XML: <reply> / <draft> / <ready> / <suggestions>, mỗi thẻ phải mở và đóng đầy đủ.
- Tên thẻ chỉ dùng chữ thường tiếng Anh, không viết lại thành <REPLY> / <REWRITE> / <trả lời> hay bất kỳ biến thể nào.
- Ngoài thẻ không được thêm bất kỳ giải thích, suy nghĩ hay code fence nào.
- Bên trong <draft> cho phép Markdown nhiều dòng, xuống dòng trực tiếp, không cần escape.
- <ready> chỉ viết true hoặc false. Điền true khi thông tin đã đủ.
- Khi <ready>true</ready> thì <suggestions> có thể rỗng (giữ thẻ rỗng <suggestions></suggestions> là được).`

// CoCreateProgressKind xác định loại nội dung của callback luồng.
const (
	CoCreateProgressThinking = "thinking"
	CoCreateProgressReply    = "reply"
)

// Xuất bốn đoạn theo thẻ XML. Phong cách XML bền vững hơn marker dấu ngoặc vuông — dữ liệu training của Claude/GPT
// có rất nhiều định dạng <thinking>...</thinking>, mô hình hầu như không viết lại <reply> thành <REWRITE>
// hay biến thể nào khác; thẻ đóng cũng giúp cắt đứt giữa chừng của luồng chính xác hơn (không cần tìm marker tiếp theo để cắt đuôi).
const (
	tagReply       = "reply"
	tagDraft       = "draft"
	tagReady       = "ready"
	tagSuggestions = "suggestions"
)

func coCreateStream(ctx context.Context, models *bootstrap.ModelSet, sessions *store.SessionStore, sysPrompt string, history []CoCreateMessage, onProgress func(kind, text string)) (reply CoCreateReply, err error) {
	if len(history) == 0 {
		return CoCreateReply{}, fmt.Errorf("cocreate history is empty")
	}

	model := models.ForRole("thinking")
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()

	msgs := []agentcore.Message{agentcore.SystemMsg(sysPrompt)}
	for _, item := range history {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(item.Role)) {
		case "assistant":
			msgs = append(msgs, assistantMsg(content))
		default:
			msgs = append(msgs, agentcore.UserMsg(content))
		}
	}

	var raw, thinking strings.Builder

	// Tra cứu các vấn đề không thường xuyên như "cocreate empty response" cần xem mô hình thực sự trả về gì.
	// Mỗi vòng toàn bộ được ghi xuống <output>/meta/sessions/cocreate.jsonl, cùng vị trí với log session sáng tác chính thức.
	start := time.Now()
	defer func() {
		if sessions == nil {
			return
		}
		_ = sessions.LogCoCreate(coCreateLogEntry{
			Time:         time.Now(),
			DurationMS:   time.Since(start).Milliseconds(),
			InputHistory: history,
			RawResponse:  raw.String(),
			RawLen:       len([]rune(raw.String())),
			Thinking:     thinking.String(),
			ParsedReply:  reply.Message,
			ParsedDraft:  reply.Prompt,
			ParsedReady:  reply.Ready,
			ParsedSugs:   reply.Suggestions,
			Error:        errString(err),
		})
	}()

	streamCh, err := model.GenerateStream(ctx, msgs, nil, agentcore.WithMaxTokens(2048))
	if err != nil {
		return CoCreateReply{}, fmt.Errorf("cocreate generate: %w", err)
	}

	var streamed bool
	for ev := range streamCh {
		switch ev.Type {
		case agentcore.StreamEventThinkingDelta:
			thinking.WriteString(ev.Delta)
			if onProgress != nil {
				onProgress(CoCreateProgressThinking, thinking.String())
			}
		case agentcore.StreamEventTextDelta:
			streamed = true
			raw.WriteString(ev.Delta)
			if onProgress != nil {
				onProgress(CoCreateProgressReply, extractReplyPreview(raw.String()))
			}
		case agentcore.StreamEventDone:
			if !streamed {
				raw.WriteString(ev.Message.TextContent())
			}
		case agentcore.StreamEventError:
			if ev.Err != nil {
				return CoCreateReply{}, fmt.Errorf("cocreate generate: %w", ev.Err)
			}
			return CoCreateReply{}, fmt.Errorf("cocreate generate failed")
		}
	}

	// Fallback channel: mô hình kiểu suy nghĩ (R1/GLM-Z1/QwQ, v.v.) đôi khi viết toàn bộ câu trả lời vào
	// reasoning_content rồi không chuyển sang kênh final answer, khiến raw rỗng nhưng thinking chứa
	// đủ bốn đoạn. Thực nghiệm trong meta/sessions/cocreate.jsonl — dùng trực tiếp thinking như raw để parse,
	// tầng giao thức đã có xử lý hạ cấp (khi không có marker [REPLY] thì cả đoạn làm reply), sau khi cứu vãn trải nghiệm UI không khác gì.
	rawText := raw.String()
	if strings.TrimSpace(rawText) == "" {
		if t := strings.TrimSpace(thinking.String()); t != "" {
			rawText = t
		}
	}
	reply, err = parseCoCreateResponse(rawText)
	return reply, err
}

// coCreateLogEntry là cấu trúc một dòng ghi vào meta/sessions/cocreate.jsonl.
// Tên trường gần với thói quen tra cứu trực tiếp jsonl (snake_case), tiện lọc bằng jq.
type coCreateLogEntry struct {
	Time         time.Time         `json:"time"`
	DurationMS   int64             `json:"duration_ms"`
	InputHistory []CoCreateMessage `json:"input_history"`
	RawResponse  string            `json:"raw_response"`
	RawLen       int               `json:"raw_len"`
	Thinking     string            `json:"thinking,omitempty"`
	ParsedReply  string            `json:"parsed_reply"`
	ParsedDraft  string            `json:"parsed_draft"`
	ParsedReady  bool              `json:"parsed_ready"`
	ParsedSugs   []string          `json:"parsed_sugs,omitempty"`
	Error        string            `json:"error,omitempty"`
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func assistantMsg(text string) agentcore.Message {
	return agentcore.Message{
		Role:      agentcore.RoleAssistant,
		Content:   []agentcore.ContentBlock{agentcore.TextBlock(text)},
		Timestamp: time.Now(),
	}
}

// parseCoCreateResponse phân tích xuất thẻ XML. Nếu mô hình không tuân thủ giao thức (nói tự nhiên trực tiếp),
// toàn bộ đoạn hiển thị như reply, draft để rỗng để session giữ lại vòng trước.
func parseCoCreateResponse(raw string) (CoCreateReply, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return CoCreateReply{}, fmt.Errorf("cocreate empty response")
	}

	reply, draft, ready, suggestions := splitCoCreateMarkers(raw)
	if reply == "" {
		// Mô hình không tuân thủ giao thức XML: cả đoạn làm reply.
		return CoCreateReply{Message: raw, Prompt: "", Ready: false, Raw: raw}, nil
	}
	return CoCreateReply{
		Message:     reply,
		Prompt:      draft,
		Ready:       ready,
		Suggestions: suggestions,
		Raw:         raw,
	}, nil
}

// splitCoCreateMarkers cắt văn bản theo bốn thẻ XML.
// Thẻ có thể thiếu (luồng bị cắt giữa chừng hoặc mô hình bỏ sót), phần thiếu tương ứng trường rỗng / false / nil.
// Khi thiếu thẻ đóng, extractTagContent sẽ lấy đến cuối chuỗi, vẫn cố parse hết sức.
func splitCoCreateMarkers(s string) (reply, draft string, ready bool, suggestions []string) {
	reply = extractTagContent(s, tagReply)
	draft = extractTagContent(s, tagDraft)
	readyStr := strings.ToLower(extractTagContent(s, tagReady))
	ready = readyStr == "true" || readyStr == "yes"
	suggestions = parseSuggestions(extractTagContent(s, tagSuggestions))
	return
}

// extractTagContent trích xuất văn bản giữa <tag>...</tag> từ s.
// Xử lý dự phòng ba tình huống lỗi bất ngờ, tránh rơi thẳng vào hạ cấp mất trường:
//  1. Có mở không có đóng (luồng bị cắt giữa chừng) → cắt đến trước thẻ mở đã biết tiếp theo
//  2. Không mở có đóng (mô hình gõ nhầm, như <suggestions> thành <uggestions>) → từ vị trí kết thúc
//     của thẻ đóng đầy đủ đã biết gần nhất, đến trước </tag>
//  3. reply hoàn toàn không có thẻ mở (mô hình mở đầu bằng ngôn ngữ tự nhiên, cuối dán </reply>) → từ đầu đến </reply>
func extractTagContent(s, tag string) string {
	open := "<" + tag + ">"
	closeTag := "</" + tag + ">"
	oIdx := strings.Index(s, open)
	if oIdx >= 0 {
		rest := s[oIdx+len(open):]
		if cIdx := strings.Index(rest, closeTag); cIdx >= 0 {
			return strings.TrimSpace(rest[:cIdx])
		}
		// Có mở không có đóng → cắt đến trước thẻ mở đã biết tiếp theo
		for _, other := range []string{"<reply>", "<draft>", "<ready>", "<suggestions>"} {
			if other == open {
				continue
			}
			if idx := strings.Index(rest, other); idx >= 0 {
				rest = rest[:idx]
			}
		}
		return strings.TrimSpace(rest)
	}

	// Không mở có đóng → từ vị trí kết thúc của thẻ đóng đầy đủ đã biết gần nhất, đến </tag>.
	if cIdx := strings.Index(s, closeTag); cIdx >= 0 {
		prefix := s[:cIdx]
		start := 0
		for _, t := range []string{"</reply>", "</draft>", "</ready>", "</suggestions>"} {
			if t == closeTag {
				continue
			}
			if i := strings.LastIndex(prefix, t); i >= 0 {
				if end := i + len(t); end > start {
					start = end
				}
			}
		}
		return strings.TrimSpace(prefix[start:])
	}
	return ""
}

// parseSuggestions trích từng dòng của đoạn <suggestions>, bỏ tiền tố danh sách "- " / "* " / "1. " v.v.
// Giữ tối đa 3 mục; bỏ qua dòng trống, quá ngắn (<2 ký tự), hoặc cả dòng trông như thẻ XML (dư cặn thẻ mở gõ nhầm,
// ví dụ <uggestions>).
func parseSuggestions(text string) []string {
	if text == "" {
		return nil
	}
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Cả dòng trông như thẻ XML → bỏ qua (ngăn ô nhiễm thẻ mở gõ nhầm)
		if strings.HasPrefix(line, "<") && strings.HasSuffix(line, ">") {
			continue
		}
		// Bỏ tiền tố danh sách
		switch {
		case strings.HasPrefix(line, "- "):
			line = strings.TrimSpace(line[2:])
		case strings.HasPrefix(line, "* "):
			line = strings.TrimSpace(line[2:])
		case isOrderedSuggestion(line):
			line = stripOrderedPrefix(line)
		}
		if len([]rune(line)) < 2 {
			continue
		}
		out = append(out, line)
		if len(out) >= 3 {
			break
		}
	}
	return out
}

// isOrderedSuggestion kiểm tra đầu dòng có dạng "1. " / "12. " (số + chấm + dấu cách) không.
func isOrderedSuggestion(line string) bool {
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	return i > 0 && i+1 < len(line) && line[i] == '.' && line[i+1] == ' '
}

func stripOrderedPrefix(line string) string {
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 || i+1 >= len(line) {
		return line
	}
	return strings.TrimSpace(line[i+2:])
}

// extractReplyPreview xem trước luồng: khi raw còn đang tăng trưởng, cung cấp cho UI một đoạn văn bản có thể hiển thị.
// Tìm nội dung sau <reply>, cắt đến </reply> hoặc trước thẻ mở <draft> tiếp theo.
// Khi mô hình tuân thủ một nửa (thiếu thẻ mở <reply>), từ đầu đến </reply> hoặc <draft> đều tính là reply.
func extractReplyPreview(raw string) string {
	trimmed := strings.TrimSpace(raw)
	open := "<" + tagReply + ">"
	closeTag := "</" + tagReply + ">"
	draftOpen := "<" + tagDraft + ">"

	rest := trimmed
	if rIdx := strings.Index(trimmed, open); rIdx >= 0 {
		rest = trimmed[rIdx+len(open):]
	}
	if cIdx := strings.Index(rest, closeTag); cIdx >= 0 {
		return strings.TrimSpace(rest[:cIdx])
	}
	if dIdx := strings.Index(rest, draftOpen); dIdx >= 0 {
		rest = rest[:dIdx]
	}
	return strings.TrimSpace(rest)
}

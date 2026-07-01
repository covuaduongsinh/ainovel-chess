package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/voocel/agentcore/schema"
)

// AskUserResponse kết quả câu trả lời của người dùng.
type AskUserResponse struct {
	Answers map[string]string // question text → câu trả lời người dùng chọn
	Notes   map[string]string // question text → nhập tự do (khi chọn "khác")
}

// AskUserHandler chặn chờ người dùng trả lời, được tiêm bởi CLI hoặc TUI.
type AskUserHandler func(ctx context.Context, questions []Question) (*AskUserResponse, error)

// Question một câu hỏi đơn lẻ.
type Question struct {
	Question    string   `json:"question"`
	Header      string   `json:"header"`
	Options     []Option `json:"options"`
	MultiSelect bool     `json:"multiSelect"`
}

// Option lựa chọn có thể chọn.
type Option struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// AskUserTool cho phép LLM đặt câu hỏi có cấu trúc đến người dùng.
type AskUserTool struct {
	mu      sync.RWMutex
	handler AskUserHandler
}

func NewAskUserTool() *AskUserTool {
	return &AskUserTool{}
}

// SetHandler tiêm UI callback, CLI và TUI tự triển khai.
func (t *AskUserTool) SetHandler(h AskUserHandler) {
	t.mu.Lock()
	t.handler = h
	t.mu.Unlock()
}

func (t *AskUserTool) Name() string  { return "ask_user" }
func (t *AskUserTool) Label() string { return "Hỏi người dùng" }

// Công cụ tương tác: chặn chờ câu trả lời của người dùng, rõ ràng không thể lên lịch song song.
func (t *AskUserTool) ReadOnly(_ json.RawMessage) bool        { return false }
func (t *AskUserTool) ConcurrencySafe(_ json.RawMessage) bool { return false }
func (t *AskUserTool) Description() string {
	return "Khi thông tin yêu cầu không đủ và thông tin còn thiếu sẽ ảnh hưởng rõ ràng đến hướng hoạch định, đặt 1-4 câu hỏi có cấu trúc cho người dùng. Mỗi câu hỏi phải chứa header, question và 2-4 lựa chọn; người dùng có thể chọn lựa chọn đặt sẵn hoặc bổ sung tự do. Kết quả trả về là tóm tắt có thể đọc trực tiếp, định dạng tương tự: Người dùng trả lời: [độ dài] trường thiên; [trọng tâm] leo thang tình tiết (bổ sung: không harem). Chỉ dùng khi không thể ổn định phán đoán độ dài, trọng tâm cốt truyện, ràng buộc quan trọng hoặc sở thích rõ ràng; không đẩy những câu hỏi có thể tự suy luận hợp lý cho người dùng."
}

func (t *AskUserTool) Schema() map[string]any {
	option := schema.Object(
		schema.Property("label", schema.String("văn bản hiển thị lựa chọn (1-5 từ)")).Required(),
		schema.Property("description", schema.String("giải thích ý nghĩa lựa chọn")).Required(),
	)
	question := schema.Object(
		schema.Property("question", schema.String("văn bản câu hỏi đầy đủ")).Required(),
		schema.Property("header", schema.String("nhãn ngắn (tối đa 12 ký tự)")).Required(),
		schema.Property("options", schema.Array("2-4 lựa chọn", option)).Required(),
		schema.Property("multiSelect", schema.Bool("có cho phép chọn nhiều không")),
	)
	return schema.Object(
		schema.Property("questions", schema.Array("1-4 câu hỏi", question)).Required(),
	)
}

type askUserArgs struct {
	Questions []Question `json:"questions"`
}

func (t *AskUserTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a askUserArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if err := validateQuestions(a.Questions); err != nil {
		return json.Marshal(fmt.Sprintf("kiểm tra tham số thất bại: %s", err))
	}

	t.mu.RLock()
	h := t.handler
	t.mu.RUnlock()

	if h == nil {
		return json.Marshal("Môi trường hiện tại không hỗ trợ hỏi tương tác, vui lòng tự quyết định theo phán đoán của bạn và tiếp tục.")
	}

	resp, err := h(ctx, a.Questions)
	if err != nil {
		return json.Marshal(fmt.Sprintf("tương tác người dùng thất bại: %s. Vui lòng tự quyết định theo phán đoán của bạn và tiếp tục.", err))
	}

	return json.Marshal(formatAnswers(a.Questions, resp))
}

func validateQuestions(questions []Question) error {
	if len(questions) == 0 {
		return fmt.Errorf("cần ít nhất một câu hỏi")
	}
	if len(questions) > 4 {
		return fmt.Errorf("tối đa 4 câu hỏi, hiện tại %d câu", len(questions))
	}
	for i, q := range questions {
		if q.Question == "" {
			return fmt.Errorf("câu hỏi %d: văn bản câu hỏi không được để trống", i+1)
		}
		if q.Header == "" {
			return fmt.Errorf("câu hỏi %d: header không được để trống", i+1)
		}
		if utf8.RuneCountInString(q.Header) > 12 {
			return fmt.Errorf("câu hỏi %d: header %q vượt quá 12 ký tự", i+1, q.Header)
		}
		if len(q.Options) < 2 || len(q.Options) > 4 {
			return fmt.Errorf("câu hỏi %d: cần 2-4 lựa chọn, hiện tại %d cái", i+1, len(q.Options))
		}
		for j, opt := range q.Options {
			if opt.Label == "" {
				return fmt.Errorf("câu hỏi %d lựa chọn %d: label không được để trống", i+1, j+1)
			}
			if opt.Description == "" {
				return fmt.Errorf("câu hỏi %d lựa chọn %d: description không được để trống", i+1, j+1)
			}
		}
	}
	return nil
}

func formatAnswers(questions []Question, resp *AskUserResponse) string {
	if resp == nil || len(resp.Answers) == 0 {
		return "Người dùng không cung cấp câu trả lời, vui lòng tự quyết định theo phán đoán của bạn và tiếp tục."
	}
	var parts []string
	for _, q := range questions {
		answer, ok := resp.Answers[q.Question]
		if !ok {
			continue
		}
		entry := fmt.Sprintf("[%s] %s", q.Header, answer)
		if note, hasNote := resp.Notes[q.Question]; hasNote {
			entry += "(bổ sung: " + note + ")"
		}
		parts = append(parts, entry)
	}
	return fmt.Sprintf("Người dùng trả lời: %s", strings.Join(parts, "; "))
}

package userrules

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/voocel/agentcore"
)

func TestExtractJSON_StripsCodeFences(t *testing.T) {
	cases := []struct{ in, wantHas string }{
		{"```json\n{\"a\":1}\n```", `"a":1`},
		{"```\n{\"a\":1}\n```", `"a":1`},
		{"tiền tố giải thích\n{\"a\":1}\nsau tố", `"a":1`},
		{"{\"a\":1}", `"a":1`},
	}
	for _, c := range cases {
		got := extractJSON(c.in)
		if got == "" {
			t.Fatalf("extractJSON(%q) trả về rỗng", c.in)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(got), &m); err != nil {
			t.Fatalf("extractJSON(%q)=%q không phải JSON hợp lệ: %v", c.in, got, err)
		}
	}
	if extractJSON("không có JSON nào cả") != "" {
		t.Fatal("khi không có JSON nên trả về chuỗi rỗng")
	}
}

func TestCoerceUncertain_HandlesAllDriftForms(t *testing.T) {
	// Thực chứng prototype: uncertain lúc là chuỗi, lúc là []string, lúc là [{item,reason}].
	cases := []struct {
		name string
		raw  string
		want int // số mục kỳ vọng (>0 là được, kiểm tra không mất)
	}{
		{"array_of_string", `["ít dùng ẩn dụ: không có ngưỡng"]`, 1},
		{"plain_string", `"chapter_words quá mơ hồ chưa nâng cấp"`, 1},
		{"array_of_object", `[{"item":"ít dùng ẩn dụ","reason":"không có ngưỡng rõ ràng"}]`, 1},
		{"array_of_field_object", `[{"field":"chapter_words.min","reason":"chưa cung cấp giới hạn dưới"}]`, 1},
		{"empty_array", `[]`, 0},
		{"empty_string", `""`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := coerceUncertain(json.RawMessage(c.raw))
			if len(got) != c.want {
				t.Fatalf("coerceUncertain(%s)=%v, kỳ vọng %d mục", c.raw, got, c.want)
			}
		})
	}
}

func TestParseNormalizerJSON_FullOutput(t *testing.T) {
	raw := "```json\n" + `{
  "structured": {"chapter_words": {"min": 1200, "max": 1600}, "forbidden_phrases": ["ở một mức độ nào đó"]},
  "preferences": "nhân vật chính bình tĩnh kiềm chế",
  "uncertain": [{"item": "ít dùng ẩn dụ", "reason": "không có ngưỡng"}]
}` + "\n```"
	out, ok := parseNormalizerJSON(raw)
	if !ok {
		t.Fatal("nên phân tích thành công")
	}
	if out.Structured.ChapterWords == nil || out.Structured.ChapterWords.Min != 1200 {
		t.Fatalf("lỗi phân tích chapter_words: %+v", out.Structured.ChapterWords)
	}
	if len(out.Structured.ForbiddenPhrases) != 1 || out.Structured.ForbiddenPhrases[0] != "ở một mức độ nào đó" {
		t.Fatalf("lỗi phân tích forbidden_phrases: %v", out.Structured.ForbiddenPhrases)
	}
	if out.Preferences != "nhân vật chính bình tĩnh kiềm chế" {
		t.Fatalf("lỗi phân tích preferences: %q", out.Preferences)
	}
	if got := coerceUncertain(out.Uncertain); len(got) != 1 {
		t.Fatalf("uncertain nên có 1 mục, got %v", got)
	}
}

func TestParseNormalizerJSON_GarbageFails(t *testing.T) {
	if _, ok := parseNormalizerJSON("mô hình chỉ trả về một câu, không có JSON"); ok {
		t.Fatal("không có JSON nên phân tích thất bại (kích hoạt giáng cấp)")
	}
	if _, ok := parseNormalizerJSON("{ không đầy đủ"); ok {
		t.Fatal("JSON không đầy đủ nên phân tích thất bại")
	}
}

func TestNormalize_NilModelDegrades(t *testing.T) {
	// Không có mô hình: toàn bộ giáng cấp thành raw preferences, không tạo structured, không bao giờ panic/trả lỗi.
	var n *Normalizer = NewNormalizer(nil)
	cand := n.Normalize(t.Context(), "startup_prompt", "mỗi chương 1200 chữ, nhân vật chính bình tĩnh")
	if !cand.Degraded {
		t.Fatal("không có mô hình nên giáng cấp")
	}
	if cand.Preferences == "" {
		t.Fatal("giáng cấp nên giữ văn bản gốc làm preferences")
	}
	if cand.Structured.ChapterWords != nil {
		t.Fatal("giáng cấp không nên tạo structured")
	}
}

// scriptedModel là ChatModel giả tối thiểu: thải ra phản hồi đặt trước theo thứ tự gọi, và ghi lại
// messages nhận được ở vòng cuối, để kiểm tra xem thử lại có phản hồi có đưa gợi ý sửa vào vòng hội thoại tiếp không. Khi hết phản hồi thì lặp lại cái cuối cùng.
type scriptedModel struct {
	replies  []string
	calls    int
	lastMsgs []agentcore.Message
	lastCfg  agentcore.CallConfig
}

func (m *scriptedModel) Generate(_ context.Context, messages []agentcore.Message, _ []agentcore.ToolSpec, opts ...agentcore.CallOption) (*agentcore.LLMResponse, error) {
	var cfg agentcore.CallConfig
	for _, o := range opts {
		o(&cfg)
	}
	m.lastCfg = cfg
	m.lastMsgs = messages
	i := m.calls
	m.calls++
	if i >= len(m.replies) {
		i = len(m.replies) - 1
	}
	return &agentcore.LLMResponse{Message: agentcore.Message{
		Role:    agentcore.RoleAssistant,
		Content: []agentcore.ContentBlock{agentcore.TextBlock(m.replies[i])},
	}}, nil
}

func (m *scriptedModel) GenerateStream(context.Context, []agentcore.Message, []agentcore.ToolSpec, ...agentcore.CallOption) (<-chan agentcore.StreamEvent, error) {
	return nil, nil
}

func (m *scriptedModel) SupportsTools() bool { return false }

// Thử lại có phản hồi: vòng đầu thải ra JSON xấu, vòng thứ hai mới hợp lệ. Normalize nên thành công, và vòng thứ hai
// phải kèm đầu ra xấu của vòng trước với gợi ý sửa (có phản hồi, không phải thử lại mù nguyên xi).
func TestNormalize_FeedbackRetryRecovers(t *testing.T) {
	model := &scriptedModel{replies: []string{
		"đây không phải JSON",
		`{"structured":{"chapter_words":{"min":1200,"max":1600}},"preferences":"","uncertain":[]}`,
	}}
	n := NewNormalizer(model)

	cand := n.Normalize(t.Context(), "startup_prompt", "mỗi chương 1200 đến 1600 chữ")
	if cand.Degraded {
		t.Fatal("vòng thứ hai đã trả về JSON hợp lệ, không nên giáng cấp")
	}
	if cand.Structured.ChapterWords == nil || cand.Structured.ChapterWords.Min != 1200 {
		t.Fatalf("nên phân tích ra chapter_words, got %+v", cand.Structured)
	}
	if model.calls != 2 {
		t.Fatalf("nên thành công ở lần thứ 2, thực tế gọi %d lần", model.calls)
	}

	var sawBad, sawHint bool
	for _, msg := range model.lastMsgs {
		switch msg.TextContent() {
		case "đây không phải JSON":
			sawBad = true
		case normalizerRetryHint:
			sawHint = true
		}
	}
	if !sawBad || !sawHint {
		t.Errorf("vòng thứ hai nên kèm đầu ra xấu của vòng trước và gợi ý sửa, sawBad=%v sawHint=%v", sawBad, sawHint)
	}
}

// Chuẩn hóa là trích xuất cơ học: với mô hình hỗ trợ tắt tư duy nên tắt tư duy tường minh, và dành đủ max_tokens cho JSON.
// scriptedModel chưa triển khai CapabilityProvider → chiến lược tư duy mặc định cho phép off → nên Resolve thành off.
func TestNormalize_DisablesThinkingAndReservesTokens(t *testing.T) {
	model := &scriptedModel{replies: []string{`{"preferences":"x"}`}}
	n := NewNormalizer(model)

	_ = n.Normalize(t.Context(), "startup_prompt", "một quy tắc tùy ý")
	if model.lastCfg.ThinkingLevel != agentcore.ThinkingOff {
		t.Errorf("nên tắt tư duy với mô hình có thể tắt được, got %q", model.lastCfg.ThinkingLevel)
	}
	if model.lastCfg.MaxTokens != normalizeMaxTokens {
		t.Errorf("max_tokens nên là %d, got %d", normalizeMaxTokens, model.lastCfg.MaxTokens)
	}
}

// JSON xấu toàn bộ: giáng cấp sau khi hết số lần thử lại, và đúng normalizeMaxAttempts lần.
func TestNormalize_FeedbackRetryExhaustsThenDegrades(t *testing.T) {
	model := &scriptedModel{replies: []string{"xấu"}}
	n := NewNormalizer(model)

	cand := n.Normalize(t.Context(), "startup_prompt", "mỗi chương 1200 chữ")
	if !cand.Degraded {
		t.Fatal("JSON xấu toàn bộ nên giáng cấp")
	}
	if model.calls != normalizeMaxAttempts {
		t.Fatalf("nên thử %d lần, thực tế %d", normalizeMaxAttempts, model.calls)
	}
}

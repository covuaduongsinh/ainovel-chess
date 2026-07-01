package imp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

type mockLLM struct {
	out string
	err error
	got []agentcore.Message
}

func (m *mockLLM) Generate(_ context.Context, msgs []agentcore.Message, _ []agentcore.ToolSpec, _ ...agentcore.CallOption) (*agentcore.LLMResponse, error) {
	m.got = msgs
	if m.err != nil {
		return nil, m.err
	}
	return &agentcore.LLMResponse{
		Message: agentcore.Message{
			Role:      agentcore.RoleAssistant,
			Content:   []agentcore.ContentBlock{agentcore.TextBlock(m.out)},
			Timestamp: time.Now(),
		},
	}, nil
}

const validEnvelope = `=== PREMISE ===
# Tên truyện thử nghiệm

## Thể loại và không khí
Huyền bí đô thị hiện đại

## Xung đột cốt lõi
Nhà báo điều tra vụ mất tích liên hoàn

## Mục tiêu nhân vật chính
Tìm ra hung thủ thật sự và minh oan cho bản thân

## Hướng kết thúc
Sự thật phơi bày, nhân vật chính phải lựa chọn

## Vùng cấm kỵ trong viết lách
Kinh dị đẫm máu, thoát ly thực tế

## Điểm bán hàng khác biệt
- Tự sự hai tuyến
- Góc nhìn nữ tính

## Điểm móc dị biệt
Tất cả nạn nhân mất tích đều họ "Trần"

## Lời hứa cốt lõi
Đọc xong có thể trải nghiệm đầy đủ giải mã huyền bí

=== CHARACTERS ===
[
  {"name":"Lâm Vãn","role":"nhân vật chính","description":"phóng viên tự do","arc":"giai đoạn đầu bị động, giai đoạn sau chủ động","traits":["nhạy bén","cứng đầu"]},
  {"name":"Trần Trầm","role":"phản diện","description":"hung thủ đứng sau","arc":"giai đoạn đầu ẩn náu, giai đoạn sau lộ diện","traits":["điềm tĩnh","tàn nhẫn"]}
]

=== WORLD_RULES ===
[
  {"category":"society","rule":"bối cảnh đô thị hiện đại, hệ thống cảnh sát hoàn chỉnh","boundary":"không siêu nhiên"}
]

=== LAYERED_OUTLINE ===
[
  {
    "index":1,
    "title":"Bóng Mây Mất Tích",
    "theme":"nhà báo điều tra vụ mất tích liên hoàn",
    "arcs":[
      {
        "index":1,
        "title":"Điều Tra Ban Đầu",
        "goal":"Lâm Vãn nhận vụ và khóa chặt manh mối họ Trần",
        "chapters":[
          {"title":"Lần Đầu Gặp Gỡ","core_event":"Lâm Vãn nhận được tin tố cáo ẩn danh","hook":"manh mối chỉ đến gia tộc họ Trần","scenes":["tòa soạn","quán cà phê"]},
          {"title":"Truy Tìm Dấu Vết","core_event":"Lâm Vãn thăm gia đình nạn nhân","hook":"phát hiện biểu tượng vật tế lễ chung","scenes":["nhà cũ","kho lưu trữ"]}
        ]
      }
    ]
  }
]

=== COMPASS ===
{
  "ending_direction":"sự thật phơi bày, nhân vật chính chọn giữa vạch trần và tự bảo vệ",
  "open_threads":["sự thật về nghi lễ tế lễ của gia tộc họ Trần","cáo buộc oan sai của Lâm Vãn"],
  "estimated_scale":"dự kiến 20-40 chương"
}
`

func TestReverseFoundation_ParsesValid(t *testing.T) {
	llm := &mockLLM{out: validEnvelope}
	chapters := []Chapter{
		{Title: "Lần Đầu Gặp Gỡ", Content: "Lâm Vãn mở phong bì ẩn danh..."},
		{Title: "Truy Tìm Dấu Vết", Content: "Cô gõ cửa căn nhà cũ kỹ đó..."},
	}
	got, err := ReverseFoundation(context.Background(), llm, "system prompt with ${chapter_count}", chapters)
	if err != nil {
		t.Fatalf("ReverseFoundation: %v", err)
	}
	if !strings.HasPrefix(got.Premise, "# Tên truyện thử nghiệm") {
		t.Errorf("premise head: %q", got.Premise[:20])
	}
	if len(got.Characters) != 2 || got.Characters[0].Name != "Lâm Vãn" {
		t.Errorf("characters wrong: %+v", got.Characters)
	}
	if len(got.Volumes) != 1 || len(domain.FlattenOutline(got.Volumes)) != 2 {
		t.Errorf("volumes wrong: %+v", got.Volumes)
	}
	if got.Compass == nil || len(got.Compass.OpenThreads) == 0 {
		t.Errorf("compass should be parsed with open_threads: %+v", got.Compass)
	}
	if !strings.Contains(llm.got[0].TextContent(), "with 2") {
		t.Errorf("system prompt expected ${chapter_count}=2 substituted, got: %q",
			llm.got[0].TextContent())
	}
	if !strings.Contains(llm.got[1].TextContent(), "Lâm Vãn mở phong bì ẩn danh") {
		t.Errorf("user prompt should contain chapter 1 content")
	}
}

func TestReverseFoundation_RejectsLengthMismatch(t *testing.T) {
	llm := &mockLLM{out: validEnvelope}
	chapters := []Chapter{
		{Title: "ch1", Content: "..."},
		{Title: "ch2", Content: "..."},
		{Title: "ch3", Content: "..."},
	}
	_, err := ReverseFoundation(context.Background(), llm, "x", chapters)
	if err == nil || !strings.Contains(err.Error(), "chapter count mismatch") {
		t.Fatalf("want chapter-count-mismatch error, got %v", err)
	}
}

func TestReverseFoundation_MissingTagFails(t *testing.T) {
	llm := &mockLLM{out: "=== PREMISE ===\n# x\n"}
	_, err := ReverseFoundation(context.Background(), llm,
		"x", []Chapter{{Title: "a", Content: "b"}})
	if err == nil || !strings.Contains(err.Error(), "missing required tags") {
		t.Fatalf("want missing-tags error, got %v", err)
	}
}

func TestParseFoundation_FencedJSONStripped(t *testing.T) {
	src := strings.ReplaceAll(validEnvelope,
		`=== CHARACTERS ===
[`,
		"=== CHARACTERS ===\n```json\n[",
	)
	src = strings.ReplaceAll(src, `]

=== WORLD_RULES ===`, "]\n```\n\n=== WORLD_RULES ===")
	got, err := parseFoundationOutput(src, 2)
	if err != nil {
		t.Fatalf("fenced parse: %v", err)
	}
	if len(got.Characters) != 2 {
		t.Errorf("characters: %+v", got.Characters)
	}
}

func TestPersistFoundation_PromotesPhaseToWriting(t *testing.T) {
	dir := t.TempDir()
	st := store.NewStore(dir)
	if err := st.Progress.Init("import-test", 0); err != nil {
		t.Fatalf("init progress: %v", err)
	}

	fr := mustParse(t, validEnvelope, 2)
	if err := PersistFoundation(context.Background(), st, domain.PlanningTierShort, fr); err != nil {
		t.Fatalf("PersistFoundation: %v", err)
	}

	prog, err := st.Progress.Load()
	if err != nil {
		t.Fatalf("load progress: %v", err)
	}
	if prog.Phase != domain.PhaseWriting {
		t.Errorf("phase: got %q want writing", prog.Phase)
	}
	if prog.TotalChapters != 2 {
		t.Errorf("total chapters: %d", prog.TotalChapters)
	}
	if !prog.Layered {
		t.Errorf("imported book must be layered so it can be continued/extended")
	}
	if c, _ := st.Outline.LoadCompass(); c == nil {
		t.Errorf("compass must be saved for continuation")
	}
	if prog.NovelName != "Tên truyện thử nghiệm" {
		t.Errorf("novel name: %q", prog.NovelName)
	}
	if got := st.FoundationMissing(); len(got) != 0 {
		t.Errorf("foundation should be complete, missing: %v", got)
	}
}

func mustParse(t *testing.T, raw string, expect int) *FoundationResult {
	t.Helper()
	fr, err := parseFoundationOutput(raw, expect)
	if err != nil {
		t.Fatalf("parse helper: %v", err)
	}
	return fr
}

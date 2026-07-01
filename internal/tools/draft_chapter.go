package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/store"
)

// DraftChapterTool ghi toàn bộ bản nháp chương, thay thế pipeline write_scene + polish_chapter cũ.
// Agent tự quyết định viết xong một lần hay viết tiếp theo đợt.
type DraftChapterTool struct {
	store *store.Store
}

func NewDraftChapterTool(store *store.Store) *DraftChapterTool {
	return &DraftChapterTool{store: store}
}

func (t *DraftChapterTool) Name() string { return "draft_chapter" }
func (t *DraftChapterTool) Description() string {
	return "Ghi nội dung chương. mode=write ghi đè toàn bộ chương, mode=append bổ sung vào bản nháp hiện có (viết tiếp/chỉnh sửa)"
}
func (t *DraftChapterTool) Label() string { return "Ghi chương" }

// Công cụ ghi, cấm song song (tranh chấp đọc-sửa-ghi).
func (t *DraftChapterTool) ReadOnly(_ json.RawMessage) bool        { return false }
func (t *DraftChapterTool) ConcurrencySafe(_ json.RawMessage) bool { return false }

func (t *DraftChapterTool) Schema() map[string]any {
	// mode được đánh dấu required để tương thích với OpenAI strict tool calling — chế độ strict
	// yêu cầu tất cả properties đều nằm trong danh sách required. Hành vi cũ "bỏ qua mode thì đi theo
	// mặc định write" giờ yêu cầu model truyền tường minh mode="write", nhánh default của Execute không đổi.
	return schema.Object(
		schema.Property("chapter", schema.Int("số chương")).Required(),
		schema.Property("content", schema.String("nội dung chương")).Required(),
		schema.Property("mode", schema.Enum("chế độ ghi", "write", "append")).Required(),
	)
}

// StrictSchema bật strict tool calling của OpenAI, buộc model phải tuân thủ nghiêm ngặt
// schema: tất cả trường required phải điền, arguments không thể "EOT sớm" xuất hiện đối tượng rỗng.
// litellm truyền xuyên trường strict; các backend hỗ trợ như OpenAI / xAI sẽ thực thi, các backend khác
// theo thông lệ HTTP/JSON bỏ qua trường không biết. Anthropic/Gemini/Bedrock đi qua chuỗi chuyển đổi riêng
// tự nhiên sẽ không thấy trường này.
func (t *DraftChapterTool) StrictSchema() bool { return true }

func (t *DraftChapterTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Chapter int    `json:"chapter"`
		Content string `json:"content"`
		Mode    string `json:"mode"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w: %w", errs.ErrToolArgs, err)
	}
	if a.Chapter <= 0 {
		return nil, fmt.Errorf("chapter must be > 0: %w", errs.ErrToolArgs)
	}
	if a.Content == "" {
		return nil, fmt.Errorf("content must not be empty: %w", errs.ErrToolArgs)
	}
	if err := t.store.Progress.ValidateChapterWork(a.Chapter); err != nil {
		return nil, err
	}
	if err := EnsureChapterExpanded(t.store, a.Chapter); err != nil {
		return nil, err
	}
	if t.store.Progress.IsChapterCompleted(a.Chapter) {
		// đường dẫn trau chuốt/viết lại: chương tuy đã hoàn thành nhưng vẫn trong pending_rewrites, cho phép ghi đè bản nháp
		progress, _ := t.store.Progress.Load()
		inRewriteQueue := progress != nil && slices.Contains(progress.PendingRewrites, a.Chapter)
		if !inRewriteQueue {
			return json.Marshal(map[string]any{
				"chapter":   a.Chapter,
				"skipped":   true,
				"completed": true,
				"reason":    fmt.Sprintf("Chương %d đã nộp hoàn thành, không thể ghi đè", a.Chapter),
			})
		}
	}
	if err := t.store.Progress.StartChapter(a.Chapter); err != nil {
		return nil, fmt.Errorf("mark chapter in progress: %w", err)
	}

	switch a.Mode {
	case "append":
		if err := t.store.Drafts.AppendDraft(a.Chapter, a.Content); err != nil {
			return nil, fmt.Errorf("append draft: %w", err)
		}
		full, err := t.store.Drafts.LoadDraft(a.Chapter)
		if err != nil {
			return nil, fmt.Errorf("load draft after append: %w", err)
		}
		if _, err := t.store.Checkpoints.AppendArtifact(
			domain.ChapterScope(a.Chapter), "draft",
			fmt.Sprintf("drafts/%02d.draft.md", a.Chapter),
		); err != nil {
			return nil, fmt.Errorf("checkpoint draft: %w", err)
		}
		return json.Marshal(map[string]any{
			"written":    true,
			"chapter":    a.Chapter,
			"mode":       "append",
			"word_count": utf8.RuneCountInString(full),
			"next_step":  "Đầu tiên read_chapter(source=draft) đọc lại bản nháp, sau đó gọi check_consistency, cuối cùng commit_chapter",
		})
	default: // write
		if err := t.store.Drafts.SaveDraft(a.Chapter, a.Content); err != nil {
			return nil, fmt.Errorf("save draft: %w", err)
		}
		if _, err := t.store.Checkpoints.AppendArtifact(
			domain.ChapterScope(a.Chapter), "draft",
			fmt.Sprintf("drafts/%02d.draft.md", a.Chapter),
		); err != nil {
			return nil, fmt.Errorf("checkpoint draft: %w", err)
		}
		return json.Marshal(map[string]any{
			"written":    true,
			"chapter":    a.Chapter,
			"mode":       "write",
			"word_count": utf8.RuneCountInString(a.Content),
			"next_step":  "Đầu tiên read_chapter(source=draft) đọc lại bản nháp, sau đó gọi check_consistency, cuối cùng commit_chapter",
		})
	}
}

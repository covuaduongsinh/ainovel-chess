package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/voocel/agentcore/schema"
	agentcoretools "github.com/voocel/agentcore/tools"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/store"
)

// EditChapterTool thay thế chuỗi điểm định trong bản nháp chương, phù hợp cho cảnh trau chuốt.
// So với viết lại toàn chương draft_chapter, tiết kiệm token hơn 10x.
//
// Cam kết lưu đĩa: chỉ sửa drafts/{ch:02d}.draft.md, cấm sửa trực tiếp chapters/ (bản cuối do commit_chapter độc quyền).
// Ngữ nghĩa Seed: drafts không tồn tại nhưng chapters có → tự động sao chép chapters vào drafts làm điểm khởi đầu.
// Kiểm tra quyền sở hữu: khi chương đã hoàn thành phải nằm trong hàng đợi PendingRewrites, nếu không thì từ chối.
//
// Công cụ này là lớp bao mỏng của agentcore.EditTool, logic tìm-thay thế (khớp chịu lỗi đa cấp, đầu ra diff, giữ nguyên dòng cuối/BOM)
// đều tái sử dụng từ triển khai thượng nguồn.
type EditChapterTool struct {
	store *store.Store
	edit  *agentcoretools.EditTool
}

func NewEditChapterTool(s *store.Store) *EditChapterTool {
	return &EditChapterTool{
		store: s,
		edit:  agentcoretools.NewEdit(s.Dir(), nil),
	}
}

func (t *EditChapterTool) Name() string  { return "edit_chapter" }
func (t *EditChapterTool) Label() string { return "Chỉnh sửa chương" }

// ReadOnly khai báo tường minh là công cụ ghi (phối hợp với ConcurrencySafeTool để ngăn lên lịch song song).
func (t *EditChapterTool) ReadOnly(_ json.RawMessage) bool { return false }

// ConcurrencySafe cấm tường minh song song: nhiều edit_chapter song song trên cùng chương sẽ tranh chấp đọc-sửa-ghi,
// ngay cả các chương khác nhau song song cũng sẽ xen kẽ thứ tự checkpoint. Thống nhất tuần tự là ổn định nhất.
func (t *EditChapterTool) ConcurrencySafe(_ json.RawMessage) bool { return false }

// ActivityDescription cung cấp mô tả hoạt động của công cụ hiện tại cho UI/log.
func (t *EditChapterTool) ActivityDescription(_ json.RawMessage) string { return "Chỉnh sửa bản nháp chương" }

func (t *EditChapterTool) Description() string {
	return "Thay thế chuỗi điểm định trong bản nháp chương (ưu tiên cho cảnh trau chuốt, tiết kiệm token hơn viết lại toàn chương draft_chapter)." +
		"Tìm old_string và thay bằng new_string, yêu cầu khớp chính xác và duy nhất (nhiều chỗ khớp cần replace_all=true)." +
		"Ghi vào drafts/{ch}.draft.md; khi drafts không tồn tại tự động gieo hạt từ chapters." +
		"Từ chối thực thi khi chương đã hoàn thành và không nằm trong hàng đợi PendingRewrites. Mỗi lần gọi chỉ sửa một chỗ, sửa nhiều chỗ hãy gọi nhiều lần."
}

func (t *EditChapterTool) Schema() map[string]any {
	return schema.Object(
		schema.Property("chapter", schema.Int("số chương")).Required(),
		schema.Property("old_string", schema.String("đoạn gốc chính xác cần thay thế, nhiều dòng cần có xuống dòng; khi không có replace_all phải xuất hiện duy nhất trong bản nháp")).Required(),
		schema.Property("new_string", schema.String("văn bản mới sau khi thay thế")).Required(),
		schema.Property("replace_all", schema.Bool("thay thế tất cả khớp (mặc định false)")),
	)
}

func (t *EditChapterTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Chapter    int    `json:"chapter"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w: %w", errs.ErrToolArgs, err)
	}
	if a.Chapter <= 0 {
		return nil, fmt.Errorf("chapter must be > 0: %w", errs.ErrToolArgs)
	}
	if a.OldString == "" {
		return nil, fmt.Errorf("old_string không được để trống: %w", errs.ErrToolArgs)
	}
	if a.OldString == a.NewString {
		return nil, fmt.Errorf("old_string và new_string giống nhau, không cần sửa: %w", errs.ErrToolArgs)
	}

	// kiểm tra quyền sở hữu: chương đã hoàn thành phải nằm trong hàng đợi viết lại, tránh làm ô nhiễm bản cuối
	if t.store.Progress.IsChapterCompleted(a.Chapter) {
		progress, _ := t.store.Progress.Load()
		if progress == nil || !slices.Contains(progress.PendingRewrites, a.Chapter) {
			return nil, fmt.Errorf("chương %d đã hoàn thành và không nằm trong hàng đợi PendingRewrites, không thể chỉnh sửa; muốn sửa hãy để editor thẩm định kích hoạt viết lại/trau chuốt trước: %w", a.Chapter, errs.ErrToolPrecondition)
		}
	}

	// Seed: khi drafts không tồn tại sao chép từ chapters làm điểm khởi đầu
	if err := t.ensureDraft(a.Chapter); err != nil {
		return nil, err
	}

	// ủy thác cho agentcore.EditTool hoàn thành tìm-thay thế
	subArgs, _ := json.Marshal(map[string]any{
		"path":        fmt.Sprintf("drafts/%02d.draft.md", a.Chapter),
		"file_path":   fmt.Sprintf("drafts/%02d.draft.md", a.Chapter),
		"old_text":    a.OldString,
		"old_string":  a.OldString,
		"new_text":    a.NewString,
		"new_string":  a.NewString,
		"replace_all": a.ReplaceAll,
	})
	result, err := t.edit.Execute(ctx, subArgs)
	if err != nil {
		return nil, fmt.Errorf("apply edit: %w: %w", errs.ErrToolPrecondition, err)
	}

	if _, err := t.store.Checkpoints.AppendArtifact(
		domain.ChapterScope(a.Chapter), "edit",
		fmt.Sprintf("drafts/%02d.draft.md", a.Chapter),
	); err != nil {
		return nil, fmt.Errorf("checkpoint edit: %w: %w", errs.ErrStoreWrite, err)
	}

	// đính kèm hướng dẫn: để writer biết các bước tiếp theo, tránh bỏ sót check_consistency / commit_chapter
	var passthrough map[string]any
	if err := json.Unmarshal(result, &passthrough); err != nil {
		return result, nil
	}
	passthrough["chapter"] = a.Chapter
	passthrough["next_step"] = "edit đã lưu đĩa. Vẫn còn lỗi nghiêm trọng có thể edit_chapter lần nữa; nếu không thì check_consistency rồi commit_chapter"
	return json.Marshal(passthrough)
}

// ensureDraft đảm bảo drafts/{ch}.draft.md tồn tại:
//   - đã có bản nháp → trả về ngay
//   - không có bản nháp nhưng có bản cuối → sao chép bản cuối vào drafts làm điểm khởi đầu chỉnh sửa (thường gặp trong cảnh trau chuốt)
//   - cả hai đều không có → báo lỗi, gợi ý dùng draft_chapter tạo bản nháp đầu tiên
func (t *EditChapterTool) ensureDraft(chapter int) error {
	draft, err := t.store.Drafts.LoadDraft(chapter)
	if err != nil {
		return fmt.Errorf("load draft: %w: %w", errs.ErrStoreRead, err)
	}
	if draft != "" {
		return nil
	}
	text, err := t.store.Drafts.LoadChapterText(chapter)
	if err != nil {
		return fmt.Errorf("load chapter: %w: %w", errs.ErrStoreRead, err)
	}
	if text == "" {
		return fmt.Errorf("chương %d không có bản nháp cũng không có bản cuối, vui lòng gọi draft_chapter(mode=write, chapter=%d) để tạo bản nháp đầu tiên trước: %w", chapter, chapter, errs.ErrToolPrecondition)
	}
	if err := t.store.Drafts.SaveDraft(chapter, text); err != nil {
		return fmt.Errorf("seed draft from chapter: %w: %w", errs.ErrStoreWrite, err)
	}
	return nil
}

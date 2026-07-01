package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/store"
)

// ReopenBookTool mở lại cuốn sách đã hoàn thành để vào trạng thái trả lại (chỉ Coordinator nắm giữ).
// Sau khi hoàn thành, completePhaseGate chặn cứng mọi phân công subagent, người dùng không thể trả lại chương đã viết.
// Công cụ này không phải subagent, có thể gọi trong giai đoạn complete: nó nguyên tử chuyển phase về writing, đưa
// chương đích vào PendingRewrites, flow=rewriting, sau đó Flow Router theo hàng đợi trả lại sẵn có phân writer viết lại từng chương,
// hàng đợi chạy xong commit_chapter tự động kết thúc lại. Gate / Router / edit / commit logic trả lại đều không cần thay đổi.
type ReopenBookTool struct {
	store *store.Store
}

func NewReopenBookTool(s *store.Store) *ReopenBookTool {
	return &ReopenBookTool{store: s}
}

func (t *ReopenBookTool) Name() string  { return "reopen_book" }
func (t *ReopenBookTool) Label() string { return "Mở lại để trả lại" }

func (t *ReopenBookTool) Description() string {
	return "Mở lại toàn bộ cuốn sách đã hoàn thành (phase=complete) để vào trạng thái trả lại, dùng khi người dùng yêu cầu viết lại/đánh bóng một số chương sau khi hoàn thành. " +
		"chapters là danh sách số chương đã hoàn thành cần trả lại; sau khi gọi các chương này vào hàng đợi viết lại, Host sẽ phân writer viết lại từng chương, hoàn thành tất cả sẽ tự động kết thúc lại. " +
		"Chỉ dùng khi toàn bộ cuốn sách đã hoàn thành và người dùng yêu cầu rõ ràng sửa đổi chương đã viết; người dùng muốn thêm tình tiết/mở rộng dung lượng không thuộc trả lại, không dùng công cụ này."
}

// Công cụ ghi, cấm song song.
func (t *ReopenBookTool) ReadOnly(_ json.RawMessage) bool        { return false }
func (t *ReopenBookTool) ConcurrencySafe(_ json.RawMessage) bool { return false }

func (t *ReopenBookTool) ActivityDescription(_ json.RawMessage) string { return "Mở lại toàn bộ sách để trả lại" }

func (t *ReopenBookTool) Schema() map[string]any {
	return schema.Object(
		schema.Property("chapters", schema.Array("danh sách số chương đã hoàn thành cần trả lại (ít nhất một chương)", schema.Int(""))).Required(),
		schema.Property("reason", schema.String("lý do trả lại (tùy chọn, ví dụ \"dọn ký tự đặc biệt\")")),
	)
}

func (t *ReopenBookTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Chapters []int  `json:"chapters"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w: %w", errs.ErrToolArgs, err)
	}
	if len(a.Chapters) == 0 {
		return nil, fmt.Errorf("chapters không được để trống, cần chỉ rõ các chương cần trả lại: %w", errs.ErrToolArgs)
	}

	progress, err := t.store.Progress.Load()
	if err != nil {
		return nil, fmt.Errorf("load progress: %w: %w", errs.ErrStoreRead, err)
	}
	if progress == nil {
		return nil, fmt.Errorf("progress chưa khởi tạo: %w", errs.ErrToolPrecondition)
	}
	// Chỉ có thể trả lại chương đã viết; số chương không nằm trong tập đã hoàn thành thuộc tiếp tục viết/vượt giới hạn, từ chối rõ ràng hướng dẫn người dùng đi điều chỉnh dung lượng.
	var invalid []int
	for _, ch := range a.Chapters {
		if !slices.Contains(progress.CompletedChapters, ch) {
			invalid = append(invalid, ch)
		}
	}
	if len(invalid) > 0 {
		return nil, fmt.Errorf("chương %v chưa viết xong, reopen chỉ có thể trả lại chương đã hoàn thành (thêm/mở rộng tình tiết hãy dùng điều chỉnh dung lượng): %w", invalid, errs.ErrToolPrecondition)
	}

	// Kiểm tra tiền điều kiện phase trong store.Reopen (chỉ complete mới có thể gọi).
	if err := t.store.Progress.Reopen(a.Chapters, a.Reason); err != nil {
		return nil, fmt.Errorf("reopen: %w: %w", errs.ErrStoreWrite, err)
	}

	// checkpoint: đối xứng với complete_book (GlobalScope + meta/progress.json).
	if _, err := t.store.Checkpoints.AppendArtifact(domain.GlobalScope(), "reopen", "meta/progress.json"); err != nil {
		return nil, fmt.Errorf("checkpoint reopen: %w: %w", errs.ErrStoreWrite, err)
	}

	return json.Marshal(map[string]any{
		"reopened":         true,
		"phase":            string(domain.PhaseWriting),
		"pending_rewrites": a.Chapters,
		"next_step":        "Đã mở lại và đưa các chương đích vào hàng đợi. Vui lòng chờ lệnh Host phân writer trả lại từng chương; sau khi hoàn thành tất cả sẽ tự động kết thúc lại.",
	})
}

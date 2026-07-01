package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/rules"
	"github.com/voocel/ainovel-cli/internal/userrules"
)

// SaveUserRulesTool lưu bền yêu cầu "phong cách viết/chất lượng" lâu dài của người dùng (chỉ Coordinator nắm giữ).
//
// Đây là điểm vào thống nhất cho quy tắc viết lâu dài: các quy tắc phong cách/chất lượng
// luôn có hiệu lực, ràng buộc bút pháp của writer (như "mỗi chương 1500 chữ",
// "ít dùng ẩn dụ", "cấm xuất hiện 'ở một mức độ nào đó'", "tỷ lệ thoại cao hơn",
// "nhân vật chính tổng thể bình tĩnh kiềm chế") được LLM chuẩn hóa thành ràng buộc cấu trúc
// ghi vào bản chụp meta/user_rules.json, novel_context tiêm vào working_memory.user_rules,
// commit_chapter tự kiểm theo đó. Điều chỉnh cốt truyện/cấu trúc/nhân vật/giai đoạn đi qua architect,
// chương đã viết cần làm lại thì trước tiên qua editor vào hàng đợi, sau đó Host phái writer viết lại.
//
// Chuẩn hóa thất bại không báo lỗi (giáng cấp thành raw preferences), chỉ có ghi đĩa thất bại mới trả tool error —
// chi tiết kỹ thuật không nên ném ngược về Coordinator coi như lỗi luồng.
type SaveUserRulesTool struct {
	svc *userrules.Service
}

func NewSaveUserRulesTool(svc *userrules.Service) *SaveUserRulesTool {
	return &SaveUserRulesTool{svc: svc}
}

func (t *SaveUserRulesTool) Name() string  { return "save_user_rules" }
func (t *SaveUserRulesTool) Label() string { return "Lưu quy tắc viết" }

func (t *SaveUserRulesTool) Description() string {
	return "Chuẩn hóa yêu cầu phong cách viết/chất lượng lâu dài của người dùng thành quy tắc cấu trúc của cuốn sách và lưu bền " +
		"(ví dụ: \"khoảng 1500 chữ mỗi chương\", \"ít dùng ẩn dụ và điệp cú\", \"cấm xuất hiện 'ở một mức độ nào đó'\"). " +
		"Sau khi lưu, tất cả subagent mỗi chương đều thấy trong working_memory.user_rules, writer viết theo đó, commit_chapter tự kiểm theo đó, có hiệu lực qua khởi động lại. " +
		"text bắt buộc, chuyển lại nguyên yêu cầu của người dùng là được, hệ thống sẽ tự trích xuất cấu trúc. " +
		"Trả về ràng buộc cấu trúc đã hiểu lần này và ràng buộc toàn bộ đang có hiệu lực — hãy hiển thị lại cho người dùng xác nhận có hiểu đúng không. " +
		"Chỉ lưu quy tắc phong cách/chất lượng \"luôn có hiệu lực\"; điều chỉnh cốt truyện/cấu trúc/nhân vật/giai đoạn (như \"thêm 10 chương\", \"tập này viết nhiều chiến đấu hơn\") đi qua architect, chương đã viết làm lại đi qua editor, không lưu ở đây."
}

// Công cụ ghi, cấm song song.
func (t *SaveUserRulesTool) ReadOnly(_ json.RawMessage) bool        { return false }
func (t *SaveUserRulesTool) ConcurrencySafe(_ json.RawMessage) bool { return false }

func (t *SaveUserRulesTool) ActivityDescription(_ json.RawMessage) string {
	return "Lưu quy tắc viết"
}

func (t *SaveUserRulesTool) Schema() map[string]any {
	return schema.Object(
		schema.Property("text", schema.String("Yêu cầu viết lâu dài của người dùng (chuyển lại nguyên văn, có thể đúc gọn), hệ thống sẽ chuẩn hóa thành ràng buộc cấu trúc")).Required(),
	)
}

func (t *SaveUserRulesTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w: %w", errs.ErrToolArgs, err)
	}
	text := strings.TrimSpace(a.Text)
	if text == "" {
		return nil, fmt.Errorf("text không được để trống: %w", errs.ErrToolArgs)
	}

	// Chuẩn hóa thất bại chỉ giáng cấp mục đó thành raw preferences (không báo lỗi); chỉ ghi đĩa thất bại mới trả tool error.
	snap, cand, err := t.svc.AddRuntimeRule(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("lưu quy tắc viết thất bại: %w", err)
	}

	return json.Marshal(map[string]any{
		"saved":      true,
		"status":     snap.Status,
		"understood": userRuleUnderstanding(cand), // hiểu lần này, dùng để hiển thị xác nhận
		"in_effect":  snap.Payload(),              // ràng buộc toàn bộ đang có hiệu lực
	})
}

// userRuleUnderstanding chuyển ứng viên chuẩn hóa lần này thành góc nhìn sự thật cho LLM —
// Coordinator dựa vào đó hiển thị lại cho người dùng "tôi đã hiểu câu này thành gì", để kịp thời chỉnh sửa.
func userRuleUnderstanding(c rules.Candidate) map[string]any {
	m := map[string]any{"degraded": c.Degraded}
	if !c.Structured.IsEmpty() {
		m["structured"] = c.Structured
	}
	if p := strings.TrimSpace(c.Preferences); p != "" {
		m["preferences"] = p
	}
	if len(c.Uncertain) > 0 {
		m["uncertain"] = c.Uncertain // mục cố ý không đưa lên thành kiểm tra cứng + lý do
	}
	return m
}

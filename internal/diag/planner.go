package diag

import "fmt"

// PlanActions tạo các hành động có thể thực thi dựa trên Finding có độ tin cậy cao.
// Chỉ những Finding có Confidence==high && AutoLevel==safe mới tạo ra Action.
func PlanActions(findings []Finding) []Action {
	var actions []Action
	seen := make(map[string]struct{})

	for _, f := range findings {
		if f.Confidence != ConfHigh || f.AutoLevel != AutoSafe {
			continue
		}
		if _, ok := seen[f.Rule]; ok {
			continue
		}
		seen[f.Rule] = struct{}{}

		actions = append(actions, planRule(f)...)
	}
	return actions
}

func planRule(f Finding) []Action {
	key := findingFingerprint(f)

	switch f.Rule {
	case "PhaseFlowMismatch":
		return []Action{
			{SourceRule: f.Rule, Kind: ActionEmitNotice, Severity: f.Severity, Summary: f.Title, Message: f.Title, Fingerprint: key},
			{SourceRule: f.Rule, Kind: ActionEnqueueFollowUp, Severity: f.Severity, Summary: "Sửa lỗi máy trạng thái", Message: "Lỗi máy trạng thái: " + f.Evidence + ". Vui lòng kiểm tra và sửa trạng thái phase/flow của progress trước khi tiếp tục chạy.", Fingerprint: key},
		}
	case "OutlineExhausted":
		return []Action{
			{SourceRule: f.Rule, Kind: ActionEnqueueFollowUp, Severity: f.Severity, Summary: "Xử lý dàn ý đã hết", Message: "Số chương đã hoàn thành đạt giới hạn đã lập kế hoạch. Hãy ưu tiên gọi Architect để mở rộng cung tiếp theo hoặc thêm tập mới trước khi tiếp tục viết.", Fingerprint: key},
		}
	case "OrphanedSteer":
		return []Action{
			{SourceRule: f.Rule, Kind: ActionEnqueueFollowUp, Severity: f.Severity, Summary: "Xử lý lệnh can thiệp người dùng chưa tiêu thụ", Message: "Có lệnh can thiệp người dùng chưa được tiêu thụ, hãy ưu tiên xử lý pending steer trước khi tiếp tục nhiệm vụ hiện tại.", Fingerprint: key},
		}
	default:
		return nil
	}
}

func findingFingerprint(f Finding) string {
	return fmt.Sprintf("%s|%s|%s|%s", f.Rule, f.Target, f.Title, f.Evidence)
}

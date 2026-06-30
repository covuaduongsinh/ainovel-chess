// Package flow hiện thực định tuyến theo loại: Host căn theo sự thật quyết định gọi subagent nào làm gì kế tiếp.
//
// Nguyên tắc thiết kế:
//   - Route là hàm thuần: đầu vào State, đầu ra *Instruction. Không IO, không gọi Store, test đơn vị được.
//   - State do LoadState (không thuần) dựng từ Store, đọc gọn một lần mọi sự thật mà định tuyến cần.
//   - Trả về nil là hợp lệ: nghĩa là "tình huống phán quyết, để Coordinator LLM tự quyết định".
//
// Router phụ trách các quyết định "kiểu tra bảng" (bước kế mỗi chương, hậu xử lý cuối cung, hàng đợi dẫn dắt),
// không phụ trách các quyết định "kiểu hiểu ngữ nghĩa" (chọn nhà hoạch định, xử lý Steer của người dùng, xuất tổng kết).
package flow

import (
	"fmt"

	"github.com/voocel/ainovel-cli/internal/domain"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

// Instruction chỉ thị subagent và nhiệm vụ mà Host yêu cầu Coordinator gọi ở bước kế.
type Instruction struct {
	Agent   string // architect_long / architect_short / writer / editor
	Task    string // mô tả nhiệm vụ giao cho subagent
	Reason  string // lý do cho Coordinator xem (tùy chọn, tiện debug và log)
	Chapter int    // số chương mà nhiệm vụ writer dính tới (viết tiếp/viết lại/trau chuốt); 0 nghĩa là không dính (nhiệm vụ editor/architect)
}

// State là đầu vào của Route: mọi sự thật phải khai báo tường minh ở đây, cấm Route tự đọc Store bên trong.
type State struct {
	Progress *domain.Progress

	// Chương đã hoàn thành cuối cùng (cuối Progress.CompletedChapters); bằng 0 nghĩa là chưa bắt đầu viết.
	LastCompleted int

	// Thông tin ranh giới cung của chương trước; khi IsArcEnd=false thì các trường khác vô nghĩa.
	// Khi LastCompleted=0 hoặc không phải chế độ Layered thì phải là nil.
	ArcBoundary *storepkg.ArcBoundary

	// Ba sự thật hậu xử lý cuối cung: thẩm định / tóm tắt cung / tóm tắt tập đã hoàn thành chưa.
	HasArcReview     bool
	HasArcSummary    bool
	HasVolumeSummary bool

	// Các mục thiết định nền tảng còn thiếu (tín hiệu bổ sung của giai đoạn hoạch định).
	FoundationMissing []string
}

// Route căn theo sự thật trả về lệnh bước kế; trả về nil nghĩa là để Coordinator LLM tự phán quyết.
//
// Độ ưu tiên quyết định (loại trừ nhau, từ trên xuống khớp cái đầu tiên):
//  1. Phase=Complete        → nil (LLM xuất tổng kết)
//  2. Phase!=Writing        → nil (LLM phán định chọn nhà hoạch định / bổ sung hoạch định)
//  3. PendingRewrites khác rỗng → writer theo hàng đợi viết lại/trau chuốt
//  4. Flow=Reviewing        → nil (editor vừa lưu review, phân nhánh verdict do tầng công cụ xử lý)
//  5. Flow=Steering         → nil (đang xử lý người dùng can thiệp)
//  6. Thiếu thẩm định cuối cung → editor(arc review)
//  7. Có thẩm định cuối cung nhưng thiếu tóm tắt cung → editor(arc summary)
//  8. Cuối tập có tóm tắt cung nhưng thiếu tóm tắt tập → editor(volume summary)
//  9. Cung kế tiếp là bộ khung → architect_long(expand_arc)
//
// 10. Cuối tập cần quyết định tập kế → architect_long(append_volume / complete_book)
// 11. Còn lại                → writer (viết next_chapter)
func Route(s State) *Instruction {
	p := s.Progress
	if p == nil {
		return nil
	}

	// 1. Trạng thái cuối: để LLM xuất tổng kết
	if p.Phase == domain.PhaseComplete {
		return nil
	}

	// 2. Giai đoạn hoạch định do Coordinator phán định (chọn architect_long/short + vòng bổ sung)
	if p.Phase != domain.PhaseWriting {
		return nil
	}

	// 3. Hàng đợi viết lại/trau chuốt ưu tiên (sự thật đã lưu xuống ở tầng công cụ, Router chỉ phân theo đơn)
	if len(p.PendingRewrites) > 0 {
		ch := p.PendingRewrites[0]
		verb := "Viết lại"
		if p.Flow == domain.FlowPolishing {
			verb = "Trau chuốt"
		}
		return &Instruction{
			Agent:   "writer",
			Task:    fmt.Sprintf("%s chương %d", verb, ch),
			Reason:  fmt.Sprintf("Hàng đợi PendingRewrites còn %d chương", len(p.PendingRewrites)),
			Chapter: ch,
		}
	}

	// 4. Đang thẩm định: save_review vừa lưu xuống, nâng/hạ verdict do tầng công cụ xử lý, định tuyến không can thiệp
	if p.Flow == domain.FlowReviewing {
		return nil
	}

	// 5. Đang xử lý người dùng can thiệp: Coordinator đang phán định, Host không giành quyền
	if p.Flow == domain.FlowSteering {
		return nil
	}

	// 6-10. Hậu xử lý cuối cung của chế độ phân tầng
	if p.Layered && s.ArcBoundary != nil && s.ArcBoundary.IsArcEnd {
		b := s.ArcBoundary
		switch {
		case !s.HasArcReview:
			return &Instruction{
				Agent:  "editor",
				Task:   fmt.Sprintf("Thẩm định cấp cung cho cung %d của tập %d (scope=arc)", b.Arc, b.Volume),
				Reason: "Thẩm định cuối cung chưa hoàn thành",
			}
		case !s.HasArcSummary:
			return &Instruction{
				Agent:  "editor",
				Task:   fmt.Sprintf("Sinh tóm tắt cung %d của tập %d (save_arc_summary)", b.Arc, b.Volume),
				Reason: "Tóm tắt cung chưa hoàn thành",
			}
		case b.IsVolumeEnd && !s.HasVolumeSummary:
			return &Instruction{
				Agent:  "editor",
				Task:   fmt.Sprintf("Sinh tóm tắt tập cho tập %d (save_volume_summary)", b.Volume),
				Reason: "Tóm tắt tập chưa hoàn thành",
			}
		case b.NeedsExpansion && b.NextArc > 0:
			return &Instruction{
				Agent:  "architect_long",
				Task:   fmt.Sprintf("Mở cung %d của tập %d (save_foundation type=expand_arc)", b.NextArc, b.NextVolume),
				Reason: "Cung kế tiếp dạng bộ khung chờ mở",
			}
		case b.NeedsNewVolume:
			return &Instruction{
				Agent:  "architect_long",
				Task:   "Đánh giá rồi gọi save_foundation type=append_volume (viết tiếp) hoặc type=complete_book (kết thúc toàn sách)",
				Reason: "Cuối tập cần quyết định thêm tập mới hay kết thúc toàn sách",
			}
		}
	}

	// 12. Viết tiếp bình thường
	next := p.NextChapter()
	if next <= 0 {
		return nil
	}
	return &Instruction{
		Agent:   "writer",
		Task:    fmt.Sprintf("Viết chương %d", next),
		Reason:  "Viết tiếp chương kế",
		Chapter: next,
	}
}

// FormatMessage định dạng Instruction thành thông điệp người dùng gửi cho Coordinator.
// Định dạng cố định, tiện cho prompt Coordinator nhận diện và LLM phản hồi thẳng.
func FormatMessage(i *Instruction) string {
	return fmt.Sprintf(
		"[Host ra lệnh]\nBước tiếp theo: gọi subagent(%s, %q)\nagent: %s\ntask: %q\nLý do: %s\nĐây là lệnh tường minh ở tầng luồng, hãy thực thi ngay; tham số agent/task của subagent bắt buộc dùng nguyên văn agent/task ở trên, đừng viết lại task, đừng gọi novel_context trước, đừng xuất suy luận trước.",
		i.Agent, i.Task, i.Agent, i.Task, i.Reason,
	)
}

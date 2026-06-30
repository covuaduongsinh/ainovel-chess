package host

import (
	"fmt"
	"os"
	"strings"

	"github.com/voocel/ainovel-cli/internal/domain"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

// buildResumePrompt sinh prompt ngắn dùng cho Resume và nhãn UI dựa trên sự thật.
//
// Ghi chú tái cấu trúc (2026-04-20): mọi quyết định "bước tiếp theo cụ thể nên làm gì" đã hạ xuống Host Flow Router.
// Hàm này không còn thay Coordinator hoạch định hành động nữa, chỉ làm ba việc:
//  1. Xét có cần khôi phục không (Phase=Complete hoặc không có Progress → trả về label rỗng nghĩa là tạo mới)
//  2. Sinh label phù hợp hiển thị trên UI (kiểu "Khôi phục: thẩm định cuối cung chờ xử lý (V2 A3)")
//  3. Truyền tường minh PendingSteer mà người dùng để lại trong lúc dừng cho Coordinator
//
// Trả về (prompt, label, error). label rỗng nghĩa là không có trạng thái khôi phục (nên đi tạo mới).
func buildResumePrompt(store *storepkg.Store) (string, string, error) {
	progress, err := store.Progress.Load()
	if err != nil && !os.IsNotExist(err) {
		return "", "", err
	}
	if progress == nil || progress.Phase == domain.PhaseComplete {
		return "", "", nil
	}

	label := describeResume(store, progress)

	var b strings.Builder
	title := progress.NovelName
	if title == "" {
		title = "tiểu thuyết hiện tại"
	}
	b.WriteString(fmt.Sprintf("[Khôi phục] Sách này 「%s」", title))
	if n := len(progress.CompletedChapters); n > 0 {
		b.WriteString(fmt.Sprintf("đã hoàn thành %d chương", n))
		if progress.TotalChapters > 0 {
			b.WriteString(fmt.Sprintf(" (tổng %d chương)", progress.TotalChapters))
		}
		b.WriteString(fmt.Sprintf(", tổng %d chữ", progress.TotalWordCount))
	}
	b.WriteString(".\n")
	b.WriteString("Host sẽ căn theo sự thật hiện tại ra lệnh bước tiếp theo `[Host ra lệnh]`. Nhận xong thực thi ngay, đừng gọi novel_context suy luận trước.\n")

	if meta, _ := store.RunMeta.Load(); meta != nil && meta.PendingSteer != "" {
		b.WriteString("\nNgười dùng đã để lại một ý kiến can thiệp trong lúc dừng:\n「")
		b.WriteString(meta.PendingSteer)
		b.WriteString("」\nHãy đánh giá xử lý theo quy tắc người dùng can thiệp của coordinator.md trước.")
	}

	return b.String(), label, nil
}

// describeResume sinh nhãn khôi phục đọc được cho người; không ảnh hưởng hành vi của Coordinator.
// Mọi định tuyến thực thi do Flow Router suy từ sự thật; chỗ này chỉ là "Khôi phục: xxx" hướng UI.
func describeResume(store *storepkg.Store, progress *domain.Progress) string {
	switch progress.Phase {
	case domain.PhasePremise, domain.PhaseOutline:
		return fmt.Sprintf("Khôi phục: giai đoạn hoạch định (%s)", progress.Phase)
	case domain.PhaseWriting:
		// Ưu tiên căn theo độ ưu tiên quyết định của Router, để label nhất quán với lệnh sắp phân phối.
		if pending, _ := store.Signals.LoadPendingCommit(); pending != nil {
			return fmt.Sprintf("Khôi phục: chương %d nộp dở", pending.Chapter)
		}
		if len(progress.PendingRewrites) > 0 {
			verb := "Viết lại"
			if progress.Flow == domain.FlowPolishing {
				verb = "Trau chuốt"
			}
			return fmt.Sprintf("Khôi phục: %s %d chương chờ xử lý", verb, len(progress.PendingRewrites))
		}
		if progress.Flow == domain.FlowReviewing {
			return "Khôi phục: thẩm định dở"
		}
		if progress.InProgressChapter > 0 {
			return fmt.Sprintf("Khôi phục: chương %d đang tiến hành", progress.InProgressChapter)
		}
		if label := describeArcEndLabel(store, progress); label != "" {
			return label
		}
		return fmt.Sprintf("Khôi phục: tiếp tục từ chương %d", progress.NextChapter())
	}
	return "Khôi phục"
}

// describeArcEndLabel sinh nhãn hợp UI cho nhiều trạng thái trung gian cuối cung/cuối tập.
// Giữ cùng thứ tự với nhánh cuối cung của flow.Route, bảo đảm label khớp lệnh đầu tiên của Router.
func describeArcEndLabel(store *storepkg.Store, progress *domain.Progress) string {
	if !progress.Layered || len(progress.CompletedChapters) == 0 {
		return ""
	}
	lastCh := progress.CompletedChapters[len(progress.CompletedChapters)-1]
	boundary, err := store.Outline.CheckArcBoundary(lastCh)
	if err != nil || boundary == nil || !boundary.IsArcEnd {
		return ""
	}
	vol, arc := boundary.Volume, boundary.Arc
	switch {
	case !store.World.HasArcReview(lastCh):
		return fmt.Sprintf("Khôi phục: thẩm định cuối cung chờ xử lý (V%d A%d)", vol, arc)
	case !store.Summaries.HasArcSummary(vol, arc):
		return fmt.Sprintf("Khôi phục: tóm tắt cung chờ sinh (V%d A%d)", vol, arc)
	case boundary.IsVolumeEnd && !store.Summaries.HasVolumeSummary(vol):
		return fmt.Sprintf("Khôi phục: tóm tắt tập chờ sinh (V%d)", vol)
	case boundary.NeedsExpansion && boundary.NextArc > 0:
		return fmt.Sprintf("Khôi phục: chờ mở cung kế tiếp (V%d A%d)", boundary.NextVolume, boundary.NextArc)
	case boundary.NeedsNewVolume:
		return fmt.Sprintf("Khôi phục: chờ quyết định tập kế (cuối V%d)", vol)
	}
	return ""
}

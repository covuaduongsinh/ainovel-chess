package host

import (
	"fmt"
	"strings"

	"github.com/voocel/ainovel-cli/internal/store"
)

// buildStoryStateSummary tổng hợp một đoạn tóm tắt súc tích về hiện trạng câu chuyện, để trợ lý đồng sáng tác giai đoạn hiểu "đã viết được gì".
// Dùng lại điểm truy cập store, chỉ lấy các sự thật cấp cao cần thiết cho lên kế hoạch hướng đi (tiến độ / la bàn / tập gần nhất / nhân vật chính / phục bút đang mở);
// không kéo chính văn, không cấp toàn bộ JSON của novel_context — đồng sáng tác là hội thoại, cần tổng quan đọc được, không phải ngữ cảnh viết.
// Mục nào thiếu thì bỏ qua (best-effort), trả về chuỗi rỗng nghĩa là chưa có tiến độ nào khả dụng.
func buildStoryStateSummary(s *store.Store) string {
	if s == nil {
		return ""
	}
	var b strings.Builder

	if progress, _ := s.Progress.Load(); progress != nil {
		if name := strings.TrimSpace(progress.NovelName); name != "" {
			fmt.Fprintf(&b, "- Tên sách：《%s》\n", name)
		}
		fmt.Fprintf(&b, "- Tiến độ：đã hoàn thành %d chương", len(progress.CompletedChapters))
		if progress.TotalChapters > 0 {
			fmt.Fprintf(&b, " / kế hoạch %d chương", progress.TotalChapters)
		}
		fmt.Fprintf(&b, "，khoảng %d chữ，chương tiếp theo là chương %d\n", progress.TotalWordCount, progress.NextChapter())
		if progress.Layered && progress.CurrentVolume > 0 {
			fmt.Fprintf(&b, "- Vị trí hiện tại：Tập %d Cung %d\n", progress.CurrentVolume, progress.CurrentArc)
		}
	}

	if compass, _ := s.Outline.LoadCompass(); compass != nil {
		if dir := strings.TrimSpace(compass.EndingDirection); dir != "" {
			fmt.Fprintf(&b, "- Hướng kết thúc：%s\n", dir)
		}
		if compass.EstimatedScale != "" {
			fmt.Fprintf(&b, "- Quy mô ước tính：%s\n", compass.EstimatedScale)
		}
		if len(compass.OpenThreads) > 0 {
			fmt.Fprintf(&b, "- Mạch dài đang mở：%s\n", strings.Join(compass.OpenThreads, "；"))
		}
	}

	// Tóm tắt tập gần nhất, để trợ lý biết câu chuyện vừa đi đến đâu
	if vols, _ := s.Summaries.LoadAllVolumeSummaries(); len(vols) > 0 {
		last := vols[len(vols)-1]
		fmt.Fprintf(&b, "- Gần đây《%s》：%s\n", last.Title, truncate(last.Summary, 200))
	}

	// Nhân vật chính (core/important), tối đa 8 người
	if chars, _ := s.Characters.Load(); len(chars) > 0 {
		var names []string
		for _, c := range chars {
			if c.Tier == "secondary" || c.Tier == "decorative" {
				continue
			}
			line := c.Name
			if role := strings.TrimSpace(c.Role); role != "" {
				line += "（" + role + "）"
			}
			names = append(names, line)
			if len(names) >= 8 {
				break
			}
		}
		if len(names) > 0 {
			fmt.Fprintf(&b, "- Nhân vật chính：%s\n", strings.Join(names, "、"))
		}
	}

	// Phục bút chưa thu, tối đa 6 mục
	if fs, _ := s.World.LoadActiveForeshadow(); len(fs) > 0 {
		var items []string
		for _, f := range fs {
			items = append(items, truncate(f.Description, 40))
			if len(items) >= 6 {
				break
			}
		}
		fmt.Fprintf(&b, "- Phục bút chưa thu：%s\n", strings.Join(items, "；"))
	}

	return strings.TrimSpace(b.String())
}

// stageSystemPrompt tổng hợp system prompt đầy đủ cho đồng sáng tác giai đoạn: prompt giai đoạn + tóm tắt trạng thái câu chuyện hiện tại.
// Tóm tắt được gắn vào cuối như phụ lục dữ liệu (ngăn cách bằng đường kẻ với quy chuẩn định dạng), tương ứng với hướng dẫn "xem tiến độ ở dưới" trong prompt.
func stageSystemPrompt(s *store.Store) string {
	prompt := stageCoCreateSystemPrompt
	if summary := buildStoryStateSummary(s); summary != "" {
		prompt += "\n\n---\n## Trạng thái câu chuyện hiện tại\n（Dưới đây là tóm tắt khách quan nội dung đã viết, để bạn tham chiếu khi lên kế hoạch tiếp theo, không sao chép nguyên văn vào <draft>）\n" + summary
	}
	return prompt
}

package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/voocel/ainovel-cli/internal/host"
)

// outlineGridThreshold là ngưỡng chương để đại cương chuyển sang đa cột.
// Giới hạn trên của short tier là 25 chương, dưới 20 thì đơn cột vừa một màn hình và giữ được huy hiệu "đang tiến hành";
// chế độ layered dài tập sau khi cuộn mở rộng n tự nhiên vượt 20, chuyển mượt sang đa cột.
const outlineGridThreshold = 20

// renderOutlineSection chọn bố cục theo số chương: ít thì đơn cột (có huy hiệu "đang tiến hành"), nhiều thì lưới đa cột.
func renderOutlineSection(snap host.UISnapshot, contentW int) string {
	if len(snap.Outline) < outlineGridThreshold {
		return renderOutlineList(snap, contentW)
	}
	return renderOutlineGrid(snap, contentW)
}

// renderOutlineList danh sách chương đơn cột (dùng cho truyện ngắn). Cuối mỗi dòng có huy hiệu "đang tiến hành", nhịp đọc dọc gần với mục lục hơn.
func renderOutlineList(snap host.UISnapshot, contentW int) string {
	var b strings.Builder
	for _, e := range snap.Outline {
		ch := fmt.Sprintf("%2d", e.Chapter)
		var marker, chStyle string
		titleStyle := cardContentStyle
		switch {
		case snap.CompletedCount >= e.Chapter:
			marker = lipgloss.NewStyle().Foreground(colorSuccess).Render("●")
			chStyle = lipgloss.NewStyle().Foreground(colorDim).Render(ch)
		case snap.InProgressChapter == e.Chapter:
			marker = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("▸")
			chStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(ch)
			titleStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
		default:
			marker = lipgloss.NewStyle().Foreground(colorDim).Render("○")
			chStyle = lipgloss.NewStyle().Foreground(colorDim).Render(ch)
			titleStyle = lipgloss.NewStyle().Foreground(colorMuted)
		}
		title := truncate(e.Title, contentW-6)
		line := marker + chStyle + " " + titleStyle.Render(title)
		if snap.InProgressChapter == e.Chapter {
			line += lipgloss.NewStyle().Foreground(colorAccent).Italic(true).Render(" Đang tiến hành")
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// renderOutlineGrid điền chương đại cương theo "ưu tiên cột" thành lưới đa cột, tránh để trống nhiều trên màn hình rộng đơn cột.
// Số cột tự thích nghi theo contentW (1-4), chương trong cột tăng dần liên tục ("đọc hết một cột rồi đọc cột tiếp theo").
// Đánh đổi với bố cục đơn cột: bỏ huy hiệu "đang tiến hành" ở cuối —— trên đa cột huy hiệu sẽ phá vỡ căn chỉnh cột,
// và ▸ dấu + vàng + "Đang viết Chương N" ở thanh tổng quan bên trái đã nói rõ thông tin đang tiến hành rồi.
func renderOutlineGrid(snap host.UISnapshot, contentW int) string {
	n := len(snap.Outline)
	if n == 0 {
		return ""
	}
	chNumW := 2
	titleW := 0
	for _, e := range snap.Outline {
		if w := len(strconv.Itoa(e.Chapter)); w > chNumW {
			chNumW = w
		}
		if w := lipgloss.Width(e.Title); w > titleW {
			titleW = w
		}
	}
	// Giới hạn trên chiều rộng tiêu đề là 14 (khoảng 7 chữ Hán); tiêu đề dài thỉnh thoảng xuất hiện sẽ bị cắt, tránh một hai tiêu đề dài làm phình toàn bộ cell
	if titleW > 14 {
		titleW = 14
	} else if titleW < 4 {
		titleW = 4
	}
	cellW := 3 + chNumW + titleW // marker(1) + khoảng trắng(1) + số chương + khoảng trắng(1) + tiêu đề
	gutter := 4
	cols := (contentW + gutter) / (cellW + gutter)
	if cols < 1 {
		cols = 1
	} else if cols > 4 {
		cols = 4
	}
	rows := (n + cols - 1) / cols

	var b strings.Builder
	cellStyle := lipgloss.NewStyle().Width(cellW)
	gutterStr := strings.Repeat(" ", gutter)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			idx := c*rows + r
			if idx >= n {
				break
			}
			cell := renderOutlineCell(snap.Outline[idx], snap, chNumW, titleW)
			// Khi cột tiếp theo còn cell thì căn đều theo cellW + gutter; ngược lại cell hiện tại là cuối dòng không bù
			if c < cols-1 && (c+1)*rows+r < n {
				b.WriteString(cellStyle.Render(cell))
				b.WriteString(gutterStr)
			} else {
				b.WriteString(cell)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// renderOutlineCell render một cell chương đơn lẻ: hoàn thành (xanh ●) / đang tiến hành (vàng ▸) / chưa bắt đầu (mờ ○).
func renderOutlineCell(e host.OutlineSnapshot, snap host.UISnapshot, chNumW, titleW int) string {
	chStr := fmt.Sprintf("%*d", chNumW, e.Chapter)
	title := truncateWidth(e.Title, titleW)
	var marker, chRendered, titleRendered string
	switch {
	case snap.CompletedCount >= e.Chapter:
		marker = lipgloss.NewStyle().Foreground(colorSuccess).Render("●")
		chRendered = lipgloss.NewStyle().Foreground(colorDim).Render(chStr)
		titleRendered = cardContentStyle.Render(title)
	case snap.InProgressChapter == e.Chapter:
		marker = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("▸")
		chRendered = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(chStr)
		titleRendered = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(title)
	default:
		marker = lipgloss.NewStyle().Foreground(colorDim).Render("○")
		chRendered = lipgloss.NewStyle().Foreground(colorDim).Render(chStr)
		titleRendered = lipgloss.NewStyle().Foreground(colorMuted).Render(title)
	}
	return marker + " " + chRendered + " " + titleRendered
}

// truncateWidth cắt theo "chiều rộng thị giác" (ký tự tiếng Trung tính 2 cột), cùng nguồn với lipgloss.Width.
// truncate thông thường tính theo số rune, với tiếng Trung sẽ cắt đến độ rộng gấp đôi, không dùng được khi cần căn cột.
func truncateWidth(s string, maxW int) string {
	if lipgloss.Width(s) <= maxW {
		return s
	}
	var b strings.Builder
	cur := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if cur+rw > maxW {
			break
		}
		b.WriteRune(r)
		cur += rw
	}
	return b.String()
}

// renderDetailContent xây dựng nội dung panel chi tiết bên phải.
// Ưu tiên hiển thị thiết lập cơ bản (đại cương, nhân vật), sau đó là thông tin runtime (commit, đánh giá, v.v.).
func renderDetailContent(snap host.UISnapshot, contentW int) string {
	var b strings.Builder

	// Đại cương
	if len(snap.Outline) > 0 {
		outlineHeader := ":: Đại cương"
		if snap.Layered {
			outlineHeader = fmt.Sprintf(":: Đại cương (%s · đại cương quy hoạch động)", snap.CurrentVolumeArc)
		}
		b.WriteString(panelTitleStyle.Render(outlineHeader))
		b.WriteString("\n")
		b.WriteString(renderOutlineSection(snap, contentW))
		// Gợi ý quy hoạch cuộn
		compassStyle := lipgloss.NewStyle().Foreground(colorDim).Italic(true)
		if snap.Layered {
			if snap.NextVolumeTitle != "" {
				b.WriteString(compassStyle.Render("  ┄ Tập tiếp theo: " + snap.NextVolumeTitle))
				b.WriteString("\n")
			}
			b.WriteString(compassStyle.Render("  ··· Các chương tiếp theo sẽ tự động tạo theo tiến độ sáng tác"))
			b.WriteString("\n")
			if snap.CompassDirection != "" {
				direction := fmt.Sprintf("  → Kết thúc: %s", snap.CompassDirection)
				if snap.CompassScale != "" {
					direction += " (" + snap.CompassScale + ")"
				}
				b.WriteString(compassStyle.Render(truncate(direction, contentW)))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	// Nhân vật
	if len(snap.Characters) > 0 {
		b.WriteString(panelTitleStyle.Render(":: Nhân vật"))
		b.WriteString("\n")
		for _, c := range snap.Characters {
			b.WriteString(cardContentStyle.Render("· " + truncate(c, contentW-2)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Hệ sinh thái nhân vật phụ: tổng số nhân vật phụ đã xuất hiện + top 5 hoạt động gần đây
	if snap.SupportingCount > 0 {
		b.WriteString(panelTitleStyle.Render(":: Nhân vật phụ"))
		b.WriteString("\n")
		b.WriteString(cardContentStyle.Render(truncate(fmt.Sprintf("Đã xuất hiện: %d nhân vật", snap.SupportingCount), contentW)))
		b.WriteString("\n")
		for _, name := range snap.RecentSupporting {
			b.WriteString(cardContentStyle.Render("· " + truncate(name, contentW-2)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Tiền đề
	if snap.Premise != "" {
		b.WriteString(panelTitleStyle.Render(":: Tiền đề"))
		b.WriteString("\n")
		for _, line := range wrapStreamText(snap.Premise, contentW) {
			b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render(line))
			b.WriteString("\n")
		}
		b.WriteString("\n\n")
	}

	if snap.LastCommitSummary != "" {
		b.WriteString(cardTitleStyle.Render("~ Commit gần nhất ~"))
		b.WriteString("\n")
		b.WriteString(cardContentStyle.Render(snap.LastCommitSummary))
		b.WriteString("\n\n")
	}

	if snap.LastReviewSummary != "" {
		b.WriteString(cardTitleStyle.Render("~ Đánh giá gần nhất ~"))
		b.WriteString("\n")
		b.WriteString(cardContentStyle.Render(snap.LastReviewSummary))
		b.WriteString("\n\n")
	}

	if len(snap.RecentSummaries) > 0 {
		b.WriteString(cardTitleStyle.Render("~ Tóm tắt ~"))
		b.WriteString("\n")
		for _, s := range snap.RecentSummaries {
			b.WriteString(cardContentStyle.Render(truncate(s, contentW)))
			b.WriteString("\n")
		}
	}

	return b.String()
}

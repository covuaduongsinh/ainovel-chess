package tui

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/voocel/ainovel-cli/internal/host"
)

// renderInputBox render vùng nhập liệu phía dưới.
// Ô nhập chỉ phụ trách nhập liệu và gợi ý, không mang thanh chế độ khởi động.
func renderInputBox(inputView, hints string, snap host.UISnapshot, outputDir string, width int) string {
	innerW := width - 4 // border + padding
	if innerW < 12 {
		innerW = 12
	}

	// Dòng nhập: dấu nhắc + ô nhập
	prompt := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("❯ ")
	inputLine := prompt + inputView

	// Dòng gợi ý: phím tắt bên trái, tiến trình bên phải
	info := buildRightInfo(snap, outputDir)
	line2 := joinInlineSides(hints, info, innerW)

	// Vùng nhập (một hộp duy nhất, tránh xuất hiện hai ô nhập về mặt thị giác)
	inputStyle := lipgloss.NewStyle().
		Width(width).
		Border(baseBorder, true, false, true, false).
		BorderForeground(colorDim).
		Padding(0, 1)
	inputBlock := inputStyle.Render(inputLine)

	// Dòng gợi ý (không viền, sát ngay dưới đường ngang dưới)
	hintStyle := lipgloss.NewStyle().
		Width(width).
		Padding(0, 2)
	hintBlock := hintStyle.Render(line2)

	return inputBlock + "\n" + hintBlock + "\n"
}

// buildRightInfo tạo thông tin bên phải: provider · model(cửa sổ) · chi phí · thư mục.
// Thông tin tiến trình chương/số từ do panel "tổng quan" bên trái đảm nhiệm, không lặp lại ở đây.
func buildRightInfo(snap host.UISnapshot, outputDir string) string {
	var parts []string

	if snap.Provider != "" {
		parts = append(parts, snap.Provider)
	}
	if snap.ModelName != "" {
		if suffix := modelInfoSuffix(snap); suffix != "" {
			parts = append(parts, snap.ModelName+"("+suffix+")")
		} else {
			parts = append(parts, snap.ModelName)
		}
	}
	if cost := formatCostUSD(snap.TotalCostUSD); cost != "" {
		parts = append(parts, cost)
	}
	if outputDir != "" {
		parts = append(parts, "./"+filepath.Base(outputDir))
	}

	if len(parts) == 0 {
		return lipgloss.NewStyle().Foreground(colorDim).Render("READY")
	}
	return lipgloss.NewStyle().Foreground(colorDim).Render(strings.Join(parts, " · "))
}

func modelInfoSuffix(snap host.UISnapshot) string {
	var parts []string
	if w := formatContextWindow(snap.ModelContextWindow); w != "" {
		parts = append(parts, w)
	}
	if t := formatThinkingLevel(snap.ThinkingLevel); t != "" {
		parts = append(parts, t)
	}
	return strings.Join(parts, ",")
}

func formatThinkingLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "":
		return "auto"
	case "medium":
		return "med"
	default:
		return strings.ToLower(strings.TrimSpace(level))
	}
}

func joinInlineSides(left, right string, width int) string {
	if width <= 0 {
		return left + right
	}
	if strings.TrimSpace(right) == "" {
		return fitInlineLine(left, width)
	}

	right = fitInlineLine(right, width)
	rightW := ansi.StringWidth(right)
	if rightW >= width {
		return right
	}

	leftMax := width - rightW - 1
	if leftMax < 0 {
		leftMax = 0
	}
	left = fitInlineLine(left, leftMax)
	gap := width - ansi.StringWidth(left) - rightW
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func fitInlineLine(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(text) <= width {
		return text
	}
	return ansi.Truncate(text, width, "...")
}

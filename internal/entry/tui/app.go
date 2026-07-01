package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/voocel/ainovel-cli/assets"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/logger"
)

// Run khởi động TUI.
// Quy ước phân tầng chế độ khởi động:
// 1. Chế độ nhanh, chế độ cộng tác thuộc "điều phối khởi động";
// 2. Phiên sáng tác chính thức đi vào host.Host;
// 3. Nếu sau này thêm chế độ dùng chung như "tiếp tục tiểu thuyết có sẵn", thống nhất đặt tại internal/entry/startup.
func Run(cfg bootstrap.Config, bundle assets.Bundle, version string) error {
	rt, err := host.New(cfg, bundle)
	if err != nil {
		return err
	}
	bridge := newAskUserBridge()
	rt.AskUser().SetHandler(bridge.handler)
	cleanup := logger.SetupFile(rt.Dir(), "tui.log", false)
	defer cleanup()
	defer rt.Close()

	m := NewModel(rt, bridge, version)
	// Không bật báo cáo chuột toàn cục khi khởi động: trang chào không dùng chuột,
	// tắt báo cáo giữ nguyên tính năng kéo chọn sao chép gốc của terminal.
	// Khi vào bàn làm việc sáng tác (modeRunning), enterRunning sẽ bật báo cáo,
	// hỗ trợ nhấp để chuyển panel / con lăn / kéo thanh bên.
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

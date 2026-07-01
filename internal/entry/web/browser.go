package web

import (
	"os/exec"
	"runtime"
)

// openBrowser cố gắng mở trình duyệt mặc định tới URL chỉ định; thất bại thì im lặng (người dùng có thể sao chép URL thủ công).
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

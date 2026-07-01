// Package notify cung cấp kênh cảnh báo không người giám sát.
//
// Định vị hợp hiến (architecture.md §2.3): hành động tầng quan sát thuần túy -- cảnh báo không bao giờ can thiệp vào luồng điều khiển
// (không thử lại, không điều phái lại, không dừng máy), chỉ "hét to" các sự kiện đã có trong TUI ra ngoài màn hình.
// Send thực thi bất đồng bộ, không bao giờ chặn Host, thất bại chỉ ghi slog.
package notify

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Notification chứa toàn bộ thông tin của một cảnh báo.
type Notification struct {
	Kind  string `json:"kind"`  // run_end / repeat / budget
	Level string `json:"level"` // info / warn / error
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Notifier phân phát thông báo theo cấu hình. Giá trị zero không dùng được, phải tạo qua New; nil an toàn (Send noop).
type Notifier struct {
	command string          // khi không rỗng thì thay thế kênh system (push điện thoại đi qua đây)
	events  map[string]bool // nil = thả qua tất cả kind
	timeout time.Duration
}

// New tạo Notifier. Khi command rỗng thì dùng kênh system nội tích (macOS osascript /
// Linux notify-send, không tìm thấy lệnh thì im lặng hạ cấp xuống chỉ slog); khi events không rỗng thì chỉ thả qua kind đã liệt kê.
func New(command string, events []string) *Notifier {
	n := &Notifier{command: strings.TrimSpace(command), timeout: 10 * time.Second}
	if len(events) > 0 {
		n.events = make(map[string]bool, len(events))
		for _, ev := range events {
			n.events[ev] = true
		}
	}
	return n
}

// Send gửi một thông báo bất đồng bộ. Lọc, thực thi, xử lý thất bại đều không ảnh hưởng đến người gọi.
func (n *Notifier) Send(nt Notification) {
	if !n.allows(nt.Kind) {
		return
	}
	go n.deliver(nt)
}

// allows trả về kind này có được thả qua không (chặn khi nil Notifier / chưa liệt kê vào events).
func (n *Notifier) allows(kind string) bool {
	if n == nil {
		return false
	}
	return n.events == nil || n.events[kind]
}

// deliver thực thi đồng bộ một lần gửi (chạy trong goroutine; test có thể gọi trực tiếp để xác nhận đồng bộ).
func (n *Notifier) deliver(nt Notification) {
	defer func() { recover() }()
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	defer cancel()

	var err error
	if n.command != "" {
		err = runCommand(ctx, n.command, nt)
	} else {
		err = runSystem(ctx, nt)
	}
	if err != nil {
		slog.Warn("Gửi thông báo thất bại", "module", "notify", "kind", nt.Kind, "err", err)
	}
}

// runCommand thực thi lệnh người dùng đã cấu hình: các trường được truyền qua biến môi trường (một dòng curl không phụ thuộc, không rủi ro injection),
// JSON đầy đủ đồng thời ghi vào stdin (các kịch bản phân phát phức tạp tự phân tích). Hết thời gian bị ctx giết cưỡng bức.
func runCommand(ctx context.Context, command string, nt Notification) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Env = append(os.Environ(),
		"NOTIFY_KIND="+nt.Kind,
		"NOTIFY_LEVEL="+nt.Level,
		"NOTIFY_TITLE="+nt.Title,
		"NOTIFY_BODY="+nt.Body,
	)
	payload, _ := json.Marshal(nt)
	cmd.Stdin = strings.NewReader(string(payload))
	return cmd.Run()
}

// runSystem thông báo máy tính để bàn nội tích: chỉ bao phủ tình huống "người ngồi bên máy tính", không tìm thấy lệnh thì im lặng hạ cấp.
func runSystem(ctx context.Context, nt Notification) error {
	switch runtime.GOOS {
	case "darwin":
		script := "display notification " + appleScriptString(nt.Body) + " with title " + appleScriptString(nt.Title)
		return exec.CommandContext(ctx, "osascript", "-e", script).Run()
	case "linux":
		if _, err := exec.LookPath("notify-send"); err != nil {
			slog.Info("Thông báo hạ cấp xuống nhật ký (không có notify-send)", "module", "notify", "title", nt.Title, "body", nt.Body)
			return nil
		}
		return exec.CommandContext(ctx, "notify-send", nt.Title, nt.Body).Run()
	default:
		slog.Info("Thông báo hạ cấp xuống nhật ký (nền tảng không có kênh system)", "module", "notify", "title", nt.Title, "body", nt.Body)
		return nil
	}
}

// appleScriptString bọc văn bản tùy ý thành literal chuỗi AppleScript.
func appleScriptString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

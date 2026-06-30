package flow

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/voocel/agentcore"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

// Dispatcher tính định tuyến tại ranh giới công cụ đồng bộ khi subagent trả về và ra lệnh Host.
type Dispatcher struct {
	coordinator *agentcore.Agent
	store       *storepkg.Store

	enabled atomic.Bool // do Host kiểm soát có phân phối hay không (trước khi khởi động xong thì nên tắt)

	// Theo dõi lặp: nhớ Agent+Task của lần phân phối gần nhất và số lần ra lệnh liên tiếp.
	// Cùng một lệnh tính lại trùng (sau khi subagent trả về trạng thái không tiến triển, Route tính lại kết quả
	// không đổi) không nuốt lặng, mà phát lại kèm sự thật số lần — "kết quả định tuyến giống nhau N lần liên tiếp"
	// là sự thật chỉ Host quan sát được; nếu im lặng, Coordinator sẽ rơi vào mâu thuẫn kép giữa "cấm tự quyết bước
	// tiếp theo" (coordinator.md) và "cấm dừng máy" (StopGuard), tự do phát huy chính là vòng lặp chết freelance kiểu #24.
	// Quyền phán định vẫn ở LLM: thông điệp phát lại chỉ kèm sự thật và giấy phép đối chiếu, không đặt ngưỡng, không ngắt mạch (kiến trúc §10.13).
	// Thông điệp vì kèm số lần nên khác nhau, không đẩy lặp lệnh giống chữ vào steeringQ.
	lastMu   sync.Mutex
	lastSent *Instruction
	repeats  int

	// onRepeat là callback telemetry thuần (dùng cho cảnh báo không người trực), kích hoạt một lần khi cùng một
	// lệnh ra đến lần thứ repeatNotifyAt; không ảnh hưởng ngược tới phân phối, logic phân phối vô cảm với sự tồn tại của nó.
	onRepeat func(agent, task string, n int)
}

// repeatNotifyAt viết chết không vào config: nó không phải ngưỡng luồng điều khiển (không kích hoạt hành động nào,
// chỉ "gọi người"), chỉnh nó không có lợi ích; vào config ngược lại ngầm gợi ý có thể chỉnh ra khác biệt hành vi.
const repeatNotifyAt = 3

// NewDispatcher tạo Dispatcher.
func NewDispatcher(coordinator *agentcore.Agent, store *storepkg.Store) *Dispatcher {
	d := &Dispatcher{coordinator: coordinator, store: store}
	return d
}

// Enable bật phân phối định tuyến; khi tắt thì Dispatch không sinh lệnh.
// Host bật sau khi Start/Resume hoàn thành prompt đầu tiên, tránh xung đột với luồng khởi động.
func (d *Dispatcher) Enable() { d.enabled.Store(true) }

// Dispatch tính định tuyến ngay và ra lệnh; có thể được Host chủ động gọi vào thời điểm đặc biệt (như sau Resume).
func (d *Dispatcher) Dispatch() {
	if !d.enabled.Load() {
		return
	}
	state := LoadState(d.store)
	inst := Route(state)
	if inst == nil {
		return
	}
	n := d.trackRepeat(inst)
	// Nhiệm vụ Writer: ngay khi phân phối thì đánh dấu chương là đang tiến hành, dàn ý bên phải UI lập tức phản ánh
	// "▸ đang tiến hành", không phải chờ plan_chapter thực sự chạy (plan_chapter sẽ gọi StartChapter lần nữa, lũy đẳng).
	if inst.Agent == "writer" && inst.Chapter > 0 && d.store != nil {
		if err := d.store.Progress.ValidateChapterWork(inst.Chapter); err != nil {
			slog.Error("flow router refuses invalid writer dispatch", "module", "host.flow", "chapter", inst.Chapter, "err", err)
			return
		}
		if err := d.store.Progress.StartChapter(inst.Chapter); err != nil {
			slog.Warn("flow router pre-mark in-progress failed", "module", "host.flow", "chapter", inst.Chapter, "err", err)
		}
	}
	msg := formatDispatchMessage(inst, n)
	slog.Debug("flow router dispatch", "module", "host.flow", "agent", inst.Agent, "reason", inst.Reason, "repeat", n)
	d.coordinator.Steer(agentcore.UserMsg(msg))
}

// formatDispatchMessage lắp ráp thông điệp lệnh ra cho Coordinator.
// Khi n>1 thì đính kèm sự thật lặp — báo "sau lần phân phối trước sự thật định tuyến không đổi" và mở giấy phép đối chiếu,
// để LLM tự phán định thực thi như cũ hay đổi phân công; không làm phân nhánh cưỡng bức nào ở tầng Host.
func formatDispatchMessage(inst *Instruction, n int) string {
	msg := FormatMessage(inst)
	if n > 1 {
		msg += fmt.Sprintf("\n(Lưu ý: lệnh này là lần ra lệnh thứ %d — sau lần phân phối trước sự thật định tuyến không đổi. Lần này cho phép gọi novel_context đối chiếu sự thật trước, rồi phán định thực thi như cũ hay đổi phân công subagent khác.)", n)
	}
	return msg
}

// SetOnRepeat đăng ký callback telemetry cho lệnh lặp. Phải gọi một lần trước khi bắt đầu phân phối.
func (d *Dispatcher) SetOnRepeat(cb func(agent, task string, n int)) {
	d.onRepeat = cb
}

// trackRepeat ghi số lần ra lệnh giống nhau liên tiếp và trả về số lần hiện tại (1 = lệnh mới).
// Dùng đẳng thức Agent+Task (không so Reason, vì Reason là văn bản phụ cho người xem).
// Khi số lần đúng bằng repeatNotifyAt thì kích hoạt onRepeat một lần ngoài lock (sau khi khóa đổi, đếm lại rồi vũ trang lại).
func (d *Dispatcher) trackRepeat(next *Instruction) int {
	d.lastMu.Lock()
	if d.lastSent != nil && d.lastSent.Agent == next.Agent && d.lastSent.Task == next.Task {
		d.repeats++
	} else {
		cp := *next
		d.lastSent = &cp
		d.repeats = 1
	}
	n := d.repeats
	d.lastMu.Unlock()

	if n == repeatNotifyAt && d.onRepeat != nil {
		d.onRepeat(next.Agent, next.Task, n)
	}
	return n
}

// ResetRepeat xóa theo dõi lặp. Host gọi khi Resume / Start mới,
// bảo đảm lệnh đầu tiên sau khi khôi phục hoặc tạo mới ra theo ngữ nghĩa "lần thứ 1".
func (d *Dispatcher) ResetRepeat() {
	d.lastMu.Lock()
	defer d.lastMu.Unlock()
	d.lastSent = nil
	d.repeats = 0
}

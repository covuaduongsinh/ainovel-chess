package host

import (
	"fmt"
	"math"
	"sync/atomic"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
)

// Máy trạng thái ngân sách: tiến đơn điệu, mỗi lần chuyển đúng kích hoạt một lần tác dụng phụ, không lùi.
// Tăng ngân sách = người dùng ủy quyền lại = thay cấu hình rồi khởi động lại/Host instance mới, không lùi trạng thái trong instance này.
const (
	budgetNormal      int32 = iota // Chưa đến mức cảnh báo
	budgetWarned                   // Đã phát cảnh báo, chưa vượt ngưỡng
	budgetStopPending              // Đã vượt ngưỡng, chờ dừng tại ranh giới subagent
	budgetStopped                  // Đã thực thi dừng máy
)

// BudgetSentinel giám sát chi phí lũy kế, thực thi chính sách ngân sách của người dùng (khối config budget).
//
// Định vị hợp pháp (architecture.md §8.3/§10): Không đánh giá hành vi mô hình — vượt ngưỡng dừng máy tương đương
// người dùng tại thời điểm đó Abort thủ công, Host chỉ thay mặt thực thi một chỉ thị đã ký trước.
// Nó ảnh hưởng luồng điều khiển, vì vậy không phải observer, định vị là thành phần chính sách Host
// ngang hàng với flow.Dispatcher; tầng Route/công cụ không cảm nhận.
//
// Thời điểm dừng: mặc định tại ranh giới subagent (Host gọi đồng bộ HandleBoundary), không lãng phí chương đang chạy;
// khi hardStop=true thì vượt ngưỡng dừng ngay. Xử lý ranh giới trước khi flow.Dispatcher dispatch bước tiếp theo,
// tầng Route/công cụ không cảm nhận ngân sách.
type BudgetSentinel struct {
	limit     float64
	warnRatio float64
	hardStop  bool

	costNow func() float64              // Chi phí lũy kế hiện tại (bọc usage.Totals; có thể inject test stub)
	abort   func(reason string)         // Bọc dừng máy Host (kèm sự kiện lý do)
	report  func(level, summary string) // Đầu ra cảnh báo (emitEvent + notify, do Host inject)

	state atomic.Int32

	// Phát hiện vùng mù tính phí: mô hình không có giá trong registry và provider không tự báo cost
	// thì mỗi lần ghi tài tăng lên $0, ngân sách âm thầm mất hiệu lực. Phán định theo "nhiều lần
	// tăng liên tiếp bằng 0" thay vì total==0 — cách sau không bắt được kịch bản giữa chừng /model
	// chuyển sang mô hình không giá (total dừng ở giá trị lịch sử khác 0 nhưng không tăng nữa).
	// Mô hình miễn phí cũng kích hoạt, gợi ý "ngân sách không kích hoạt" với chúng cũng đúng.
	lastTotal   atomic.Uint64 // math.Float64bits(chi phí lũy kế lần callback trước)
	zeroStreak  atomic.Int32
	blindWarned atomic.Bool
}

// blindZeroStreak bao nhiêu lần ghi tài tăng liên tiếp bằng 0 thì cảnh báo. Mô hình tính giá bình thường
// mỗi lần tăng nhất định > 0 (cost là float lũy kế không làm tròn), lấy 5 chỉ để tránh đột biến cực đoan,
// không phải ngưỡng chiến lược có thể điều chỉnh.
const blindZeroStreak = 5

// NewBudgetSentinel tạo BudgetSentinel; khi chính sách chưa bật thì trả về nil (tất cả phương thức an toàn với nil).
func NewBudgetSentinel(cfg bootstrap.BudgetConfig, costNow func() float64, abort func(reason string), report func(level, summary string)) *BudgetSentinel {
	if !cfg.Enabled() {
		return nil
	}
	return &BudgetSentinel{
		limit:     cfg.BookUSD,
		warnRatio: cfg.WarnRatio,
		hardStop:  cfg.HardStop,
		costNow:   costNow,
		abort:     abort,
		report:    report,
	}
}

// OnCost được UsageTracker gọi sau mỗi lần ghi tài, mang theo chi phí lũy kế mới nhất (ngoài lock).
// Một lần callback có thể nhảy qua hai cấp (normal→warned→stopPending), hai tác dụng phụ mỗi cái kích hoạt một lần.
func (s *BudgetSentinel) OnCost(total float64) {
	if s == nil {
		return
	}
	if prev := s.lastTotal.Swap(math.Float64bits(total)); total == math.Float64frombits(prev) {
		if s.zeroStreak.Add(1) >= blindZeroStreak && s.blindWarned.CompareAndSwap(false, true) {
			s.report("warn", fmt.Sprintf("Vùng mù ngân sách: ghi tài liên tục nhưng chi phí lũy kế dừng ở $%.2f không tăng nữa (mô hình hiện tại không có giá trong registry và provider không tự báo cost, hoặc là mô hình miễn phí) — ngưỡng ngân sách sẽ không kích hoạt", total))
		}
	} else {
		s.zeroStreak.Store(0)
	}
	if total >= s.limit*s.warnRatio && s.state.CompareAndSwap(budgetNormal, budgetWarned) {
		s.report("warn", fmt.Sprintf("Cảnh báo ngân sách: đã chi $%.2f, đạt %.0f%% ngân sách $%.2f", total, s.warnRatio*100, s.limit))
	}
	if total >= s.limit && s.state.CompareAndSwap(budgetWarned, budgetStopPending) {
		if s.hardStop {
			s.report("error", fmt.Sprintf("Ngân sách cạn: đã chi $%.2f, vượt ngân sách $%.2f, dừng máy ngay lập tức", total, s.limit))
			s.stop(total)
			return
		}
		s.report("error", fmt.Sprintf("Ngân sách cạn: đã chi $%.2f, vượt ngân sách $%.2f, sẽ dừng máy sau khi nhiệm vụ subagent hiện tại kết thúc", total, s.limit))
	}
}

// HandleEvent thực thi lệnh dừng đang chờ tại ranh giới subagent. Đăng ký phải trước Dispatcher.
// Không bỏ qua IsError — trả về lỗi cũng là ranh giới, dừng máy không nên bị trì hoãn do subagent thất bại.
func (s *BudgetSentinel) HandleEvent(ev agentcore.Event) {
	if s == nil {
		return
	}
	if ev.Type != agentcore.EventToolExecEnd || ev.Tool != "subagent" {
		return
	}
	s.HandleBoundary()
}

func (s *BudgetSentinel) HandleBoundary() bool {
	if s == nil || s.state.Load() != budgetStopPending {
		return false
	}
	s.stop(s.costNow())
	return true
}

func (s *BudgetSentinel) stop(total float64) {
	if s.state.CompareAndSwap(budgetStopPending, budgetStopped) {
		s.abort(fmt.Sprintf("Dừng máy do ngân sách: đã chi $%.2f, vượt ngân sách $%.2f; tăng budget.book_usd có thể tiếp tục chạy", total, s.limit))
	}
}

// Refuse kiểm tra trước khi khởi động: ngân sách đã vượt thì trả về lỗi từ chối (đường khôi phục Start/Resume/Continue gọi).
// Người dùng tăng ngân sách = ủy quyền lại, Refuse tự nhiên cho qua với cấu hình mới.
func (s *BudgetSentinel) Refuse() error {
	if s == nil {
		return nil
	}
	if cost := s.costNow(); cost >= s.limit {
		return fmt.Errorf("cuốn sách này đã chi $%.2f, đạt giới hạn ngân sách $%.2f; vui lòng tăng cấu hình budget.book_usd rồi thử lại", cost, s.limit)
	}
	return nil
}

// Limit 返回预算上限（UI 展示用）；未启用返回 0。
func (s *BudgetSentinel) Limit() float64 {
	if s == nil {
		return 0
	}
	return s.limit
}

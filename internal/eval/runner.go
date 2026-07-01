package eval

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/assets"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/entry/startup"
	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/logger"
)

// RunOptions điều khiển một lần chạy case đơn.
type RunOptions struct {
	OutputDir string        // thư mục đầu ra cô lập (bắt buộc)
	Timeout   time.Duration // giới hạn thời gian thực cho mỗi case; 0 = không giới hạn
	Progress  io.Writer     // đầu ra dòng tiến trình (tùy chọn, nil thì không in)
}

// RunCase điều khiển một lần chạy case: lắp ráp host → khởi động → tiến theo giới hạn số chương → Abort khi đến điểm.
// bundle đã được phía gọi ghi đè variant (nếu có). error trả về là "lỗi runtime" (cơ sở hard fail);
// viết xong bình thường hoặc dừng bình thường đều trả về nil. Không đặt ask_user handler —
// trong chế độ không người trực, công cụ này tự động trả về thông báo không chặn.
//
// RunCase chiếm độc quyền và reset OutputDir: StartPrepared chỉ reset progress/checkpoints,
// không xóa artifact như chapters/foundation, tái sử dụng thư mục cũ sẽ để sản phẩm còn sót
// làm ô nhiễm diag và novel_context. Vì vậy xóa sạch trước khi chạy để đảm bảo cách ly.
func RunCase(cfg bootstrap.Config, bundle assets.Bundle, c Case, opts RunOptions) error {
	if strings.TrimSpace(opts.OutputDir) == "" {
		return fmt.Errorf("RunCase: thiếu OutputDir")
	}
	if err := os.RemoveAll(opts.OutputDir); err != nil {
		return fmt.Errorf("dọn dẹp thư mục đầu ra: %w", err)
	}
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return fmt.Errorf("tạo thư mục đầu ra: %w", err)
	}
	cfg.OutputDir = opts.OutputDir
	if c.Style != "" {
		cfg.Style = c.Style
	}

	eng, err := host.New(cfg, bundle)
	if err != nil {
		return fmt.Errorf("lắp ráp host: %w", err)
	}
	// Ghi vào logs/headless.log, các quy tắc runtime của diag (stream idle storm, v.v.) lấy bằng chứng từ đây;
	// session jsonl được engine tự ghi, không cần nối thêm. Thứ tự defer căn chỉnh với headless:
	// Close thực thi trước cleanup, log kết thúc vẫn được file bắt.
	cleanup := logger.SetupFile(eng.Dir(), "headless.log", false)
	defer cleanup()
	defer eng.Close()

	plan, err := startup.PrepareQuick(startup.Request{
		Mode:       startup.ModeQuick,
		UserPrompt: c.Prompt,
		OutputDir:  eng.Dir(),
	})
	if err != nil {
		return err
	}
	if err := eng.PrepareUserRules(plan.RawPrompt); err != nil {
		return fmt.Errorf("chuẩn bị quy tắc người dùng: %w", err)
	}
	if err := eng.StartPrepared(plan.StartPrompt); err != nil {
		return fmt.Errorf("khởi động: %w", err)
	}

	return drive(eng, c.MaxChapters, opts)
}

// driveEngine là giao diện engine tối thiểu mà drive tiêu thụ (*host.Host tự nhiên thỏa mãn).
// Được tách ra để viết test xác định luận cho kỷ luật drain-to-Done —
// đoạn logic concurrent này từng có lỗi send-on-closed-channel.
type driveEngine interface {
	Events() <-chan host.Event
	Stream() <-chan string
	Done() <-chan struct{}
	Snapshot() host.UISnapshot
	Abort() bool
}

// drive tiêu thụ luồng sự kiện engine, đến giới hạn số chương hoặc timeout thì Abort, chờ Done để kết thúc.
//
// Kỷ luật quan trọng: dù hoàn thành bình thường, dừng theo số chương hay timeout, đều phải
// drain đến Done mới trả về. host nền waitDone sẽ gửi vào done một lần, còn eng.Close()
// (defer của RunCase) sẽ close(done) — trả về sớm kích hoạt Close sẽ cạnh tranh với gửi của
// waitDone để đóng channel mà panic (send on closed channel). headless cũng dựa vào "Done trước
// Close". Đồng thời phải drain hết Events và Stream để tránh chặn engine.
func drive(eng driveEngine, maxChapters int, opts RunOptions) error {
	var timeoutCh <-chan time.Time
	if opts.Timeout > 0 {
		t := time.NewTimer(opts.Timeout)
		defer t.Stop()
		timeoutCh = t.C
	}

	aborted, timedOut := false, false
	// finish được gọi sau khi drain đến Done (hoặc channel đóng): timeout thì trả về error, ngược lại kết thúc bình thường.
	finish := func() error {
		if timedOut {
			return fmt.Errorf("chạy quá thời gian (%s)", opts.Timeout)
		}
		return nil
	}
	for {
		select {
		case ev, ok := <-eng.Events():
			if !ok {
				return finish()
			}
			if opts.Progress != nil && strings.TrimSpace(ev.Summary) != "" {
				fmt.Fprintf(opts.Progress, "    [%s] %s\n", ev.Category, ev.Summary)
			}
			if !aborted && capReached(eng.Snapshot(), maxChapters) {
				eng.Abort()
				aborted = true
				timeoutCh = nil // đã đạt điều kiện dừng, chuyển sang kết thúc bình thường, không còn bị ràng buộc timeout (tránh nhầm dừng thành công với timeout)
			}
		case <-eng.Stream():
			// Drain hết dòng gia tăng, không tiêu thụ nội dung — eval không quan tâm luồng chính văn, chỉ nhìn sự thật đã ghi đĩa.
		case _, ok := <-eng.Done():
			if !ok {
				return finish()
			}
			return finish()
		case <-timeoutCh:
			eng.Abort() // tại đây aborted nhất định là false (cap dừng sẽ đặt timeoutCh thành nil)
			aborted, timedOut = true, true
			timeoutCh = nil // tắt timer, tiếp tục drain cho đến Done, rồi finish trả về lỗi timeout
		}
	}
}

// capReached kiểm tra có đạt điều kiện dừng không. maxChapters>0 theo số chương đã hoàn thành;
// <=0 được coi là "loại quy hoạch", quy hoạch hoàn thành (vào writing hoặc đã complete) thì dừng.
func capReached(snap host.UISnapshot, maxChapters int) bool {
	if maxChapters <= 0 {
		return snap.Phase == string(domain.PhaseWriting) || snap.Phase == string(domain.PhaseComplete)
	}
	return snap.CompletedCount >= maxChapters
}

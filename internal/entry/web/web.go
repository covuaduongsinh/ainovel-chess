// Package web là điểm vào thứ ba của ainovel-cli (sau TUI / headless):
// Phơi bày API engine host.Host không phụ thuộc giao diện qua HTTP + SSE cho trình duyệt,
// cung cấp một bàn làm việc sáng tác cục bộ, thân thiện và trực quan. Không thay đổi bất kỳ logic engine nào.
package web

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/voocel/ainovel-cli/assets"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/rules"
)

// Options kiểm soát hành vi của điểm vào web.
type Options struct {
	Port      int    // cổng lắng nghe; 0 hoặc bị chiếm thì tự động chọn cổng trống
	Open      bool   // có tự động mở trình duyệt không
	OutputDir string // ghi đè thư mục đầu ra (mặc định output/novel)
}

// swapHandler cho phép cùng một cổng lắng nghe chuyển đổi liền mạch giữa "trang thiết lập lần đầu" và "bàn làm việc chính thức":
// Sau khi thiết lập xong chỉ cần đổi handler nội bộ thành mux chính thức, trình duyệt làm mới là vào bàn làm việc.
type swapHandler struct{ h atomic.Value }

func (s *swapHandler) set(h http.Handler) { s.h.Store(h) }
func (s *swapHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h, ok := s.h.Load().(http.Handler); ok {
		h.ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}

// RunWeb là điều phối tổng thể của điểm vào web: xử lý thiết lập lần đầu (hướng dẫn trong trình duyệt) rồi khởi động bàn làm việc,
// chặn cho đến khi nhận tín hiệu ngắt hoặc máy chủ thoát. configPath rỗng nghĩa là dùng đường dẫn tìm kiếm mặc định.
func RunWeb(configPath, version string, opts Options) error {
	rules.EnsureHomeRulesDir()

	ln, err := listen(opts.Port)
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String()

	swap := &swapHandler{}
	httpServer := &http.Server{Handler: swap}
	serveErr := make(chan error, 1)
	go func() {
		if e := httpServer.Serve(ln); e != nil && e != http.ErrServerClosed {
			serveErr <- e
			return
		}
		serveErr <- nil
	}()

	// Tắt nhẹ nhàng: nhận tín hiệu ngắt → Shutdown.
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		fmt.Fprintln(os.Stderr, "\nĐang tắt máy chủ web...")
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}()

	browserOpened := false

	// Thiết lập lần đầu: hướng dẫn trong trình duyệt, viết cấu hình xong rồi tiếp tục.
	if bootstrap.NeedsSetup(configPath) {
		done := make(chan struct{}, 1)
		swap.set(newSetupMux(configPath, done))
		banner(url, "Thiết lập lần đầu — hãy chọn nhà cung cấp và nhập API key trên trình duyệt.")
		if opts.Open {
			openBrowser(url)
			browserOpened = true
		}
		select {
		case <-done:
		case e := <-serveErr:
			return e
		}
	}

	cfg, err := bootstrap.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	cfg.FillDefaults()
	bundle := assets.Load(cfg.Style)

	// root là thư mục cha chứa các dự án (mặc định "output", cha của output/novel).
	root := filepath.Dir(cfg.OutputDir)

	sm := newSessionManager(swap, cfg, bundle, version, configPath, root)
	defer sm.closeCurrent()

	if opts.OutputDir != "" {
		// Tương thích script: --output mở thẳng dự án được chỉ định, bỏ qua màn chọn dự án.
		if err := sm.open(opts.OutputDir); err != nil {
			return err
		}
		banner(url, "Thư mục tác phẩm: "+opts.OutputDir)
	} else {
		sm.showPicker()
		banner(url, "Hãy chọn hoặc tạo một dự án sách trên trình duyệt.")
	}
	if opts.Open && !browserOpened {
		openBrowser(url)
	}

	return <-serveErr
}

func banner(url, note string) {
	fmt.Println("══════════════════════════════════════════════")
	fmt.Println("  ainovel web đang chạy tại:  " + url)
	if note != "" {
		fmt.Println("  " + note)
	}
	fmt.Println("  Nhấn Ctrl+C để dừng máy chủ.")
	fmt.Println("══════════════════════════════════════════════")
}

// listen ràng buộc 127.0.0.1:port. Khi port bị chiếm (hoặc bằng 0) thì dùng cổng trống ngẫu nhiên.
func listen(port int) (net.Listener, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		return ln, nil
	}
	if port == 0 {
		return nil, err
	}
	return net.Listen("tcp", "127.0.0.1:0")
}

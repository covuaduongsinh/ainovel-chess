package web

import (
	"sync"

	"github.com/voocel/ainovel-cli/assets"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/logger"
)

// sessionManager quản lý vòng đời của "phiên dự án" đang mở trong Web: tại một thời điểm chỉ có
// đúng một host.Host + Server + hub gắn với một thư mục dự án. Đổi dự án = đóng phiên cũ hoàn toàn
// rồi dựng phiên mới, sau đó swap handler trên cùng cổng (tận dụng swapHandler sẵn có).
//
// host.Host cố tình là vỏ mỏng gắn cứng một thư mục suốt vòng đời; mọi logic "chuyển dự án" nằm ở
// tầng này, không đụng vào package host.
type sessionManager struct {
	swap       *swapHandler
	cfg        bootstrap.Config
	bundle     assets.Bundle
	version    string
	configPath string
	root       string // thư mục cha chứa các dự án (mặc định là cha của cfg.OutputDir)

	mu       sync.Mutex
	cur      *Server
	curClean func() // dọn dẹp phiên hiện tại (đóng hub + Close host + đóng log)
}

func newSessionManager(swap *swapHandler, cfg bootstrap.Config, bundle assets.Bundle, version, configPath, root string) *sessionManager {
	return &sessionManager{
		swap:       swap,
		cfg:        cfg,
		bundle:     bundle,
		version:    version,
		configPath: configPath,
		root:       root,
	}
}

// open mở (hoặc chuyển sang) dự án tại dir: đóng phiên cũ, dựng host mới, swap sang workbench.
func (sm *sessionManager) open(dir string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.closeCurrentLocked()

	pcfg := sm.cfg
	pcfg.OutputDir = dir
	clean := logger.SetupFile(dir, "web.log", false)

	eng, err := host.New(pcfg, sm.bundle)
	if err != nil {
		clean()
		return err
	}

	srv := newServer(eng, pcfg, sm.bundle, sm.version)
	srv.sm = sm
	go srv.hub.run()

	sm.cur = srv
	sm.curClean = func() {
		// Thứ tự bắt buộc: dừng hub TRƯỚC (điểm dừng tất định, tránh spurious done/Snapshot),
		// rồi mới Close host.
		srv.hub.close()
		srv.hub.wait()
		srv.hub.dropAllClients()
		eng.Close()
		clean()
	}
	sm.swap.set(srv.mux)
	return nil
}

// showPicker đóng phiên hiện tại và chuyển giao diện về màn chọn dự án.
func (sm *sessionManager) showPicker() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.closeCurrentLocked()
	sm.swap.set(newPickerMux(sm))
}

// closeCurrent đóng phiên hiện tại (nếu có). An toàn khi gọi qua defer lúc RunWeb thoát.
func (sm *sessionManager) closeCurrent() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.closeCurrentLocked()
}

func (sm *sessionManager) closeCurrentLocked() {
	if sm.curClean != nil {
		sm.curClean()
		sm.curClean = nil
	}
	sm.cur = nil
}

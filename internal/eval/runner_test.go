package eval

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/host"
)

// fakeEngine mô phỏng host: sau Abort gửi vào done một lần như waitDone. done có buffer 1,
// test dùng để kiểm định drive có drain Done không (len(done)==0 là đã tiêu thụ) —
// đây là bất biến quan trọng ngăn panic send-on-closed-channel.
type fakeEngine struct {
	events chan host.Event
	stream chan string
	done   chan struct{}

	mu      sync.Mutex
	snap    host.UISnapshot
	aborted bool
}

func newFakeEngine() *fakeEngine {
	return &fakeEngine{
		events: make(chan host.Event, 4),
		stream: make(chan string),
		done:   make(chan struct{}, 1),
	}
}

func (f *fakeEngine) Events() <-chan host.Event { return f.events }
func (f *fakeEngine) Stream() <-chan string     { return f.stream }
func (f *fakeEngine) Done() <-chan struct{}     { return f.done }

func (f *fakeEngine) Snapshot() host.UISnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snap
}

func (f *fakeEngine) Abort() bool {
	f.mu.Lock()
	f.aborted = true
	f.mu.Unlock()
	select { // mô phỏng waitDone: sau khi abort kích hoạt thì gửi vào done một lần
	case f.done <- struct{}{}:
	default:
	}
	return true
}

func (f *fakeEngine) wasAborted() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.aborted
}

// Đường dẫn timeout phải Abort rồi drain đến Done mới trả về lỗi timeout —
// nếu không Close của RunCase sẽ cạnh tranh với waitDone để đóng channel done mà panic (Codex review #1).
func TestDriveTimeoutDrainsToDone(t *testing.T) {
	f := newFakeEngine()
	err := drive(f, 1, RunOptions{Timeout: 30 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "quá thời gian") {
		t.Fatalf("timeout phải trả về lỗi timeout, nhận được %v", err)
	}
	if !f.wasAborted() {
		t.Fatal("timeout phải kích hoạt Abort")
	}
	if len(f.done) != 0 {
		t.Fatal("drive phải drain Done rồi mới trả về (nếu không cạnh tranh đóng channel với Close mà panic)")
	}
}

// Đạt giới hạn số chương: Abort rồi drain đến Done, trả về nil (dừng bình thường, không phải timeout).
func TestDriveCapStopsAndDrains(t *testing.T) {
	f := newFakeEngine()
	f.mu.Lock()
	f.snap = host.UISnapshot{CompletedCount: 1}
	f.mu.Unlock()
	f.events <- host.Event{Category: "SYSTEM", Summary: "committed"} // kích hoạt kiểm tra cap

	err := drive(f, 1, RunOptions{Timeout: time.Second})
	if err != nil {
		t.Fatalf("dừng bình thường phải trả về nil, nhận được %v", err)
	}
	if !f.wasAborted() {
		t.Fatal("đạt giới hạn số chương phải Abort")
	}
	if len(f.done) != 0 {
		t.Fatal("phải drain Done rồi mới trả về")
	}
}

// Engine tự nhiên Done (viết xong): không cần Abort, trả về nil.
func TestDriveNaturalDoneReturnsNil(t *testing.T) {
	f := newFakeEngine()
	f.done <- struct{}{}

	err := drive(f, 1, RunOptions{Timeout: time.Second})
	if err != nil {
		t.Fatalf("hoàn thành tự nhiên phải trả về nil, nhận được %v", err)
	}
	if f.wasAborted() {
		t.Fatal("hoàn thành tự nhiên không được Abort")
	}
}

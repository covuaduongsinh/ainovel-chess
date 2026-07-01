package web

import (
	"sync"
	"time"

	"github.com/voocel/ainovel-cli/internal/host"
)

// hub là trung tâm phát sóng SSE: tiêu thụ duy nhất ba kênh đơn tiêu thụ Events/Stream/Done của engine,
// phân phát tới tất cả các tab trình duyệt đã kết nối. Kênh engine có ngữ nghĩa đơn tiêu thụ, do đó vòng lặp
// run của hub phải chạy suốt vòng đời tiến trình (không phụ thuộc vào việc có client kết nối hay không).
type hub struct {
	eng *host.Host

	mu       sync.RWMutex
	clients  map[*sseClient]struct{}
	backlog  []sseMessage // bộ đệm cuộn trong bộ nhớ, bổ sung nội dung gần đây cho kết nối mới
	maxLog   int
	dirty    bool      // có thay đổi kể từ lần đẩy snapshot cuối không
	lastSnap time.Time // thời gian đẩy snapshot lần cuối (giới hạn tần suất)

	stop chan struct{}
}

// sseClient là một kết nối trình duyệt đơn. Khi ch đầy sẽ bỏ qua (client chậm không chặn phát sóng).
type sseClient struct {
	ch chan sseMessage
}

func newHub(eng *host.Host) *hub {
	return &hub{
		eng:     eng,
		clients: make(map[*sseClient]struct{}),
		maxLog:  600,
		stop:    make(chan struct{}),
	}
}

func (h *hub) addClient() *sseClient {
	c := &sseClient{ch: make(chan sseMessage, 512)}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	backlog := append([]sseMessage(nil), h.backlog...)
	h.mu.Unlock()

	// Kết nối mới nhận snapshot trước, sau đó bổ sung backlog, giúp trang sau khi làm mới lập tức khôi phục trạng thái.
	c.ch <- sseMessage{Type: msgSnapshot, Data: h.eng.Snapshot()}
	for _, m := range backlog {
		select {
		case c.ch <- m:
		default:
		}
	}
	return c
}

func (h *hub) removeClient(c *sseClient) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.ch)
	}
	h.mu.Unlock()
}

// broadcast phân phát một tin nhắn và ghi vào bộ đệm cuộn khi cần.
func (h *hub) broadcast(msg sseMessage, buffer bool) {
	h.mu.Lock()
	if buffer {
		h.backlog = append(h.backlog, msg)
		if len(h.backlog) > h.maxLog {
			h.backlog = h.backlog[len(h.backlog)-h.maxLog:]
		}
	}
	for c := range h.clients {
		select {
		case c.ch <- msg:
		default: // client chậm: bỏ qua, tránh làm chậm phát sóng
		}
	}
	h.mu.Unlock()
}

// pushSnapshot đẩy ngay một snapshot mới nhất tới tất cả client (không vào backlog — kết nối mới đã có snapshot sẵn).
func (h *hub) pushSnapshot() {
	h.broadcast(sseMessage{Type: msgSnapshot, Data: h.eng.Snapshot()}, false)
	h.mu.Lock()
	h.dirty = false
	h.lastSnap = time.Now()
	h.mu.Unlock()
}

// run là vòng lặp chính của hub: tiêu thụ kênh engine và phát sóng; snapshot được đẩy theo ticker có giới hạn tần suất.
func (h *hub) run() {
	events := h.eng.Events()
	stream := h.eng.Stream()
	done := h.eng.Done()
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-h.stop:
			return
		case ev, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			h.broadcast(sseMessage{Type: msgEvent, Data: newEventDTO(ev)}, true)
			h.markDirty()
		case delta, ok := <-stream:
			if !ok {
				stream = nil
				continue
			}
			if delta == host.StreamClearSentinel {
				h.broadcast(sseMessage{Type: msgStream, Data: streamDTO{Clear: true}}, true)
				continue
			}
			if delta == "" {
				continue
			}
			h.broadcast(sseMessage{Type: msgStream, Data: streamDTO{Text: delta}}, true)
		case _, ok := <-done:
			if !ok {
				done = nil
				continue
			}
			h.broadcast(sseMessage{Type: msgDone}, true)
			h.pushSnapshot()
		case <-ticker.C:
			h.mu.RLock()
			dirty := h.dirty
			h.mu.RUnlock()
			if dirty {
				h.pushSnapshot()
			}
		}
	}
}

func (h *hub) markDirty() {
	h.mu.Lock()
	h.dirty = true
	h.mu.Unlock()
}

// emit cho phép các bridge khác (askuser / cocreate / import) đẩy tin nhắn tùy chỉnh qua hub.
func (h *hub) emit(msg sseMessage, buffer bool) {
	h.broadcast(msg, buffer)
}

func (h *hub) close() {
	close(h.stop)
}

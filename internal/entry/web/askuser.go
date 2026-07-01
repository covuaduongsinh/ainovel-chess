package web

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/voocel/ainovel-cli/internal/tools"
)

// askUserBridge kết nối callback AskUser chặn của engine với "SSE đẩy câu hỏi + HTTP trả lời".
// Tại một thời điểm chỉ có một yêu cầu chờ trả lời (coordinator hỏi tuần tự), dùng id để tránh nhầm lẫn.
type askUserBridge struct {
	hub *hub

	mu      sync.Mutex
	pending *pendingAsk
	seq     int
}

type pendingAsk struct {
	id       string
	resultCh chan *tools.AskUserResponse
}

func newAskUserBridge(h *hub) *askUserBridge {
	return &askUserBridge{hub: h}
}

// handler được tiêm vào eng.AskUser().SetHandler, chặn chờ trình duyệt trả lời.
func (b *askUserBridge) handler(ctx context.Context, questions []tools.Question) (*tools.AskUserResponse, error) {
	b.mu.Lock()
	b.seq++
	id := strconv.Itoa(b.seq) + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	p := &pendingAsk{id: id, resultCh: make(chan *tools.AskUserResponse, 1)}
	b.pending = p
	b.mu.Unlock()

	b.hub.emit(sseMessage{Type: msgAskUser, Data: askUserDTO{ID: id, Questions: questions}}, true)

	defer func() {
		b.mu.Lock()
		if b.pending == p {
			b.pending = nil
		}
		b.mu.Unlock()
	}()

	select {
	case resp := <-p.resultCh:
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// resolve được gọi bởi POST /api/answer, mở khóa yêu cầu chờ trả lời tương ứng theo id.
func (b *askUserBridge) resolve(id string, answers, notes map[string]string) error {
	b.mu.Lock()
	p := b.pending
	b.mu.Unlock()
	if p == nil || p.id != id {
		return fmt.Errorf("không có câu hỏi đang chờ (hoặc đã hết hạn)")
	}
	select {
	case p.resultCh <- &tools.AskUserResponse{Answers: answers, Notes: notes}:
		return nil
	default:
		return fmt.Errorf("câu hỏi đã được trả lời")
	}
}

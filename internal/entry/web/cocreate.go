package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/voocel/ainovel-cli/internal/entry/startup"
	"github.com/voocel/ainovel-cli/internal/host"
)

// stageOpener là câu mở đầu tổng hợp của cộng tác giai đoạn (tương đương stageCoCreateOpener của TUI).
const stageOpener = "Tôi muốn tạm dừng và cùng bạn lên kế hoạch cho hướng tiếp theo."

// coCreateBridge duy trì một phiên cộng tác đang hoạt động (khởi động nguội hoặc giai đoạn), đẩy phản hồi stream qua SSE tới frontend.
type coCreateBridge struct {
	s   *Server
	hub *hub

	mu      sync.Mutex
	session *startup.CoCreateSession
	stage   bool
	cancel  context.CancelFunc
	running bool
}

func newCoCreateBridge(s *Server, h *hub) *coCreateBridge {
	return &coCreateBridge{s: s, hub: h}
}

// coCreateDTO là tin nhắn SSE của cộng tác sáng tác. Phase quyết định cách frontend xử lý.
type coCreateDTO struct {
	Phase       string        `json:"phase"` // delta | round | error | closed
	Kind        string        `json:"kind"`  // khi delta: thinking / reply
	Text        string        `json:"text"`  // văn bản delta hoặc thông báo lỗi
	Stage       bool          `json:"stage"`
	Draft       string        `json:"draft"`
	Ready       bool          `json:"ready"`
	CanStart    bool          `json:"canStart"`
	Suggestions []string      `json:"suggestions"`
	History     []cocreateMsg `json:"history"`
}

type cocreateMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (b *coCreateBridge) emit(d coCreateDTO) {
	d.Stage = b.stage
	b.hub.emit(sseMessage{Type: msgCoCreate, Data: d}, false)
}

// open tạo phiên và chạy vòng đầu tiên. stage=true sẽ tạm dừng sáng tác hiện tại trước.
func (b *coCreateBridge) open(stage bool, initial string) error {
	b.mu.Lock()
	if b.session != nil {
		b.mu.Unlock()
		return fmt.Errorf("đang trong một phiên cộng tác khác")
	}
	if stage {
		if !b.s.eng.PauseForCoCreate() {
			b.mu.Unlock()
			return fmt.Errorf("không thể vào cộng tác giai đoạn: truyện đã hoàn thành hoặc đang cộng tác")
		}
		initial = stageOpener
	}
	b.stage = stage
	b.session = startup.NewCoCreateSession(strings.TrimSpace(initial))
	b.mu.Unlock()
	b.runRound()
	return nil
}

func (b *coCreateBridge) send(text string) error {
	b.mu.Lock()
	if b.session == nil {
		b.mu.Unlock()
		return fmt.Errorf("chưa có phiên cộng tác")
	}
	if b.running {
		b.mu.Unlock()
		return fmt.Errorf("AI đang trả lời, vui lòng đợi")
	}
	b.session.AppendUser(text)
	b.mu.Unlock()
	b.runRound()
	return nil
}

// runRound chạy một vòng cộng tác stream (bất đồng bộ); tiến trình được đẩy qua SSE.
func (b *coCreateBridge) runRound() {
	b.mu.Lock()
	if b.session == nil || b.running {
		b.mu.Unlock()
		return
	}
	b.running = true
	stage := b.stage
	history := b.session.History()
	ctx, cancel := context.WithCancel(context.Background())
	b.cancel = cancel
	b.mu.Unlock()

	go func() {
		onProgress := func(kind, text string) {
			b.emit(coCreateDTO{Phase: "delta", Kind: kind, Text: text})
		}
		var reply host.CoCreateReply
		var err error
		if stage {
			reply, err = b.s.eng.StageCoCreateStream(ctx, history, onProgress)
		} else {
			reply, err = b.s.eng.CoCreateStream(ctx, history, onProgress)
		}
		b.mu.Lock()
		b.running = false
		sess := b.session
		b.mu.Unlock()
		if err != nil {
			b.emit(coCreateDTO{Phase: "error", Text: err.Error()})
			return
		}
		if sess == nil {
			return
		}
		sess.ApplyReply(reply)
		b.emitRound()
	}()
}

func (b *coCreateBridge) emitRound() {
	b.mu.Lock()
	sess := b.session
	b.mu.Unlock()
	if sess == nil {
		return
	}
	b.emit(coCreateDTO{
		Phase:       "round",
		Draft:       sess.DraftPrompt(),
		Ready:       sess.Ready(),
		CanStart:    sess.CanStart(),
		Suggestions: sess.Suggestions(),
		History:     displayHistory(sess),
	})
}

// start áp dụng kết quả cộng tác: khởi động nguội → mở sách mới; giai đoạn → tiêm hướng viết tiếp.
func (b *coCreateBridge) start() error {
	b.mu.Lock()
	sess := b.session
	stage := b.stage
	b.mu.Unlock()
	if sess == nil {
		return fmt.Errorf("chưa có phiên cộng tác")
	}
	if !sess.CanStart() {
		return fmt.Errorf("AI chưa tổng hợp đủ chỉ thị sáng tác")
	}
	if stage {
		if err := b.s.eng.ResumeFromCoCreate(sess.DraftPrompt()); err != nil {
			return err
		}
	} else {
		plan, err := sess.BuildPlan()
		if err != nil {
			return err
		}
		if err := b.s.eng.PrepareUserRules(plan.RawPrompt); err != nil {
			return err
		}
		if err := b.s.eng.StartPrepared(plan.StartPrompt); err != nil {
			return err
		}
	}
	b.clear()
	return nil
}

func (b *coCreateBridge) cancelSession() {
	b.mu.Lock()
	cancel := b.cancel
	stage := b.stage
	had := b.session != nil
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if stage && had {
		b.s.eng.CancelCoCreate()
	}
	b.clear()
}

func (b *coCreateBridge) clear() {
	b.mu.Lock()
	b.session = nil
	b.cancel = nil
	b.running = false
	b.stage = false
	b.mu.Unlock()
	b.emit(coCreateDTO{Phase: "closed"})
}

// displayHistory chiếu lịch sử phiên thành hội thoại hiển thị cho frontend (assistant chỉ giữ đoạn <reply>).
func displayHistory(sess *startup.CoCreateSession) []cocreateMsg {
	var out []cocreateMsg
	for _, m := range sess.History() {
		content := m.Content
		if m.Role == "assistant" {
			content = extractReply(content)
		}
		out = append(out, cocreateMsg{Role: m.Role, Content: strings.TrimSpace(content)})
	}
	return out
}

// extractReply cắt lấy <reply>...</reply> (tương đương extractReplyForDisplay của TUI).
func extractReply(content string) string {
	rest := content
	if i := strings.Index(content, "<reply>"); i >= 0 {
		rest = content[i+len("<reply>"):]
	}
	if i := strings.Index(rest, "</reply>"); i >= 0 {
		return strings.TrimSpace(rest[:i])
	}
	cut := len(rest)
	for _, mark := range []string{"<draft>", "<ready>", "<suggestions>"} {
		if i := strings.Index(rest, mark); i >= 0 && i < cut {
			cut = i
		}
	}
	if cut == len(rest) && !strings.Contains(content, "<") {
		return content
	}
	return strings.TrimSpace(rest[:cut])
}

// ── Xử lý HTTP ──

func (s *Server) handleCoCreateOpen(w http.ResponseWriter, r *http.Request) {
	var req cocreateOpenRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if !req.Stage && strings.TrimSpace(req.Initial) == "" {
		writeErr(w, http.StatusBadRequest, errMsg("vui lòng nhập ý tưởng ban đầu"))
		return
	}
	if err := s.cocreate.open(req.Stage, req.Initial); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeOK(w, nil)
}

func (s *Server) handleCoCreateSend(w http.ResponseWriter, r *http.Request) {
	var req textRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeErr(w, http.StatusBadRequest, errMsg("nội dung trống"))
		return
	}
	if err := s.cocreate.send(req.Text); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeOK(w, nil)
}

func (s *Server) handleCoCreateStart(w http.ResponseWriter, _ *http.Request) {
	if err := s.cocreate.start(); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeOK(w, nil)
}

func (s *Server) handleCoCreateCancel(w http.ResponseWriter, _ *http.Request) {
	s.cocreate.cancelSession()
	writeOK(w, nil)
}

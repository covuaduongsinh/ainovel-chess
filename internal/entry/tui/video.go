package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/host/adapt"
)

// videoState là trạng thái modal trong quá trình chạy lệnh /video (chuyển thể sản phẩm video).
// Mirror importState: tạo khi bắt đầu, theo dõi luồng sự kiện, Esc để hủy/đóng.
type videoState struct {
	reqID      int
	label      string
	stage      adapt.Stage
	current    int
	total      int
	startedAt  time.Time
	finishedAt time.Time
	history    []videoLine
	err        error
	done       bool
	cancel     context.CancelFunc
	viewport   viewport.Model
}

type videoLine struct {
	at      time.Time
	stage   adapt.Stage
	current int
	total   int
	message string
	err     error
}

func newVideoState(reqID int, label string, width, height int, cancel context.CancelFunc) *videoState {
	boxW, boxH := reportModalSize(width, height)
	contentW := paddedModalContentWidth(boxW)
	vp := viewport.New(contentW, boxH-4)
	s := &videoState{
		reqID:     reqID,
		label:     label,
		startedAt: time.Now(),
		stage:     adapt.StageContext,
		cancel:    cancel,
		viewport:  vp,
	}
	s.refresh(contentW)
	return s
}

func (s *videoState) appendEvent(ev adapt.Event, contentW int) {
	s.stage = ev.Stage
	s.current = ev.Current
	s.total = ev.Total
	if ev.Err != nil {
		s.err = ev.Err
	}
	s.history = append(s.history, videoLine{
		at: ev.Time, stage: ev.Stage, current: ev.Current, total: ev.Total,
		message: ev.Message, err: ev.Err,
	})
	if ev.Stage == adapt.StageDone || ev.Stage == adapt.StageError {
		s.done = true
		s.finishedAt = ev.Time
	}
	s.refresh(contentW)
}

func (s *videoState) refresh(contentW int) {
	titleStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	mutedStyle := lipgloss.NewStyle().Foreground(colorMuted)
	okStyle := lipgloss.NewStyle().Foreground(colorSuccess)
	errStyle := lipgloss.NewStyle().Foreground(colorError)
	stageStyle := lipgloss.NewStyle().Foreground(colorAccent2)

	var b strings.Builder
	b.WriteString(titleStyle.Render("Chuyển thành sản phẩm video"))
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("Sản phẩm "))
	b.WriteString(s.label)
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Bắt đầu "))
	b.WriteString(formatReportTime(s.startedAt))
	if !s.finishedAt.IsZero() {
		b.WriteString(dimStyle.Render("  Hoàn thành "))
		b.WriteString(formatReportTime(s.finishedAt))
	}
	b.WriteString("\n\n")

	b.WriteString(mutedStyle.Render("Giai đoạn "))
	b.WriteString(stageStyle.Render(string(s.stage)))
	if s.total > 0 {
		b.WriteString(mutedStyle.Render("  Tiến độ "))
		if s.current > 0 {
			b.WriteString(fmt.Sprintf("%d/%d", s.current, s.total))
		} else {
			b.WriteString(fmt.Sprintf("0/%d", s.total))
		}
	}
	b.WriteString("\n\n")

	b.WriteString(titleStyle.Render("Nhật ký quy trình"))
	b.WriteString(" ")
	b.WriteString(dimStyle.Render(fmt.Sprintf("(%d mục)", len(s.history))))
	b.WriteString("\n")
	for _, ln := range s.history {
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(ln.at.Format("15:04:05")))
		b.WriteString(" ")
		b.WriteString(stageStyle.Render(string(ln.stage)))
		if ln.total > 0 && ln.current > 0 {
			b.WriteString(mutedStyle.Render(fmt.Sprintf(" %d/%d", ln.current, ln.total)))
		}
		b.WriteString(" ")
		if ln.err != nil {
			b.WriteString(errStyle.Render(ln.message + " — " + ln.err.Error()))
		} else {
			b.WriteString(wrapText(ln.message, contentW))
		}
	}

	b.WriteString("\n\n")
	switch {
	case !s.done:
		b.WriteString(dimStyle.Render("Esc hủy chuyển thể"))
	case s.err != nil:
		b.WriteString(errStyle.Render("Chuyển thể thất bại"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("Esc đóng panel"))
	default:
		b.WriteString(okStyle.Render("Chuyển thể hoàn thành"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("Esc đóng panel"))
	}

	s.viewport.SetContent(b.String())
	if !s.done {
		s.viewport.GotoBottom()
	}
}

func renderVideoModal(width, height int, s *videoState) string {
	if s == nil {
		return ""
	}
	boxW, boxH := reportModalSize(width, height)
	contentW := paddedModalContentWidth(boxW)
	if s.viewport.Width != contentW {
		s.viewport.Width = contentW
		s.refresh(contentW)
	}
	if s.viewport.Height != boxH-4 {
		s.viewport.Height = boxH - 4
	}
	hint := "  ↑↓ cuộn · Esc hủy/đóng"
	modal := renderPaddedModalFrame(boxW, boxH, "Chuyển thành sản phẩm video", hint,
		strings.Split(s.viewport.View(), "\n"))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

func (m Model) handleVideoKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.videoer == nil {
		return m, nil
	}
	switch msg.Type {
	case tea.KeyEsc:
		if !m.videoer.done && m.videoer.cancel != nil {
			m.videoer.cancel()
			return m, nil
		}
		m.videoer = nil
		return m, m.textarea.Focus()
	case tea.KeyUp:
		m.videoer.viewport.ScrollUp(1)
	case tea.KeyDown:
		m.videoer.viewport.ScrollDown(1)
	case tea.KeyPgUp:
		m.videoer.viewport.HalfPageUp()
	case tea.KeyPgDown:
		m.videoer.viewport.HalfPageDown()
	}
	return m, nil
}

// videoEventMsg gửi một lần adapt.Event.
type videoEventMsg struct {
	reqID int
	ev    adapt.Event
	ch    <-chan adapt.Event
}

// startVideo khởi động một lần chuyển thể: phân tích tham số → tạo modal state → lắng nghe luồng sự kiện.
func startVideo(rt *host.Host, reqID int, args []string, width, height int) (*videoState, tea.Cmd, error) {
	opts, label, err := parseVideoArgs(args)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := rt.Adapt(ctx, opts)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	state := newVideoState(reqID, label, width, height, cancel)
	return state, listenVideoEvent(reqID, ch), nil
}

func listenVideoEvent(reqID int, ch <-chan adapt.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return videoEventMsg{reqID: reqID, ev: ev, ch: ch}
	}
}

// parseVideoArgs phân tích `/video [product...] [from=N] [to=M] [style=...] [--overwrite]`.
func parseVideoArgs(args []string) (adapt.Options, string, error) {
	var opts adapt.Options
	var products []adapt.Product
	for _, a := range args {
		switch {
		case a == "--overwrite":
			opts.Overwrite = true
		case strings.Contains(a, "="):
			k, v, _ := strings.Cut(a, "=")
			switch strings.ToLower(k) {
			case "from":
				n, err := strconv.Atoi(v)
				if err != nil || n < 0 {
					return adapt.Options{}, "", fmt.Errorf("from phải là số nguyên không âm: %q", v)
				}
				opts.From = n
			case "to":
				n, err := strconv.Atoi(v)
				if err != nil || n < 0 {
					return adapt.Options{}, "", fmt.Errorf("to phải là số nguyên không âm: %q", v)
				}
				opts.To = n
			case "style":
				opts.StyleHint = v
			default:
				return adapt.Options{}, "", fmt.Errorf("tham số không rõ %q (hỗ trợ: from, to, style)", k)
			}
		case a == "all":
			// tất cả — để products rỗng
		default:
			products = append(products, adapt.Product(a))
		}
	}
	opts.Products = products
	label := "tất cả"
	if len(products) > 0 {
		parts := make([]string, len(products))
		for i, p := range products {
			parts[i] = string(p)
		}
		label = strings.Join(parts, ", ")
	}
	return opts, label, nil
}

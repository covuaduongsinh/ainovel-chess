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
	"github.com/voocel/ainovel-cli/internal/host/comic"
)

// comicState là trạng thái modal trong quá trình chạy lệnh /truyentranh.
// Mirror videoState: tạo khi bắt đầu, theo dõi luồng sự kiện, Esc để hủy/đóng.
type comicState struct {
	reqID      int
	label      string
	stage      comic.Stage
	current    int
	total      int
	startedAt  time.Time
	finishedAt time.Time
	history    []comicLine
	err        error
	done       bool
	cancel     context.CancelFunc
	viewport   viewport.Model
}

type comicLine struct {
	at      time.Time
	stage   comic.Stage
	current int
	total   int
	message string
	err     error
}

func newComicState(reqID int, label string, width, height int, cancel context.CancelFunc) *comicState {
	boxW, boxH := reportModalSize(width, height)
	contentW := paddedModalContentWidth(boxW)
	vp := viewport.New(contentW, boxH-4)
	s := &comicState{
		reqID:     reqID,
		label:     label,
		startedAt: time.Now(),
		stage:     comic.StageContext,
		cancel:    cancel,
		viewport:  vp,
	}
	s.refresh(contentW)
	return s
}

func (s *comicState) appendEvent(ev comic.Event, contentW int) {
	s.stage = ev.Stage
	s.current = ev.Current
	s.total = ev.Total
	if ev.Err != nil {
		s.err = ev.Err
	}
	s.history = append(s.history, comicLine{
		at: ev.Time, stage: ev.Stage, current: ev.Current, total: ev.Total,
		message: ev.Message, err: ev.Err,
	})
	if ev.Stage == comic.StageDone || ev.Stage == comic.StageError {
		s.done = true
		s.finishedAt = ev.Time
	}
	s.refresh(contentW)
}

func (s *comicState) refresh(contentW int) {
	titleStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	mutedStyle := lipgloss.NewStyle().Foreground(colorMuted)
	okStyle := lipgloss.NewStyle().Foreground(colorSuccess)
	errStyle := lipgloss.NewStyle().Foreground(colorError)
	stageStyle := lipgloss.NewStyle().Foreground(colorAccent2)

	var b strings.Builder
	b.WriteString(titleStyle.Render("Làm truyện tranh"))
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
		b.WriteString(dimStyle.Render("Esc hủy"))
	case s.err != nil:
		b.WriteString(errStyle.Render("Làm truyện tranh thất bại"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("Esc đóng panel"))
	default:
		b.WriteString(okStyle.Render("Làm truyện tranh hoàn thành"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("Esc đóng panel"))
	}

	s.viewport.SetContent(b.String())
	if !s.done {
		s.viewport.GotoBottom()
	}
}

func renderComicModal(width, height int, s *comicState) string {
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
	modal := renderPaddedModalFrame(boxW, boxH, "Làm truyện tranh", hint,
		strings.Split(s.viewport.View(), "\n"))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

func (m Model) handleComicKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.comicer == nil {
		return m, nil
	}
	switch msg.Type {
	case tea.KeyEsc:
		if !m.comicer.done && m.comicer.cancel != nil {
			m.comicer.cancel()
			return m, nil
		}
		m.comicer = nil
		return m, m.textarea.Focus()
	case tea.KeyUp:
		m.comicer.viewport.ScrollUp(1)
	case tea.KeyDown:
		m.comicer.viewport.ScrollDown(1)
	case tea.KeyPgUp:
		m.comicer.viewport.HalfPageUp()
	case tea.KeyPgDown:
		m.comicer.viewport.HalfPageDown()
	}
	return m, nil
}

// comicEventMsg gửi một lần comic.Event.
type comicEventMsg struct {
	reqID int
	ev    comic.Event
	ch    <-chan comic.Event
}

// startComic khởi động một lần làm truyện tranh.
func startComic(rt *host.Host, reqID int, args []string, width, height int) (*comicState, tea.Cmd, error) {
	opts, label, err := parseComicArgs(args)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := rt.Comic(ctx, opts)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	state := newComicState(reqID, label, width, height, cancel)
	return state, listenComicEvent(reqID, ch), nil
}

func listenComicEvent(reqID int, ch <-chan comic.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return comicEventMsg{reqID: reqID, ev: ev, ch: ch}
	}
}

// parseComicArgs phân tích:
// `/truyentranh [product...] [preset=...] [style=...] [from=N] [to=M] [size=a4|b5]
//
//	[format=pdf,cbz,epub] [maximages=N] [imagesize=1K|2K] [--overwrite]`
func parseComicArgs(args []string) (comic.Options, string, error) {
	var opts comic.Options
	var products []comic.Product
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
					return comic.Options{}, "", fmt.Errorf("from phải là số nguyên không âm: %q", v)
				}
				opts.From = n
			case "to":
				n, err := strconv.Atoi(v)
				if err != nil || n < 0 {
					return comic.Options{}, "", fmt.Errorf("to phải là số nguyên không âm: %q", v)
				}
				opts.To = n
			case "preset":
				opts.StylePreset = v
			case "style":
				opts.StyleHint = v
			case "size":
				switch strings.ToLower(v) {
				case "a4", "b5":
					opts.PageSize = strings.ToLower(v)
				default:
					return comic.Options{}, "", fmt.Errorf("size chỉ nhận a4|b5: %q", v)
				}
			case "format", "formats":
				for _, f := range strings.Split(v, ",") {
					if f = strings.TrimSpace(f); f != "" {
						opts.Formats = append(opts.Formats, comic.Format(strings.ToLower(f)))
					}
				}
			case "maximages":
				n, err := strconv.Atoi(v)
				if err != nil || n < 0 {
					return comic.Options{}, "", fmt.Errorf("maximages phải là số nguyên không âm: %q", v)
				}
				opts.MaxImages = n
			case "imagesize":
				opts.ImageSize = strings.ToUpper(v)
			case "out":
				opts.OutDir = v
			default:
				return comic.Options{}, "", fmt.Errorf(
					"tham số không rõ %q (hỗ trợ: preset, style, from, to, size, format, maximages, imagesize, out)", k)
			}
		case a == "all":
			// tất cả — để products rỗng
		default:
			products = append(products, comic.Product(a))
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

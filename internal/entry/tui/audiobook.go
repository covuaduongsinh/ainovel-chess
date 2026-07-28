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
	"github.com/voocel/ainovel-cli/internal/host/tts"
)

// audiobookState là trạng thái modal trong quá trình chạy lệnh /sachnoi.
// Mirror videoState: tạo khi bắt đầu, theo dõi luồng sự kiện, Esc để hủy/đóng.
type audiobookState struct {
	reqID      int
	label      string
	stage      tts.Stage
	current    int
	total      int
	startedAt  time.Time
	finishedAt time.Time
	history    []audiobookLine
	err        error
	done       bool
	cancel     context.CancelFunc
	viewport   viewport.Model
}

type audiobookLine struct {
	at      time.Time
	stage   tts.Stage
	current int
	total   int
	message string
	err     error
}

func newAudiobookState(reqID int, label string, width, height int, cancel context.CancelFunc) *audiobookState {
	boxW, boxH := reportModalSize(width, height)
	contentW := paddedModalContentWidth(boxW)
	vp := viewport.New(contentW, boxH-4)
	s := &audiobookState{
		reqID:     reqID,
		label:     label,
		startedAt: time.Now(),
		stage:     tts.StageContext,
		cancel:    cancel,
		viewport:  vp,
	}
	s.refresh(contentW)
	return s
}

func (s *audiobookState) appendEvent(ev tts.Event, contentW int) {
	s.stage = ev.Stage
	s.current = ev.Current
	s.total = ev.Total
	if ev.Err != nil {
		s.err = ev.Err
	}
	s.history = append(s.history, audiobookLine{
		at: ev.Time, stage: ev.Stage, current: ev.Current, total: ev.Total,
		message: ev.Message, err: ev.Err,
	})
	if ev.Stage == tts.StageDone || ev.Stage == tts.StageError {
		s.done = true
		s.finishedAt = ev.Time
	}
	s.refresh(contentW)
}

func (s *audiobookState) refresh(contentW int) {
	titleStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	mutedStyle := lipgloss.NewStyle().Foreground(colorMuted)
	okStyle := lipgloss.NewStyle().Foreground(colorSuccess)
	errStyle := lipgloss.NewStyle().Foreground(colorError)
	stageStyle := lipgloss.NewStyle().Foreground(colorAccent2)

	var b strings.Builder
	b.WriteString(titleStyle.Render("Tạo sách nói"))
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("Giọng đọc "))
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
		b.WriteString(dimStyle.Render("Esc dừng tạo sách nói"))
	case s.err != nil:
		b.WriteString(errStyle.Render("Tạo sách nói thất bại"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("Esc đóng panel"))
	default:
		b.WriteString(okStyle.Render("Tạo sách nói hoàn thành"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("Esc đóng panel"))
	}

	s.viewport.SetContent(b.String())
	if !s.done {
		s.viewport.GotoBottom()
	}
}

func renderAudiobookModal(width, height int, s *audiobookState) string {
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
	hint := "  ↑↓ cuộn · Esc dừng/đóng"
	modal := renderPaddedModalFrame(boxW, boxH, "Tạo sách nói", hint,
		strings.Split(s.viewport.View(), "\n"))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

func (m Model) handleAudiobookKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.audiobooker == nil {
		return m, nil
	}
	switch msg.Type {
	case tea.KeyEsc:
		if !m.audiobooker.done && m.audiobooker.cancel != nil {
			m.audiobooker.cancel()
			return m, nil
		}
		m.audiobooker = nil
		return m, m.textarea.Focus()
	case tea.KeyUp:
		m.audiobooker.viewport.ScrollUp(1)
	case tea.KeyDown:
		m.audiobooker.viewport.ScrollDown(1)
	case tea.KeyPgUp:
		m.audiobooker.viewport.HalfPageUp()
	case tea.KeyPgDown:
		m.audiobooker.viewport.HalfPageDown()
	}
	return m, nil
}

// audiobookEventMsg gửi một lần tts.Event.
type audiobookEventMsg struct {
	reqID int
	ev    tts.Event
	ch    <-chan tts.Event
}

// startAudiobook khởi động một lần tạo sách nói: phân tích tham số → tạo modal state → lắng nghe sự kiện.
func startAudiobook(rt *host.Host, reqID int, args []string, width, height int) (*audiobookState, tea.Cmd, error) {
	opts, label, err := parseAudiobookArgs(args)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := rt.Audiobook(ctx, opts)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	state := newAudiobookState(reqID, label, width, height, cancel)
	return state, listenAudiobookEvent(reqID, ch), nil
}

func listenAudiobookEvent(reqID int, ch <-chan tts.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return audiobookEventMsg{reqID: reqID, ev: ev, ch: ch}
	}
}

// parseAudiobookArgs phân tích
// `/sachnoi [voice=CODE] [from=N] [to=M] [speed=1.0] [bitrate=128] [format=mp3|wav] [samplerate=24000] [out=PATH] [--overwrite]`.
func parseAudiobookArgs(args []string) (tts.Options, string, error) {
	var opts tts.Options
	for _, a := range args {
		if a == "--overwrite" {
			opts.Overwrite = true
			continue
		}
		if !strings.Contains(a, "=") {
			return tts.Options{}, "", fmt.Errorf("tham số không rõ %q (dùng dạng khóa=giá trị, vd voice=hn_female_ngochuyen_full_48k-fhg)", a)
		}
		k, v, _ := strings.Cut(a, "=")
		switch strings.ToLower(k) {
		case "voice":
			opts.VoiceCode = v
		case "from":
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return tts.Options{}, "", fmt.Errorf("from phải là số nguyên không âm: %q", v)
			}
			opts.From = n
		case "to":
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return tts.Options{}, "", fmt.Errorf("to phải là số nguyên không âm: %q", v)
			}
			opts.To = n
		case "speed":
			f, err := strconv.ParseFloat(v, 64)
			if err != nil || f < 0.25 || f > 1.9 {
				return tts.Options{}, "", fmt.Errorf("speed phải nằm trong khoảng 0.25–1.9: %q", v)
			}
			opts.Speed = f
		case "bitrate":
			n, err := strconv.Atoi(v)
			if err != nil {
				return tts.Options{}, "", fmt.Errorf("bitrate phải là số: %q", v)
			}
			opts.Bitrate = n
		case "samplerate":
			n, err := strconv.Atoi(v)
			if err != nil {
				return tts.Options{}, "", fmt.Errorf("samplerate phải là số: %q", v)
			}
			opts.SampleRate = n
		case "format":
			switch strings.ToLower(v) {
			case "mp3", "wav":
				opts.OutputFormat = strings.ToLower(v)
			default:
				return tts.Options{}, "", fmt.Errorf("format chỉ nhận mp3|wav: %q", v)
			}
		case "out":
			opts.OutDir = v
		default:
			return tts.Options{}, "", fmt.Errorf("tham số không rõ %q (hỗ trợ: voice, from, to, speed, bitrate, format, samplerate, out)", k)
		}
	}
	label := opts.VoiceCode
	if label == "" {
		label = "giọng mặc định"
	}
	return opts, label, nil
}

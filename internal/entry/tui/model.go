package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/tools"
	"github.com/voocel/ainovel-cli/internal/utils"
)

const maxEvents = 500

// maxStreamRounds giới hạn số lượt giữ lại trong panel stream. Mỗi LLM call kết thúc kích hoạt một streamClear mở lượt mới,
// writer một chương khoảng 3~5 lượt (agent header / thinking / draft / commit), 32 lượt khoảng bằng
// xem lại output stream của 6~10 chương gần nhất. Nội dung chương đã commit được ghi vào store/drafts,
// vượt quá thì bỏ để tránh mỗi token delta kích hoạt tái render O(toàn văn). Giới hạn bộ nhớ ổn định khoảng 512KB, thấp hơn nhiều so với ngưỡng giật lag.
const maxStreamRounds = 32

type focusPane int

const (
	focusEvents focusPane = iota
	focusStream
	focusDetail
	focusState // thanh bên trạng thái bên trái (có thể cuộn)

	focusPaneCount // tổng số tiêu điểm, dùng cho Tab vòng quanh
)

type appMode int

const (
	modeNew     appMode = iota // đang chờ người dùng nhập yêu cầu tiểu thuyết
	modeRunning                // đang sáng tác (bao gồm dừng do lỗi, nhập để khôi phục)
	modeDone                   // sáng tác hoàn thành
)

// spinnerFrames là chuỗi frame spinner dùng chung cho thanh trên / hoạt động stream (bubbles.Spinner.MiniDot).
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// toolSpinnerFrames là chuỗi frame spinner chuyên dụng cho dòng "đang tiến hành" trong luồng sự kiện (bubbles.Spinner.Dot).
// 7 điểm + 1 khoảng trống xoay theo chiều kim đồng hồ trên lưới 3×3, nhìn như vòng tải hoàn chỉnh.
// Dùng chỉ số frame độc lập + tick nhanh hơn, không ảnh hưởng nhịp điệu thanh trên và animation sao.
var toolSpinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// Model là trạng thái cấp cao nhất của TUI.
type Model struct {
	runtime        *host.Host
	askBridge      *askUserBridge
	askState       *askUserState
	cocreate       *cocreateState
	help           *helpState
	modelSwitch    *modelSwitchState
	report         *reportState
	version        string
	importer       *importState
	importSeq      int
	simulator      *simulationState
	simSeq         int
	compItems      []commandPaletteItem
	compIdx        int
	compActive     bool
	snapshot       host.UISnapshot
	events         []host.Event
	eventIndex     map[string]int   // event.ID → chỉ số m.events; sự kiện dạng gọi đến thì cập nhật tại chỗ
	viewport       viewport.Model   // viewport luồng sự kiện
	streamVP       viewport.Model   // viewport đầu ra stream
	detailVP       viewport.Model   // viewport chi tiết bên phải
	stateVP        viewport.Model   // viewport thanh bên trạng thái bên trái (có thể cuộn)
	streamBuf      *strings.Builder // bộ đệm tích lũy văn bản stream
	streamRounds   []string
	textarea       textarea.Model
	width          int
	height         int
	autoScroll     bool
	streamScroll   bool      // panel stream tự động theo dõi
	streamDirty    bool      // streamRounds có delta chưa được làm mới; được streamFlushTick hợp nhất 60fps
	lastKeyAt      time.Time // thời gian bấm phím không phải Enter lần cuối; throttle KeyEnter tránh \n trong paste kích hoạt submit nhầm
	inputHistory   []string  // lịch sử nhập đã gửi (loại trùng: không trùng liền kề)
	historyIdx     int       // chỉ số duyệt hiện tại; == len(inputHistory) nghĩa là "chưa duyệt, đang chỉnh sửa nháp"
	historyDraft   string    // nháp lưu trước khi vào duyệt lịch sử, khôi phục khi về cuối
	focusPane      focusPane
	hoverPane      focusPane
	hoverActive    bool
	mode           appMode
	starting       bool // UI đã vào bàn làm việc, Host đang thực hiện khởi tạo startup
	startupMode    startupMode
	cocreateSeq    int
	reportSeq      int
	err            error
	spinnerIdx     int
	toolSpinnerIdx int  // chỉ số frame độc lập của dòng đang tiến hành trong luồng sự kiện (150ms tick, không ảnh hưởng thanh trên/sao)
	cursorIdx      int  // chỉ số frame con trỏ stream (tick độc lập)
	streamRound    int  // đếm lượt đầu ra stream
	quitPending    bool // xác nhận thoát bằng Ctrl+C hai lần
	abortPending   bool // tạm dừng thủ công đang chờ Done về
	mouseOff       bool // true = đã tắt báo cáo chuột, cho phép người dùng kéo chọn sao chép tự nhiên; bật lại khi chuyển tiếp
}

// NewModel tạo TUI Model.
func NewModel(rt *host.Host, bridge *askUserBridge, version string) Model {
	ta := textarea.New()
	ta.Placeholder = placeholderForNewMode(startupModeQuick)
	ta.CharLimit = 5000
	ta.SetHeight(1)
	// MaxHeight=6 cho phép nhập quá dài tự động wrap theo chiều rộng hiển thị nhiều dòng (giới hạn hiển thị 6 dòng).
	ta.MaxHeight = 6
	ta.ShowLineNumbers = false
	ta.Focus()

	// Mặc định Enter không xuống dòng (do handleEnterKey gửi);
	// xuống dòng chủ động được ràng buộc lại với ctrl+j (unix \n) và alt+enter (thói quen GUI).
	// Lớp giao thức terminal không phân biệt được Shift+Enter với Enter nên không hỗ trợ Shift+Enter.
	ta.KeyMap.InsertNewline.SetKeys("ctrl+j", "alt+enter")

	vp := viewport.New(80, 20)
	vp.SetContent("")

	svp := viewport.New(80, 10)
	svp.SetContent("")

	dvp := viewport.New(40, 20)
	dvp.SetContent("")

	stvp := viewport.New(32, 20)
	stvp.SetContent("")

	return Model{
		runtime:      rt,
		askBridge:    bridge,
		version:      strings.TrimSpace(version),
		autoScroll:   true,
		streamScroll: true,
		mode:         modeNew,
		startupMode:  startupModeQuick,
		textarea:     ta,
		viewport:     vp,
		streamVP:     svp,
		detailVP:     dvp,
		stateVP:      stvp,
		streamBuf:    &strings.Builder{},
		eventIndex:   make(map[string]int),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		listenEvents(m.runtime),
		listenAskUser(m.askBridge),
		listenDone(m.runtime),
		listenStream(m.runtime),
		tickSnapshot(m.runtime),
		bootstrapRuntime(m.runtime),
		tickSpinner(),
		tickToolSpinner(),
		tickCursor(),
		tickStreamFlush(),
	)
}

func (m *Model) paneAtMouse(x, y int) (focusPane, bool) {
	if m.width == 0 || m.height == 0 {
		return focusEvents, false
	}

	topH, _, bodyH := m.layoutHeights()
	if bodyH < 1 {
		return focusEvents, false
	}

	bodyStartY := topH
	bodyEndY := topH + bodyH
	if y < bodyStartY || y >= bodyEndY {
		return focusEvents, false
	}

	leftW := m.sidebarWidth()
	rightW := m.detailWidth()
	centerStartX := leftW
	rightStartX := m.width - rightW

	if x >= rightStartX {
		return focusDetail, true
	}
	if x < centerStartX {
		return focusState, true
	}

	eventH, _ := m.splitHeights(bodyH)
	if y-bodyStartY < eventH {
		return focusEvents, true
	}
	return focusStream, true
}

func (m *Model) paneHighlighted(pane focusPane) bool {
	if m.focusPane == pane {
		return true
	}
	return m.hoverActive && m.hoverPane == pane
}

// hasRunningEvent kiểm tra có tồn tại sự kiện dạng gọi chưa hoàn thành (spinner vẫn đang quay) không.
// toolSpinnerTick dùng điều này để quyết định có đáng tái render không: khi không có sự kiện running, frame spinner không ảnh hưởng đầu ra,
// toàn bộ refreshEventViewport là công việc vô ích xác định.
func (m *Model) hasRunningEvent() bool {
	for i := range m.events {
		if m.events[i].Running() {
			return true
		}
	}
	return false
}

// flushStreamIfDirty render streamRounds tích lũy vào viewport; đánh dấu là đã làm mới.
// Trả về có thực sự làm mới không, để bên gọi quyết định có nên GotoBottom không.
func (m *Model) flushStreamIfDirty() bool {
	if !m.streamDirty {
		return false
	}
	m.refreshStreamViewport()
	m.streamDirty = false
	return true
}

// refreshEventViewport tái render nội dung luồng sự kiện và cập nhật viewport.
func (m *Model) refreshEventViewport() {
	centerW := m.eventFlowWidth()
	content := renderEventContent(m.events, centerW, m.toolSpinnerIdx)
	snap := m.snapshot
	if m.starting {
		snap.IsRunning = true
	}
	if activity := renderEventActivity(snap, m.spinnerIdx, centerW); activity != "" {
		if strings.TrimSpace(content) != "" {
			content += "\n" + activity
		} else {
			content = activity
		}
	}
	m.viewport.SetContent(content)
	if m.autoScroll {
		m.viewport.GotoBottom()
	}
}

func (m *Model) refreshStreamViewport() {
	cursor := ""
	if m.snapshot.IsRunning {
		cursor = renderStreamCursor(m.cursorIdx)
	}
	m.streamVP.SetContent(renderStreamContent(m.streamRounds, m.streamVP.Width, cursor))
}

func (m *Model) refreshDetailViewport() {
	rightW := m.detailWidth()
	if rightW <= 4 {
		return
	}
	m.detailVP.SetContent(renderDetailContent(m.snapshot, rightW-4))
}

// refreshStateViewport đẩy nội dung thanh bên trạng thái bên trái vào viewport.
// Nội dung thanh bên hoàn toàn được suy ra từ snapshot, nên cần làm mới khi snapshot hoặc kích thước thay đổi.
func (m *Model) refreshStateViewport() {
	leftW := m.sidebarWidth()
	if leftW <= 4 {
		return
	}
	m.stateVP.SetContent(renderStateContent(m.snapshot, leftW-4))
}

// updateViewportSize cập nhật kích thước viewport theo kích thước cửa sổ hiện tại.
func (m *Model) updateViewportSize() {
	centerW := m.eventFlowWidth()
	rightW := m.detailWidth()
	bodyH := m.bodyHeight()
	eventH, streamH := m.splitHeights(bodyH)
	m.viewport.Width = centerW - 2
	m.viewport.Height = eventH - 1 // -1 cho dòng header của event panel
	m.streamVP.Width = centerW - 2
	m.streamVP.Height = streamH - 1 // -1 cho dòng header của stream panel
	m.detailVP.Width = rightW - 2
	m.detailVP.Height = bodyH
	leftW := m.sidebarWidth()
	m.stateVP.Width = max(1, leftW-2)
	m.stateVP.Height = max(1, bodyH-2) // -2 cho khoảng trống trên dưới của Padding(1,1) thanh trạng thái
}

// splitHeights tính phân bổ chiều cao cho luồng sự kiện và đầu ra stream.
func (m *Model) splitHeights(bodyH int) (eventH, streamH int) {
	eventH = bodyH * 40 / 100
	if eventH < 3 {
		eventH = 3
	}
	streamH = bodyH - eventH - 1 // -1 为分隔线
	if streamH < 3 {
		streamH = 3
	}
	return
}

func (m *Model) inputWidth() int {
	if m.width == 0 {
		return 60
	}
	return m.width - 6 // border + padding + ký hiệu nhắc "❯ "
}

func (m *Model) currentInputWidth() int {
	if m.cocreate != nil {
		return coCreateInputWidth(m.width, m.height)
	}
	return m.inputWidth()
}

// refitTextareaHeight ước tính số dòng hiển thị theo nội dung hiện tại và SetHeight động.
// Dòng hiển thị = tổng số dòng sau khi mỗi đoạn (phân tách bởi \n) được wrap theo chiều rộng.
// Kết hợp với MaxHeight=6 để thực hiện "nội dung quá dài/xuống dòng chủ động tự động hiển thị nhiều dòng, tối đa 6 dòng".
func (m *Model) refitTextareaHeight() {
	w := m.textarea.Width()
	if w <= 0 {
		return
	}
	// Trong chế độ cộng tác, input cố định 1 dòng: nội dung nhiều dòng của textarea sẽ được textarea
	// cuộn theo con trỏ để hiển thị. Nếu không, chiều cao inputBox thay đổi theo nội dung sẽ làm
	// conversation bên trái thu nhỏ và input trôi dạt theo chiều dọc, phá vỡ ổn định bố cục.
	if m.cocreate != nil {
		m.textarea.SetHeight(1)
		return
	}
	text := m.textarea.Value()
	if text == "" {
		m.textarea.SetHeight(1)
		return
	}
	// Trừ 2 cột dư thừa (prompt symbol + cursor nội bộ của textarea), lệch 1 dòng là chấp nhận được.
	contentW := w - 2
	if contentW < 1 {
		contentW = 1
	}
	total := 0
	for line := range strings.SplitSeq(text, "\n") {
		lw := lipgloss.Width(line)
		if lw == 0 {
			total++
			continue
		}
		total += (lw + contentW - 1) / contentW
	}
	if total < 1 {
		total = 1
	}
	m.textarea.SetHeight(total) // SetHeight nội bộ clamp theo MaxHeight
}

// resizeTextarea đồng bộ cài đặt chiều rộng và chiều cao dựa trên nội dung.
// Thay thế các lời gọi SetWidth(currentInputWidth()) rải rác, đảm bảo chiều cao theo chiều rộng khi thay đổi.
func (m *Model) resizeTextarea() {
	m.textarea.SetWidth(m.currentInputWidth())
	m.refitTextareaHeight()
}

// maxInputHistory giới hạn độ dài lịch sử, tránh bộ nhớ tăng trong phiên dài.
const maxInputHistory = 200

// pushInputHistory thêm nội dung đã gửi thành công vào lịch sử, loại trùng liền kề. Đồng bộ đặt lại chỉ số duyệt.
func (m *Model) pushInputHistory(text string) {
	if text == "" {
		return
	}
	if n := len(m.inputHistory); n == 0 || m.inputHistory[n-1] != text {
		m.inputHistory = append(m.inputHistory, text)
		if len(m.inputHistory) > maxInputHistory {
			m.inputHistory = m.inputHistory[len(m.inputHistory)-maxInputHistory:]
		}
	}
	m.historyIdx = len(m.inputHistory)
	m.historyDraft = ""
}

// tryHistoryUp di chuyển đến lịch sử cũ hơn một mục; trả về có xử lý phím không.
// Lần đầu vào duyệt lịch sử thì lưu nội dung textarea hiện tại làm draft, khôi phục khi về cuối.
// Bên gọi cần tự xét xem ở trường hợp nhiều dòng có nên bỏ qua không (để textarea xử lý di chuyển con trỏ trong dòng).
func (m *Model) tryHistoryUp() bool {
	if len(m.inputHistory) == 0 || m.historyIdx <= 0 {
		return false
	}
	if m.historyIdx == len(m.inputHistory) {
		m.historyDraft = m.textarea.Value()
	}
	m.historyIdx--
	m.textarea.SetValue(m.inputHistory[m.historyIdx])
	m.textarea.CursorEnd()
	m.refitTextareaHeight()
	return true
}

// tryHistoryDown di chuyển đến lịch sử mới hơn một mục; về đến cuối thì khôi phục draft.
func (m *Model) tryHistoryDown() bool {
	if m.historyIdx >= len(m.inputHistory) {
		return false
	}
	m.historyIdx++
	if m.historyIdx == len(m.inputHistory) {
		m.textarea.SetValue(m.historyDraft)
		m.historyDraft = ""
	} else {
		m.textarea.SetValue(m.inputHistory[m.historyIdx])
	}
	m.textarea.CursorEnd()
	m.refitTextareaHeight()
	return true
}

// textareaIsMultiline kiểm tra nội dung textarea hiện tại có chứa xuống dòng chủ động không; dùng để quyết định ↑↓ là duyệt lịch sử hay di chuyển trong dòng.
func (m *Model) textareaIsMultiline() bool {
	return strings.Contains(m.textarea.Value(), "\n")
}

// inputHints tạo văn bản gợi ý phía dưới theo trạng thái hiện tại.
// Thêm copySuffix vào cuối để người dùng luôn thấy cách sao chép chọn văn bản ở bất kỳ trạng thái không khẩn cấp nào;
// khi chuột đã tắt thì hiển thị gợi ý màu đỏ nổi bật, nhắc nhở bấm phím để khôi phục tương tác chuột.
func (m *Model) inputHints() string {
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	if m.quitPending {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Bold(true).Render("Press Ctrl+C again to exit")
	}
	limitHint := m.inputLimitHint()
	// Trang chào (modeNew) không bật báo cáo chuột, kéo gốc của terminal là có thể sao chép, không cần gợi ý Ctrl+R;
	// bàn làm việc mới bật báo cáo, sao chép cần Ctrl+R tạm tắt.
	suffix := limitHint + " · Ctrl+R chuyển sang chế độ chọn sao chép"
	if m.mode == modeNew {
		suffix = limitHint
	}
	if m.mouseOff && m.mode != modeNew {
		// Bàn làm việc chuyển thủ công sang chọn sao chép: dùng màu nổi bật gợi ý đang ở trạng thái "kéo chọn tự do", bấm Ctrl+R để khôi phục
		return lipgloss.NewStyle().Foreground(colorAccent).Bold(true).
			Render("✂ Chế độ chọn sao chép: có thể kéo chọn văn bản để sao chép · Ctrl+R thoát khôi phục tương tác chuột")
	}
	if m.cocreate != nil {
		scrollHint := " · Tab cuộn:hội thoại"
		if m.cocreate.focusPrompt {
			scrollHint = " · Tab cuộn:chỉ thị sáng tác"
		}
		switch {
		case m.cocreate.awaiting:
			return dimStyle.Render("Chờ AI phản hồi · Esc thoát cộng tác" + scrollHint + suffix)
		case m.cocreate.canStart():
			startLabel := "Ctrl+S bắt đầu sáng tác"
			if m.cocreate.stage {
				startLabel = "Ctrl+S áp dụng và tiếp tục"
			}
			return dimStyle.Render("Enter gửi · " + startLabel + " · Esc thoát cộng tác" + scrollHint + suffix)
		default:
			return dimStyle.Render("Enter gửi · Esc thoát cộng tác" + scrollHint + suffix)
		}
	}
	if m.mode == modeNew {
		if m.startupMode == startupModeQuick {
			return dimStyle.Render("Tab chuyển chế độ khởi động · nhập / tìm lệnh · Enter bắt đầu sáng tác ngay · Esc xóa nhập" + suffix)
		}
		return dimStyle.Render("Tab chuyển chế độ khởi động · nhập / tìm lệnh · Enter bắt đầu hội thoại cộng tác · Esc xóa nhập" + suffix)
	}
	switch m.snapshot.RuntimeState {
	case "pausing":
		return dimStyle.Render("Đang tạm dừng sáng tác · vui lòng chờ lượt hiện tại kết thúc" + suffix)
	case "paused":
		return dimStyle.Render("nhập / tìm lệnh · Enter tiếp tục sáng tác · Esc xóa nhập" + suffix)
	}
	return dimStyle.Render("nhập / tìm lệnh · click/Tab chuyển panel · ↑↓ cuộn · End nhảy xuống · Ctrl+L xóa màn hình · Esc tạm dừng · Enter gửi" + suffix)
}

func (m *Model) inputLimitHint() string {
	limit := m.textarea.CharLimit
	if limit <= 0 {
		return ""
	}
	used := m.textarea.Length()
	if used < limit*4/5 {
		return ""
	}
	return fmt.Sprintf(" · nhập %d/%d", used, limit)
}

func (m *Model) eventFlowWidth() int {
	if m.width == 0 {
		return 80
	}
	leftW := m.sidebarWidth()
	rightW := m.detailWidth()
	return m.width - leftW - rightW
}

func (m *Model) sidebarWidth() int {
	if m.width == 0 {
		return 32
	}
	return m.width * 23 / 100
}

func (m *Model) detailWidth() int {
	if m.width == 0 {
		return 40
	}
	return m.width * 27 / 100
}

func (m *Model) bodyHeight() int {
	_, _, bodyH := m.layoutHeights()
	return bodyH
}

func (m *Model) currentSpinnerFrame() string {
	if !m.snapshot.IsRunning && !m.starting {
		return ""
	}
	return spinnerFrames[m.spinnerIdx%len(spinnerFrames)]
}

func (m *Model) outputDir() string {
	if m.runtime == nil {
		return ""
	}
	return m.runtime.Dir()
}

func defaultSteerPlaceholder() string {
	return "Nhập can thiệp cốt truyện, ví dụ: đưa tuyến tình cảm lên chương 4"
}

func (m *Model) syncRuntimePlaceholder() {
	if m.mode != modeRunning || m.cocreate != nil {
		return
	}
	if m.starting {
		m.textarea.Placeholder = "Đang khởi tạo sáng tác..."
		return
	}
	switch m.snapshot.RuntimeState {
	case "completed":
		m.textarea.Placeholder = "Sáng tác đã hoàn thành"
	case "pausing":
		m.textarea.Placeholder = "Đang tạm dừng sáng tác..."
	case "paused":
		m.textarea.Placeholder = "Sáng tác đã tạm dừng, nhập bất kỳ nội dung để tiếp tục sáng tác"
	default:
		if !m.snapshot.IsRunning {
			m.textarea.Placeholder = "Chạy bị gián đoạn, nhập bất kỳ nội dung để khôi phục sáng tác"
		} else {
			m.textarea.Placeholder = defaultSteerPlaceholder()
		}
	}
}

func (m *Model) renderBottomBar() string {
	inputBox := renderInputBox(
		m.textarea.View(),
		m.inputHints(),
		m.snapshot,
		m.outputDir(),
		m.width,
	)
	if m.mode != modeNew || m.cocreate != nil {
		return inputBox
	}
	return renderStartupModeBar(m.width, m.startupMode) + "\n" + inputBox
}

func (m *Model) layoutHeights() (topH, inputH, bodyH int) {
	if m.width == 0 || m.height == 0 {
		return 1, 4, 20
	}
	topH = lipgloss.Height(renderTopBar(m.snapshot, m.width, m.currentSpinnerFrame(), m.version))
	inputH = lipgloss.Height(m.renderBottomBar())
	bodyH = m.height - topH - inputH
	if bodyH < 3 {
		bodyH = 3
	}
	return
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Đang tải..."
	}
	if m.width < 100 {
		return lipgloss.NewStyle().
			Width(m.width).Height(m.height).
			AlignHorizontal(lipgloss.Center).
			AlignVertical(lipgloss.Center).
			Render("Chiều rộng terminal không đủ, vui lòng mở rộng tối thiểu 100 cột")
	}
	if m.askState != nil {
		return renderAskUserModal(m.width, m.height, m.askState)
	}
	if m.cocreate != nil {
		return renderCoCreateModal(m.width, m.height, m.cocreate, errorText(m.err), m.textarea.View(), m.spinnerIdx, m.quitPending)
	}
	if m.help != nil {
		return renderHelpModal(m.width, m.height, m.help)
	}
	if m.report != nil {
		return renderReportModal(m.width, m.height, m.report)
	}
	if m.importer != nil {
		return renderImportModal(m.width, m.height, m.importer)
	}
	if m.simulator != nil {
		return renderSimulationModal(m.width, m.height, m.simulator)
	}

	topBar := renderTopBar(m.snapshot, m.width, m.currentSpinnerFrame(), m.version)
	inputBox := m.renderBottomBar()
	_, inputH, bodyH := m.layoutHeights()

	var body string
	if m.mode == modeNew {
		errMsg := ""
		if m.err != nil {
			errMsg = m.err.Error()
		}
		body = renderWelcome(m.width, bodyH, errMsg, m.startupMode)
	} else {
		leftW := m.sidebarWidth()
		rightW := m.detailWidth()
		centerW := m.width - leftW - rightW
		eventH, streamH := m.splitHeights(bodyH)

		if m.viewport.Width != centerW-2 || m.viewport.Height != eventH-1 {
			m.viewport.Width = centerW - 2
			m.viewport.Height = eventH - 1 // -1 cho dòng header event panel
		}
		if m.streamVP.Width != centerW-2 || m.streamVP.Height != streamH-1 {
			m.streamVP.Width = centerW - 2
			m.streamVP.Height = streamH - 1 // -1 cho dòng header stream panel
		}

		eventFlow := renderEventFlowViewport(m.viewport, centerW, eventH, m.paneHighlighted(focusEvents))
		streamPanel := renderStreamPanel(m.streamVP, centerW, streamH, m.paneHighlighted(focusStream), m.snapshot.IsRunning || m.starting, m.spinnerIdx)
		center := lipgloss.JoinVertical(lipgloss.Left, eventFlow, streamPanel)

		left := renderStatePanel(m.stateVP, leftW, bodyH, m.paneHighlighted(focusState))
		right := renderDetailPanel(m.detailVP, rightW, bodyH, m.paneHighlighted(focusDetail))
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, center, right)
	}

	view := lipgloss.JoinVertical(lipgloss.Left, topBar, body, inputBox)

	// Chồng phủ popup: nổi trên phần dưới body, không ảnh hưởng bố cục
	if m.modelSwitch != nil {
		commandBar := renderModelSwitchBar(m.width, m.modelSwitch)
		view = overlayAboveInput(view, commandBar, inputH)
	} else if m.compActive {
		commandBar := renderCommandPalette(m.width, m.compItems, m.compIdx)
		view = overlayAboveInput(view, commandBar, inputH)
	}
	return view
}

// sendCoCreate gửi một lượt yêu cầu cộng tác, xử lý thống nhất reqID, textarea, placeholder.
func (m *Model) sendCoCreate() tea.Cmd {
	m.cocreateSeq++
	m.cocreate.reqID = m.cocreateSeq
	m.cocreate.awaiting = true
	m.resizeTextarea()
	m.textarea.Placeholder = placeholderForCoCreate(m.cocreate)
	m.textarea.Blur()
	return runCoCreate(m.runtime, m.cocreate)
}

func (m Model) handleCoCreateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.cocreate == nil {
		return m, nil
	}
	state := m.cocreate

	// Bàn phím ↑↓/PgUp/PgDn/Home/End cuộn; Tab chuyển tiêu điểm cuộn giữa cột hội thoại bên trái ↔ cột chỉ thị sáng tác bên phải
	// (mặc định cột trái, người dùng xem lại nội dung chính). Trang chào đã tắt báo cáo chuột để giữ sao chép gốc, khi cột phải tràn thì dùng Tab
	// chuyển tiêu điểm rồi cuộn bằng bàn phím. Cột trái: cuộn lên tắt follow, cuộn đến đáy bật lại follow (theo dõi stream).
	switch msg.Type {
	case tea.KeyTab:
		state.focusPrompt = !state.focusPrompt
		return m, nil
	case tea.KeyUp, tea.KeyPgUp:
		if state.focusPrompt {
			var cmd tea.Cmd
			state.promptVP, cmd = state.promptVP.Update(msg)
			return m, cmd
		}
		state.convFollow = false
		var cmd tea.Cmd
		state.convVP, cmd = state.convVP.Update(msg)
		return m, cmd
	case tea.KeyDown, tea.KeyPgDown:
		if state.focusPrompt {
			var cmd tea.Cmd
			state.promptVP, cmd = state.promptVP.Update(msg)
			return m, cmd
		}
		var cmd tea.Cmd
		state.convVP, cmd = state.convVP.Update(msg)
		if state.convVP.AtBottom() {
			state.convFollow = true
		}
		return m, cmd
	case tea.KeyHome:
		if state.focusPrompt {
			state.promptVP.GotoTop()
			return m, nil
		}
		state.convFollow = false
		state.convVP.GotoTop()
		return m, nil
	case tea.KeyEnd:
		if state.focusPrompt {
			state.promptVP.GotoBottom()
			return m, nil
		}
		state.convFollow = true
		state.convVP.GotoBottom()
		return m, nil
	case tea.KeyEsc:
		return m.exitCoCreate()
	}

	// 等待 AI 回复时编辑类（字符输入/退格/光标/Ctrl+U/多行换行）放行——
	// 用户能在 AI 思考期间预输入下一句。提交类的屏蔽下沉到各 case 内部，
	// 让 Enter 节流先于 awaiting 屏蔽——这样粘贴的 \n 残片仍能补空格。

	switch msg.Type {
	case tea.KeyCtrlS:
		if state.awaiting {
			return m, nil
		}
		if !state.canStart() {
			return m, nil
		}
		// Cộng tác giai đoạn: tiêm "brief hướng đi tiếp theo" và khôi phục sáng tác, quay về bàn làm việc.
		if state.stage {
			draft := state.draftPrompt()
			m.cocreate = nil
			m.err = nil
			m.resizeTextarea()
			m.textarea.Placeholder = defaultSteerPlaceholder()
			return m, tea.Batch(resumeFromCoCreate(m.runtime, draft), m.textarea.Focus())
		}
		// Cộng tác khởi động lạnh: bắt đầu sáng tác bằng chỉ thị sáng tác đã được sắp xếp.
		plan, err := state.buildPlan()
		if err != nil {
			m.err = err
			return m, nil
		}
		cmd := m.enterStarting(plan.RawPrompt)
		return m, tea.Batch(startRuntime(m.runtime, plan), cmd)
	case tea.KeyEnter:
		// Alt+Enter → xuống dòng chủ động, để textarea.Update tiếp quản (KeyMap.InsertNewline đã ràng buộc phím này)
		if msg.Alt {
			break
		}
		// Khoảng cách với lần bấm ký tự trước quá ngắn → coi là mảnh vụn \n của luồng paste: thay bằng khoảng trắng thay vì gửi.
		// Phải xét trước khi chặn awaiting——nếu không, mảnh vụn \n khi paste trong awaiting sẽ bị chặn,
		// dẫn đến "abc\ndef" bị nuốt thành "abcdef", không nhất quán với ngữ nghĩa đường base.
		if !m.lastKeyAt.IsZero() && time.Since(m.lastKeyAt) < 50*time.Millisecond {
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
			m.refitTextareaHeight()
			return m, cmd
		}
		// Ý định gửi thực sự: chặn trong awaiting (không thể gửi yêu cầu đồng thời)
		if state.awaiting {
			return m, nil
		}
		text := utils.CleanInputLine(m.textarea.Value())
		if text == "" {
			return m, nil
		}
		m.err = nil
		state.appendUser(text)
		m.textarea.Reset()
		m.refitTextareaHeight()
		cmd := m.sendCoCreate()
		return m, cmd
	case tea.KeyCtrlU:
		m.textarea.Reset()
		m.refitTextareaHeight()
		return m, nil
	}

	// Phím số 1/2/3 khi textarea trống và có gợi ý → điền gợi ý tương ứng (không gửi, có thể chỉnh sửa).
	// Chỉ chặn khi ô nhập trống, tránh ảnh hưởng người dùng chủ động gõ số. Khi awaiting gợi ý không hiển thị,
	// ở đây cũng không cần xét thêm (state.suggestions rỗng thì bỏ qua).
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && !state.awaiting {
		if r := msg.Runes[0]; r >= '1' && r <= '3' {
			if strings.TrimSpace(m.textarea.Value()) == "" {
				if sugs := state.suggestions(); int(r-'0') <= len(sugs) {
					m.textarea.SetValue(sugs[r-'1'])
					m.refitTextareaHeight()
					return m, nil
				}
			}
		}
	}

	// Nhập thông thường chuyển tiếp đến textarea
	if msg.Type == tea.KeyRunes && (containsSGRFragment(string(msg.Runes)) || isCSILeak(msg.Runes)) {
		return m, nil
	}
	var ok bool
	if msg, ok = cleanHumanKeyRunes(msg); !ok {
		return m, nil
	}
	if msg.Type == tea.KeyRunes {
		m.lastKeyAt = time.Now()
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m.refitTextareaHeight()
	return m, cmd
}

// exitCoCreate thoát chế độ cộng tác, hủy yêu cầu LLM đang tiến hành, khôi phục trạng thái ô nhập.
func (m Model) exitCoCreate() (tea.Model, tea.Cmd) {
	if m.cocreate.cancel != nil {
		m.cocreate.cancel()
	}
	stage := m.cocreate.stage
	initial := m.cocreate.initialInput()
	m.cocreate = nil
	m.resizeTextarea()
	// Hủy cộng tác giai đoạn: xóa cờ chiếm dụng, giữ trạng thái tạm dừng, quay về trạng thái nhập bàn làm việc (không điền lại câu mở đầu).
	if stage {
		m.textarea.SetValue("")
		m.textarea.Placeholder = defaultSteerPlaceholder()
		return m, tea.Batch(cancelCoCreate(m.runtime), fetchSnapshot(m.runtime), m.textarea.Focus())
	}
	m.textarea.SetValue(initial)
	m.textarea.Placeholder = placeholderForNewMode(m.startupMode)
	return m, m.textarea.Focus()
}

func (m Model) handleAskUserKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.askState == nil {
		return m, nil
	}
	state := m.askState
	q := state.currentQuestion()

	if state.typing {
		switch msg.Type {
		case tea.KeyEsc:
			state.cancelCurrentTyping()
			return m, nil
		case tea.KeyEnter:
			if state.finishCurrentAnswer() {
				state.submit()
				m.askState = nil
				return m, m.textarea.Focus()
			}
			return m, nil
		case tea.KeyBackspace, tea.KeyCtrlH:
			if state.input != "" {
				_, size := utf8.DecodeLastRuneInString(state.input)
				state.input = state.input[:len(state.input)-size]
			}
			return m, nil
		default:
			if msg.Type == tea.KeyRunes {
				state.input += utils.CleanInputRunes(msg.Runes)
			}
			return m, nil
		}
	}

	switch msg.Type {
	case tea.KeyEsc:
		// đóng popup, trả về câu trả lời rỗng
		state.request.resultCh <- askUserResult{
			resp: &tools.AskUserResponse{
				Answers: make(map[string]string),
				Notes:   make(map[string]string),
			},
		}
		m.askState = nil
		return m, m.textarea.Focus()
	case tea.KeyUp:
		state.moveCursor(-1)
	case tea.KeyDown:
		state.moveCursor(1)
	case tea.KeySpace:
		if q.MultiSelect {
			state.toggleSelection()
			if state.cursor == len(q.Options) && !state.selected[state.cursor] {
				state.input = ""
			}
		}
	case tea.KeyEnter:
		if q.MultiSelect {
			if state.cursor == len(q.Options) {
				state.toggleSelection()
				if state.selected[state.cursor] {
					state.typing = true
				}
				return m, nil
			}
			if len(state.selected) == 0 {
				state.toggleSelection()
			}
		}
		if state.finishCurrentAnswer() {
			state.submit()
			m.askState = nil
			return m, m.textarea.Focus()
		}
	}
	return m, nil
}

// overlayAboveInput đặt nổi overlay chồng phủ lên phần dưới của view base (phía trên inputBox),
// không thay đổi chiều cao tổng thể của bố cục. Chỉ che đúng chiều rộng của card overlay, nội dung bên dưới lộ ra bên phải.
func overlayAboveInput(base, overlay string, inputLineCount int) string {
	baseLines := strings.Split(base, "\n")
	overLines := strings.Split(strings.TrimRight(overlay, "\n"), "\n")

	endY := len(baseLines) - inputLineCount
	startY := endY - len(overLines)
	if startY < 0 {
		startY = 0
	}

	for i, ol := range overLines {
		y := startY + i
		if y >= 0 && y < endY {
			olW := lipgloss.Width(ol)
			// Cắt bỏ olW ký tự hiển thị bên trái của dòng gốc, ghép overlay + nội dung bên phải còn lại
			right := ansi.TruncateLeft(baseLines[y], olW, "")
			baseLines[y] = ol + right
		}
	}
	return strings.Join(baseLines, "\n")
}

// isCSILeak phát hiện KeyRunes có phải là mảnh vụn rò rỉ từ chuỗi thoát CSI không.
// Khi terminal gửi phím mũi tên \x1b[A, bấm nhanh có thể khiến chuỗi bị phân tách:
// \x1b được phân tích là Escape, "[" hoặc "[A" rò rỉ vào textarea như KeyRunes.
func isCSILeak(runes []rune) bool {
	if len(runes) == 0 || runes[0] != '[' {
		return false
	}
	for _, r := range runes[1:] {
		if (r >= '0' && r <= '9') || r == ';' ||
			(r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '~' {
			continue
		}
		return false
	}
	return true
}

// containsSGRFragment phát hiện văn bản có chứa mảnh vụn chuỗi chuột SGR (mẫu "<số;số;") không.
func containsSGRFragment(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '<' {
			continue
		}
		j := i + 1
		if j >= len(s) || s[j] < '0' || s[j] > '9' {
			continue
		}
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j < len(s) && s[j] == ';' {
			return true
		}
	}
	return false
}

func cleanHumanKeyRunes(msg tea.KeyMsg) (tea.KeyMsg, bool) {
	if msg.Type != tea.KeyRunes {
		return msg, true
	}
	cleaned := utils.CleanInputRunes(msg.Runes)
	if cleaned == "" {
		return msg, false
	}
	msg.Runes = []rune(cleaned)
	return msg, true
}

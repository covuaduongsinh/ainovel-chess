package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/voocel/ainovel-cli/internal/diag"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/entry/startup"
	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/store"
)

// Kiểu thông điệp
type (
	eventMsg       host.Event
	snapshotMsg    host.UISnapshot
	doneMsg        struct{ complete bool } // complete=true toàn bộ sách hoàn thành, false dừng do lỗi
	abortResultMsg struct{ stopped bool }
	bootstrapMsg   struct {
		replay  []domain.RuntimeQueueItem
		resumed bool
		err     error
	}
	reportLoadedMsg struct {
		reqID      int
		report     diag.Report
		exportPath string // đường dẫn tuyệt đối file chẩn đoán đã ẩn danh; rỗng = xuất thất bại
		finishedAt time.Time
	}
	askUserMsg       askUserRequest
	startResultMsg   struct{ err error }
	cocreateDeltaMsg struct {
		reqID int
		kind  string // host.CoCreateProgressThinking | host.CoCreateProgressReply
		text  string
	}
	// cocreateStreamItem là payload nội bộ của deltaCh, gửi kind stream và văn bản tích lũy cùng nhau đến TUI.
	cocreateStreamItem struct {
		kind string
		text string
	}
	cocreateDoneMsg struct {
		reqID int
		reply host.CoCreateReply
		err   error
	}
	steerResultMsg     struct{}
	continueResultMsg  struct{ err error }
	spinnerTickMsg     time.Time
	toolSpinnerTickMsg time.Time // tick độc lập của spinner công cụ luồng sự kiện (nhanh hơn, độc lập với thanh đầu/ngôi sao)
	cursorTickMsg      time.Time // tick độc lập của con trỏ stream
	streamDeltaMsg     string    // delta token stream
	streamClearMsg     struct{}  // xóa buffer stream (bắt đầu tin nhắn mới)
	streamFlushTickMsg struct{}  // làm mới panel stream 60fps với giới hạn tốc độ (hợp nhất delta cấp token)
	quitResetMsg       struct{}  // đặt lại timeout Ctrl+C kép
)

// --- Hàm Cmd ---

func listenEvents(rt *host.Host) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-rt.Events()
		if !ok {
			return nil
		}
		return eventMsg(ev)
	}
}

func listenDone(rt *host.Host) tea.Cmd {
	return func() tea.Msg {
		_, ok := <-rt.Done()
		if !ok {
			return nil
		}
		snap := rt.Snapshot()
		return doneMsg{complete: snap.Phase == "complete"}
	}
}

func tickSnapshot(rt *host.Host) tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return snapshotMsg(rt.Snapshot())
	})
}

func fetchSnapshot(rt *host.Host) tea.Cmd {
	return func() tea.Msg {
		return snapshotMsg(rt.Snapshot())
	}
}

func bootstrapRuntime(rt *host.Host) tea.Cmd {
	return func() tea.Msg {
		replay, err := rt.ReplayQueue(0)
		if err != nil {
			return bootstrapMsg{err: err}
		}
		label, err := rt.Resume()
		if err != nil {
			return bootstrapMsg{replay: replay, err: err}
		}
		if label == "" && len(replay) == 0 {
			return nil
		}
		return bootstrapMsg{replay: replay, resumed: label != ""}
	}
}

func startRuntime(rt *host.Host, plan startup.Plan) tea.Cmd {
	return func() tea.Msg {
		// Phía khởi động tạo xác định snapshot quy tắc người dùng cho cuốn sách này (chuẩn hóa bằng prompt gốc), phải trước StartPrepared.
		if err := rt.PrepareUserRules(plan.RawPrompt); err != nil {
			return startResultMsg{err: err}
		}
		err := rt.StartPrepared(plan.StartPrompt)
		return startResultMsg{err: err}
	}
}

func runCoCreate(rt *host.Host, state *cocreateState) tea.Cmd {
	history := state.session.History()
	ctx, cancel := context.WithCancel(context.Background())
	state.cancel = cancel
	state.deltaCh = make(chan cocreateStreamItem, 64)
	state.doneCh = make(chan cocreateDoneMsg, 1)
	// Cộng tác giai đoạn mang tóm tắt trạng thái câu chuyện, tạo ra "brief hướng đi tiếp theo"; khởi động lạnh làm rõ yêu cầu từ đầu. Cả hai có chữ ký giống nhau.
	stream := rt.CoCreateStream
	if state.stage {
		stream = rt.StageCoCreateStream
	}
	start := func() tea.Msg {
		go func() {
			reply, err := stream(ctx, history, func(kind, text string) {
				select {
				case state.deltaCh <- cocreateStreamItem{kind: kind, text: text}:
				default:
				}
			})
			state.doneCh <- cocreateDoneMsg{reply: reply, err: err}
			close(state.deltaCh)
			close(state.doneCh)
		}()
		return nil
	}
	return tea.Batch(start, listenCoCreateDelta(state), listenCoCreateDone(state))
}

func listenCoCreateDelta(state *cocreateState) tea.Cmd {
	if state == nil || state.deltaCh == nil {
		return nil
	}
	// Lấy tham chiếu cục bộ của channel: tránh trường hợp sau này state.deltaCh bị reassign
	// closure listen cũ đọc nhầm channel mới (dù luồng hiện tại không kích hoạt, không nên để lại bẫy bảo trì).
	reqID := state.reqID
	ch := state.deltaCh
	return func() tea.Msg {
		item, ok := <-ch
		if !ok {
			return nil
		}
		return cocreateDeltaMsg{reqID: reqID, kind: item.kind, text: item.text}
	}
}

func listenCoCreateDone(state *cocreateState) tea.Cmd {
	if state == nil || state.doneCh == nil {
		return nil
	}
	reqID := state.reqID
	ch := state.doneCh
	return func() tea.Msg {
		result, ok := <-ch
		if !ok {
			return nil
		}
		result.reqID = reqID
		return result
	}
}

func steerRuntime(rt *host.Host, text string) tea.Cmd {
	return func() tea.Msg {
		rt.Steer(text)
		return steerResultMsg{}
	}
}

func continueRuntime(rt *host.Host, text string) tea.Cmd {
	return func() tea.Msg {
		err := rt.Continue(text)
		return continueResultMsg{err: err}
	}
}

// resumeFromCoCreate tiêm brief hướng đi tiếp theo từ cộng tác giai đoạn và khôi phục sáng tác.
// Tái sử dụng continueResultMsg: thành công thì tiếp tục với listenDone, thất bại hiển thị lỗi.
func resumeFromCoCreate(rt *host.Host, draft string) tea.Cmd {
	return func() tea.Msg {
		err := rt.ResumeFromCoCreate(draft)
		return continueResultMsg{err: err}
	}
}

// cancelCoCreate hủy bỏ cộng tác giai đoạn: xóa cờ chiếm dụng, giữ trạng thái tạm dừng. Sự kiện hồi lưu qua kênh events, không cần trả thông điệp.
func cancelCoCreate(rt *host.Host) tea.Cmd {
	return func() tea.Msg {
		rt.CancelCoCreate()
		return nil
	}
}

func abortRuntime(rt *host.Host) tea.Cmd {
	return func() tea.Msg {
		return abortResultMsg{stopped: rt.Abort()}
	}
}

func loadReport(dir string, reqID int) tea.Cmd {
	return func() tea.Msg {
		s := store.NewStore(dir)
		// Diagnose = chẩn đoán sáng tác + phát hiện runtime, Finding của runtime cũng vào báo cáo trên màn hình.
		rep, rc := diag.Diagnose(s)
		// Tái sử dụng rep+rc để ghi file chẩn đoán đã ẩn danh (xuất thất bại không ảnh hưởng báo cáo trên màn hình).
		exportPath, _ := diag.WriteExport(s, rep, rc)
		return reportLoadedMsg{
			reqID:      reqID,
			report:     rep,
			exportPath: exportPath,
			finishedAt: time.Now(),
		}
	}
}

func tickSpinner() tea.Cmd {
	return tea.Tick(350*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg(t)
	})
}

// tickToolSpinner điều khiển spinner của dòng "đang tiến hành" trong luồng sự kiện. Độc lập với tickSpinner, nhịp nhanh hơn (150ms).
func tickToolSpinner() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg {
		return toolSpinnerTickMsg(t)
	})
}

func tickCursor() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg {
		return cursorTickMsg(t)
	})
}

// tickStreamFlush điều khiển làm mới panel stream có giới hạn tốc độ. streamDelta không còn tái render ngay mỗi token,
// mà đánh dấu dirty; tick này kiểm tra và hợp nhất làm mới mỗi 16ms (~60fps), nén "hàng chục lần tái render đầy đủ mỗi giây"
// trong giai đoạn LLM stream tốc độ cao xuống giới hạn 60 lần.
func tickStreamFlush() tea.Cmd {
	return tea.Tick(16*time.Millisecond, func(t time.Time) tea.Msg {
		return streamFlushTickMsg{}
	})
}

func listenStream(rt *host.Host) tea.Cmd {
	return func() tea.Msg {
		delta, ok := <-rt.Stream()
		if !ok {
			return nil
		}
		// sentinel được phân phát dưới dạng streamClearMsg, đảm bảo đến TUI theo thứ tự emit trong cùng kênh với delta thông thường.
		// Khi dùng hai kênh, clearCh và streamCh không có thứ tự, header ✻ thường bị nhét nhầm vào cuối đoạn thinking trước.
		if delta == host.StreamClearSentinel {
			return streamClearMsg{}
		}
		return streamDeltaMsg(delta)
	}
}

func listenAskUser(bridge *askUserBridge) tea.Cmd {
	return func() tea.Msg {
		req, ok := <-bridge.requests
		if !ok {
			return nil
		}
		return askUserMsg(req)
	}
}

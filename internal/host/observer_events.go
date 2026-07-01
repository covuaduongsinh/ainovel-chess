package host

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"encoding/json"
	"github.com/voocel/agentcore"
	"log/slog"
)

// isCancellationNoise kiểm tra một lỗi có phải là nhiễu dẫn xuất do abort gây ra không.
// Chỉ có ý nghĩa khi trả về true trong khi Host ở trạng thái aborting —
// context.Canceled không phải trong giai đoạn abort có thể phản ánh vấn đề thực (như ctx bên ngoài bị hủy), vẫn cần báo cáo.
func (o *observer) isCancellationNoise(err error, msg string) bool {
	if !o.aborting.Load() {
		return false
	}
	if err != nil && errors.Is(err, context.Canceled) {
		return true
	}
	return strings.Contains(strings.ToLower(msg), "context canceled")
}

func (o *observer) handle(ev agentcore.Event) {
	switch ev.Type {
	case agentcore.EventToolExecStart:
		o.handleToolStart(ev)
	case agentcore.EventToolExecUpdate:
		o.handleToolUpdate(ev)
	case agentcore.EventToolExecEnd:
		o.handleToolEnd(ev)
	case agentcore.EventMessageUpdate:
		o.handleMessageUpdate(ev)
	case agentcore.EventMessageEnd:
		o.streamClear()
	case agentcore.EventTurnStart:
		if ev.Progress != nil && ev.Progress.Kind == agentcore.ProgressTurnCounter {
			o.updateAgent(ev.Progress.Agent, func(a *agentState) {
				a.turn = ev.Progress.Turn
			})
		}
	case agentcore.EventRetry:
		if ev.RetryInfo != nil {
			msg := ""
			if ev.RetryInfo.Err != nil {
				msg = ev.RetryInfo.Err.Error()
			}
			prefix := retryPrefix(ev.RetryInfo.Attempt, ev.RetryInfo.MaxRetries, ev.RetryInfo.Delay)
			retryEv := Event{
				ID:       o.retryEventID("coordinator", ev.RetryInfo.Attempt),
				Time:     time.Now(),
				Category: "SYSTEM",
				Summary:  prefix + truncate(msg, 80),
				Detail:   prefix + msg,
				Kind:     errorKind(ev.RetryInfo.Err, msg),
				Level:    "warn",
			}
			o.emitEv(retryEv)
			o.persistEvent(retryEv)
		}
	case agentcore.EventError:
		if ev.Err != nil {
			fullMsg := ev.Err.Error()
			if o.isCancellationNoise(ev.Err, fullMsg) {
				// Lỗi ctx-cancel dẫn xuất từ người dùng chủ động abort; đã có sự kiện "người dùng tạm dừng thủ công", không lặp lại.
				o.flushActiveCalls(true)
				slog.Debug("suppressed cancel-derived error", "module", "agent", "msg", fullMsg)
				return
			}
			o.flushActiveCalls(true)
			errEv := Event{
				Time:     time.Now(),
				Category: "ERROR",
				Summary:  truncate(fullMsg, 120),
				Detail:   fullMsg,
				Kind:     errorKind(ev.Err, fullMsg),
				Level:    "error",
			}
			o.emitEv(errEv)
			o.persistEvent(errEv)
		}
	}
}

func retryPrefix(attempt, maxRetries int, delay time.Duration) string {
	if text := formatRetryDelay(delay); text != "" {
		return fmt.Sprintf("Thử lại (%d/%d，sau %s): ", attempt, maxRetries, text)
	}
	return fmt.Sprintf("Thử lại (%d/%d): ", attempt, maxRetries)
}

func formatRetryDelay(delay time.Duration) string {
	if delay <= 0 {
		return ""
	}
	seconds := int64(delay / time.Second)
	if delay%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		seconds = 1
	}
	return (time.Duration(seconds) * time.Second).String()
}

func (o *observer) handleMessageUpdate(ev agentcore.Event) {
	if ev.Delta == "" {
		return
	}
	if ev.DeltaKind == agentcore.DeltaToolCall {
		o.handleCoordinatorToolDelta(ev)
		return
	}
	o.emitStreamDelta(ev.Delta, ev.DeltaKind == agentcore.DeltaThinking)
}

func (o *observer) handleThinkingProgress(ev agentcore.Event) {
	agent := ev.Progress.Agent
	thinking := ev.Progress.Thinking
	if agent == "" || thinking == "" {
		return
	}

	prev := o.lastThinkingByAgent[agent]
	delta := thinking
	if strings.HasPrefix(thinking, prev) {
		delta = thinking[len(prev):]
	}
	o.lastThinkingByAgent[agent] = thinking
	if delta == "" {
		return
	}
	o.emitStreamDelta(delta, true)
}

func (o *observer) handleContextProgress(ev agentcore.Event) {
	if ev.Progress == nil || len(ev.Progress.Meta) == 0 {
		return
	}
	var payload struct {
		Tokens        int     `json:"tokens"`
		ContextWindow int     `json:"context_window"`
		Percent       float64 `json:"percent"`
		Scope         string  `json:"scope"`
		Strategy      string  `json:"strategy"`
	}
	if json.Unmarshal(ev.Progress.Meta, &payload) != nil {
		return
	}

	agent := ev.Progress.Agent
	if agent == "" {
		agent = "coordinator"
	}

	// Cập nhật snapshot agent (thanh bên TUI luôn hiển thị)
	o.updateAgent(agent, func(a *agentState) {
		a.context = AgentContextSnapshot{
			Tokens:        payload.Tokens,
			ContextWindow: payload.ContextWindow,
			Percent:       payload.Percent,
			Scope:         payload.Scope,
			Strategy:      payload.Strategy,
		}
	})

	level := "info"
	if payload.Percent > 85 {
		level = "warn"
	}
	summary := fmt.Sprintf("%s ngữ cảnh %.0f%% (%d/%d) chiến lược: %s", agent, payload.Percent, payload.Tokens, payload.ContextWindow, payload.Strategy)

	depth := 0
	if agent != "coordinator" {
		depth = 1
	}

	if payload.Strategy != "" {
		// Đã kích hoạt nén → luồng sự kiện + log
		ctxEv := Event{Time: time.Now(), Category: "SYSTEM", Agent: agent, Summary: summary, Level: level, Depth: depth}
		o.emitEv(ctxEv)
		o.persistEvent(ctxEv)
	} else {
		// Báo cáo tỷ lệ sử dụng thông thường → chỉ log
		slogLevel := slog.LevelInfo
		if level == "warn" {
			slogLevel = slog.LevelWarn
		}
		slog.Log(context.Background(), slogLevel, summary, "module", "context", "agent", agent)
	}
}

package host

import (
	"fmt"
	"strings"
	"time"

	"encoding/json"
	"github.com/voocel/agentcore"
	"log/slog"
)

func (o *observer) handleToolStart(ev agentcore.Event) {
	if ev.Tool == "" {
		return
	}
	agent := agentFromEvent(ev)

	// subagent gọi → sự kiện DISPATCH (đang tiến hành)
	if ev.Tool == "subagent" {
		sub := parseSubagentArgs(ev.Args)
		target := sub.agent
		if target == "" {
			target = "subagent"
		}
		dispatchSummary := dispatchSummary(target, sub.task)
		o.updateAgent(agent, func(a *agentState) {
			a.state = "working"
			a.tool = ev.Tool
			a.summary = fmt.Sprintf("%s → %s", agent, dispatchSummary)
		})
		o.currentDispatchTarget = target
		if call, ok := o.dispatchStarts["subagent"]; ok {
			delete(o.dispatchStarts, "subagent")
			o.dispatchStarts[target] = call
			o.updateDispatchSummary(target, dispatchSummary)
			return
		}
		id := nextEventID()
		o.dispatchStarts[target] = &activeCall{id: id, start: time.Now(), summary: dispatchSummary}
		o.emitAndLog(Event{
			ID:       id,
			Time:     time.Now(),
			Category: "DISPATCH",
			Agent:    agent,
			Summary:  dispatchSummary,
			Level:    "info",
		})
		return
	}

	// Công cụ của chính coordinator (đang tiến hành)
	toolName := displayToolName(ev.Tool, ev.Args)
	if _, ok := o.toolStarts[agent]; ok {
		o.updateToolCallSummary(agent, ev.Tool, toolName)
		return
	}
	o.updateAgent(agent, func(a *agentState) {
		a.state = "working"
		a.tool = ev.Tool
		a.summary = fmt.Sprintf("%s → %s", agent, toolName)
	})
	id := nextEventID()
	o.toolStarts[agent] = &activeCall{id: id, start: time.Now(), summary: toolName}
	o.emitAndLog(Event{
		ID:       id,
		Time:     time.Now(),
		Category: "TOOL",
		Agent:    agent,
		Summary:  toolName,
		Level:    "info",
	})
	o.emitFallbackStreamHeader(ev.Tool)
}

func (o *observer) handleToolUpdate(ev agentcore.Event) {
	if ev.Progress == nil {
		return
	}
	switch ev.Progress.Kind {
	case agentcore.ProgressToolDelta:
		if ev.Progress.Delta != "" {
			o.handleSubagentDelta(ev.Progress)
		}
	case agentcore.ProgressToolStart:
		// Gọi công cụ nội bộ của subagent (ví dụ: writer → draft_chapter).
		// Lưu ý: dòng TOOL có thể đã được handleSubagentDelta phát sớm trong giai đoạn nhận diện luồng.
		// Ở đây: nếu đã phát → chỉ cập nhật summary (args lúc này đầy đủ, có thể hiển thị "tool(chương N)"); nếu chưa thì phát bình thường.
		if ev.Progress.Agent == "" || ev.Progress.Tool == "" {
			break
		}
		toolName := displayToolName(ev.Progress.Tool, ev.Progress.Args)
		if _, ok := o.toolStarts[ev.Progress.Agent]; ok {
			o.updateToolCallSummary(ev.Progress.Agent, ev.Progress.Tool, toolName)
			o.updateAgent(ev.Progress.Agent, func(a *agentState) {
				a.state = "working"
				a.tool = ev.Progress.Tool
				a.summary = fmt.Sprintf("%s → %s", ev.Progress.Agent, toolName)
			})
			break
		}
		// Chưa từng phát sớm → luồng bình thường
		// (Mô hình với tool args không theo luồng sẽ không kích hoạt ensureSubagentToolStarted,
		// fallback header phải được bổ sung trên đường dẫn này, nếu không các công cụ như read_chapter
		// không có extractor sẽ không có ✻ đầu trang trên bảng luồng, dính liền với đoạn thinking phía trước.)
		id := nextEventID()
		o.toolStarts[ev.Progress.Agent] = &activeCall{id: id, start: time.Now(), summary: toolName, depth: 1}
		o.emitAndLog(Event{
			ID:       id,
			Time:     time.Now(),
			Category: "TOOL",
			Agent:    ev.Progress.Agent,
			Summary:  toolName,
			Level:    "info",
			Depth:    1,
		})
		o.updateAgent(ev.Progress.Agent, func(a *agentState) {
			a.state = "working"
			a.tool = ev.Progress.Tool
			a.summary = fmt.Sprintf("%s → %s", ev.Progress.Agent, toolName)
		})
		o.emitFallbackStreamHeader(ev.Progress.Tool)
	case agentcore.ProgressToolEnd:
		delete(o.streamExtractors, ev.Progress.Agent)
		if ev.Progress.Agent == "" {
			return
		}
		call, ok := o.toolStarts[ev.Progress.Agent]
		if !ok {
			return
		}
		delete(o.toolStarts, ev.Progress.Agent)
		// Sự kiện cập nhật cùng ID: TUI định vị dòng TOOL gốc theo ID, điền lại FinishedAt / Duration.
		// Summary / Depth cũng được gửi kèm, đảm bảo runtime queue replay có thể khôi phục dòng đầy đủ.
		finishEv := Event{
			ID:         call.id,
			Time:       call.start,
			FinishedAt: time.Now(),
			Category:   "TOOL",
			Agent:      ev.Progress.Agent,
			Summary:    call.summary,
			Level:      "info",
			Depth:      call.depth,
			Duration:   time.Since(call.start),
		}
		o.emitEv(finishEv)
		o.persistEvent(finishEv)
	case agentcore.ProgressThinking:
		o.handleThinkingProgress(ev)
	case agentcore.ProgressRetry:
		prefix := retryPrefix(ev.Progress.Attempt, ev.Progress.MaxRetries, 0)
		retryEv := Event{
			ID:       o.retryEventID(ev.Progress.Agent, ev.Progress.Attempt),
			Time:     time.Now(),
			Category: "SYSTEM",
			Agent:    ev.Progress.Agent,
			Summary:  prefix + truncate(ev.Progress.Message, 80),
			Detail:   prefix + ev.Progress.Message,
			Kind:     errorKind(nil, ev.Progress.Message),
			Level:    "warn",
			Depth:    1,
		}
		o.emitEv(retryEv)
		o.persistEvent(retryEv)
	case agentcore.ProgressToolError:
		delete(o.streamExtractors, ev.Progress.Agent)
		msg := ev.Progress.Message
		if msg == "" {
			msg = "unknown error"
		}
		// Nếu có dòng TOOL đang tiến hành, đánh dấu tại chỗ là thất bại; nếu không thì thêm dòng ERROR độc lập.
		if call, ok := o.toolStarts[ev.Progress.Agent]; ok {
			delete(o.toolStarts, ev.Progress.Agent)
			finishEv := Event{
				ID:         call.id,
				Time:       call.start,
				FinishedAt: time.Now(),
				Failed:     true,
				Category:   "TOOL",
				Agent:      ev.Progress.Agent,
				Summary:    call.summary,
				Level:      "error",
				Depth:      call.depth,
				Duration:   time.Since(call.start),
			}
			o.emitEv(finishEv)
			o.persistEvent(finishEv)
		}
		// Thêm dòng chi tiết ERROR (bổ sung thông tin lỗi, tiện tra cứu)
		errEv := Event{
			Time:     time.Now(),
			Category: "ERROR",
			Agent:    ev.Progress.Agent,
			Summary:  fmt.Sprintf("%s lỗi: %s", ev.Progress.Tool, truncate(msg, 100)),
			Detail:   fmt.Sprintf("%s lỗi: %s", ev.Progress.Tool, msg),
			Kind:     errorKind(nil, msg),
			Level:    "error",
			Depth:    1,
		}
		o.emitEv(errEv)
		o.persistEvent(errEv)
	case agentcore.ProgressContext:
		o.handleContextProgress(ev)
	}
}

func (o *observer) ensureCoordinatorToolStarted(tool string) {
	const agent = "coordinator"
	if tool == "" {
		return
	}
	if _, ok := o.toolStarts[agent]; ok {
		return
	}
	o.resetStreamArgLabel(agent, tool)
	id := nextEventID()
	o.toolStarts[agent] = &activeCall{id: id, start: time.Now(), summary: tool}
	o.updateAgent(agent, func(a *agentState) {
		a.state = "working"
		a.tool = tool
		a.summary = fmt.Sprintf("%s → %s", agent, tool)
	})
	o.emitAndLog(Event{
		ID:       id,
		Time:     time.Now(),
		Category: "TOOL",
		Agent:    agent,
		Summary:  tool,
		Level:    "info",
	})
	o.emitFallbackStreamHeader(tool)
}

func (o *observer) ensureCoordinatorDispatchStarted(call agentcore.ToolCall) {
	if _, ok := o.dispatchStarts["subagent"]; ok {
		return
	}
	o.resetStreamArgLabel("coordinator", call.Name)
	id := nextEventID()
	o.dispatchStarts["subagent"] = &activeCall{id: id, start: time.Now(), summary: "subagent"}
	o.currentDispatchTarget = "subagent"
	o.updateAgent("coordinator", func(a *agentState) {
		a.state = "working"
		a.tool = call.Name
		a.summary = "coordinator → subagent"
	})
	o.emitAndLog(Event{
		ID:       id,
		Time:     time.Now(),
		Category: "DISPATCH",
		Agent:    "coordinator",
		Summary:  "subagent",
		Level:    "info",
	})
}

func (o *observer) updateCoordinatorDispatchSummaryFromDelta(delta string) {
	const key = "subagent"
	prefix := o.streamArgPrefixes[streamArgKey("coordinator", key)] + delta
	if len(prefix) > 1024 {
		prefix = prefix[:1024]
	}
	o.streamArgPrefixes[streamArgKey("coordinator", key)] = prefix

	agent := firstJSONStringField(prefix, "agent")
	if agent == "" {
		return
	}
	task := firstJSONStringField(prefix, "task")
	summary := dispatchSummary(agent, task)
	labelKey := streamArgKey("coordinator", key)
	if o.streamArgLabels[labelKey] == summary {
		return
	}
	o.streamArgLabels[labelKey] = summary
	o.updateDispatchSummary("subagent", summary)
}

func dispatchSummary(agent, task string) string {
	if agent == "" {
		agent = "subagent"
	}
	if task == "" {
		return agent
	}
	firstLine := strings.TrimSpace(strings.SplitN(task, "\n", 2)[0])
	if firstLine == "" {
		return agent
	}
	return agent + "（" + truncate(firstLine, 30) + "）"
}

func (o *observer) updateToolCallSummary(agent, tool, summary string) {
	if agent == "" || summary == "" {
		return
	}
	call, ok := o.toolStarts[agent]
	if !ok || call.summary == summary {
		return
	}
	call.summary = summary
	o.emitEv(Event{
		ID:       call.id,
		Time:     call.start,
		Category: "TOOL",
		Agent:    agent,
		Summary:  summary,
		Level:    "info",
		Depth:    call.depth,
	})
	o.updateAgent(agent, func(a *agentState) {
		a.state = "working"
		a.tool = tool
		a.summary = fmt.Sprintf("%s → %s", agent, summary)
	})
}

func (o *observer) updateDispatchSummary(target, summary string) {
	if target == "" || summary == "" {
		return
	}
	call, ok := o.dispatchStarts[target]
	if !ok || call.summary == summary {
		return
	}
	call.summary = summary
	o.emitEv(Event{
		ID:       call.id,
		Time:     call.start,
		Category: "DISPATCH",
		Agent:    "coordinator",
		Summary:  summary,
		Level:    "info",
		Depth:    call.depth,
	})
}

func (o *observer) updateToolCallSummaryFromDelta(agent, tool, delta string) {
	key := streamArgKey(agent, tool)
	prefix := o.streamArgPrefixes[key] + delta
	if len(prefix) > 512 {
		prefix = prefix[:512]
	}
	o.streamArgPrefixes[key] = prefix

	summary := streamedToolLabel(tool, prefix)
	if summary == "" {
		return
	}
	if o.streamArgLabels[key] == summary {
		return
	}
	o.streamArgLabels[key] = summary
	o.updateToolCallSummary(agent, tool, summary)
}

func streamArgKey(agent, tool string) string {
	return agent + "\x00" + tool
}

func streamedToolLabel(tool, delta string) string {
	if tool != "save_foundation" || delta == "" {
		return ""
	}
	typ := firstJSONStringField(delta, "type")
	if typ == "" {
		return ""
	}
	return fmt.Sprintf("%s[%s]", tool, typ)
}

func firstJSONStringField(raw, field string) string {
	needle := `"` + field + `"`
	idx := strings.Index(raw, needle)
	if idx < 0 {
		return ""
	}
	rest := raw[idx+len(needle):]
	colon := strings.IndexByte(rest, ':')
	if colon < 0 {
		return ""
	}
	rest = strings.TrimLeft(rest[colon+1:], " \t\r\n")
	if len(rest) == 0 || rest[0] != '"' {
		return ""
	}
	var value strings.Builder
	escape := false
	for i := 1; i < len(rest); i++ {
		c := rest[i]
		if escape {
			value.WriteByte(c)
			escape = false
			continue
		}
		switch c {
		case '\\':
			escape = true
		case '"':
			return value.String()
		default:
			value.WriteByte(c)
		}
	}
	return ""
}

func (o *observer) emitCallFinish(call *activeCall, category, agentName string, failed bool) {
	if call == nil {
		return
	}
	level := "success"
	if failed {
		level = "error"
	}
	finishEv := Event{
		ID:         call.id,
		Time:       call.start,
		FinishedAt: time.Now(),
		Failed:     failed,
		Category:   category,
		Agent:      agentName,
		Summary:    call.summary,
		Level:      level,
		Depth:      call.depth,
		Duration:   time.Since(call.start),
	}
	o.emitEv(finishEv)
	o.persistEvent(finishEv)
}

func (o *observer) flushActiveCalls(failed bool) {
	for target, call := range o.dispatchStarts {
		o.emitCallFinish(call, "DISPATCH", target, failed)
		delete(o.dispatchStarts, target)
	}
	for agent, call := range o.toolStarts {
		o.emitCallFinish(call, "TOOL", agent, failed)
		delete(o.toolStarts, agent)
	}
	clear(o.streamExtractors)
	clear(o.streamArgPrefixes)
	clear(o.streamArgLabels)
	o.currentDispatchTarget = ""
}

func (o *observer) handleToolEnd(ev agentcore.Event) {
	agent := agentFromEvent(ev)
	// Công cụ kết thúc: chuyển trạng thái về idle, nếu không thanh bên sẽ mãi ở working.
	// Trạng thái của dispatchTarget khi kết thúc dispatch subagent sẽ được xóa riêng ở bên dưới.
	o.updateAgent(agent, func(a *agentState) {
		a.tool = ""
		a.state = "idle"
	})
	delete(o.lastThinkingByAgent, agent)

	// Lấy bản ghi DISPATCH đang tiến hành (ev.Args của handleToolEnd có thể rỗng, lấy từ currentDispatchTarget)
	var dispatchCall *activeCall
	var dispatchTarget string
	if ev.Tool == "subagent" {
		dispatchTarget = o.currentDispatchTarget
		o.currentDispatchTarget = ""
		if dispatchTarget == "" {
			if sub := parseSubagentArgs(ev.Args); sub.agent != "" {
				dispatchTarget = sub.agent
			}
		}
		if dispatchTarget == "" {
			dispatchTarget = "subagent"
		}
		if call, ok := o.dispatchStarts[dispatchTarget]; ok {
			dispatchCall = call
			delete(o.dispatchStarts, dispatchTarget)
		}
		// Dispatch kết thúc: reset trạng thái subagent về idle (tất cả đường thành công/thất bại/lỗi đều cần dọn dẹp này)
		if dispatchTarget != "subagent" {
			o.updateAgent(dispatchTarget, func(a *agentState) {
				a.state = "idle"
				a.tool = ""
			})
		}
	}

	// Lấy bản ghi đang tiến hành của công cụ trực tiếp coordinator (không phải subagent) (hiếm, nhưng đảm bảo nhất quán)
	var toolCall *activeCall
	if ev.Tool != "subagent" {
		if call, ok := o.toolStarts[agent]; ok {
			toolCall = call
			delete(o.toolStarts, agent)
		}
	}

	// Trạng thái hoàn thành gọi thống nhất (thành công/thất bại), cập nhật dòng gốc thông qua cùng ID
	emitFinish := func(call *activeCall, category, agentName string, failed bool) {
		o.emitCallFinish(call, category, agentName, failed)
	}
	emitDispatchFinish := func(failed bool) {
		emitFinish(dispatchCall, "DISPATCH", dispatchTarget, failed)
	}
	emitToolFinish := func(failed bool) {
		emitFinish(toolCall, "TOOL", agent, failed)
	}
	// Phòng thủ cuối: nếu khi subagent kết thúc, bên trong subagent đó còn có lần gọi TOOL chưa hoàn thành
	// (ví dụ ensureSubagentToolStarted đã phát sự kiện đang tiến hành sớm, nhưng sau đó abort/context cancel
	// khiến ProgressToolEnd không đến), ở đây cưỡng chế phát finish, tránh dòng TOOL mãi "đang tiến hành".
	// Trạng thái theo sau dispatch đồng bộ.
	flushOrphanSubagentTool := func(failed bool) {
		if dispatchTarget == "" {
			return
		}
		call, ok := o.toolStarts[dispatchTarget]
		if !ok {
			return
		}
		delete(o.toolStarts, dispatchTarget)
		delete(o.streamExtractors, dispatchTarget)
		emitFinish(call, "TOOL", dispatchTarget, failed)
	}

	if ev.IsError {
		depth := 0
		if agent != "coordinator" {
			depth = 1
		}
		errText := ""
		if len(ev.Result) > 0 {
			errText = string(ev.Result)
		}
		// ctx-cancel dẫn xuất từ người dùng chủ động abort: vẫn phải dọn dẹp trạng thái (dòng dispatch / tool phải về hoàn thành),
		// nhưng bỏ qua dòng ERROR độc lập + log lỗi, giữ nhất quán với đường EventError.
		if o.isCancellationNoise(nil, errText) {
			slog.Debug("suppressed cancel-derived tool error", "module", "agent", "tool", ev.Tool, "msg", errText)
			flushOrphanSubagentTool(true)
			emitDispatchFinish(true)
			emitToolFinish(true)
			return
		}
		summary := fmt.Sprintf("%s thất bại", ev.Tool)
		detail := summary
		kind := ""
		if errText != "" {
			kind = errorKind(nil, errText)
			detail = fmt.Sprintf("%s → %s: %s", agent, ev.Tool, errText)
			summary += ": " + truncate(errText, 120)
		}
		flushOrphanSubagentTool(true)
		emitDispatchFinish(true)
		emitToolFinish(true)
		errEv := Event{
			Time:     time.Now(),
			Category: "ERROR",
			Agent:    agent,
			Summary:  summary,
			Detail:   detail,
			Kind:     kind,
			Level:    "error",
			Depth:    depth,
		}
		o.emitEv(errEv)
		o.persistEvent(errEv)
		return
	}

	if errEv, fullErr := o.subagentResultErrorEvent(ev); errEv != nil {
		if o.isCancellationNoise(nil, fullErr) {
			slog.Debug("suppressed cancel-derived subagent error", "module", "agent", "tool", ev.Tool, "msg", fullErr)
			flushOrphanSubagentTool(true)
			emitDispatchFinish(true)
			return
		}
		if dispatchTarget != "" && dispatchTarget != "subagent" {
			errEv.Agent = dispatchTarget
		}
		flushOrphanSubagentTool(true)
		emitDispatchFinish(true)
		o.emitEv(*errEv)
		o.persistEvent(*errEv)
		return
	}

	// subagent hoàn thành thành công → cập nhật dòng DISPATCH gốc thành trạng thái hoàn thành (kèm thời gian)
	if ev.Tool == "subagent" {
		flushOrphanSubagentTool(false)
		emitDispatchFinish(false)
		return
	}

	// Công cụ trực tiếp coordinator hoàn thành thành công
	emitToolFinish(false)
}

func (o *observer) subagentResultErrorEvent(ev agentcore.Event) (*Event, string) {
	if ev.Tool != "subagent" || len(ev.Result) == 0 {
		return nil, ""
	}
	sub := parseSubagentArgs(ev.Args)
	errMsg := parseSubagentResultError(ev.Result)
	if errMsg == "" {
		return nil, ""
	}

	target := "subagent"
	if sub.agent != "" {
		target = sub.agent
	}
	fullErr := fmt.Sprintf("%s thất bại: %s", target, errMsg)
	return &Event{
		Time:     time.Now(),
		Category: "ERROR",
		Agent:    "coordinator",
		Summary:  fmt.Sprintf("%s thất bại: %s", target, truncate(errMsg, 120)),
		Detail:   fullErr,
		Kind:     errorKind(nil, errMsg),
		Level:    "error",
	}, fullErr
}

func displayToolName(tool string, args json.RawMessage) string {
	if len(args) == 0 {
		return tool
	}
	switch tool {
	case "save_foundation":
		var p struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(args, &p) == nil && p.Type != "" {
			return fmt.Sprintf("%s[%s]", tool, p.Type)
		}
	case "commit_chapter", "plan_chapter", "draft_chapter", "check_consistency":
		var p struct {
			Chapter int `json:"chapter"`
		}
		if json.Unmarshal(args, &p) == nil && p.Chapter > 0 {
			return fmt.Sprintf("%s(chương%d)", tool, p.Chapter)
		}
	case "save_review":
		var p struct {
			Chapter int    `json:"chapter"`
			Scope   string `json:"scope"`
			Verdict string `json:"verdict"`
		}
		if json.Unmarshal(args, &p) == nil {
			label := ""
			switch p.Scope {
			case "arc":
				label = "cung này"
			case "global":
				label = "toàn cục"
			default:
				if p.Chapter > 0 {
					label = fmt.Sprintf("chương%d", p.Chapter)
				}
			}
			if label == "" {
				return tool
			}
			if p.Verdict != "" {
				return fmt.Sprintf("%s(%s·%s)", tool, label, p.Verdict)
			}
			return fmt.Sprintf("%s(%s)", tool, label)
		}
	case "novel_context":
		var p struct {
			Chapter int `json:"chapter"`
		}
		if json.Unmarshal(args, &p) == nil && p.Chapter > 0 {
			return fmt.Sprintf("%s(chương%d)", tool, p.Chapter)
		}
	case "read_chapter":
		var p struct {
			Chapter   int    `json:"chapter"`
			Source    string `json:"source"`
			Character string `json:"character"`
		}
		if json.Unmarshal(args, &p) == nil && p.Chapter > 0 {
			suffix := ""
			if p.Character != "" {
				suffix = "·" + p.Character + "hội thoại"
			} else if p.Source == "draft" {
				suffix = "·bản nháp"
			}
			return fmt.Sprintf("%s(chương%d%s)", tool, p.Chapter, suffix)
		}
	}
	return tool
}

type subagentInvocation struct {
	agent string
	task  string
}

func parseSubagentResultError(result json.RawMessage) string {
	if len(result) == 0 {
		return ""
	}
	// Lỗi chính: đối tượng {"error": "..."} (unknown agent / invalid model / subagent thực thi thất bại)
	var obj struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(result, &obj); err == nil && obj.Error != "" {
		return obj.Error
	}
	// Tương thích với trả về lỗi chuỗi thuần của agentcore SubAgentTool:
	// "Invalid parameters: ..." / "background mode requires ..." / "Too many parallel tasks ..."
	// Đây là kiểm tra tham số tầng tool thất bại, is_error=false nhưng nội dung là mô tả lỗi, cần nhận diện là lỗi để tránh nhầm thành công.
	var s string
	if json.Unmarshal(result, &s) == nil && isSubagentErrorString(s) {
		return s
	}
	return ""
}

var subagentErrorPrefixes = []string{
	"Invalid parameters",
	"background mode requires",
	"Too many parallel tasks",
}

func isSubagentErrorString(s string) bool {
	for _, p := range subagentErrorPrefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func parseSubagentArgs(args json.RawMessage) subagentInvocation {
	if len(args) == 0 {
		return subagentInvocation{}
	}
	var p struct {
		Agent string `json:"agent"`
		Task  string `json:"task"`
	}
	if json.Unmarshal(args, &p) == nil && p.Agent != "" {
		return subagentInvocation{agent: p.Agent, task: p.Task}
	}
	return subagentInvocation{}
}

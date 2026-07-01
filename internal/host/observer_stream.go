package host

import (
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/utils"
)

// handleSubagentDelta phân luồng văn bản và tham số gọi công cụ của subagent:
// - DeltaText trực tiếp xuất như markdown
// - DeltaToolCall chỉ trích trường và xuất cho các công cụ có nội dung dài đã biết (như draft_chapter.content); tham số JSON của các công cụ khác đều bị bỏ
func (o *observer) handleSubagentDelta(p *agentcore.ProgressPayload) {
	if p.DeltaKind != agentcore.DeltaToolCall {
		o.emitStreamDelta(p.Delta, false)
		return
	}
	if p.Tool == "" {
		return // Tên công cụ chưa sẵn sàng, thử lại delta tiếp theo
	}

	// Khi nhận diện tên công cụ qua luồng, phát sớm sự kiện TOOL đang tiến hành,
	// để spinner bao phủ toàn bộ giai đoạn LLM sinh
	// (nếu không, "đang tiến hành" của các công cụ như draft_chapter chỉ hiển thị trong vài chục ms của Execute thực sự).
	// Khi ProgressToolStart thực sự đến nơi, nhận ra toolStarts đã có record, chỉ bổ sung summary.
	o.ensureSubagentToolStarted(p.Agent, p.Tool)
	o.updateToolCallSummaryFromDelta(p.Agent, p.Tool, p.Delta)

	cur, ok := o.streamExtractors[p.Agent]
	// Sau khi args của cùng lần gọi công cụ đã đóng (hit } cấp đỉnh), vẫn có thể nhận được trailing delta:
	// Một số provider (thực đo với deepseek-v4-flash) chia args một lần thành nhiều chunk,
	// chunk cuối cùng sau `}` còn theo sau bởi khoảng trắng hoặc ký tự lặp. Lúc này nếu xử lý theo "tên công cụ khớp +
	// Done thì xây lại", extractor mới lại emit thêm một lần ✻ header và parse phần đuôi token như args mới.
	// Những delta này là đuôi thừa, bỏ đi là được.
	if ok && cur.tool == p.Tool && cur.ext.Done() {
		return
	}
	// Tên công cụ thay đổi hoặc chưa từng tạo: tạo mới.
	if !ok || cur.tool != p.Tool {
		ext := newToolExtractor(p.Tool)
		if ext == nil {
			delete(o.streamExtractors, p.Agent)
			return
		}
		cur = &agentExtractor{tool: p.Tool, ext: ext}
		o.streamExtractors[p.Agent] = cur
	}
	if emitted := cur.ext.Feed(p.Delta); emitted != "" {
		if !cur.emittedAny {
			cur.emittedAny = true
			// streamClear cho ✻ header của extractor rơi vào điểm bắt đầu round mới, phối hợp
			// kiểm tra HasPrefix("✻") của renderStreamContent để đi đường renderAgentBlock highlight;
			// dùng ensureStreamParagraphBreak chỉ chèn dòng trống không mở round, ✻ vẫn bị
			// thinking/chính văn phía trước bao bọc, rơi vào renderChapterBlock dùng màu mặc định vẽ mất.
			o.streamClear()
			// streamClear đã phòng thủ xóa sạch streamExtractors. cur hiện tại vẫn cần tiếp tục Feed
			// các delta tiếp theo của lần gọi công cụ này, phải lập tức đăng ký lại ngay;
			// nếu không khi delta đoạn tiếp theo đến sẽ tạo extractor mới, parse từ giữa chừng args
			// (vào psBeforeKey tại `{` của object lồng nhau), coi timeline_events.time / foreshadow_updates.id
			// v.v. như trường cấp đỉnh, TUI xuất hiện ✻ header lặp lại.
			o.streamExtractors[p.Agent] = cur
		}
		o.emitStreamDelta(emitted, false)
	}
}

func (o *observer) handleCoordinatorToolDelta(ev agentcore.Event) {
	msg, ok := ev.Message.(agentcore.Message)
	if !ok {
		return
	}
	call, ok := latestToolCall(msg)
	if !ok || call.Name == "" {
		return
	}
	if call.Name == "subagent" {
		o.ensureCoordinatorDispatchStarted(call)
		o.updateCoordinatorDispatchSummaryFromDelta(ev.Delta)
		return
	}
	o.ensureCoordinatorToolStarted(call.Name)
	o.updateToolCallSummaryFromDelta("coordinator", call.Name, ev.Delta)
}

func latestToolCall(msg agentcore.Message) (agentcore.ToolCall, bool) {
	calls := msg.ToolCalls()
	if len(calls) == 0 {
		return agentcore.ToolCall{}, false
	}
	return calls[len(calls)-1], true
}

func (o *observer) emitStreamDelta(delta string, thinking bool) {
	if delta == "" {
		return
	}
	if thinking != o.streamThinking {
		o.emitD(utils.ThinkingSep)
		o.streamThinking = thinking
	}
	o.emitD(delta)
	o.streamHasContent = true
	o.streamLastByte = delta[len(delta)-1]
}

// ensureSubagentToolStarted đăng ký sớm một lần gọi TOOL đang tiến hành cho agent khi lần đầu nhận diện tool_call
// qua luồng, để spinner của luồng sự kiện bao phủ giai đoạn "LLM sinh tham số tool_call theo luồng"
// (thường chiếm 99% tổng thời gian gọi). args lúc này chưa hoàn chỉnh, tạm dùng tên công cụ thuần làm
// summary; đến khi ProgressToolStart thực sự đến sẽ bổ sung summary kèm tham số.
func (o *observer) ensureSubagentToolStarted(agent, tool string) {
	if agent == "" || tool == "" {
		return
	}
	if _, ok := o.toolStarts[agent]; ok {
		return // Đã có lần gọi đang tiến hành, idempotent
	}
	o.resetStreamArgLabel(agent, tool)
	id := nextEventID()
	o.toolStarts[agent] = &activeCall{
		id:      id,
		start:   time.Now(),
		summary: tool, // Dùng tên công cụ thuần trước, khi ProgressToolStart đến có thể cập nhật thành tool(chương N)
		depth:   1,
	}
	o.emitAndLog(Event{
		ID:       id,
		Time:     time.Now(),
		Category: "TOOL",
		Agent:    agent,
		Summary:  tool,
		Level:    "info",
		Depth:    1,
	})
	o.updateAgent(agent, func(a *agentState) {
		a.state = "working"
		a.tool = tool
	})
	o.emitFallbackStreamHeader(tool)
}

func (o *observer) resetStreamArgLabel(agent, tool string) {
	key := streamArgKey(agent, tool)
	delete(o.streamArgPrefixes, key)
	delete(o.streamArgLabels, key)
}

// emitFallbackStreamHeader bổ sung một dòng ✻ tiêu đề vào bảng luồng cho các công cụ chưa cấu hình extractor.
// Ba đường dẫn đều phải gọi để đảm bảo nhất quán:
//  1. ensureSubagentToolStarted —— subagent tool args theo luồng (DeltaToolCall)
//  2. handleToolUpdate ProgressToolStart —— subagent tool args không theo luồng
//  3. handleToolStart —— công cụ của chính coordinator
//
// Thiếu bất kỳ đường nào, cùng một công cụ sẽ "gọi từ writer có ✻, gọi từ coordinator không có ✻" hoặc ngược lại.
func (o *observer) emitFallbackStreamHeader(tool string) {
	if _, has := toolDisplays[tool]; has {
		return // Đã có extractor, header do extractor tự xuất
	}
	o.streamClear()
	o.emitStreamDelta(streamHeaderFallback(tool)+"\n", false)
}

// streamHeaderFallback sinh văn bản header luồng cho các công cụ chưa cấu hình extractor,
// để người dùng có thể thấy "đang gọi cái gì" ngay cả với các công cụ đọc nhẹ.
//
// Tiền tố "✻ " là ký hiệu quy ước "khối agent dispatch" — renderStreamContent của TUI khi thấy
// tiền tố này sẽ đi đường renderAgentBlock để render (icon + label highlight + đường kẻ phân cách),
// nếu không sẽ rơi vào đường khối chính văn dùng màu mặc định terminal, header trông như văn bản thường không nổi bật.
func streamHeaderFallback(tool string) string {
	label := tool
	switch tool {
	case "ask_user":
		label = "Hỏi người dùng"
	}
	return "✻ " + label
}

// streamClear thông báo TUI bắt đầu một streamRound mới, đồng thời reset trạng thái liên quan đến ngăn cách đoạn.
// Về mặt logic round mới là "stream rỗng", nếu không lần emit đầu tiên của extractor tiếp theo sẽ bổ sung nhầm dòng trống đầu.
//
// streamThinking phải được reset cùng: emitStreamDelta dùng streamThinking để theo dõi liên gọi
// xem đoạn trước có phải thinking không. Trong round mới chưa xuất bất kỳ nội dung nào,
// lần emit(thinking=false) tiếp theo không nên chèn ThinkingSep nữa.
// Nếu không fallback header (như ✻ đọc chương) sẽ bị \x02 chiếm đầu,
// HasPrefix("✻") của renderStreamContent không khớp, toàn đoạn rơi vào đường chính văn
// rồi bị ThinkingSep cắt thành đoạn thinking, màu title bị vẽ thành màu thinking.
func (o *observer) streamClear() {
	o.emitC()
	o.streamHasContent = false
	o.streamLastByte = 0
	o.streamThinking = false
	// ProgressToolEnd của subagent vòng trước đã delete trước khi kết thúc, đây là phòng thủ xóa sạch.
	if len(o.streamExtractors) > 0 {
		o.streamExtractors = make(map[string]*agentExtractor)
	}
}

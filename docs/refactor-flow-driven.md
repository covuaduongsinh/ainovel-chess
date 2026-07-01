# Đề Xuất Tái Cấu Trúc: Hybrid Coordinator — Host Định Tuyến × LLM Phán Quyết

> Trạng thái: **Đã được chấp thuận và triển khai** (2026-04-20)
> Thời gian nghiên cứu: 2026-04-20
> Tài liệu hiện hành tương ứng: `docs/architecture.md` §2 / §3 / §7 / §8 / §13 đã được cập nhật đồng bộ
>
> **Đây là bản thảo thứ hai.** Vấn đề của bản thảo đầu tiên (phương án triệt để — xóa hoàn toàn Coordinator) được trình bày ở Phụ lục A, giữ lại để tránh đi lại vết xe đổ.
>
> Kết quả triển khai:
> - `internal/host/flow/` được tạo mới (router.go / state.go / dispatcher.go / router_test.go, 15 unit test phân nhánh đều pass)
> - `internal/host/reminder/` đã xóa `flow.go` / `queue_guard.go` / `book_complete.go`; giữ lại StopGuard và sub-agent Guard
> - `assets/prompts/coordinator.md` rút từ 88 dòng xuống ~45 dòng (trách nhiệm thu hẹp thành thực thi chỉ thị Host + phán quyết + chọn loại khởi động)
> - `internal/host/resume.go` đơn giản hóa đáng kể, chỉ tạo label và prompt ngắn gọn, bước tiếp theo cụ thể do Router phát sau TurnEnd đầu tiên
> - `internal/store/` bổ sung các phương thức hỗ trợ `HasArcReview` / `HasArcSummary` / `HasVolumeSummary` / `CheckConsistency`
> - Đã sửa bug `observer.go` khiến agent state bị kẹt ở trạng thái working

---

## 1. Bối Cảnh

### 1.1 Định vị dự án

```
agentcore       — Framework agent đa năng
litellm         — Gateway LLM đa năng
ainovel-cli     — Agent chuyên biệt sáng tác tiểu thuyết (dự án này)
```

Không gian ra quyết định của agent chuyên biệt là **khép kín**: sơ đồ quy trình cố định, các nhánh có giới hạn, được điều khiển bởi dữ liệu thực tế. Triết lý thiết kế của agent đa năng ("đặt cược vào năng lực mô hình") khi áp vào tình huống chuyên biệt có phần quá thuần túy.

### 1.2 Mục tiêu người dùng (theo thứ tự ưu tiên)

1. **Ổn định** — Viết liên tục không bị gián đoạn do lỗi định tuyến
2. **Thừa hưởng lợi ích từ nâng cấp LLM** — Kiến trúc không chống lại năng lực mô hình
3. **Tận dụng tốt khả năng multi-agent** — Phân công chức năng rõ ràng

Đề xuất này thực hiện **cải tiến Pareto** giữa ba mục tiêu (không đánh đổi mục tiêu nào lấy mục tiêu khác).

---

## 2. Khảo Sát Hiện Trạng

### 2.1 Phân loại các điểm quyết định của Coordinator

Trích xuất từng điểm quyết định trong `coordinator.md`:

| # | Điểm quyết định | Bản chất | Tần suất |
|---|---|---|---|
| 1 | Khởi động chọn architect_long / short | Phán quyết (hiểu ngữ nghĩa) | 1 lần/cuốn |
| 2 | Mở rộng đầu vào (<20 ký tự tự động bổ sung) | Phán quyết (sáng tạo) | 0-1 lần/cuốn |
| 3 | Vòng lặp hoàn thiện quy hoạch | Định tuyến (dữ liệu điều khiển) | 1-3 lần |
| 4 | Bước tiếp theo sau commit mỗi chương | **Định tuyến** | **1-2 lần/chương** |
| 5 | Thực hiện từng bước đánh giá cuối arc | Định tuyến | 3-5 lần/arc |
| 6 | Phân nhánh verdict đánh giá | Định tuyến (đã code hóa, xem §2.3) | 1 lần/arc |
| 7 | Xử lý can thiệp người dùng | Phán quyết (bắt buộc LLM) | Bất kỳ lúc nào |
| 8 | Tái phân công khi sub-agent báo lỗi | Định tuyến | Không thường xuyên |
| 9 | Xuất tóm tắt khi hoàn thành toàn bộ cuốn | Định tuyến | 1 lần |

**Kết luận**: Trong 9 điểm quyết định, 6 điểm là định tuyến thuần túy (tra bảng), 3 điểm mới thực sự cần LLM để phán quyết. **Tần suất định tuyến cao hơn rất nhiều so với phán quyết** (1-2 lần/chương vs vài lần/cuốn).

### 2.2 Kênh Reminder đã là code hóa quy trình nửa vời

Các generator trong `internal/host/reminder/` mỗi vòng tạo ra **chỉ thị cụ thể đến từng hành động** dựa trên dữ liệu thực tế:

- `flow.go` → `"flow hiện tại=writing, next_chapter=37. Hãy gọi ngay subagent(writer, \"Viết chương 37\")..."`
- `queue_guard.go` → `"flow hiện tại=rewriting, hàng đợi chờ xử lý: [3,5]. Hãy gọi writer ngay để viết lại từng chương..."`
- `book_complete.go` → `"Toàn bộ cuốn đã hoàn thành. Hãy xuất tóm tắt toàn cuốn..."`

**Kiến trúc hiện tại có vấn đề double dispatch**:
```
Tầng quy tắc: coordinator.md định nghĩa "nếu A thì B"
  ↓
Tầng Reminder: Mỗi vòng cụ thể hóa quy tắc dựa trên dữ liệu → tạo ra "bây giờ hãy làm B"
  ↓
Tầng LLM: Đọc reminder tạo tool_call (về cơ bản chỉ là nhắc lại reminder)
  ↓
SubAgent thực thi
```

**LLM thực chất chỉ "thực thi" chỉ thị mà Reminder đã đưa ra**. Bước trung gian này vừa tốn tokens, vừa tạo ra không chắc chắn (LLM có thể không tuân thủ hoàn toàn reminder, ví dụ lỗi định tuyến mid đã được quan sát).

### 2.3 Tầng công cụ đã đảm nhận phần lớn việc phán đoán

- `save_review.evaluateScorecardGate()`: Cổng kiểm tra thẻ điểm, tự động nâng cấp accept thành polish/rewrite
- `save_review.ContractStatus` kiểm tra: contract=missed tự động nâng cấp thành rewrite
- `commit_chapter.CheckArcBoundary()`: Tính ngay lập tức `arc_end / needs_expansion / needs_new_volume`
- `commit_chapter.applyCompletion()`: Phán định ngay lập tức `book_complete`
- `CommitResult` trả về 17 trường dữ liệu thực tế

**Kết luận**: Tầng công cụ đã code hóa phần lớn "phán đoán", các quyết định Coordinator đưa ra từ dữ liệu này về cơ bản chỉ là if-else.

### 2.4 Chi phí thực tế của hiện trạng

Số vòng LLM của Coordinator mỗi chương:
- **1-2 turns/chương** (đọc system prompt ~3000 tokens + reminder ~200 tokens + lịch sử + CommitResult ~500 tokens → tạo tool_call ~50 tokens)
- Truyện dài 200 chương khoảng **200-400 turns** gọi Coordinator LLM
- Trong đó **~90% là định tuyến thuần túy** (LLM nhắc lại reminder), **~10% là phán quyết**

**~3500-7000 tokens/chương dùng cho quyết định của Coordinator, 95% là dư thừa** (Reminder đã tính ra câu trả lời).

---

## 3. Phương Án Thiết Kế: Hybrid Coordinator

### 3.1 Ý tưởng cốt lõi

**Chuyển quyết định quy trình từ LLM sang Host, nhưng giữ lại Coordinator làm nút phán quyết và kênh thực thi chỉ thị**.

```
┌──────────────────────────────────────────────────────────┐
│                   Entry (TUI / headless)                   │
└────────────────────────────────┬─────────────────────────┘
                                 │ Start / Resume / Steer
┌────────────────────────────────▼─────────────────────────┐
│                            Host                            │
│                                                             │
│   ┌──────────────────────────────────────────────────┐     │
│   │  Flow Router (bổ sung cốt lõi)                    │     │
│   │  ───────────                                      │     │
│   │  Đăng ký sự kiện Coordinator: kích hoạt khi      │     │
│   │  sub-agent tool trả về                            │     │
│   │  Hàm thuần túy: route(Progress, Checkpoint,      │     │
│   │      Boundary) → NextInstruction                  │     │
│   │  Có chỉ thị → coordinator.FollowUp(chỉ thị)      │     │
│   │  Không có chỉ thị (tình huống phán quyết) →      │     │
│   │      không can thiệp, để LLM tự chủ              │     │
│   └──────────────────────────────────────────────────┘     │
│                                                             │
│   Giữ lại: Lifecycle API / Observer / Usage Tracker        │
│   Giữ lại: resume.go (đơn giản hóa, không đổi logic cốt)  │
└────────────────────────────────┬─────────────────────────┘
                                 │
┌────────────────────────────────▼─────────────────────────┐
│                    Coordinator Agent (LLM)                  │
│                                                             │
│   Trách nhiệm thu hẹp về hai loại:                         │
│   1. Nhận chỉ thị FollowUp từ Host → tạo tool_call         │
│      tương ứng                                              │
│   2. Tự chủ phán quyết khi người dùng Steer đến            │
│      (truy vấn/sửa đổi đánh giá)                           │
│                                                             │
│   coordinator.md: 88 dòng → ~25 dòng                       │
│   MaxTurns: Giữ 1000 (xử lý steer từ người dùng +          │
│   thực thi chỉ thị Host)                                    │
└────────────────────────────────┬─────────────────────────┘
                                 │
                                 ▼
         ┌──────────────────────┼───────────────────────┐
         ▼                      ▼                       ▼
    ┌────────┐             ┌────────┐             ┌────────┐
    │Architect│             │ Writer │             │ Editor │
    └────────┘             └────────┘             └────────┘
```

### 3.2 Phân chia trách nhiệm lại

| Tầng | Làm gì | Không làm gì |
|---|---|---|
| **Host / Flow Router** | Đọc dữ liệu → định tuyến hàm thuần túy → chỉ thị FollowUp | Tự gọi SubAgent (vẫn thông qua Coordinator) |
| **Coordinator** | Thực thi chỉ thị Host + phán quyết can thiệp người dùng + chọn planner lúc khởi động | Tự quyết "bước tiếp theo làm gì" |
| **SubAgent (A/W/E)** | Công việc chuyên môn của từng agent | Không thay đổi |
| **Tầng công cụ** | Ghi nguyên tử + trả về dữ liệu thực tế | Không thay đổi |

**Tính bất biến quan trọng**:
- ✅ Coordinator vẫn là một agent run liên tục, giữ "nhận thức xuyên suốt toàn cuốn"
- ✅ Người dùng Steer vẫn qua `coordinator.Inject()`, năng lực ngắt tức thì được giữ nguyên
- ✅ SubAgentTool vẫn do LLM gọi (đi theo đường gốc của agentcore), luồng sự kiện / ContextManager / chuyển đổi mô hình đều không đổi
- ✅ agentcore không cần sửa đổi

### 3.3 Logic cụ thể của Flow Router

```go
// internal/host/flow/router.go

type NextInstruction struct {
    Agent  string   // architect_long / architect_short / writer / editor
    Task   string   // Mô tả nhiệm vụ cho sub-agent
    Reason string   // Lý do cho Coordinator xem (tùy chọn, tiện debug)
}

type RouterState struct {
    Progress        *domain.Progress
    LatestCheckpoint *domain.Checkpoint
    // Ranh giới arc trong chế độ phân tầng (tính khi chương cuối hoàn thành)
    LastCompleted   int
    ArcBoundary     *store.ArcBoundary
    HasArcReview    bool
    HasArcSummary   bool
    // Các mục cơ sở bị thiếu
    FoundationMissing []string
}

// Route trả về chỉ thị bước tiếp theo. Trả về nil nghĩa là để Coordinator tự phán quyết (tình huống phán quyết).
func Route(s RouterState) *NextInstruction {
    p := s.Progress

    // 0. Trạng thái kết thúc: để LLM xuất tóm tắt, không định tuyến
    if p.Phase == domain.PhaseComplete {
        return nil
    }

    // 1. Giai đoạn quy hoạch: phán quyết (chọn planner) do LLM, không định tuyến
    if p.Phase != domain.PhaseWriting {
        return nil
    }

    // 2. Giai đoạn viết
    // 2a. Hàng đợi viết lại/đánh bóng ưu tiên trước
    if len(p.PendingRewrites) > 0 {
        ch := p.PendingRewrites[0]
        verb := "Viết lại"
        if p.Flow == domain.FlowPolishing {
            verb = "Trau chuốt"
        }
        return &NextInstruction{
            Agent:  "writer",
            Task:   fmt.Sprintf("%s chương %d", verb, ch),
            Reason: fmt.Sprintf("Hàng đợi PendingRewrites còn %d chương", len(p.PendingRewrites)),
        }
    }

    // 2b. Đang đánh giá: không định tuyến, để Coordinator xử lý phân nhánh verdict của save_review
    if p.Flow == domain.FlowReviewing {
        return nil
    }

    // 2c. Xử lý hậu kỳ cuối arc trong chế độ phân tầng
    if p.Layered && s.ArcBoundary != nil && s.ArcBoundary.IsArcEnd {
        b := s.ArcBoundary
        if !s.HasArcReview {
            return &NextInstruction{
                Agent:  "editor",
                Task:   fmt.Sprintf("Thẩm định cấp cung cho cung %d của tập %d (scope=arc)", b.Arc, b.Volume),
                Reason: "Thẩm định cuối cung chưa hoàn thành",
            }
        }
        if !s.HasArcSummary {
            return &NextInstruction{
                Agent:  "editor",
                Task:   fmt.Sprintf("Sinh tóm tắt cung %d của tập %d (save_arc_summary)", b.Arc, b.Volume),
                Reason: "Tóm tắt cung chưa hoàn thành",
            }
        }
        if b.NeedsExpansion {
            return &NextInstruction{
                Agent:  "architect_long",
                Task:   fmt.Sprintf("Mở cung %d của tập %d (save_foundation type=expand_arc)", b.NextArc, b.NextVolume),
                Reason: "Cung kế tiếp dạng bộ khung chờ mở",
            }
        }
        if b.NeedsNewVolume {
            return &NextInstruction{
                Agent:  "architect_long",
                Task:   "Đánh giá rồi gọi save_foundation type=append_volume (viết tiếp) hoặc type=complete_book (kết thúc toàn sách)",
                Reason: "Cuối tập cần quyết định thêm tập mới hay kết thúc toàn sách",
            }
        }
    }

    // 2d. Tiếp tục viết bình thường
    next := p.NextChapter()
    return &NextInstruction{
        Agent:  "writer",
        Task:   fmt.Sprintf("Viết chương %d", next),
        Reason: "Viết tiếp",
    }
}
```

**Đặc điểm của hàm**:
- Hàm thuần túy (đầu vào RouterState, đầu ra NextInstruction)
- Có thể unit test (cho trạng thái xác định, kiểm định kết quả định tuyến)
- **Trả về nil là hợp lệ** — nghĩa là "đây là tình huống phán quyết, hãy để LLM tự chủ"

### 3.4 Thời điểm kích hoạt

Host đăng ký sự kiện `agentcore.EventToolExecEnd`:

```go
coordinator.Subscribe(func(ev agentcore.Event) {
    if ev.Type == agentcore.EventToolExecEnd && ev.Tool == "subagent" && !ev.IsError {
        // SubAgent vừa trả về → đọc trạng thái mới nhất → định tuyến
        h.flowRouter.Dispatch()
    }
})
```

```go
func (r *FlowRouter) Dispatch() {
    state := r.loadState()
    instruction := Route(state)
    if instruction == nil {
        return // Tình huống phán quyết, để LLM tự chủ
    }
    msg := formatInstruction(instruction)
    _ = r.coordinator.FollowUp(agentcore.UserMsg(msg))
}

func formatInstruction(i *NextInstruction) string {
    return fmt.Sprintf(
        "[Host ra lệnh] Bước tiếp theo: gọi subagent(%s, %q)\n"+
        "Lý do: %s\n"+
        "Đây là lệnh tường minh ở tầng luồng, hãy thực thi ngay; đừng gọi novel_context trước, đừng xuất suy luận trước.",
        i.Agent, i.Task, i.Reason,
    )
}
```

### 3.5 Tính phản hồi và đồng thời

**Đường xử lý Steer của người dùng** (không thay đổi):
```
Steer → coordinator.Inject(UserMsg("[Người dùng can thiệp] xxx"))
```

- Đang chạy: Tin nhắn được chèn vào hàng đợi run hiện tại
- Idle: Resume run
- Đã tạm dừng: Xếp vào hàng chờ

**Đồng thời giữa chỉ thị định tuyến và Steer**:
- Cả hai đều vào hàng đợi tin nhắn của Coordinator, được xử lý theo thứ tự gốc của agentcore
- Nếu Host vừa gửi `FollowUp("[Host ra lệnh] Viết chương 37")`, ngay sau đó người dùng Steer `"Dừng lại, điều chỉnh văn phong"`
  - Coordinator xử lý chỉ thị Host trước hay Steer trước?
  - **Ngữ nghĩa của `Inject` là chen vào đầu hàng đợi hiện tại**, nên Steer được xử lý trước
  - Đây là hành vi mong muốn: can thiệp của người dùng có độ ưu tiên cao hơn lịch trình thông thường của Host

**Tránh xung đột giữa chỉ thị Host và Steer**:
- Flow Router **tạm dừng ngắn** vài turn sau khi nhận tín hiệu "Steer đã được chèn" (để Coordinator xử lý xong Steer rồi mới định tuyến)
- Phát hiện kết quả xử lý Steer thông qua đăng ký `agentcore.EventMessageEnd` và kiểm tra thay đổi trạng thái Progress

### 3.6 Ví dụ đơn giản hóa coordinator.md

Rút từ 88 dòng xuống khoảng 25 dòng:

```markdown
Bạn là tổng điều phối sáng tác tiểu thuyết.

## Cách thức làm việc

**Mạch chính**: Sau mỗi lần sub-agent trả về, Host sẽ gửi tin nhắn `[Host ra lệnh]` cho bạn biết bước tiếp theo gọi sub-agent nào để làm gì. Nhận được chỉ thị là lập tức tạo tool_call tương ứng, không gọi novel_context để suy luận trước, không nhắc lại nội dung.

**Phán quyết**: Trong các tình huống sau bạn cần tự chủ phán đoán (Host không gửi chỉ thị, bạn phải chủ động hành động):

### Lúc khởi động: Chọn planner

- Mặc định → `architect_long`
- Chỉ khi người dùng yêu cầu rõ ràng truyện ngắn/đơn tập/tác phẩm nhỏ và giới hạn trong 25 chương → `architect_short`

Nếu đầu vào người dùng < 20 ký tự, hãy bổ sung hướng đặc trưng hóa, đối tượng độc giả mục tiêu, ít nhất một móc câu chuyện khác thường vào phần mô tả task rồi mới phân công.

### Steer của người dùng

Định dạng: `[Người dùng can thiệp] xxx`

- **Loại truy vấn** (hỏi trạng thái/thiết định): Trả lời văn bản trực tiếp, không cần gọi thêm công cụ; Host sẽ tiếp tục phân công.
- **Loại sửa đổi** (yêu cầu thay đổi thiết định/viết lại/điều chỉnh văn phong): Đánh giá phạm vi ảnh hưởng:
  - Liên quan thay đổi thiết định → Gọi architect_* thực hiện `save_foundation(type=...)`
  - Liên quan chương đã viết → Để công cụ tự động đưa chương mục tiêu vào `PendingRewrites` (có thể nêu rõ ý định viết lại khi gọi writer lần tiếp)
  - Chỉ ảnh hưởng văn phong tương lai → Mô tả ngắn gọn yêu cầu, đính kèm vào mô tả task của writer khi nhận chỉ thị Host tiếp theo

## Công cụ

- `subagent(agent, task)`: Gọi sub-agent
- `novel_context`: Chỉ dùng khi truy vấn của người dùng cần, không gọi sau khi nhận chỉ thị Host

## Sub-agent

- `architect_long` / `architect_short` / `writer` / `editor`

## Cấm

- Gọi novel_context trước khi hành động khi có chỉ thị Host
- Tự quyết bước tiếp theo khi không có Steer của người dùng và không có chỉ thị Host
```

### 3.7 Kênh Reminder thu hẹp đáng kể

**Xóa bỏ**:
- `flow.go` (Host FollowUp đã gửi chỉ thị cụ thể, nhắc nhở định tuyến của Reminder mất giá trị)
- `queue_guard.go` (hàng đợi đã được Host Router đảm bảo)
- `book_complete.go` (Host FollowUp chỉ thị xuất tóm tắt khi Phase=Complete)

**Giữ lại**:
- `subagent_guards.go` (StopGuard của Writer/Architect/Editor, đảm bảo sub-agent không kết thúc tay trắng)
- Thêm một `foundation_reminder.go` nhẹ: Thông báo cho Coordinator về các mục thiếu trong giai đoạn quy hoạch (đây là **thông tin cần thiết cho phán quyết**, không phải chỉ thị định tuyến)

**Giữ lại StopGuard**:
- StopGuard của Coordinator được giữ lại (chặn end_turn khi `Phase != Complete` như phương án dự phòng)
- Thêm nhắc nhở khi "nhận được chỉ thị Host nhưng vòng này chưa gọi subagent tương ứng"

### 3.8 resume.go đơn giản hóa nhỏ

`buildResumePrompt` hiện tại tạo chỉ thị ngôn ngữ tự nhiên chính xác đến từng step dựa trên checkpoint (121 dòng).

Kiến trúc mới:
- Khi Resume, đọc Progress trước, Flow Router tính `NextInstruction`
- Coordinator nhận một **prompt resume rất ngắn**, sau đó chờ chỉ thị FollowUp từ Host

```
[Khôi phục] Sách này 「xxx」đã hoàn thành N chương, đang ở giai đoạn XX.
Hãy chờ chỉ thị [Host ra lệnh] tiếp theo, hoặc xử lý can thiệp người dùng có thể đã để lại trong lúc dừng.
```

Hầu hết logic phân nhánh được đưa xuống Flow Router (Router vốn đã định tuyến theo trạng thái, Resume không cần đường riêng).

---

## 4. Đánh Giá Mức Độ Đạt Mục Tiêu

### 4.1 Ổn định

| Rủi ro | Hiện tại | Kiến trúc mới |
|---|---|---|
| Coordinator chọn sai architect | Đã xảy ra (lỗi định tuyến mid) | Lúc khởi động vẫn là phán quyết, nhưng prompt từ ba lựa chọn rút thành nhị phân (đã làm), diện tích lỗi thu hẹp đáng kể |
| Coordinator không tuân theo "chỉ nói viết chương N" | Đã xảy ra | Host gửi chỉ thị định dạng cố định, không cần LLM tạo mô tả task nữa |
| Coordinator bỏ sót kiểm tra queue_drained | Đã xảy ra | Host Router đảm bảo theo thứ tự bắt buộc |
| Coordinator quên gọi editor sau commit cuối arc | Có thể xảy ra | Host Router phát hiện IsArcEnd && !HasArcReview rồi phân công trực tiếp |
| Nhánh khôi phục sau crash bị bỏ sót | Đã biết | State machine của Flow Router tự nhiên bao phủ tất cả nhánh |
| StopGuard liên tiếp chặn 5 lần nâng cấp fatal | Tồn tại | Khi chỉ thị Host rõ ràng, LLM khó liên tiếp bị chặn (trừ khi prompt hỏng hoàn toàn) |

### 4.2 Lợi ích từ nâng cấp LLM

| Chiều | Mức giữ lại |
|---|---|
| Nâng cấp mô hình Writer → Chất lượng viết | 100% |
| Nâng cấp mô hình Editor → Đánh giá chính xác hơn | 100% |
| Nâng cấp mô hình Architect → Quy hoạch tinh tế hơn | 100% |
| **Nâng cấp mô hình Coordinator → Phán quyết chính xác hơn** | **100%** (tình huống phán quyết được giữ lại) |
| ~~Nâng cấp mô hình Coordinator → Định tuyến chính xác hơn~~ | Từ bỏ (tỷ lệ lỗi định tuyến lẽ ra phải là 0, không cần LLM thông minh hơn) |

**Điểm quan trọng được giữ lại**: Các tình huống phán quyết như đánh giá can thiệp người dùng, chọn loại planner, phán xét ranh giới verdict vẫn do LLM xử lý, được hưởng lợi trực tiếp khi nâng cấp mô hình.

### 4.3 Năng lực multi-agent

- Số lượng SubAgent, chức năng, cách lắp ghép **hoàn toàn không đổi**
- Cấu hình mô hình dị loại (coordinator/architect/writer/editor độc lập cấu hình) **hoàn toàn không đổi**
- Coordinator vẫn là run liên tục, giữ "góc nhìn toàn cuốn"
- Phương tiện cộng tác (các sản phẩm trong Store) không đổi

### 4.4 Tính phản hồi

- Năng lực ngắt bằng `coordinator.Inject` của Steer người dùng **được giữ nguyên hoàn toàn**
- Host Router phát chỉ thị khi SubAgent trả về, đi cùng kênh tin nhắn với Steer của người dùng
- Độ ưu tiên của Inject cao hơn FollowUp (ngữ nghĩa `Inject` là chen hàng), Steer không bị chỉ thị Host lấn át

### 4.5 Chi phí token

Hiện tại mỗi chương: Coordinator ~3500-7000 tokens × 1-2 turns = 3500-14000 tokens

Kiến trúc mới mỗi chương:
- Prompt Coordinator rút từ ~3000 tokens xuống ~800 tokens
- Mỗi chương vẫn cần 1 turn (Coordinator đọc chỉ thị FollowUp + tạo tool_call)
- Tổng cộng ~1000-1500 tokens

**Tiết kiệm 60-80%**. Truyện dài 200 chương tiết kiệm khoảng 400k-1M tokens (không bằng phương án triệt để 100%, nhưng không đánh đổi tính phản hồi và góc nhìn toàn cuốn).

---

## 5. Tác Động Đến docs/architecture.md

### 5.1 Điều chỉnh §2 Nguyên tắc cốt lõi

**Nguyên tắc một** (Vòng lặp chính điều khiển bởi LLM) → Điều chỉnh thành:
```
LLM điều khiển sáng tác và phán quyết, Host điều khiển định tuyến quy trình.

- Sáng tác và phán quyết (các quyết định cần hiểu ngữ nghĩa, đánh giá chất lượng,
  nhận dạng ý định) vẫn giao cho LLM
- Định tuyến quy trình (đọc dữ liệu → tra bảng → gửi chỉ thị) do code Host đảm nhận
- Host không bỏ qua Coordinator để gọi thẳng SubAgent, mà gửi chỉ thị rõ ràng
  qua FollowUp, giữ Coordinator làm kênh thực thi chỉ thị và nút phán quyết
```

**Nguyên tắc hai** (Đặt cược vào năng lực mô hình, không đặt cược vào hard-code) → Điều chỉnh thành:
```
Đặt cược vào mô hình ở chiều sáng tác và phán quyết
(năng lực phán quyết của Writer/Editor/Architect/Coordinator),
dùng code để biểu đạt ở chiều định tuyến quy trình
(không gian quyết định của agent chuyên biệt là khép kín, task tra bảng không có
lợi ích gì từ LLM thông minh hơn).
```

### 5.2 Điều chỉnh §13 Danh sách cấm

- §13.13 "Không làm Host đọc file tín hiệu → inject chỉ thị bước tiếp theo như mặt điều khiển xác định" →
  **Sửa diễn đạt**: "Không dùng file tín hiệu làm IPC (đọc trực tiếp Progress + Checkpoint là đủ), Host đọc dữ liệu thực tế rồi gửi chỉ thị gọi sub-agent cụ thể qua `coordinator.FollowUp` là định tuyến chuyên biệt hợp lý"
- §13.14 "Không hard-code state machine chuyển đổi Flow" →
  **Sửa diễn đạt**: "Nhãn Flow vẫn chỉ được cập nhật bởi công cụ (không viết 'nếu A thì SetFlow(B)' trong Host), nhưng Flow Router có thể dựa trên Flow và các dữ liệu thực tế khác để quyết định bước tiếp theo gọi ai"

### 5.3 Điều chỉnh §7 Lắp ghép Agent

- Giữ lại lắp ghép Coordinator
- `coordinator.md` rút từ 88 dòng xuống ~25 dòng
- Kênh Reminder thu hẹp (xóa flow/queue_guard/book_complete, giữ foundation/subagent_guards)
- Thêm package `internal/host/flow/`

---

## 6. Điểm Yếu Đã Biết (Liệt Kê Trung Thực)

### 6.1 Sự phát triển dài hạn của Flow Router

- Khi thêm các tình huống mới (trạng thái flow mới, xử lý hậu kỳ arc mới), switch-case của Router sẽ dài ra
- Cần ràng buộc nghiêm: **chỉ xử lý định tuyến, không xử lý logic nghiệp vụ**; quy tắc quyết định phải viết unit test
- Cảnh báo tương tự `handleSubAgentDone` v0.0.1 luôn có giá trị; nhưng phương án này tránh trượt thành God Object bằng "hàm thuần túy + unit test + chỉ gọi dữ liệu thuần túy"

### 6.2 Sự phức tạp của can thiệp người dùng

- Thiết kế hiện tại giao Steer hoàn toàn cho phán quyết LLM của Coordinator
- Nhưng một số Steer trải dài nhiều loại (ví dụ "sửa rõ nhân vật A trong vài chương đầu + sau đó thêm nhánh phụ cho anh ta")
- Cần dựa vào năng lực LLM để phân tách, prompt cần hướng dẫn rõ ràng
- **Phần này được hưởng lợi trực tiếp từ nâng cấp mô hình** (so với hard-code phân loại enum của InterventionAgent, LLM phán quyết linh hoạt hơn phù hợp với tình huống thực tế)

### 6.3 Phụ thuộc tiền điều kiện tính nhất quán tầng dữ liệu

- Router ra quyết định dựa trên Progress + Checkpoint, tầng dữ liệu phải đáng tin cậy
- `withWriteLock` hiện tại đóng gói tốt, ba bước của commit_chapter hoàn thành nguyên tử
- Nhưng nếu tầng dữ liệu mất nhất quán (ví dụ Progress nói chương 3 đã xong nhưng thư mục chapters/ không có), Router sẽ ra quyết định sai
- Đề xuất: Thêm một lần **kiểm tra tính nhất quán tầng dữ liệu** lúc khởi động (nếu phát hiện Progress.CompletedChapters không khớp với thư mục chapters/, báo warning)

### 6.4 Coordinator vẫn còn khả năng định tuyến bằng LLM

- Dù chỉ thị rõ ràng, LLM có thể "sáng tạo" mà không thực thi (ví dụ tạo ra đoạn suy nghĩ trước rồi mới gọi công cụ)
- StopGuard dự phòng: Nhận được chỉ thị Host nhưng vòng này chưa gọi subagent thì inject nhắc nhở
- Đây là dự phòng, không phải cấm đoán — mô hình mạnh thỉnh thoảng "suy nghĩ thêm một bước" cũng không phải điều xấu

### 6.5 Yêu cầu độ bao phủ test tăng cao

- Flow Router là hàm thuần túy, phải có unit test đầy đủ (bao phủ tất cả tổ hợp Phase × Flow × Boundary)
- Integration test: Mô phỏng chuỗi hoàn chỉnh "commit → router → FollowUp → coordinator phản hồi → subagent"
- Test khôi phục crash: Kill tiến trình rồi resume, kiểm định Router suy ra đúng bước tiếp theo

---

## 7. Lộ Trình Triển Khai

### Giai đoạn 1: Tăng cường tầng dữ liệu (~0.5 ngày)

- Bổ sung kiểm tra nhất quán của §6.3: Quét một lần khi khởi động/Resume, tạo warning
- Đảm bảo API `store.HasArcReview(vol, arc)` và `HasArcSummary(vol, arc)` sẵn dùng (thêm nếu chưa có)

### Giai đoạn 2: Giới thiệu khung Flow Router (~1 ngày)

- Tạo package `internal/host/flow/`:
  - `route.go` — Hàm thuần túy `Route(state) → *NextInstruction`
  - `dispatcher.go` — Đăng ký sự kiện + FollowUp gửi chỉ thị
  - `route_test.go` — Unit test bao phủ tất cả nhánh
- Kiểm soát có kích hoạt hay không qua config `flow_driven: true/false`
- Mặc định tắt (false), chạy đối chiếu trước

### Giai đoạn 3: Kích hoạt và xác minh (~1 ngày)

- Bật `flow_driven: true`
- Chạy một cuốn 30-50 chương, so sánh chỉ số:
  - Số lần gọi Coordinator LLM
  - Số lỗi định tuyến (phải là 0)
  - Tính phản hồi (steer ngắt có bình thường không)
- Sửa bug, điều chỉnh quy tắc Router

### Giai đoạn 4: Đơn giản hóa coordinator.md + Thu gọn Reminder (~0.5 ngày)

- Sửa coordinator.md theo §3.6
- Xóa `reminder/flow.go / queue_guard.go / book_complete.go`
- Giữ lại foundation reminder cần thiết
- Cập nhật StopGuard subagent nếu cần (thường không cần)

### Giai đoạn 5: Đơn giản hóa resume.go (~0.5 ngày)

- Xóa phần lớn nhánh của `buildResumePrompt`
- Thay bằng tin nhắn tổng quát ngắn gọn "[Khôi phục] Hãy chờ chỉ thị [Host ra lệnh]"
- Sau Resume, Router tự nhiên suy ra hành động tiếp tục

### Giai đoạn 6: Cập nhật tài liệu kiến trúc (~0.5 ngày)

- Sửa `docs/architecture.md` §2 / §13 / §7 theo §5
- Đổi trạng thái tài liệu đề xuất này thành "Đã chấp thuận", lưu trữ vào `docs/history/`

### Giai đoạn 7: Giai đoạn quan sát (2-4 tuần)

- Chạy liên tục 2-3 cuốn truyện dài (mỗi cuốn 100+ chương)
- Ghi lại tất cả lỗi định tuyến (nếu có), vấn đề phản hồi, hành vi bất thường của Coordinator
- Tinh chỉnh quy tắc Router và coordinator.md dựa trên quan sát

**Tổng cộng khoảng 4 ngày triển khai + giai đoạn quan sát**.

---

## 8. Bảng So Sánh

| Chiều | Kiến trúc hiện tại | Hybrid (phương án này) | Phương án triệt để (Phụ lục A) |
|---|---|---|---|
| Ổn định | Trung bình (LLM thỉnh thoảng định tuyến sai) | **Cao** | Cao |
| Tính phản hồi | Cao | **Cao** | **Thấp** (Host gọi thẳng SubAgent không thể ngắt) |
| Lợi ích LLM | 100% | **100%** | 85% (từ bỏ chiều định tuyến) |
| Tiết kiệm token | 0 | ~70% | ~95% |
| Góc nhìn toàn cuốn | Có | **Có** | Không (mỗi SubAgent run độc lập) |
| Chi phí triển khai | - | Trung bình (~4 ngày) | Cao (~1 tuần + sửa agentcore) |
| Cập nhật tài liệu | - | Nhỏ (§2/§13 tinh chỉnh) | Lớn (viết lại §2 nguyên tắc) |
| Cần sửa agentcore | - | Không | Có thể (gọi thẳng SubAgent) |
| Khó khăn rollback | - | Thấp (config switch) | Cao |

---

## 9. Điểm Quyết Định

1. **Có chấp nhận đề xuất này (Hybrid Coordinator) không?** [ ] Chấp nhận · [ ] Chấp nhận sau khi sửa · [ ] Không chấp nhận
2. Giai đoạn 3 có làm PR độc lập để xác minh trước không? [ ]
3. Điều chỉnh §2 / §13 của `docs/architecture.md` có xử lý cùng lần này không? [ ]
4. Độ dài giai đoạn quan sát: [ ] 2 tuần · [ ] 4 tuần · [ ] Dài hơn

---

## Phụ Lục A: Phương Án Triệt Để Đã Đánh Giá (Xóa Hoàn Toàn Coordinator)

> Phương án bản thảo đầu tiên. Bị hạ cấp xuống tham khảo vì tính phản hồi thụt lùi, khả thi kỹ thuật còn nghi ngờ, mất góc nhìn toàn cuốn của Coordinator.

Cốt lõi phương án triệt để: Host gọi trực tiếp `SubAgentTool.Execute`, không qua Coordinator LLM.

**Các vấn đề đã xác định**:

1. **Tính phản hồi thụt lùi**: `SubAgentTool.Execute` là lời gọi đồng bộ blocking, Steer của người dùng phải đợi SubAgent hiện tại trả về mới được xử lý. Kiến trúc hiện tại `Inject` có thể ngắt tức thì.
2. **Khả thi kỹ thuật còn nghi ngờ**:
   - Host gọi trực tiếp SubAgentTool vi phạm quy ước sử dụng agentcore
   - Luồng sự kiện (`Subscribe` Event) có thể không bubble đúng cách đến observer
   - Đường callback `ContextManagerFactory` / `OnMessage` của SubAgent chưa rõ
   - Cần sửa agentcore hoặc sửa lớn observer
3. **Mất góc nhìn toàn cuốn của Coordinator**: Mỗi SubAgent run độc lập, không có "người canh gác LLM liên tục". Trong chạy dài, các vấn đề như trượt văn phong, đứt gãy nhân vật mất đi một lớp bảo vệ vô hình.
4. **InterventionAgent đơn giản hóa quá mức**: Phương án triệt để dùng enum (query/modify_setting/rewrite_chapters/adjust_style/noop) để phân loại ý định người dùng, Steer thực tế có thể trải dài nhiều loại, schema ép buộc sẽ phân loại sai.
5. **Khối lượng viết lại tài liệu kiến trúc lớn**: §2 nguyên tắc cốt lõi bị lật, 30% luận điểm trong tài liệu bị ảnh hưởng.
6. **FlowDriver sẽ phình thành God Object**: Một vòng lặp nhồi tất cả logic định tuyến, mỗi khi thêm tình huống phải sửa, đồng cấu với `handleSubAgentDone` v0.0.1.

Phương án Hybrid tránh được 4 vấn đề đầu, vấn đề thứ 5 giảm xuống thành tinh chỉnh, vấn đề thứ 6 được kiểm soát bằng "hàm thuần túy + unit test".

---

## Phụ Lục B: Chi Tiết Vị Trí Các Điểm Quyết Định

| Điểm quyết định | Vị trí hiện tại | Vị trí kiến trúc mới | Loại |
|---|---|---|---|
| Chọn planner | coordinator.md L26-29 | Coordinator LLM phán quyết (lúc khởi động) | Phán quyết |
| Mở rộng đầu vào | coordinator.md L31 | Coordinator LLM phán quyết (lúc khởi động) | Phán quyết |
| Vòng lặp hoàn thiện quy hoạch | coordinator.md L36-38 | Host Router nhánh Phase=Premise/Outline (trả nil để LLM tự chủ hoặc FollowUp architect rõ ràng) | Hỗn hợp |
| Bước tiếp theo mỗi chương | coordinator.md L46-51 + reminder/flow | **Host Router nhánh 2d** (FollowUp writer) | Định tuyến |
| Đánh giá cuối arc | coordinator.md L78-82 | **Host Router nhánh 2c** (FollowUp editor/architect) | Định tuyến |
| Phân nhánh verdict | coordinator.md L59-61 + công cụ save_review | Tầng công cụ đã code hóa, Router chỉ đọc Flow | Định tuyến (đã hoàn thành) |
| Can thiệp người dùng | coordinator.md L67-70 | Coordinator LLM phán quyết (khi nhận tin nhắn Inject) | Phán quyết |
| Tái phân công khi planner báo lỗi | coordinator.md L40 | Host Router phát hiện FoundationMissing không đổi, đếm retry | Định tuyến |
| Tóm tắt hoàn thành toàn cuốn | coordinator.md L63-65 + reminder/book_complete | Host Router phát hiện Phase=Complete → FollowUp "xuất tóm tắt" | Định tuyến |

---

## Phụ Lục C: Vị Trí Source Code Tham Chiếu

- `assets/prompts/coordinator.md` — Cần đơn giản hóa
- `internal/host/reminder/flow.go` / `queue_guard.go` / `book_complete.go` — Sẽ xóa
- `internal/host/reminder/subagent_guards.go` — Giữ lại
- `internal/host/reminder/stop_guard.go` — Giữ lại + thêm kiểm tra "phải thực thi khi nhận chỉ thị Host"
- `internal/host/resume.go` — Đơn giản hóa đáng kể
- `internal/host/observer.go` — Thêm đăng ký EventToolExecEnd kích hoạt Router
- `internal/host/flow/` — Package mới
- `internal/tools/commit_chapter.go` L220-280 — CommitResult 17 trường đã đầy đủ
- `internal/tools/save_review.go` L76-116 — Nâng cấp verdict và chuyển đổi Flow đã code hóa
- `internal/store/outline.go` `CheckArcBoundary` — API dữ liệu ranh giới arc

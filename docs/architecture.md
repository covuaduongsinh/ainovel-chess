# Kiến trúc runtime của ainovel-cli

> Để LLM viết hoàn chỉnh một cuốn tiểu thuyết trong một lần Run, Host chỉ đảm nhận khởi động / khôi phục / định tuyến / quan sát, quyền quyết định để lại cho mô hình nhiều nhất có thể.

---

## 1. Mục tiêu (theo thứ tự ưu tiên)

1. **Ổn định**: Nhập một câu, viết xong toàn bộ tiểu thuyết (200~500 chương) một cách ổn định. Không tự ngắt giữa chừng vì lý do kiến trúc.
2. **Chất lượng có thể lặp lại**: prompt / tài liệu tham khảo / tiêu chí xét duyệt / chiến lược ngữ cảnh có thể điều chỉnh độc lập, không ảnh hưởng kiến trúc.
3. **Có thể khôi phục**: Sau khi crash, mất mạng, tạm dừng, có thể tiếp tục từ checkpoint gần nhất.
4. **Có thể quan sát**: Tiến độ, sản phẩm, thời gian của mỗi chương mỗi bước đều có thể tra cứu.

"Ổn định" là tiền đề, "chất lượng" là tầng trên. Mọi quyết định kiến trúc đều ưu tiên phục vụ tính ổn định.

---

## 2. Nguyên tắc cốt lõi

### 2.1 LLM điều khiển sáng tác và phán xét, Host điều khiển định tuyến quy trình

Không gian quyết định của agent chuyên biệt là đóng: biểu đồ luồng cố định, phân nhánh có giới hạn, dựa trên sự kiện. Hai loại quyết định đi qua các phương tiện khác nhau:

- **Sáng tác và phán xét** (ngữ nghĩa/chất lượng/hiểu ý định) → LLM. Năng lực phán xét của Writer/Editor/Architect/Coordinator hưởng lợi tuyến tính theo nâng cấp mô hình
- **Định tuyến quy trình** (đọc sự kiện tra bảng) → Code. `flow.Router` là hàm thuần túy + unit test, tỷ lệ lỗi tiệm cận 0

Host không gọi trực tiếp SubAgent, mà tại ranh giới đồng bộ khi công cụ `subagent` / `reopen_book` của Coordinator trả về thành công, Flow Router tính toán lệnh và đưa vào lượt tiếp theo của run hiện tại thông qua `coordinator.Steer("[Host ra lệnh]…")`. `FollowUp` chỉ được xả ra sau khi agent tự nhiên nhàn rỗi, không thể đảm nhận định tuyến quy trình chính.

### 2.2 Công cụ là giao diện duy nhất của tầng sự kiện

Tất cả tương tác với hệ thống tệp, Progress, Checkpoint đều được thực hiện qua công cụ. **Công cụ ghi bắt buộc ba bước nguyên tử**: artifact ghi xuống đĩa + Progress tiến lên + Checkpoint nối thêm, hoàn thành trong mutex. Chạy lại cùng một công cụ cho kết quả giống nhau hoặc bỏ qua trực tiếp (digest idempotent).

### 2.3 Tầng quan sát chỉ quan sát

UI, chẩn đoán, nhật ký sự kiện đều là consumer thụ động được chiếu ra từ luồng sự kiện / artifact chỉ đọc. Đọc sự kiện, không tạo ra sự kiện, không ảnh hưởng luồng điều khiển.

**`internal/diag` là subsystem khả quan sát duy nhất của engine** — cơ sở hạ tầng hàng đầu, nhưng không phải cốt lõi sản phẩm (cốt lõi là engine sáng tác ở §6; không có diag vẫn viết tiểu thuyết được). Nó đọc chéo hầu hết artifact + session + log + checkpoint, đảm nhận hai vai: ① **Chẩn đoán chất lượng sáng tác** (quy tắc → Finding, báo cáo trên màn hình `/diag`); ② **Gỡ lỗi runtime + xuất khử nhạy cảm** (lột phần khung hành vi khỏi nội dung chính + tổng hợp vòng lặp → `meta/diag-export.md` ghi đè, để người dùng dán vào issue; người bảo trì không có dữ liệu `output/` cục bộ vẫn có thể xác định vòng lặp chết/vấn đề ngắt quãng).

**Kỷ luật quan sát viên (không thể nới lỏng)**: diag có thể chẩn đoán, có thể đề xuất, nhưng **không bao giờ tự tay làm** — không tự động sửa chữa, không tiếp tục chạy, không thay đổi luồng. Nó càng mạnh, càng có người muốn nó "tiện tay sửa luôn", càng phải giữ vững điều này, nếu không sẽ rơi vào cái bẫy đã bị xóa như idleResume / StallDetector (xem §10.5, §10.14). Cấu trúc đối ngoại (như `RuntimeCapture`) cần được duy trì như hợp đồng cơ sở hạ tầng, không tùy tiện thay đổi trường.

### 2.4 Tầng sự kiện phẳng

Chỉ có ba loại sự kiện:

- **Progress** — chỉ số tiến độ (đang viết chương mấy, danh sách chờ viết lại)
- **Checkpoint** — bản ghi tiến hành cấp bước (plan / draft / commit / review / arc_summary)
- **Artifact** — nội dung chương, dàn ý, nhân vật, tóm tắt và các sản phẩm khác

Không giới thiệu các khái niệm trừu tượng như WorkflowInstance / TaskInstance / Command / Dispatcher.

### 2.5 Ba nguyên tắc sắt

**Nguyên tắc sắt 1: Công cụ chỉ trả về sự kiện, không trả lệnh điều phối chéo**. `commit_chapter` trả về các trường có cấu trúc như `arc_end_reached` / `next_skeleton_arc`; không nhúng chuỗi lệnh kiểu `[hệ thống]`. Trường `next_step` trong subagent là hướng dẫn inline về sự kiện ("tôi vừa lưu plan, bước tiếp theo là draft"), không vi phạm — xem §6.4.

**Nguyên tắc sắt 2: Định tuyến quy trình do Flow Router đảm nhận**. `Route(state) → *Instruction` trong `internal/host/flow/router.go` là hàm thuần túy; Host kích hoạt `Dispatch` tại ranh giới đồng bộ của chuỗi thực thi công cụ Coordinator, dùng `Steer` đặt `[Host ra lệnh]` vào đầu vào lượt tiếp theo của run hiện tại. Trả về nil nghĩa là "tình huống phán xét, để LLM tự chủ". **Kênh lệnh không im lặng**: Khi Route liên tục tính ra cùng một lệnh (có nghĩa trạng thái không tiến sau lần gửi trước), Dispatcher gửi lại với thực tế "lần thứ N ra lệnh" thay vì âm thầm bỏ qua — "kết quả định tuyến lặp lại" là sự kiện chỉ Host có thể quan sát, im lặng sẽ khiến Coordinator rơi vào mâu thuẫn kép "không có lệnh thì không được hành động / StopGuard không cho dừng". Không đặt ngưỡng, không ngắt mạch, cách thoát khỏi bế tắc do LLM phán xét.

**Nguyên tắc sắt 3: Coordinator không thể kết thúc lượt vật lý, trừ khi Phase=Complete**. StopGuard tại tầng agentcore chặn `end_turn` và chèn user message; chặn liên tiếp 5 lần sẽ nâng cấp lên terminate. Ba subagent (architect / writer / editor) mỗi cái có `CheckpointDeltaGuard` riêng.

---

## 3. Toàn cảnh kiến trúc

```
[Entry: TUI / headless]
        │ prompt / steer
[Host vỏ mỏng]
   ├── observer        sự kiện → chiếu UI/log
   ├── flow.Dispatcher ranh giới công cụ đồng bộ → Route(state) → Steer
   └── usage / quản lý mô hình
        │
[Coordinator (LLM, MaxTurns=100_000)]
   ├── Phán xét lúc khởi động architect_short / long
   ├── Nhận [Host ra lệnh] → tạo subagent tool_call
   └── Nhận [người dùng can thiệp] → tự chủ phán xét
        │
[architect / writer / editor SubAgent (mỗi cái có run + context + mô hình độc lập)]
        │ gọi công cụ
[Tools]  novel_context · read_chapter · plan_chapter · draft_chapter · edit_chapter
         check_consistency · commit_chapter · save_review · save_arc_summary
         save_volume_summary · save_foundation
        │ ba bước nguyên tử
[Store: hệ thống tệp (tmp + rename)]
   Progress · Checkpoints · Outline · Drafts · Summaries · Characters · World · Signals
```

| Tầng | Làm gì | Không làm gì |
|---|---|---|
| Entry | Hiển thị, nhận đầu vào | Quyết định nghiệp vụ |
| Host | Khởi động/khôi phục/can thiệp/chiếu sự kiện/định tuyến Flow | Vượt qua Coordinator gọi thẳng SubAgent; ghi trạng thái |
| Coordinator | Thực hiện lệnh Host, phán xét Steer của người dùng, khởi động chọn người lập kế hoạch | Tự quyết bước tiếp theo mỗi chương; ghi tệp |
| Agents | Suy nghĩ, viết lách, xét duyệt | Đọc ghi trực tiếp Store |
| Tools | IO nguyên tử + checkpoint + idempotent | Lệnh điều phối chéo subagent |
| Store | Ghi xuống hệ thống tệp | Logic nghiệp vụ |

Phụ thuộc một chiều: `entry → host → agents → tools → store → domain`. `tools/` không tham chiếu `agents/host/`, `host/` không tham chiếu trực tiếp `tools/store/`. Module độc lập ngang: `errs/` có thể được tham chiếu bởi bất kỳ tầng nào, `diag/` đăng ký luồng sự kiện host + chỉ đọc `store/`.

---

## 4. Mô hình dữ liệu

### 4.1 Progress (`internal/domain/runtime.go`)

```go
type Progress struct {
    NovelName         string
    Phase             Phase           // init / premise / outline / writing / complete
    CurrentChapter    int
    TotalChapters     int
    CompletedChapters []int
    TotalWordCount    int
    ChapterWordCounts map[int]int
    InProgressChapter int             // chương đang được viết
    Flow              FlowState       // writing / reviewing / rewriting / polishing / steering
    PendingRewrites   []int
    StrandHistory     []string        // chuỗi dominant_strand
    HookHistory       []string        // chuỗi hook_type
    CurrentVolume, CurrentArc int     // phân tầng tác phẩm dài
    Layered           bool
}
```

Logic điều khiển chỉ đọc các trường sự kiện trên, không phụ thuộc vào bất kỳ "timestamp cập nhật" nào — thông tin thời gian được mang bởi `OccurredAt` của checkpoint.

### 4.2 Checkpoint (`internal/domain/checkpoint.go`)

```go
type Scope      struct { Kind ScopeKind; Chapter, Volume, Arc int }
type Checkpoint struct {
    Seq        int64       // tăng đơn điệu
    Scope      Scope       // chapter / arc / volume / global
    Step       string      // plan / draft / commit / review / arc_summary / ...
    Artifact   string
    Digest     string
    OccurredAt time.Time
}
```

Lưu trữ: `meta/checkpoints.jsonl`, chỉ nối thêm. Ghi trùng cùng `Scope+Step+Digest` được coi là idempotent, không tạo dòng mới.

### 4.3 Artifact và Signals

Artifact nằm trong `store/outline.go` `drafts.go` `summaries.go` `characters.go` `world.go` — mỗi loại sản phẩm đều có thể được checkpoint tham chiếu.

Signals: `PendingCommit` (khôi phục khi commit bị ngắt) / `PendingSteer` (can thiệp người dùng trong khi tắt máy). Đọc khi khởi động/khôi phục, không đọc khi đang chạy.

---

## 5. Quy ước công cụ

Công cụ là điểm tương tác duy nhất giữa tầng sự kiện và Agent.

### 5.1 Công cụ đọc

`novel_context(scope)` / `read_chapter(n)` — có thể gọi bất kỳ lúc nào, không phụ thuộc trạng thái tiền đề, dữ liệu trả về đủ để LLM quyết định độc lập.

### 5.2 Công cụ ghi (ba bước nguyên tử)

Mỗi lần gọi thành công phải: artifact ghi xuống đĩa → Progress tiến lên → checkpoint nối thêm. Ba bước hoàn thành trong mutex.

| Công cụ | Artifact | Step |
|---|---|---|
| `plan_chapter` | drafts/chXX.plan.json | plan |
| `draft_chapter` | drafts/chXX.draft.md | draft |
| `edit_chapter` | drafts/chXX.draft.md | edit |
| `check_consistency` | không có (chỉ đọc, trả về inline) | consistency_check |
| `commit_chapter` | chapters/chXX.md + Progress | commit |
| `save_review` | reviews/chXX.json (global là chXX-global.json) | review |
| `save_arc_summary` | summaries/arc-vNNaNN.json | arc_summary |
| `save_volume_summary` | summaries/vol-vNN.json | volume_summary |
| `save_foundation` | foundation/*.json | premise / outline / layered_outline / characters / world_rules / expand_arc / append_volume / update_compass / complete_book |

`commit_chapter` đảm nhận phát hiện kết thúc cung/tập/toàn bộ sách, trả về 19 trường sự kiện (`arc_end` / `needs_expansion` / `book_complete` v.v.; khi bật kiểm tra quy tắc cơ học còn thêm `rule_violations`). `save_review` đảm nhận nâng cấp verdict (cổng thẻ điểm, hợp đồng missed → rewrite). Logic từng nằm rải rác ở tầng policy nay đã được cố định bên trong công cụ.

`edit_chapter` là lớp bao mỏng của `agentcore.EditTool`, kiểm tra quyền sở hữu đảm bảo chương đã hoàn thành phải có trong `PendingRewrites` mới được chỉnh sửa.

### 5.3 Phân tầng lỗi

| Loại lỗi | Tầng xử lý | Hành động |
|---|---|---|
| Timeout mạng / streaming EOF | Tools | Thử lại 3 lần |
| provider 429/503 | litellm | Failover sang provider dự phòng |
| Xác thực / mô hình không tồn tại | Tools | Ném lỗi terminal |
| Thiếu artifact tiền đề | Tools | Ném lỗi conflict, LLM gọi `novel_context` rồi thử lại |
| Tham số công cụ không hợp lệ | Tools | Ném lỗi validation, LLM sửa tham số |
| MaxTurns cạn kiệt | agentcore | Run kết thúc, Host gửi done |
| Tin nhắn LLM không hợp lệ (thinking-only stop v.v.) | agentcore (`llm/litellm.go` `convertMessages`) | Dự phòng khi vào stack + lọc khi ra; Host không nhận biết |
| Phản hồi streaming rỗng / suy nghĩ lâu | litellm (`StreamIdleTimeout=5min`) | watchdog kích hoạt thử lại |

### 5.4 Idempotent

Trước khi thực hiện, mỗi công cụ ghi kiểm tra checkpoint: nếu `Step+Digest` của checkpoint mới nhất trong scope hiện tại giống với lần này, trả về sản phẩm đã có ngay. LLM có thể thử lại thoải mái, không tạo ra chương trùng lặp hay sai lệch tiến độ.

---

## 6. Lắp ráp Agent

> Về lý thuyết một Prompt siêu lớn + một Agent chạy xong cả cuốn sách là khả thi, nhưng ba điều sẽ cản trở tính ổn định: **bùng nổ ngữ cảnh** (200 chương dù nén mạnh cũng suy giảm), **nhiễu trách nhiệm** (lập kế hoạch chặt chẽ / viết lách tưởng tượng / xét duyệt phê bình trong cùng một prompt làm nhạt lẫn nhau), **mất lợi nhuận mô hình dị cấu** (lập kế hoạch dùng Opus, viết lách dùng Sonnet, xét duyệt dùng Pro, chọn mô hình độc lập là không gian tối ưu chi phí/chất lượng đáng kể cho tác phẩm dài). Cấu trúc đa agent vì vậy là cần thiết.

### 6.1 Coordinator

Trình điều khiển vòng lặp chính duy nhất. Được lắp ráp trong `internal/agents/build.go`:

```go
agent := agentcore.NewAgent(
    agentcore.WithModel(coordinatorModel),
    agentcore.WithSystemPrompt(bundle.Prompts.Coordinator),
    agentcore.WithTools(subagentTool, contextTool),
    agentcore.WithMaxTurns(100_000),
    agentcore.WithToolsAreIdempotent(true),
    agentcore.WithMaxToolErrors(0),  // subagent không ngắt mạch
    agentcore.WithMaxRetries(subagentMaxRetries),
    agentcore.WithContextManager(...),
    agentcore.WithStopGuard(reminder.NewStopGuard(store, nil)),
    agentcore.WithToolGate(completePhaseGate(store)),  // phase=complete chặn cứng gửi subagent
)
```

Trách nhiệm: Khởi động chọn người lập kế hoạch → vòng lặp bổ sung kế hoạch → nhận `[Host ra lệnh]` tạo ngay `subagent` tool_call → xử lý `[người dùng can thiệp]` tự chủ phán xét → xuất tóm tắt sau khi `book_complete=true`.

Không làm: Ghi tệp, đọc trực tiếp Progress (dùng novel_context), tự quyết bước tiếp theo khi lệnh Host đến.

> **Tại sao không xóa Coordinator để Host gọi trực tiếp subagent?** Trông có vẻ "gọn" hơn, nhưng sẽ mất bốn thứ: (1) Quyết định "làm gì tiếp theo" được giữ ở tầng LLM, mô hình nâng cấp trực tiếp có lợi; (2) Phán xét mềm về verdict xét duyệt (accept/polish/rewrite + phạm vi ảnh hưởng) được rút ra khỏi code Go; (3) Đánh giá tác động của Steer người dùng giao cho mô hình — một câu "động cơ nhân vật phụ cần rõ ràng hơn" nên viết lại những chương nào, Coordinator có thể phán xét, hard-code ở Host thì không; (4) Các nhánh bất thường (phản hồi dàn ý từ writer, editor phát hiện lỗ hổng thế giới quan) do mô hình tự xử lý, tránh viết state machine Go cho từng nhánh. **Xóa Coordinator bằng với đặt cược từ "mô hình ngày càng mạnh" sang "code Go của tôi ngày càng mạnh" — đây không phải cược tốt**.

### 6.2 Cấu trúc subagent và mô hình dị cấu

```
Coordinator (1 agent run, MaxTurns=100_000)
    ↓ subagent()
architect_short/long  ·  writer  ·  editor
    ↓ gọi công cụ
Store (phương tiện cộng tác, subagent không giao tiếp trực tiếp với nhau)
```

Số lượt của subagent được tính độc lập (agentcore gốc), không chiếm quota 100_000 lượt của Coordinator. Các subagent giao tiếp qua artifact có cấu trúc trong Store, Coordinator chỉ truyền "mô tả nhiệm vụ" không truyền nội dung.

`bootstrap.ModelSet` hỗ trợ mô hình theo vai: coordinator/architect/writer/editor mỗi cái cấu hình độc lập + failover provider. Writer chạy Sonnet thay vì Opus trong tác phẩm 200 chương có thể tiết kiệm chi phí một bậc độ lớn.

### 6.3 Ba chế độ cộng tác

Các subagent không giao tiếp trực tiếp, tất cả luồng thông tin đi qua artifact có cấu trúc trong Store. Ba chế độ bao phủ toàn bộ quy trình làm việc của hệ thống:

**Chế độ A · Chuyển giao nối tiếp (luồng chính)**: Coordinator → Architect lập kế hoạch → Writer chương 1..N → Editor xét duyệt cuối cung → Writer viết lại. Chế độ phổ biến nhất, Coordinator dùng `novel_context` tra trạng thái hiện tại để quyết định gọi ai tiếp theo.

**Chế độ B · Phản hồi xét duyệt (vòng kín)**: Writer phát hiện dàn ý lệch trong khi phác thảo → giá trị trả về của `commit_chapter` mang trường `writer_feedback` → Coordinator thấy phản hồi phán xét có nâng cấp thành gọi architect điều chỉnh dàn ý không. Writer không gọi trực tiếp Architect, phản hồi được gửi lại Coordinator qua trường có cấu trúc.

**Chế độ C · Mở rộng khung (lập kế hoạch cuộn)**: `commit_chapter` phát hiện cung tiếp theo vẫn còn là khung → trả về `arc_end_reached + next_skeleton_arc` → Flow Router gửi lệnh → Coordinator gọi architect_long mở rộng cung tiếp theo chi tiết → Writer tiếp tục. Khả năng "lập kế hoạch cuộn" của tác phẩm dài chính là vòng kín này thực hiện.

### 6.4 Ràng buộc code cho quy trình subagent (không dựa vào nạng prompt)

> Giai đoạn đầu quy trình writer dựa vào ràng buộc "nghiêm ngặt theo thứ tự sau" trong `writer.md`. LLM thường vi phạm — bỏ qua plan đi thẳng vào draft, nói thêm một đoạn sau commit tiêu tốn token, chỉ viết nội dung trong chat không lưu xuống đĩa. **Ràng buộc quy trình bằng prompt không ổn định**, mạnh hay yếu hoàn toàn phụ thuộc vào mức độ "nghe lời" của mô hình lúc đó, thậm chí nâng cấp mô hình có thể khiến nó "sáng tạo mà không tuân thủ".

Bốn lớp ràng buộc code (cùng lúc có hiệu lực):

| Lớp | Vị trí | Tác dụng |
|---|---|---|
| `StopAfterTools` / `StopAfterToolResult` | `agents/build.go` SubAgentConfig | Công cụ quan trọng thành công thì end_turn thoát subagent run. Writer khi `commit_chapter` kích hoạt thì dừng (`StopAfterTools`); Công cụ `save_arc_summary`/`save_volume_summary` của Editor và kết thúc cung/tập của Architect đi qua `StopAfterToolResult`. `save_review` của Editor không dừng cứng — nếu dừng sẽ cắt ngang run tóm tắt cung qua StopGuard, kết thúc giao cho `NewEditorStopGuard` |
| `CheckpointDeltaGuard` | `host/reminder/subagent_guards.go` | Lấy baseline checkpoint làm ranh giới, trước khi kết thúc lượt này phải thấy checkpoint mới của bước tương ứng, nếu không từ chối `end_turn`; chặn liên tục 3 lần nâng cấp terminate (dự phòng vòng lặp chết mô hình yếu) |
| `next_step` inline trong công cụ | Trường giá trị trả về của từng công cụ | Mỗi sự kiện tự mang "gợi ý bước tiếp theo". Ví dụ `plan_chapter` trả về `next_step: "gọi ngay draft_chapter..."`. LLM thấy sự kiện là biết bước tiếp theo, không cần quay lại system prompt tìm kiếm |
| Kiểm tra quyền sở hữu/tiền đề trong công cụ | `edit_chapter` `commit_chapter` v.v. | Chặn vật lý ở tầng dữ liệu: `edit_chapter` từ chối sửa chương đã hoàn thành không có trong `PendingRewrites`; `commit_chapter` từ chối commit rỗng khi bản nháp == bản cuối; `ConcurrencySafe=false` ngăn race condition đồng thời |

writer.md trong kiến trúc mới chỉ đảm nhận: hướng dẫn chất lượng viết lách, mô hình nhận thức tiếp tục từ điểm ngắt, giải thích hợp đồng chương. **Không còn điều phối quy trình** — khi LLM bỏ qua bước, prompt sẽ không giải cứu, code sẽ làm. architect / editor cũng có bốn lớp ràng buộc tương tự trong các công cụ/Guard riêng.

> Về nguyên tắc sắt 1: `next_step` là trình bày sự kiện inline trong công cụ ("tôi vừa lưu plan"), không phải điều phối quy trình được Host chèn qua nhiều cuộc gọi. Điều phối chéo subagent ở tầng Coordinator vẫn nghiêm ngặt đi qua Flow Router → Steer.

### 6.5 Phụ thuộc agentcore

`../agentcore` là thư viện Agent chung của dự án này (liên kết qua go.work). Tất cả các primitive mà kiến trúc mới sử dụng đều đã tồn tại: `Prompt` / `Inject` / `Steer` / `Subscribe` / `WithMaxTurns` / `WithStopGuard` / `WithToolGate` / `WithMiddlewares` / `SubAgentConfig` / `WithContextManager`.

**Ranh giới chỉnh sửa**:

- Có thể vào agentcore: chiến lược ContextManager mới, adapter provider mới, loại sự kiện mới, mẫu chèn tin nhắn chung
- Không vào agentcore: mô hình nghiệp vụ như Progress/Checkpoint/Scope, công cụ nghiệp vụ như novel_context/commit_chapter, quy tắc nghiệp vụ như phát hiện kết thúc cung/cổng xét duyệt

Tiêu chí phán xét: Giả sử agentcore trong tương lai sẽ được coding agent / customer service agent sử dụng, tính năng mới thêm vào vẫn có ý nghĩa trong bối cảnh đó mới được phép vào. **Cấm viết bản vá dự phòng ở tầng ứng dụng** (proxy, wrapper, monkey patch) — thiếu tính năng thì trực tiếp vào agentcore sửa.

**Tính năng cố tình không dùng** (tránh nhầm lẫn):

- `Agent.TaskRuntime() / Tasks() / StopTask()` — trình quản lý tác vụ nền tích hợp sẵn của agentcore (fire-and-forget background subagent). Kiến trúc mới tất cả các cuộc gọi subagent đều là đồng bộ foreground, **không sử dụng**
- `Agent.Steer(msg)` — kênh lệnh quy trình của `flow.Dispatcher`, dùng để đưa `[Host ra lệnh]` vào Coordinator run đang chạy; phải kích hoạt tại ranh giới công cụ đồng bộ, đảm bảo đến sau kết quả công cụ, trước lần gọi mô hình tiếp theo
- `Agent.FollowUp(msg)` — kênh tin nhắn theo dõi sau nhàn rỗi, không dùng cho Flow Router; nó chỉ được xả ra khi agent sắp dừng tự nhiên, dùng để ra lệnh quy trình chính sẽ khiến lệnh đến trễ
- `Agent.Inject(msg)` / `InjectContext` — cổng vào can thiệp người dùng/bên ngoài: ghi vào hàng đợi steering khi đang chạy, tự động khôi phục run khi nhàn rỗi và có thể tiếp tục; `Steer(text)` của Host đi qua nó, Resume đi qua `Prompt` để khởi động run mới
- `WithPermission*` — cơ chế phê duyệt quyền (phê duyệt thủ công cho thao tác nguy hiểm), ứng dụng tiểu thuyết không có thao tác nguy hiểm, **không sử dụng**

**Hook chiến lược đã bật**: `WithToolGate` — mục đích duy nhất là chặn cứng gửi `subagent` khi `phase=complete` (`agents/build.go` `completePhaseGate`). Sau khi hoàn thành nếu người dùng yêu cầu viết tiếp/viết lại, Coordinator LLM vẫn có thể tự gửi subagent, còn Writer ghi chương vượt giới hạn sẽ bị `commit_chapter` từ chối, `CheckpointDeltaGuard` lại không cho `end_turn` → vòng lặp chết. Flow Router trả về nil ở trạng thái complete chỉ chặn Host tự động gửi, không chặn được LLM chủ động gửi, nên cần Gate bổ sung một lớp bảo vệ trạng thái cuối tại điểm yết hầu. Đây là dự phòng quy trình mục đích hẹp, **không phải luồng phê duyệt `WithPermission*`**, không được nhầm lẫn.

---

## 7. Tầng Host

### 7.1 Cấu trúc

```go
type Host struct {
    cfg               bootstrap.Config
    bundle            assets.Bundle
    store             *store.Store
    models            *bootstrap.ModelSet
    coordinator       *agentcore.Agent
    coordinatorCtxMgr *corecontext.ContextEngine  // liên kết cửa sổ ngữ cảnh khi đổi mô hình
    askUser           *tools.AskUserTool
    writerRestore     *ctxpack.WriterRestorePack

    observer     *observer
    router       *flow.Dispatcher  // ranh giới công cụ đồng bộ + Route + Steer
    usage        *UsageTracker
    usageCancel  context.CancelFunc
    budget       *BudgetSentinel   // thành phần chính sách Host: thực thi tuyên bố ngân sách người dùng (tương đương Abort đại diện), ưu tiên hơn Dispatcher tại ranh giới đồng bộ
    notifier     *notify.Notifier  // tầng quan sát: bản sao ngoài màn hình của ba loại cảnh báo run_end/repeat/budget, không bao giờ can thiệp luồng điều khiển

    events, streamCh, done chan ...

    mu        sync.Mutex
    lifecycle lifecycle  // idle / running / paused / completed
    closeOnce sync.Once
}
```

### 7.2 API công khai

**Vòng đời** (cổng vào Run của Coordinator): `Start` / `StartPrepared` / `Resume` / `Continue` / `Steer` / `Abort` / `Close`

**Kênh quan sát**: `Events` / `Stream` / `Done` (xả luồng đi qua sentinel trong streamCh)

**Tổng hợp UI**: `Snapshot()` — TUI lấy tất cả dữ liệu hiển thị một lần

**Cấu hình/Mở rộng**: Quản lý mô hình (`SwitchModel`), nhập suy ngược tiểu thuyết bên ngoài (`ImportFrom`), đối thoại đồng sáng tác (`CoCreateStream`), phát lại sự kiện (`ReplayQueue`), hồ sơ mô phỏng (`Simulate`/`ImportSimulationProfile`), xuất (`Export`)

Không có các phương thức điều phối nghiệp vụ như `decideNext` `retryActiveTask`. Flow Router là hàm thuần túy + tổ hợp mỏng của gửi Steer, không giữ trạng thái ngầm như "tác vụ đang thử lại".

### 7.3 Dạng `waitDone`

```go
func (h *Host) waitDone() {
    h.coordinator.WaitForIdle()
    h.observer.finalize()

    if Phase == Complete { lifecycle=completed; gửi sự kiện "sáng tác hoàn thành" }
    else if running        { lifecycle=idle;     gửi sự kiện "Coordinator dừng (đã hoàn thành N chương)" }

    select { case h.done <- struct{}{}: default: }
}
```

Ba việc: Chờ idle → Chuyển lifecycle → Gửi sự kiện trạng thái cuối + gửi tín hiệu done. **Cấm `Inject` / `FollowUp` / `Prompt` xuất hiện trong thân hàm**. Sau khi LLM chạy xong một Run, toàn bộ Host vào trạng thái cuối.

Chỉ có hai cách để hoạt động trở lại: người dùng chủ động `Continue`/`Start`, hoặc khởi động lại tiến trình đi qua `Resume`.

> Bài học lịch sử: Đã từng thêm bản vá tự động khởi động lại Run bằng `idleResumeCount` trong hàm này. Trong lần chạy dài mimo duy nhất thực sự kích hoạt, 100% không giải cứu được, mà còn che giấu nguyên nhân thực sự ở tầng agentcore là "tin nhắn thinking-only stop đi vào lịch sử". **"Khởi động lại phòng thủ" ở tầng Host luôn là sửa chữa sai chỗ**. Xem `feedback_no_host_resilience.md` và §10 mục 5.

---

## 8. Khởi động và Khôi phục

### 8.1 Tạo mới

```
Người dùng: "yêu cầu một câu"
  → Host.Start
    → store.Progress.Init / store.Checkpoints.Reset
    → coordinator.Prompt(userPrompt) + flow.Dispatcher.Enable + Dispatch
    → Vòng lặp dài Coordinator: lập kế hoạch → viết 1..N → xét duyệt → done
```

### 8.2 Khôi phục (khởi động lại sau crash)

```
Tiến trình khởi động
  → Đọc Progress + Checkpoint gần nhất + PendingCommit + PendingSteer
  → buildResumePrompt → thông báo ngắn (không phải lệnh cấp bước)
  → coordinator.Prompt(resumePrompt) + Dispatcher.Enable + Dispatch
  → Coordinator tiếp tục theo lệnh Host
```

Resume dùng `Prompt` khởi động Run mới (đếm lượt reset, ngữ cảnh sạch), không phải `FollowUp`. Bước đầu tiên rõ ràng sau khi khôi phục được Flow Router `Dispatch` ngay lập tức, các bước tiếp theo phát sinh tại ranh giới đồng bộ khi subagent trả về thành công.

### 8.3 Can thiệp người dùng

| Cổng vào | Tiền tố | Ngữ nghĩa | Triển khai |
|---|---|---|---|
| `Steer(text)` | `[người dùng can thiệp]` | Sửa đổi/tra vấn, cần Coordinator phán xét | Khi đang chạy đi qua `Inject`; khi tắt máy ghi PendingSteer vào `meta/run.json` |
| `Continue(text)` | `[người dùng can thiệp]` | Tiếp tục viết, đánh thức sau khi tắt máy | Khi đang chạy đi qua `FollowUp`; khi tắt máy đi qua `Inject` tự động khôi phục run |

Hai cổng vào thống nhất qua helper `interventionMsg` thêm tiền tố `[người dùng can thiệp]` — đây là điểm neo phân loại can thiệp của `coordinator.md`; từng có lúc Continue gửi văn bản trần sẽ bỏ qua phân loại, bị nhầm phân công writer sửa chương đã viết (đã sửa).

Ngữ nghĩa `Inject`: Chèn hàng đợi khi đang chạy; tự động khôi phục run và chèn khi nhàn rỗi; xếp hàng chờ khi tạm dừng.

**Tầng lưu trữ bền vững cho can thiệp dài hạn**: Yêu cầu dài hạn về "cách viết" trong phân loại can thiệp (phong cách viết/quy tắc chất lượng) được Coordinator gọi `save_user_rules` để LLM chuẩn hóa, hợp nhất vào snapshot quy tắc cuốn sách `meta/user_rules.json`, `novel_context` đưa vào `working_memory.user_rules` — tất cả subagent tự động thấy mỗi chương, có hiệu lực qua nén, qua khởi động lại, không phụ thuộc bộ nhớ hội thoại Coordinator và chuyển tiếp phân công (chi tiết xem [Snapshot quy tắc người dùng](user-rules-runtime.md)). Ba loại can thiệp còn lại bản thân đã rơi vào store (phạm vi/cốt truyện/cấu trúc → compass/outline, thiết lập/nhân vật → foundation, sửa chương cũ → PendingRewrites). Đi qua phong bì không đi qua system prompt: bảo vệ cache tiền tố system chéo chương của writer.

> Lịch sử: Giai đoạn đầu "yêu cầu dài hạn về phong cách" đi qua `save_directive` độc lập → `meta/user_directives.json` (có điểm neo tiến độ at_chapter). Ngày 2026-06-28 hợp nhất với `save_user_rules` — hai cái trùng lặp về sở thích văn bản tự do, "có hay không có điểm neo" là câu hỏi phân loại mờ, nên bỏ `save_directive`, yêu cầu viết lách dài hạn thống nhất về user_rules; `meta/user_directives.json` cũ không còn được đọc hay di chuyển. Yêu cầu thực sự gắn với tiến độ cốt truyện/cấu trúc giao cho architect, không dùng lệnh văn bản làm phương tiện nữa.

---

## 9. Cấu trúc thư mục

```
internal/
  domain/         Dữ liệu thuần túy: Phase / FlowState / Progress / Checkpoint / Scope / Story / Plan /
                  Review / StateChange / quy tắc chuyển tiếp Phase-Flow
  store/          Lưu trữ bền vững hệ thống tệp (tmp+rename + ba bước nguyên tử): progress / checkpoints / outline /
                  drafts / summaries / characters / world / signals / run_meta / runtime / session
  tools/          11 công cụ Agent, công cụ ghi toàn bộ ba bước nguyên tử + digest idempotent + ConcurrencySafe=false
                  + premise_structure (dùng nội bộ trong save_foundation) + ask_user
  agents/         build.go lắp ráp Coordinator + ba subagent; ctxpack/ chiến lược nén ngữ cảnh Writer
  host/           host.go + resume.go + observer.go + events.go + usage.go + usage_replay.go
                  + stream_extract.go + cocreate.go
    flow/         router.go (hàm thuần túy 11 nhánh) + state.go + dispatcher.go + router_test.go
    reminder/     stop_guard.go (Coordinator) + subagent_guards.go (CheckpointDeltaGuard ×3)
    imp/          Nhập suy ngược tiểu thuyết bên ngoài: tách → foundation → phân tích từng chương
    exp/          Xuất chương đã hoàn thành: gộp chương → TXT / EPUB 3, hậu tố đường dẫn điều khiển; chỉ đọc thuần túy, không phụ thuộc LLM
  entry/          tui (Bubble Tea) / headless / startup
  bootstrap/      config + ModelSet + provider failover + trình hướng dẫn cài đặt
  models/         Bảng đăng ký mô hình công khai OpenRouter v.v. + làm mới giá (cache đĩa 24h)
  errs/           Phân tầng lỗi
  diag/           Module chẩn đoán chỉ đọc đăng ký luồng sự kiện host
  utils/          Di sản kiến trúc cũ (một số công cụ phân tích, code mới không nên phụ thuộc)

assets/
  prompts/        coordinator (~55 dòng) / architect-short|long / writer / editor / import-* / simulation-*
  references/     Kỹ thuật viết + mẫu thể loại + lập kế hoạch tác phẩm dài v.v.
  styles/         Mặc định/kiếm hiệp/ngôn tình/trinh thám

../agentcore     Framework Agent chung (thư mục anh em go.work, có thể thêm tính năng chung, không thêm nghiệp vụ)
../litellm       Cổng LLM
```

### 9.1 Cột mốc phát triển

| Thời gian | Tái cấu trúc | Kết quả thuần |
|---|---|---|
| 2026-04-10 | `internal/orchestrator/` (6342 dòng) → `host/` + `agents/` | Core runtime -74% |
| 2026-04-20 | Hybrid Coordinator: Tạo mới `host/flow/`, `reminder/` gọn lại, `coordinator.md` 88 dòng → 45 dòng | Tỷ lệ lỗi định tuyến tiệm cận 0 |
| 2026-05-02 | agentcore `WithMaxToolErrors(0)` + `isReasoningOnlyStopAssistant`; `StreamIdleTimeout=5min`; Xóa bản vá tiếp tục chạy `idleResumeCount` | mimo / slow-thinking streaming chạy thông |
| 2026-06-05 | Vòng kín lập kế hoạch cuộn (`expand_arc`/`append_volume`) + `/import` suy ngược phân tầng tiếp tục viết + can thiệp phạm vi người dùng | 200+ chương chạy thông lần đầu |

Thực nghiệm: hy3-preview free 12 chương / 73 phút, mimo-v2.5-pro 10 chương / 84.000 chữ (trung bình 8400/chương), đều chạy xong một lần; tác phẩm dài gpt-5.4 "Phàm Cốt" 235 chương / 1,27 triệu chữ / trung bình 5407/chương, vòng kín lập kế hoạch cuộn chạy thông.

---

## 10. Những điều rõ ràng không làm

Vi phạm đồng nghĩa kiến trúc lệch hướng.

1. **Không giới thiệu khái niệm Task / Job / WorkItem**. "Nhiệm vụ hiện tại" hiển thị trên UI là chiếu luồng sự kiện, không phải sự kiện.
2. **Không giới thiệu Dispatcher / Scheduler / Ready Evaluator**. Quyền quyết định nằm ở Coordinator LLM và tầng công cụ.
3. **Không làm cơ chế "tiếp tục chạy khi nhàn rỗi" kiểu `idle_dispatch`**. Coordinator Run kết thúc = Host gửi done.
4. **Không ở Host vượt qua Coordinator gọi thẳng SubAgent**. Flow Router ra lệnh `[Host ra lệnh]` qua `coordinator.Steer`, để Coordinator tạo tool_call. Resume dùng `Prompt` khởi động Run mới.
5. **Không thêm bản vá tự động tiếp tục chạy cho trường hợp LLM dừng bất thường ở Host**. Run kết thúc = Host vào trạng thái cuối. `idleResumeCount` trước đây đã bị xóa (xem §7.3, `feedback_no_host_resilience.md`).
6. **Không suy luận hoàn thành nhiệm vụ dựa trên "tool exec end"**. Bằng chứng duy nhất của hoàn thành là checkpoint được ghi.
7. **Không làm mô hình bốn lớp WorkflowInstance / TaskInstance / Command + Apply v.v.**. Tầng sự kiện chỉ có ba loại Progress + Checkpoint + Artifact.
8. **Không hỗ trợ tác vụ song song**. Một Coordinator Run hoạt động duy nhất, một cuốn sách tiến hành nối tiếp. Nhiều tiểu thuyết hãy dùng nhiều tiến trình.
9. **Không gọi LLM ở tầng công cụ** (ngoại trừ công cụ Agent chính nó). IO thuần túy + xác thực + idempotent.
10. **Không để UI đọc trực tiếp Store**. Chỉ có thể đăng ký sự kiện hoặc đọc `Snapshot()` của Host.
11. **Không dùng tệp tín hiệu làm IPC**. Host đọc trực tiếp Progress + Checkpoint + dàn ý phân tầng, `flow.Route` suy ra lệnh từ sự kiện là định tuyến chuyên biệt hợp lý.
12. **Không viết state machine Flow ở Host**. Nhãn Flow chỉ được cập nhật bởi công cụ, Router chỉ đọc không ghi.
13. **Không viết hard-code dự phòng cho "LLM ảo giác"**. Tối ưu prompt, cải thiện cấu trúc giá trị trả về công cụ, làm cho `novel_context` trình bày sự kiện rõ ràng hơn — thay vì Host ép thay đổi luồng.
14. **Không để diag / tầng quan sát can thiệp luồng điều khiển**. Chẩn đoán chỉ đọc, chỉ tạo Finding và xuất khử nhạy cảm; tự động sửa chữa / tiếp tục chạy / thay đổi luồng đều không làm (xem §2.3 kỷ luật quan sát viên).
15. **Ngân sách và cảnh báo không vào Route/tầng công cụ, cảnh báo không vào luồng điều khiển**. `BudgetSentinel` là thành phần chính sách Host (thực thi Abort do người dùng ký trước, không đánh giá hành vi mô hình); `notify` là thuần quan sát (không thử lại, không đổi phân công, không tắt máy). `flow.Route` giữ nguyên hàm thuần túy, không nhận biết cả hai.

---

## 11. Chiến lược xác thực

### 11.1 Kịch bản ổn định

- **A Chạy dài**: 80~200 chương chạy xong một lần, Phase=complete. Cho phép failover provider, thử lại transient của công cụ; cấm Host tiếp tục chạy hoặc Coordinator chạy nhiều Run.
- **B Phục hồi crash**: Kill tiến trình sau khi draft chương N / trước khi commit → Resume → Tiếp tục từ consistency_check, không viết lại bản nháp đã ghi xuống đĩa. `checkpoints.jsonl` không có bước trùng lặp.
- **C Provider không ổn định**: Mô phỏng 503 gián đoạn → litellm failover; vòng lặp chính LLM không nhận biết.
- **D Can thiệp người dùng**: Steer khi đang chạy → Coordinator xử lý ở lượt tiếp theo; Steer khi tắt máy → prompt Resume lần sau bao gồm.

### 11.2 Tuân thủ (có thể viết thành linter / test)

- `internal/host/` không được phép `import "internal/scheduler"` hay các gói điều phối tương tự
- Số lượng API vòng đời của `host.go` ổn định; phương thức công khai mới thêm chỉ có thể là loại "cổng vào mở rộng" (đồng sáng tác/nhập/quản lý mô hình)
- Thân hàm `waitDone` không được phép `coordinator.Inject` / `FollowUp` / `Prompt`
- Code liên quan đến `recovery` chỉ được xuất hiện trong `host/resume.go`
- `flow.Route` phải là hàm thuần túy: cấm đọc Store / bất kỳ IO nào

### 11.3 Lặp lại chất lượng

Sửa `writer.md` ngay lập tức tạo ra thay đổi phong cách; Thêm tiêu chí xét duyệt editor mới tương thích ngược (save_review nhận JSON có cấu trúc). Thêm một tài liệu tham khảo md cần đấu dây ở ba chỗ (`tools.References` field + `loadReferences` trong `assets/load.go` + chèn `writerReferences`/`architectReferences` trong `novel_context.go`), không phải đặt vào thư mục là tự động tải — `References` là ánh xạ trường tường minh, tiện cắt theo vai/chương.

**Thống kê phong cách toàn tập (`internal/stylestat`)**: Cửa sổ xét duyệt trong cung với "tic câu trung bình vài chục lần/chương, hình thái đồng cấu cuối chương, lặp từng chữ chéo chương" loại cố định toàn tập này về mặt tự nhiên là mù — nhìn từng chương mỗi chỗ đều bình thường. `novel_context` chạy thống kê xác định trên tất cả chương đã hoàn thành theo đường dẫn chương (loại mẫu câu/cụm từ tần số cao gần cửa sổ/câu lặp chéo chương/hình thái cuối chương/định dạng tiêu đề lẫn lộn), đưa vào `episodic_memory.style_stats`: editor phán xét theo số trong chiều aesthetic, writer dựa vào đó tự tránh. **Thống kê giao code, phán xét giao LLM** — ngưỡng không hard-code trong code, số có thành bệnh hay không do mô hình phán xét theo thể loại. Song song với nó là `rules.Lint` đáy chất lượng sản phẩm (markdown còn sót/đoạn không phải tiếng Việt) luôn thực thi trong commit_chapter, chỉ trả về sự kiện.

---

## 12. Tóm tắt

> **Để LLM viết hoàn chỉnh một cuốn tiểu thuyết trong một lần Run, Host chỉ đảm nhận khởi động / khôi phục / định tuyến / quan sát, ghi sự kiện do công cụ ghi xuống đĩa nguyên tử, quyền quyết định để lại cho mô hình nhiều nhất có thể.**

Không có workflow engine, không có task queue, không có dispatcher, không có scheduler. Chỉ có:

- Một Coordinator 100_000 lượt
- Ba loại subagent chức năng (ngữ cảnh và mô hình độc lập)
- 11 công cụ nguyên tử
- Một tệp checkpoint jsonl
- Vỏ Host ~860 dòng
- Hàm thuần túy Flow Router ~150 dòng (11 nhánh + unit test)

Mỗi dòng code nghiệp vụ Host là cược đặt chống lại việc nâng cấp mô hình. **Host tối thiểu, Prompt tối đa (tầng chất lượng), Công cụ mạnh nhất** để kiến trúc tự động tốt hơn mỗi năm — Coordinator quyết định chính xác hơn, Writer viết tốt hơn, Editor xét duyệt chính xác hơn, Architect lập kế hoạch tinh tế hơn, đều là lợi ích trực tiếp từ việc đổi mô hình mà kiến trúc không cảm nhận được.

Ngược lại hard-code trong Host các quy tắc như "lần xét duyệt trước nói viết lại chương 3, 5" hay "liên tiếp 3 lần không tiến thì tắt máy", khi mô hình nâng cấp sẽ khiến nó trở thành **lợi nhuận âm**: Phán xét lẽ ra LLM làm trở nên thừa, logic bảo vệ trở thành báo cáo sai. **Tệ nhất là không ai dám xóa — xóa đồng nghĩa với "tin tưởng mô hình", gánh nặng tâm lý còn khó dọn sạch hơn code**. Loại code này để lại càng nhiều, chi phí tái cấu trúc tương lai càng cao.

**Khả năng mở rộng đến từ điểm mở rộng đúng**: Đổi phong cách → sửa prompt; Tiêu chí xét duyệt mới → sửa prompt; Thể loại mới → thêm tài liệu tham khảo; Loại subagent mới → thêm một dòng SubAgentConfig; Song song nhiều tiểu thuyết → nhiều tiến trình.

Kỷ luật duy nhất: **Khi có người muốn "làm Host thông minh hơn một chút", hãy hỏi trước "tại sao không làm LLM thông minh hơn một chút"**. Câu hỏi này không trả lời được lý do "Host phải làm", thì không thêm code vào Host.

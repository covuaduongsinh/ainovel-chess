# Hệ thống đánh giá ainovel-cli

> Đánh giá không phải xây dựng một bộ script kiểm tra mới, mà là **lấy bộ chẩn đoán sự kiện đã có (`diag`), bộ thống kê văn thể toàn tập (`stylestat`), xét duyệt nguyên bản bảy chiều (`ReviewEntry`) làm bộ đánh giá**, rồi bọc một lớp harness offline theo lô. Một định nghĩa sự kiện, hai nơi không còn sai lệch.

---

## 0. Tại sao cần thiết kế lại

Tính ổn định đã chạy thông: tác phẩm dài 235 chương / 1,27 triệu chữ viết xong một lần, vòng kín lập kế hoạch cuộn thành công (xem `architecture.md` §9.1). Nút thắt cổ chai đã chuyển sang — **chất lượng có thể lặp lại**:

- Sau khi sửa một prompt, quy trình có còn ổn định không? Chuỗi công cụ, tiến tiến trạng thái, sự kiện lưu trữ bền vững có còn đúng không?
- Chất lượng nội dung, dàn ý, xét duyệt có thực sự cải thiện, hay chỉ là lần này tình cờ gặp kết quả tốt?
- Trong tác phẩm dài, nhân vật, dòng thời gian, phục bút, ngữ cảnh có liên tục đáng tin cậy không?
- **Cố định văn thể toàn tập** (tic câu trung bình vài chục lần/chương, hình thái đồng cấu cuối chương, lặp từng chữ chéo chương) có tốt hơn hay tệ hơn không? Đây là thủ phạm thực sự của điểm 6,5/10 trong thực nghiệm 196 chương, xét duyệt từng chương về bản chất là mù với nó.

Hiện tại những đánh giá này dựa vào "cảm tính + đọc mẫu thủ công". Hệ thống đánh giá cần biến sửa đổi prompt từ cảm tính thành quy trình kỹ thuật **có hồi quy, có bằng chứng, có đọc mẫu thủ công**.

Nhưng dự án này không cần và không nên copy nguyên xi nền tảng eval thông dụng trong ngành (dataset / experiment / scorer / database / Web UI). Lý do rất đơn giản: **các năng lực cốt lõi này — kiểm tra xác định và tín hiệu chất lượng — đã tồn tại trong dự án, được viết bằng Go và chia sẻ cùng một mô hình sự kiện với runtime.**

---

## 1. Luận điểm cốt lõi: Bộ đánh giá đã tồn tại

Bốn loại bộ đánh giá của hệ thống đánh giá, ba loại đã được triển khai trong codebase, chỉ là chưa từng được gọi với tư cách "bộ đánh giá":

| Bộ đánh giá | Năng lực đã có trong dự án | Cổng vào | Đầu ra |
|---|---|---|---|
| **Chẩn đoán sự kiện xác định** | Một tập quy tắc artifact + quy tắc runtime của `internal/diag` | `diag.Diagnose(store)` | `Report{Stats, Findings}`, Finding có Severity/Evidence |
| **Hồi quy văn thể toàn tập** | `internal/stylestat` | `stylestat.Compute(input)` | Mẫu câu trung bình/chương, câu lặp chéo chương, tỷ lệ câu ngắn cuối chương, tiêu đề lẫn lộn |
| **Phán xét chất lượng (rubric)** | Rubric có phiên bản (ban đầu suy ra từ bảy chiều của `editor.md`) | LLM Judge (thước chuẩn cố định làm A/B) | consistency/character/pacing/continuity/foreshadow/hook/aesthetic |
| **Xuất hành vi khử nhạy cảm** | Xuất của `internal/diag` | `diag.WriteExport(store, rep, rc)` | Khung hành vi, dùng cho đọc mẫu thủ công và lưu trữ |

`diag.Analyze(s *store.Store)` nhận một Store là có thể tạo ra `Report` hoàn chỉnh — **nó vốn đã có thể chạy offline trên bất kỳ thư mục đầu ra nào**. `stylestat.Compute` là hàm thuần túy. Điều này có nghĩa hệ thống đánh giá không cần triển khai lại "chương có ghi xuống đĩa không, progress có tiến không, checkpoint có tồn tại không, pending có còn sót không, quy trình có vòng lặp chết không" — những điều này diag đều làm rồi, và mỗi quy tắc tương ứng với một bẫy thực tế đã từng rơi vào (`PhaseFlowMismatch`, `OrphanedSteer`, `OutlineExhausted`, `repeatedErrors`/`stuckStep` tương ứng với các sự cố lịch sử như idleResume / deadlock cạn dàn ý / công cụ được gọi như in văn bản v.v.).

> **Việc của hệ thống đánh giá không phải tạo ra kiểm tra, mà là: thúc đẩy theo lô + chạy bộ đánh giá đã có trên đầu ra + ánh xạ Finding/thống kê thành cổng kiểm soát + tổng hợp báo cáo.**

---

## 2. Nguyên tắc thiết kế

### 2.1 Bộ đánh giá chính là chẩn đoán viên, tuyệt đối không tái tạo kiểm tra xác định

Kiểm tra xác định chỉ gọi `diag.Diagnose`, không phân tích lại `progress.json` / `checkpoints.jsonl` / `sessions/*.jsonl` ở tầng đánh giá. Lý do là nguyên tắc DRY sắt của dự án này: **"trạng thái hợp lệ là gì" chỉ được có một định nghĩa.** Nếu đánh giá dùng Python phân tích lại checkpoint một lần nữa để phán xét commit có thiếu không, thì sẽ có hai định nghĩa "commit hoàn thành", runtime sửa quy tắc diag mà đánh giá không sửa theo, cổng kiểm soát lập tức mất chính xác.

→ Eval harness dùng **Go**, gọi in-process `diag` và `stylestat`, chia sẻ `internal/domain` và `internal/store` với runtime. Đây là sự khác biệt căn bản nhất giữa thiết kế này và phiên bản trước.

### 2.2 Hồi quy văn thể toàn tập là tín hiệu chất lượng đầu tiên

LLM Judge từng chương trông chương nào cũng "bình thường", nhưng nút thắt cổ chai chính là cố định chéo chương. Nên **cột sống xác định của hồi quy chất lượng là `stylestat`, không phải LLM Judge**.

**Tiền đề: `stylestat.Compute` dưới 5 chương trả về nil trực tiếp** (`stylestat.go` `minChapters=5`, mẫu quá nhỏ tần suất vô nghĩa). Do đó hồi quy văn thể **chỉ có hiệu lực ở tầng Quality / Longform ≥5 chương**, 1 chương Smoke không lấy được tín hiệu văn thể — điều này quyết định chi phí và chiến lược mặc định ở phần dưới. Các chỉ số bao gồm:

- Mẫu câu trung bình/chương của variant vs baseline (`patterns[].per_chapter`)
- Tỷ lệ câu ngắn cuối chương (`ending.short_ratio` tiệm cận 1 là bệnh)
- Số câu lặp từng chữ chéo chương (`repeated_sentences`)
- Định dạng tiêu đề lẫn lộn (`title_formats`)
- Tỷ lệ từ chỉ thời gian mở đầu (`opening_time_rate`)

Đây là các chỉ số không tốn LLM, xác định, và đánh trúng vào nút thắt chất lượng. **LLM Judge là bổ sung, stylestat delta là luồng chính.**

### 2.3 LLM Judge căn chỉnh theo rubric nguyên bản bảy chiều, không khởi xướng mới

Judge không phát minh chiều đánh giá mới — chiều nghiêm ngặt bằng bảy mục của `domain.DimensionScore`, thực hiện so sánh baseline/variant.

**Nhưng rubric phải có phiên bản, có thể cố định**, lưu dưới dạng snapshot trong `evals/rubrics/*.json`, không phải đọc `editor.md` realtime trong runtime. Lý do: khi đối tượng được kiểm tra chính là `editor.md` bản thân, nếu trọng tài cũng thay đổi cùng với `editor.md`, tiêu chuẩn đánh giá sẽ trôi dạt — trọng tài và đối tượng kiểm tra cùng nguồn gốc khiến "sửa editor tốt hay xấu" không thể phán xét. Nên rubric ban đầu **suy ra** từ bảy chiều của editor (đảm bảo khẩu kính nhất quán), sau đó **phát triển độc lập, tăng phiên bản tường minh**; báo cáo ghi lại dùng phiên bản rubric nào.

### 2.4 Finding xác định quyết định cổng kiểm soát, LLM và con người chỉ làm phán xét chất lượng

Căn chỉnh theo nguyên tắc sắt của kiến trúc "thống kê giao code, phán xét giao LLM":

- **Chỉ có bằng chứng xác định mới có thể chặn hợp nhất**: Finding `SevCritical` của `diag`, khẳng định hợp đồng theo case thất bại.
- **LLM Judge và đọc mẫu thủ công tạo ra warning và gợi ý sắp xếp**, không đơn độc quyết định hợp nhất.
- Một câu: `Finding.Severity` ánh xạ trực tiếp mức cổng kiểm soát, không giới thiệu hệ thống phân loại mức độ nghiêm trọng mới.

### 2.5 Đánh giá chỉ quan sát, không can thiệp luồng điều khiển

Đánh giá tái sử dụng `diag`, nhưng **loại bỏ `Action` và `Planner` của diag** — đó là thứ của luồng điều khiển runtime. Trong ngữ cảnh đánh giá `diag.Report` chỉ lấy `Stats` và `Findings`, Action đều bỏ qua. Đánh giá không tự động sửa prompt, không tự động rollback, không tiếp tục chạy. Đây là kỷ luật quan sát viên (`architecture.md` §2.3) được mở rộng sang ngữ cảnh đánh giá.

### 2.6 Thất bại phơi bày tường minh

Không mock thành công, không nuốt lỗi, không dùng template giả vờ qua. Mô hình, công cụ, cấu hình, hệ thống tệp, phân tích, judge bất kỳ cái nào thất bại, báo cáo ghi tường minh lý do. **Thất bại chính là kết quả đánh giá** — một case chạy crash, cổng kiểm soát chính là FAIL, không phải "bỏ qua".

### 2.7 Mỗi lần chỉ xác thực một biến số

Ràng buộc cứng của A/B: cùng yêu cầu, cùng cấu hình, cùng mô hình/provider, cùng phong cách, thư mục đầu ra cách ly. Baseline = prompt chính thức hiện tại, Variant = chỉ thay thế tệp prompt cần xác thực lần này. Một thí nghiệm không được đồng thời sửa Writer/Architect/Editor/Coordinator.

---

## 3. Toàn cảnh kiến trúc

```text
[Cases]  evals/cases/*.json —— tập khẳng định sự kiện, không phải hàng dataset thông dụng
   │
[Runner]  internal/eval —— lắp ráp in-process host điều khiển (dừng theo giới hạn số chương), ghi đè bộ nhớ bundle.Prompts làm variant
   │       baseline run ┐
   │       variant  run ┘  thư mục đầu ra cách ly riêng
   ▼
[Collectors]  Thu thập trên mỗi thư mục đầu ra:
   ├── diag.Diagnose(store)      → Report{Stats, Findings}      (sự kiện + runtime)
   ├── stylestat.Compute(input)  → thống kê văn thể toàn tập    (cột sống hồi quy chất lượng)
   ├── khẳng định hợp đồng case  → checkpoint/phase/hợp đồng công cụ dự kiến (không diag bao phủ)
   ├── usage / cost / token      → đọc từ meta/usage.json
   └── tool_calls                → đọc lời gọi công cụ thực tế từ meta/sessions/*.jsonl
   ▼
[Graders]
   ├── Cổng xác định: Finding.Severity + khẳng định hợp đồng → hard_fail / regression
   ├── stylestat delta: chênh lệch chỉ số văn thể variant vs baseline
   ├── LLM Judge (tùy chọn): so sánh A/B rubric bảy chiều
   └── Human: con người đọc sản phẩm baseline/variant
   ▼
[Report]  report.json (máy đọc) + report.md (người đọc) + xuất hành vi khử nhạy cảm
   └── Gate: PASS / WARN / FAIL
```

Hướng phụ thuộc: `eval → host → agents → tools → store → domain`, tái sử dụng ngang `diag` / `stylestat`. Tầng đánh giá **không phụ thuộc ngược** luồng điều khiển runtime, chỉ đọc Store và bộ đánh giá chỉ đọc.

> **Triển khai hiện tại bao phủ luồng chính xác định**: Không có `--variant` là `mode=single`; truyền `--variant` là `mode=ab`, cùng một case chạy cách ly baseline và variant, và tạo delta. Collectors đã kết nối `diag.Diagnose`, khẳng định hợp đồng case, `stylestat.Compute`, `meta/usage.json`, đếm tool call của session; Graders đã kết nối cổng xác định, diag delta baseline/variant, cost/token/tool call delta, stylestat delta. Runner trực tiếp `host.New` lắp ráp và tự có tính năng dừng theo giới hạn số chương, **không tái sử dụng `headless.Run`** (sau này không có giới hạn số chương, và sẽ đặt handler ask_user tương tác). LLM Judge và Human vẫn là tầng tùy chọn theo sau, không tham gia cổng xác định hiện tại.

---

## 4. Tại sao là Go in-process, không phải shell + Python

| Chiều | shell chép source + Python phân tích (đường cũ) | Go in-process (thiết kế này) |
|---|---|---|
| Kiểm tra xác định | Python phân tích lại JSON, hai định nghĩa với quy tắc diag | Trực tiếp `diag.Diagnose(store)`, một định nghĩa |
| Chuyển đổi variant | Chép toàn bộ cây source + biên dịch lại hai binary | `bundle.OverridePrompt(...)` ghi đè bộ nhớ rồi lắp ráp host, không chép không biên dịch lại |
| Hồi quy văn thể | Cần viết lại logic tách câu tiếng Trung của stylestat trong Python | Trực tiếp `stylestat.Compute` |
| Judge rubric | Chiều rải rác trong Python | Tái sử dụng `domain.DimensionScore`, cùng nguồn với bản online |
| Rủi ro sai lệch | Cao: runtime sửa mô hình sự kiện, đánh giá không theo | Thấp: biên dịch đã phơi bày thay đổi trường |

`prompt_ab.sh` cũ phải chép source biên dịch lại vì prompt được nhúng vào binary (`go:embed`). Nhưng `assets.Bundle.Prompts` là struct thông thường, **runner sửa một trường trong bộ nhớ là có thể làm variant**, hoàn toàn không cần chép source. Đây là sự đơn giản hóa lớn nhất có được khi dùng Go viết harness.

> **Ràng buộc triển khai**: `assets.Load` qua `loadPrompts` thống nhất nối thêm hậu tố `withSimulationGuidance` cho prompt cốt lõi (coordinator/architect/writer/editor), mà `withSimulationGuidance`/`loadPrompts` đều là **không export** — `internal/eval` không thể gọi trực tiếp. Nếu variant chỉ nhét văn bản thô vào `bundle.Prompts.Writer`, thì mất hậu tố hồ sơ mô phỏng mà baseline có, A/B không tương đương.
>
> Cách làm đúng là **thêm helper export ghi đè trong gói `assets`** (như `assets.OverridePrompt(b *Bundle, role, raw string)` hoặc export `assets.WithSimulationGuidance(raw, role) string`), nội bộ đi qua cùng cách bao bọc với `Load`; eval gọi nó, không sao chép logic bao bọc. Điều này tuân thủ nguyên tắc "thiếu tính năng thì vào nguồn thêm, không viết bản vá dự phòng ở tầng ứng dụng" của dự án.

> Phiên bản tài liệu trước giữ lại `prompt_ab.sh` / `prompt_ab_report.py` và "dần dần trích xuất năng lực". Thiết kế này từ bỏ con đường đó: vấn đề chúng giải quyết (chạy cách ly + tổng hợp chỉ số) là tập con trong Go harness in-process, buộc tái sử dụng ngược lại mang theo keo giao diện ba ngôn ngữ shell/Python/Go. **Go harness là con đường chính duy nhất**; Go harness hiện tại đã bao phủ chạy cách ly baseline/variant, tổng hợp repeat và delta xác định. Script cũ (`scripts/prompt_ab.sh`, `scripts/prompt_ab_report.py`) và hướng dẫn vận hành `docs/prompt-ab.md` đã được xóa cùng với việc triển khai thiết kế này, không còn được giữ lại.

---

## 5. Case Manifest

Case là đơn vị nhỏ nhất của đầu vào đánh giá, cũng là một tập **khẳng định sự kiện**. Dùng JSON mô tả, tránh quy tắc rải rác trong tham số dòng lệnh.

```json
{
  "id": "writer_first_chapter_xianxia",
  "category": "smoke",
  "role": "writer",
  "description": "Xác thực chất lượng nội dung chương đầu tiên của Writer và tính ổn định chuỗi công cụ",
  "prompt": "Viết một tiểu thuyết tu tiên dài, nhân vật chính xuất phát từ tạp dịch vùng biên thành, nhờ ký ức bất thường phá giải án cũ tông môn và cuốn vào cục trường sinh.",
  "style": "fantasy",
  "max_chapters": 1,
  "target_prompts": ["writer.md"],
  "rubric": "writer_chapter",

  "expect": {
    "phase": "writing",
    "min_completed_chapters": 1,
    "required_checkpoints": ["chapter:1:plan", "chapter:1:draft", "chapter:1:commit"],
    "no_pending": ["pending_commit", "pending_steer"]
  },

  "gate": {
    "max_severity": "warning",
    "max_cost_delta_ratio": 0.3,
    "max_tool_call_delta_ratio": 0.3,
    "stylestat_regression": "warn"
  }
}
```

**Ngữ nghĩa trường**:

- `expect`: Khẳng định hợp đồng cấp case, **chỉ khai báo kỳ vọng liên quan mạnh đến case này mà quy tắc chung của diag không bao phủ** (ví dụ "smoke case này phải tạo ra chính xác chapter:1:commit"). Các điều chung như "không còn pending / phase-flow nhất quán / không có khoảng trống chương" giao cho diag, không khai báo lại trong case.
- `category`: Tầng đánh giá ∈ `smoke` / `workflow` / `quality` / `longform` / `recovery` / `steering`. Quyết định chạy bộ cổng kiểm soát nào và mặc định có bật stylestat/Judge không.
- `role`: Vai được kiểm tra ∈ `writer` / `architect` / `editor` / `coordinator`. Trực giao với `category` — tầng quyết định "xác thực đến độ sâu nào", vai quyết định "xác thực subagent nào". Tầng Workflow chọn tập khẳng định theo `role`.
- `max_severity`: Mức độ nghiêm trọng cao nhất cho phép của diag Finding. Vượt quá là hard fail.
- `gate.max_cost_delta_ratio` / `gate.max_tool_call_delta_ratio`: Ngưỡng mức tăng chi phí và lời gọi công cụ của variant so với baseline; mặc định `0.3` khi bỏ qua, `0` tường minh nghĩa là không cho tăng, số âm nghĩa là tắt delta gate đó.
- `rubric`: Bật bảng điểm LLM Judge phiên bản nào. Thiếu thì không chạy Judge.
- `gate.stylestat_regression`: `block` / `warn` / `off`, kiểm soát hồi quy văn thể có chặn không (chỉ có hiệu lực với case ≥5 chương).

---

## 6. Phân tầng đánh giá

Mỗi tầng rõ ràng **dùng bộ đánh giá nào đã có**, tránh "tầng đánh giá tự mình lại viết một lần phán xét".

### 6.1 Smoke (phải chạy mỗi lần sửa prompt, tập tối thiểu)

Chỉ phán xét hệ thống có còn chạy ổn định không, không phán xét văn bút. 1 chương / giai đoạn lập kế hoạch là đủ để phơi bày.

| Case | Mục tiêu | Bộ đánh giá chính |
|---|---|---|
| `writer_first_chapter` | Writer hoàn thành và commit chương đầu tiên | `expect.required_checkpoints` + diag |
| `architect_short` | Lập kế hoạch ngắn lưu đủ premise/outline/characters/world_rules | Kiểm tra foundation cùng nguồn với `MissingSummaries` của diag + `expect` |
| `architect_long` | Lập kế hoạch dài lưu layered_outline/compass, triển khai cung đầu | `OutlineExhausted`/`CompassDrift` của diag + `expect` |
| `editor_review` | Đến điểm xét duyệt Editor lưu review (đủ bảy chiều) | Khẳng định trường `ReviewEntry` |

Chi phí: 1 chương × baseline+variant, cấp giây đến phút, không bật Judge, không chạy stylestat (số chương không đủ 5, `Compute` trả về nil). CI mặc định chỉ chạy tầng này.

### 6.2 Workflow (xác thực hành vi Agent tuân thủ hợp đồng kiến trúc)

**Kỷ luật quan trọng: Khẳng định hợp đồng, không khẳng định chuỗi công cụ chính xác.** Kiến trúc đặt cược LLM tự chủ quyết định quy trình (`architecture.md` §2.1), viết cứng thứ tự công cụ sẽ giới thiệu lại "hard-code hành vi LLM" đã bị §10.13 từ chối ở tầng đánh giá. Nên đây chỉ khẳng định **sự kiện tất yếu**:

- Writer: Checkpoint `chapter:N:commit` tồn tại; subagent kết thúc lượt sau commit (không có nội dung đuôi dài theo sau); checkpoint draft trước checkpoint commit. **Không** khẳng định "phải theo thứ tự chính xác novel_context→read_chapter→plan→draft→check→commit".
- Architect: Outline trong giai đoạn viết chỉ tăng không ghi đè toàn bộ (checkpoint của `expand_arc`/`append_volume`, không có dòng thứ hai `layered_outline` ghi đầy đủ); số chương outline phẳng và layered nhất quán sau khi triển khai.
- Editor: `ReviewEntry.Verdict` hợp lệ (accept/polish/rewrite); rewrite/polish phải tạo ra affected chapters; cuối cung có checkpoint `arc_summary`, cuối tập có checkpoint `volume_summary`.
- Coordinator: Sau khi nhận `[Host ra lệnh]` lần gọi subagent tiếp theo khớp với agent của lệnh (đọc từ session trace, `repeatedErrors` của diag dự phòng vòng lặp).

Những điều này phần lớn có thể trực tiếp bao phủ bởi quy tắc diag + khẳng định checkpoint, một số ít (nội dung đuôi sau commit) cần thêm một kiểm tra trace nhẹ trong collector.

### 6.3 Quality (chỉ chạy sau khi quy trình thông, đánh giá chất lượng nội dung)

Hai chân:

1. **stylestat delta (xác định, luồng chính)**: Chênh lệch chỉ số văn thể variant vs baseline. Đây là bằng chứng cứng của hồi quy chất lượng. **Yêu cầu case chạy đủ ≥5 chương** (nếu không `Compute` trả về nil, mục này đánh dấu `insufficient_sample`), nên case Quality thuần 1 chương không lấy được hồi quy văn thể, cần đặt `max_chapters` lên 5 trở lên.
2. **LLM Judge (bổ trợ)**: So sánh A/B rubric bảy chiều (xem §8).

Chỉ case đã qua §6.1/§6.2 mới vào Quality — quy trình không đúng thì nói chất lượng không có ý nghĩa.

### 6.4 Longform & Recovery (thay đổi lớn / chạy hàng đêm)

Không cần chạy mỗi lần. Bao phủ tính ổn định tác phẩm dài và khả năng khôi phục, chính là sân chơi chủ yếu của quy tắc runtime và quy tắc context của diag:

- Viết liên tục 3 chương / 5 chương → `GhostCharacter`/`TimelineGaps`/`RelationshipStagnation`/`ChapterGaps` của diag + lặp chéo chương stylestat.
- Xét duyệt cuối cung + triển khai cung tiếp theo → `OutlineExhausted`/`StaleForeshadow`/`CompassDrift`.
- Người dùng can thiệp giữa chừng (steering case) → user_rules có rơi vào `meta/user_rules.json` không, các chương tiếp theo có tuân thủ không.
- Phục hồi crash: chạy đến draft chương N rồi kill → Resume → diag xác nhận `checkpoints.jsonl` không có bước trùng lặp, không viết lại bản nháp đã ghi xuống đĩa, `pending_commit` cuối cùng về 0.
- Công cụ gọi phình to / chi phí bất thường → `repeatedErrors`/`stuckStep`/`streamIdleStorm` của diag + usage delta.

---

## 7. Cổng xác định

Mức cổng kiểm soát được suy ra trực tiếp từ **Severity của diag Finding** + **khẳng định hợp đồng case**, không đặt ra hệ thống phân loại mới.

### 7.1 Hard Fail (chặn hợp nhất)

- Tiến trình panic / headless trả về error.
- diag tạo ra Finding `SevCritical` (`InvalidPendingRewrites` / `PhaseFlowMismatch` v.v.).
- Khẳng định hợp đồng `expect` của case thất bại: thiếu commit checkpoint, phase chưa đạt kỳ vọng, pending đã khai báo chưa về 0.
- Số lỗi / số Critical Finding của variant nhiều hơn baseline (hồi quy tệ hơn).

### 7.2 Regression (mặc định warning, có chặn không do gate của case quyết định)

- diag tăng thêm Finding `SevWarning` (variant nhiều hơn baseline).
- Tool calls / cost / input token / output token tăng vượt ngưỡng case (mặc định 30%).
- **Hồi quy stylestat**: Mẫu câu trung bình/chương tăng, tỷ lệ câu ngắn cuối chương tăng, câu lặp chéo chương tăng, tiêu đề lẫn lộn xuất hiện — theo `gate.stylestat_regression` quyết định warn/block.
- Số chữ chương thấp hơn 60% baseline hoặc cao hơn 180% (ngưỡng cùng nguồn với `WordCountAnomaly` của diag).

### 7.3 Quality Gate (con người dự phòng)

- LLM Judge chỉ làm bổ trợ và sắp xếp.
- Judge phán variant tệ hơn rõ ràng → phải có con người đọc mẫu xác nhận.
- Con người đọc mẫu xác định suy giảm → chặn.
- Judge phán variant tốt hơn nhưng xác định có hard fail → vẫn chặn.

### 7.4 Điều kiện hợp nhất được đề xuất

Sửa prompt thường ngày: Smoke toàn thông + Workflow mục tiêu vai toàn thông (Smoke 1 chương không có hồi quy văn thể; nếu đã chạy case Quality ≥5 chương, thì stylestat không có hồi quy đáng kể).
Thay đổi lớn: Thêm 2-3 case Quality + 1-2 case Longform + đọc mẫu thủ công.

---

## 8. LLM Judge

Judge là bổ trợ chất lượng, bản chất là **dùng rubric có phiên bản (ban đầu suy ra từ bảy chiều của editor.md) để so sánh A/B offline baseline/variant**. Rubric là thước chuẩn cố định, phát triển độc lập với `editor.md` online (lý do xem §2.3), báo cáo ghi lại phiên bản rubric đã dùng.

### 8.1 Đầu vào (kiểm soát kích thước, tuyệt đối không nhồi cả cuốn sách)

- Yêu cầu gốc của người dùng + dàn ý/hợp đồng chương hiện tại.
- Nội dung **cùng chương** của baseline và variant.
- Tóm tắt 1-2 chương gần nhất + tóm tắt trạng thái nhân vật (đọc từ store).
- Lát cắt stylestat liên quan của chương đó (để Judge thấy sự kiện như "câu này lặp lại 7 lần trong toàn tập").

### 8.2 Đầu ra (có cấu trúc, căn chỉnh bảy chiều)

```json
{
  "scores": {
    "consistency": 8, "character": 7, "pacing": 8, "continuity": 8,
    "foreshadow": 7, "hook": 7, "aesthetic": 6
  },
  "winner": "variant",
  "confidence": "medium",
  "reasons": ["variant thúc đẩy hành động tập trung hơn", "baseline phức thuật tiền tình nặng hơn"],
  "risks": ["variant phục bút động cơ nhân vật phụ hơi ít"]
}
```

- Chiều nghiêm ngặt bằng bảy mục của `domain.DimensionScore`, mỗi mục 0-10.
- `winner` ∈ baseline/variant/tie; `confidence` ∈ low/medium/high.
- `reasons`/`risks` mỗi câu ≤ 80 chữ, trích nguyên văn phải ngắn.

### 8.3 Ranh giới

Judge **không thể**: Quyết định quy trình có thông qua không, sửa đổi sản phẩm, tự động sửa prompt, là căn cứ hợp nhất duy nhất, tạo ra trích dẫn nguyên văn dài.
Judge **có thể**: Sắp xếp thứ tự cho đánh giá thủ công, đánh dấu suy giảm rõ ràng, tóm tắt sự khác biệt A/B, phơi bày tác dụng phụ của thay đổi prompt.

---

## 9. Báo cáo

Mỗi thí nghiệm tạo ra `report.json` (máy đọc, có thể tái tạo markdown) + `report.md` (người đọc) + `artifacts/{case_id}/{baseline,variant}/` (sản phẩm gốc). Khi có `--repeat N` đường dẫn là `artifacts/{case_id}/rN/{baseline,variant}/`.

### 9.1 Delta chỉ số

Báo cáo hiển thị sự khác biệt của variant so với baseline, giá trị tuyệt đối và tỷ lệ song song:

```text
completed: baseline=5 variant=5   ← ≥5 chương, chỉ số văn thể mới có ý nghĩa
tool_calls: baseline=12 variant=16  +4 (+33.3%)
cost_usd: baseline=0.42 variant=0.55  +0.13 (+31.0%)
output_tokens: baseline=8200 variant=9100  +900 (+11.0%)
critical_findings: baseline=0 variant=0
warning_findings: baseline=1 variant=2  +1
stylestat.pattern_top_per_chapter: baseline=3.1 variant=5.4  +2.3   ← hồi quy văn thể
stylestat.ending_short_ratio: baseline=0.42 variant=0.71  +0.29     ← đồng cấu cuối chương nặng hơn
```

### 9.2 Tổng hợp Repeat

Khi có `--repeat N` không chỉ xem lần cuối, triển khai hiện tại hiển thị tỷ lệ qua, số lần hard fail, số lần warning, min/avg/max của cost/tool_calls. Sau khi kết nối Judge thêm phân phối winner, tránh trộn nhiễu trọng tài mô hình vào báo cáo xác định mặc định.

```text
writer_first_chapter_xianxia repeat=3
- pass_rate: 3/3
- cost_usd: avg=0.41 min=0.38 max=0.44
- tool_calls: avg=13 min=12 max=15
- stylestat.pattern_top_per_chapter: avg delta=+0.4 (không có hồi quy đáng kể)
```

### 9.3 Báo cáo khả thi tối thiểu

```text
Gate: FAIL

Hard Fail:
- writer_first_chapter_xianxia: missing checkpoint chapter:1:commit

Warnings:
- writer_dialogue_density: tool_calls +35%
- writer_anti_ai_tone: ending_short_ratio +0.28 (hồi quy văn thể)

Quality:
- writer_anti_ai_tone: judge prefers variant, confidence=medium

Artifacts:
- workspace/evals/20260629-120000/report.json
```

---

## 10. Cấu trúc thư mục và lệnh

```text
internal/eval/
  case.go        Cấu trúc Case manifest + tải
  eval.go        Điều phối CLI: single / A/B / repeat
  runner.go      Lắp ráp host điều khiển (dừng theo giới hạn số chương + drain đến Done), ghi đè bộ nhớ bundle.OverridePrompt
  collect.go     Chạy diag.Diagnose + stylestat.Compute + usage/tool_calls + khẳng định hợp đồng trên thư mục đầu ra
  grade.go       Ánh xạ Finding→cổng kiểm soát + delta baseline/variant + quyết định stylestat gate
  report.go      report.json + report.md

cmd/ainovel-cli  Cổng vào lệnh con eval

evals/
  cases/         smoke/ workflow/ quality/ longform/ recovery/ steering/
  rubrics/       writer_chapter.json / architect_outline.json / editor_review.json
  variants/      writer-anti-ai-tone/writer.md v.v. (mỗi thư mục chỉ để tệp prompt cần thay thế)
  reports/       Lưu trữ báo cáo lịch sử
```

Lệnh:

```bash
# Nhiều case theo lô (CI mặc định chỉ chạy smoke, không bật judge)
ainovel-cli eval --cases evals/cases/smoke \
  --variant evals/variants/writer-anti-ai-tone \
  --out workspace/evals/writer-anti-ai-tone --ci
```

**Tham số đã triển khai kỳ này**: `--cases` (thư mục hoặc một manifest đơn), `--variant` (thư mục prompt variant; truyền vào tự động chạy baseline+variant A/B), `--repeat N` (mỗi case lặp N lần), `--config`, `--out`, `--max-chapters N` (ghi đè mặc định của case), `--timeout` (giới hạn giờ đồng hồ mỗi case), `--ci` (triệt tiêu đầu ra từng sự kiện; mã thoát không 0 là hard fail, không truyền cũng có hiệu lực).

**Đang lên kế hoạch (chưa triển khai, đừng dùng trên dòng lệnh, nếu không sẽ báo flag chưa định nghĩa)**: `--judge`/`--no-judge` (Phase 3 LLM Judge). Thay đổi prompt lớn hiện tại có thể dùng A/B xác định + repeat trước:

```bash
# Thay đổi prompt lớn: A/B + repeat giảm tính ngẫu nhiên
ainovel-cli eval --cases evals/cases/quality \
  --variant evals/variants/writer-anti-ai-tone \
  --repeat 3 --ci
```

---

## 11. Những điều rõ ràng không làm

Vi phạm đồng nghĩa đánh giá lệch định vị.

1. **Không sao chép logic chẩn đoán chung của diag ở tầng đánh giá** — Phán xét chung (pending còn sót, phase/flow nhất quán, khoảng trống chương, vòng lặp chết) đều đi qua `diag`, phán xét sự kiện chỉ có một định nghĩa. Khẳng định hợp đồng cấp case (`expect.required_checkpoints` v.v.) cho phép đọc trực tiếp API `store`/checkpoint, nhưng chỉ làm **khẳng định mỏng** — xác thực kỳ vọng cụ thể liên quan mạnh đến case này, tuyệt đối không viết lại quy tắc chung đã có của diag.
2. **Không tái triển khai quy tắc xác định** — diag đã có một tập quy tắc artifact + quy tắc runtime. Thiếu quy tắc thì vào diag thêm, tầng đánh giá chỉ tiêu thụ.
3. **Không viết lại logic văn thể tiếng Việt của stylestat trong Python** — Gọi trực tiếp gói Go.
4. **Không để LLM Judge quyết định quy trình có thông qua không** — Cổng kiểm soát chỉ nhận bằng chứng xác định.
5. **Không để đánh giá can thiệp luồng điều khiển** — Loại bỏ Action/Planner của diag, không tự động sửa prompt, không rollback, không tiếp tục chạy, không phát hành.
6. **Không khẳng định chuỗi lời gọi công cụ chính xác** — Chỉ khẳng định hợp đồng (commit xảy ra, checkpoint tồn tại), bảo vệ cược "LLM điều khiển quy trình".
7. **Không giới thiệu database / Web UI / nền tảng đánh giá online** — Giai đoạn hiện tại cần hồi quy địa phương có thể lặp lại, có thể triển khai, chi phí thấp.
8. **Không chép source biên dịch lại để làm variant** — Ghi đè bộ nhớ `bundle.Prompts`.
9. **Không mock thành công, không nuốt lỗi** — Bất kỳ khâu nào thất bại ghi tường minh, case chạy crash là FAIL.
10. **Case không thay đổi thường xuyên theo prompt** — Case là tập test ổn định; sửa case để variant thông qua là gian lận.

---

## 12. Triển khai theo giai đoạn

### Phase 1 · Runner + Cổng xác định (MVP, chứng minh giả thuyết trước)

- `internal/eval`: Cấu trúc Case + runner (in-process headless + ghi đè bundle) + collect (gọi `diag.Diagnose`) + grade (Finding→cổng kiểm soát + hợp đồng `expect`).
- `evals/cases/smoke/` để 3-4 case.
- Báo cáo xuất `report.json` + markdown tối thiểu trước.

**Nghiệm thu**: Một lệnh chạy xong smoke; Writer bỏ qua commit, pending còn sót, checkpoint thiếu, phase không đúng **đều có thể bị cổng kiểm soát chặn lại** (những điều này diag vốn đã kiểm tra được, xác thực là harness kết nối đúng).

### Phase 2 · A/B + repeat + hồi quy stylestat (đã triển khai)

- `--variant` tự động chạy baseline và variant, artifact đầu ra cách ly.
- `--repeat N` tổng hợp pass rate, số lần hard fail, số lần warning, min/avg/max của cost/tool_calls.
- collect thêm `stylestat.Compute`, grade thêm delta văn thể.
- Báo cáo hiển thị so sánh baseline-variant của mẫu câu/chương / tỷ lệ câu ngắn cuối chương / câu lặp chéo chương / tiêu đề lẫn lộn.

**Nghiệm thu**: Dùng một case ≥5 chương + một variant "tic câu nặng hơn", có thể bị hồi quy văn thể đánh dấu warning; case số chương không đủ hiển thị rõ ràng `insufficient_sample` thay vì nhầm phán qua.

### Phase 3 · LLM Judge

- `evals/rubrics/` + `judge.go`, so sánh A/B rubric bảy chiều.
- Judge thất bại (JSON không hợp lệ) → báo cáo ghi thất bại, không ảnh hưởng kết quả xác định.

**Nghiệm thu**: Đầu ra Judge vào json+md, và không làm ô nhiễm cổng xác định.

### Phase 4 · Longform & Recovery

- 3-5 chương liên tục / xét duyệt cuối cung / can thiệp người dùng / replay pending_commit / case áp lực nén ngữ cảnh.
- Tái sử dụng quy tắc context+runtime của diag.

**Nghiệm thu**: Có thể phát hiện dòng thời gian lặp, pending còn sót, thiếu tóm tắt cuối cung, vòng lặp công cụ.

---

## 13. Quy chuẩn bảo trì Case

- **Kiềm chế số lượng**: Smoke 3-5, Workflow mỗi vai 3-5, Quality 2-4, Longform/Recovery mỗi loại 2-3. Quá nhiều không ai muốn chạy.
- **Case tốt**: Đầu vào ngắn gọn rõ ràng, bao phủ rủi ro thực tế, ít chương phơi bày vấn đề, không phụ thuộc mô hình tạo câu cố định, không viết sở thích phong cách quá tỉ mỉ.
- **Case xấu**: Đầu vào quá dài, nhiều mục tiêu đồng thời, cần chạy vài chục chương mới phán xét được, chỉ dựa vào cảm nhận chủ quan.
- **Đặt tên Variant**: `writer-anti-ai-tone` / `architect-rolling-outline` / `editor-strict-review`, mỗi thư mục chỉ để tệp prompt cần thay thế.

---

## 14. Rủi ro và ranh giới

- **Tính ngẫu nhiên của mô hình**: Cùng prompt chạy nhiều lần cũng thay đổi. Thay đổi quan trọng thì `--repeat 3` xem xu hướng.
- **Chi phí**: Judge và longform đốt tiền. Mặc định địa phương chỉ chạy **smoke** (1 chương × baseline+variant, cổng diag xác định, không bật Judge, không chạy stylestat); **stylestat chỉ bật ở Quality/Longform ≥5 chương** (số chương smoke không đủ, `Compute` trả về nil, báo cáo đánh dấu `insufficient_sample`); bộ suite đầy đủ để dành cho thay đổi lớn.
- **Thiên vị Judge**: Judge cũng là mô hình, ưa thích văn bản giải thích gọn gàng, chưa chắc tương đương với tiểu thuyết hay đọc — nên chỉ làm bổ trợ, stylestat là luồng chính xác định.
- **Chỉ số hóa quá mức**: Số chữ/số lần công cụ/chi phí/thống kê văn thể đều là tín hiệu không phải mục tiêu. Số stylestat có thành bệnh hay không do con người phán xét theo thể loại, **ngưỡng không hard-code** (nhất quán với editor.md).
- **Không tự động rollback online**: Công cụ hồi quy offline, không chịu trách nhiệm tự động sửa prompt / phát hành online.

---

## 15. Tóm tắt

Giá trị của hệ thống đánh giá này không phải tự động phán xét chất lượng văn học, mà là biến sửa đổi prompt từ "cảm tính" thành "có hồi quy, có bằng chứng, có đọc mẫu thủ công".

Sự khác biệt căn bản với thiết kế phiên bản trước chỉ có một câu: **Bộ đánh giá đã có trong codebase.** `diag` là bộ chẩn đoán sự kiện xác định, `stylestat` là bộ hồi quy văn thể toàn tập, bảy chiều `ReviewEntry` là rubric nguyên bản. Việc hệ thống đánh giá cần làm là một lớp Go harness mỏng — thúc đẩy theo lô, thu thập, ánh xạ Finding và thống kê thành cổng kiểm soát, tổng hợp báo cáo — thay vì dùng ngôn ngữ khác viết lại những phán xét sự kiện này một lần nữa.

Một định nghĩa sự kiện, không bao giờ sai lệch. Đây chính là kỷ luật xuyên suốt từ kiến trúc đến đánh giá của dự án này: **harness tối thiểu, tái sử dụng tối đa, xác định giao code, phán xét giao LLM và con người.**

---

## 16. Tham khảo

Cấu trúc chung của eval LLM trong ngành (dataset / experiment / scorer / trace / regression gate) là nguồn cảm hứng tư tưởng của thiết kế này, nhưng **cố ý không copy nguyên xi** — "scorer" của dự án này là `diag`/`stylestat` đã có, "trace" là tầng sự kiện checkpoint/session đã có, "dataset" là case khẳng định sát tầng sự kiện.

- OpenAI Evals · https://developers.openai.com/api/docs/guides/evals (Lưu ý: nền tảng Evals lưu trữ của họ đã công bố lịch trình ngừng hoạt động, chỉ tham khảo **tư tưởng** kiểm tra có cấu trúc/tự động chấm điểm/hiệu chỉnh thủ công, không làm phụ thuộc tương lai)
- Braintrust · https://www.braintrust.dev/foundations/what-is-an-eval
- LangSmith · https://docs.langchain.com/langsmith/evaluation-concepts

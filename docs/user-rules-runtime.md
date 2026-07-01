# Thiết Kế Thống Nhất Quy Tắc Người Dùng

## Một câu tóm tắt

Tất cả quy tắc viết lâu dài đều được chuẩn hóa vào một snapshot quy tắc duy nhất của cuốn sách hiện tại; trong quá trình chạy, chỉ dùng `novel_context` để inject snapshot này, không còn nhồi văn bản quy tắc gốc vào prompt nhiều lần nữa.

```text
Prompt khởi động / File rules của người dùng / Yêu cầu lâu dài trong lúc chạy
        ↓
LLM chuẩn hóa ngữ nghĩa (theo từng nguồn)
        ↓
Go hợp nhất xác định (theo thứ tự ưu tiên)  ←  Quy tắc mặc định hệ thống (built-in trong code, vào thẳng hợp nhất, không qua LLM)
        ↓
output/novel/meta/user_rules.json
        ↓
novel_context inject
        ↓
Architect / Writer / Editor / commit kiểm tra dùng chung
```

## Trạng thái triển khai (2026-06-28, đã triển khai + sửa lỗ hổng sau review)

Thiết kế này đã được triển khai, `go build` / `go vet` / `go test` trên 24 package đều xanh. Sau một vòng code review đã sửa 4 lỗ hổng (tất cả đã khắc phục): ①Quy tắc prompt khởi động chỉ được hook vào method chết `Host.Start`, entry point thực tế dùng `StartPrepared` nên bị bỏ sót khi tạo snapshot — đã chuyển prompt gốc qua `Plan.RawPrompt` đến hai đường vào quick/cocreate, thống nhất gọi `Host.PrepareUserRules`; ②Ghi snapshot thất bại bị nuốt im — `PrepareUserRules` đã đổi thành: ghi thất bại thì return error dừng mở sách (đường resume giữ nguyên best-effort để tránh đưa vào lỗi mới cho sách cũ); ③Lỗi đọc file rules bị bỏ qua im lặng — `raw.go` đã log lỗi các trường hợp không phải "không tồn tại" (như lỗi quyền truy cập); ④README vẫn hướng dẫn YAML/front matter cũ và link đến file đã xóa — đã viết lại.

Triển khai cơ bản khớp với tài liệu này, hai điểm lựa chọn triển khai khác với mô tả chữ được ghi lại dưới đây:

1. **Chuẩn hóa không phải "lời gọi bị ràng buộc bởi provider schema", mà là "chỉ thị JSON trong prompt + xác thực phía Go".**
   Lý do: Structured output của litellm phụ thuộc từng provider (OpenAI/Gemini hỗ trợ JSONSchema, Anthropic không hỗ trợ),
   để nhất quán xuyên provider, chuẩn hóa dùng system prompt thỏa thuận hình dạng JSON + phía Go parse/xác thực kiểu/sanitize miền giá trị làm dự phòng,
   không gửi JSONSchema xuống mô hình. "Ràng buộc schema" trong phần dưới đều theo nghĩa này (xác thực schema phía Go, không phải ràng buộc phía provider).
2. **Khi giá trị một trường không hợp lệ, hạ cấp xuống "coi trường đó thiếu", không hạ cấp toàn bộ nguồn.**
   Ví dụ `chapter_words` xuất hiện `min>max`, sanitize loại bỏ trường đó (coi như chưa khai báo), giữ lại các trường hợp lệ khác của nguồn;
   chỉ khi "toàn bộ chuẩn hóa thất bại" (mạng/mô hình/JSON không hợp lệ/parse thất bại) mới hạ cấp toàn bộ nguồn thành raw preferences,
   đặt `status=degraded`. Như vậy một trường xấu không kéo theo các quy tắc hợp lệ khác của cùng nguồn. Lỗi kỹ thuật vào log,
   retry có giới hạn 1 lần rồi hạ cấp (`normalizeMaxAttempts=2`).

Vị trí code: `internal/rules` (dữ liệu thuần túy + hợp nhất xác định: snapshot.go / raw.go / types.go), `internal/userrules`
(LLM chuẩn hóa + điều phối + ghi disk: normalize.go / service.go), `internal/store/user_rules.go` (lưu trữ snapshot),
`internal/tools/save_user_rules.go` (vỏ công cụ lúc chạy), `assets/prompts/coordinator.md` (phân luồng ba loại).
Quy tắc mặc định hệ thống đã được di chuyển từ `assets/rules/default.md` vào `rules.SystemDefaults()` built-in trong code, đường parse YAML và
dependency yaml.v3 đã xóa. **Chưa xác minh**: Quy trình đầy đủ mở sách bằng LLM thực / `save_user_rules` lúc chạy (prototype offline của normalizer đã xác minh 10/10).

## Lý Do

Writer mỗi chương không nhận được ổn định prompt đầy đủ ban đầu của người dùng. Nó chủ yếu dựa vào nhiệm vụ chương hiện tại và `novel_context(chapter=N)`.

Vì vậy quy tắc lâu dài không thể dựa vào bộ nhớ lịch sử hội thoại, cũng không nên dùng regex lén đoán từ ngôn ngữ tự nhiên. Cách đúng là: chuẩn hóa tường minh quy tắc lâu dài thành trạng thái, rồi để `novel_context` phân phát thống nhất.

"Chuẩn hóa" ở đây bắt buộc phải tận dụng năng lực hiểu ngôn ngữ tự nhiên của mô hình lớn, không phải liệt kê các cách diễn đạt trong Go. Code chỉ định nghĩa một số ít trường có thể kiểm tra máy móc, chịu trách nhiệm schema, hợp nhất xác định, xác thực, ghi disk và kiểm tra commit; "mỗi chương khoảng một nghìn rưỡi chữ", "đừng vượt quá hai nghìn mỗi chương", "không viết kiểu 'bánh xe số phận' nữa" — loại diễn đạt này do LLM hiểu ngữ nghĩa.

## Trạng Thái Thống Nhất

Trong quá trình chạy, cuốn sách chỉ duy trì một nguồn dữ liệu thực tế quy tắc người dùng:

```text
output/novel/meta/user_rules.json
```

Hình dạng giữ đơn giản:

```json
{
  "version": 1,
  "status": "ready",
  "structured": {
    "genre": "tiên hiệp",
    "chapter_words": {"min": 1200, "max": 1600},
    "forbidden_chars": [],
    "forbidden_phrases": ["ở một mức độ nào đó"],
    "fatigue_words": {}
  },
  "preferences": "Nhân vật chính lạnh lùng kiềm chế; ít giải thích, nhiều hành động và đối thoại.",
  "sources": [
    "startup_prompt",
    ".ainovel/rules/style.md"
  ],
  "uncertain": [
    "Ít dùng ẩn dụ: không có ngưỡng rõ ràng, xử lý như sở thích phong cách"
  ]
}
```

Ranh giới các trường:

- `version`: Phiên bản schema snapshot, tiện cho việc di chuyển về sau.
- `status`: `ready` / `degraded`, đánh dấu chuẩn hóa có thành công hoàn toàn không; chỉ dùng để hiển thị và chẩn đoán, không đi vào phán đoán sáng tác.
- `structured`: Quy tắc có thể kiểm tra máy móc hoặc tiêu thụ ổn định bởi code.
- `preferences`: Sở thích ngôn ngữ tự nhiên không thể kiểm tra máy móc nhưng có hiệu lực lâu dài cho sáng tác.
- `sources`: Kiểm tra nguồn gốc, không đi vào phán đoán sáng tác.
- `uncertain`: Chẩn đoán chuẩn hóa, chỉ dùng để hiển thị và kiểm tra, không đi vào phán đoán sáng tác.

Chỉ inject `structured` và `preferences` cho mô hình; `version` / `status` / `sources` / `uncertain` là metadata vận hành và chẩn đoán, không vào `working_memory.user_rules`. Lỗi kỹ thuật không vào snapshot, chỉ vào log (xem §Thất bại và hạ cấp).

## Nguồn Đầu Vào

Quy tắc lâu dài có bốn nguồn đầu vào:

1. **Prompt khởi động**: Yêu cầu lâu dài người dùng viết lúc mở sách.
2. **File rules của người dùng**: Sở thích lâu dài cấp toàn cục hoặc theo dự án, đọc dưới dạng ngôn ngữ tự nhiên thông thường.
3. **Quy tắc mặc định hệ thống**: Baseline máy móc built-in trong code.
4. **Yêu cầu lâu dài lúc chạy**: Người dùng nói "từ nay trở đi đều như vậy" giữa chừng, qua `save_user_rules` đi vào.

Các nguồn đầu vào này không đi thẳng vào prompt của Writer, cũng không được đọc lại nhiều lần trong lúc chạy. Chúng chỉ tham gia chuẩn hóa khi tạo hoặc cập nhật snapshot, kết quả được hợp nhất vào `meta/user_rules.json`.

## File Rules

File rules là prompt dài hạn thông thường, không phải prompt runtime, cũng không phải file cấu hình. Nó chỉ dùng làm đầu vào chuẩn hóa, không hỗ trợ YAML:

```md
# Sở thích viết

Mỗi chương 1200-1600 chữ.
Nhân vật chính lạnh lùng kiềm chế, không thiên vị.
Ít giải thích, nhiều hành động và đối thoại.
Không dùng câu "theo một nghĩa nào đó".
```

Hệ thống đọc xong chuẩn hóa thành:

```json
{
  "structured": {
    "chapter_words": {"min": 1200, "max": 1600},
    "forbidden_phrases": ["ở một mức độ nào đó"]
  },
  "preferences": "Nhân vật chính lạnh lùng kiềm chế, không thiên vị; ít giải thích, nhiều hành động và đối thoại."
}
```

Nếu file có YAML front matter, cũng xử lý như văn bản thông thường, không coi là khai báo cấu trúc. Kết quả cấu trúc chỉ đến từ quy trình chuẩn hóa thống nhất.

Sau khi khởi động, nếu người dùng sửa file rules, cuốn sách hiện tại sẽ không tự động thay đổi; cần tạo lại snapshot. Như vậy sách cũ không bị trượt hành vi do file rules toàn cục thay đổi.

## Chuẩn Hóa Ngữ Nghĩa

Chuẩn hóa là lời gọi LLM độc lập, bị ràng buộc bởi schema — mỗi nguồn chuẩn hóa một lần riêng, không trộn vào quá trình tạo sáng tác, cũng không dùng regex hoặc bảng từ khóa để parse cứng.

Đầu vào:

- Văn bản gốc của một nguồn duy nhất (prompt khởi động / một file rules / một yêu cầu lúc chạy)
- Mô tả các trường `structured` mà hệ thống hiện hỗ trợ

Quy tắc mặc định hệ thống không nằm trong danh sách này — chúng là quy tắc cấu trúc đã biên dịch built-in trong code, đi thẳng vào §Quy tắc hợp nhất, không qua normalizer.

Đầu ra:

- `structured` ứng viên của nguồn đó
- `preferences` ứng viên của nguồn đó
- `sources`
- `uncertain`

Trách nhiệm phía Go:

- Cung cấp schema.
- Xác thực kiểu trường và miền giá trị.
- Theo thứ tự ưu tiên của §Quy tắc hợp nhất, hợp nhất xác định từng nguồn (LLM không phán xét độ ưu tiên nguồn).
- Lưu snapshot.
- Inject snapshot trong `novel_context`.
- Dùng cùng snapshot đó để kiểm tra máy móc trong `commit_chapter`.

Trách nhiệm phía LLM:

- Hiểu quy tắc ngôn ngữ tự nhiên của nguồn duy nhất.
- Nâng các quy tắc rõ ràng, có thể kiểm tra máy móc lên `structured`.
- Giữ lại sở thích thẩm mỹ, phong cách, nhân vật trong `preferences`.
- Bảo thủ với nội dung không chắc chắn, không tự phát minh ngưỡng.

### Nâng cấp bảo thủ

`structured` là quy tắc cứng hoặc tham số ổn định, không phải "vùng mô hình đoán mò". Quy tắc nâng cấp phải bảo thủ:

- Chỉ khi người dùng diễn đạt rõ ràng, không mơ hồ thì mới ghi vào `structured`.
- `forbidden_chars` / `forbidden_phrases` là trường cấp error, phải đặc biệt bảo thủ; chỉ những từ ngữ cấm tường minh như "không được xuất hiện X", "cấm X", "đừng viết X" mới được nâng cấp.
- `fatigue_words` chỉ nâng cấp khi người dùng cho biết từ và ngưỡng rõ ràng; các yêu cầu không có ngưỡng như "ít dùng ẩn dụ", "đừng quá sách vở", "giảm câu cửa miệng" thì vào `preferences`.
- `chapter_words` chỉ nâng cấp khi người dùng cho biết khoảng, giới hạn trên, giới hạn dưới hoặc số chữ mục tiêu rõ ràng; yêu cầu mơ hồ như "ngắn thôi", "nhịp độ nhanh hơn" thì vào `preferences`.
- Yêu cầu không thể máy móc hóa, không có ngưỡng rõ ràng, phụ thuộc ngữ cảnh đều vào `preferences`.

Nguyên tắc:

```text
Thà bỏ sót xuống structured, hạ cấp thành soft preference;
Không được nhầm vào structured, tạo ra lỗi cứng mỗi chương.
```

Hậu quả của bỏ sót là sở thích phong cách yếu hơn một chút; hậu quả của nhầm là mỗi chương phát sinh dữ liệu quy tắc lỗi.

## Thất Bại và Hạ Cấp

Chuẩn hóa là đường tăng cường, không phải điều kiện tiên quyết của sáng tác chính. Mô hình hiểu thất bại, tuyệt đối không được ngăn việc viết sách.

- **Hạ cấp theo nguồn**: Khi một nguồn chuẩn hóa thất bại (mạng / mô hình / JSON không hợp lệ / xác thực schema thất bại), nguồn đó hạ cấp thành raw preferences, không tạo ra `structured`; các nguồn thành công khác vẫn đóng góp `structured` bình thường.
- **Retry có giới hạn**: Thất bại có thể retry hữu hạn lần (ví dụ 1 lần), vẫn thất bại thì hạ cấp, không retry vô hạn.
- **Lỗi kỹ thuật vào log**: JSON / schema / mạng và các lỗi kỹ thuật khác ghi vào log, không vào `working_memory.user_rules`, không dùng làm đầu vào sáng tác.
- **Đánh dấu snapshot**: Khi bất kỳ nguồn nào bị hạ cấp, snapshot `status=degraded`.
- **Miễn là ghi được thì tiếp tục**: Miễn là `meta/user_rules.json` có thể ghi vào, sáng tác chính phải tiếp tục.
- **Chỉ khi ghi disk thất bại mới dừng**: Chỉ khi snapshot không thể ghi vào disk mới dừng, vì lúc đó quá trình chạy tiếp theo không có nguồn dữ liệu ổn định.

Hợp đồng công cụ `save_user_rules` (lúc chạy): Luôn cố gắng hết sức trả về dữ liệu quy tắc; khi normalizer thất bại, lưu snapshot degraded, trả về `status=degraded`, **không ném lỗi kỹ thuật thành tool error cho Coordinator** (nếu không lỗi JSON/schema/mạng sẽ làm ô nhiễm luồng chính, lịch sử dự án đã từng có ô nhiễm loại này dẫn đến vòng lặp chết); chỉ khi ghi disk thất bại và các vấn đề không thể tiếp tục mới trả về tool error.

## Quy Tắc Mặc Định Hệ Thống

`System defaults` là baseline máy móc built-in trong code, không phải file rules của người dùng, cũng không dùng YAML.

Nó không qua LLM chuẩn hóa — đã ở dạng cấu trúc, đi thẳng vào §Quy tắc hợp nhất của Go với độ ưu tiên thấp nhất. Như vậy quy tắc mặc định không có vấn đề LLM thất bại, trượt hành vi, chi phí.

Quy tắc mặc định máy móc của hệ thống trước đây tạm lưu ở `assets/rules/default.md` (chi tiết triển khai cũ, không phải YAML người dùng cần tương thích); khi triển khai thiết kế này đã được chuyển vào `rules.SystemDefaults()` built-in trong code, đường parse YAML đã xóa (xem §Trạng thái triển khai).

Khi di chuyển, giữ lại comment cần thiết giải thích nguồn gốc ngưỡng, ví dụ một số ngưỡng fatigue word đến từ dữ liệu thực tế của truyện dài. Không phải để tương thích YAML cũ, mà để người bảo trì tương lai biết tại sao ngưỡng mặc định tồn tại, khi nào nên điều chỉnh.

## Quy Tắc Hợp Nhất

Thứ tự hợp nhất theo "càng cụ thể càng ưu tiên":

```text
System defaults
→ Kết quả biên dịch Global rules
→ Kết quả biên dịch Project rules
→ Kết quả biên dịch Startup prompt
→ Runtime user update
```

Nguồn có độ ưu tiên cao hơn ghi đè nguồn thấp hơn.

Hợp nhất do Go thực hiện xác định: LLM chỉ chuẩn hóa ngôn ngữ tự nhiên của một nguồn duy nhất thành `structured`/`preferences` ứng viên, Go thực hiện ghi đè trường và nối văn bản theo thứ tự trên, không giao độ ưu tiên cho LLM phán xét.

- `structured`: Ghi đè theo trường, trường trùng tên của nguồn sau ghi đè nguồn trước.
- `preferences`: Không ghi đè nhau, xếp theo thứ tự ưu tiên thành văn bản có thể đọc được (nguồn ưu tiên cao hơn ở sau), để LLM thấy được thứ tự nguồn.

Hạn chế đã biết: `preferences` được sắp xếp theo độ ưu tiên nhưng Go không giải quyết mâu thuẫn. Trong chạy dài nếu người dùng lần lượt đưa ra sở thích mâu thuẫn nhau (ví dụ trước "lạnh lùng kiềm chế" sau "hay nói"), cả hai sẽ ở lại trong văn bản, do LLM cân nhắc theo thứ tự và ngữ cảnh; những gì cần ghi đè cứng xác định, hãy diễn đạt thành trường `structured` có thể máy móc hóa.

## Điểm Vào Ghi Disk

Chuẩn hóa, hợp nhất, ghi disk là cùng một bộ logic, nhưng có hai bên gọi, phải phân biệt rõ, nếu không sẽ trộn chuẩn bị khởi động vào context sáng tác chính:

- **Mở sách / Làm mới (phía khởi động, xác định)**: Được gọi trực tiếp bởi Host / quy trình khởi động để tạo snapshot ban đầu, không qua Coordinator, không vào Run sáng tác chính. Đây là tác vụ chuẩn bị khởi động xác định.
- **Cập nhật lúc chạy (công cụ của Coordinator)**: `save_user_rules` là vỏ công cụ runtime, tái sử dụng cùng bộ logic xác thực / hợp nhất / ghi disk, hợp nhất yêu cầu mới không có điểm tiến độ vào snapshot với vai trò `Runtime user update`.

(Về triển khai, khuyến nghị tập trung bộ logic này thành một service nội bộ, hai bên gọi dùng chung; tên cụ thể để triển khai quyết định.)

Dù bên nào gọi, cuối cùng đều ghi vào cùng một `meta/user_rules.json`. Logic ghi disk chỉ làm ba việc:

1. Xác thực trường cấu trúc hóa.
2. Hợp nhất vào snapshot hiện tại của cuốn sách theo thứ tự ưu tiên của §Quy tắc hợp nhất.
3. Trả về toàn bộ dữ liệu quy tắc sau khi lưu.

Không làm:

- Không phân công sub-agent.
- Không sửa đại cương.
- Không nuốt im trường không hợp lệ (ghi log và hạ cấp, xem §Thất bại và hạ cấp).
- Không inject văn bản gốc thẳng vào prompt làm kết quả cuối.

Ví dụ cập nhật lúc chạy: Người dùng nói "từ nay trở đi đều như vậy" (không có điểm tiến độ) → Coordinator gọi `save_user_rules` → Chuẩn hóa yêu cầu đó → Hợp nhất vào snapshot với độ ưu tiên cao nhất theo vai trò `Runtime user update` → Coordinator hiển thị ngắn gọn dựa trên dữ liệu trả về.

## Hiển Thị Kết Quả

Mỗi lần tạo hoặc cập nhật snapshot `user_rules`, đều phải hiển thị kết quả chuẩn hóa cho người dùng:

```text
Đã tạo snapshot quy tắc cuốn sách:
- Quy tắc máy móc: Mỗi chương 1200-1600 chữ; cụm từ bị cấm "ở một mức độ nào đó"
- Sở thích phong cách: Nhân vật chính lạnh lùng kiềm chế; ít giải thích, nhiều hành động và đối thoại
- Chưa nâng thành quy tắc máy móc: Ít dùng ẩn dụ (không có ngưỡng rõ ràng, xử lý như sở thích phong cách)
```

- Khởi động / Làm mới: Tái sử dụng năng lực log quy tắc khởi động hiện có để in snapshot, không thêm cơ chế mới; trong tình huống đồng sáng tác có thể gộp hiển thị vào bước xác nhận đồng sáng tác.
- Lúc chạy: Sau khi gọi `save_user_rules`, Coordinator hiển thị ngắn gọn dựa trên dữ liệu thực tế công cụ trả về.
- Hạ cấp: Khi `status=degraded`, hiển thị nêu rõ nguồn nào chưa parse được, hiện đang chạy với raw preferences, có thể tạo lại snapshot.

Hiển thị không phải cổng phê duyệt lần hai; tác dụng của nó là để người dùng biết hệ thống hiểu thành cái gì, phát hiện sai sót thì có thể tạo lại snapshot.

## Cách Agent Tiêu Thụ

Tất cả agent chỉ xem:

```json
working_memory.user_rules
```

Phân công trách nhiệm:

- Architect: Dùng `chapter_words` để điều chỉnh mật độ cốt truyện mỗi chương và số lượng chương tách ra.
- Writer: Viết theo quy tắc cứng của `structured`, điều chỉnh phong cách theo `preferences`.
- Editor: Đánh giá theo cùng bộ quy tắc.
- `commit_chapter`: Dùng `structured` để kiểm tra máy móc và trả về violations.

Writer không tự hiểu lại prompt khởi động gốc, cũng không đọc file rules gốc.

## Phân Loại Can Thiệp: Ba Hướng Đi (save_directive đã loại bỏ)

Yêu cầu viết lâu dài thống nhất đi qua `save_user_rules`, không còn kênh `save_directive` độc lập. Can thiệp lúc chạy theo "muốn đổi gì" chia ba loại:

- **Viết như thế nào** (kỹ thuật viết / phong cách / chất lượng: số chữ, dùng từ, từ cấm, câu chữ, tỷ lệ đối thoại, định dạng tiêu đề, v.v.) → `save_user_rules`, chuẩn hóa hợp nhất vào `meta/user_rules.json`. Ví dụ: "mỗi chương 1500 chữ", "tiêu đề chỉ dùng tiếng Việt", "nhân vật chính tổng thể lạnh lùng kiềm chế", "tỷ lệ đối thoại cao hơn".
- **Viết gì** (cốt truyện / cấu trúc / hướng đi nhân vật / độ dài) → architect, ghi vào compass / outline / hồ sơ nhân vật. Ví dụ: "tập này viết nhiều mạch chiến đấu hơn", "từ chương 30 trở đi giọng điệu nhân vật chính chuyển lạnh", "tăng lên 40 chương".
- **Sửa bản đã viết** (viết lại / chỉnh sửa các chương cụ thể) → editor, xếp vào hàng đợi PendingRewrites.

Tiêu chí: **"Viết như thế nào" → save_user_rules; "Viết gì" → architect; "Sửa bản đã viết" → editor**.

> Trước đây từng có `save_directive` (với điểm neo tiến độ `at_chapter`) tồn tại song song với `save_user_rules`. Thực tế cho thấy hai cái trùng lặp ở sở thích văn bản tự do, còn "có hay không có điểm neo tiến độ" là bài toán phân loại mơ hồ (hầu hết yêu cầu lúc chạy tự nhiên đều là "từ bây giờ trở đi"), chỉ tăng thêm gánh nặng phân loại cho Coordinator và từng gây vấn đề định tuyến. Yêu cầu thực sự gắn với tiến độ cốt truyện lẽ ra nên do architect đảm nhận, vì vậy ngày 2026-06-28 đã xóa `save_directive`. Đây là breaking change có chủ ý: `meta/user_directives.json` còn lại trong sách cũ sẽ không được đọc nữa, không di chuyển, sách vẫn có thể restore và tiếp tục viết, nhưng các sở thích lịch sử trong directive cũ sẽ không còn có hiệu lực.

## Xử Lý Sách Cũ

Sách cũ nếu chưa có `meta/user_rules.json`:

1. Lần khởi động đầu tiên, tạo snapshot lazy dùng prompt khởi động hiện có, file rules của người dùng và quy tắc mặc định hệ thống.
2. Lưu vào `meta/user_rules.json`.
3. In hiển thị khởi động, nêu rõ nguồn gốc snapshot.

Các lần chạy sau chỉ đọc snapshot, không còn trượt hành vi do file rules bên ngoài thay đổi. `meta/user_directives.json` phiên bản cũ bị bỏ qua; yêu cầu lịch sử cần giữ lại nên do người dùng nhập lại, đi qua `save_user_rules` ghi vào snapshot mới.

## Các Bước Triển Khai

1. Thêm `meta/user_rules.json` store.
2. Thêm LLM chuẩn hóa pass độc lập (theo từng nguồn), dùng ràng buộc schema xuất ra `structured/preferences/sources/uncertain` ứng viên.
3. Thêm hợp nhất xác định phía Go: Theo độ ưu tiên, ghi đè trường và nối văn bản của từng nguồn, tạo snapshot.
4. Tập trung chuẩn hóa / hợp nhất / ghi disk thành một bộ logic, hai bên gọi dùng chung: Phía khởi động gọi trực tiếp tạo snapshot ban đầu (không qua Coordinator); thêm vỏ công cụ runtime `save_user_rules` tái sử dụng nó (gắn cho Coordinator). Khi thất bại, xử lý theo §Thất bại và hạ cấp: nguồn hạ cấp thành raw preferences, snapshot `status=degraded`, sáng tác chính tiếp tục; `save_user_rules` không ném tool error kỹ thuật cho Coordinator.
5. Di chuyển quy tắc mặc định máy móc hệ thống trong `assets/rules/default.md` hiện tại vào struct built-in trong code hoặc JSON asset, giữ lại comment nguồn gốc ngưỡng; xóa đường parse YAML của rules người dùng, không làm lớp tương thích.
6. Sau khi đọc file rules không còn inject nội dung trực tiếp thành prompt, mà chuẩn hóa xong hợp nhất vào snapshot `user_rules`.
7. Sách cũ chưa có snapshot, lần khởi động đầu tiên tạo snapshot lazy và hiển thị nguồn gốc.
8. `novel_context` chỉ inject `working_memory.user_rules` từ `meta/user_rules.json`.
9. `commit_chapter` dùng cùng `user_rules.structured` để kiểm tra.
10. Prompt Coordinator nêu rõ ba loại phân luồng theo "muốn đổi gì": Yêu cầu lâu dài về phong cách / chất lượng viết thì `save_user_rules` trước rồi mới quy hoạch hoặc tiếp tục viết; cốt truyện / cấu trúc / nhân vật / độ dài thì qua architect; chỉnh sửa chương đã viết thì qua editor (xem chi tiết §Phân loại can thiệp: Ba hướng đi).

## Tiêu Chí Nghiệm Thu

- Người dùng viết "mỗi chương 1200-1600 chữ" trong prompt khởi động, `novel_context` mà Writer nhận được ở chương đầu tiên phải có `chapter_words`.
- File rules chỉ viết ngôn ngữ tự nhiên, cũng có thể chuẩn hóa vào cùng `user_rules` khi tạo snapshot.
- File rules không cần và không hỗ trợ YAML; tất cả chuẩn hóa theo quy tắc ngôn ngữ tự nhiên.
- Lúc chạy không còn đọc file rules; chỉ đọc `meta/user_rules.json`.
- Quy tắc mặc định máy móc không còn đến từ file YAML, rules của người dùng cũng không có lớp tương thích YAML.
- Chuẩn hóa không dùng regex/từ khóa hard-code; hiểu ngôn ngữ tự nhiên do LLM thực hiện.
- Quy tắc mơ hồ không được nâng cấp thành trường `structured` cấp error.
- Quy tắc mặc định hệ thống không qua LLM, đi thẳng vào hợp nhất Go.
- Độ ưu tiên nguồn và ghi đè trường do Go thực hiện xác định, cùng đầu vào cho ra cùng snapshot.
- Người dùng nói "từ nay trở đi đều như vậy" lúc chạy, qua `save_user_rules` hợp nhất vào snapshot, `novel_context` của các chương tiếp theo phải thấy cập nhật.
- Chuẩn hóa thất bại không ngăn viết sách: Nguồn thất bại hạ cấp thành raw preferences, snapshot `status=degraded`, sáng tác chính tiếp tục; chỉ khi snapshot không ghi được disk mới dừng.
- `save_user_rules` gặp normalizer thất bại trả về `status=degraded`, không ném tool error kỹ thuật cho Coordinator.
- Sau khi tạo hoặc cập nhật snapshot, sẽ hiển thị `structured` / `preferences` / các mục chưa nâng cấp; khi hạ cấp, hiển thị giải thích nguồn bị hạ cấp.
- Mở sách mới không kế thừa `user_rules` của sách trước.
- Trường cấu trúc không hợp lệ không bị bỏ qua im: Ghi log và hạ cấp nguồn đó, không ngăn luồng chính.

## Rõ Ràng Không Làm (Đã Phán Định Không Cần, Không Phải Chia Giai Đoạn)

Các năng lực sau không có lợi ích trong yêu cầu hiện tại, không đưa vào thiết kế, tránh thiết kế quá mức:

- Ngữ nghĩa xóa / thu hồi cấp trường như `clear_fields`.
- Tự động làm mới khi file rules thay đổi (nếu sửa file thì tạo lại snapshot tường minh là đủ).
- Điểm neo thời gian / giải quyết ghi đè của `preferences` (những gì cần ghi đè cứng xác định, hãy dùng trường `structured`).
- Lưu mảng `diagnostics` lâu dài trong snapshot (lỗi kỹ thuật vào log là đủ, snapshot chỉ giữ `status`).
- Tự động tạo mô tả trường schema từ kiểu Go (tự tay duy trì một mô tả ngắn gọn là đủ).

Nguyên tắc thiết kế không đổi: LLM chịu trách nhiệm hiểu ngôn ngữ tự nhiên, Go chịu trách nhiệm hợp nhất xác định, xác thực, ghi disk và kiểm tra.

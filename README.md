# ainovel-cli

Engine sáng tác tiểu thuyết dài AI hoàn toàn tự động. Coordinator điều phối ba sub-agent Architect / Writer / Editor trong một lần Prompt duy nhất để hoàn thành toàn bộ cuốn sách, Host chỉ làm nhiệm vụ khởi động, khôi phục và quan sát. Từ một câu yêu cầu đến tiểu thuyết hoàn chỉnh, toàn bộ quá trình không cần can thiệp thủ công.

<p align="center">
  <img src="scripts/sample.gif" alt="ainovel-cli demo" width="800">
  <img src="scripts/novel.png" alt="ainovel-cli bg" width="800">
</p>

## Tính năng

- **Cộng tác đa agent** — Coordinator lên lịch cho ba sub-agent Architect / Writer / Editor trong một vòng lặp dài, tự chủ quyết định luồng sáng tác
- **Vòng lặp dài do LLM điều khiển** — Một lần Prompt viết xong toàn bộ cuốn sách, Host không can thiệp vào lịch trình. Càng đơn giản càng ổn định, từ chối biên soạn phức tạp
- **Khôi phục điểm dừng cấp Step** — Sau mỗi lần thực thi công cụ thành công sẽ ghi checkpoint, sau sự cố có thể khôi phục chính xác đến bước plan/draft/check/commit
- **Quy hoạch cuộn hai tầng tập-arc** — Tiểu thuyết dài không còn quy hoạch tất cả chương một lần. Ban đầu chỉ quy hoạch khung xương 2 tập + các chương chi tiết của arc đầu tiên, các arc/tập tiếp theo được Architect triển khai khi tiến độ viết đến nơi, mỗi lần triển khai đều tham chiếu tóm tắt nội dung trước và trạng thái nhân vật, quy hoạch xa không rỗng tuếch
- **Đề xuất thông minh các chương liên quan** — Khi viết mỗi chương, tự động đề xuất các chương lịch sử liên quan theo bốn chiều: phục bút, nhân vật xuất hiện, thay đổi trạng thái, mối quan hệ; kết hợp với đoạn giới thiệu chương tiếp theo, đảm bảo tính liên tục của tiểu thuyết 500+ chương
- **Chiến lược ngữ cảnh thích ứng** — Tự động chuyển đổi toàn lượng / cửa sổ trượt / tóm tắt phân tầng theo tổng số chương, hỗ trợ tiểu thuyết dài 500+ chương
- **Xét duyệt chất lượng bảy chiều** — Editor xét duyệt theo bảy chiều: nhất quán thiết định, hành vi nhân vật, nhịp độ, mạch lạc tự sự, phục bút, hook, chất lượng thẩm mỹ; chiều thẩm mỹ chia nhỏ thành năm hạng mục: cảm giác miêu tả, thủ pháp tự sự, khả năng phân biệt đối thoại, chất lượng dùng từ, sức lay động cảm xúc; mỗi hạng mục phải trích dẫn nguyên văn làm bằng chứng
- **Can thiệp người dùng thời gian thực** — Có thể chèn ý kiến sửa đổi vào ô nhập liệu bất kỳ lúc nào trong quá trình viết (không cần tạm dừng), hệ thống tự động đánh giá phạm vi ảnh hưởng và viết lại các chương bị ảnh hưởng
- **Cổng vào TUI thống nhất** — Giao diện tương tác theo dõi tiến độ thời gian thực, cũng hỗ trợ khởi động trực tiếp với một câu yêu cầu
- **Hỗ trợ đa LLM** — Chuyển đổi tùy ý giữa OpenRouter / Anthropic / Gemini / OpenAI / Claude Code và nhiều nhà cung cấp khác

## Kiến trúc

Thiết kế cốt lõi: **LLM điều khiển, Host phục vụ**. Coordinator tự chủ quyết định luồng sáng tác toàn bộ cuốn sách trong một lần Run, Host chỉ làm khởi động, khôi phục và quan sát sự kiện.

```
┌─────────────────────────────────────────────────┐
│                Host (vỏ mỏng)                   │
│          Khởi động / Khôi phục / Quan sát / Chèn can thiệp  │
└──────────────────────┬──────────────────────────┘
                       │ Một lần Prompt
┌──────────────────────▼──────────────────────────┐
│          Coordinator (vòng lặp dài LLM)         │
│  Đọc novel_context → Gọi sub-agent → Đọc kết quả → Tiếp tục │
└────┬──────────┬──────────┬──────────────────────┘
     │          │          │
 ┌───▼────┐ ┌───▼───┐ ┌────▼────┐
 │Architect│ │Writer │ │ Editor  │
 └───┬────┘ └───┬───┘ └────┬────┘
     └──────────┼──────────┘
                │ Gọi công cụ (IO + checkpoint)
┌───────────────▼─────────────────────────────────┐
│                   Store                         │
│  Progress / Checkpoint / Outline / Drafts / ... │
└─────────────────────────────────────────────────┘
```

- **Host** — Khởi động Coordinator, khôi phục sự cố, chiếu sự kiện lên TUI. Không đưa ra bất kỳ quyết định lịch trình nào
- **Coordinator** — Người quyết định duy nhất, điều khiển luồng hoàn chỉnh quy hoạch → viết → xét duyệt → tóm tắt trong một lần Run
- **SubAgents** — Architect / Writer / Editor mỗi agent có context độc lập, cộng tác qua các artifact trong Store
- **Tools** — IO nguyên tử + ghi checkpoint, chỉ trả về JSON sự kiện, không kèm theo lệnh

### Nhiệm vụ của từng agent

| Agent | Nhiệm vụ | Công cụ |
|--------|------|------|
| **Coordinator** | Điều phối toàn cục, xử lý phán quyết xét duyệt và can thiệp người dùng | `subagent` `novel_context` |
| **Architect** | Tạo tiền đề, dàn ý, hồ sơ nhân vật, quy tắc thế giới | `novel_context` `save_foundation` |
| **Writer** | Tự chủ hoàn thành một chương: lên ý tưởng, viết, tự xét và nộp | `novel_context` `read_chapter` `plan_chapter` `draft_chapter` `check_consistency` `commit_chapter` |
| **Editor** | Đọc bản gốc, xét duyệt ở hai tầng cấu trúc và thẩm mỹ | `novel_context` `read_chapter` `save_review` `save_arc_summary` `save_volume_summary` |

### Luồng viết

```
Yêu cầu người dùng → Architect quy hoạch khung + các chương arc đầu → Writer viết từng chương → Editor xét duyệt cấp arc
                                                  ↑                   │
                                                  ├── viết lại/đánh bóng ◄──────┘
                                                  │
                                           Architect triển khai arc/tập tiếp theo
                                          (tham chiếu tóm tắt trước + ảnh chụp nhân vật)
```

Writer hoàn thành mỗi chương theo thứ tự cố định (nội dung viết hoàn toàn tự chủ, thứ tự gọi công cụ nghiêm ngặt):

1. `novel_context` — Tải ngữ cảnh (tóm tắt nội dung trước, phục bút, trạng thái nhân vật, quy tắc phong cách, đề xuất chương liên quan)
2. `read_chapter` — Đọc lại nội dung trước để lấy lại giọng văn và nhịp điệu
3. `plan_chapter` — Lên ý tưởng mục tiêu, xung đột, đường cong cảm xúc của chương
4. `draft_chapter` — Viết toàn bộ nội dung chương
5. `check_consistency` — Đối chiếu dữ liệu trạng thái kiểm tra nhất quán (phải sau draft)
6. `commit_chapter` — Nộp bản cuối, trả về các trường sự kiện (`arc_end_reached` / `next_chapter` v.v.), bước tiếp theo được Reminder điều khiển

### Quy tắc chuyển trạng thái

Hệ thống chia trạng thái chạy thành hai tầng:

- **Phase** — Giai đoạn lớn, biểu thị tác phẩm hiện đang ở giai đoạn thiết định, viết hay đã hoàn thành
- **Flow** — Luồng đang hoạt động hiện tại, biểu thị hệ thống lúc này đang viết bình thường, xét duyệt, viết lại, đánh bóng hay xử lý can thiệp người dùng

#### Phase

`Phase` tuân theo quy tắc "chỉ tiến không lùi":

```text
init -> premise -> outline -> writing -> complete
  \-------> outline ------^
  \--------------> writing
```

Ý nghĩa:

- `init` — Nhiệm vụ đã tạo, chưa hình thành thiết định ổn định
- `premise` — Đã lưu tiền đề câu chuyện
- `outline` — Đã lưu dàn ý, có thể bước vào viết chính thức
- `writing` — Đã bước vào giai đoạn sáng tác chương
- `complete` — Luồng toàn bộ cuốn sách kết thúc

Giải thích quy tắc:

- Cho phép cập nhật đồng trạng thái, ví dụ `writing -> writing`
- Cho phép tiến lên, ví dụ `outline -> writing`
- Không cho phép lùi lại, ví dụ `writing -> premise`, `complete -> writing`

#### Flow

`Flow` chỉ mô tả luồng đang hoạt động trong giai đoạn viết, cho phép chuyển đổi giữa một số workflow:

```text
writing   -> reviewing / rewriting / polishing / steering / writing
reviewing -> writing / rewriting / polishing / steering / reviewing
rewriting -> writing / steering / rewriting
polishing -> writing / steering / polishing
steering  -> writing / reviewing / rewriting / polishing / steering
```

Ý nghĩa:

- `writing` — Tiếp tục chương tiếp theo bình thường
- `reviewing` — Editor đang xét duyệt
- `rewriting` — Xử lý các chương phải viết lại
- `polishing` — Xử lý các chương chỉ cần đánh bóng
- `steering` — Đang đánh giá và xử lý can thiệp người dùng

Giải thích quy tắc:

- Cho phép `writing -> reviewing`, ví dụ kích hoạt xét duyệt sau khi nộp chương
- Cho phép `reviewing -> rewriting/polishing/writing`, do kết quả xét duyệt quyết định
- Cho phép `steering -> writing/reviewing/rewriting/polishing`, do phạm vi ảnh hưởng của can thiệp quyết định
- Không cho phép nhảy rõ ràng bất thường, ví dụ `rewriting -> reviewing`

Các quy tắc này hiện được ràng buộc thống nhất bởi kiểm tra nhẹ trong mã, tránh trạng thái bị lùi hoặc nhảy sang nhánh luồng không hợp lý.

### Quy hoạch cuộn tiểu thuyết dài

Phương án truyền thống quy hoạch tất cả chương một lần, khi 300+ chương dàn ý trở nên rỗng tuếch, nhịp độ như đang chạy cho xong tiến độ. Hệ thống này áp dụng **quy hoạch la bàn định hướng + cuộn tầm nhìn**, mô phỏng quy trình sáng tác thực tế của tác giả tiểu thuyết mạng:

```
Quy hoạch ban đầu               Khi arc kết thúc                  Khi tập kết thúc
┌────────────────────┐    ┌─────────────────────┐    ┌─────────────────────┐
│ Hướng kết cục (la bàn)  │    │ Editor xét duyệt cấp arc  │    │ Editor xét duyệt cấp tập  │
│ Khởi đầu 2 tập, sau theo nhu cầu │    │ Tóm tắt arc + ảnh chụp nhân vật │    │ Tóm tắt tập         │
│ Các chương chi tiết arc 1 │ →  │ Architect triển khai arc tiếp │ →  │ Architect tự chủ tạo │
│ Nhân vật + thế giới quan │    │ Writer tiếp tục viết   │    │ Tập tiếp + cập nhật la bàn │
└────────────────────┘    └─────────────────────┘    └─────────────────────┘
```

- **La bàn định hướng (Compass)** — Hướng kết cục + tuyến dài đang hoạt động + ước tính quy mô, mỗi ranh giới tập được Architect cập nhật, hướng câu chuyện có thể tiến hóa theo sáng tác
- **Tạo theo nhu cầu** — Sau khi viết xong tập hiện tại, Architect tự chủ tạo tập tiếp theo dựa trên nội dung đã viết. Quy hoạch ban đầu tạo 2 tập làm điểm khởi đầu, các tập sau tạo theo nhu cầu
- **Arc khung xương** — Chỉ có goal + ước tính số chương, khi đến nơi mới triển khai các chương chi tiết
- **Tinh chỉnh dần dần** — Mỗi lần triển khai đều tham chiếu tóm tắt nội dung trước, ảnh chụp nhân vật, quy tắc phong cách, càng viết về sau càng chính xác
- **Mẫu nhịp điệu thông dụng** — Arc trưởng thành-đột phá / Arc đối kháng thi đấu / Arc khám phá-phát hiện / Arc oán thù-xung đột / Arc chuyển tiếp thường ngày, mỗi loại arc có mật độ tham chiếu và ánh xạ chủ đề phù hợp

### Quản lý ngữ cảnh tiểu thuyết dài

Tiểu thuyết 500+ chương sử dụng tóm tắt ba cấp + đường ống nén bốn cấp + đề xuất thông minh:

```
Tập (Volume) → Tóm tắt tập
└── Arc → Tóm tắt arc + Ảnh chụp nhân vật + Quy tắc phong cách
    └── Chương (Chapter) → Tóm tắt chương (cửa sổ trượt 3 chương gần nhất)
```

- **Tóm tắt phân tầng** — Gần dùng tóm tắt chương, tầm trung dùng tóm tắt arc, xa dùng tóm tắt tập, nén từng tầng không mất thông tin
- **Đề xuất chương liên quan** — Khi viết mỗi chương, tra cứu ngược lịch sử chương theo bốn chiều: phục bút, nhân vật xuất hiện, thay đổi trạng thái, mối quan hệ; đề xuất Writer đọc lại theo nhu cầu
- **Đoạn giới thiệu chương tiếp** — Tải dàn ý chương tiếp theo, giúp Writer thiết kế hook cuối chương và kết nối phục bút
- **Phát hiện ranh giới arc** — Tự động nhận diện kết thúc arc/tập, kích hoạt xét duyệt, tạo tóm tắt và triển khai arc/tập tiếp theo

#### Đường ống nén ngữ cảnh

Khi hội thoại vượt quá cửa sổ ngữ cảnh của mô hình, nén dần theo thứ tự từ chi phí thấp đến cao:

```
ToolResultMicrocompact → LightTrim → StoreSummaryCompact → FullSummary
     Dọn kết quả công cụ cũ    Cắt bớt văn bản dài    Nén store không dùng LLM    LLM tóm tắt dự phòng
```

- **StoreSummaryCompact** — Dành riêng cho Writer, dùng tóm tắt chương, ảnh chụp nhân vật, sổ phục bút đã có trong store để thay thế trực tiếp tin nhắn cũ, không tốn chi phí LLM
- **FullSummary tùy chỉnh tiểu thuyết** — Writer sử dụng gợi ý tóm tắt hướng đến tính liên tục tự sự, yêu cầu rõ ràng giữ lại trạng thái nhân vật, manh mối phục bút, các mục chờ sửa sau xét duyệt, điểm neo phong cách
- **Gói khôi phục sau nén** — Sau FullSummary tự động chèn kế hoạch chương hiện tại, dàn ý và ảnh chụp nhân vật, tránh Writer "mất trí nhớ" sau nén
- **Bộ ngắt mạch** — Khi nén thất bại liên tiếp tự động bỏ qua và cảnh báo rõ ràng, dùng chế độ nửa mở, tự động thử lại lượt sau
- **Ước tính Token CJK** — Tiếng Trung `runes × 1.5`, không bị ước thấp do `bytes/4` khiến kích hoạt nén bị trễ
- **Thanh sức khỏe TUI gradient** — Mức sử dụng ngữ cảnh xanh (<70%) → vàng (70-85%) → đỏ (>85%) hiển thị thời gian thực

## Bắt đầu nhanh

```bash
# Cài đặt một lệnh (macOS / Linux, không cần Go)
curl -fsSL https://raw.githubusercontent.com/voocel/ainovel-cli/main/scripts/install.sh | sh

# Cài đặt phiên bản cụ thể
curl -fsSL https://raw.githubusercontent.com/voocel/ainovel-cli/main/scripts/install.sh | sh -s -- v1.2.3

# Hoặc cài qua Go
go install github.com/voocel/ainovel-cli/cmd/ainovel-cli@latest

# Xem phiên bản / Cập nhật lên phiên bản mới nhất
ainovel-cli --version
ainovel-cli update

# Lần chạy đầu tiên, tự động vào quy trình hướng dẫn (chọn Provider → nhập API Key → Base URL → tên mô hình)
ainovel-cli
```

> Windows hoặc cài thủ công: Truy cập [Releases](https://github.com/voocel/ainovel-cli/releases/latest) để tải gói tương ứng nền tảng.

### Giao diện Web

Ngoài TUI trên terminal, cũng có thể dùng bàn làm việc trực quan trong trình duyệt (giao diện bằng tiếng Việt). Đây là cổng vào thứ ba song song với TUI / headless, bên dưới tái sử dụng cùng một engine `host.Host`, không thay đổi logic sáng tác.

```bash
ainovel-cli --web                 # Mặc định lắng nghe 127.0.0.1:8765 và tự động mở trình duyệt
ainovel-cli --web --port 9000     # Chỉ định cổng (tự động dùng cổng rảnh khi bị chiếm)
```

- **Chỉ máy cục bộ**: Cố định bind `127.0.0.1`, không phơi ra ngoài, không cần đăng nhập.
- **Hướng dẫn lần đầu**: Khi chưa cấu hình, hoàn thành thiết lập Provider / API Key / mô hình trực tiếp trong trình duyệt, không cần vào terminal.
- **Tính năng tương đương TUI**: Xem trực tiếp luồng sáng tác, can thiệp người dùng (steer), tạm dừng / tiếp tục, xem tiến độ / dàn ý / nhân vật, xuất TXT/EPUB, chuyển đổi mô hình và cường độ suy luận, cùng tạo quy hoạch, nhập tiếp nối, hồ sơ hành văn, chẩn đoán `/diag`.
- **Thời gian thực và kết nối lại**: Sự kiện, nội dung streaming, trạng thái được đẩy qua SSE; sau khi tải lại trang tự động khôi phục bằng ảnh chụp nhanh + phát lại.

Mỗi tiểu thuyết cũng được bind vào thư mục khởi động (`cd` sang thư mục khác = đổi tiểu thuyết / tự động khôi phục), nhất quán với TUI.

### Docker

Ảnh Docker phù hợp để chạy tác vụ dài headless trên máy chủ/NAS, cũng có thể dùng `-it` để vào TUI. Thư mục cấu hình và tác phẩm nên được mount vào máy chủ:

```bash
mkdir -p config workspace

# TUI
docker run --rm -it \
  -v "$PWD/config:/root/.ainovel" \
  -v "$PWD/workspace:/workspace" \
  ghcr.io/voocel/ainovel-cli:latest

# Headless
docker run --rm \
  -v "$PWD/config:/root/.ainovel" \
  -v "$PWD/workspace:/workspace" \
  ghcr.io/voocel/ainovel-cli:latest \
  --headless --prompt "Viết một tiểu thuyết huyền huyễn dài, nhân vật chính khởi đầu từ thị trấn biên giới nhỏ"
```

Cũng có thể dùng Compose:

```bash
docker compose run --rm ainovel
docker compose run --rm ainovel --headless --prompt "Viết một truyện ngắn huyền bí"
```

Sau khi vào TUI, giai đoạn khởi động hỗ trợ hai loại tương tác trước:

- `Bắt đầu nhanh`: Một câu đi thẳng vào sáng tác
- `Cùng tạo quy hoạch`: Đối thoại nhiều lượt với AI để làm rõ yêu cầu, **bên phải đồng bộ thời gian thực bản nháp lệnh sáng tác đã được sắp xếp**; mỗi lượt AI chủ động cung cấp 1-3 gợi ý dẫn dắt, nhấn phím số để điền vào ô nhập liệu, nhấn `Ctrl+S` để bắt đầu sáng tác chính thức

Cả hai chế độ cuối cùng đều hội tụ thành cùng một lệnh sáng tác, rồi vào cùng một engine sáng tác.

### Quản lý nhiều tiểu thuyết

Mỗi tiểu thuyết được bind vào thư mục khởi động, sản phẩm lưu ở `{cwd}/output/novel/`. Đổi thư mục khởi động = đổi tiểu thuyết, `cd` về thư mục cũ khởi động = tự động khôi phục từ checkpoint gần nhất. Cấu hình `~/.ainovel/config.json` dùng chung toàn cục, không cần sao chép.

### Tệp cấu hình

Khi chạy lần đầu tiên tự động tạo tệp cấu hình `~/.ainovel/config.json` qua quy trình hướng dẫn, sau đó có thể chỉnh sửa trực tiếp tệp này để điều chỉnh cài đặt. Xóa tệp cấu hình rồi chạy lại sẽ vào lại quy trình hướng dẫn.

Cũng có thể tạo thủ công tệp cấu hình, tham chiếu `config.example.jsonc` ở thư mục gốc kho lưu trữ. Quy trình hướng dẫn lần đầu cũng sẽ sao chép một bản đến `~/.ainovel/config.example.jsonc`, tiện xem offline trên máy.

```jsonc
{
  "provider": "openrouter",
  "model": "google/gemini-2.5-flash",
  "reasoning_effort": "medium",
  "providers": {
    "openrouter": {
      "api_key": "sk-or-v1-xxx",
      "base_url": "https://openrouter.ai/api/v1",
      "models": ["google/gemini-2.5-flash", "google/gemini-2.5-pro"],
      "extra": {
        "user_agent": "my-client/1.0",
        "headers": { "X-Custom-Client": "my-client" }
      }
    }
  },
  "style": "default"
}
```

#### Thứ tự tìm kiếm tệp cấu hình (sau ghi đè trước)

1. `~/.ainovel/config.json` — Cấu hình toàn cục
2. `./.ainovel/config.json` — Ghi đè cấp dự án (tùy chọn)
3. `--config path/to/config.json` — Chỉ định qua dòng lệnh

> `.ainovel/` cấp dự án là bản phản chiếu của `~/.ainovel/` toàn cục: cùng cấu trúc, chỉ thay thư mục gốc từ thư mục home sang dự án hiện tại. Cấu hình để ở `./.ainovel/config.json`, quy tắc viết để ở `./.ainovel/rules/*.md` (xem phần "Khử mùi AI và quy tắc tùy chỉnh" bên dưới). Thư mục này chứa khóa bí mật, đã mặc định thêm vào `.gitignore`.

Giải thích quy tắc ghi đè:

- Trường vô hướng theo sau ghi đè trước, ví dụ `provider`, `model`, `reasoning_effort`, `style`
- `providers` và `roles` hợp nhất theo key, các mục cùng tên ghi đè theo trường bên trong
- Trường chưa điền sẽ kế thừa cấu hình tầng trên, ví dụ khi cấu hình cấp dự án chỉ viết `base_url` thì vẫn giữ lại `api_key` trong cấu hình toàn cục
- Hiện tại không hỗ trợ dùng chuỗi rỗng để xóa tường minh giá trị tầng trên đã có; nếu cần xóa, hãy chỉnh sửa trực tiếp tệp cấu hình có ưu tiên cao hơn

> ⚠️ Giá trị của `provider` (cũng như `roles.*.provider`) là **tên key** trong `providers` — một con trỏ, không phải tên giao thức. Nếu cấp dự án chuyển `provider` sang một tài khoản không tồn tại trong `providers` toàn cục, phải bổ sung đồng thời thông tin xác thực của tài khoản đó ở cấp dự án (`api_key` / `base_url`), nếu không khi khởi động sẽ báo lỗi "chưa cấu hình thông tin xác thực".

`providers.<name>.models` là trường tùy chọn, dùng để khai báo danh sách mô hình được phép chuyển đổi trong bảng điều khiển `/model` TUI cho provider đó; nếu chưa cấu hình, hệ thống sẽ dự phòng về các mô hình của provider đó đã xuất hiện trong tệp cấu hình hiện tại.

`reasoning_effort` là cường độ suy luận mặc định, các giá trị hợp lệ là `off` / `low` / `medium` / `high` / `xhigh` / `max`; bỏ qua hoặc chuỗi rỗng có nghĩa là dùng mặc định của mô hình/provider. `roles.<role>.reasoning_effort` có thể ghi đè theo từng nhân vật, khi chưa cấu hình thì kế thừa `reasoning_effort` ở tầng trên. Khi bảng điều khiển `/model` TUI chuyển đổi provider, model hoặc cường độ suy luận, sẽ ghi lại vào cấu hình toàn cục `~/.ainovel/config.json`.

`providers.<name>.api` chỉ có hiệu lực với `type: "openai"` hoặc `openai` tích hợp sẵn, dùng để chọn endpoint giao thức OpenAI: `chat` (mặc định, `/v1/chat/completions`) hoặc `responses` (`/v1/responses`). Các proxy loại Codex thường cần cấu hình thành `responses`.

`providers.<name>.extra` là cấu hình cấp provider, sẽ truyền cho HTTP client bên dưới, phù hợp để cấu hình các trường nhận diện proxy như `user_agent`, `headers`, `anthropic_beta`; `providers.<name>.extra_body` mới là tham số mở rộng thân request, không nên nhầm lẫn hai cái.

## Báo cáo chẩn đoán

Nhập `/diag` trong TUI có thể phân tích chẩn đoán các sản phẩm output của tiểu thuyết hiện tại, đưa ra các phát hiện và đề xuất cải thiện có thể thực hiện được.

Chẩn đoán bao gồm bốn chiều:

- **Luồng** — Vòng lặp viết lại bị treo, lệnh chuyển hướng chưa tiêu thụ, trạng thái phase/flow bất thường, số thứ tự chương bị nhảy
- **Chất lượng** — Điểm liên tục thấp ở các chiều xét duyệt, tỷ lệ thực hiện hợp đồng, tỷ lệ viết lại, số từ chương bất thường
- **Quy hoạch** — Phục bút bị đình trệ, la bàn định hướng lỗi thời, dàn ý cạn kiệt, tóm tắt thiếu
- **Ngữ cảnh** — Nhân vật biến mất, khoảng trống dòng thời gian, dữ liệu mối quan hệ bị đình trệ

Mỗi phát hiện bao gồm: mô tả vấn đề, bằng chứng dữ liệu, đề xuất cải thiện (chỉ đến prompt/flow/config cụ thể).

`/diag` đồng thời sẽ ghi ra một tệp `meta/diag-export.md` **đã ẩn danh** (loại bỏ nội dung tiểu thuyết, chỉ giữ lại khung hành vi như gọi công cụ, chuỗi lỗi, số lần lặp lại). Khi gặp vấn đề vòng lặp vô tận / ngắt quãng, dán tệp này vào GitHub issue là đủ, tiện cho người bảo trì định vị lỗi mà không cần dữ liệu cục bộ.

## Hồ sơ hành văn

Đặt bài viết tham chiếu vào thư mục `simulate/` ở thư mục khởi động hiện tại, sau đó nhập `/simulate` trong TUI. Hệ thống sẽ đọc đệ quy các tệp `.txt`, `.md`, `.markdown`, dùng mô hình architect phân tích ngữ liệu và ghi vào:

```text
output/novel/meta/simulation_profile.json
```

Khi chạy `/simulate` lần nữa, sẽ bỏ qua các tệp chưa thay đổi theo `relative_path + sha256`; nếu không có nội dung mới thêm hoặc thay đổi, sẽ nhắc "hồ sơ đã là mới nhất" và không gọi LLM. Nếu đã có hồ sơ và trong `simulate/` xuất hiện bài viết mới thêm hoặc sửa đổi, hệ thống sẽ tiếp tục tổng hợp dựa trên hồ sơ gốc.

Cũng có thể nhập hồ sơ đã tạo trước đó, tránh phân tích lại cùng một bộ bài viết:

```text
/simulate
/importsim ./profile.json
```

`/importsim` chỉ chấp nhận JSON `simulation_profile.v1` được tạo bởi tính năng này, hợp nhất theo dấu vân tay ngữ liệu, nguồn trùng lặp sẽ bị bỏ qua. Chỉ nhập tệp hồ sơ từ nguồn tin cậy; nội dung nhập sẽ trở thành tham chiếu ngữ cảnh cho Agent tiếp theo. Hồ sơ sẽ được chèn vào `novel_context` ở dạng compact, Coordinator, Architect, Writer, Editor đều có thể đọc; mỗi Agent chỉ tham khảo cấu trúc, nhịp điệu, hook và thủ thuật thu hút độc giả, không sao chép biểu đạt nguyên văn hay thiết định độc quyền.

## Nhập

Nhập `/import <đường dẫn tệp>` trong TUI có thể nhập ngược một tiểu thuyết đã có: đầu tiên phân cắt theo chương, sau đó dùng LLM suy ngược ra tiền đề / nhân vật / thế giới quan / dàn ý phân tầng / la bàn định hướng, từng chương lưu vào đĩa. Bản gốc được tạo thành tập đầu tiên có thể tiếp nối, sau khi nhập hoàn thành sẽ **tự động tiếp tục viết** — Coordinator thực hiện xét duyệt/tóm tắt ở cuối tập đầu, thêm tập mới, tiếp tục từ chương tiếp theo.

```
/import ~/tieu-thuyet-cua-toi.txt              # Nhập từ đầu và suy ngược foundation
/import ~/tieu-thuyet-cua-toi.txt from=50      # Nhập tiếp từ chương 50 (bỏ qua suy ngược)
```

**Quy tắc phân cắt chương**: Tự động nhận diện các định dạng tiêu đề sau (đầu dòng, có thể có tiền tố Markdown `#`/`##`, bao bọc `【】`/`〖〗`, khoảng trắng toàn góc, tương thích mã hóa GBK/BOM):

- Số thứ tự Trung văn: `第一章` `第3回` `第十话` `第二卷` `第五节` `第二幕`, `卷一` độc lập, số hỗ trợ chữ hoa (`第壹章`), có thể có phụ đề (`第三章：决战`)
- Đơn vị đặc biệt Trung văn: `序章` `楔子` `引子` `前言` `尾声` `终章` `后记` `番外` `外传`
- Tiếng Anh: `Chapter 1` `Chapter II`, `Prologue` `Epilogue`, có thể có phụ đề (`Chapter 1: The Beginning`)

Nếu thông báo **"Không nhận diện được chương nào"**, hãy xác nhận tệp thực sự là văn bản tiểu thuyết phân chương (tiêu đề chương chiếm một dòng riêng, nằm ở đầu dòng).

> Nhập là phát lại xác định, không qua Coordinator; bản gốc sẽ được lưu từng chữ thành các chương đã hoàn thành, do đó phù hợp với "tiếp nối cùng một cuốn sách". Nếu chỉ muốn mượn thiết định để sáng tác hoàn toàn mới, hãy bắt đầu một cuốn sách mới theo cách thông thường và mô tả phong cách thiết định mong muốn trong yêu cầu.

## Xuất

Nhập `/export` trong TUI có thể hợp nhất và xuất các chương đã hoàn thành, mặc định TXT, ghi vào `{novelDir}/{NovelName}.txt`. Xuất là thao tác chỉ đọc, có thể lấy "sản phẩm giai đoạn hiện tại" bất kỳ lúc nào giữa chừng viết, không ảnh hưởng đến Coordinator đang chạy.

Định dạng do **phần mở rộng đường dẫn đầu ra** quyết định (`.txt` / `.epub`):

```text
/export                            # Mặc định TXT, {novelDir}/{NovelName}.txt
/export ~/anh-sang.txt             # Phần mở rộng .txt → TXT
/export ~/anh-sang.epub            # Phần mở rộng .epub → EPUB (Apple Books / WeChat Reading / Kindle converter có thể đọc)
/export from=10 to=30 --overwrite  # Khoảng chương + ghi đè
/export from=10 ~/x.epub --overwrite
```

- **TXT** — `《Tên sách》` → Phân cách tập → Nội dung chương (chế độ phân tầng tiểu thuyết dài tự động thêm phân cách tập). Hai loại dữ liệu nội bộ **không vào xuất**: premise (bản thiết kế sáng tác, chứa đối tượng độc giả mục tiêu / vùng cấm viết v.v. thông tin hậu trường, dành cho tác giả và engine xem), phân cách arc (từ góc nhìn độc giả, arc là cấu trúc nội bộ quá chi tiết). Trình xuất thống nhất tạo "Chương N Tiêu đề", tiêu đề trùng lặp tự mang của writer trong nội dung (`# Chương N…` hoặc `# Tên chương`) sẽ bị loại bỏ.
- **EPUB** — Container chuẩn EPUB 3, bao gồm trang bìa, mục lục, XHTML phân tách theo chương, định danh được dẫn xuất ổn định từ nội dung (xuất lại cùng một cuốn sách, trình đọc nhận diện là phiên bản cập nhật). Không có ảnh bìa.

Các chương chưa hoàn thành trong khoảng sẽ bị bỏ qua và hiển thị trong kết quả, không tính là lỗi.

#### Dùng mô hình khác nhau cho từng nhân vật

Thông qua trường `roles` để phân bổ mô hình khác nhau cho các agent khác nhau, nhân vật chưa cấu hình sẽ dùng mô hình mặc định:

```jsonc
{
  "provider": "openrouter",
  "model": "google/gemini-2.5-flash",
  "reasoning_effort": "medium",
  "providers": {
    "openrouter": { "api_key": "sk-or-v1-xxx", "base_url": "https://openrouter.ai/api/v1" },
    "anthropic": { "api_key": "sk-ant-xxx" }
  },
  "roles": {
    "writer": { "provider": "anthropic", "model": "claude-sonnet-4", "reasoning_effort": "high" },
    "architect": { "provider": "openrouter", "model": "google/gemini-2.5-pro", "reasoning_effort": "low" }
  }
}
```

Các nhân vật có thể cấu hình: `coordinator` / `architect` / `writer` / `editor`

#### Tùy chỉnh proxy

Sau khi chọn bất kỳ Provider nào, điền địa chỉ proxy là được, hoặc dùng Custom Proxy và chỉ định loại giao thức API. `api_key` của proxy tùy chỉnh là tùy chọn; nếu proxy của bạn không cần xác thực, có thể bỏ qua:

```jsonc
{
  "provider": "my-proxy",
  "model": "gpt-4o",
  "providers": {
    "my-proxy": {
      "type": "openai",
      "base_url": "https://proxy.example.com/v1",
      "extra": {
        "user_agent": "my-client/1.0",
        "headers": { "X-Custom-Client": "my-client" }
      }
    }
  }
}
```

Các Provider được hỗ trợ: `openrouter` / `anthropic` / `gemini` / `openai` / `deepseek` / `qwen` / `glm` / `grok` / `ollama` / `bedrock` / `claude-code` và bất kỳ proxy tùy chỉnh nào.

### Dùng gói thuê bao Claude Code (tự động / tự chọn model)

Có thể dùng chính bộ mô hình trong Claude Code (Opus 4.8, Opus 4.7, Sonnet 4.6, Haiku 4.5) để viết truyện mới và viết tiếp bộ truyện hiện có, với hai chế độ: **tự động chọn** model theo vai, hoặc **tự chọn tay** trong bảng `/model`.

> 📖 Hướng dẫn đầy đủ (thiết lập Meridian, preset cân bằng, xử lý sự cố): [docs/claude-code.md](docs/claude-code.md).

**Lưu ý xác thực (quan trọng):** engine bên dưới (`litellm`) chỉ xác thực Anthropic bằng `x-api-key`, **không** đăng nhập gói thuê bao trực tiếp trong app được. Có **hai cách hợp lệ**:

- **Dùng thuê bao (qua Agent SDK):** chạy một cầu nối nội bộ đi qua **Claude Agent SDK chính thức** — ví dụ [Meridian](https://github.com/rynfar/meridian): `npm i -g @rynfar/meridian` → `claude login` → `meridian` (mở `http://127.0.0.1:3456`, nói giao thức Anthropic Messages). Usage rút từ **hạn mức Agent SDK** của gói (Pro $20 / Max 5x $100 / Max 20x $200 mỗi tháng, tính theo giá API, có trần). ⚠️ **Không** dùng proxy phát lại token OAuth thẳng lên `api.anthropic.com` — vi phạm điều khoản và đã bị chặn từ 4/2026.
- **Dùng API key (trả theo token):** đặt `base_url = https://api.anthropic.com` và `api_key = sk-ant-...`. Không có trần tháng — hợp cho truyện dài.

1. **Chuẩn bị backend:** khởi động cầu nối Meridian ở `http://127.0.0.1:3456` (đường thuê bao), hoặc chuẩn bị một API key (đường trả-theo-token).
2. **Thiết lập:** chạy `ainovel-cli` (hoặc `--web`), ở wizard chọn **"Claude Code"**, để Base URL mặc định `http://127.0.0.1:3456` (Meridian) — hoặc đổi thành `https://api.anthropic.com` + điền API key, rồi đồng ý **bật preset tự-chọn cân bằng**. Wizard sẽ dựng sẵn provider `claude-code` (type anthropic) kèm danh mục 4 model.
3. **Tự chọn tay:** gõ `/model` (TUI) hoặc mở bảng Mô hình (Web) — 4 model Claude hiện sẵn để đổi theo từng vai.
4. **Tự động chọn (2 preset):** gõ `/model auto` (TUI) hoặc bấm nút **"Tự chọn: Chuẩn / Tiết kiệm"** (Web) để áp một trong hai preset:

| Vai trò | **Chuẩn** (`/model auto`) | **Tiết kiệm** (`/model auto tietkiem`) |
|---|---|---|
| Writer | `claude-opus-4-8` · high | `claude-sonnet-4-6` · high |
| Architect | `claude-opus-4-8` · high | `claude-sonnet-4-6` · medium |
| Editor | `claude-sonnet-4-6` · medium | `claude-sonnet-4-6` · medium |
| Coordinator | `claude-sonnet-4-6` · medium | `claude-haiku-4-5` · medium |

- **Chuẩn** — chất lượng cao nhất: Opus cho viết prose + dựng thế giới.
- **Tiết kiệm** — bỏ Opus (khoản tốn nhất vì Writer chạy mỗi chương), giữ Writer/Architect/Editor ở Sonnet, Coordinator xuống Haiku. Rẻ hơn ~40–50% nhưng prose vẫn ở mức Sonnet — hợp khi hạn mức thuê bao eo hẹp hoặc viết truyện rất dài.

Mọi thay đổi được ghi vào `~/.ainovel/config.json`. Muốn tinh chỉnh riêng vai nào thì đổi tay trong `/model` (ví dụ đưa Editor lên Opus, hoặc hạ Writer xuống `claude-haiku-4-5` cho rẻ nhất). Với "viết tiếp bộ truyện hiện có": `cd` vào thư mục truyện cũ rồi khởi động — engine tự khôi phục và viết tiếp bằng model Claude đã chọn.

Ví dụ khối `providers.claude-code` tương ứng (tham chiếu `config.example.jsonc`):

```jsonc
{
  "provider": "claude-code",
  "model": "claude-sonnet-4-6",
  "reasoning_effort": "medium",
  "providers": {
    "claude-code": {
      "type": "anthropic",
      "base_url": "http://127.0.0.1:3456",
      "models": ["claude-opus-4-8", "claude-opus-4-7", "claude-sonnet-4-6", "claude-haiku-4-5"]
    }
  },
  "roles": {
    "writer":      { "provider": "claude-code", "model": "claude-opus-4-8",   "reasoning_effort": "high" },
    "architect":   { "provider": "claude-code", "model": "claude-opus-4-8",   "reasoning_effort": "high" },
    "editor":      { "provider": "claude-code", "model": "claude-sonnet-4-6", "reasoning_effort": "medium" },
    "coordinator": { "provider": "claude-code", "model": "claude-sonnet-4-6", "reasoning_effort": "medium" }
  }
}
```

### Tùy chỉnh proxy Anthropic (nâng cao)

Nếu proxy sử dụng giao thức Anthropic và giới hạn chỉ có thể truy cập bởi client Claude Code, `type` nên đặt là `anthropic`, `anthropic_beta` đặt ở tầng trên của `extra`, các HTTP header như Stainless đặt trong `extra.headers`:

```jsonc
{
  "provider": "claude-code-proxy",
  "model": "claude-sonnet-4-6",
  "providers": {
    "claude-code-proxy": {
      "type": "anthropic",
      "api_key": "sk-xxx",
      "base_url": "https://proxy.example.com",
      "extra": {
        "user_agent": "claude-code/2.1.183",
        "anthropic_beta": "claude-code-20250219",
        "headers": {
          "X-Stainless-Lang": "js",
          "X-Stainless-Package-Version": "0.94.0",
          "X-Stainless-Runtime": "node"
        }
      }
    }
  }
}
```

Nếu proxy sử dụng giao thức OpenAI/NewAPI và giới hạn chỉ có thể truy cập bởi client Codex, `type` nên đặt là `openai`, dùng `extra.user_agent` để ghi đè mặc định `litellm-go/0.1`, và truyền các header nhận dạng Codex trong `extra.headers`. `Session_id` và `X-Codex-Turn-Metadata` trong ví dụ nên thay bằng các giá trị ngẫu nhiên ổn định; chúng đồng thời tương thích với mẫu chuyển tiếp Codex của New API và kiểm tra dấu vân tay `x-codex-*` của sub2api:

```jsonc
{
  "provider": "codex-proxy",
  "model": "gpt-5.4",
  "providers": {
    "codex-proxy": {
      "type": "openai",
      "api_key": "sk-xxx",
      "base_url": "https://proxy.example.com/v1",
      "models": ["gpt-5.4", "gpt-5.4-mini", "MiniMax-M3"],
      "api": "responses",
      "extra": {
        "user_agent": "codex-tui/0.142.3 (Mac OS 26.5.1; arm64) Apple_Terminal/470.2 (codex-tui; 0.142.3)",
        "headers": {
          "Originator": "codex-tui",
          "Session_id": "replace-with-random-session-id",
          "X-Codex-Turn-Metadata": "replace-with-random-turn-metadata"
        }
      }
    }
  }
}
```

Về `api_key`:

- Các giao diện được quản lý như `openrouter` / `anthropic` / `gemini` / `openai` / `deepseek` / `qwen` / `glm` / `grok` thường cần điền `api_key`
- `ollama` và `bedrock` cho phép không điền `api_key`; Bedrock cần cấu hình `region`, `access_key_id`, `secret_access_key` (tùy chọn `session_token`) trong `extra`
- Proxy tùy chỉnh đã chỉ định rõ `type` cho phép không điền `api_key`

Ví dụ cấu hình `ollama` cục bộ:

```jsonc
{
  "provider": "ollama",
  "model": "qwen3:latest",
  "providers": {
    "ollama": {
      "base_url": "http://localhost:11434/v1"
    }
  }
}
```

### Phong cách viết

Chuyển đổi qua trường `style` trong tệp cấu hình:

- `default` — Phong cách thông dụng
- `suspense` — Huyền bí trinh thám
- `fantasy` — Kỳ huyễn tiên hiệp
- `romance` — Ngôn tình

### Khử mùi AI và quy tắc tùy chỉnh

Tích hợp sẵn một baseline khử mùi AI (mặc định xuất xưởng): danh sách đen cơ học (câu mẫu / từ mệt mỏi, tích hợp trong mã `rules.SystemDefaults()`, kiểm tra xác định khi commit) + tiêu chí ngữ nghĩa `assets/references/anti-ai-tone.md` (chèn vào writer / editor để tránh và làm bằng chứng).

Muốn chồng thêm sở thích của mình **không cần sửa mã nguồn**: trong thư mục `~/.ainovel/rules/` (toàn cục, đặt bất kỳ `.md` nào, hợp nhất theo thứ tự tên tệp từ điển) hoặc thư mục `./.ainovel/rules/` (cuốn sách này, cũng đặt bất kỳ `.md` nào, cùng hình thức với toàn cục), **viết sở thích bằng ngôn ngữ thông thường là được** (ví dụ: "nhân vật chính đừng viết thành thánh mẫu", "dùng nhiều cảm nhận cơ thể", "mỗi chương khoảng 3000 chữ", "không xuất hiện 'ở một mức độ nào đó'") — không cần định dạng, không cần YAML. Hệ thống sẽ dùng mô hình chuẩn hóa các yêu cầu ngôn ngữ tự nhiên này thành ảnh chụp nhanh quy tắc cuốn sách (khoảng từ / từ cấm / ngưỡng từ mệt mỏi và các ràng buộc có cấu trúc khác + sở thích phong cách), tự động tuân theo khi viết, tự động kiểm tra cơ học khi commit; baseline cơ học cho các câu mẫu AI thông thường và từ mệt mỏi đã được tích hợp sẵn, không viết vẫn dùng được, ghi đè gần nhất, cộng hưởng với baseline tích hợp.

## Cấu trúc đầu ra

Tất cả dữ liệu sáng tác (chương, dàn ý, nhân vật, tiến độ v.v.) được lưu trong thư mục output. Chạy lại sau khi gián đoạn sẽ tự động tiếp tục từ tiến độ lần trước. Xóa thư mục output sẽ bắt đầu lại từ đầu.

```
output/{novel_name}/
├── chapters/           # Bản cuối (Markdown)
│   ├── 01.md
│   └── ...
├── summaries/          # Tóm tắt chương (JSON)
├── drafts/             # Bản nháp chương
├── reviews/            # Báo cáo xét duyệt
├── meta/
│   ├── premise.md      # Tiền đề câu chuyện
│   ├── outline.json    # Dàn ý chương phẳng (chỉ chứa các chương đã triển khai)
│   ├── layered_outline.json # Dàn ý phân tầng (tập hiện tại + tập xem trước, chế độ dài)
│   ├── compass.json   # La bàn định hướng kết cục (chế độ dài)
│   ├── characters.json # Hồ sơ nhân vật
│   ├── world_rules.json# Quy tắc thế giới
│   ├── progress.json   # Trạng thái tiến độ
│   ├── timeline.json   # Dòng thời gian
│   ├── foreshadow.json # Sổ phục bút
│   ├── state_changes.json # Ghi chép thay đổi trạng thái nhân vật
│   ├── style_rules.json# Quy tắc phong cách viết (tinh chỉnh tại ranh giới arc)
│   ├── snapshots/      # Ảnh chụp nhanh trạng thái nhân vật (tiểu thuyết dài)
│   ├── checkpoints.jsonl # Checkpoint cấp Step (thêm vào sau mỗi công cụ thành công)
│   ├── characters.md   # Hồ sơ nhân vật (phiên bản có thể đọc)
│   └── world_rules.md  # Quy tắc thế giới (phiên bản có thể đọc)
```

## Khôi phục điểm dừng

Viết một tiểu thuyết dài có thể mất hàng giờ thậm chí nhiều ngày, sự cố giữa chừng, mất kết nối, Ctrl+C đều là tình huống thường gặp. Hệ thống **tự động khôi phục khi chạy lại trong cùng thư mục**, không cần thao tác thủ công.

### Các tình huống khôi phục

| Thời điểm gián đoạn | Hành vi khôi phục |
|---|---|
| Giai đoạn quy hoạch (đang xây dựng thế giới quan/dàn ý) | Kiểm tra thiết định đã lưu, tự động bổ sung phần còn thiếu |
| Một chương đang viết (có bản nháp chưa nộp) | Tiếp tục viết từ chương đó, đọc bản nháp đã có để tiếp tục |
| Xét duyệt đang tiến hành | Kích hoạt lại xét duyệt Editor |
| Hàng đợi viết lại/đánh bóng chưa xử lý xong | Tiếp tục xử lý các chương chờ viết lại |
| Triển khai arc/tập bị gián đoạn (đã xét duyệt nhưng chưa triển khai arc tiếp) | Tự động phát hiện arc/tập khung xương, kích hoạt Architect triển khai |
| Can thiệp người dùng chưa hoàn thành | Chèn lại lệnh can thiệp lần trước |
| Gián đoạn khi đang viết bình thường | Tiếp tục từ chương tiếp theo |

### Nguyên lý hoạt động

Tất cả sản phẩm sáng tác được lưu bền vững trong thư mục `output/`. Sau mỗi lần thực thi công cụ thành công sẽ ghi checkpoint (`meta/checkpoints.jsonl`). Khi khởi động lại:

1. Đọc `progress.json` + checkpoint gần nhất + tín hiệu chờ xử lý
2. Tạo lệnh khôi phục chính xác đến cấp step (ví dụ: "Chương 7 draft đã lưu vào đĩa, vui lòng tiếp tục check_consistency")
3. Một lần `Prompt` khởi động Coordinator, vào vòng lặp dài tiếp tục sáng tác

> Ghi tệp sử dụng thao tác nguyên tử temp + fsync + rename, ngay cả khi mất điện giữa chừng lúc đang ghi cũng không làm hỏng dữ liệu đã có.

## Can thiệp thời gian thực (Steer)

Có thể chèn ý kiến sửa đổi vào ô nhập liệu bất kỳ lúc nào trong quá trình sáng tác, **không cần tạm dừng hay khởi động lại**.

### Chế độ TUI

Sau khi khởi động sáng tác, ô nhập liệu dưới cùng tự động chuyển sang chế độ can thiệp:

```
❯ Đưa tuyến cảm xúc lên chương 4, tăng thêm cảnh đối đầu giữa nam và nữ chính
```

Nhập xong nhấn Enter, hệ thống tự động:
1. Ghi lệnh can thiệp vào `run.json` (dùng cho khôi phục sự cố)
2. Chèn vào Coordinator đang chạy
3. Coordinator đánh giá phạm vi ảnh hưởng, quyết định sửa thiết định, viết lại các chương đã có, hay điều chỉnh trong các chương tiếp theo

### Ví dụ can thiệp

| Lệnh can thiệp | Phản hồi có thể của hệ thống |
|---|---|
| "Đổi nhân vật chính thành nữ" | Sửa thiết định nhân vật, đánh giá xem các chương đã viết có cần viết lại không |
| "Đưa tuyến cảm xúc lên chương 4" | Điều chỉnh dàn ý, có thể viết lại chương 4 và các chương tiếp theo |
| "Thêm một nhân vật phản diện" | Cập nhật hồ sơ nhân vật và quy tắc thế giới, giới thiệu trong các chương tiếp theo |
| "Nhịp độ quá chậm, đẩy nhanh tiến độ" | Điều chỉnh mật độ dàn ý các chương tiếp theo |

## Triết lý thiết kế

> **Chuyển sự phức tạp từ mã sang mô hình.** Mã càng ít, nơi có thể hỏng càng ít. Quyền quyết định giao cho nhân vật giỏi quyết định hơn.

### LLM điều khiển, càng đơn giản càng ổn định

- **Quyền quyết định thuộc LLM** — Tất cả quyết định luồng đều do Coordinator tự chủ phán đoán, Host không can thiệp. Khi công cụ thất bại sẽ trả về lỗi có cấu trúc, LLM tự quyết định thử lại hay điều chỉnh chiến lược
- **Công cụ chỉ trả về sự kiện** — IO nguyên tử + ghi checkpoint, giá trị trả về là các trường JSON sự kiện (`final_verdict` / `pending_rewrites` / `arc_end_reached`), không kèm theo bất kỳ chuỗi lệnh nào
- **Reminder điều khiển mỗi lượt** — Host đọc tầng sự kiện trước mỗi lần gọi LLM, chạy generator hàm thuần túy để tạo `<system-reminder>` chèn vào, lệnh không vào lịch sử lưu trữ, mỗi lượt tính lại từ sự kiện
- **StopGuard canh vật lý** — Khi `Phase ≠ Complete`, Coordinator vật lý không thể `end_turn`, liên tục bị chặn vượt ngưỡng mới nâng cấp kết thúc
- **Từ chối biên soạn phức tạp** — Không có task queue, không có scheduler, không có policy engine. Một lần Run của Coordinator là luồng điều khiển duy nhất
- **Mô hình càng mạnh lợi ích càng lớn** — Kiến trúc để lại quyền quyết định trong prompt và ngữ nghĩa công cụ, sau khi nâng cấp mô hình trực tiếp hưởng lợi, Host không cần sửa một dòng nào

### Vòng kín hoàn toàn tự động

Nhập một câu, đầu ra tiểu thuyết hoàn chỉnh:

```
"Viết một tiểu thuyết huyền bí" → Xây dựng thế giới quan → Thiết kế nhân vật → Quy hoạch dàn ý
                → Viết từng chương → Xét duyệt chất lượng → Tự động viết lại
                → Tóm tắt cấp arc → Ảnh chụp nhân vật → Thành sách hoàn chỉnh
```

- **Coordinator tự chủ điều phối** — Trong một vòng lặp dài đọc tầng sự kiện + Reminder quyết định bước tiếp theo, không cần Host can thiệp
- **Writer tự chủ sáng tác** — Mỗi chương độc lập hoàn thành vòng kín hoàn chỉnh plan → draft → check → commit
- **Editor tự chủ xét duyệt** — Phân tích vấn đề cấu trúc xuyên chương, đưa ra phán quyết và phạm vi ảnh hưởng
- **Architect tự chủ xây dựng** — Suy ra thiết định hoàn chỉnh từ một câu yêu cầu, tự chủ triển khai quy hoạch tiếp theo tại ranh giới arc/tập
- **Quản lý phục bút tự động** — Cài đặt, thúc đẩy, thu hồi toàn bộ được Agent tự theo dõi
- **Điều tiết nhịp độ tự động** — Theo dõi lịch sử tuyến tự sự và loại hook, tránh các chương liên tiếp có cấu trúc giống nhau

### Tách biệt sự kiện và lệnh

Công cụ chỉ trả về sự kiện, lệnh được Reminder tính lại từ tầng sự kiện mỗi lượt:

- `commit_chapter` / `save_review` trả về sự kiện có cấu trúc (`final_verdict` / `pending_rewrites` / `arc_end_reached` / `next_chapter`), không kèm theo bất kỳ chuỗi `[hệ thống]` nào
- Generator hàm thuần túy trong `internal/host/reminder/` đọc `Progress` + `Outline`, mỗi lượt pre-turn tạo `<system-reminder>`: `flow` (hiện tại nên làm gì / phanh dừng cuối arc) / `queue_guard` (hàng đợi chưa xử lý xong cấm chương mới) / `book_complete` (toàn bộ sách hoàn thành mới thả). Dự phòng vật lý do `StopGuard` từ chối `end_turn` khi `phase≠Complete` đảm nhận
- Reminder chỉ tồn tại một lượt, không vào lịch sử, không tham gia nén; quy tắc có unit test, thoái hóa có thể được kiểm tra hồi quy phát hiện

Như vậy lệnh sẽ không bị nuốt bởi gọi chuỗi, cũng không bị trôi dạt trong sản phẩm công cụ. Sửa lỗi chỉ cần thêm một generator + một test.

## Ngăn xếp công nghệ

- **Go 1.25** — Ngôn ngữ chính
- **[agentcore](https://github.com/voocel/agentcore)** — Nhân Agent cực giản (tool-calling + streaming)
- **[litellm](https://github.com/voocel/litellm)** — Thích ứng giao diện LLM thống nhất
- **[Bubble Tea](https://github.com/charmbracelet/bubbletea)** — Framework TUI terminal

## Tài liệu thêm

- [CHANGELOG.md](CHANGELOG.md) — nhật ký thay đổi
- [CLAUDE.md](CLAUDE.md) — định hướng cho AI agent / người đóng góp
- [docs/](docs/) — kiến trúc, quản lý ngữ cảnh, đánh giá, quan sát, Claude Code, hướng dẫn sử dụng

## Giấy phép

MIT

Dự án này tích cực tham gia và công nhận [cộng đồng linux.do](https://linux.do/).

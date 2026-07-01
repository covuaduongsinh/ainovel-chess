# Hướng dẫn sử dụng ainovel (giao diện Web) — dành cho người mới bắt đầu

> Tài liệu này viết cho người **chưa từng dùng phần mềm**. Bạn chỉ cần đọc lần lượt từ trên
> xuống là có thể tự cài đặt, mở giao diện, hiểu **từng nút bấm — từng ô nhập liệu**, và viết
> được cuốn tiểu thuyết đầu tiên. Không cần biết lập trình.

---

## Mục lục

1. [Ainovel là gì?](#1-ainovel-là-gì)
2. [Vài khái niệm nên biết trước](#2-vài-khái-niệm-nên-biết-trước)
3. [Chuẩn bị & cài đặt](#3-chuẩn-bị--cài-đặt)
4. [Màn hình Thiết lập lần đầu (từng ô)](#4-màn-hình-thiết-lập-lần-đầu-từng-ô)
5. [Bản đồ màn hình chính](#5-bản-đồ-màn-hình-chính)
6. [Chi tiết từng khu vực & từng nút/ô](#6-chi-tiết-từng-khu-vực--từng-nútô)
7. [Các cửa sổ chức năng (pop-up) — từng ô, từng nút](#7-các-cửa-sổ-chức-năng-pop-up--từng-ô-từng-nút)
8. [Hướng dẫn theo tình huống (làm theo từng bước)](#8-hướng-dẫn-theo-tình-huống-làm-theo-từng-bước)
9. [Phím tắt](#9-phím-tắt)
10. [Câu hỏi thường gặp & xử lý sự cố](#10-câu-hỏi-thường-gặp--xử-lý-sự-cố)
11. [Các cách dùng khác (nâng cao, tùy chọn)](#11-các-cách-dùng-khác-nâng-cao-tùy-chọn)

---

## 1. Ainovel là gì?

**ainovel** là một phần mềm giúp bạn **viết tiểu thuyết dài bằng AI, gần như hoàn toàn tự động**.

Ý tưởng cốt lõi rất đơn giản:

> **Bạn gõ một câu yêu cầu → AI tự viết cả cuốn sách.**

Ví dụ bạn gõ: *"Viết một truyện tiên hiệp về chàng trai nghèo tu luyện thành tiên"* — phần mềm sẽ
tự lên dàn ý, xây dựng nhân vật, viết từng chương, tự kiểm tra chất lượng và viết tiếp cho đến khi
hoàn thành. Bạn có thể ngồi xem, và **chen ý kiến bất cứ lúc nào** (ví dụ: "thêm một nhân vật phản diện").

### Bên trong có một "ê-kíp làm sách" gồm 4 vai AI

Bạn không cần điều khiển từng vai — chúng tự phối hợp với nhau. Nhưng hiểu sơ để dễ theo dõi màn hình:

| Vai (tên trên màn hình) | Ví như ai trong ê-kíp | Nhiệm vụ |
|---|---|---|
| **Điều phối** | Tổng đạo diễn | Quyết định lúc nào viết, lúc nào xét duyệt, lúc nào lên kế hoạch tiếp |
| **Kiến trúc** | Người dựng cốt truyện | Tạo tiền đề, dàn ý, hồ sơ nhân vật, bối cảnh thế giới |
| **Người viết** | Nhà văn | Viết nội dung từng chương |
| **Biên tập** | Biên tập viên | Đọc lại, chấm điểm, chỉ ra lỗi, yêu cầu viết lại nếu cần |

💡 **Điều quan trọng nhất cần nhớ:** bạn không phải "ra lệnh" cho từng vai. Bạn chỉ cần **nhập yêu cầu
và nhấn nút Bắt đầu** — mọi thứ còn lại diễn ra tự động.

---

## 2. Vài khái niệm nên biết trước

Bạn sẽ gặp các từ này trên màn hình. Không cần học thuộc, chỉ cần biết nghĩa:

| Thuật ngữ | Nghĩa dễ hiểu |
|---|---|
| **Tiền đề** | Ý tưởng gốc của truyện (bối cảnh, nhân vật chính, điểm hấp dẫn). Đây là "hậu trường", không in vào sách. |
| **Dàn ý (Đại cương)** | Danh sách các chương và sự kiện chính của từng chương. |
| **La bàn định hướng** | "Kim chỉ nam" cho truyện dài: hướng đi tới cái kết + quy mô ước tính (vd "dự kiến 4–6 tập"). AI tự cập nhật khi viết. |
| **Chương** | Một chương truyện đã viết xong, lưu thành file. |
| **Nhân vật** | Hồ sơ các nhân vật (tên, vai trò, tính cách, tuyến phát triển). |
| **Phục bút** | Chi tiết "gài" trước để về sau khai thác (giúp truyện logic, không quên tình tiết). |
| **Checkpoint (điểm khôi phục)** | Phần mềm tự lưu tiến độ liên tục. Lỡ tắt máy/mất điện, mở lại là viết tiếp được, không mất dữ liệu. |
| **Chi phí / Ngân sách** | Mỗi lần AI làm việc sẽ tốn một ít tiền tính theo lượng chữ (gọi là "token") trả cho nhà cung cấp AI. Phần mềm hiển thị số tiền đã dùng. |
| **Nhà cung cấp (Provider)** | Hãng cung cấp AI mà bạn dùng (OpenAI, Anthropic, Google Gemini, OpenRouter…). |
| **Mô hình (Model)** | "Bộ não" AI cụ thể của nhà cung cấp (vd `gpt-4o`, `claude-sonnet-4-5`, `gemini-2.5-flash`). |
| **Mức suy luận** | AI "suy nghĩ" kỹ tới đâu: `off / low / medium / high…`. Kỹ hơn = chất lượng tốt hơn nhưng chậm và tốn hơn. |
| **Phong cách** | Giọng văn định sẵn: mặc định / hồi hộp / kỳ ảo / ngôn tình. |

⚠️ **Về chi phí:** phần mềm dùng AI của bên thứ ba, nên bạn cần **khóa API (API Key)** của một nhà
cung cấp, và mỗi cuốn sách sẽ tốn một khoản tiền (tùy nhà cung cấp và độ dài truyện). Xem mục
[Ngân sách](#101-tôi-có-bị-tốn-nhiều-tiền-không) để kiểm soát.

---

## 3. Chuẩn bị & cài đặt

### 3.1 Bạn cần gì trước khi bắt đầu

1. **Một máy tính** (hướng dẫn này minh họa trên Windows).
2. **Khóa API (API Key) của một nhà cung cấp AI.** Đây là một chuỗi ký tự bí mật, thường bắt đầu
   bằng `sk-...`. Bạn lấy nó trên trang web của nhà cung cấp (xem [mục 4](#4-màn-hình-thiết-lập-lần-đầu-từng-ô)).
   - 💡 Nếu bạn hoàn toàn mới và muốn đơn giản, **OpenRouter** là lựa chọn dễ vì một khóa dùng được
     cho nhiều mô hình.

### 3.2 Cài đặt phần mềm

Phần mềm có tên lệnh là **`ainovel-cli`**. Có hai cách phổ biến:

- **Tải bản dựng sẵn (khuyến nghị cho Windows):** vào trang **Releases** của dự án trên GitHub, tải
  file dành cho Windows về, giải nén.
- **Nếu bạn đã cài Go:** chạy `go install github.com/voocel/ainovel-cli/cmd/ainovel-cli@latest`.

Sau khi cài, bạn sẽ có một file chạy tên `ainovel-cli` (trên Windows là `ainovel-cli.exe`).

### 3.3 Mở giao diện Web

Mở **Terminal / PowerShell / Command Prompt**, di chuyển tới thư mục chứa file `ainovel-cli`, rồi gõ:

```
ainovel-cli --web
```

Chuyện gì xảy ra:

- Phần mềm tự khởi động một máy chủ **chỉ chạy trong máy bạn** (địa chỉ `http://127.0.0.1:...`).
- **Trình duyệt tự động mở ra** trang giao diện. Nếu không tự mở, hãy đọc dòng địa chỉ mà cửa sổ
  terminal in ra và tự dán vào trình duyệt.
- Cổng (con số cuối địa chỉ) được **tự chọn ngẫu nhiên** cho khỏi trùng. Nếu bạn muốn cố định một
  cổng, thêm `--port`, ví dụ:

```
ainovel-cli --web --port 8080
```

💡 **Mỗi thư mục là một tác phẩm.** Dữ liệu truyện được lưu vào thư mục con `output/` ngay tại nơi bạn
chạy lệnh. Muốn viết cuốn khác độc lập, hãy chạy lệnh trong một thư mục khác.

⚠️ **Đừng tắt cửa sổ terminal** trong khi đang dùng — đó là "động cơ" chạy phần mềm. Đóng nó là dừng
phần mềm (nhưng yên tâm: tiến độ đã được lưu, mở lại vẫn viết tiếp được).

---

## 4. Màn hình Thiết lập lần đầu (từng ô)

Lần đầu chạy, do chưa có cấu hình, trình duyệt sẽ hiện trang **"✦ Chào mừng đến với ainovel — Thiết lập
lần đầu"**. Đây là nơi bạn khai báo dùng AI nào. (Ở chế độ Web, việc thiết lập diễn ra **ngay trong
trình duyệt**, không phải trên cửa sổ dòng lệnh.)

Trang này có một biểu mẫu với các ô sau, lần lượt từ trên xuống:

### Ô ① — **Nhà cung cấp (Provider)**  *(ô chọn / dropdown)*

Bấm vào để mở danh sách và chọn hãng AI bạn dùng. Có 12 lựa chọn:

`OpenRouter` · `Claude Code` · `Anthropic` · `Gemini` · `OpenAI` · `DeepSeek` · `Qwen` · `GLM` ·
`Grok` · `Ollama` · `Bedrock` · `Custom Proxy`.

💡 Gợi ý cho người mới: chọn **OpenRouter** (một khóa dùng nhiều mô hình) hoặc nhà cung cấp mà bạn đã
có sẵn khóa API.

### Ô ② — **Tên nhà cung cấp tùy chỉnh** và **Giao thức API**  *(chỉ hiện khi chọn `Custom Proxy`)*

Hai ô này **ẩn đi** với người dùng bình thường. Chúng chỉ xuất hiện khi bạn chọn `Custom Proxy` (dành
cho người tự dựng máy chủ trung gian riêng):
- **Tên nhà cung cấp tùy chỉnh:** đặt một cái tên bất kỳ, vd `my-proxy`.
- **Giao thức API:** chọn kiểu tương thích — `Tương thích OpenAI`, `Tương thích Anthropic`, hoặc
  `Tương thích Gemini`.

Nếu bạn không dùng máy chủ riêng, **bỏ qua** hai ô này.

### Ô ③ — **API Key**  *(ô mật khẩu)*

Dán **khóa API** của bạn vào đây (thường dạng `sk-...`). Ô này hiển thị dạng chấm để bảo mật.

- Với vài nhà cung cấp chạy nội bộ (như `Ollama`, `Claude Code`), nhãn ô sẽ đổi thành
  **"API Key (có thể để trống)"** — nghĩa là bạn có thể để trống.
- **Lấy khóa ở đâu?** Đăng nhập trang của nhà cung cấp → tìm mục *API Keys* → tạo khóa mới → sao chép.
  Ví dụ: OpenRouter tại trang cài đặt khóa của họ; OpenAI/Anthropic/Google trong bảng điều khiển nhà
  phát triển tương ứng.

⚠️ Khóa API là **bí mật** như mật khẩu — đừng chia sẻ hay đăng công khai.

### Ô ④ — **Base URL**  *(ô văn bản, thường để trống)*

Địa chỉ máy chủ AI. **Để trống** thì phần mềm dùng địa chỉ mặc định của nhà cung cấp (đúng cho hầu hết
mọi người). Ô gợi ý sẽ hiện sẵn địa chỉ mặc định. Chỉ điền khi bạn dùng máy chủ trung gian riêng.

### Ô ⑤ — **Tên mô hình**  *(ô văn bản)*

Gõ tên "bộ não" AI muốn dùng. Ngay dưới ô có dòng **gợi ý** thay đổi theo nhà cung cấp bạn chọn. Vài ví dụ:

| Nhà cung cấp | Ví dụ tên mô hình |
|---|---|
| OpenRouter | `google/gemini-2.5-flash` (OpenRouter dùng dạng `hãng/mô-hình`) |
| Anthropic | `claude-sonnet-4-5` |
| Gemini | `gemini-2.5-flash` |
| OpenAI | `gpt-4o` |
| Ollama | `llama3.1` |

⚠️ **Lưu ý về tiền tố:** chỉ **OpenRouter** mới viết dạng `hãng/mô-hình` (vd `google/...`). Các nhà
cung cấp gốc dùng **tên trần** (vd `gemini-2.5-flash`, **không** thêm `google/`). Ô gợi ý dưới ô sẽ
nhắc bạn điều này.

> Riêng **Claude Code**: cần một proxy nội bộ đăng nhập bằng tài khoản Claude Code. Nếu **để trống ô
> mô hình**, phần mềm dùng bộ mặc định cân bằng (Opus cho vai Người viết/Kiến trúc, Sonnet cho phần
> còn lại). Đây là lựa chọn nâng cao.

### Nút — **Lưu & bắt đầu**

Bấm để lưu cấu hình. Nếu thành công, bạn thấy dòng chữ xanh:

> ✓ Đã lưu cấu hình. Đang khởi động xưởng sáng tác...

Trang sẽ **tự chuyển sang màn hình chính** sau vài giây. Nếu có lỗi (vd khóa sai), một dòng chữ **đỏ**
hiện ra để bạn sửa rồi bấm lại.

---

## 5. Bản đồ màn hình chính

Sau khi thiết lập xong, bạn vào **màn hình chính** (tiêu đề "Xưởng sáng tác AI"). Nó chia làm **5 khu vực**:

```
┌───────────────────────────────────────────────────────────────────────┐
│ ①  THANH TRÊN CÙNG:  ✦  Tên tác phẩm  [Trạng thái]   Mô hình · Chi phí · Ngữ cảnh · ⚙ Mô hình │
├──────────────┬──────────────────────────────────┬─────────────────────┤
│ ② CỘT TRÁI   │ ③ KHU GIỮA                       │ ④ CỘT PHẢI          │
│              │                                  │                     │
│  Tiến độ     │   AI đang viết                   │  [Đại cương]        │
│  Tác nhân    │   (nội dung hiện ra tại đây)      │  [Nhân vật]         │
│              │                                  │  [Chi tiết]         │
│              │   Nhật ký hoạt động              │                     │
├──────────────┴──────────────────────────────────┴─────────────────────┤
│ ⑤ THANH THAO TÁC DƯỚI:  [6 nút lệnh]   [Ô nhập yêu cầu]   [Nút Bắt đầu] │
└───────────────────────────────────────────────────────────────────────┘
```

Phần dưới mô tả **chi tiết từng khu vực**.

---

## 6. Chi tiết từng khu vực & từng nút/ô

### 6.1 ① Thanh trên cùng

Dải ngang trên đỉnh, hiển thị thông tin tổng quát (từ trái sang phải):

| Thành phần | Hiển thị mặc định | Ý nghĩa |
|---|---|---|
| **Logo** ✦ | ✦ | Biểu tượng phần mềm. |
| **Tên tác phẩm** | "Chưa có tác phẩm" | Tên cuốn tiểu thuyết đang viết (tự đặt sau khi AI lên tiền đề). |
| **Badge trạng thái** | "SẴN SÀNG" | Tình trạng hiện tại (xem bảng dưới). |
| **Mô hình** | "—" | Tên mô hình đang dùng, kèm mức suy luận nếu có (vd `gpt-4o · high`). Rê chuột để xem nhà cung cấp. |
| **Chi phí** | "$0.00" | Số tiền API đã tiêu. Nếu bạn đặt ngân sách, hiện dạng `$1.20 / $5` (đã dùng / giới hạn). |
| **Ngữ cảnh** | "0%" | Mức "trí nhớ làm việc" của AI đã dùng, kèm một thanh màu. |
| **Nút ⚙ Mô hình** | ⚙ Mô hình | Bấm để mở cửa sổ đổi mô hình / mức suy luận (xem [mục 7.6](#76-cửa-sổ-mô-hình--mức-suy-luận)). |

**Các trạng thái của badge:**

| Badge | Nghĩa |
|---|---|
| **SẴN SÀNG** | Chưa bắt đầu, đang chờ bạn nhập yêu cầu. |
| **ĐANG VIẾT** | AI đang chạy (viết chương). |
| **ĐÁNH GIÁ** | Biên tập đang xét duyệt. |
| **VIẾT LẠI** | Đang sửa/viết lại chương theo góp ý. |
| **HOÀN THÀNH** | Cả cuốn đã xong. |

**Thanh "Ngữ cảnh" đổi màu** để cảnh báo trí nhớ AI:
- 🟢 **Xanh** (dưới 70%): thoải mái.
- 🟡 **Vàng** (70–85%): sắp đầy, phần mềm chuẩn bị "nén" bớt thông tin cũ.
- 🔴 **Đỏ** (trên 85%): gần đầy, đang nén. Đây là hoạt động bình thường, bạn không cần làm gì.

### 6.2 ② Cột trái — Tiến độ & Tác nhân

**Khối "Tiến độ"** — bảng 4 ô số:

| Ô | Ý nghĩa |
|---|---|
| **Chương hiện tại** | Số thứ tự chương AI đang viết. |
| **Đã hoàn thành** | Số chương đã viết xong. |
| **Tổng chương** | Tổng số chương dự kiến. |
| **Tổng số chữ** | Tổng số chữ đã viết. |

Bên dưới có thể xuất hiện thêm hai dòng (khi có dữ liệu):
- **📖 …** — tập/cung hiện tại và tập kế tiếp (với truyện dài nhiều tập).
- **⟲ Có thể khôi phục: …** — báo có điểm khôi phục để tiếp tục (khi đang không chạy).

**Khối "Tác nhân (Agents)"** — cho thấy 4 vai AI đang làm gì. Mỗi vai (**Điều phối / Kiến trúc /
Người viết / Biên tập**) hiển thị tên, trạng thái (vd `idle` = đang chờ, hoặc đang "thinking/writing"…)
và việc đang làm. Vai đang hoạt động được **tô sáng**; vai đang chờ thì mờ. Khi chưa chạy, hiện
"Chưa có hoạt động."

### 6.3 ③ Khu giữa — Nội dung & Nhật ký

**Panel "AI đang viết"** — khu vực lớn hiển thị **nội dung AI đang gõ ra theo thời gian thực**. Bạn sẽ
thấy chữ chạy dần khi AI viết. Panel tự cuộn xuống theo.

**Panel "Nhật ký hoạt động"** — danh sách các sự kiện theo thời gian, mỗi dòng gồm: **giờ** ·
**loại sự kiện** · **mô tả ngắn**. Màu sắc cho biết mức độ: bình thường (trắng), cảnh báo (vàng),
lỗi (đỏ), thành công (xanh). Đây là nơi theo dõi "AI đang làm gì" một cách vắn tắt.

### 6.4 ④ Cột phải — 3 tab tra cứu

Có 3 tab, bấm để chuyển. Tab **Đại cương** được chọn sẵn.

| Tab | Hiển thị |
|---|---|
| **Đại cương** | Danh sách các chương: "Chương N · Tên chương" và sự kiện cốt lõi. Khi chưa có, ghi "Chưa có đại cương." |
| **Nhân vật** | Danh sách nhân vật chính; và mục "Phụ gần đây (số lượng)" liệt kê nhân vật phụ mới xuất hiện. |
| **Chi tiết** | Các thông tin hậu trường: Phase/Flow, Tiền đề, Định hướng kết (Compass), Quy mô ước tính, Commit gần nhất, Đánh giá gần nhất, Checkpoint, chương "Chờ viết lại", "Can thiệp đang chờ", "Tóm tắt gần đây". |

### 6.5 ⑤ Thanh thao tác dưới cùng

Đây là nơi bạn **ra lệnh** cho phần mềm. Gồm ba phần.

**a) Hàng 6 nút lệnh** (từ trái sang phải):

| Nút | Chức năng | Mở cửa sổ? |
|---|---|---|
| **✧ Cộng tác** | Trò chuyện với AI để bàn ý tưởng/định hướng trước khi viết. | Có → [7.5](#75-cửa-sổ-cộng-tác) |
| **⇲ Nhập truyện** | Nạp một file truyện có sẵn (.txt/.md) rồi viết tiếp. | Có → [7.2](#72-cửa-sổ-nhập-truyện-có-sẵn) |
| **⟲ Phỏng tác** | Học văn phong từ các bài mẫu bạn cung cấp. | Có → [7.3](#73-cửa-sổ-phỏng-tác-mô-phỏng-văn-phong) |
| **⚕ Chẩn đoán** | Phân tích "sức khỏe" của cuốn truyện, chỉ ra vấn đề. | Có → [7.4](#74-cửa-sổ-chẩn-đoán) |
| **⬇ Xuất file** | Xuất các chương thành file TXT hoặc EPUB. | Có → [7.1](#71-cửa-sổ-xuất-file) |
| **⏸ Tạm dừng** | Dừng tiến trình đang chạy. *(chỉ hiện khi AI đang viết)* | Không |

**b) Ô nhập yêu cầu chính** *(textarea lớn)*

Đây là ô bạn gõ nội dung. **Câu gợi ý (placeholder) trong ô thay đổi theo trạng thái**:

| Khi | Ô gợi ý |
|---|---|
| SẴN SÀNG | "Nhập một câu yêu cầu sáng tác rồi nhấn Bắt đầu…" |
| ĐANG VIẾT | "Nhập ý kiến can thiệp (vd: thêm một nhân vật phản diện)…" |
| HOÀN THÀNH | "Tác phẩm đã hoàn thành. Có thể nhập để tiếp tục mở rộng…" |
| Có thể tiếp tục | "Nhập để tiếp tục, hoặc để trống rồi nhấn Khôi phục…" |

- Ô tự cao lên khi bạn gõ nhiều dòng.
- 💡 **Gửi nhanh bằng phím tắt:** nhấn **Ctrl + Enter** (Windows/Linux) hoặc **Cmd + Enter** (Mac).

**c) Nút chính (bên phải ô nhập)** — **nhãn và tác dụng đổi theo trạng thái**:

| Nhãn nút | Khi nào xuất hiện | Bấm vào sẽ |
|---|---|---|
| **Bắt đầu** | Chưa có gì (mới tinh) | Bắt đầu sáng tác từ câu yêu cầu bạn vừa nhập. |
| **Can thiệp** | Đang viết | Chèn ý kiến của bạn vào (AI sẽ điều chỉnh, viết lại phần bị ảnh hưởng). |
| **Tiếp tục** | Đang dừng và bạn **có** gõ nội dung | Viết tiếp theo hướng bạn vừa nhập. |
| **Khôi phục** | Đang dừng và ô nhập **trống** | Tiếp tục từ điểm khôi phục gần nhất. |
| **Đã hoàn thành** | Cả cuốn đã xong | (Có thể nhập thêm để mở rộng.) |

**d) Thanh thông báo (toast)** — dải nhỏ hiện chớp nhoáng ở đáy để báo **thành công** (xanh) hoặc
**lỗi** (đỏ), tự ẩn sau vài giây.

---

## 7. Các cửa sổ chức năng (pop-up) — từng ô, từng nút

Khi bấm các nút lệnh, một **cửa sổ pop-up** hiện ra (nền phía sau mờ đi). 💡 **Đóng cửa sổ:** bấm ra
vùng tối bên ngoài, hoặc nhấn phím **Esc** (trừ cửa sổ đang bắt buộc trả lời).

### 7.1 Cửa sổ "Xuất file"

Mở bằng nút **⬇ Xuất file**. Dùng để gộp các chương đã viết thành một file đọc được.

| Ô / Nút | Ý nghĩa |
|---|---|
| **Đường dẫn** | Nơi lưu file. Để trống = lưu mặc định trong thư mục tác phẩm. **Đuôi file quyết định định dạng:** `.txt` (văn bản thường) hoặc `.epub` (sách điện tử). Vd: `D:\truyen\tac-pham.epub`. |
| **Từ chương** | Chương bắt đầu xuất. Để trống = từ đầu. |
| **Đến chương** | Chương kết thúc xuất. Để trống = đến cuối. |
| **☐ Ghi đè nếu file đã tồn tại** | Tích vào nếu muốn ghi đè file trùng tên. |
| Nút **Hủy** | Đóng, không làm gì. |
| Nút **Xuất** | Thực hiện. Xong sẽ báo: "✓ Đã xuất N chương → đường dẫn". |

### 7.2 Cửa sổ "Nhập truyện có sẵn"

Mở bằng nút **⇲ Nhập truyện**. Dùng khi bạn **đã có một truyện viết dở** và muốn AI đọc hiểu rồi viết tiếp.

| Ô / Nút | Ý nghĩa |
|---|---|
| **Đường dẫn file truyện** | Đường dẫn tới file `.txt` hoặc `.md`. Vd: `D:\truyen\tieu-thuyet.txt`. |
| **Bắt đầu từ chương** | Viết tiếp từ chương nào. `0` = từ đầu (mặc định). |
| Nút **Hủy** / **Nhập** | Hủy, hoặc bắt đầu nhập. Khi nhập, một **cửa sổ tiến trình** hiện ra để theo dõi (xem [7.7](#77-cửa-sổ-tiến-trình)). |

Phần mềm sẽ **tự suy ngược ra tiền đề, nhân vật, dàn ý** từ nội dung file rồi mới viết tiếp.

### 7.3 Cửa sổ "Phỏng tác (mô phỏng văn phong)"

Mở bằng nút **⟲ Phỏng tác**. Dùng khi bạn muốn AI **viết theo văn phong của một tác giả/tác phẩm mẫu**.

- Cách chính: đặt vài bài mẫu (`.txt`/`.md`) vào thư mục tên **`simulate`** trong thư mục bạn chạy phần
  mềm, rồi bấm **Chạy**. AI sẽ đọc và rút ra "hồ sơ văn phong".
- **Ô "Hoặc nhập sẵn hồ sơ đã có"**: nếu bạn đã có sẵn một file hồ sơ `.json` từ trước, dán đường dẫn
  vào đây thay vì phân tích lại thư mục.
- Nút **Hủy** / **Chạy**. Khi chạy sẽ hiện cửa sổ tiến trình.

### 7.4 Cửa sổ "Chẩn đoán"

Mở bằng nút **⚕ Chẩn đoán**. Phần mềm tự phân tích rồi hiện báo cáo (không cần nhập gì):

- **Thống kê:** số chương (đã xong/tổng), tổng chữ, số lần đánh giá, số lần viết lại, điểm trung bình.
- **Danh sách vấn đề phát hiện được**, mỗi mục gắn mức độ: `[CRITICAL]` (nghiêm trọng) / `[WARNING]`
  (cảnh báo) / `[INFO]` (thông tin), kèm bằng chứng và đề xuất khắc phục.
- Nếu không có vấn đề: "✓ Không phát hiện vấn đề."

Đóng bằng phím **Esc** hoặc bấm ra ngoài.

### 7.5 Cửa sổ "Cộng tác"

Mở bằng nút **✧ Cộng tác**. Đây là chế độ **bàn bạc với AI trước khi viết** (hoặc giữa chừng, để định
hướng chặng tiếp theo). Có hai giai đoạn:

**Giai đoạn 1 — Nhập ý tưởng (khi bắt đầu mới):**
- Ô **"Ý tưởng ban đầu của bạn"**: gõ ý tưởng gốc, vd *"Một câu chuyện trinh thám lấy bối cảnh Hà Nội
  thập niên 1990…"*.
- Nút **Hủy** / **Bắt đầu trò chuyện**.

**Giai đoạn 2 — Trò chuyện (màn hình 2 cột):**
- **Cột trái** = khu trò chuyện: lịch sử đối thoại với AI, các **nút gợi ý** (bấm để điền nhanh), ô
  **"Nhập trả lời cho AI…"** và nút **Gửi**. 💡 Trong ô này, nhấn **Enter** để gửi.
- **Cột phải** = **bản dự thảo chỉ thị sáng tác** mà AI tổng hợp dần từ cuộc trò chuyện.
- Nút cuối: **Hủy**, và nút chính là **Bắt đầu sáng tác** (khi mới) hoặc **Áp dụng & tiếp tục** (khi
  đang viết dở). Nút này chỉ bật khi AI đã sẵn sàng.

### 7.6 Cửa sổ "Mô hình & mức suy luận"

Mở bằng nút **⚙ Mô hình** trên thanh trên cùng. Cho phép chọn mô hình AI **riêng cho từng vai**.

Bảng gồm các cột: **Vai trò** (Mặc định / Điều phối / Kiến trúc / Người viết / Biên tập) · **Provider** ·
**Mô hình** · **Suy luận** · nút **Áp dụng** ở mỗi hàng.

- Đổi Provider/Mô hình/Mức suy luận cho một hàng rồi bấm **Áp dụng** để lưu cho vai đó.
- Nút **Tự chọn (Claude cân bằng)**: tự đặt cấu hình cân bằng dùng Claude (Opus cho Người viết/Kiến
  trúc, Sonnet cho Điều phối/Biên tập) — tiện nếu bạn không muốn tự chọn.
- Nút **Đóng**: đóng cửa sổ.

💡 Với người mới, thường **không cần đụng tới đây**; cấu hình mặc định đã dùng được.

### 7.7 Cửa sổ "Tiến trình"

Xuất hiện khi bạn chạy **Nhập truyện** hoặc **Phỏng tác**. Nó hiện **nhật ký từng bước** theo thời
gian thực. Khi xong, tiêu đề đổi thành "Hoàn tất" (hoặc "Đã dừng (lỗi)"), và nút **Dừng** đổi thành
**Đóng**. Bấm **Dừng** giữa chừng để hủy công việc.

### 7.8 Cửa sổ "AI cần bạn bổ sung thông tin"

Đôi khi AI cần bạn quyết định (vd chọn thể loại, chọn hướng đi). Một cửa sổ hiện câu hỏi kèm các lựa
chọn (chọn một hoặc nhiều), và ô **"Khác (tự nhập)"** để bạn tự ghi phương án riêng.
- Nút **Bỏ qua (AI tự quyết)**: để AI tự chọn.
- Nút **Gửi câu trả lời**: gửi lựa chọn của bạn.

---

## 8. Hướng dẫn theo tình huống (làm theo từng bước)

### 8.1 Viết cuốn tiểu thuyết đầu tiên (nhanh nhất)

1. Mở giao diện: `ainovel-cli --web` → hoàn tất [Thiết lập](#4-màn-hình-thiết-lập-lần-đầu-từng-ô) nếu
   là lần đầu.
2. Ở màn hình chính, bấm vào **ô nhập yêu cầu** dưới cùng và gõ một câu, ví dụ:
   *"Viết truyện tiên hiệp về chàng trai nghèo tu luyện thành tiên, khoảng 20 chương."*
3. Bấm nút **Bắt đầu** (hoặc nhấn **Ctrl + Enter**).
4. Theo dõi: badge chuyển **ĐANG VIẾT**; nội dung chạy dần ở panel **AI đang viết**; tiến độ tăng ở
   cột trái; dàn ý và nhân vật dần hiện ở cột phải.
5. Cứ để AI chạy. Khi badge chuyển **HOÀN THÀNH** là xong. Sang [mục 8.4](#84-xuất-thành-file-txtepub)
   để xuất file.

### 8.2 Bàn ý tưởng trước khi viết (dùng Cộng tác)

1. Bấm **✧ Cộng tác**.
2. Gõ ý tưởng vào ô **"Ý tưởng ban đầu của bạn"** → **Bắt đầu trò chuyện**.
3. Trao đổi qua lại với AI (gõ ở ô cột trái, nhấn **Enter** để gửi; có thể bấm các **nút gợi ý**).
4. Xem **bản dự thảo chỉ thị** hình thành ở cột phải.
5. Ưng ý thì bấm **Bắt đầu sáng tác**.

### 8.3 Chen ý kiến khi AI đang viết (Can thiệp)

Bạn **không cần dừng** AI. Bất cứ lúc nào đang viết:
1. Gõ ý kiến vào ô nhập, vd *"Thêm một nhân vật phản diện xuất hiện ở chương sau"* hay *"Nhịp hơi
   chậm, đẩy nhanh hơn"*.
2. Bấm **Can thiệp**. AI sẽ tự đánh giá và điều chỉnh (có thể viết lại vài chương bị ảnh hưởng).

### 8.4 Xuất thành file TXT/EPUB

1. Bấm **⬇ Xuất file**.
2. (Tùy chọn) điền **Đường dẫn** lưu, đặt đuôi `.txt` hoặc `.epub`. Có thể chọn **Từ chương / Đến chương**.
3. Bấm **Xuất**. Xem thông báo "✓ Đã xuất N chương → …".

### 8.5 Nhập một truyện có sẵn để viết tiếp

1. Bấm **⇲ Nhập truyện**.
2. Điền **Đường dẫn file truyện** (`.txt`/`.md`), chọn **Bắt đầu từ chương** nếu cần.
3. Bấm **Nhập** → theo dõi cửa sổ **Tiến trình** cho tới khi xong, rồi viết tiếp như bình thường.

### 8.6 Khôi phục sau khi tắt máy / mất điện

Yên tâm — phần mềm tự lưu liên tục.
1. Mở lại giao diện: `ainovel-cli --web` (trong đúng thư mục tác phẩm cũ).
2. Nếu cột trái hiện dòng **"⟲ Có thể khôi phục: …"**, chỉ cần **để trống ô nhập** và bấm nút **Khôi
   phục**. AI viết tiếp từ chỗ dừng.

---

## 9. Phím tắt

| Phím | Tác dụng |
|---|---|
| **Ctrl + Enter** / **Cmd + Enter** | Gửi nội dung ở ô nhập chính (tương đương bấm nút Bắt đầu/Can thiệp/Tiếp tục). |
| **Enter** (trong ô trò chuyện Cộng tác) | Gửi tin nhắn cho AI. |
| **Esc** | Đóng cửa sổ pop-up đang mở (trừ cửa sổ bắt buộc trả lời). |
| Bấm ra vùng tối ngoài cửa sổ | Cũng đóng cửa sổ đó. |

---

## 10. Câu hỏi thường gặp & xử lý sự cố

### 10.1 Tôi có bị tốn nhiều tiền không?

Chi phí phụ thuộc nhà cung cấp, mô hình, và độ dài truyện. Cách kiểm soát:
- Xem mục **Chi phí** trên thanh trên cùng để biết đã tiêu bao nhiêu.
- Bạn có thể đặt **ngân sách** trong file cấu hình (`~/.ainovel/config.json`, mục `budget.book_usd`) để
  phần mềm cảnh báo hoặc dừng khi vượt ngưỡng.
- Dùng mô hình rẻ/nhanh (vd loại `flash`, `mini`) và **mức suy luận** thấp hơn để tiết kiệm.

### 10.2 Đang viết có tắt được không? Có mất dữ liệu không?

Được, và **không mất dữ liệu**. Phần mềm lưu điểm khôi phục liên tục. Muốn dừng tạm: bấm **⏸ Tạm dừng**
hoặc đóng cửa sổ terminal. Mở lại rồi bấm **Khôi phục** (xem [8.6](#86-khôi-phục-sau-khi-tắt-máy--mất-điện)).

### 10.3 Tôi lỡ đóng tab trình duyệt thì sao?

Không sao. Miễn là **cửa sổ terminal vẫn chạy**, chỉ cần mở lại địa chỉ `http://127.0.0.1:...` (in ở
terminal) là giao diện hiện lại đúng trạng thái. Nếu đã lỡ tắt cả terminal, chạy lại `ainovel-cli --web`.

### 10.4 Báo lỗi liên quan API Key / máy chủ

- Kiểm tra lại **API Key** đã dán đúng, còn hiệu lực, và tài khoản còn số dư.
- Kiểm tra **Tên mô hình** đúng chuẩn nhà cung cấp (nhớ quy tắc tiền tố ở [mục 4](#4-màn-hình-thiết-lập-lần-đầu-từng-ô), ô Tên mô hình).
- Muốn sửa cấu hình, mở lại thiết lập hoặc sửa file `~/.ainovel/config.json`.

### 10.5 Muốn đổi mô hình khác giữa chừng

Bấm **⚙ Mô hình** ở thanh trên cùng, đổi trong bảng rồi **Áp dụng** (xem [7.6](#76-cửa-sổ-mô-hình--mức-suy-luận)).

### 10.6 Muốn viết một cuốn khác, độc lập hoàn toàn

Chạy `ainovel-cli --web` trong **một thư mục khác**. Mỗi thư mục quản lý một tác phẩm riêng (dữ liệu
nằm trong thư mục con `output/`).

---

## 11. Các cách dùng khác (nâng cao, tùy chọn)

Ngoài giao diện Web, phần mềm còn hai cách dùng khác — bạn **không bắt buộc** phải dùng:

- **TUI (giao diện trong cửa sổ dòng lệnh):** chạy `ainovel-cli` (không kèm `--web`). Cùng tính năng
  nhưng thao tác bằng bàn phím và các lệnh gạch chéo (`/help`, `/model`, `/export`…). Hợp với người
  quen dùng terminal.
- **Chế độ tự động (headless):** chạy `ainovel-cli --headless --prompt "yêu cầu của bạn"` để AI viết
  hoàn toàn tự động, không giao diện — hữu ích khi chạy trên máy chủ hoặc theo lô.

Muốn hiểu sâu cách phần mềm hoạt động bên trong, xem tài liệu kỹ thuật trong thư mục `docs/` (ví dụ
[`docs/architecture.md`](architecture.md)).

---

*Chúc bạn viết nên những cuốn tiểu thuyết thật hay! Nếu gặp khó khăn, hãy quay lại
[mục 10 — Câu hỏi thường gặp](#10-câu-hỏi-thường-gặp--xử-lý-sự-cố).*

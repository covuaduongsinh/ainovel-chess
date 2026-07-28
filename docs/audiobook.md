# Sách nói — đọc bản thảo bằng giọng Vbee

Khả năng ngang `internal/host/tts` biến một dự án sách **đã viết xong chương** thành
**sách nói**: mỗi chương một tệp MP3, kèm danh sách phát và mục lục. Đối xứng với
`imp` (nhập) / `sim` (phỏng tác) / `exp` (xuất) / `adapt` (video): tác vụ **nhiều bước,
chỉ đọc** dữ liệu trong store rồi ghi file ra ngoài. Vào qua lệnh **`/sachnoi`** (TUI) và
nút **🎧 Sách nói** (Web); cả hai gọi `host.Host.Audiobook(ctx, tts.Options)` và dùng
chung `tts.Options` để mirror nhau.

Khác `adapt` ở một điểm bản lề: **không gọi LLM**. Chi phí nằm ở tín dụng Vbee, tính theo
**số ký tự** gửi đi, nên không đi qua `UsageTracker`/`Budget` của sách.

## ⚠ Chi phí — đọc trước khi chạy

Vbee tính phí theo **số ký tự**, không theo phút audio. Đo trên dự án thật trong repo:
khoảng **18.000–20.000 ký tự mỗi chương**, tức **~600.000 ký tự cho một cuốn 32 chương**,
chưa nhân `credit_factor` của giọng (giọng cao cấp có hệ số > 1).

Thói quen an toàn:

1. Bấm **Kiểm tra kết nối** trong hộp thoại Sách nói trước. Nút này gọi chế độ `sync` với
   một câu ~30 ký tự — xác thực được thông tin đăng nhập mà gần như không mất tín dụng, và
   **không cần** `webhook_url`.
2. Chạy thử **một chương**: `/sachnoi to=1` (TUI) hoặc đặt *Đến chương = 1* (Web).
3. Đối chiếu số tín dụng bị trừ với dòng **Tổng ký tự đã gửi Vbee** trong `audio/index.md`.
4. Rồi mới chạy cả sách.

Hộp thoại Web hiện sẵn ước tính ký tự cho phạm vi đang chọn, và hỏi xác nhận khi phạm vi
vượt 5 chương.

## Yêu cầu

Một tài khoản Vbee có **App ID** và **Access Token**, tạo tại
[studio.vbee.vn/apps](https://studio.vbee.vn/apps) → mục *Tích hợp API*. Token có thể đặt
hạn 7/30/60/90 ngày hoặc vĩnh viễn; hết hạn thì phải tạo ứng dụng mới.

Cấu hình bằng một trong hai cách:

- **Hộp thoại Web** — mở 🎧 Sách nói → *Thông tin Vbee* → nhập → **Lưu**.
- **Tệp cấu hình** — mục `vbee` trong `~/.ainovel/config.json`, xem
  [config.example.jsonc](../config.example.jsonc).

TUI **không** có form nhập thông tin xác thực — để bí mật nằm trong scrollback terminal là
ý tồi. Chưa cấu hình thì `/sachnoi` báo lỗi kèm hướng dẫn.

> Lưu ý: bấm **Lưu** sẽ ghi lại toàn bộ `~/.ainovel/config.json`, làm mất các dòng chú
> thích `//` trong tệp. Đây là hành vi có sẵn của `/model` (`SwitchModel`), không phải mới.

## Đầu ra

```
{novelDir}/audio/
  tap-01/
    chuong-001.mp3
    chuong-002.mp3
    chuong-003-p1.mp3      # chỉ khi chương vượt ngưỡng chia phần
    chuong-003-p2.mp3
    playlist.m3u           # danh sách phát riêng của tập
  tap-02/
    ...
  playlist.m3u             # danh sách phát cả sách
  index.md                 # mục lục + tổng ký tự đã gửi Vbee
```

Gom tập theo đúng ngữ nghĩa của `adapt`: `tap-%02d` dựng từ `layered_outline.json`; sách
không phân tập thì tất cả nằm trong `tap-01`.

`playlist.m3u` là Extended M3U (UTF-8 không BOM). Thời lượng ghi `-1` vì công cụ **không**
phân tích khung MP3 và **không** dùng ffmpeg — mọi trình phát phổ thông đều chấp nhận.
Đường dẫn trong danh sách phát luôn dùng dấu `/` để còn mở được trên máy khác.

Cả `playlist.m3u` và `index.md` **luôn được ghi đè** ở cuối mỗi lần chạy: chúng là bản tóm
tắt dẫn xuất, không phải sản phẩm tốn tiền. Cờ `Overwrite` chỉ bảo vệ tệp âm thanh.

## Quy trình

```
đọc store → làm sạch Markdown → chia phần → GỬI → CHỜ (poll) → TẢI → ghi tệp → mục lục
```

Vbee có hai chế độ: `sync` (trả thẳng byte audio, tối đa **300 ký tự**) và `async` (trả
`requestId`, tối đa **100.000 ký tự**). Chương truyện dài hơn 300 ký tự rất nhiều nên phải
dùng `async`.

**Vì sao poll thay vì dùng webhook.** Tài liệu Vbee ghi `webhookUrl` là bắt buộc với chế
độ `async`, nhưng ứng dụng chạy trên `localhost` và không có URL công khai để nhận callback.
Vì thế ta gửi một **URL giữ chỗ** (`vbee.webhook_url`, mặc định
`https://example.com/vbee-callback`) rồi **chủ động hỏi** `GET /v1/tts/requests/{id}` cho
tới khi xong. Nếu tài khoản của bạn từ chối URL giữ chỗ, hãy điền một URL nhận-và-bỏ
(ví dụ `https://webhook.site/<uuid>`) vào `vbee.webhook_url`.

Nhịp chờ:

| Mốc | Giá trị |
|---|---|
| Chờ trước lần hỏi đầu | 5 giây |
| Nhịp hỏi | 3 giây, nhân 1,5 mỗi lần, chặn ở 20 giây |
| Hạn cho một phần | 15 phút (quá thì bỏ chương, ghi kèm `requestId` để tra ở bảng điều khiển Vbee) |
| Thử lại khi gửi / tải hỏng | 3 lần, lùi 2s → 4s |
| Số lỗi hỏi trạng thái liên tiếp tối đa | 5 |

**`audioLink` chỉ sống 3 phút** kể từ lúc trạng thái chuyển `COMPLETED` (tệp vẫn nằm trên
máy chủ Vbee 3 ngày). Nên: thấy `COMPLETED` là tải ngay, và mỗi lần tải hỏng thì **xin
đường dẫn mới** qua `Status` rồi thử lại, chứ không thử lại đường dẫn đã chết. Việc tải
dùng một `http.Client` riêng với hạn 10 phút — một chương 25 phút audio nặng khoảng 24 MB ở
128 kbps, client API 60 giây sẽ không kịp.

**Chạy tuần tự, không song song.** Vbee tính tiền theo ký tự: mỗi lỗi đồng thời là tiền
thật, và hủy giữa chừng vẫn bị tính mọi yêu cầu đang bay. Ngoài ra `Event.Current/Total` và
nhật ký trên giao diện đều giả định thứ tự đơn điệu.

## Làm sạch văn bản

Nguồn là bản thảo cuối `chapters/{NN}.md`. Nếu gửi thẳng, Vbee sẽ đọc cả dấu sao và dấu
thăng, nên trước khi gửi:

- Gỡ tiêu đề `#`, đậm `**`, nghiêng `*`/`_`, mã `` ` ``, gạch ngang `~~`, trích dẫn `>`,
  gạch đầu dòng, liên kết (giữ phần chữ, bỏ URL), ảnh, thẻ HTML, khối mã.
- Bỏ emoji và ký hiệu trang trí, **nhưng giữ dấu gạch ngang `—`** — đó là ký hiệu mở lời
  thoại xuyên suốt bản thảo tiếng Việt, và Vbee đọc nó thành một nhịp nghỉ.
- Đường kẻ ngang `---` thành ranh giới đoạn; đặt `SceneBreakText` nếu muốn đọc thành một
  câu chuyển cảnh.
- Gom khoảng trắng và dòng trống thừa.

**Lời dẫn đầu chương.** Phần lớn tệp chương trong repo vào thẳng văn xuôi, không có dòng
tiêu đề nào — nghe sẽ không biết đang ở chương mấy. Vì thế mỗi tệp âm thanh luôn mở đầu
bằng `Chương N. Tên chương.`, lấy tên theo thứ tự: tên đọc được từ chính tệp chương (ba
dạng `# Chương 1: X`, `## Chương 3: X`, `Chương 1: X` trần đều nhận ra), rồi tới tiêu đề
trong dàn ý.

**Không chuẩn hóa số và số La Mã** — đây là điều cố ý. Vbee đã tự bung số Ả Rập, ngày
tháng và tiền tệ; bung thêm sẽ đọc đôi. Còn normalizer số La Mã thì sẽ phá chữ "I", "V",
"X" đứng một mình trong lời thoại và tên riêng.

**Chia phần.** Chương vượt `MaxChars` (mặc định 90.000 rune, dưới giới hạn 100.000 của
Vbee một quãng đệm) sẽ được chia: ưu tiên ranh giới đoạn văn, rồi ranh giới câu (không cắt
giữa cặp nháy `“ ”`), cuối cùng mới cắt cứng ở khoảng trắng. Đếm theo **rune** chứ không
theo byte — dấu tiếng Việt chiếm 2–3 byte nên đếm byte sẽ chia sớm gấp ~2,5 lần. Việc chia
là tất định theo văn bản, nhờ vậy chạy lại vẫn sinh đúng tên phần cũ và resume đúng chỗ.
Thực tế chương chỉ khoảng 20.000 rune nên gần như không bao giờ phải chia.

## Tham số (`tts.Options`)

| Trường | Ý nghĩa |
|---|---|
| `From`, `To` | Phạm vi chương; `0/0` = toàn bộ chương đã hoàn thành |
| `VoiceCode` | Mã giọng, vd `hn_female_ngochuyen_full_48k-fhg`; rỗng = lấy `vbee.voice_code` |
| `Speed` | 0,25–1,9; rỗng = 1,0 |
| `Bitrate` | 8/16/32/64/128 kbps; rỗng = 128 |
| `SampleRate` | 8000/16000/22050/24000/32000/44100/48000 Hz; rỗng = 24000 |
| `OutputFormat` | `mp3` (mặc định) hoặc `wav` (không nén, một chương dài có thể vài trăm MB) |
| `Pause` | Tinh chỉnh khoảng nghỉ ở dấu chấm/phẩy/chấm phẩy/xuống dòng (giây) |
| `WebhookURL` | Rỗng = `vbee.webhook_url` rồi tới URL giữ chỗ mặc định |
| `MaxChars` | Ngưỡng chia phần theo rune; rỗng = 90.000 |
| `SceneBreakText` | Câu đọc thay cho `---`; rỗng = chỉ ngắt đoạn |
| `OutDir` | Rỗng = `{novelDir}/audio/` |
| `Overwrite` | `false` (mặc định) = bỏ qua tệp đã có → chạy tiếp phần còn thiếu |
| `VoiceMap` | **Chưa dùng** — chỗ dành sẵn cho giọng-theo-nhân-vật sau này |

## Hành vi khi lỗi

**Dừng cả lượt chạy:**

| Tình huống | Lý do |
|---|---|
| Thiếu `app_id` / `access_token` / giọng đọc | Báo lỗi ngay, chưa gửi yêu cầu nào |
| Không có chương nào hoàn thành trong phạm vi | Không có gì để đọc |
| Vbee trả `UNAUTHORIZED` bất cứ lúc nào | Sai thông tin xác thực thì mọi chương sau cũng hỏng y hệt |
| Vbee trả `BAD_REQUEST` **khi chưa chương nào thành công** | Lỗi hình dạng yêu cầu (`webhookUrl` bị từ chối, `voiceCode` lạ, `speed` ngoài khoảng) là lỗi đồng nhất — lộ ra ngay ở chương đầu thay vì phí cả cuốn |
| Ghi tệp thất bại | Lỗi cục bộ, mọi chương sau cũng hỏng |
| Người dùng bấm Dừng / Esc | Hủy ngữ cảnh |

**Bỏ qua chương rồi đi tiếp** (fail-soft, giống `adapt`): chương thiếu hoặc rỗng sau khi
làm sạch; Vbee trả `FAILED`; quá hạn 15 phút; mất liên lạc khi hỏi trạng thái; tải hỏng hết
lượt thử; `BAD_REQUEST` **sau khi** đã có ít nhất một chương thành công.

Kết thúc: còn ít nhất một chương thành công → `done`, thông báo nêu rõ những chương bị bỏ
qua. Không chương nào thành công → `error`, để giao diện không hiện màu xanh "Hoàn thành"
giả. Chương lỗi giữa chừng **không** để lại nửa bộ tệp: các phần chỉ được ghi sau khi cả
chương tải xong, và mọi lần ghi đều nguyên tử (tmp + fsync + rename).

## Cách dùng

**TUI**

```
/sachnoi                                   # cả sách, giọng mặc định trong config
/sachnoi to=1                              # CHẠY THỬ MỘT CHƯƠNG TRƯỚC
/sachnoi from=5 to=12 speed=1.1
/sachnoi voice=sg_male_trungkien_vdts_48k-fhg format=mp3 bitrate=128
/sachnoi out=D:\sachnoi --overwrite
```
Alias: `/audiobook`, `/tts`. Esc dừng khi đang chạy, đóng panel khi đã xong.

**Web** — nút 🎧 Sách nói trên thanh lệnh.

Ô **Mã giọng đọc** là thứ quyết định: giá trị trong đó chính là `voiceCode` được gửi đi,
và luôn dán tay được. Danh sách bên dưới chỉ là công cụ tra cứu — chọn một dòng thì nó
điền vào ô đó. Nhờ vậy vẫn dùng được một mã giọng không thuộc nhóm đang xem, và sự cố ở
API danh sách giọng không chặn việc tạo sách nói.

Kho giọng chia làm ba nhóm, phải gọi **riêng từng nhóm** (xem mục dưới). Một tài khoản
thực tế có khoảng 462 giọng Vbee + 1187 giọng cộng đồng + vài giọng cá nhân, nên có ô lọc
theo tên/mã/mã ngôn ngữ; danh sách hiện tối đa 300 dòng mỗi lần và báo rõ còn bao nhiêu.
Nhãn mỗi dòng gồm ngôn ngữ, tên, giới tính, hệ số tín dụng và **mã** — nhiều giọng trùng
tên hiển thị mà chỉ khác mã (kiểu đọc, tần số lấy mẫu).

**HTTP**

```
POST /api/audiobook       {from,to,voice,speed,bitrate,sampleRate,outputFormat,outDir,overwrite}
POST /api/job/cancel      {text: "<job id>"}
GET  /api/vbee/config     GET/POST cấu hình (app_id và token luôn trả về dạng che)
GET  /api/vbee/voices     ?ownership=VBEE|COMMUNITY|PERSONAL&lang=&gender=&limit=
POST /api/vbee/preview    {voice,text} → trả thẳng audio/mpeg
```

Quy ước với `POST /api/vbee/config`: máy chủ **bỏ qua** mọi `appId`/`accessToken` chứa
`****` (chính là giá trị đã che mà trình duyệt vừa hiển thị) và giữ nguyên bí mật đang lưu;
gõ giá trị mới thì thay; gõ đúng ký tự `-` thì xóa hẳn.

`ownership` của `/api/vbee/voices` là tham số **bắt buộc phía Vbee** — thiếu nó API trả
`400` chứ không trả về mọi nhóm, nên handler tự điền `VBEE` khi để trống. Muốn lấy đủ kho
giọng thì phải gọi ba lần, mỗi lần một nhóm. Bỏ `lang` để thấy hết các thứ tiếng: lọc
`lang=vi-VN` cắt nhóm Vbee từ 462 xuống 25 giọng.

## Giới hạn đã biết

- **Một giọng duy nhất** cho cả sách. `Options.VoiceMap` đã có sẵn trong kiểu dữ liệu để
  thêm giọng-theo-nhân-vật sau mà không phải đổi hình dạng `Options`, nhưng runner chưa đọc.
- **Không ghép tệp**, không đo thời lượng, không sinh chương mục — công cụ không phụ thuộc
  ffmpeg. Nếu cần một tệp duy nhất, hãy ghép ngoài bằng ffmpeg.
- **Không chặn chạy trùng.** `guardExclusive` không đặt cờ nên hai lượt sách nói song song
  sẽ tiêu tiền gấp đôi — kế thừa từ `Adapt`.
- **`webhook_url` là URL giữ chỗ.** Xem mục Quy trình; nếu tài khoản từ chối, đổi sang URL
  nhận-và-bỏ.
- **API danh sách giọng nằm ở host khác** endpoint TTS (`vbee.vn` vs `api.vbee.vn`). Token
  có thể bị giới hạn theo host và 401 ở đây trong khi việc tạo sách nói vẫn chạy bình thường.
- **Cách đếm ký tự của Vbee có thể khác** cách đếm rune (họ có thể đếm sau khi chuẩn hóa
  số). `MaxChars` mặc định chừa sẵn ~11% biên.

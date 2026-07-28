# Nhật ký thay đổi

Mọi thay đổi đáng chú ý của dự án được ghi ở đây. Định dạng theo tinh thần
[Keep a Changelog](https://keepachangelog.com/vi/1.0.0/).

Nhật ký này tập trung vào các thay đổi của **bản Việt hóa** so với upstream
`github.com/voocel/ainovel-cli`.

## [Chưa phát hành]

### Đã thêm
- **Truyện tranh** — khả năng ngang mới [internal/host/comic](internal/host/comic/) dựng
  **trang truyện tranh hoàn chỉnh, xuất bản được** từ các chương đã hoàn thành, ghi vào
  `{novelDir}/truyen-tranh/`. Tài liệu: [docs/comic.md](docs/comic.md).
  - **Điểm vào mirror nhau**: lệnh `/truyentranh` (alias `/comic`) trên TUI và nút
    **▤ Truyện tranh** trên Web, cả hai gọi `host.Host.Comic(ctx, comic.Options)`.
  - **Gói dựng ảnh** [internal/comicdraw](internal/comicdraw/) — gói lá thuần đồ hoạ, dựng
    trang ra **PNG lẫn SVG** từ cùng một mô tả nên hai đầu ra không thể lệch nhau. Bản SVG
    giữ chữ ở dạng `<text>` nên tút lại bằng Illustrator/Inkscape được, và dịch sang ngôn
    ngữ khác chỉ cần thay chữ chứ không phải vẽ lại tranh.
  - **LLM ra ngữ nghĩa, Go ra hình học**: LLM chỉ quyết định số khung, cỡ khung và chỗ xuống
    hàng; toạ độ do Go tính xác định. Để LLM sinh toạ độ thì khung chồng nhau và hở lỗ — lỗi
    chỉ lộ ra khi nhìn trang đã in. Nay "lát kín trang, không chồng lấn" là **bất biến có test**.
  - **Chỉ ba bước gọi LLM** (định hướng mỹ thuật · model sheet nhân vật · kịch bản từng
    chương); bố cục, prompt khung, dàn trang và đóng gói đều thuần Go, không tốn token.
  - **Tái dùng di sản `/video`** khi có (`consistency-bible`, model sheet, storyboard) thay
    vì sinh lại — vừa rẻ hơn vừa giữ bản truyện tranh cùng thế giới hình ảnh với bản video.
  - **Đóng gói xuất bản viết tay, không thêm thư viện**: PDF 1.7 in ấn (MediaBox/BleedBox/
    TrimBox, JPEG qua `/DCTDecode`, tiêu đề UTF-16BE), CBZ (zip Store + `ComicInfo.xml`),
    EPUB3 fixed-layout (`rendition:layout pre-paginated`, ảnh bọc `<svg>`).
  - **Chữ tiếng Việt**: bắt buộc chuẩn hoá NFC vì `x/image/font/sfnt` không có GSUB lẫn
    mark-to-base — chuỗi dạng tổ hợp sẽ vẽ dấu chồng đống ở gốc bút; và căn giữa theo **đỉnh
    mực thật** chứ không theo ascent typographic (đo trên Patrick Hand 64px: ascent −66,7
    nhưng mực chữ thường −30, căn theo ascent đẩy chữ xuống ~36px).
  - **Font nhúng** ba bộ SIL OFL 1.1 đã kiểm chứng phủ đủ 165 glyph tiếng Việt: Patrick Hand,
    Bangers, Be Vietnam Pro. ⚠ Comic Neue tuy OFL nhưng **thiếu hẳn khối U+1EA0–1EF9**.
  - **Giai đoạn 2 — sinh ảnh thật qua Gemini.** Client HTTP thuần ở
    [internal/imggen](internal/imggen/) (stdlib, test bằng `httptest`), tách **phương ngữ dây**
    khỏi **chính sách** vì Google đang có hai bề mặt API sinh ảnh song song và đã gắn nhãn
    đường cũ là "Legacy" mà chưa công bố ngày khai tử — đổi được bằng cấu hình `comic.dialect`.
    Có nút **"Kiểm tra kết nối (1 ảnh)"** bắt buộc dùng trước khi chạy cả sách. Khoá để trống
    thì tự mượn `providers["gemini"].api_key`. 429 liên tiếp 5 lần thì tự nâng thành lỗi chí
    tử (429 vừa nghĩa "quá nhanh" vừa nghĩa "cạn hạn mức", tầng transport không phân biệt được).
  - **Hai bẫy prompt chỉ lộ ra khi nhìn tranh thật**: (a) câu *"chừa chỗ … cho bong bóng
    thoại"* khiến mô hình **vẽ hẳn một bong bóng rỗng** — nhắc tên thứ mình không muốn vẽ là
    hỏng, kể cả khi đã cấm trong negative; (b) token `comic panel` khiến mô hình **tự vẽ khung
    viền** bên trong ảnh, thành khung lồng khung sau khi dàn trang. Đã sửa cả hai.
  - **Chia hai giai đoạn.** Giai đoạn 1 chạy trọn đường ống với **ảnh giữ chỗ**,
    **không tốn một xu tiền sinh ảnh** — đủ để nghiệm thu bố cục và typography trước. Giai
    đoạn 2 nối API sinh ảnh qua interface `ImageSource` đã định sẵn. Vì bộ dựng đọc ảnh
    **theo đường dẫn tệp**, bạn có thể tự vẽ hoặc tự chạy bộ sinh ảnh rồi thả tệp vào
    `art/` là trang có tranh ngay, không cần sửa dòng code nào.
  - **⚠ Chi phí giai đoạn 2**: muốn **in** thì phải sinh ảnh ở **2K** chứ không phải 1K
    (A4 @300 DPI = 2480×3508 px), gần như gấp đôi chi phí. Một cuốn 32 chương ≈ 1.900 khung
    ≈ $74–192 tuỳ model. Có `maximages` làm cầu dao và ước tính hiện trên hộp thoại Web.
  - `golang.org/x/image` chuyển từ dependency gián tiếp sang trực tiếp (`go.sum` không đổi).
- **Sách nói qua Vbee TTS** — khả năng ngang mới [internal/host/tts](internal/host/tts/)
  đọc các chương đã hoàn thành thành tệp MP3, **mỗi chương một tệp**, kèm `playlist.m3u`
  (cả sách + từng tập) và `index.md`, ghi vào `{novelDir}/audio/`. Tài liệu:
  [docs/audiobook.md](docs/audiobook.md).
  - **Điểm vào mirror nhau**: lệnh `/sachnoi` (alias `/audiobook`, `/tts`) trên TUI và nút
    **🎧 Sách nói** trên Web, cả hai gọi `host.Host.Audiobook(ctx, tts.Options)`.
  - **Client Vbee** ở [internal/vbee](internal/vbee/) — gói HTTP thuần, không phụ thuộc gì
    trong repo, test bằng `httptest.Server`.
  - **⚠ Vbee tính phí theo SỐ KÝ TỰ**, không theo phút audio: một cuốn 32 chương ≈ 600.000
    ký tự. Hộp thoại Web hiện ước tính ký tự và hỏi xác nhận khi vượt 5 chương; nút "Kiểm
    tra kết nối" dùng chế độ `sync` ~30 ký tự để thử thông tin đăng nhập gần như miễn phí.
    Hãy chạy `/sachnoi to=1` trước khi tạo cả sách.
  - **Chờ bằng poll, không dùng webhook**: tài liệu Vbee ghi `webhookUrl` là bắt buộc với
    chế độ `async`, nhưng ứng dụng chạy localhost nên không có URL công khai — ta gửi URL
    giữ chỗ (cấu hình được qua `vbee.webhook_url`) rồi chủ động hỏi
    `GET /v1/tts/requests/{id}`. `audioLink` chỉ sống 3 phút nên tải ngay khi thấy
    `COMPLETED`, và mỗi lần tải hỏng thì xin đường dẫn mới thay vì thử lại link đã chết.
    Việc tải dùng `http.Client` riêng (hạn 10 phút) vì một chương ≈ 24 MB.
  - **Làm sạch Markdown trước khi đọc**: gỡ `#`/`**`/`*`/`` ` ``/`>`/liên kết/ảnh/emoji,
    **giữ dấu `—`** vì đó là ký hiệu mở lời thoại. Mọi tệp mở đầu bằng lời dẫn
    `Chương N. Tên chương.` — phần lớn tệp chương trong repo vào thẳng văn xuôi nên nghe sẽ
    không biết đang ở chương mấy. Cố ý **không** chuẩn hóa số/số La Mã (Vbee đã tự bung số,
    bung thêm sẽ đọc đôi).
  - **Chạy tuần tự** chứ không song song: API tính tiền theo ký tự nên mỗi lỗi đồng thời là
    tiền thật. **Fail-soft** như `adapt`: chương lỗi thì bỏ qua và đi tiếp, nhưng
    `UNAUTHORIZED` — hoặc `BAD_REQUEST` khi chưa chương nào thành công — thì dừng ngay để
    lỗi cấu hình lộ ra ở chương đầu. Tất cả chương đều hỏng thì kết thúc bằng `error` chứ
    không phải "Hoàn thành" giả. **Resume incremental**: chạy lại không `--overwrite` chỉ
    làm phần còn thiếu.
  - **Chọn giọng**: ô "Mã giọng đọc" luôn dán tay được và là thứ quyết định; danh sách chỉ
    là công cụ tra cứu điền vào ô đó. `voiceOwnership` là tham số **bắt buộc** phía Vbee
    (thiếu là `400`, không phải trả mọi nhóm) nên phải gọi riêng từng nhóm `VBEE` /
    `COMMUNITY` / `PERSONAL` — một tài khoản thật có ~462 + 1187 + 2 giọng. Kèm bộ chọn
    nhóm, ô lọc theo tên/mã/ngôn ngữ, và nhãn có cả mã vì nhiều giọng trùng tên hiển thị.
  - **Cấu hình**: mục `vbee` mới trong `~/.ainovel/config.json` (app_id, access_token,
    voice_code, webhook_url, speed, bitrate, sample_rate, output_format), nhập được ngay
    trong hộp thoại Web — token hiển thị dạng che `abcd****wxyz`, gửi lại nguyên chuỗi che
    thì giữ bí mật cũ, gõ `-` để xóa. Lưu ý **lưu sẽ ghi lại toàn bộ `config.json` và làm
    mất các dòng chú thích `//`** — hành vi có sẵn của `/model`, không phải mới.
  - TUI **không** có form nhập thông tin xác thực (cố ý: bí mật không nên nằm trong
    scrollback terminal) — chưa cấu hình thì `/sachnoi` báo lỗi kèm hướng dẫn.
- **Thiết kế lại màn chọn dự án (Web)** — [internal/entry/web/static/projects.html](internal/entry/web/static/projects.html)
  tách thành `projects.html` + `projects.css` + `projects.js` (mirror cấu trúc
  `index.html`/`styles.css`/`app.js` của workbench), phục vụ qua 2 route tĩnh mới trong
  [picker.go](internal/entry/web/picker.go):
  - **Sửa lỗi không cuộn được**: trang kế thừa `body { overflow: hidden }` của `styles.css`
    nhưng lại đặt `justify-content: center`, nên danh sách dài bị cắt cụt ở cả hai đầu và ô
    "Tạo dự án mới" nằm dưới đáy phải zoom out mới thấy (chỉ xảy ra khi cửa sổ > 1100px).
    Bố cục mới: **header cố định** (tạo dự án + lọc + sắp xếp + tab) `flex: none`, danh sách
    là **lưới tự cuộn** `flex: 1; min-height: 0; overflow-y: auto`, 2–3 cột theo bề rộng.
  - **Thẻ dự án** thay cho dòng danh sách: badge trạng thái **tiếng Việt** có màu (Khởi tạo /
    Ý tưởng / Dàn ý / Đang viết / Hoàn thành / Chưa bắt đầu), thanh tiến độ chương, số chữ,
    thời điểm sửa cuối; bấm cả thẻ để mở.
  - **Ô lọc** theo tên/slug (gõ **không dấu** vẫn khớp tên có dấu) và **sắp xếp** (Sửa gần
    nhất / Tên A→Z / Nhiều chữ nhất / Tiến độ cao nhất) — thuần phía trình duyệt.
  - **Đổi tên · Lưu trữ · Xóa** qua menu ⋯ trên thẻ, kèm 4 route API mới
    (`/api/projects/rename|archive|unarchive|delete`) và các hàm store tương ứng
    (`store.Rename` / `Archive` / `Unarchive` / `Delete` / `ListArchived`).
    Đổi tên cập nhật **cả** `novel_name` trong `meta/progress.json` **lẫn** tên thư mục.
    Lưu trữ = chuyển thư mục vào `output/_archive/` (đảo ngược được, `List` bỏ qua thư mục
    này). Xóa là vĩnh viễn nên có hai lớp hàng rào: hộp thoại buộc **gõ đúng tên dự án**, và
    máy chủ kiểm tra lại chuỗi xác nhận + chỉ chấp nhận thư mục thật sự là dự án sách
    (chặn xóa nhầm chính `_archive` hay thư mục lạ), cộng với `resolveUnderRoot` sẵn có.
    Các route này chỉ tồn tại trên picker mux — tức chỉ khi **không** có dự án nào đang mở.
- **`/video` gói theo chương + đóng gói lồng theo tập** — cải tổ thứ tự xử lý của
  `internal/host/adapt`:
  - Thêm `Options.Grouping`: **`chapter` (mặc định mới)** chạy phần cấp-sách
    (concept/character/prop/consistency) một lần, rồi duyệt **tập → chương**, mỗi chương làm
    trọn kịch bản → phân cảnh → animation → prompt **trước khi** sang chương kế → **tập
    trước hoàn chỉnh trước**, dựng video từng tập được ngay (trước đây quét theo loại trên
    toàn bộ chương nên phải chờ gần hết pipeline). Giữ `product` làm chế độ tương thích cũ.
  - **Đóng gói lồng** (chỉ chế độ `chapter`): mỗi chương một thư mục
    `video/tap-{VV}/chuong-{NNN}/{kich-ban.md, phan-canh.json/.md, animation.md,
    prompt-anh.md, prompt-video.md}` + mục lục tập `_tap-{VV}.md`. Render thuần, không gọi
    thêm LLM. **Bố cục theo loại cũ** (`video/screenplay/`…) vẫn giữ nguyên.
  - TUI: `/video ... group=chapter|product`; Web: ô "Cách đóng gói" trong modal Làm video.
  - Tài liệu cập nhật: [docs/video.md](docs/video.md).
- **Chuyển thể sách → sản phẩm làm video** — khả năng ngang mới `internal/host/adapt`
  (đối xứng `imp`/`sim`/`exp`), lệnh **`/video`** (TUI) và **🎬 Làm video** (Web):
  - Từ các chương đã hoàn thành, sinh 9 loại sản phẩm phục vụ dựng video. **6 loại dùng
    LLM**: `concept` (art direction), `character` (thiết kế nhân vật), `prop` (đạo cụ),
    `consistency` (bảng nhất quán khóa token trực quan), `screenplay` (kịch bản),
    `storyboard` (phân cảnh). **3 loại render thuần** (không tốn LLM, tổng hợp từ
    storyboard): `animation`, `imageprompt`, `videoprompt`.
  - Chạy `all` theo thứ tự `concept → character → prop → consistency → screenplay →
    storyboard → animation → imageprompt → videoprompt`; các bước hình ảnh chạy trước tạo
    "style bible", storyboard tiêm token chuẩn để nhân vật/đạo cụ nhất quán xuyên suốt.
  - Prompt sinh ảnh/video **trung lập, giàu chi tiết**, **song ngữ** (prompt EN + mô tả VI).
  - Chỉ đọc store (chương/dàn ý/nhân vật/thế giới), ghi nguyên tử vào `{novelDir}/video/`;
    `guardExclusive` để không chạy chồng Coordinator; fail-soft theo từng chương; resume
    incremental (bỏ qua file đã có nếu không `--overwrite`). Model dùng vai `architect`.
  - TUI: `/video [product...|all] [from=N] [to=M] [style=...] [--overwrite]`; Web: `POST
    /api/adapt` qua jobRegistry + SSE, mirror nhau qua `adapt.Options`.
  - Tài liệu: [docs/video.md](docs/video.md).
- **Xuất bản thảo đa định dạng** — mở rộng khả năng `/export`:
  - Thêm định dạng **Markdown (`.md`)** (`exp.FormatMD`, `renderMD`) bên cạnh TXT và EPUB —
    tiêu đề ATX `#`/`##`, dễ đọc và convert tiếp.
  - **Mặc định xuất cùng lúc 3 file** `.md` + `.txt` + `.epub` (`exp.DefaultFormats()`), thay
    cho mặc định chỉ TXT trước đây.
  - Đường dẫn được hiểu là **base**: không đuôi → 3 định dạng; có đuôi nhận biết
    (`.md`/`.txt`/`.epub`) → đúng 1 định dạng. Web và TUI dùng chung `exp.FormatsForPath()` để
    hành vi mirror nhau. TUI: `/export [base] [from=N] [to=M] [--overwrite]`; Web: ô đường dẫn
    điền sẵn `exportBase` từ `GET /api/meta`.
  - Tài liệu: [docs/export.md](docs/export.md).
- **Tích hợp Claude Code** — dùng chính bộ model Claude (Opus 4.8/4.7, Sonnet 4.6,
  Haiku 4.5) để viết truyện:
  - Provider `claude-code` (type `anthropic`) với danh mục 4 model dựng sẵn.
  - Hai đường xác thực: cầu nối **Meridian** ở `http://127.0.0.1:3456` (thuê bao qua
    Agent SDK) hoặc **API key trực tiếp** `https://api.anthropic.com`.
  - **Preset "cân bằng"** (`bootstrap.BalancedClaudeRoles()`): Writer/Architect →
    `claude-opus-4-8` (high), Coordinator/Editor → `claude-sonnet-4-6` (medium).
  - Áp preset qua wizard, lệnh TUI **`/model auto`**, hoặc nút Web
    **"Tự chọn (Claude cân bằng)"** (`POST /api/model/auto`).
  - Tài liệu: [docs/claude-code.md](docs/claude-code.md).
- **Tài liệu dự án**: `CLAUDE.md` (định hướng cho AI agent/contributor),
  `docs/claude-code.md`, và `CHANGELOG.md` (tệp này).

### Đã thay đổi (breaking)
- **`POST /api/export`** đổi shape phản hồi: bỏ `path`/`bytes`, thay bằng mảng
  `files: [{ format, path, bytes }]` (kèm `chapters`, `skipped`) để đại diện cho nhiều file
  đầu ra. Client cũ đọc `r.path` cần chuyển sang đọc `r.files`.

### Đã sửa
- **`draft_chapter`** — tham số `mode` chuyển từ **bắt buộc** sang **tùy chọn** (bỏ
  `StrictSchema`). Trước đây `mode` bị đánh `required` để tương thích strict tool calling
  của OpenAI, nhưng Gemini không hỗ trợ strict và thỉnh thoảng gọi tool không kèm `mode`
  → agentcore từ chối với "required parameter `mode` is missing" trước cả khi `Execute`
  chạy, đốt lượt của Writer. `Execute` vốn coi `mode` rỗng là `write`, nên để tùy chọn
  vừa khớp hành vi vừa hết lỗi. Xem [internal/tools/draft_chapter.go](internal/tools/draft_chapter.go).

## Việt hóa (đã phát hành trên nhánh `main`)

Việt hóa toàn bộ engine, tài liệu và tài sản, chia thành các giai đoạn:

- **GĐ 1a** — Việt hóa lõi engine (prompt gắn liền với hằng số Go). (`9acfb0d`)
- **GĐ 1b** — Việt hóa toàn bộ `assets/references` và `assets/styles`. (`2f51094`)
- **GĐ 2–4** — Việt hóa chuỗi runtime, test fixtures, config, docs, scripts, evals. (`7f2f3bc`)
- **GĐ 5** — Rà soát cuối: dịch `host/`, `tools/`, docs và mọi tệp còn sót. (`e74545a`)

## Giao diện Web

- Thêm **cổng vào thứ ba** `internal/entry/web` — bàn làm việc trong trình duyệt
  (`ainovel-cli --web`), chỉ localhost `127.0.0.1`, giao diện tiếng Việt, đẩy sự kiện qua
  SSE, tính năng tương đương TUI (start/steer/pause/continue, export, đổi model & cường độ
  suy luận, cùng tạo quy hoạch, import, simulate, `/diag`). Bên dưới tái sử dụng cùng engine
  `host.Host`, không thay đổi logic sáng tác. Hướng dẫn: [docs/huong-dan-su-dung.md](docs/huong-dan-su-dung.md).

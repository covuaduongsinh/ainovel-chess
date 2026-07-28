# Làm truyện tranh: sách → trang truyện tranh xuất bản được

Khác với [/video](video.md) chỉ sinh **nhiên liệu** (kịch bản, prompt) cho người khác dựng
video, `/truyentranh` dựng ra **sản phẩm cuối**: trang truyện tranh đã ghép tranh, đã vẽ bong
bóng và lồng chữ tiếng Việt, đã đóng gói in được.

## Chạy

```bash
# TUI
/truyentranh                       # chạy tất cả các bước
/truyentranh to=1                  # ⚠ chạy thử MỘT chương trước
/truyentranh preset=manga size=b5
/truyentranh page publish --overwrite   # dựng lại trang + đóng gói

# Web: nút "▤ Truyện tranh"
```

Tham số: `preset=` · `style=` · `from=` `to=` · `size=a4|b5` · `format=pdf,cbz,epub` ·
`maximages=N` · `imagesize=1K|2K` · `out=PATH` · `--overwrite`.

## Sản phẩm

| # | Bước | LLM? | Đầu ra |
|---|---|---|---|
| 1 | `style` | 1 lần | `style/art-direction.{json,md}` — token phong cách khoá cho cả sách |
| 2 | `character` | 1 lần | `nhan-vat/` — model sheet, `canonical_prompt` khoá diện mạo |
| 3 | `refsheet` | ảnh | `nhan-vat/*.png` — **ảnh** model sheet *(giai đoạn 2)* |
| 4 | `script` | mỗi chương | `kich-ban/{NN}.{json,md}` — chương → **trang → khung** + lời thoại |
| 5 | `layout` | **không** | `bo-cuc/{NN}.json` — toạ độ khung, vùng bong bóng |
| 6 | `panelprompt` | **không** | `prompts/chuong-{NNN}.{json,md}` — bảng prompt khung |
| 7 | `panelart` | ảnh | `art/chuong-{NNN}/` — tranh từng khung *(giai đoạn 2)* |
| 8 | `page` | **không** | `trang/chuong-{NNN}/{PP}.png` + `.svg` |
| 9 | `publish` | **không** | `xuat-ban/{TênSách}.{pdf,cbz,epub}` |

Chỉ ba bước gọi LLM. Toàn bộ phần còn lại thuần Go, không tốn token.

## Nguyên tắc: LLM ra ngữ nghĩa, Go ra hình học

LLM **không** sinh toạ độ. Nó chỉ quyết định mỗi trang mấy khung, mỗi khung `size`
(`nho`/`vua`/`lon`/`tran-trang`), chỗ `row_break`, và cú hích cuối trang. Go tra bảng trọng
số rồi tính rect **xác định**.

Lý do: LLM sinh toạ độ thì khung chồng nhau, hở lỗ, không lát kín trang — lỗi chỉ lộ ra khi
nhìn trang đã in và rất khó kiểm tự động. Tính bằng code thì **lát kín là bất biến có test**
(`TestLayoutTilesPage`).

Trọng số ăn khớp ngân sách hàng 3,0 cho ra đúng các tổ hợp quen thuộc:

```
nho+nho+nho = 3,0 ✓    vua+vua = 3,0 ✓    lon+nho = 3,0 ✓    vua+nho = 2,5 ✓
lon+vua = 3,5 ✗ tách hàng               lon+lon = 4,0 ✗ tách hàng
```

## Chữ trong tranh bị cấm tuyệt đối

Mọi negative prompt đều chứa `text, letters, words, speech bubble, watermark`. Chữ do bộ dàn
trang vẽ bằng font thật, vì mô hình sinh ảnh viết tiếng Việt có dấu gần như luôn sai.

Hệ quả tốt: chữ trong bản SVG vẫn là `<text>` — sửa lại được trước khi in, và **dịch sang
ngôn ngữ khác chỉ cần thay nội dung `<text>`**, không phải vẽ lại tranh.

## Nhất quán nhân vật — ba tầng

Đây là chỗ truyện tranh do AI vẽ hay hỏng nhất.

1. `canonical_prompt` — mô tả cố định, chèn **nguyên văn** vào mọi khung có nhân vật đó.
2. `refsheet` — sinh **ảnh** model sheet một lần *(giai đoạn 2)*.
3. `panelart` — truyền chính ảnh đó làm **ảnh tham chiếu** cho từng khung *(giai đoạn 2)*.

Nếu dự án đã chạy `/video`, các bước trên **nạp lại** `video/consistency-bible.json`,
`video/characters/*.json`, `video/concept/art-direction.json` và `video/storyboard/{NN}.json`
thay vì sinh mới — vừa rẻ hơn, vừa giữ cho bản truyện tranh và bản video cùng một thế giới
hình ảnh.

## Chữ tiếng Việt — hai cạm bẫy kỹ thuật

**1. Bắt buộc chuẩn hoá NFC.** `x/image/font/sfnt` chỉ nối dây bảng GPOS *kern*, **không có
GSUB và không có mark-to-base**; `font.Face` lại là API theo từng rune. Chuỗi dạng tổ hợp
(`e` + U+0302 + U+0301 thay vì `ế` U+1EBF — hay gặp với văn bản dán từ macOS) sẽ bị vẽ dấu
chồng đống ở gốc bút. May là cả 134 chữ cái tiếng Việt đều có dạng tiền-kết-hợp nên NFC là
đủ, **không cần engine shaping nào**.

**2. Không căn giữa bằng `Metrics().Ascent`.** Đó là ascent *typographic*, một hằng số thiết
kế. Đo thực tế trên Patrick Hand ở 64px: ascent = −66,7 nhưng mực chữ thường chỉ −30 — căn
giữa bằng ascent sẽ đẩy chữ xuống ~36px. Bộ dựng quét `GlyphBounds` của **chính chuỗi sắp
vẽ** để lấy đỉnh mực thật, và đặt giãn dòng mặc định **1,30** (tiếng Anh chỉ ~1,15) để dấu
chồng dòng dưới không đụng đuôi chữ dòng trên.

## Font

Nhúng sẵn ba font **SIL OFL 1.1**, đã kiểm chứng phủ đủ **165 glyph** tiếng Việt
(`TestFontsCoverVietnamese`): **Patrick Hand** (thoại) · **Bangers** (tượng thanh) ·
**Be Vietnam Pro** (thuyết minh).

> ⚠ Đổi font phải chạy lại test phủ glyph. Rất nhiều font lettering truyện tranh **không có
> dấu tiếng Việt**: **Comic Neue** tuy OFL nhưng thiếu hẳn khối U+1EA0–1EF9 nên `Ở`, `ệ`, `ữ`
> thành ô vuông lặng lẽ; Blambot *Comicrazy*/*Wildwords* là font thương mại.

## Đóng gói xuất bản

- **PDF 300 DPI** — `MediaBox`/`BleedBox`/`TrimBox`, ảnh JPEG nhét thẳng qua `/DCTDecode`.
  PDF **không có** thuộc tính DPI: kích thước vật lý nằm trọn ở ma trận `W 0 0 H 0 0 cm`.
  Tiêu đề tiếng Việt mã hoá UTF-16BE (chuỗi literal PDF sẽ hỏng dấu).
  A4 @300 DPI = 2480×3508 px; kèm tràn lề 3mm = **2551×3579 px**.
- **CBZ** — zip `Store` (ảnh đã nén sẵn), đánh số 3 chữ số, kèm `ComicInfo.xml`.
- **EPUB3 fixed-layout** — `rendition:layout pre-paginated`, viewport đúng bằng pixel ảnh,
  ảnh bọc `<svg>` nên manifest bắt buộc `properties="svg"`. Ảnh màn hình **cắt bỏ tràn lề**.
- **PNG + SVG** từng trang — có sẵn từ bước `page`.

Không dùng thư viện ngoài nào; cả ba bộ đóng gói đều viết tay, đúng tinh thần EPUB chữ ở
[internal/host/exp/epub.go](../internal/host/exp/epub.go).

## Hai giai đoạn

**Giai đoạn 1 — không tốn một xu tiền ảnh.** Chạy trọn đường ống, khung dùng ô
giữ chỗ có gạch chéo. Kết quả là **trang truyện tranh thật, chỉ thiếu tranh** — đủ để chấm
bố cục, nhịp trang, vị trí bong bóng, typography tiếng Việt, và thử cả bốn định dạng xuất bản.

**Giai đoạn 2 (đã có) — sinh ảnh thật qua Gemini.** Bật bằng cách điền khoá API:

```jsonc
"comic": { "api_key": "", "model": "gemini-2.5-flash-image", "image_size": "2K" }
```

`api_key` để trống thì hệ thống **tự mượn** `providers["gemini"].api_key` — không phải khai lại.
Hoặc nhập thẳng trong khối *🎨 Sinh ảnh khung* của hộp thoại Truyện tranh trên Web.

> **Bấm "Kiểm tra kết nối (1 ảnh)" trước khi chạy cả sách.** Nó sinh đúng một ảnh nhỏ để
> xác thực khoá và phương ngữ dây. Google đang có **hai** bề mặt API sinh ảnh và đã gắn nhãn
> đường cũ là "Legacy"; chạy cả cuốn rồi mới phát hiện sai phương ngữ thì mất cả trăm đô.
> Nếu đường mặc định lỗi, đổi `dialect` sang `interactions` rồi kiểm tra lại.

Bộ dựng đọc ảnh **theo đường dẫn tệp**, nên:

- Giai đoạn 2 chỉ việc đặt tệp vào `art/chuong-{NNN}/t{PP}-k{KK}.png`.
- **Bạn tự vẽ tay hoặc tự chạy bộ sinh ảnh rồi thả tệp vào đúng chỗ cũng chạy ngay** — xem
  `prompts/chuong-{NNN}.md` để biết prompt và đường dẫn. Chạy lại `/truyentranh page publish`
  là có trang hoàn chỉnh, không cần sửa dòng code nào.
- Vẽ lại một khung hỏng = xoá đúng một tệp PNG rồi chạy lại.

### Hai bẫy prompt đã gặp thật khi chạy trên tranh thật

Cả hai chỉ lộ ra khi **nhìn ảnh sinh ra**, test không bắt được:

1. **Đừng nhắc tên thứ mình không muốn vẽ.** Bản đầu ghi *"leave empty negative space … for a
   speech balloon"* — mô hình nghe thấy danh từ thì nó **vẽ hẳn một bong bóng rỗng** vào tranh,
   và câu khẳng định đó còn thắng cả token cấm trong negative prompt. Nay chỉ mô tả vùng ảnh
   mong muốn ("giữ góc trên-trái thoáng, nền trơn") mà không nói vùng đó để làm gì.
2. **Chữ "panel" khiến mô hình tự vẽ khung viền.** Token phong cách có `comic panel` làm mỗi
   ảnh sinh ra là một khung tranh có sẵn viền và lề trắng — rồi bộ dàn trang vẽ viền lần nữa
   thành khung lồng khung. Nay preset dùng `comic book illustration`, và negative cấm thẳng
   `panel border, picture frame, white margin, multiple panels`.

## Cắt chi phí sinh ảnh

Sinh ảnh là bước tốn tiền duy nhất. Bốn đòn bẩy, xếp theo hiệu quả:

**1. Chạy lượt `nhap` trước, `in` sau.** Mặc định là `nhap` (1K, model rẻ). Chỉ khi đã ưng
một chương mới chạy `pass=in` cho riêng chương đó ở 2K. Nháp cả sách rồi in 4 chương chọn lọc
≈ **$88**, so với in thẳng cả sách ≈ **$192**.

```bash
/truyentranh to=32              # nháp cả sách, 1K
/truyentranh panelart page pass=in from=1 to=4 --overwrite   # chỉ in 4 chương đã chọn
```

**2. Chỉ trả tiền cho khung CÒN THIẾU.** Cơ chế bỏ-qua-nếu-đã-có làm chạy lại gần như miễn
phí. Hộp thoại Web hiển thị đúng con số này (đếm thật từ `prompts/`), không phải tổng số khung.

**3. Ít khung mà to.** Chi phí tỉ lệ thuận số khung, mà truyện tranh hay lại thường ít khung
mà to — nhồi khung nhỏ là lỗi của người mới. Trần mặc định 6 khung/trang, đổi bằng `maxpanels=`.

**4. Chọn đúng model.** Xem bảng dưới. Lưu ý `gemini-2.5-flash-image` nhận tham số `2K` rồi
**lặng lẽ trả về 1K** mà vẫn tính tiền đủ — hệ thống nay tự phát hiện và cảnh báo.

### Model nào làm được gì

| Model | Độ phân giải | Giá/ảnh | Dùng khi |
|---|---|---|---|
| `gemini-3.1-flash-lite-image` **(mặc định)** | **chỉ 1K** | $0,0336 | bản nháp, đọc màn hình |
| `gemini-2.5-flash-image` | thực tế ~1K | $0,039 | bản cũ, không có lợi thế gì |
| `gemini-3.1-flash-image` | 512/1K/2K/4K | $0,067 / $0,101 (2K) | **in 300 DPI** |
| `gemini-3-pro-image` | 1K/2K/4K | $0,134 | chất lượng cao nhất |

Hệ thống **không im lặng khi độ phân giải bị bỏ qua**: nếu model không làm được cỡ đã chọn thì
tham số bị bỏ đi kèm cảnh báo, và ảnh trả về luôn được **đo pixel thật** để đối chiếu.

> **Định dạng tệp theo MIME thật.** Các model trả định dạng khác nhau — bản 2.5 trả PNG, bản
> 3.1-flash-lite trả JPEG. Ảnh khung được ghi với đuôi khớp nội dung thật (`.png`/`.jpg`), nếu
> không thì bản SVG sẽ trỏ tới `.png` chứa JPEG và trình đọc nghiêm ngặt sẽ từ chối.

### Không có nguồn miễn phí nào phù hợp

Đã tra thực tế, không phải phỏng đoán:

- **OpenRouter**: 341 model, **11 model sinh ảnh, 0 miễn phí**. 15 model hậu tố `:free` đều
  **chỉ ra chữ**. Giá ảnh bằng đúng Google.
- **9Router**: cổng trung chuyển mã nguồn mở, miễn phí — nhưng backend vẫn tính tiền. Chỉ thật
  sự miễn phí khi trỏ vào ComfyUI/SD WebUI chạy máy nhà, mà việc đó cần GPU rời.
- **Pollinations**: miễn phí thật, không cần khoá, nhưng **không nhận ảnh tham chiếu** → mất
  hẳn cơ chế giữ nhất quán nhân vật.
- **Free tier Gemini**: trang giá chính thức của Google ghi **"Not available"** cho mọi model ảnh.

## Chi phí sinh ảnh *(bảng tham khảo)*

Một cuốn 32 chương ≈ **1.900 khung**.

| Model | Giá/ảnh | 1.900 khung |
|---|---|---|
| `gemini-3.1-flash-lite-image` (1K) | $0,0336 | ~$64 |
| `gemini-2.5-flash-image` | $0,039 | ~$74 |
| `gemini-3.1-flash-image` 1K / 2K | $0,067 / $0,101 | ~$127 / ~$192 |
| `gemini-3-pro-image` | $0,134 | ~$255 |

> ⚠ **Muốn in thì phải sinh ở 2K**, không phải 1K — A4 @300 DPI là 2480×3508 px, một khung
> nửa trang đã cần ~2480×1200 px. Điều này gần như **gấp đôi** chi phí so với ước tính hồn
> nhiên. Bản 1K chỉ đủ cho CBZ/EPUB đọc màn hình.

Biện pháp: `maximages=N` làm cầu dao mỗi lần chạy · hộp thoại Web ước tính trước khi chạy ·
mặc định **không** ghi đè nên chạy lại chỉ lấp chỗ trống · chạy `/truyentranh to=1` trước.

## Hành vi

- **Ghi nguyên tử** (temp + fsync + rename), đầu ra mặc định `{novelDir}/truyen-tranh/`.
- **guardExclusive** — không chạy chồng với Coordinator; Web hủy qua `POST /api/job/cancel`,
  TUI bằng `Esc`.
- **Bỏ qua mềm từng chương/khung** — một chương hỏng không làm sập cả lần chạy.
- **Chạy lại tăng dần** — không `--overwrite` thì bỏ qua tệp đã có, kể cả bước LLM.
- Model dùng vai trò **`architect`**.

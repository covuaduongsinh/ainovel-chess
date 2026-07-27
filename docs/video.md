# Chuyển thể sách → sản phẩm làm video

Khả năng ngang `internal/host/adapt` biến một dự án sách **đã hoàn thành cốt truyện**
thành bộ sản phẩm phục vụ sản xuất video. Đối xứng với `imp` (nhập) / `sim` (phỏng tác) /
`exp` (xuất): đây là tác vụ **LLM-nặng, nhiều bước, chỉ đọc** dữ liệu trong store rồi ghi
file ra ngoài. Vào qua lệnh **`/video`** (TUI) và nút **🎬 Làm video** (Web); cả hai gọi
`host.Host.Adapt(ctx, adapt.Options)` và dùng chung `adapt.Options` để mirror nhau.

## Sản phẩm

Chín loại (`adapt.Product`), chia hai nhóm:

**Dùng LLM** (mỗi bước gọi mô hình):

| Product | Lời gọi | Đầu ra dưới `video/` |
|---|---|---|
| `concept` | 1 | `concept/art-direction.{json,md}` — phong cách tổng thể + palette/ánh sáng/ngôn ngữ máy quay + `style_tokens` + địa điểm chính |
| `character` | N nhân vật core/important | `characters/{slug}.json` + `characters/characters.{json,md}` — ngoại hình/trang phục/key-art/turnaround/negative prompt |
| `prop` | 1 | `props/props.{json,md}` — đạo cụ chủ chốt + image prompt |
| `consistency` | 1 | `consistency-bible.{json,md}` — canonical prompt cố định cho mỗi nhân vật/đạo cụ/địa điểm + style tokens chung |
| `screenplay` | M chương | `screenplay/{NN}.md` — scene heading / action / lời thoại |
| `storyboard` | M chương | `storyboard/{NN}.{json,md}` — cảnh → shots: góc máy, chuyển động, thời lượng, image/video prompt song ngữ, lời thoại |

**Render thuần** (không gọi LLM, tổng hợp cơ học từ `storyboard/{NN}.json` đã có):

| Product | Đầu ra |
|---|---|
| `animation` | `animation/{NN}.md` — chỉ đạo chuyển động máy/nhân vật, chuyển cảnh, nhịp theo shot |
| `imageprompt` | `prompts/image-prompts.md` — bảng phẳng mọi image prompt (kèm nhân vật + địa điểm), copy-paste |
| `videoprompt` | `prompts/video-prompts.md` — bảng phẳng mọi video prompt + thời lượng |

> Ba loại render thuần yêu cầu `storyboard` đã chạy trước. Nếu chưa có, bước phát một sự
> kiện nhắc "hãy chạy storyboard trước" và **không** tự sinh (tránh phát sinh chi phí ngầm).

## Chế độ đóng gói (`Grouping`)

Hai cách tổ chức thứ tự xử lý + đầu ra (chọn qua ô "Cách đóng gói" trên Web, hoặc
`group=chapter|product` trên TUI):

- **`chapter` (mặc định)** — *gói theo chương*: chạy phần cấp-sách (concept/character/prop/
  consistency) **một lần** làm "style bible", rồi duyệt **tập → chương** theo thứ tự; mỗi
  chương làm trọn kịch bản → phân cảnh → animation → prompt **trước khi** sang chương kế, và
  ghi ngay một **thư mục đóng gói lồng** cho chương đó. Nhờ vậy **tập trước hoàn chỉnh
  trước** — có thể dựng video từng tập trong khi các tập sau còn đang sinh.
- **`product`** — *gói theo loại* (hành vi cũ): mỗi loại sản phẩm quét toàn bộ chương (xong
  hết kịch bản 1…N rồi mới phân cảnh 1…N…). Không tạo đóng gói lồng.

Cả hai chế độ **đều giữ nguyên** bố cục thư mục theo loại ở trên (`video/screenplay/`,
`video/storyboard/`, …). Chế độ `chapter` bổ sung thêm đóng gói lồng.

### Đóng gói lồng (chỉ ở chế độ `chapter`)
Mỗi chương có một thư mục gom đủ liệu để dựng video, nhóm theo tập:

```
video/
  tap-{VV}/
    _tap-{VV}.md          # mục lục tập: liên kết tới từng chương
    chuong-{NNN}/
      kich-ban.md         # kịch bản
      phan-canh.json/.md  # phân cảnh
      animation.md        # chỉ đạo animation
      prompt-anh.md       # prompt ảnh của riêng chương
      prompt-video.md     # prompt video của riêng chương
```
`VV` = số tập (sách không phân tầng → `01`); `NNN` = số chương toàn cục. Đóng gói là **render
thuần** (không gọi LLM) từ kết quả cấp-chương; nếu bỏ chọn screenplay/storyboard, gói tự nạp
lại từ các file theo loại đã có trên đĩa.

## Thứ tự sản phẩm cấp-chương

`screenplay → storyboard → animation → imageprompt → videoprompt`

Phần cấp-sách (`concept → character → prop → consistency`) luôn chạy trước để tạo "style
bible"; `storyboard` tiêm token chuẩn từ `consistency-bible` (hoặc concept/character/prop nếu
chưa tổng hợp consistency) vào từng prompt, giữ nhân vật/đạo cụ **nhất quán xuyên suốt**
(chương 5 và chương 80 mô tả giống nhau).

## Quy ước prompt

- **Trung lập, giàu chi tiết** (subject + bối cảnh + phong cách + ánh sáng + ống kính/khung
  hình; video prompt thêm chuyển động). Không khóa vào cú pháp một công cụ (dùng được cho
  Midjourney / Runway / Kling / Veo…). Có `negative_prompt` và `duration_sec`.
- **Song ngữ:** các trường `*_prompt` và `style_tokens` viết **tiếng Anh**; mô tả/nhãn
  (`description`, `appearance`, `heading`, `summary`…) viết **tiếng Việt**.

## Tham số & hành vi

`adapt.Options`: `Products` (rỗng = tất cả), `Grouping` (rỗng = `chapter`; hoặc `product`),
`From`/`To` (phạm vi chương, 0/0 = toàn bộ đã hoàn thành; chỉ áp cho
screenplay/storyboard/animation/imageprompt/videoprompt), `StyleHint` (gợi ý phong cách),
`OutDir` (mặc định `{novelDir}/video/`), `Overwrite`.

- **Ghi nguyên tử** (temp + fsync + rename) — output có thể nằm ngoài `store.Dir()`.
- **guardExclusive**: job dài, không chạy chồng Coordinator; Web hủy qua `POST /api/job/cancel`,
  TUI hủy bằng `Esc`.
- **Fail-soft theo chương**: một chương lỗi (parse/rỗng) chỉ bị bỏ qua, các chương khác vẫn chạy.
- **Resume incremental**: không `--overwrite` thì bỏ qua file đã tồn tại — chạy lại chỉ bù phần thiếu.
- Model dùng vai **`architect`** (`h.models.ForRole("architect")`).

## Cách dùng

**TUI:**

```
/video                                  # chạy tất cả, toàn bộ chương
/video concept                          # chỉ art direction
/video character prop consistency       # cụm thiết kế hình ảnh
/video screenplay storyboard to=3       # kịch bản + phân cảnh 3 chương đầu (gói theo chương)
/video imageprompt                      # bảng prompt ảnh (cần storyboard trước)
/video all from=1 to=10 style=anime --overwrite
/video group=product                    # chạy theo kiểu cũ (gói theo loại, không đóng gói lồng)
```

**Web:** nút **🎬 Làm video** → chọn sản phẩm (không chọn = tất cả) + phạm vi + gợi ý phong
cách + ghi đè → tiến trình hiện realtime qua SSE, có nút **Dừng**.

**HTTP:** `POST /api/adapt` body `{products:[], from, to, style, outDir, overwrite}` → trả
`{id}`; tiến trình đẩy qua SSE (`progressDTO`, `job:"adapt"`).

## Chi phí

`all` trên tiểu thuyết dài tốn nhiều lời gọi LLM (≈ số chương × 2 cho screenplay+storyboard,
cộng thiết kế). Nên giới hạn `from/to`, chạy lẻ từng product, và tận dụng resume incremental.
Ba product render thuần không tốn LLM.

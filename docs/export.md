# Xuất bản thảo (Export)

Tài liệu này gom toàn bộ kiến thức về khả năng **xuất các chương đã hoàn thành** thành file
đọc được. Trọng tâm mới: **xuất đa định dạng** — mặc định tạo cùng lúc 3 file `.md` / `.txt` /
`.epub` trong một lần chạy.

Mã nguồn liên quan:
- [internal/host/exp/types.go](../internal/host/exp/types.go) — `Format`, `Options`, `Output`, `Result`, `FormatsForPath`
- [internal/host/exp/exporter.go](../internal/host/exp/exporter.go) — `Run` (vòng lặp đa định dạng)
- [internal/host/exp/md.go](../internal/host/exp/md.go) — `renderMD`
- [internal/entry/tui/export.go](../internal/entry/tui/export.go) — lệnh `/export`
- [internal/entry/web/api_command.go](../internal/entry/web/api_command.go) — `POST /api/export`
- [internal/entry/web/server.go](../internal/entry/web/server.go) — `exportBase` trong `GET /api/meta`

## 1. Export là gì

Export **hợp nhất các bản thảo chương đã hoàn thành** (đọc từ `store.Drafts` + `Progress`)
thành một hay nhiều file hoàn chỉnh. Đặc điểm cốt lõi:

- **Chỉ đọc.** Không gọi LLM, không đổi trạng thái store. Có thể lấy "sản phẩm giai đoạn hiện
  tại" **bất kỳ lúc nào** giữa chừng viết, không ảnh hưởng Coordinator đang chạy — đây là một
  *khả năng ngang* (đối xứng với `imp/` nhập truyện).
- **Ghi nguyên tử.** Mỗi file được ghi qua temp + `fsync` + `rename` (`atomicWrite`), tránh file
  rác khi bị ngắt giữa chừng.
- **Nguồn dữ liệu:** chỉ các chương nằm trong `Progress.CompletedChapters`. Tên sách, dàn ý
  (tiêu đề chương) và cấu trúc tập (khi tiểu thuyết ở chế độ phân tầng) được ghép vào đầu ra.

## 2. Ba định dạng

`Format` (trong `types.go`) có 3 giá trị: `md`, `txt`, `epub`. `DefaultFormats()` trả về
`[md, txt, epub]`.

| Định dạng | Đặc điểm |
|---|---|
| **MD** (`renderMD`) | Markdown tiêu đề ATX: tên sách → `# Tên sách`; đầu tập → `## Tập N — Tiêu đề` (chỉ khi dàn ý phân tầng); mỗi chương → `## Chương N — Tiêu đề` (hoặc `## Chương N`). Thân chương giữ nguyên văn, các chương cách nhau một dòng trống. Dễ đọc/mở ở mọi editor và convert tiếp (pandoc → DOCX/PDF…). |
| **TXT** (`renderTXT`) | `《Tên sách》` → phân cách tập → nội dung chương. Định dạng thuần văn bản, gọn nhẹ. |
| **EPUB** (`renderEPUB`) | Container EPUB 3 chuẩn: trang bìa, mục lục, XHTML tách theo chương. Định danh dẫn xuất ổn định từ nội dung (xuất lại cùng sách → trình đọc coi là bản cập nhật). **Không có ảnh bìa.** Apple Books / WeChat Reading / Kindle converter đọc được. |

**Dữ liệu nội bộ không vào bản xuất** (áp dụng cho mọi định dạng):
- **premise** — bản thiết kế sáng tác (đối tượng độc giả mục tiêu, vùng cấm viết…): thông tin
  hậu trường cho tác giả và engine, không phải lời mở đầu cho độc giả.
- **phân cách arc** — từ góc nhìn độc giả, arc là cấu trúc nội bộ quá chi tiết. (Tên sách và
  phân cách **tập** thì vẫn giữ.)

Trình xuất thống nhất sinh tiêu đề "Chương N — Tiêu đề"; nếu writer đã tự chèn tiêu đề trùng
lặp trong thân chương (`# Chương N…` hoặc `# Tên chương`) thì sẽ bị loại bỏ
(`stripChapterTitleHeader`).

## 3. Đường dẫn base & quy tắc chọn định dạng

Đường dẫn người dùng nhập được coi là **đường dẫn base** (không đuôi định dạng). Hàm chung
`FormatsForPath(path)` quyết định danh sách định dạng — **Web và TUI dùng chung** để hành vi
mirror nhau:

- Nếu path có **đuôi nhận biết** (`.md` / `.txt` / `.epub`) → chỉ định dạng đó.
- Ngược lại (không đuôi hoặc để trống) → `DefaultFormats()` = cả 3.

Khi ghi, `exp.Run` bỏ đuôi nhận biết khỏi base (`stripRecognizedExt`) rồi ghi mỗi định dạng ra
`base + "." + ext`. Nếu base rỗng, mặc định là `{novelDir}/{TenSách đã làm sạch}`.

| Đường dẫn nhập | File xuất ra |
|---|---|
| _(để trống)_ | `{novelDir}/{TenSách}.md`, `.txt`, `.epub` |
| `D:\truyen\tac-pham` | `tac-pham.md`, `tac-pham.txt`, `tac-pham.epub` |
| `D:\truyen\tac-pham.md` | chỉ `tac-pham.md` |
| `D:\truyen\tac-pham.txt` | chỉ `tac-pham.txt` |
| `D:\truyen\tac-pham.epub` | chỉ `tac-pham.epub` |

> Lưu ý: đuôi *không* nhận biết (vd tên file có dấu chấm như `tap-1.5`) được giữ nguyên làm base,
> tránh cắt nhầm — kết quả sẽ là `tap-1.5.md/.txt/.epub`.

## 4. Cách dùng

### TUI

Cú pháp: `/export [base] [from=N] [to=M] [--overwrite]`

```text
/export                            # 3 file .md/.txt/.epub tại {novelDir}/{TenSách}.*
/export ~/anh-sang                 # base không đuôi → 3 file ~/anh-sang.md/.txt/.epub
/export ~/anh-sang.epub            # chỉ EPUB
/export from=10 to=30 --overwrite  # khoảng chương 10..30, ghi đè (vẫn ra 3 file)
/export from=10 ~/x.md --overwrite # chỉ Markdown, từ chương 10
```

- Tối đa **một** tham số vị trí, dùng làm base.
- `from` / `to` là số nguyên không âm; `0`/bỏ trống = từ chương 1 / đến chương cuối.
- Khi xuất nhiều file, thông báo thành công liệt kê từng file kèm dung lượng.

### Web UI

- Modal **⬇ Xuất file**: ô đường dẫn nhận **base** và được **điền sẵn** gợi ý `exportBase`
  (lấy từ `GET /api/meta`, giá trị `{dir}/{tên thư mục}`).
- Để trống / không đuôi → 3 file; gõ đuôi `.md`/`.txt`/`.epub` → 1 định dạng.
- Toast kết quả: `✓ Đã xuất N chương → K file (.md, .txt, .epub)`.

## 5. Hợp đồng API Web

### `POST /api/export`

Request: `{ path, from, to, overwrite }` (`path` là base). Server tự tính `Formats` bằng
`exp.FormatsForPath(path)`.

Response (**shape mới** — mảng `files`):

```json
{
  "files": [
    { "format": "md",   "path": "…/tac-pham.md",   "bytes": 12345 },
    { "format": "txt",  "path": "…/tac-pham.txt",  "bytes": 12000 },
    { "format": "epub", "path": "…/tac-pham.epub", "bytes": 40960 }
  ],
  "chapters": 30,
  "skipped": [31, 32]
}
```

> **Breaking:** shape cũ `{ path, bytes, chapters, skipped }` đã bị bỏ. Client đọc `r.path`
> cần chuyển sang duyệt `r.files`.

### `GET /api/meta`

Trả thêm trường `exportBase` = `{novelDir}/{tên thư mục}` — đường dẫn base gợi ý (không đuôi)
để frontend điền sẵn ô đường dẫn.

## 6. Chương chưa hoàn thành

Các số chương nằm trong khoảng `from..to` nhưng **chưa hoàn thành** không phải lỗi: chúng bị bỏ
qua và trả về trong `Result.Skipped` (Web: `skipped`), hiển thị cho người dùng biết. Ví dụ yêu
cầu `to=100` nhưng mới viết đến chương 80 → các chương 81..100 vào `Skipped`.

Các trường hợp thực sự là **lỗi** (trả về error, dừng xuất): chưa có chương hoàn thành nào;
`Progress` đánh dấu chương đã xong nhưng file bản thảo thiếu/rỗng (bug bất nhất giữa progress và
hệ thống file, cố ý để lộ); file đích đã tồn tại mà không kèm `--overwrite`.

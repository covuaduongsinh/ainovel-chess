# Bản đồ nội dung assets

Trước khi thêm "một đoạn/một tài liệu/một quy tắc" vào hệ thống, hãy tra bảng dưới để xác định vị trí, rồi xem cách nối dây.

| Thư mục | Chứa gì | Ai tiêu thụ | Cách nối dây |
|---------|---------|------------|-------------|
| `prompts/` | System prompt thường trú cho các vai (coordinator / writer / editor / architect×2) và prompt nhiệm vụ một lần (import×2 / simulation×2) | `agents/build.go` lắp ráp; imp / sim runner | Trường `Prompts` của `load.go`. Lưu ý: `simulation_guidance` được `load.go` tiêm khi tải, không thấy trong file .md |
| `references/` | Tài liệu kiến thức viết lách không phụ thuộc đề tài. Không vào system prompt, được `novel_context` cắt xén theo vai/chương rồi tiêm vào `reference_pack` | writer / editor / architect | **Ba chỗ nối dây**: thêm trường `tools.References` + `load.go` `loadReferences` đọc vào + `novel_context.go` `writerReferences` / `architectReferences` tiêm. Đặt vào thư mục không tự động tải |
| `references/genres/<style>/` | Kiến thức chuyên biệt theo đề tài (style-references / arc-templates) | Như trên, tải khi `style != default` | `load.go` `loadReferences` |
| `rules/` | Thư mục quy tắc tích hợp cũ đã bỏ; cơ sở cơ học đã chuyển vào code, quy tắc người dùng đến từ snapshot ngôn ngữ tự nhiên `~/.ainovel/rules/*.md` / `./.ainovel/rules/*.md` | `userrules.Service` chuẩn hóa thành `meta/user_rules.json`; `novel_context` tiêm; `commit_chapter` kiểm tra | Cơ sở tích hợp xem `SystemDefaults()` trong `internal/rules/snapshot.go`; file `.md` của người dùng không cần format, không cần YAML, chuẩn hóa theo ngôn ngữ tự nhiên |
| `styles/<style>.md` | Chỉ thị phong cách viết theo đề tài | Ghép vào system prompt của **writer** (`agents/build.go`) | Tên file chính là giá trị `config.style`. Cùng với `references/genres/<style>/` là hai dạng mang của cùng một khái niệm đề tài: cái trước là chỉ thị phong cách, cái sau là tài liệu kiến thức |

## Xác định vị trí nội dung mới (năm câu hỏi)

1. Quy trình này có phải được **đảm bảo**? → Không viết vào prompt, viết ràng buộc code (StopAfterTools / tool guard / Flow Router)
2. Đây là tiêu chí phán định (khi nào phái ai)? → `prompts/coordinator.md`
3. Đây là tiêu chuẩn thẩm mỹ/thực thi của một vai? → `prompts/<role>.md`
4. Đây là quy tắc mặc định có thể liệt kê cơ học (từ cấm / số chữ / ngưỡng)? → `SystemDefaults()` trong `internal/rules/snapshot.go`; quy tắc người dùng tùy chỉnh viết vào `.ainovel/rules/*.md`, được snapshot chuẩn hóa tiêu thụ
5. Đây là tài liệu kiến thức viết lách? → `references/` (nhớ ba chỗ nối dây)

## Đảm bảo nhất quán

Đường dẫn phong bì mà prompt tham chiếu (`working_memory.*` v.v.) và tài liệu tham số `commit_chapter` trong writer.md
được `prompts_consistency_test.go` kiểm tra tự động — hai loại trôi dạt này không báo lỗi, chỉ làm model lặng lẽ kém đi, dựa vào đèn đỏ test để phát hiện.
Đoạn quy trình trong prompt là "hướng dẫn người dùng," chân lý quy trình ở tầng code; khi hai cái không khớp, code là chuẩn và phải quay lại sửa prompt.

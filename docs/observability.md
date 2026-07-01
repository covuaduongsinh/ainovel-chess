# Cẩm Nang Quan Sát Hệ Thống

Khi chạy tiểu thuyết dài, làm thế nào để biết các cơ chế có thực sự đang hoạt động đúng không?

Tài liệu này không phải bản sao lại các quy tắc diag, mà tập trung vào **vận hành thực tế**: khi bạn đang chạy đến chương N, nên mở file nào, xem trường nào, và phán đoán hệ thống đang khỏe hay có vấn đề.

---

## 1. Quy trình kiểm tra chung

```
1. /diag                       # Tự động chẩn đoán, xem phần Findings
2. cd output/{novel}/meta/     # Trực tiếp cat các artifact quan trọng
3. cat meta/sessions/coordinator.jsonl | tail  # Xem hành vi LLM vài vòng gần nhất
```

Những điều `/diag` chưa bao phủ (bao gồm các mục "chưa có chẩn đoán" liệt kê trong tài liệu này) cần kiểm tra thủ công theo bước 2-3.

### Báo issue: Xuất chẩn đoán đã ẩn danh

Mỗi lần chạy `/diag` sẽ tự động ghi thêm file `output/{novel}/meta/diag-export.md` — một bản chẩn đoán **đã được ẩn danh** (nội dung tiểu thuyết / prompt / quá trình suy nghĩ đã được loại bỏ, chỉ giữ lại khung hành vi: tên công cụ, chuỗi lỗi, số lần lặp, phase/flow, bước bị kẹt, phân loại lỗi log). Khi gặp vòng lặp vô tận hoặc sự cố gián đoạn, chỉ cần đính kèm file này lên GitHub issue; người bảo trì có thể định vị vấn đề mà không cần dữ liệu `output/` của bạn.

---

## 2. Bảng tra cứu nhanh các artifact quan trọng

Sắp xếp theo "đường kiểm tra phổ biến nhất khi có sự cố":

| Artifact | Đường dẫn | Xem gì | Bình thường | Bất thường |
|---|---|---|---|---|
| Tiến độ | `meta/progress.json` | `phase` / `flow` / `completed_chapters` | phase tiến đơn điệu, flow nằm trong tập hợp hợp lệ | phase thụt lùi / flow kẹt ở một trạng thái |
| La bàn | `meta/compass.json` | `last_updated` và khoảng cách với chương mới nhất | gap < 15 chương | gap > 15 chương (CompassDrift kích hoạt) |
| Danh sách nhân vật phụ | `meta/cast_ledger.json` | Số mục / tỷ lệ điền brief_role / tính nhất quán tên | Xem §4 | Xem §4 |
| Bảng theo dõi foreshadow | `meta/foreshadow.json` | Số chương đình trệ lâu nhất của `status="planted"` | < số_chương/3 | > số_chương/3 (StaleForeshadow kích hoạt) |
| Đại cương | `meta/layered_outline.json` | Số chương còn chưa viết trong tập hiện tại | Đã mở rộng trước 1-2 chương | Đã viết đến chương hiện tại nhưng chương tiếp theo không có outline (OutlineExhausted) |
| Hồ sơ nhân vật | `meta/characters.json` | Có tìm được nhân vật core/important trong tóm tắt N chương gần nhất không | Tìm được tất cả | Vắng mặt (GhostCharacter kích hoạt) |
| Checkpoint | `meta/checkpoints.jsonl` | `step` ở dòng cuối có khớp với progress không | Khớp | Không khớp (khôi phục sau crash chưa tự phục hồi) |
| Phiên Coordinator | `meta/sessions/coordinator.jsonl` | Mẫu tool_call trong 5-10 vòng gần nhất | Mỗi vòng tiến nhanh | Cùng một công cụ gọi nhiều lần trống (kẹt vòng lặp) |

---

## 3. Quan sát La Bàn (compass)

**Thời điểm sửa**: 2026-05-08 (commit `fix: update_compass 工具自动填 last_updated`)

### Xem gì

```bash
cat output/{novel}/meta/compass.json
```

Ý nghĩa các trường:
- `ending_direction`: Hướng kết truyện (phải khớp với phần "hướng kết truyện" trong `premise.md`)
- `open_threads`: Các mạch truyện dài đang mở (architect thêm/bớt tại ranh giới mỗi tập)
- `estimated_scale`: Quy mô dự kiến (ví dụ "4-6 tập", cập nhật tại ranh giới mỗi tập)
- `last_updated`: **Công cụ tự điền** bằng số chương đã hoàn thành lớn nhất tại thời điểm cập nhật (không còn phụ thuộc LLM tự điền)

### Đánh giá độ khỏe

| Tín hiệu | Đánh giá |
|---|---|
| `last_updated` nằm trong khoảng `[latest-15, latest]` | Bình thường |
| `last_updated` chậm hơn latest quá 15 chương | architect chưa cập nhật tại ranh giới arc/tập — kiểm tra prompt architect-long.md |
| `last_updated == 0` | **Dữ liệu cũ trước khi sửa lỗi**, lần gọi `update_compass` tiếp theo sẽ tự phục hồi |
| `ending_direction` không khớp với phần "hướng kết truyện" trong premise.md | architect đã âm thầm thay đổi ý định của người dùng — ghi lại và cân nhắc có nên đóng băng trường này không (vấn đề thiết kế, xem todo.md) |

### Cách xác minh bản sửa có hiệu lực

So sánh trước và sau khi chạy truyện dài:
- **Trước khi sửa**: Sau 30+ chương, `compass.last_updated` rất có thể là `0` hoặc một số chương từ đầu
- **Sau khi sửa**: Mỗi lần architect gọi `update_compass`, `last_updated` đều được công cụ ghi đè bằng giá trị latest hiện tại

---

## 4. Quan sát Danh Sách Nhân Vật Phụ (cast_ledger)

**Tính năng triển khai**: 2026-05-08 (commit `feat: 新增配角名册自动追踪次要角色`)

### Xem gì

```bash
cat output/{novel}/meta/cast_ledger.json | jq 'length'                     # Tổng số mục
cat output/{novel}/meta/cast_ledger.json | jq '[.[] | select(.brief_role == "" or .brief_role == null)] | length'  # Số mục thiếu brief_role
cat output/{novel}/meta/cast_ledger.json | jq '[.[] | select(.appearance_count >= 3)] | length'   # Số nhân vật xuất hiện nhiều (≥3 lần)
cat output/{novel}/meta/cast_ledger.json | jq 'sort_by(-.appearance_count) | .[:10]'  # 10 nhân vật xuất hiện nhiều nhất
```

### Đánh giá độ khỏe

| Chiều | Bình thường | Bất thường | Cách xử lý |
|---|---|---|---|
| **Số mục vs số chương đã hoàn thành** | Số mục ledger ≈ số chương × 0.3-0.6 | > số chương × 0.8 (nhân vật thoáng qua bị đăng ký nhầm) | Kiểm tra phần `cast_intros` trong prompt writer.md có đủ rõ ràng không |
| **Tỷ lệ điền brief_role** | Thiếu < 30% | Thiếu > 50% | Writer bỏ điền nhiều — hướng dẫn trong prompt chưa đủ |
| **Độ tương đồng tên trùng nhau** | Không có tên nghi ngờ trùng người | Cùng xuất hiện "Lý X" / "Bác Lý" / "Chưởng quầy X" | LLM viết tên không nhất quán — thêm ràng buộc "dùng tên nhất quán" vào prompt hoặc thêm công cụ gộp tên theo yêu cầu người dùng |
| **Nhân vật xuất hiện nhiều** | Ít mục có `appearance_count >= 5` | Nhiều mục xuất hiện cao xuyên arc | Nên cân nhắc nâng cấp lên hồ sơ nhân vật chính (kênh thăng cấp giai đoạn 3) |
| **Có sử dụng kết quả recent_cast không** | Khi Writer viết nhân vật cũ, trường `characters` của `commit_chapter` chứa tên đã có trong ledger | Writer phát minh lại tên đã có (xuất hiện "Bác Châu A" và "Bác Châu B") | recent_cast chưa được tiêu thụ — kiểm tra phần "tính liên tục nhân vật phụ" trong writer.md |

### Xác minh luồng dữ liệu (đầu cuối)

Sau 5 chương:
1. `cat meta/cast_ledger.json` phải không rỗng (trừ khi mỗi chương chỉ dùng nhân vật chính)
2. Nếu Writer đã giới thiệu "Bác Châu" ở chương 1:
   - `cast_ledger` phải có mục `Bác Châu`, `appearance_count=1`
3. Nếu chương 5 viết thêm về Bác Châu:
   - `Bác Châu.appearance_count=2`, `last_seen_chapter=5`
4. Trong `meta/sessions/agents/writer-*.jsonl` của chương 5, giá trị trả về của `novel_context` phải có Bác Châu trong `episodic_memory.recent_cast`
5. Nếu bước trên thấy có mà Writer không sử dụng (nhân vật Bác Châu viết ra không khớp với chương 1) — đây là vấn đề prompt

### Hiện chưa có chẩn đoán tự động (nhưng snapshot đã được tải)

`diag.Snapshot.CastLedger` đã được đọc trong `Load()`, các quy tắc có thể tiêu thụ trực tiếp — nhưng hiện chưa có quy tắc nào được viết. Việc xác minh vẫn phải dùng lệnh `jq` thủ công ở trên.

Các quy tắc chẩn đoán có thể bổ sung sau (danh sách ứng viên):
- `CastBriefRoleMissing`: Cảnh báo khi tỷ lệ thiếu > 50%
- `CastBloat`: Cảnh báo khi số mục > số chương × 0.8
- `CastPromotionCandidate`: appearance_count ≥ 5 và xuất hiện xuyên arc → đề xuất thăng cấp

Chưa chốt ngưỡng ngay — đợi có dữ liệu truyện dài thực tế, xem phân phối thực rồi mới định. Bản thân code quy tắc chỉ cần 30-50 dòng.

---

## 5. Writer có đang hoạt động đúng kỳ vọng không

Khi chạy truyện dài, điều quan tâm nhất là **Writer có thực sự tuân theo prompt không**. Cách quan sát trực tiếp nhất là xem session log:

```bash
ls output/{novel}/meta/sessions/agents/    # Mỗi sub-agent một file jsonl
tail -50 output/{novel}/meta/sessions/agents/writer-*.jsonl
```

Kiểm tra một số hành vi cụ thể:

| Hành vi mong đợi | Biểu hiện trong jsonl |
|---|---|
| Writer đã đọc recent_cast | Giá trị trả về của công cụ `novel_context` có trường `episodic_memory.recent_cast` không rỗng |
| Writer điền cast_intros trong commit_chapter | Tham số tool_call `cast_intros` là mảng không rỗng (chỉ ở chương giới thiệu nhân vật mới) |
| Writer dùng gợi ý chương liên quan | Số lần gọi `read_chapter` > 1 (mặc định 1 lần, nhiều hơn là có tra cứu lại) |
| Writer không vi phạm thứ tự công cụ | Chuỗi tool_call phải đúng thứ tự `novel_context → read_chapter → plan_chapter → draft_chapter → check_consistency → commit_chapter` |

Nếu trong jsonl thấy Writer gọi novel_context nhiều lần trống, hoặc sau commit_chapter lại gọi thêm công cụ khác — đây là dấu hiệu prompt chưa kiểm soát được hành vi.

---

## 6. Ngưỡng đỏ cho truyện chạy dài

Khi chạy truyện 100+ chương, nếu bất kỳ điều nào dưới đây xảy ra hãy dừng lại để kiểm tra:

- [ ] CompassDrift kích hoạt và kéo dài qua 2 arc mà không được giải quyết
- [ ] Số mục cast_ledger > số chương đã hoàn thành × 0.8
- [ ] Tỷ lệ điền brief_role trong cast_ledger < 30%
- [ ] Cùng một nhân vật xuất hiện với tên nghi ngờ trùng ("Bác Lý" / "Lý chưởng quầy" cùng tồn tại)
- [ ] Writer viết chương mới mà không đọc nhân vật cũ đã có trong recent_cast (phát minh lại)
- [ ] Trong phiên Coordinator xuất hiện ≥ 5 lần gọi novel_context trống liên tiếp
- [ ] Sau khi commit bất kỳ chương nào, `meta/checkpoints.jsonl` không có step `commit_chapter` tương ứng

4 điều đầu là chỉ số sức khỏe của các cơ chế mới; 3 điều sau là sự ổn định của các cơ chế đã có.

---

## 7. Quy chuẩn bảo trì tài liệu

**Khi thêm artifact mới ở tầng dữ liệu (tạo `meta/*.json` / `meta/*.jsonl` mới), đồng thời:**

1. Thêm một dòng vào bảng tra cứu nhanh §2 trong tài liệu này
2. Nếu artifact cần quan sát chuyên biệt (không chỉ đơn giản là "có / không"), thêm mục §X riêng
3. Nếu muốn chẩn đoán tự động, tải vào `internal/diag/snapshot.go::Load` và thêm quy tắc trong `internal/diag/rules_*.go`

**Không nên:**
- Không sao chép toàn bộ quy tắc trong `internal/diag/` vào tài liệu này (đó là tài liệu tham chiếu quy tắc, không phải cẩm nang quan sát)
- Không viết quy tắc chẩn đoán cho mọi cơ chế — ngưỡng tùy tiện sẽ sai, hãy quan sát trước rồi mới bổ sung

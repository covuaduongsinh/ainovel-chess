# Mẫu lên kế hoạch dàn ý

Mẫu này không phải để ép mọi tác phẩm vào độ dài cố định, mà giúp trước tiên phán định cấp độ tác phẩm, rồi mới chọn độ chi tiết dàn ý.

## Bước 1: Phán định trước cấp độ độ dài tác phẩm

### Truyện ngắn / Cốt truyện đơn tập

- Áp dụng: xung đột đơn, mục tiêu đơn, ít nhân vật, kết cục tập trung
- Độ dài tham khảo: 8-25 chương
- Định dạng đề xuất: `outline` dẹt

### Truyện vừa / Cốt truyện đa giai đoạn

- Áp dụng: có thăng cấp giai đoạn, nhiều tuyến phụ, quan hệ nhân vật sẽ thay đổi
- Độ dài tham khảo: 25-60 chương
- Định dạng đề xuất: `outline` dẹt hoặc phân tầng nhẹ

### Truyện dài / Tiểu thuyết mạng kiểu kết cấu dài hơi

- Áp dụng: đề tài thiên nhiên có không gian thăng cấp liên tục, căng thẳng quan hệ dài hạn, nhiều mục tiêu giai đoạn, thế giới có thể mở rộng, bí ẩn dài hạn hoặc tuyến trưởng thành dài hạn
- Độ dài tham khảo: 80-200+ chương
- Định dạng đề xuất: `layered_outline` phân tầng

## Bước 2: Phán định có nhất thiết dùng dàn ý phân tầng không

Chỉ cần thỏa mãn bất kỳ 2 điều kiện nào dưới đây, ưu tiên dùng `layered_outline`:

- Thế giới quan cần triển khai dần, không thể kể hết một lần
- Nhân vật chính trưởng thành không phải một bước nhảy, mà là thăng cấp đa giai đoạn
- Quan hệ nhân vật sẽ liên tục thay đổi qua nhiều giai đoạn
- Giai đoạn giữa và cuối tồn tại các loại mâu thuẫn chính khác nhau
- Cần nhiều lần chuyển đổi bản đồ/thế lực/danh tính/mục tiêu
- Đề tài rõ ràng giống tiểu thuyết mạng kết cấu dài hơi thương mại, hơn là cốt truyện đơn tập

## Bước 3: Khi dài hơi, không làm thẳng "liệt kê chương toàn bộ"

Trình tự lên kế hoạch truyện dài đề xuất là:

1. Điểm bán và sự khác biệt của tác phẩm
2. Động cơ câu chuyện dài hạn
3. Chủ đề tập và thăng cấp
4. Mục tiêu cung và bước ngoặt giai đoạn
5. Sự kiện chương và móc câu

Cách làm sai:

- Viết trước khái quát 20 chương, rồi cố kéo dài
- Mỗi tập đều lặp lại "gặp địch-mạnh hơn-đổi bản đồ"
- Chỉ có thăng cấp tuyến chính, không có thăng cấp quan hệ
- Giai đoạn đầu tiêu hết mọi bí mật lớn, giai đoạn giữa sau chỉ có thể lặp khuôn

## Mẫu dàn ý dẹt (truyện ngắn/vừa)

```json
[
  {
    "chapter": 1,
    "title": "Tiêu đề chương",
    "core_event": "Sự kiện cốt lõi của chương này",
    "hook": "Móc câu cuối chương",
    "scenes": ["Cảnh 1", "Cảnh 2", "Cảnh 3"]
  }
]
```

## Mẫu dàn ý phân tầng (truyện dài — tập+cung triển khai cuộn theo hai tầng)

Lên kế hoạch ban đầu dùng triển khai cuộn hai tầng: 2 tập đầu có bộ khung cung, các tập còn lại là tập khung; cung đầu tiên có chương chi tiết.

```json
[
  {
    "index": 1,
    "title": "Tiêu đề tập một",
    "theme": "Mâu thuẫn cốt lõi/chủ đề mới thêm vào tập này",
    "arcs": [
      {
        "index": 1,
        "title": "Cung thứ nhất (đã triển khai)",
        "goal": "Mục tiêu cục bộ, sức cản và bước ngoặt",
        "chapters": [
          {"chapter": 1, "title": "Tiêu đề chương", "core_event": "Sự kiện cốt lõi", "hook": "Móc câu cuối chương", "scenes": ["Cảnh 1", "Cảnh 2"]}
        ]
      },
      {
        "index": 2,
        "title": "Cung thứ hai (cung khung)",
        "goal": "Khái quát mục tiêu cung này",
        "estimated_chapters": 12,
        "chapters": []
      }
    ]
  },
  {
    "index": 2,
    "title": "Tiêu đề tập hai",
    "theme": "Chủ đề tập hai",
    "arcs": [
      {"index": 1, "title": "Tiêu đề cung", "goal": "Mục tiêu cung", "estimated_chapters": 15, "chapters": []},
      {"index": 2, "title": "Tiêu đề cung", "goal": "Mục tiêu cung", "estimated_chapters": 10, "chapters": []}
    ]
  },
  {
    "index": 3,
    "title": "Tiêu đề tập ba (tập khung)",
    "theme": "Hướng chủ đề tập ba",
    "estimated_chapters": 60,
    "arcs": []
  }
]
```

- Triển khai cung: khi viết tiến đến cung khung, Architect triển khai chương chi tiết của cung đó
- Triển khai tập: khi viết tiến đến tập khung, Architect triển khai cấu trúc cung + chương đầu của tập đó

## Danh sách kiểm tra cấp tập truyện dài

Mỗi tập phải trả lời:

- Tập này thêm thông tin thế giới gì mới?
- Tập này nâng cấp mâu thuẫn cốt lõi gì?
- Tập này để nhân vật chính được gì, mất gì?
- Tập này thay đổi quan hệ nhân vật chính như thế nào?
- Sau khi kết tập, tại sao câu chuyện phải sang tập tiếp theo?

## Danh sách kiểm tra cấp cung truyện dài

Mỗi cung phải trả lời:

- Mục tiêu rõ ràng của cung này là gì?
- Sức cản đến từ ai, quy tắc nào, cái giá nào?
- Điểm bước ngoặt là gì?
- Sau khi kết cung, trạng thái nào đã thay đổi không thể đảo ngược?

## Danh sách kiểm tra cấp chương

- Mỗi chương phải phục vụ mục tiêu của cung đang ở trong
- Mỗi chương phải có ít nhất một sự kiện đẩy tiến không thể xóa
- Móc câu cần đa dạng, đừng chỉ dựa vào "phát hiện bí mật" một kiểu
- Các chương đầu không thể chỉ "giới thiệu thế giới," phải đồng thời đẩy tiến nhân vật và xung đột

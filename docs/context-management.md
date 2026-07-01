# Tài liệu quản lý ngữ cảnh

Tài liệu này giải thích hệ thống quản lý ngữ cảnh hiện tại của `ainovel-cli`, bao gồm:

- Tại sao cần quản lý ngữ cảnh
- Ngữ cảnh đến từ đâu
- Cách nén, khôi phục, chuyển giao trong runtime
- Giá trị, điều kiện kích hoạt và phạm vi áp dụng của mỗi chiến lược
- Khi có vấn đề nên xem xét ở đâu trước

Mục tiêu không phải giới thiệu khái niệm trừu tượng, mà để người bảo trì sau này mở tài liệu này lên có thể nhanh chóng hiểu triển khai hiện tại và điểm vào gỡ lỗi.

## 1. Mục tiêu thiết kế

Quản lý ngữ cảnh của dự án này không phải cho kịch bản trò chuyện thông thường, mà hướng tới kịch bản sáng tác tiểu thuyết. Nó cần giải quyết đồng thời nhiều loại vấn đề:

1. Hội thoại dài sẽ vượt quá cửa sổ ngữ cảnh của mô hình.
2. Sáng tác tiểu thuyết cần giữ lại không phải "lịch sử trò chuyện bản thân", mà là ký ức tường thuật có cấu trúc.
3. Sau khi nén, Writer không thể mất trạng thái nhân vật, phục bút, kế hoạch chương, ràng buộc phong cách, mục chờ sửa từ xét duyệt.
4. Khi khôi phục viết lách không thể giả định mô hình còn "nhớ đã nói gì trước đây", phải ưu tiên dựa vào artifact lưu trữ bền vững.

Do đó chúng tôi áp dụng phương án "bộ nhớ phân tầng":

- Bộ nhớ ngắn hạn: Phần đuôi tin nhắn gần nhất được giữ lại
- Bộ nhớ trung hạn: `ContextSummary` được tạo ra bởi nén
- Bộ nhớ dài hạn: Artifact có cấu trúc trong project store
- Bộ nhớ khôi phục: handoff / restore pack / novel_context

## 2. Kiến trúc tổng thể

### 2.1 Các tầng chính

Quản lý ngữ cảnh hiện tại được chia thành bốn tầng:

1. `agentcore/context`
   Phụ trách ngân sách ngữ cảnh chung, pipeline chiến lược, framework nén/khôi phục.

2. `internal/tools/novel_context`
   Phụ trách lắp ráp dữ liệu có cấu trúc từ project tiểu thuyết thành ngữ cảnh có thể dùng ở lượt hiện tại.

3. `internal/orchestrator/store_summary_*`
   Phụ trách nén nhanh dựa trên store chuyên dùng cho Writer.

4. `internal/orchestrator/writer_restore.go`
   Phụ trách nối thêm một gói khôi phục sau `FullSummary`, đảm bảo Writer có thể tiếp tục viết.

### 2.2 Luồng dữ liệu

Trong runtime có hai đường dẫn ngữ cảnh chính:

1. Đường dẫn làm việc bình thường
   - Agent gọi `novel_context`
   - `novel_context` đọc dữ liệu từ store như tóm tắt chương, kế hoạch, nhân vật, dòng thời gian v.v.
   - Dữ liệu này vào prompt lượt hiện tại

2. Đường dẫn ngữ cảnh quá dài
   - `ContextManager` phát hiện áp lực token
   - Nén theo thứ tự chiến lược
   - Ưu tiên thử nén nhẹ và nén dựa trên store
   - Chỉ khi không đủ mới đi qua LLM `FullSummary`
   - Sau `FullSummary` chèn restore pack

## 3. Tệp quan trọng

### 3.1 Engine ngữ cảnh chung

- `../agentcore/context/strategy.go`
- `../agentcore/context/engine.go`
- `../agentcore/context/strategy_tool.go`
- `../agentcore/context/strategy_trim.go`
- `../agentcore/context/strategy_summary.go`
- `../agentcore/context/message.go`
- `../agentcore/context/summary_run.go`

Tác dụng:

- Định nghĩa `Strategy` / `ForceCompactionStrategy`
- Phụ trách thực thi chuỗi chiến lược dựa trên ngân sách
- Phụ trách biểu diễn `ContextSummary` và chuyển đổi LLM
- Phụ trách nén tóm tắt LLM của `FullSummary`

### 3.2 Đấu dây phía project

- `internal/orchestrator/agents.go`

Tác dụng:

- Lắp ráp `ContextManager` của Writer / Coordinator
- Chèn thêm `StoreSummaryCompact` cho Writer
- Cấu hình prompt `FullSummary` tùy chỉnh cho tiểu thuyết cho Writer
- Cấu hình `writerRestorePack` cho Writer

### 3.3 Nén và khôi phục phía project

- `internal/orchestrator/store_summary_strategy.go`
- `internal/orchestrator/store_summary_builder.go`
- `internal/orchestrator/writer_restore.go`

Tác dụng:

- Trước khi tóm tắt LLM, ưu tiên dùng dữ liệu store để nén nhanh
- Thống nhất xây dựng ngữ cảnh có cấu trúc cần thiết cho nén và khôi phục Writer
- Sau `FullSummary` nối thêm một tin nhắn restore thuần bộ nhớ

### 3.4 Lắp ráp ngữ cảnh có cấu trúc

- `internal/tools/novel_context.go`
- `internal/tools/novel_context_builders.go`
- `internal/domain/runtime.go`

Tác dụng:

- Định nghĩa `ContextProfile` / `MemoryPolicy`
- Quyết định tải bao nhiêu tóm tắt chương, bao nhiêu dòng thời gian, có bật tóm tắt phân tầng không
- Lắp ráp chương, nhân vật, phục bút, dòng thời gian, kinh nghiệm xét duyệt v.v. từ store

### 3.5 Chuyển giao và khôi phục

- `internal/orchestrator/handoff_policy.go`
- `internal/orchestrator/recovery_engine.go`

Tác dụng:

- Trong giai đoạn dài/làm lại/xét duyệt, ưu tiên dựa vào handoff
- Khi khôi phục, ghép gói chuyển giao có cấu trúc vào prompt

### 3.6 Khả năng quan sát

- `internal/orchestrator/run.go`
- `internal/orchestrator/runtime.go`
- `internal/entry/tui/panels.go`

Tác dụng:

- Ghi sự kiện viết lại ngữ cảnh
- Xuất tên chiến lược, thay đổi token, số tin nhắn được giữ lại
- Để TUI có thể xem ngữ cảnh hiện tại là `projected` hay `compacted`

## 4. ContextManager được lắp ráp như thế nào

Writer và Coordinator đều đi qua `newContextManager`, nhưng cấu hình khác nhau.

Tham số quan trọng hiện tại của `contextManagerConfig`:

- `ContextWindow`
  Tổng cửa sổ ngữ cảnh của mô hình.

- `ReserveTokens`
  Token dự phòng cho đầu ra mô hình.

- `KeepRecentTokens`
  Ngân sách phần đuôi tin nhắn gần nhất được giữ lại khi nén.

- `ToolMicrocompact`
  Cấu hình siêu nén kết quả công cụ.

- `ExtraStrategies`
  Chiến lược nén thêm phía project. Hiện Writer dùng để gắn `StoreSummaryCompact`.

- `Summary`
  Cấu hình của `FullSummary`, bao gồm prompt tùy chỉnh và hook post-summary.

Giá trị cấu hình thực tế hiện tại:

| Tham số | Writer | Coordinator |
|------|--------|-------------|
| ReserveTokens | 16,384 | 32,000 |
| KeepRecentTokens | 20,000 | 30,000 |
| CommitOnProject | false | true |
| IdleThreshold | 5min | không có |
| ExtraStrategies | StoreSummaryCompact | không có |
| Summary Prompt tùy chỉnh | Phiên bản tường thuật tiểu thuyết | Mặc định (phiên bản trợ lý code) |

Ngưỡng kích hoạt nén = `ContextWindow - ReserveTokens`. Ví dụ cửa sổ 128K, Writer kích hoạt ở ~112K, Coordinator kích hoạt ở ~96K.

Thứ tự pipeline chiến lược hiện tại của Writer là:

1. `ToolResultMicrocompact`
2. `LightTrim`
3. `StoreSummaryCompact`
4. `FullSummary`

Thứ tự này có ý nghĩa rõ ràng:

- Dùng cách rẻ nhất để dọn nhiễu công cụ trước
- Cắt bỏ khối văn bản quá dài
- Nếu dữ liệu store đủ, trực tiếp thực hiện nén có cấu trúc không tốn LLM
- Cuối cùng mới quay lại tóm tắt LLM

## 5. Tác dụng của mỗi chiến lược

### 5.1 ToolResultMicrocompact

Vị trí triển khai:

- `../agentcore/context/strategy_tool.go`

Tác dụng:

- Dọn dẹp `tool_result` lịch sử
- Thay thế kết quả công cụ cũ bằng văn bản chiếm chỗ ngắn gọn

Giá trị:

- Nội dung trả về của công cụ thường có khối lượng lớn, mật độ thông tin thấp
- Nhiều kết quả công cụ cũ chỉ là "nhiễu quá trình", không phải ký ức tiểu thuyết

Đặc điểm cấu hình Writer hiện tại:

- Đặt `IdleThreshold = 5m`

Điều này có nghĩa:

- Nếu tin nhắn assistant gần nhất đã nhàn rỗi hơn ngưỡng
- Sẽ tích cực hơn trong việc giảm số lượng kết quả công cụ cũ được giữ lại

Phạm vi áp dụng:

- Nhiều lượt `novel_context`
- Sau nhiều lượt công cụ read / check / draft

### 5.2 LightTrim

Vị trí triển khai:

- `../agentcore/context/strategy_trim.go`

Tác dụng:

- Cắt bỏ khối văn bản rất dài
- Giữ lại phần đầu và đuôi, thay phần giữa bằng ký tự chiếm chỗ

Giá trị:

- Giữ nguyên cấu trúc tin nhắn
- Chi phí thấp
- Rất phù hợp xử lý nội dung chương cực dài hoặc đầu ra đoạn lớn

Phạm vi áp dụng:

- Một tin nhắn đơn quá dài, nhưng chưa cần làm summary toàn bộ lịch sử

### 5.3 StoreSummaryCompact

Vị trí triển khai:

- `internal/orchestrator/store_summary_strategy.go`
- `internal/orchestrator/store_summary_builder.go`

Tác dụng:

- Khi ngữ cảnh Writer quá dài
- Ưu tiên dùng ký ức có cấu trúc từ store lưu trữ bền vững để thay thế tin nhắn cũ
- Không gọi LLM

Đây không phải tóm tắt hội thoại, mà là "thay thế ký ức có cấu trúc".

Dữ liệu cốt lõi hiện tại được giữ lại bao gồm:

- Tiến độ hiện tại
- Tóm tắt chương gần nhất
- Kế hoạch chương hiện tại
- Dàn ý chương hiện tại
- Tóm tắt cung hiện tại
- Tóm tắt tập hiện tại
- Snapshot nhân vật
- Phục bút đang hoạt động
- Vấn đề xét duyệt chờ sửa
- Dòng thời gian gần nhất
- Quy tắc phong cách

Điều kiện tiền đề kích hoạt:

- Chương hiện tại lớn hơn 1
- Đã có đủ tóm tắt lịch sử trong store
- Và chương hiện tại có ít nhất dữ liệu trạng thái làm việc
  - `chapter_plan` hoặc `current_outline`

Giá trị:

- Giảm số lần nén LLM
- Tránh thông tin quan trọng của tiểu thuyết bị trôi dạt khi tóm tắt
- Để bộ nhớ dài hạn ưu tiên dựa vào sự kiện ghi xuống đĩa, không phải lịch sử trò chuyện

Tại sao chỉ dùng cho Writer:

- Đây là chiến lược nghiệp vụ tiểu thuyết, không phải chiến lược framework chung
- Chế độ ngữ cảnh của Coordinator / Editor khác nhau
- Xác thực trước trên Writer nơi cần ký ức sáng tác liên tục nhất là hợp lý nhất

### 5.4 FullSummary

Vị trí triển khai:

- `../agentcore/context/strategy_summary.go`
- `../agentcore/context/summary_run.go`

Tác dụng:

- Khi các tầng trên vẫn không đủ, dùng mô hình tạo `ContextSummary`
- Giữ lại phần đuôi tin nhắn gần nhất
- Biến ngữ cảnh cũ hơn thành checkpoint có cấu trúc

Điểm khác biệt của Writer so với trợ lý code mặc định:

- Writer dùng prompt tóm tắt tùy chỉnh
- Nội dung tóm tắt yêu cầu rõ ràng giữ lại:
  - Tiến độ hiện tại
  - Trạng thái tức thì của nhân vật
  - Phục bút và manh mối đang hoạt động
  - Phản hồi xét duyệt và vấn đề chờ sửa
  - Phong cách và nhịp điệu
  - Quyết định quan trọng
  - Bước tiếp theo
  - Ngữ cảnh quan trọng

Giá trị:

- Là chiến lược dự phòng cuối cùng
- Dù dữ liệu store không đủ, vẫn có thể duy trì tính liên tục qua LLM

### 5.5 Ngắt mạch (Circuit Breaker)

Vị trí triển khai:

- `../agentcore/context/engine.go`

Tác dụng:

- Khi nén liên tiếp thất bại đạt ngưỡng (mặc định 3 lần), bỏ qua nén lượt hiện tại
- Khi bỏ qua vẫn phát ra `RewriteEvent` (`Reason = "circuit_breaker"`)
- TUI hiển thị scope là "ngắt mạch bỏ qua"
- Áp dụng chế độ bán mở: bỏ qua một lượt rồi lượt sau sẽ thử lại, thành công thì reset, thất bại lại thì bỏ qua lại

Tại sao cần:

- Tóm tắt LLM có thể liên tục thất bại vì mạng, mô hình từ chối v.v.
- Không có ngắt mạch thì mỗi lượt Project sẽ thử và thất bại, lãng phí API call
- Trong phiên viết tác phẩm dài sự lãng phí này sẽ tích lũy

Gỡ lỗi:

- Nếu TUI liên tục hiển thị "ngắt mạch bỏ qua", có nghĩa đường dẫn tóm tắt LLM có vấn đề
- Kiểm tra sự kiện viết lại ngữ cảnh có `reason=circuit_breaker` trong slog
- Ngắt mạch không ảnh hưởng `StoreSummaryCompact` (nó không gọi LLM)

### 5.6 Ước tính token (nhận biết CJK)

Vị trí triển khai:

- `../agentcore/context/usage.go`

Tác dụng:

- Tất cả kiểm soát ngân sách, thời điểm kích hoạt nén đều phụ thuộc ước tính token
- `estimateTextTokens` tự động phát hiện văn bản có chủ yếu là ký tự CJK không
- Văn bản chủ yếu CJK: `runes × 1.5`
- Văn bản chủ yếu ASCII: `bytes / 4`

Tại sao không thể dùng `bytes/4` tiêu chuẩn:

- Tiếng Trung UTF-8 một chữ = 3 bytes
- `bytes/4` sẽ ước tính một chữ tiếng Trung là 0.75 token, thực tế khoảng 1.5 token
- Ước tính thấp gấp đôi sẽ khiến kích hoạt nén bị trễ nghiêm trọng

Phạm vi ảnh hưởng:

- `EstimateTokens` (một tin nhắn đơn)
- `EstimateTotal` (danh sách tin nhắn)
- `EstimateContextTokens` (ước tính hỗn hợp: LLM báo cáo Usage + ước tính tin nhắn đuôi)
- Cắt ngân sách trong `store_summary_builder.go`

Lưu ý: args của ToolCall là JSON (chủ yếu ASCII), vẫn dùng `bytes/4`, không bị ảnh hưởng điều chỉnh CJK.

## 6. Tại sao Writer có hai bộ "ký ức sau nén"

Hiện tại Writer có hai chuỗi trông giống nhau nhưng trách nhiệm khác nhau:

### 6.1 StoreSummaryCompact

Trách nhiệm:

- Trực tiếp thay thế tin nhắn cũ trong quá trình nén

Đặc điểm:

- Xảy ra trước `FullSummary`
- Không tốn LLM
- Dùng store thay thế lịch sử cũ hơn

### 6.2 writerRestorePack

Vị trí triển khai:

- `internal/orchestrator/writer_restore.go`

Trách nhiệm:

- Sau `FullSummary` nối thêm một tin nhắn restore

Đặc điểm:

- Xảy ra sau nén LLM
- Chèn qua `PostSummaryHook`
- Dùng để bổ sung thông tin có cấu trúc mà Writer bắt buộc phải thấy khi khôi phục tiếp tục sáng tác

Tại sao cả hai đều cần:

- `StoreSummaryCompact` không phải lúc nào cũng trúng
  - Ví dụ chương đầu tiên hoặc khi dữ liệu store không đủ
- `FullSummary` dù làm tốt đến đâu cũng có thể bỏ sót thông tin chính xác trong store
- Nên restore pack là lớp bảo hiểm cuối cùng

Hiện nay cả hai đã dùng chung `store_summary_builder.go`, tránh sai lệch khẩu kính.

## 7. Tác dụng của novel_context

Vị trí triển khai:

- `internal/tools/novel_context.go`
- `internal/tools/novel_context_builders.go`

`novel_context` không phải chiến lược nén, nó là "bộ lắp ráp ngữ cảnh có cấu trúc" trong runtime.

Nó chia dữ liệu trong store thành mấy loại:

- `working_memory`
  - Kế hoạch chương hiện tại
  - Dàn ý chương hiện tại
  - Tóm tắt chương gần nhất
  - Dòng thời gian
  - checkpoint
  - previous tail

- `episodic_memory`
  - Trạng thái nhân vật
  - Trạng thái quan hệ
  - Thay đổi trạng thái gần nhất
  - Phục bút

- `reference_pack`
  - Dữ liệu thiết lập và tham khảo ổn định hơn

- `selected_memory`
  - Ký ức quan trọng ít được chọn ra theo nhiệm vụ hiện tại

Giá trị:

- Nó quyết định ngữ cảnh tiểu thuyết có cấu trúc thực sự "đưa cho mô hình" mỗi lượt
- `StoreSummaryCompact` không gọi nó trực tiếp, nhưng dùng chung nguồn dữ liệu và tư duy lắp ráp tương tự

## 8. ContextProfile và MemoryPolicy

Vị trí triển khai:

- `internal/domain/runtime.go`

### 8.1 ContextProfile

Tác dụng:

- Quyết định kích thước cửa sổ tải theo tổng số chương

Quy tắc hiện tại:

- `<= 15` chương
  - `10` tóm tắt chương gần nhất
  - `10` dòng thời gian chương gần nhất

- `<= 50` chương
  - `5` tóm tắt chương gần nhất
  - `8` dòng thời gian chương gần nhất

- `> 50` chương
  - `3` tóm tắt chương gần nhất
  - `5` dòng thời gian chương gần nhất
  - Bật tóm tắt phân tầng

Giá trị:

- Kiểm soát quy mô ngữ cảnh
- Tránh nhồi toàn bộ lịch sử vào prompt khi tác phẩm dài

### 8.2 MemoryPolicy

Tác dụng:

- Viết rõ ràng chiến lược sử dụng ngữ cảnh hiện tại
- Cung cấp cho đầu ra `novel_context`
- Cung cấp cho logic handoff / reminder / chẩn đoán

Trường quan trọng:

- `SummaryWindow`
- `TimelineWindow`
- `LayeredSummaries`
- `SummaryStrategy`
- `HandoffPreferred`
- `ReadOnlyThreshold`

Giá trị:

- Biến "hệ thống hiện tại nên sử dụng ký ức như thế nào" từ logic ngầm thành chiến lược runtime tường minh

## 9. Tác dụng của handoff

Vị trí triển khai:

- `internal/orchestrator/handoff_policy.go`

Khi tác phẩm bước vào giai đoạn dài hơn, phức tạp hơn, phụ thuộc nhiều hơn vào artifact có cấu trúc, hệ thống sẽ thiên về handoff.

Gói handoff sẽ ghi lại:

- Giai đoạn và flow hiện tại
- Vị trí chương tiếp theo
- Commit gần nhất
- Xét duyệt gần nhất
- Tóm tắt gần nhất
- Memory policy hiện tại
- Hướng dẫn khôi phục

Giá trị:

- Khôi phục gián đoạn không phụ thuộc lịch sử trò chuyện
- Trong kịch bản làm lại, xét duyệt, tác phẩm dài ưu tiên dựa vào artifact có cấu trúc

## 10. Khả năng quan sát và gỡ lỗi

### 10.1 Sự kiện viết lại ngữ cảnh

Vị trí triển khai:

- `internal/orchestrator/run.go`

Mỗi lần viết lại ngữ cảnh sẽ xuất ra qua `contextRewriteCallback`:

- `reason`
- `strategy`
- `committed`
- `tokens_before`
- `tokens_after`
- `messages_before`
- `messages_after`
- `compacted_count`
- `kept_count`
- `split_turn`
- `incremental`
- `summary_runes`
- `duration_ms`

Điều này đồng thời vào:

- `slog`
- Hàng đợi runtime boundary
- Sự kiện `COMPACT` của TUI

### 10.2 Có thể thấy gì trong TUI

TUI sẽ hiển thị:

- Token ngữ cảnh hiện tại (với màu gradient theo mức độ sức khỏe)
- Context window
- Scope ngữ cảnh hiện tại (bao gồm "ngắt mạch bỏ qua")
- Tên chiến lược cuối cùng hiện tại
- Số lượng summary

Ý nghĩa màu sắc của tỷ lệ phần trăm ngữ cảnh (triển khai trong `internal/entry/tui/layout.go`):

| Màu sắc | Điều kiện | Ý nghĩa |
|------|------|------|
| Xanh lá | < 70% | Dồi dào, xa ngưỡng nén |
| Vàng | 70-85% | Gần ngưỡng nén |
| Đỏ | > 85% | Sắp hoặc đang nén |

Nhãn tiếng Việt của Scope:

| Scope | Hiển thị | Ý nghĩa |
|-------|------|------|
| baseline | Cơ sở | Trạng thái bình thường |
| projected | Chiếu | Xem trước nén tạm thời |
| compacted | Đã commit | Nén đã có hiệu lực |
| recovered | Khôi phục | Khôi phục sau tràn |
| skipped | Ngắt mạch bỏ qua | Nén bị ngắt mạch bỏ qua |

Giá trị:

- Có thể nhanh chóng đánh giá mức độ sức khỏe ngữ cảnh hiện tại
- Khi vàng/đỏ có thể dự kiến nén sắp xảy ra
- Thấy "ngắt mạch bỏ qua" có nghĩa đường dẫn tóm tắt LLM có vấn đề

### 10.3 Có vấn đề nên xem đâu trước

#### Tình huống 1: Writer mất kế hoạch chương sau khi nén

Xem trước:

- `novel_context` có ổn định chèn `chapter_plan` không
- `store_summary_builder.go` có lấy được `chapterPlan` không
- `writerRestorePack` có được làm mới không

Tệp trọng tâm:

- `internal/tools/novel_context_builders.go`
- `internal/orchestrator/store_summary_builder.go`
- `internal/orchestrator/session.go`

#### Tình huống 2: Mất trạng thái nhân vật/phục bút sau khi nén

Xem trước:

- `LoadLatestSnapshots`
- `LoadActiveForeshadow`
- `store_summary_builder.go`
- Writer summary prompt có bị ghi đè không

#### Tình huống 3: Nén thường xuyên nhưng luôn không trúng store_summary

Xem trước:

- Chương hiện tại có phải `<= 1` không
- Đã có recent summaries / arc / volume summary chưa
- Có tồn tại `chapter_plan` hay `current_outline` không
- `writer.Context.Strategy` cuối cùng ghi là `full_summary` không

#### Tình huống 4: Ngữ cảnh không đủ sau khi khôi phục

Xem trước:

- handoff có được tạo không
- restore pack có được làm mới không
- recovery prompt có chèn handoff không

#### Tình huống 5: Kết quả công cụ quá nhiều khiến ngữ cảnh phình to

Xem trước:

- `ToolResultMicrocompact` có trúng không
- `IdleThreshold` có có hiệu lực không

## 11. Đánh đổi trong triển khai hiện tại

### Đã kiên trì rõ ràng

1. Không nhồi logic nghiệp vụ tiểu thuyết vào `agentcore`
2. Ưu tiên dựa vào store có cấu trúc, không phải lịch sử trò chuyện
3. Writer dùng prompt tóm tắt tiểu thuyết chuyên dụng
4. Nén và khôi phục dùng chung builder nhất có thể, tránh sai lệch khẩu kính

### Giới hạn hiện đang cố ý giữ lại

1. `StoreSummaryCompact` chỉ dùng cho Writer
2. Chương đầu tiên không trúng store-based compact
3. Khi dữ liệu store không đủ vẫn rơi xuống `FullSummary`
4. `writerRestorePack` là bù đắp nối thêm, không thay thế `FullSummary`

Những giới hạn này không phải khuyết điểm, mà là ranh giới được đặt ra ở giai đoạn hiện tại để kiểm soát độ phức tạp.

## 12. Tóm tắt một câu

Quản lý ngữ cảnh của dự án này không đơn giản là "nén hội thoại dài thành ngắn", mà là:

`Ưu tiên dùng ký ức tiểu thuyết có cấu trúc để duy trì tính liên tục, khi cần thiết mới để LLM tóm tắt hội thoại; và trong ba khâu nén, khôi phục, chuyển giao đều cố gắng dựa vào cùng một bộ artifact lưu trữ bền vững.`

Nếu bạn sau này muốn sửa hệ thống này, hãy ưu tiên giữ vững ba điều sau:

1. Đừng để ký ức quan trọng của Writer lại chỉ phụ thuộc vào lịch sử trò chuyện.
2. Đừng để `store_summary` và `writer_restore` sai lệch khẩu kính.
3. Khi gặp vấn đề liên tục, hãy kiểm tra artifact có cấu trúc có vào ngữ cảnh không trước, rồi mới quyết định có sửa prompt không.

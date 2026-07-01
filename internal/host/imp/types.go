// Package imp thực hiện việc nhập và suy ngược các chương tiểu thuyết từ bên ngoài.
//
// Ý tưởng cốt lõi: dùng LLM suy ngược foundation + sự kiện từng chương, tái sử dụng
// bộ ba nguyên tử của công cụ save_foundation / commit_chapter để lưu trữ. Sau khi nhập xong,
// trạng thái store tương đương với "viết xong N chương rồi crash"; phía gọi chỉ cần gọi host.Resume()
// là có thể tiếp tục viết liền mạch.
//
// Không đi qua Coordinator: nhập là replay xác định, không thuộc phạm vi ra quyết định của LLM;
// để Coordinator can thiệp chỉ tạo thêm bất định. Gói này gọi trực tiếp LLM client + công cụ.
package imp

import "time"

// Chapter là một chương đơn sau khi phân tách.
type Chapter struct {
	Title   string
	Content string
}

// Options kiểm soát hành vi nhập.
type Options struct {
	// SourcePath bắt buộc. Đường dẫn tệp txt/md đơn lẻ.
	SourcePath string

	// ResumeFrom tùy chọn. Bắt đầu nhập từ chương N; 0 / 1 nghĩa là từ đầu.
	// Nếu > 1, sẽ bỏ qua việc suy ngược Foundation (coi như đã lưu rồi).
	ResumeFrom int
}

// Stage biểu thị giai đoạn hiện tại của luồng nhập.
type Stage string

const (
	StageSplitting  Stage = "splitting"
	StageFoundation Stage = "foundation"
	StageChapter    Stage = "chapter"
	StageDone       Stage = "done"
	StageError      Stage = "error"
)

// Event là sự kiện tiến độ được phát ra bên ngoài từ luồng nhập.
type Event struct {
	Time    time.Time
	Stage   Stage
	Current int    // số chương hiện tại ở giai đoạn chapter; bằng 0 ở các giai đoạn khác
	Total   int    // tổng số chương
	Message string // mô tả dễ đọc cho người dùng
	Err     error  // được đính kèm khi StageError
}

package rules

import (
	"os"
	"path/filepath"
)

// LoadOptions liệt kê các thư mục nguồn tệp rules để RawFileSources quét chuẩn hóa.
//
// Thư mục không tồn tại không tính là lỗi, bỏ qua lặng lẽ khi quét.
type LoadOptions struct {
	// HomeRulesDir là thư mục ~/.ainovel/rules/; quét tất cả .md cấp đầu (hợp nhất theo thứ tự từ điển tên tệp). Rỗng thì bỏ qua.
	HomeRulesDir string

	// ProjectRulesDir là thư mục ./.ainovel/rules/ (đối xứng toàn cục, cũng quét tất cả .md cấp đầu). Rỗng thì bỏ qua.
	ProjectRulesDir string
}

// ainovelDirName là tên dotdir dùng chung ở hai cấp user / project của ainovel.
// ~/.ainovel/rules/ toàn cục và ./.ainovel/rules/ dự án đối xứng nhau qua tên này.
const ainovelDirName = ".ainovel"

// DefaultProjectRulesDir ghép đường dẫn tuyệt đối của ./.ainovel/rules/ (dựa trên thư mục dự án được truyền vào).
// Bên gọi truyền vào gốc dự án, tránh phụ thuộc vào cwd bên trong loader; đối xứng với DefaultHomeRulesDir.
func DefaultProjectRulesDir(projectDir string) string {
	if projectDir == "" {
		return ""
	}
	return filepath.Join(projectDir, ainovelDirName, "rules")
}

// DefaultHomeRulesDir ghép đường dẫn tuyệt đối của thư mục ~/.ainovel/rules/.
// Nếu phân tích home thất bại trả về chuỗi rỗng (bên gọi bỏ qua nguồn đó).
func DefaultHomeRulesDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ainovelDirName, "rules")
}

// homeRulesReadme là hướng dẫn ghi vào ~/.ainovel/rules/README.txt khi khởi động lần đầu.
// Dùng đuôi .txt thay vì .md — quét chỉ nhận .md, tệp này sẽ không bị xem là quy tắc để chuẩn hóa.
const homeRulesReadme = `Đây là nơi đặt sở thích viết toàn cục, áp dụng cho tất cả các sách.

Tạo một tệp .md (ví dụ my-style.md), viết yêu cầu bằng ngôn ngữ thường ngày là được —
không cần bất kỳ định dạng nào, không cần YAML:

    # Nhân vật
    - Nhân vật chính đừng viết kiểu thánh mẫu, lạnh ngoài nóng trong là đủ
    # Phong cách
    - Dùng nhiều cảm nhận thân thể (ngón tay trắng bệch) thay nhãn cảm xúc (hồi hộp)
    - Hội thoại đừng quá sách vở, mỗi chương khoảng 3000 chữ
    - Không dùng kiểu diễn đạt AI như "ở một mức độ nào đó"

Viết xong không cần lo định dạng: hệ thống sẽ dùng mô hình chuẩn hóa các yêu cầu ngôn ngữ tự nhiên này
thành ràng buộc có cấu trúc (khoảng số chữ, từ cấm, ngưỡng từ mòn, v.v.), tự động tuân theo khi viết và tự kiểm khi commit.

Nhiều tệp .md được hợp nhất theo thứ tự từ điển tên tệp; tệp ẩn bắt đầu bằng dấu chấm và tệp không phải .md đều bị bỏ qua
(vì vậy README.txt này sẽ không bị xem là quy tắc).

Đường đáy cơ học về câu sáo AI phổ biến và từ mòn đã được tích hợp sẵn, dùng ngay không cần viết thêm.

Ưu tiên tải (cao → thấp): ./.ainovel/rules/*.md (sách này) > ~/.ainovel/rules/*.md (đây) > mặc định tích hợp
`

// EnsureHomeRulesDir cố gắng tạo thư mục ~/.ainovel/rules/ và ghi README.txt hướng dẫn,
// giúp người dùng khám phá điểm mở rộng sở thích toàn cục này và biết cách viết.
// nice-to-have, không phải đường dẫn quan trọng: lỗi phân tích home hoặc ghi tệp đều bị nuốt lặng lẽ, tuyệt đối không chặn khởi động.
func EnsureHomeRulesDir() {
	if dir := DefaultHomeRulesDir(); dir != "" {
		_ = ensureRulesDirAt(dir)
	}
}

// ensureRulesDirAt tạo thư mục và ghi README.txt theo mẫu hướng dẫn hiện tại, là phần lõi có thể kiểm thử của EnsureHomeRulesDir.
// README.txt là tệp hướng dẫn do hệ thống tạo ra (sở thích người dùng viết trong *.md, không bị quét tải),
// luôn được ghi đè bằng mẫu mới nhất — không giữ nội dung cũ, do đó không cần bất kỳ logic tương thích phiên bản nào.
func ensureRulesDirAt(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "README.txt"), []byte(homeRulesReadme), 0o644)
}

// DefaultOptions tạo LoadOptions thông dụng dựa trên thư mục làm việc hiện tại.
//
// Phù hợp để gọi một lần khi Host khởi động, cho dịch vụ quy tắc người dùng tái sử dụng cùng một cấu hình nguồn.
// Khi phân tích cwd thất bại, ProjectRulesDir để rỗng (quét sẽ bỏ qua nguồn đó).
//
// Ngữ nghĩa đường dẫn: ProjectRulesDir gắn với **thư mục làm việc hiện tại (cwd)** chứ không phải outputDir.
// Người dùng cd vào thư mục khác để viết sách khác, ./.ainovel/rules/ tự nhiên theo cwd;
// nếu muốn dùng chung cho nhiều sách, đặt vào thư mục toàn cục ~/.ainovel/rules/ (tất cả .md ở đó đều được tải).
func DefaultOptions() LoadOptions {
	cwd, _ := os.Getwd()
	return LoadOptions{
		HomeRulesDir:    DefaultHomeRulesDir(),
		ProjectRulesDir: DefaultProjectRulesDir(cwd),
	}
}

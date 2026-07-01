package rules

import (
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RawSource là một nguồn thô chờ chuẩn hóa (toàn bộ văn bản của tệp rules).
//
// Sau khi bỏ YAML, tệp rules chỉ là prompt ngôn ngữ tự nhiên thông thường; chuẩn hóa chỉ cần văn bản gốc, không phân tích front matter nữa.
type RawSource struct {
	Label string     // nhãn nguồn, đưa vào Snapshot.Sources (ví dụ global:my-style.md)
	Kind  SourceKind // cấp ưu tiên
	Text  string     // nội dung gốc của tệp
}

// RawFileSources liệt kê các tệp .md trong thư mục rules theo thứ tự Global → Project và trả về văn bản thô.
//
// Cùng quy ước quét với readDirFromDisk (.md cấp đầu, thứ tự từ điển, bỏ qua tệp ẩn), nhưng không phân tích YAML,
// toàn bộ văn bản được chuyển nguyên vẹn cho bộ chuẩn hóa. System defaults / startup prompt / yêu cầu lúc chạy do service cung cấp riêng.
func RawFileSources(opts LoadOptions) []RawSource {
	var out []RawSource
	out = append(out, rawDir(opts.HomeRulesDir, SourceGlobal)...)
	out = append(out, rawDir(opts.ProjectRulesDir, SourceProject)...)
	return out
}

func rawDir(dir string, kind SourceKind) []RawSource {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		// Thư mục không tồn tại là bình thường, bỏ qua lặng lẽ; nhưng lỗi quyền truy cập / đường dẫn thực ra là tệp phải để lại dấu vết —
		// nếu không người dùng viết quy tắc mà hoàn toàn không có hiệu lực, không phản hồi gì cả, chi phí truy vết rất cao (xem known_rules_path_stale_readme).
		if !os.IsNotExist(err) {
			slog.Warn("Đọc thư mục quy tắc thất bại, đã bỏ qua", "module", "rules", "dir", dir, "err", err)
		}
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	var out []RawSource
	for _, name := range names {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("Đọc tệp quy tắc thất bại, đã bỏ qua", "module", "rules", "file", path, "err", err)
			continue
		}
		text := strings.TrimSpace(string(data))
		if text == "" {
			continue
		}
		out = append(out, RawSource{
			Label: kind.String() + ":" + name,
			Kind:  kind,
			Text:  text,
		})
	}
	return out
}

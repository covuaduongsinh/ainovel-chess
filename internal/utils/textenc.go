package utils

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// DecodeText giải mã byte tệp văn bản do người dùng cung cấp sang UTF-8: khi UTF-8 không hợp lệ thì chuyển mã theo GB18030
// (tập cha của GBK) — tiểu thuyết tiếng Trung lưu hành trên mạng phần lớn là mã GBK, đọc thẳng như UTF-8
// sẽ ra toàn ký tự lạ. Các byte không phải GBK sẽ được bộ giải mã thay thế bằng U+FFFD (vốn đã là ký tự lạ, để lớp gọi
// báo lỗi không khớp hướng dẫn người dùng). Cuối cùng loại bỏ UTF-8 BOM (nếu không khớp đầu dòng sẽ mang theo nó).
func DecodeText(data []byte) string {
	if !utf8.Valid(data) {
		if decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(data); err == nil {
			data = decoded
		}
	}
	return strings.TrimPrefix(string(data), "\uFEFF")
}

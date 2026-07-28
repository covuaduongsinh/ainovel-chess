package comic

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// atomicWrite ghi nguyên tử (tmp + chmod + sync + rename), bản sao của adapt/io.go.
// Cố tình KHÔNG tái dùng store.IO vì OutDir có thể nằm ngoài store.Dir().
func atomicWrite(path string, data []byte) (int, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return 0, err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	n, err := tmp.Write(data)
	if err != nil {
		_ = tmp.Close()
		return 0, err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return 0, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	return n, os.Rename(tmpPath, path)
}

// exists kiểm tra file đã tồn tại chưa (để tôn trọng cờ Overwrite).
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// slug chuyển tên tiếng Việt thành tên file an toàn, giữ chữ/số Unicode
// (nên "Tí Tốt" → "tí-tốt", dấu vẫn còn — giống adapt/io.go).
func slug(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "khong-ten"
	}
	var b strings.Builder
	prevDash := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(unicode.ToLower(r))
			prevDash = false
		default:
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "khong-ten"
	}
	return out
}

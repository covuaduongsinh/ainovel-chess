package comic

import (
	"os"
	"path/filepath"
	"sort"
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

// readDirNames liệt kê tên tệp trong một thư mục, đã sắp xếp (best-effort).
func readDirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// readOut đọc một tệp đã ghi trong outDir.
func (rc *runCtx) readOut(rel string) ([]byte, error) {
	return os.ReadFile(rc.path(rel))
}

// globPages liệt kê mọi trang PNG đã dựng, sắp theo chương rồi theo số trang.
// Thứ tự này quyết định thứ tự trang trong PDF/CBZ/EPUB nên phải ổn định.
func (rc *runCtx) globPages() ([]string, error) {
	root := rc.path("trang")
	chapters, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, c := range chapters {
		if c.IsDir() {
			dirs = append(dirs, c.Name())
		}
	}
	sort.Strings(dirs)

	var out []string
	for _, d := range dirs {
		names, err := readDirNames(filepath.Join(root, d))
		if err != nil {
			continue
		}
		for _, n := range names {
			if strings.HasSuffix(n, ".png") {
				out = append(out, filepath.Join(root, d, n))
			}
		}
	}
	return out, nil
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

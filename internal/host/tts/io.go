package tts

import (
	"os"
	"path/filepath"
)

// atomicWrite ghi nguyên tử (tmp + chmod + fsync + rename), giống adapt/io.go:atomicWrite.
// Không tái dùng store.IO vì OutDir có thể nằm ngoài store.Dir().
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
	if err := os.Rename(tmpPath, path); err != nil {
		return 0, err
	}
	return n, nil
}

// exists kiểm tra file đã tồn tại chưa (để tôn trọng cờ Overwrite).
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// fileSize trả về kích thước tệp, 0 nếu không đọc được.
func fileSize(path string) int {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return int(fi.Size())
}

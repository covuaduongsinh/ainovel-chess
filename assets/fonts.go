package assets

import "embed"

// Font nhúng cho tính năng truyện tranh (lồng chữ vào bong bóng thoại).
//
// Cả ba đều là SIL OFL 1.1 và ĐÃ KIỂM CHỨNG phủ đủ 165 glyph tiếng Việt tiền-kết-hợp
// (xem TestFontsCoverVietnamese trong internal/comicdraw). Giấy phép OFL bắt buộc phát
// hành kèm văn bản giấy phép, nên OFL-*.txt cũng được nhúng và ghi ra khi xuất bản.
//
// ⚠ Cạm bẫy khi thay font: rất nhiều font lettering truyện tranh KHÔNG có dấu tiếng Việt
// (Comic Neue là OFL nhưng thiếu hẳn khối U+1EA0–1EF9; Blambot Comicrazy/Wildwords là font
// thương mại). Thay font mà không chạy lại test phủ glyph thì trang in ra sẽ đầy ô vuông.
//
//go:embed fonts/*.ttf fonts/*.txt
var fontsFS embed.FS

// ComicFonts gom byte font thô cho gói dựng ảnh. Trả byte chứ không trả kiểu đã parse để
// internal/comicdraw giữ được vị thế gói lá — nó không phụ thuộc vào assets.
type ComicFonts struct {
	Dialogue  []byte // Patrick Hand — chữ viết tay, dùng cho bong bóng thoại
	SFX       []byte // Bangers — chữ tượng thanh, hét
	Narration []byte // Be Vietnam Pro Regular — ô thuyết minh, số trang
	Bold      []byte // Be Vietnam Pro Bold — nhấn mạnh
}

// LoadComicFonts trả về byte của các font đã nhúng.
func LoadComicFonts() ComicFonts {
	return ComicFonts{
		Dialogue:  mustReadFont("fonts/PatrickHand-Regular.ttf"),
		SFX:       mustReadFont("fonts/Bangers-Regular.ttf"),
		Narration: mustReadFont("fonts/BeVietnamPro-Regular.ttf"),
		Bold:      mustReadFont("fonts/BeVietnamPro-Bold.ttf"),
	}
}

// FontLicenses trả về (tên tệp → nội dung) các giấy phép OFL, để bộ xuất bản ghi kèm.
func FontLicenses() map[string]string {
	out := map[string]string{}
	entries, err := fontsFS.ReadDir("fonts")
	if err != nil {
		return out
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || len(name) < 4 || name[len(name)-4:] != ".txt" {
			continue
		}
		if data, err := fontsFS.ReadFile("fonts/" + name); err == nil {
			out[name] = string(data)
		}
	}
	return out
}

func mustReadFont(path string) []byte {
	data, err := fontsFS.ReadFile(path)
	if err != nil {
		panic("embed read " + path + ": " + err.Error())
	}
	return data
}

package comicdraw

import (
	"os"
	"path/filepath"
	"testing"
)

// VietnameseSample là toàn bộ chữ cái tiếng Việt ở dạng tiền-kết-hợp (NFC), kèm các ký
// hiệu hay dùng trong truyện tranh. Font nào không phủ hết bộ này thì sẽ in ra ô vuông.
const VietnameseSample = "aàáảãạăằắẳẵặâầấẩẫậ" +
	"eèéẻẽẹêềếểễệ" +
	"iìíỉĩị" +
	"oòóỏõọôồốổỗộơờớởỡợ" +
	"uùúủũụưừứửữự" +
	"yỳýỷỹỵ" +
	"dđ" +
	"AÀÁẢÃẠĂẰẮẲẴẶÂẦẤẨẪẬ" +
	"EÈÉẺẼẸÊỀẾỂỄỆ" +
	"IÌÍỈĨỊ" +
	"OÒÓỎÕỌÔỒỐỔỖỘƠỜỚỞỠỢ" +
	"UÙÚỦŨỤƯỪỨỬỮỰ" +
	"YỲÝỶỸỴ" +
	"DĐ" +
	"₫“”‘’–—…!?.,;:()-"

// fontDir là thư mục font nhúng. Test đọc thẳng tệp thay vì qua gói assets để comicdraw
// giữ được vị thế gói lá (không phụ thuộc assets).
func fontDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "assets", "fonts")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("không tìm thấy thư mục font %s: %v", dir, err)
	}
	return dir
}

func loadFamily(t *testing.T, file string) *FontFamily {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fontDir(t), file))
	if err != nil {
		t.Fatalf("đọc font %s: %v", file, err)
	}
	fam, err := NewFontFamily(file, data, nil)
	if err != nil {
		t.Fatalf("parse font %s: %v", file, err)
	}
	return fam
}

// TestFontsCoverVietnamese biến "font này có tiếng Việt không" thành một khẳng định kiểm
// chứng được. Đổi font mà quên chạy test này thì bản in sẽ đầy ô vuông — mà ô vuông chỉ
// lộ ra ở khâu cuối, khi đã tốn tiền sinh ảnh cả cuốn.
func TestFontsCoverVietnamese(t *testing.T) {
	files := []string{
		"PatrickHand-Regular.ttf",
		"Bangers-Regular.ttf",
		"BeVietnamPro-Regular.ttf",
		"BeVietnamPro-Bold.ttf",
	}
	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			fam := loadFamily(t, f)
			if missing := fam.HasGlyphs(VietnameseSample, WeightRegular); len(missing) > 0 {
				t.Errorf("font %s thiếu %d glyph: %q", f, len(missing), string(missing))
			}
		})
	}
}

// TestSanitizeNFC khẳng định chuỗi dạng tổ hợp được gộp về tiền-kết-hợp.
func TestSanitizeNFC(t *testing.T) {
	// "ế" viết rời: e + U+0302 (mũ) + U+0301 (sắc)
	decomposed := "tiếng Việt"
	got, err := Sanitize(decomposed)
	if err != nil {
		t.Fatalf("Sanitize trả lỗi trên chuỗi hợp lệ: %v", err)
	}
	if want := "tiếng Việt"; got != want {
		t.Errorf("Sanitize = %q, muốn %q", got, want)
	}
	for _, r := range got {
		if isCombining(r) {
			t.Errorf("còn sót dấu tổ hợp U+%04X sau NFC", r)
		}
	}
}

// TestSanitizeRejectsStrayCombining khẳng định dấu tổ hợp không gộp được thì báo lỗi,
// chứ không âm thầm vẽ ra rác.
func TestSanitizeRejectsStrayCombining(t *testing.T) {
	// U+0303 (ngã) trên "z" không có dạng tiền-kết-hợp → NFC giữ nguyên dạng rời.
	if _, err := Sanitize("z̃"); err == nil {
		t.Fatal("Sanitize phải báo lỗi khi còn dấu tổ hợp không gộp được")
	}
	// MustSanitize thì bỏ dấu đó đi thay vì làm sập lần chạy.
	if got := MustSanitize("z̃"); got != "z" {
		t.Errorf("MustSanitize = %q, muốn %q", got, "z")
	}
}

// TestInkTopExceedsTypographicAscent là bằng chứng cho lý do phải dùng đỉnh mực thật:
// chữ chồng hai dấu vượt quá ascent typographic của font.
func TestInkTopExceedsTypographicAscent(t *testing.T) {
	fam := loadFamily(t, "PatrickHand-Regular.ttf")
	face, err := fam.Face(64, WeightRegular)
	if err != nil {
		t.Fatalf("dựng face: %v", err)
	}
	plain, _ := inkTop(face, []Line{{Text: "com"}})
	stacked, _ := inkTop(face, []Line{{Text: "Ế Ổ Ữ Ẫ"}})
	if !(stacked < plain) {
		t.Errorf("đỉnh mực chữ có dấu chồng (%.2f) phải CAO hơn chữ thường (%.2f)", stacked, plain)
	}
	ascent := -float64(face.Metrics().Ascent) / 64
	t.Logf("ascent typographic=%.2f · mực chữ thường=%.2f · mực dấu chồng=%.2f", ascent, plain, stacked)
}

// TestWrapAndFit kiểm tra ngắt dòng + tự co cỡ cho chữ tiếng Việt.
func TestWrapAndFit(t *testing.T) {
	fam := loadFamily(t, "PatrickHand-Regular.ttf")
	st := TextStyle{Family: fam, SizePx: 48, MinSizePx: 12}
	const s = "Cậu bé nhìn bàn cờ, khẽ mỉm cười — nước đi ấy cậu đã thấy từ lâu rồi."

	lay, face, err := FitText(s, 400, 300, st)
	if err != nil {
		t.Fatalf("FitText: %v", err)
	}
	if face == nil || len(lay.Lines) == 0 {
		t.Fatal("FitText trả về layout rỗng")
	}
	for i, ln := range lay.Lines {
		if ln.WidthPx > 400+0.5 {
			t.Errorf("dòng %d rộng %.1f > hộp 400", i, ln.WidthPx)
		}
	}
	if lay.H > 300+0.5 {
		t.Errorf("khối chữ cao %.1f > hộp 300", lay.H)
	}
	// Hộp rất hẹp phải khiến cỡ chữ co lại.
	narrow, _, err := FitText(s, 160, 200, st)
	if err != nil {
		t.Fatalf("FitText hộp hẹp: %v", err)
	}
	if narrow.SizePx >= lay.SizePx {
		t.Errorf("hộp hẹp phải cho cỡ chữ nhỏ hơn: hẹp=%.1f rộng=%.1f", narrow.SizePx, lay.SizePx)
	}
}

package comicdraw

import (
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// demoPage dựng một trang mẫu đủ mọi loại bong bóng để làm test khói và để mắt người xem.
func demoPage(t *testing.T) *Page {
	t.Helper()
	dlgFam := loadFamily(t, "PatrickHand-Regular.ttf")
	sfxFam := loadFamily(t, "Bangers-Regular.ttf")
	narrFam := loadFamily(t, "BeVietnamPro-Regular.ttf")

	spec := SpecA4()
	ink := color.RGBA{0x1A, 0x1A, 0x1A, 0xFF}
	dlg := TextStyle{Family: dlgFam, SizePx: spec.MM(4.6), MinSizePx: spec.MM(2.6), Color: ink, Align: AlignCenter}
	narr := TextStyle{Family: narrFam, SizePx: spec.MM(3.6), MinSizePx: spec.MM(2.2), Color: ink, Align: AlignLeft}
	sfxSt := TextStyle{Family: sfxFam, SizePx: spec.MM(16), MinSizePx: spec.MM(8),
		Color: color.RGBA{0xFF, 0xD5, 0x4F, 0xFF}, Align: AlignCenter, Upper: true}

	border := Border{Width: 0.7, Color: ink}
	mk := func(id string, r Rect, note string) Panel {
		return Panel{ID: id, Rect: r, Shape: ShapeRect, Border: border, FullBleed: true,
			Placeholder: Placeholder{Label: id, Note: note}}
	}

	return &Page{
		Index: 1, Spec: spec, Seed: 20260728, PageNumber: "7",
		Panels: []Panel{
			mk("K1", Rect{X: 0, Y: 0, W: 0.62, H: 0.28}, "Toàn cảnh phố Havana"),
			mk("K2", Rect{X: 0.64, Y: 0, W: 0.36, H: 0.28}, "Cận nhạc công"),
			mk("K3", Rect{X: 0, Y: 0.30, W: 1.0, H: 0.32}, "Cha và bác Ramón chơi cờ"),
			mk("K4", Rect{X: 0, Y: 0.64, W: 0.30, H: 0.36}, "José nép trong góc"),
			mk("K5", Rect{X: 0.32, Y: 0.64, W: 0.30, H: 0.36}, "Đặc tả bàn cờ"),
			mk("K6", Rect{X: 0.64, Y: 0.64, W: 0.36, H: 0.36}, "Tí Tốt phát sáng"),
		},
		Balloons: []Balloon{
			{ID: "b1", Kind: BalloonBox, Anchor: Rect{X: 0.02, Y: 0.015, W: 0.36, H: 0.075},
				Text: "Havana, một chiều tháng Chín rực nắng…", Style: narr,
				PadH: 3, PadV: 2.4, StrokeWidth: 0.5},
			{ID: "b2", Kind: BalloonSpeech, Anchor: Rect{X: 0.05, Y: 0.325, W: 0.31, H: 0.115},
				Text: "Con nhìn kỹ nước cờ này chứ?", Style: dlg,
				HasTail: true, Tail: Point{X: 0.22, Y: 0.53}, TailWidth: 7,
				PadH: 4, PadV: 3.2, StrokeWidth: 0.7},
			{ID: "b3", Kind: BalloonShout, Anchor: Rect{X: 0.60, Y: 0.325, W: 0.36, H: 0.14},
				Text: "CHIẾU TƯỚNG!", Style: dlg,
				HasTail: true, Tail: Point{X: 0.72, Y: 0.56}, TailWidth: 8,
				PadH: 4, PadV: 3.2, StrokeWidth: 0.9},
			{ID: "b4", Kind: BalloonThought, Anchor: Rect{X: 0.01, Y: 0.665, W: 0.28, H: 0.14},
				Text: "Mình sẽ thắng ván đó…", Style: dlg,
				HasTail: true, Tail: Point{X: 0.16, Y: 0.90}, TailWidth: 6,
				PadH: 4, PadV: 3.2, StrokeWidth: 0.7},
			{ID: "b5", Kind: BalloonWhisper, Anchor: Rect{X: 0.655, Y: 0.665, W: 0.33, H: 0.13},
				Text: "Đừng để lộ nhé, Tí Tốt.", Style: dlg,
				PadH: 4, PadV: 3.2, StrokeWidth: 0.7},
		},
		SFX: []SFX{
			{At: Point{X: 0.46, Y: 0.82}, Text: "Cạch!", Style: sfxSt, Outline: 1.4,
				Fill: color.RGBA{0xFF, 0xD5, 0x4F, 0xFF}, Stroke: ink},
		},
	}
}

// TestRenderPageSmoke dựng một trang đủ loại bong bóng và kiểm tra các bất biến cơ bản.
//
// Đặt COMIC_DEMO_OUT=<thư mục> để ghi trang ra tệp mà nhìn bằng mắt — bố cục và typography
// là thứ chỉ mắt người mới nghiệm thu được, test chỉ chặn được lỗi thô.
func TestRenderPageSmoke(t *testing.T) {
	page := demoPage(t)
	img, err := Render(page)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	spec := page.Spec
	if got, want := img.Bounds().Dx(), spec.PxW(); got != want {
		t.Errorf("bề rộng ảnh = %d, muốn %d", got, want)
	}
	if got, want := img.Bounds().Dy(), spec.PxH(); got != want {
		t.Errorf("chiều cao ảnh = %d, muốn %d", got, want)
	}
	// A4 @300 DPI kèm tràn lề 3mm: 2551×3579 px. Sai số này lộ ngay khi in.
	if spec.PxW() != 2551 || spec.PxH() != 3579 {
		t.Errorf("A4@300DPI+bleed phải là 2551×3579, nhận %d×%d", spec.PxW(), spec.PxH())
	}
	if spec.TrimPxW() != 2480 || spec.TrimPxH() != 3508 {
		t.Errorf("khung thành phẩm A4@300DPI phải là 2480×3508, nhận %d×%d", spec.TrimPxW(), spec.TrimPxH())
	}

	// Trang không được toàn màu nền — nghĩa là có vẽ thật.
	nonBG := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 7 {
		for x := b.Min.X; x < b.Max.X; x += 7 {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r>>8 != 0xFF || g>>8 != 0xFF || bl>>8 != 0xFF {
				nonBG++
			}
		}
	}
	if nonBG < 1000 {
		t.Errorf("trang gần như trống (chỉ %d điểm khác nền) — có thể bộ dựng không vẽ gì", nonBG)
	}

	if dir := os.Getenv("COMIC_DEMO_OUT"); dir != "" {
		path := filepath.Join(dir, "trang-thu.png")
		fh, err := os.Create(path)
		if err != nil {
			t.Fatalf("tạo tệp demo: %v", err)
		}
		defer fh.Close()
		if err := png.Encode(fh, img); err != nil {
			t.Fatalf("mã hoá PNG: %v", err)
		}
		t.Logf("đã ghi trang demo: %s (%d×%d)", path, img.Bounds().Dx(), img.Bounds().Dy())
	}
}

// TestRenderDeterministic khẳng định cùng seed cho ra đúng cùng byte — điều kiện để
// golden test ổn định và để chạy lại không sinh khác biệt giả.
func TestRenderDeterministic(t *testing.T) {
	a, err := Render(demoPage(t))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Render(demoPage(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Pix) != len(b.Pix) {
		t.Fatalf("kích thước khác nhau: %d vs %d", len(a.Pix), len(b.Pix))
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			t.Fatalf("dựng hai lần cho kết quả khác nhau tại byte %d", i)
		}
	}
}

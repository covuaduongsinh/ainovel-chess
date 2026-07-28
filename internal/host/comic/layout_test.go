package comic

import (
	"fmt"
	"math"
	"testing"

	"github.com/voocel/ainovel-cli/internal/comicdraw"
)

func mkPanels(sizes ...string) []Panel {
	out := make([]Panel, len(sizes))
	for i, s := range sizes {
		out[i] = Panel{PanelNo: i + 1, Size: s, Description: fmt.Sprintf("khung %d", i+1)}
	}
	return out
}

func overlap(a, b comicdraw.Rect) bool {
	const eps = 1e-9
	return a.X < b.X+b.W-eps && b.X < a.X+a.W-eps &&
		a.Y < b.Y+b.H-eps && b.Y < a.Y+a.H-eps
}

// TestLayoutTilesPage khẳng định BẤT BIẾN then chốt: khung không chồng lấn, nằm gọn trong
// trang, và lát kín trang theo cả hai chiều (trừ đúng phần rãnh).
//
// Đây chính là thứ không thể bảo đảm nếu để LLM tự sinh toạ độ — và cũng là lỗi khó phát
// hiện nhất, vì nó chỉ lộ ra khi nhìn trang đã in.
func TestLayoutTilesPage(t *testing.T) {
	spec := comicdraw.SpecA4()
	gx, gy := gutterNorm(spec)

	cases := [][]string{
		{"vua", "vua"},
		{"lon", "nho", "nho"},
		{"nho", "nho", "nho", "vua", "vua", "lon"},
		{"tran-trang"},
		{"lon", "tran-trang", "nho", "nho"},
		{"vua"},
		{"nho", "nho", "nho", "nho", "nho", "nho", "nho", "nho"},
	}

	for ci, sizes := range cases {
		t.Run(fmt.Sprintf("case%d_%dkhung", ci, len(sizes)), func(t *testing.T) {
			page := PageSpec{PageNo: 1, Panels: mkPanels(sizes...)}
			lay := ComputeLayout(spec, page)

			if len(lay.Boxes) != len(sizes) {
				t.Fatalf("số hộp = %d, muốn %d", len(lay.Boxes), len(sizes))
			}

			// 1) Nằm gọn trong trang.
			for i, b := range lay.Boxes {
				if b.W <= 0 || b.H <= 0 {
					t.Errorf("khung %d rỗng: %+v", i, b)
				}
				if b.X < -1e-9 || b.Y < -1e-9 || b.X+b.W > 1+1e-9 || b.Y+b.H > 1+1e-9 {
					t.Errorf("khung %d tràn ra ngoài trang: %+v", i, b)
				}
			}

			// 2) Không chồng lấn.
			for i := range lay.Boxes {
				for j := i + 1; j < len(lay.Boxes); j++ {
					if overlap(lay.Boxes[i], lay.Boxes[j]) {
						t.Errorf("khung %d và %d chồng lấn: %+v / %+v", i, j, lay.Boxes[i], lay.Boxes[j])
					}
				}
			}

			// 3) Lát kín theo chiều dọc: hàng đầu chạm mép trên, hàng cuối chạm mép dưới.
			minY, maxY := math.Inf(1), math.Inf(-1)
			for _, b := range lay.Boxes {
				minY = math.Min(minY, b.Y)
				maxY = math.Max(maxY, b.Y+b.H)
			}
			if math.Abs(minY) > 1e-9 {
				t.Errorf("hàng đầu không chạm mép trên: y=%.6f", minY)
			}
			if math.Abs(maxY-1) > 1e-9 {
				t.Errorf("hàng cuối không chạm mép dưới: y=%.6f", maxY)
			}

			// 4) Lát kín theo chiều ngang trong từng hàng.
			rows := groupRows(page.Panels)
			for ri, row := range rows {
				minX, maxX := math.Inf(1), math.Inf(-1)
				for _, i := range row {
					minX = math.Min(minX, lay.Boxes[i].X)
					maxX = math.Max(maxX, lay.Boxes[i].X+lay.Boxes[i].W)
				}
				if math.Abs(minX) > 1e-9 || math.Abs(maxX-1) > 1e-9 {
					t.Errorf("hàng %d không lát kín ngang: [%.6f, %.6f]", ri, minX, maxX)
				}
			}

			// 5) Rãnh giữa các hàng đúng bằng gutter.
			if len(rows) > 1 {
				for ri := 0; ri+1 < len(rows); ri++ {
					bottom := lay.Boxes[rows[ri][0]].Y + lay.Boxes[rows[ri][0]].H
					top := lay.Boxes[rows[ri+1][0]].Y
					if math.Abs(top-bottom-gy) > 1e-9 {
						t.Errorf("rãnh dọc giữa hàng %d và %d = %.6f, muốn %.6f", ri, ri+1, top-bottom, gy)
					}
				}
			}
			_ = gx
		})
	}
}

// TestLayoutRespectsSize khẳng định khung "lon" thật sự lớn hơn khung "nho" cùng hàng.
func TestLayoutRespectsSize(t *testing.T) {
	spec := comicdraw.SpecA4()
	page := PageSpec{PageNo: 1, Panels: mkPanels("lon", "nho")}
	lay := ComputeLayout(spec, page)
	if !(lay.Boxes[0].W > lay.Boxes[1].W*1.5) {
		t.Errorf("khung lớn (%.4f) phải rộng hơn hẳn khung nhỏ (%.4f)", lay.Boxes[0].W, lay.Boxes[1].W)
	}
}

// TestLayoutRowBreak khẳng định cờ xuống hàng của LLM được tôn trọng.
func TestLayoutRowBreak(t *testing.T) {
	spec := comicdraw.SpecA4()
	panels := mkPanels("nho", "nho", "nho", "nho")
	panels[0].RowBreak = true // ép khung 1 đứng riêng một hàng
	page := PageSpec{PageNo: 1, Panels: panels}

	rows := groupRows(page.Panels)
	if len(rows) < 2 || len(rows[0]) != 1 {
		t.Fatalf("cờ RowBreak không được tôn trọng, các hàng = %v", rows)
	}
	lay := ComputeLayout(spec, page)
	if math.Abs(lay.Boxes[0].W-1) > 1e-9 {
		t.Errorf("khung đứng riêng hàng phải rộng hết trang, nhận %.6f", lay.Boxes[0].W)
	}
}

// TestAnchorsInsidePanels khẳng định vùng đặt bong bóng luôn nằm gọn trong khung của nó —
// nếu không, bong bóng sẽ tràn sang khung bên cạnh và làm hỏng thứ tự đọc.
func TestAnchorsInsidePanels(t *testing.T) {
	spec := comicdraw.SpecA4()
	panels := mkPanels("vua", "vua", "lon")
	panels[0].Balloons = []Balloon{
		{Order: 0, Kind: "thoai", Text: "Chào cậu!", Anchor: "tren-trai"},
		{Order: 1, Kind: "thoai", Text: "Lâu rồi không gặp.", Anchor: "duoi-phai"},
	}
	panels[2].Balloons = []Balloon{{Order: 0, Kind: "thuyet-minh", Text: "Ngày hôm sau…", Anchor: "tren-giua"}}
	page := PageSpec{PageNo: 1, Panels: panels}

	lay := ComputeLayout(spec, page)
	if len(lay.Anchor) != 3 {
		t.Fatalf("số vùng neo = %d, muốn 3", len(lay.Anchor))
	}
	for _, a := range lay.Anchor {
		box := lay.Boxes[a.PanelIndex]
		r := a.Rect
		if r.X < box.X-1e-9 || r.Y < box.Y-1e-9 ||
			r.X+r.W > box.X+box.W+1e-9 || r.Y+r.H > box.Y+box.H+1e-9 {
			t.Errorf("vùng neo %+v tràn khỏi khung %+v", r, box)
		}
	}
	// Ô thuyết minh không có đuôi.
	if lay.Anchor[2].HasTail {
		t.Error("ô thuyết minh không được có đuôi")
	}
	if !lay.Anchor[0].HasTail {
		t.Error("bong bóng thoại phải có đuôi")
	}
}

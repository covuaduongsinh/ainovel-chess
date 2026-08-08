package comic

import (
	"github.com/voocel/ainovel-cli/internal/comicdraw"
)

// Bố cục trang được tính XÁC ĐỊNH bằng Go từ phần NGỮ NGHĨA do LLM sinh ra
// (số khung, cỡ mỗi khung, chỗ xuống hàng), chứ KHÔNG để LLM tự sinh toạ độ.
//
// Lý do: LLM sinh toạ độ thì khung chồng nhau, hở lỗ, không lát kín trang — mà lỗi loại
// này chỉ lộ ra ở khâu cuối và rất khó kiểm tự động. Tính bằng code thì lát kín là BẤT
// BIẾN chứng minh được (xem TestLayoutTilesPage).

// PageLayout là hình học một trang đã tính xong.
type PageLayout struct {
	PageNo int              `json:"page_no"`
	Boxes  []comicdraw.Rect `json:"boxes"`  // song ánh theo thứ tự với Panels
	Anchor []BalloonAnchor  `json:"anchor"` // vùng đặt bong bóng, theo thứ tự đọc
}

// BalloonAnchor là vùng cho phép đặt một bong bóng, kèm đích của đuôi.
type BalloonAnchor struct {
	PanelIndex int            `json:"panel_index"`
	Order      int            `json:"order"`
	Rect       comicdraw.Rect `json:"rect"`
	Tail       comicdraw.Point `json:"tail"`
	HasTail    bool           `json:"has_tail"`
}

// gutterNorm đổi rãnh từ mm sang đơn vị chuẩn hoá theo hai trục.
func gutterNorm(spec comicdraw.PageSpec) (gx, gy float64) {
	return spec.MM(spec.Gutter) / spec.MM(spec.TrimW), spec.MM(spec.Gutter) / spec.MM(spec.TrimH)
}

// rowBudget là tổng trọng số tối đa trên một hàng khi LLM không đánh dấu xuống hàng.
// 3.0 tương ứng khoảng 2–3 khung mỗi hàng — nhịp đọc quen thuộc của truyện tranh trang.
const rowBudget = 3.0

// groupRows gom khung thành các hàng. Ưu tiên cờ RowBreak của LLM; nếu LLM không đánh dấu
// cờ nào thì tự ngắt theo ngân sách trọng số.
func groupRows(panels []Panel) [][]int {
	if len(panels) == 0 {
		return nil
	}
	anyBreak := false
	for _, p := range panels {
		if p.RowBreak {
			anyBreak = true
			break
		}
	}

	var rows [][]int
	cur := []int{}
	acc := 0.0
	for i, p := range panels {
		w := panelSizeWeight(p.Size)
		if w == 0 { // tràn trang: đứng riêng một hàng
			if len(cur) > 0 {
				rows = append(rows, cur)
				cur, acc = []int{}, 0
			}
			rows = append(rows, []int{i})
			continue
		}
		// Ngắt TRƯỚC khi thêm nếu hàng hiện tại đã đầy (chỉ khi tự ngắt).
		if !anyBreak && len(cur) > 0 && acc+w > rowBudget {
			rows = append(rows, cur)
			cur, acc = []int{}, 0
		}
		cur = append(cur, i)
		acc += w
		if anyBreak && p.RowBreak {
			rows = append(rows, cur)
			cur, acc = []int{}, 0
		}
	}
	if len(cur) > 0 {
		rows = append(rows, cur)
	}
	return rows
}

// ComputeLayout tính hình học cho một trang.
//
// Bất biến bảo đảm: các khung không chồng lấn, và tổng chiều cao các hàng cộng rãnh đúng
// bằng chiều cao trang (lát kín theo chiều dọc); trong mỗi hàng, tổng bề rộng cộng rãnh
// đúng bằng bề rộng trang.
func ComputeLayout(spec comicdraw.PageSpec, page PageSpec2) PageLayout {
	gx, gy := gutterNorm(spec)
	rows := groupRows(page.Panels)
	out := PageLayout{PageNo: page.PageNo, Boxes: make([]comicdraw.Rect, len(page.Panels))}
	if len(rows) == 0 {
		return out
	}

	// Chiều cao mỗi hàng tỉ lệ với trọng số LỚN NHẤT trong hàng: một hàng có khung "lớn"
	// thì phải cao hơn hàng toàn khung "nhỏ", nếu chia đều thì cỡ khung mất hết ý nghĩa.
	rowWeights := make([]float64, len(rows))
	totalRowW := 0.0
	for ri, row := range rows {
		maxW := 0.0
		for _, i := range row {
			w := panelSizeWeight(page.Panels[i].Size)
			if w == 0 {
				w = 3.0 // tràn trang: hàng cao nhất
			}
			if w > maxW {
				maxW = w
			}
		}
		rowWeights[ri] = maxW
		totalRowW += maxW
	}

	availH := 1.0 - gy*float64(len(rows)-1)
	y := 0.0
	for ri, row := range rows {
		h := availH * rowWeights[ri] / totalRowW

		// Bề rộng trong hàng tỉ lệ với trọng số từng khung.
		sumW := 0.0
		for _, i := range row {
			w := panelSizeWeight(page.Panels[i].Size)
			if w == 0 {
				w = 1
			}
			sumW += w
		}
		availW := 1.0 - gx*float64(len(row)-1)
		x := 0.0
		for _, i := range row {
			w := panelSizeWeight(page.Panels[i].Size)
			if w == 0 {
				w = 1
			}
			bw := availW * w / sumW
			out.Boxes[i] = comicdraw.Rect{X: x, Y: y, W: bw, H: h}
			x += bw + gx
		}
		y += h + gy
	}

	out.Anchor = computeAnchors(page, out.Boxes)
	return out
}

// PageSpec2 là bí danh nội bộ cho PageSpec của model.go, đặt tên khác để không đụng
// comicdraw.PageSpec (khổ trang vật lý) — hai khái niệm hoàn toàn khác nhau.
type PageSpec2 = PageSpec

// anchorFractions quy đổi khoá vị trí sang toạ độ tương đối trong khung.
// Trả về (x, y) là TÂM vùng neo, tính theo tỉ lệ bề rộng/chiều cao khung.
func anchorFractions(key string) (fx, fy float64) {
	switch key {
	case "tren-trai":
		return 0.28, 0.18
	case "tren-giua":
		return 0.50, 0.16
	case "tren-phai":
		return 0.72, 0.18
	case "giua-trai":
		return 0.24, 0.50
	case "giua-phai":
		return 0.76, 0.50
	case "duoi-trai":
		return 0.28, 0.82
	case "duoi-giua":
		return 0.50, 0.84
	case "duoi-phai":
		return 0.72, 0.82
	default:
		return 0.50, 0.18
	}
}

// balloonBoxFrac là kích thước vùng neo mặc định, tính theo tỉ lệ khung.
const (
	balloonWFrac = 0.62
	balloonHFrac = 0.30
)

// computeAnchors đặt vùng cho từng bong bóng bên trong khung của nó.
//
// Khi một khung có nhiều bong bóng, chúng được xếp so le theo chiều dọc theo THỨ TỰ ĐỌC,
// để mắt người đọc đi đúng trình tự thay vì phải đoán.
func computeAnchors(page PageSpec, boxes []comicdraw.Rect) []BalloonAnchor {
	var out []BalloonAnchor
	for pi, panel := range page.Panels {
		if pi >= len(boxes) {
			break
		}
		box := boxes[pi]
		n := len(panel.Balloons)
		for bi, b := range panel.Balloons {
			fx, fy := anchorFractions(b.Anchor)
			// Nhiều bong bóng trong một khung: dàn đều theo chiều dọc nửa trên.
			if n > 1 {
				fy = 0.14 + 0.62*float64(bi)/float64(n-1)
				if bi%2 == 1 {
					fx = 1 - fx
				}
			}
			w := box.W * balloonWFrac
			h := box.H * balloonHFrac
			if n > 1 {
				h = box.H * balloonHFrac * 0.8
			}
			r := comicdraw.Rect{
				X: box.X + box.W*fx - w/2,
				Y: box.Y + box.H*fy - h/2,
				W: w, H: h,
			}
			r = clampRect(r, box)

			a := BalloonAnchor{PanelIndex: pi, Order: bi, Rect: r}
			// Ô thuyết minh không có đuôi; các loại khác trỏ về phía dưới-giữa khung
			// (nơi nhân vật thường đứng) trừ khi kịch bản chỉ định khác.
			if b.Kind != "thuyet-minh" {
				a.HasTail = true
				tfx, tfy := tailTarget(b.TailTo)
				a.Tail = comicdraw.Point{X: box.X + box.W*tfx, Y: box.Y + box.H*tfy}
			}
			out = append(out, a)
		}
	}
	return out
}

// tailTarget quy đổi hướng đuôi sang toạ độ tương đối trong khung.
func tailTarget(key string) (fx, fy float64) {
	switch key {
	case "trai":
		return 0.18, 0.72
	case "phai":
		return 0.82, 0.72
	case "tren":
		return 0.50, 0.14
	case "duoi-trai":
		return 0.28, 0.90
	case "duoi-phai":
		return 0.72, 0.90
	default:
		return 0.50, 0.86
	}
}

// clampRect ép một hình chữ nhật nằm gọn trong khung cha.
func clampRect(r, parent comicdraw.Rect) comicdraw.Rect {
	const pad = 0.012
	minX, minY := parent.X+parent.W*pad, parent.Y+parent.H*pad
	maxX, maxY := parent.X+parent.W*(1-pad), parent.Y+parent.H*(1-pad)
	if r.W > maxX-minX {
		r.W = maxX - minX
	}
	if r.H > maxY-minY {
		r.H = maxY - minY
	}
	if r.X < minX {
		r.X = minX
	}
	if r.Y < minY {
		r.Y = minY
	}
	if r.X+r.W > maxX {
		r.X = maxX - r.W
	}
	if r.Y+r.H > maxY {
		r.Y = maxY - r.H
	}
	return r
}

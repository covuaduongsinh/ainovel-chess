package comicdraw

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

	"golang.org/x/image/vector"
)

// Path là một đường bao gồm nhiều contour, toạ độ tính bằng pixel thiết bị.
//
// Đây là kiểu then chốt của cả gói: CÙNG một Path vừa đem tô raster (Fill) vừa đem xuất
// thuộc tính d= của SVG (D). Nhờ vậy bản PNG và bản SVG không thể lệch nhau — đó là lỗi
// kinh điển của các bộ dựng hai đầu ra.
type Path struct {
	segs []pathSeg
}

type pathSeg struct {
	op   byte // 'M' 'L' 'Q' 'C' 'Z'
	args [6]float64
}

func (p *Path) MoveTo(x, y float64) *Path {
	p.segs = append(p.segs, pathSeg{op: 'M', args: [6]float64{x, y}})
	return p
}

func (p *Path) LineTo(x, y float64) *Path {
	p.segs = append(p.segs, pathSeg{op: 'L', args: [6]float64{x, y}})
	return p
}

func (p *Path) QuadTo(cx, cy, x, y float64) *Path {
	p.segs = append(p.segs, pathSeg{op: 'Q', args: [6]float64{cx, cy, x, y}})
	return p
}

func (p *Path) CubeTo(c1x, c1y, c2x, c2y, x, y float64) *Path {
	p.segs = append(p.segs, pathSeg{op: 'C', args: [6]float64{c1x, c1y, c2x, c2y, x, y}})
	return p
}

func (p *Path) Close() *Path {
	p.segs = append(p.segs, pathSeg{op: 'Z'})
	return p
}

// Empty cho biết path chưa có đoạn nào.
func (p *Path) Empty() bool { return p == nil || len(p.segs) == 0 }

// Bounds trả về hộp bao (đã làm tròn ra ngoài 1px cho an toàn khi khử răng cưa).
// Lưu ý: chỉ tính theo điểm nút và điểm điều khiển nên có thể rộng hơn hộp bao thật của
// đường cong — rộng hơn thì vô hại, hẹp hơn mới cắt mất nét.
func (p *Path) Bounds() image.Rectangle {
	if p.Empty() {
		return image.Rectangle{}
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	track := func(x, y float64) {
		minX, minY = math.Min(minX, x), math.Min(minY, y)
		maxX, maxY = math.Max(maxX, x), math.Max(maxY, y)
	}
	for _, s := range p.segs {
		switch s.op {
		case 'M', 'L':
			track(s.args[0], s.args[1])
		case 'Q':
			track(s.args[0], s.args[1])
			track(s.args[2], s.args[3])
		case 'C':
			track(s.args[0], s.args[1])
			track(s.args[2], s.args[3])
			track(s.args[4], s.args[5])
		}
	}
	if math.IsInf(minX, 1) {
		return image.Rectangle{}
	}
	return image.Rect(int(minX)-1, int(minY)-1, int(maxX)+2, int(maxY)+2)
}

// rasterize nạp path vào một vector.Rasterizer đã đặt gốc tại origin.
func (p *Path) rasterize(z *vector.Rasterizer, origin image.Point) {
	ox, oy := float32(origin.X), float32(origin.Y)
	for _, s := range p.segs {
		a := s.args
		switch s.op {
		case 'M':
			z.MoveTo(float32(a[0])-ox, float32(a[1])-oy)
		case 'L':
			z.LineTo(float32(a[0])-ox, float32(a[1])-oy)
		case 'Q':
			z.QuadTo(float32(a[0])-ox, float32(a[1])-oy, float32(a[2])-ox, float32(a[3])-oy)
		case 'C':
			z.CubeTo(float32(a[0])-ox, float32(a[1])-oy, float32(a[2])-ox, float32(a[3])-oy,
				float32(a[4])-ox, float32(a[5])-oy)
		case 'Z':
			z.ClosePath()
		}
	}
}

// Fill tô path bằng màu c lên dst, giới hạn trong clip.
//
// Rasterizer được cấp phát vừa đúng hộp bao của path chứ không phải cả trang: một trang A4
// 300 DPI là ~36 MB, cấp phát cỡ đó cho mỗi bong bóng sẽ giết hiệu năng.
func (p *Path) Fill(dst draw.Image, clip image.Rectangle, c color.Color) {
	if p.Empty() {
		return
	}
	b := p.Bounds().Intersect(clip).Intersect(dst.Bounds())
	if b.Empty() {
		return
	}
	z := vector.NewRasterizer(b.Dx(), b.Dy())
	p.rasterize(z, b.Min)
	z.Draw(dst, b, image.NewUniform(c), image.Point{})
}

// Mask dựng mặt nạ alpha khử răng cưa của path, dùng để cắt ảnh theo hình khung.
// Mặt nạ trả về có gốc trùng gốc ảnh (0,0) và kích thước w×h.
func (p *Path) Mask(w, h int) *image.Alpha {
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	if p.Empty() {
		return m
	}
	z := vector.NewRasterizer(w, h)
	p.rasterize(z, image.Point{})
	z.Draw(m, m.Bounds(), image.NewUniform(color.Alpha{A: 0xFF}), image.Point{})
	return m
}

// D xuất thuộc tính d= của SVG. Toạ độ làm tròn 2 chữ số thập phân — đủ chính xác ở
// 300 DPI mà tệp không phình vì số lẻ dài.
func (p *Path) D() string {
	var b strings.Builder
	for i, s := range p.segs {
		if i > 0 {
			b.WriteByte(' ')
		}
		a := s.args
		switch s.op {
		case 'M':
			fmt.Fprintf(&b, "M%s %s", f2(a[0]), f2(a[1]))
		case 'L':
			fmt.Fprintf(&b, "L%s %s", f2(a[0]), f2(a[1]))
		case 'Q':
			fmt.Fprintf(&b, "Q%s %s %s %s", f2(a[0]), f2(a[1]), f2(a[2]), f2(a[3]))
		case 'C':
			fmt.Fprintf(&b, "C%s %s %s %s %s %s", f2(a[0]), f2(a[1]), f2(a[2]), f2(a[3]), f2(a[4]), f2(a[5]))
		case 'Z':
			b.WriteByte('Z')
		}
	}
	return b.String()
}

func f2(v float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}

// kappa là hằng số xấp xỉ cung tròn bằng Bézier bậc ba.
const kappa = 0.5522847498307933

// AddEllipse thêm một contour ellipse (tâm cx,cy; bán kính rx,ry; phình thêm inflate).
func (p *Path) AddEllipse(cx, cy, rx, ry, inflate float64) *Path {
	rx, ry = rx+inflate, ry+inflate
	if rx <= 0 || ry <= 0 {
		return p
	}
	ox, oy := rx*kappa, ry*kappa
	p.MoveTo(cx-rx, cy)
	p.CubeTo(cx-rx, cy-oy, cx-ox, cy-ry, cx, cy-ry)
	p.CubeTo(cx+ox, cy-ry, cx+rx, cy-oy, cx+rx, cy)
	p.CubeTo(cx+rx, cy+oy, cx+ox, cy+ry, cx, cy+ry)
	p.CubeTo(cx-ox, cy+ry, cx-rx, cy+oy, cx-rx, cy)
	return p.Close()
}

// AddRoundRect thêm một contour chữ nhật bo góc (phình thêm inflate ra mọi phía).
func (p *Path) AddRoundRect(x, y, w, h, r, inflate float64) *Path {
	x, y = x-inflate, y-inflate
	w, h = w+2*inflate, h+2*inflate
	if w <= 0 || h <= 0 {
		return p
	}
	r = math.Max(0, math.Min(r+inflate, math.Min(w, h)/2))
	if r == 0 {
		p.MoveTo(x, y).LineTo(x+w, y).LineTo(x+w, y+h).LineTo(x, y+h)
		return p.Close()
	}
	o := r * kappa
	p.MoveTo(x+r, y)
	p.LineTo(x+w-r, y)
	p.CubeTo(x+w-r+o, y, x+w, y+r-o, x+w, y+r)
	p.LineTo(x+w, y+h-r)
	p.CubeTo(x+w, y+h-r+o, x+w-r+o, y+h, x+w-r, y+h)
	p.LineTo(x+r, y+h)
	p.CubeTo(x+r-o, y+h, x, y+h-r+o, x, y+h-r)
	p.LineTo(x, y+r)
	p.CubeTo(x, y+r-o, x+r-o, y, x+r, y)
	return p.Close()
}

// AddRect thêm một contour chữ nhật vuông góc.
func (p *Path) AddRect(x, y, w, h, inflate float64) *Path {
	return p.AddRoundRect(x, y, w, h, 0, inflate)
}

package comicdraw

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	xdraw "golang.org/x/image/draw"
)

// Render dựng một trang thành ảnh RGBA ở kích thước ĐÃ GỒM TRÀN LỀ.
//
// Thứ tự vẽ cố định: nền → từng khung (ảnh, cắt theo hình khung, viền) → bong bóng theo
// thứ tự đọc → chữ tượng thanh → số trang. Bong bóng vẽ sau tất cả các khung để một bong
// bóng có thể nằm vắt qua mép khung khi cần.
func Render(p *Page) (*image.RGBA, error) {
	spec := p.Spec
	w, h := spec.PxW(), spec.PxH()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	full := dst.Bounds()

	bg := spec.Background
	if bg.A == 0 {
		bg = white
	}
	draw.Draw(dst, full, image.NewUniform(bg), image.Point{}, draw.Src)

	for i := range p.Panels {
		drawPanel(dst, full, spec, &p.Panels[i])
	}
	for i := range p.Balloons {
		drawBalloon(dst, full, spec, &p.Balloons[i], p.Seed+int64(i)*7919)
	}
	for i := range p.SFX {
		drawSFX(dst, full, spec, &p.SFX[i])
	}
	drawPageNumber(dst, full, spec, p)
	return dst, nil
}

// panelDevRect tính hình chữ nhật thiết bị của một khung, nới ra mép bleed nếu FullBleed
// và cạnh đó chạm biên khung thành phẩm.
func panelDevRect(spec PageSpec, pn *Panel) (x, y, w, h float64) {
	x, y, w, h = spec.Dev(pn.Rect)
	if !pn.FullBleed {
		return
	}
	b := spec.BleedPx()
	const eps = 1e-6
	if pn.Rect.X <= eps {
		x -= b
		w += b
	}
	if pn.Rect.Y <= eps {
		y -= b
		h += b
	}
	if pn.Rect.X+pn.Rect.W >= 1-eps {
		w += b
	}
	if pn.Rect.Y+pn.Rect.H >= 1-eps {
		h += b
	}
	return
}

// panelPath dựng đường bao của khung.
func panelPath(spec PageSpec, pn *Panel, inflate float64) *Path {
	x, y, w, h := panelDevRect(spec, pn)
	p := &Path{}
	if pn.Shape == ShapeRounded {
		return p.AddRoundRect(x, y, w, h, spec.MM(pn.Corner), inflate)
	}
	return p.AddRect(x, y, w, h, inflate)
}

// drawPanel vẽ một khung: ảnh (hoặc ô giữ chỗ) đã cắt theo hình khung, rồi viền.
func drawPanel(dst *image.RGBA, clip image.Rectangle, spec PageSpec, pn *Panel) {
	x, y, w, h := panelDevRect(spec, pn)
	box := image.Rect(int(x), int(y), int(math.Ceil(x+w)), int(math.Ceil(y+h))).Intersect(clip)
	if box.Empty() {
		return
	}

	// Vẽ nội dung khung vào một lớp tạm rồi mới dán qua mặt nạ hình khung.
	layer := image.NewRGBA(box)
	if pn.Art != nil {
		fitCover(layer, box, pn.Art, pn.ArtFocus)
	} else {
		drawPlaceholder(layer, box, spec, pn)
	}

	if pn.Shape == ShapeRounded {
		mask := panelPath(spec, pn, 0).Mask(dst.Bounds().Dx(), dst.Bounds().Dy())
		draw.DrawMask(dst, box, layer, box.Min, mask, box.Min, draw.Over)
	} else {
		draw.Draw(dst, box, layer, box.Min, draw.Over)
	}

	if bw := spec.MM(pn.Border.Width); bw > 0.3 {
		col := pn.Border.Color
		if col.A == 0 {
			col = black
		}
		// Nét viền = tô bản phình ra rồi khoét bản gốc (không cần stroker).
		ring := panelPath(spec, pn, bw/2)
		ring.segs = append(ring.segs, reversePath(panelPath(spec, pn, -bw/2)).segs...)
		ring.Fill(dst, clip, col)
	}
}

// reversePath đảo chiều mọi contour để tạo lỗ khi tô theo non-zero winding.
func reversePath(p *Path) *Path {
	out := &Path{}
	// Thu các điểm nút theo thứ tự, rồi phát lại ngược chiều bằng đoạn thẳng.
	// Xấp xỉ bằng đoạn thẳng là chấp nhận được vì đây chỉ là mép TRONG của nét viền,
	// và các đường cong đã được rời rạc hoá đủ dày bởi kappa-bezier.
	var pts [][2]float64
	var startIdx int
	flush := func() {
		if len(pts)-startIdx < 2 {
			return
		}
		seg := pts[startIdx:]
		out.MoveTo(seg[len(seg)-1][0], seg[len(seg)-1][1])
		for i := len(seg) - 2; i >= 0; i-- {
			out.LineTo(seg[i][0], seg[i][1])
		}
		out.Close()
	}
	for _, s := range p.segs {
		switch s.op {
		case 'M':
			flush()
			startIdx = len(pts)
			pts = append(pts, [2]float64{s.args[0], s.args[1]})
		case 'L':
			pts = append(pts, [2]float64{s.args[0], s.args[1]})
		case 'Q':
			pts = append(pts, [2]float64{s.args[2], s.args[3]})
		case 'C':
			// Rời rạc hoá cubic thành 8 đoạn để mép trong đủ mượt.
			if n := len(pts); n > 0 {
				x0, y0 := pts[n-1][0], pts[n-1][1]
				a := s.args
				for i := 1; i <= 8; i++ {
					t := float64(i) / 8
					pts = append(pts, [2]float64{cubicAt(x0, a[0], a[2], a[4], t), cubicAt(y0, a[1], a[3], a[5], t)})
				}
			}
		case 'Z':
			flush()
			startIdx = len(pts)
		}
	}
	flush()
	return out
}

func cubicAt(p0, p1, p2, p3, t float64) float64 {
	u := 1 - t
	return u*u*u*p0 + 3*u*u*t*p1 + 3*u*t*t*p2 + t*t*t*p3
}

// fitCover co giãn ảnh lấp đầy khung theo kiểu cover (cắt bớt phần thừa, KHÔNG kéo méo).
// Bộ sinh ảnh chỉ cho chọn vài tỉ lệ cố định nên ảnh gần như không bao giờ khớp đúng hình
// khung — cắt là hợp đồng, kéo méo thì mặt nhân vật sẽ bị bóp.
func fitCover(dst *image.RGBA, box image.Rectangle, src image.Image, focus Point) {
	sb := src.Bounds()
	if sb.Empty() || box.Empty() {
		return
	}
	if focus.X == 0 && focus.Y == 0 {
		focus = Point{0.5, 0.5}
	}
	scale := math.Max(float64(box.Dx())/float64(sb.Dx()), float64(box.Dy())/float64(sb.Dy()))
	cropW := math.Min(float64(sb.Dx()), float64(box.Dx())/scale)
	cropH := math.Min(float64(sb.Dy()), float64(box.Dy())/scale)
	cx := float64(sb.Min.X) + focus.X*float64(sb.Dx())
	cy := float64(sb.Min.Y) + focus.Y*float64(sb.Dy())
	x0 := clampF(cx-cropW/2, float64(sb.Min.X), float64(sb.Max.X)-cropW)
	y0 := clampF(cy-cropH/2, float64(sb.Min.Y), float64(sb.Max.Y)-cropH)
	crop := image.Rect(int(x0), int(y0), int(x0+cropW), int(y0+cropH))
	xdraw.CatmullRom.Scale(dst, box, src, crop, draw.Src, nil)
}

func clampF(v, lo, hi float64) float64 {
	if hi < lo {
		return lo
	}
	return math.Max(lo, math.Min(hi, v))
}

// drawPlaceholder vẽ ô giữ chỗ khi khung chưa có tranh.
//
// Cố ý đặt trong gói này và kích hoạt bởi Art == nil: nhờ vậy giai đoạn 1 dựng ra TRANG
// THẬT, chỉ thiếu tranh — đủ để chấm bố cục, nhịp trang, vị trí bong bóng và typography
// tiếng Việt mà chưa tốn đồng nào tiền sinh ảnh.
func drawPlaceholder(dst *image.RGBA, box image.Rectangle, spec PageSpec, pn *Panel) {
	tint := pn.Placeholder.Tint
	if tint.A == 0 {
		tint = color.RGBA{0xEC, 0xE6, 0xDA, 0xFF}
	}
	draw.Draw(dst, box, image.NewUniform(tint), image.Point{}, draw.Src)

	// Gạch chéo mờ để nhìn là biết ngay đây chưa phải tranh thật.
	hatch := color.RGBA{0xD8, 0xD0, 0xC0, 0xFF}
	step := int(spec.MM(6))
	if step < 8 {
		step = 8
	}
	for d := box.Min.X - box.Dy(); d < box.Max.X; d += step {
		p := &Path{}
		p.MoveTo(float64(d), float64(box.Min.Y))
		p.LineTo(float64(d)+float64(box.Dy()), float64(box.Max.Y))
		p.LineTo(float64(d)+float64(box.Dy())+2, float64(box.Max.Y))
		p.LineTo(float64(d)+2, float64(box.Min.Y))
		p.Close()
		p.Fill(dst, box, hatch)
	}
}

// balloonMetrics tính tâm và bán kính bong bóng vừa đủ ôm khối chữ.
func balloonMetrics(spec PageSpec, b *Balloon, lay Layout) (cx, cy, rx, ry float64) {
	ax, ay, aw, ah := spec.Dev(b.Anchor)
	padH, padV := spec.MM(b.PadH), spec.MM(b.PadV)
	cx, cy = ax+aw/2, ay+ah/2

	switch b.Kind {
	case BalloonBox:
		rx = math.Min(aw/2, lay.W/2+padH)
		ry = math.Min(ah/2, lay.H/2+padV)
	default:
		// Ellipse phải phình theo hệ số √2 mới ôm hết hình chữ nhật chữ nội tiếp.
		rx = math.Min(aw/2, (lay.W/2+padH)*math.Sqrt2)
		ry = math.Min(ah/2, (lay.H/2+padV)*math.Sqrt2)
	}
	return cx, cy, math.Max(rx, 8), math.Max(ry, 8)
}

// drawBalloon vẽ một bong bóng: thân (+ đuôi) rồi chữ.
func drawBalloon(dst *image.RGBA, clip image.Rectangle, spec PageSpec, b *Balloon, seed int64) {
	if b.Text == "" || b.Style.Family == nil {
		return
	}
	_, _, aw, ah := spec.Dev(b.Anchor)
	padH, padV := spec.MM(b.PadH), spec.MM(b.PadV)
	// Hộp chữ khả dụng bên trong vùng neo, đã trừ đệm và phần cong của ellipse.
	shrink := 1.0
	if b.Kind != BalloonBox {
		shrink = 1 / math.Sqrt2
	}
	boxW := math.Max(16, aw*shrink-2*padH)
	boxH := math.Max(16, ah*shrink-2*padV)

	lay, face, err := FitText(b.Text, boxW, boxH, b.Style)
	if err != nil {
		return
	}
	cx, cy, rx, ry := balloonMetrics(spec, b, lay)

	fill, stroke := b.Fill, b.Stroke
	if fill.A == 0 {
		fill = white
	}
	if stroke.A == 0 {
		stroke = black
	}
	sw := spec.MM(b.StrokeWidth)
	if sw <= 0 {
		sw = spec.MM(0.6)
	}

	// Giới hạn bong bóng trong khung nếu được yêu cầu.
	drawClip := clip
	if b.ClipTo != nil {
		x, y, w, h := spec.Dev(*b.ClipTo)
		drawClip = clip.Intersect(image.Rect(int(x), int(y), int(math.Ceil(x+w)), int(math.Ceil(y+h))))
	}

	if b.Kind == BalloonWhisper {
		// Thì thầm: ruột đặc + viền nét đứt.
		balloonBody(b.Kind, cx, cy, rx, ry, 0, spec.MM(2), seed).Fill(dst, drawClip, fill)
		dashedEllipsePath(cx, cy, rx, ry, sw).Fill(dst, drawClip, stroke)
	} else {
		outer := balloonBody(b.Kind, cx, cy, rx, ry, sw, spec.MM(2), seed)
		inner := balloonBody(b.Kind, cx, cy, rx, ry, 0, spec.MM(2), seed)
		if b.HasTail {
			tx, ty := spec.DevPoint(b.Tail)
			tw := spec.MM(b.TailWidth)
			if tw <= 0 {
				tw = spec.MM(6)
			}
			switch b.Kind {
			case BalloonThought:
				// Cùng rx,ry cho cả hai; chỉ inflate bán kính để nét bao đều quanh ruột.
				outer.segs = append(outer.segs, thoughtTailPath(cx, cy, rx, ry, tx, ty, sw).segs...)
				inner.segs = append(inner.segs, thoughtTailPath(cx, cy, rx, ry, tx, ty, 0).segs...)
			case BalloonBox:
				// Ô thuyết minh không có đuôi.
			default:
				// Ruột phải NGẮN HƠN nét đúng một bề dày nét, nếu không mũi đuôi sẽ trắng nhợt.
				outer.segs = append(outer.segs, tailPath(cx, cy, rx+sw, ry+sw, tx, ty, tw+2*sw, 0).segs...)
				inner.segs = append(inner.segs, tailPath(cx, cy, rx, ry, tx, ty, tw, sw*2.2).segs...)
			}
		}
		outer.Fill(dst, drawClip, stroke)
		inner.Fill(dst, drawClip, fill)
	}

	DrawLayout(dst, drawClip, lay, face, cx-lay.W/2, cy-lay.H/2, lay.W, lay.H, b.Style)
}

// drawSFX vẽ chữ tượng thanh có viền.
func drawSFX(dst *image.RGBA, clip image.Rectangle, spec PageSpec, s *SFX) {
	if s.Text == "" || s.Style.Family == nil {
		return
	}
	x, y := spec.DevPoint(s.At)
	boxW := spec.MM(spec.TrimW) * 0.45
	boxH := spec.MM(spec.TrimH) * 0.12
	lay, face, err := FitText(s.Text, boxW, boxH, s.Style)
	if err != nil {
		return
	}
	fill, stroke := s.Fill, s.Stroke
	if fill.A == 0 {
		fill = white
	}
	if stroke.A == 0 {
		stroke = black
	}
	st := s.Style
	st.Color = fill
	drawTextOutlined(dst, clip, lay, face, x-lay.W/2, y-lay.H/2, lay.W, lay.H, st, spec.MM(s.Outline), stroke)
}

// drawPageNumber in số trang trong vùng an toàn, canh giữa mép dưới.
func drawPageNumber(dst *image.RGBA, clip image.Rectangle, spec PageSpec, p *Page) {
	if p.PageNumber == "" {
		return
	}
	fam := (*FontFamily)(nil)
	for _, b := range p.Balloons {
		if b.Style.Family != nil {
			fam = b.Style.Family
			break
		}
	}
	if fam == nil {
		return
	}
	st := TextStyle{Family: fam, SizePx: spec.MM(4), MinSizePx: spec.MM(3), Color: black, Align: AlignCenter}
	lay, face, err := FitText(p.PageNumber, spec.MM(20), spec.MM(8), st)
	if err != nil {
		return
	}
	b := spec.BleedPx()
	x := b + (spec.MM(spec.TrimW)-lay.W)/2
	y := b + spec.MM(spec.TrimH-spec.SafeMargin) - lay.H
	DrawLayout(dst, clip, lay, face, x, y, lay.W, lay.H, st)
}

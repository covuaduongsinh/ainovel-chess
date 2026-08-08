package comicdraw

import (
	"fmt"
	"html"
	"image/color"
	"math"
	"strings"
)

// SVGOptions điều khiển bộ phát SVG.
type SVGOptions struct {
	// ImageHrefPrefix được ghép trước đường dẫn ảnh khung. Ảnh được THAM CHIẾU theo đường
	// dẫn tương đối chứ không nhúng base64: tệp nhẹ, và sửa ảnh không phải phát lại SVG.
	ImageHrefPrefix string

	// FontFaceCSS là khối @font-face tuỳ chọn để tệp tự chứa font.
	FontFaceCSS string
}

// RenderSVG phát bản vector của trang, dùng ĐÚNG hình học mà Render() dùng.
//
// Giá trị của bản SVG: bong bóng vẫn là <path> và chữ vẫn là <text> nên mở bằng
// Illustrator/Inkscape sửa lại được trước khi in, và dịch sang ngôn ngữ khác chỉ cần thay
// nội dung <text>. Bản PNG không cho phép điều đó.
func RenderSVG(p *Page, o SVGOptions) string {
	spec := p.Spec
	w, h := spec.PxW(), spec.PxH()
	physW := float64(spec.TrimW + 2*spec.Bleed)
	physH := float64(spec.TrimH + 2*spec.Bleed)

	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>`+"\n")
	fmt.Fprintf(&b,
		`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" `+
			`version="1.1" width="%gmm" height="%gmm" viewBox="0 0 %d %d">`+"\n",
		physW, physH, w, h)

	b.WriteString("<defs>\n")
	if o.FontFaceCSS != "" {
		b.WriteString("<style type=\"text/css\"><![CDATA[\n" + o.FontFaceCSS + "\n]]></style>\n")
	}
	for i := range p.Panels {
		pn := &p.Panels[i]
		fmt.Fprintf(&b, `<clipPath id="clip-%s">%s</clipPath>`+"\n",
			safeID(pn.ID), pathEl(panelPath(spec, pn, 0), ""))
	}
	b.WriteString("</defs>\n")

	bg := spec.Background
	if bg.A == 0 {
		bg = white
	}
	fmt.Fprintf(&b, `<rect x="0" y="0" width="%d" height="%d" fill="%s"/>`+"\n", w, h, hexColor(bg))

	// Khung
	for i := range p.Panels {
		pn := &p.Panels[i]
		x, y, pw, ph := panelDevRect(spec, pn)
		fmt.Fprintf(&b, `<g id="%s">`+"\n", safeID(pn.ID))
		if pn.ArtHref != "" {
			fmt.Fprintf(&b,
				`<image clip-path="url(#clip-%s)" x="%s" y="%s" width="%s" height="%s" `+
					`preserveAspectRatio="xMidYMid slice" xlink:href="%s"/>`+"\n",
				safeID(pn.ID), f2(x), f2(y), f2(pw), f2(ph), html.EscapeString(o.ImageHrefPrefix+pn.ArtHref))
		} else {
			fmt.Fprintf(&b, `<rect clip-path="url(#clip-%s)" x="%s" y="%s" width="%s" height="%s" fill="%s"/>`+"\n",
				safeID(pn.ID), f2(x), f2(y), f2(pw), f2(ph), hexColor(placeholderTint(pn)))
		}
		if bw := spec.MM(pn.Border.Width); bw > 0.3 {
			col := pn.Border.Color
			if col.A == 0 {
				col = black
			}
			fmt.Fprintf(&b, `<path d="%s" fill="none" stroke="%s" stroke-width="%s"/>`+"\n",
				panelPath(spec, pn, 0).D(), hexColor(col), f2(bw))
		}
		b.WriteString("</g>\n")
	}

	// Bong bóng
	for i := range p.Balloons {
		bl := &p.Balloons[i]
		writeBalloonSVG(&b, spec, bl, p.Seed+int64(i)*7919)
	}

	// Chữ tượng thanh
	for i := range p.SFX {
		writeSFXSVG(&b, spec, &p.SFX[i])
	}

	if p.PageNumber != "" {
		bx := spec.BleedPx()
		fmt.Fprintf(&b,
			`<text x="%s" y="%s" text-anchor="middle" font-size="%s" fill="%s">%s</text>`+"\n",
			f2(bx+spec.MM(spec.TrimW)/2), f2(bx+spec.MM(spec.TrimH-spec.SafeMargin)),
			f2(spec.MM(4)), hexColor(black), html.EscapeString(p.PageNumber))
	}

	b.WriteString("</svg>\n")
	return b.String()
}

// writeBalloonSVG phát một bong bóng: thân (+đuôi) là <path>, chữ là <text> nhiều <tspan>.
//
// Chữ được phát theo ĐÚNG các dòng đã ngắt ở bản raster; trình đọc SVG KHÔNG được tự ngắt
// lại, nếu không hai đầu ra sẽ lệch nhau — đó là lỗi kinh điển của bộ dựng hai đầu ra.
func writeBalloonSVG(b *strings.Builder, spec PageSpec, bl *Balloon, seed int64) {
	if bl.Text == "" || bl.Style.Family == nil {
		return
	}
	_, _, aw, ah := spec.Dev(bl.Anchor)
	padH, padV := spec.MM(bl.PadH), spec.MM(bl.PadV)
	shrink := 1.0
	if bl.Kind != BalloonBox {
		shrink = 1 / math.Sqrt2
	}
	lay, face, err := FitText(bl.Text, math.Max(16, aw*shrink-2*padH), math.Max(16, ah*shrink-2*padV), bl.Style)
	if err != nil || face == nil {
		return
	}
	cx, cy, rx, ry := balloonMetrics(spec, bl, lay)

	fill, stroke := bl.Fill, bl.Stroke
	if fill.A == 0 {
		fill = white
	}
	if stroke.A == 0 {
		stroke = black
	}
	sw := spec.MM(bl.StrokeWidth)
	if sw <= 0 {
		sw = spec.MM(0.6)
	}

	body := balloonBody(bl.Kind, cx, cy, rx, ry, 0, spec.MM(2), seed)
	if bl.HasTail && bl.Kind != BalloonBox {
		tx, ty := spec.DevPoint(bl.Tail)
		tw := spec.MM(bl.TailWidth)
		if tw <= 0 {
			tw = spec.MM(6)
		}
		if bl.Kind == BalloonThought {
			body.segs = append(body.segs, thoughtTailPath(cx, cy, rx, ry, tx, ty, 0).segs...)
		} else {
			body.segs = append(body.segs, tailPath(cx, cy, rx, ry, tx, ty, tw, 0).segs...)
		}
	}

	dash := ""
	if bl.Kind == BalloonWhisper {
		dash = fmt.Sprintf(` stroke-dasharray="%s %s"`, f2(sw*4), f2(sw*3))
	}
	fmt.Fprintf(b, `<g id="%s">`+"\n", safeID(bl.ID))
	fmt.Fprintf(b, `<path d="%s" fill="%s" stroke="%s" stroke-width="%s"%s/>`+"\n",
		body.D(), hexColor(fill), hexColor(stroke), f2(sw), dash)
	writeTextSVG(b, lay, bl.Style, cx, cy)
	b.WriteString("</g>\n")
}

// writeTextSVG phát khối chữ đã ngắt dòng, căn giữa quanh (cx, cy).
func writeTextSVG(b *strings.Builder, lay Layout, st TextStyle, cx, cy float64) {
	st = st.withDefaults()
	baseY := cy - lay.H/2 - lay.InkTop
	anchor := "middle"
	x := cx
	if st.Align == AlignLeft {
		anchor = "start"
		x = cx - lay.W/2
	}
	fmt.Fprintf(b, `<text text-anchor="%s" font-size="%s" fill="%s" font-family="%s">`,
		anchor, f2(lay.SizePx), hexColor(st.Color), svgFontFamily(st.Family))
	for i, ln := range lay.Lines {
		fmt.Fprintf(b, `<tspan x="%s" y="%s">%s</tspan>`,
			f2(x), f2(baseY+float64(i)*lay.LineStepPx), html.EscapeString(ln.Text))
	}
	b.WriteString("</text>\n")
}

func writeSFXSVG(b *strings.Builder, spec PageSpec, s *SFX) {
	if s.Text == "" || s.Style.Family == nil {
		return
	}
	x, y := spec.DevPoint(s.At)
	lay, face, err := FitText(s.Text, spec.MM(spec.TrimW)*0.45, spec.MM(spec.TrimH)*0.12, s.Style)
	if err != nil || face == nil {
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
	fmt.Fprintf(b, `<g class="sfx" stroke="%s" stroke-width="%s" paint-order="stroke">`+"\n",
		hexColor(stroke), f2(spec.MM(s.Outline)))
	writeTextSVG(b, lay, st, x, y)
	b.WriteString("</g>\n")
}

func svgFontFamily(f *FontFamily) string {
	if f == nil {
		return "sans-serif"
	}
	switch f.Name {
	case "dialogue":
		return "Patrick Hand, Comic Sans MS, cursive"
	case "sfx":
		return "Bangers, Impact, sans-serif"
	default:
		return "Be Vietnam Pro, Arial, sans-serif"
	}
}

func placeholderTint(pn *Panel) color.RGBA {
	if pn.Placeholder.Tint.A != 0 {
		return pn.Placeholder.Tint
	}
	return color.RGBA{0xEC, 0xE6, 0xDA, 0xFF}
}

func pathEl(p *Path, extra string) string {
	return fmt.Sprintf(`<path d="%s"%s/>`, p.D(), extra)
}

func hexColor(c color.RGBA) string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

// safeID lọc id cho hợp lệ trong XML.
func safeID(s string) string {
	if s == "" {
		return "x"
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := b.String()
	if out[0] >= '0' && out[0] <= '9' {
		return "p" + out
	}
	return out
}

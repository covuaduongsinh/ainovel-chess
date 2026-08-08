package comicdraw

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/text/unicode/norm"
)

// Weight là độ đậm của mặt chữ.
type Weight uint8

const (
	WeightRegular Weight = iota
	WeightBold
)

// Align là căn lề ngang của khối chữ.
type Align uint8

const (
	AlignCenter Align = iota
	AlignLeft
)

// TextStyle mô tả cách dựng một khối chữ.
type TextStyle struct {
	Family    *FontFamily
	Weight    Weight
	SizePx    float64 // cỡ mong muốn (pixel)
	MinSizePx float64 // sàn khi phải thu nhỏ cho vừa

	// LineHeight là hệ số giãn dòng. Mặc định 1.30 cho tiếng Việt — CAO HƠN tiếng Anh
	// (~1.15) vì dấu chồng ở dòng dưới rất dễ đụng phần đuôi chữ của dòng trên.
	LineHeight float64

	Align         Align
	LetterSpacing float64 // pixel
	Upper         bool    // viết HOA (strings.ToUpper giữ nguyên dấu tiếng Việt)
	Color         color.RGBA
}

// withDefaults điền giá trị mặc định cho các trường bỏ trống.
func (s TextStyle) withDefaults() TextStyle {
	if s.LineHeight <= 0 {
		s.LineHeight = 1.30
	}
	if s.MinSizePx <= 0 {
		s.MinSizePx = math.Max(8, s.SizePx*0.45)
	}
	if s.SizePx <= 0 {
		s.SizePx = 32
	}
	return s
}

// FontFamily bọc một font đã parse kèm bộ nhớ đệm face theo cỡ.
//
// Đệm face là bắt buộc chứ không phải tối ưu vặt: FitText dò nhị phân ~7 cỡ cho MỖI bong
// bóng, mà opentype.NewFace phải dựng lại toàn bộ metric mỗi lần gọi.
type FontFamily struct {
	Name  string
	fonts map[Weight]*sfnt.Font

	mu    sync.Mutex
	cache map[faceKey]font.Face
}

type faceKey struct {
	weight Weight
	size4  int // cỡ pixel nhân 4 rồi làm tròn — gộp các cỡ sát nhau vào chung một face
}

// NewFontFamily parse byte TTF/OTF thành một họ font.
func NewFontFamily(name string, regular []byte, bold []byte) (*FontFamily, error) {
	fam := &FontFamily{Name: name, fonts: map[Weight]*sfnt.Font{}, cache: map[faceKey]font.Face{}}
	f, err := sfnt.Parse(regular)
	if err != nil {
		return nil, fmt.Errorf("parse font %s: %w", name, err)
	}
	fam.fonts[WeightRegular] = f
	if len(bold) > 0 {
		if fb, err := sfnt.Parse(bold); err == nil {
			fam.fonts[WeightBold] = fb
		}
	}
	return fam, nil
}

// sfntFor trả về font sfnt theo độ đậm (rơi về Regular nếu thiếu Bold).
func (f *FontFamily) sfntFor(w Weight) *sfnt.Font {
	if ft, ok := f.fonts[w]; ok {
		return ft
	}
	return f.fonts[WeightRegular]
}

// Face trả về face ở cỡ pixel chỉ định. DPI=72 để Size tính thẳng bằng pixel.
// HintingNone là cố ý: hinting sinh ra để cứu chữ nhỏ trên màn hình, ở 300 DPI nó chỉ
// làm méo hình dáng dấu tiếng Việt.
func (f *FontFamily) Face(sizePx float64, w Weight) (font.Face, error) {
	if sizePx < 1 {
		sizePx = 1
	}
	key := faceKey{weight: w, size4: int(sizePx*4 + 0.5)}
	f.mu.Lock()
	defer f.mu.Unlock()
	if fc, ok := f.cache[key]; ok {
		return fc, nil
	}
	fc, err := opentype.NewFace(f.sfntFor(w), &opentype.FaceOptions{
		Size:    float64(key.size4) / 4,
		DPI:     72,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return nil, err
	}
	f.cache[key] = fc
	return fc, nil
}

// HasGlyphs kiểm tra font có đủ glyph cho mọi rune trong s không.
// Trả về danh sách rune thiếu — dùng cho test phủ tiếng Việt.
func (f *FontFamily) HasGlyphs(s string, w Weight) []rune {
	ft := f.sfntFor(w)
	var buf sfnt.Buffer
	var missing []rune
	for _, r := range s {
		idx, err := ft.GlyphIndex(&buf, r)
		if err != nil || idx == 0 {
			missing = append(missing, r)
		}
	}
	return missing
}

// FontSet gom các họ font dùng cho một trang truyện tranh.
type FontSet struct {
	Dialogue  *FontFamily // bong bóng thoại
	SFX       *FontFamily // chữ tượng thanh
	Narration *FontFamily // ô thuyết minh, số trang
}

// NewFontSet dựng bộ font từ byte thô (assets cấp byte, gói này giữ vị thế gói lá).
func NewFontSet(dialogue, sfx, narration, narrationBold []byte) (*FontSet, error) {
	d, err := NewFontFamily("dialogue", dialogue, nil)
	if err != nil {
		return nil, err
	}
	s, err := NewFontFamily("sfx", sfx, nil)
	if err != nil {
		return nil, err
	}
	n, err := NewFontFamily("narration", narration, narrationBold)
	if err != nil {
		return nil, err
	}
	return &FontSet{Dialogue: d, SFX: s, Narration: n}, nil
}

// combining là khoảng dấu tổ hợp Unicode. Còn sót rune trong khoảng này sau NFC nghĩa là
// font sẽ không định vị được nó — xem Sanitize.
func isCombining(r rune) bool {
	return (r >= 0x0300 && r <= 0x036F) || (r >= 0x1AB0 && r <= 0x1AFF) || (r >= 0x20D0 && r <= 0x20F0)
}

// Sanitize chuẩn hoá chuỗi trước khi dựng chữ. BẮT BUỘC gọi trước mọi thao tác đo/vẽ.
//
// Lý do: x/image/font/sfnt chỉ nối dây bảng GPOS *kern*, KHÔNG có GSUB và KHÔNG có
// mark-to-base attachment; font.Face lại là API theo từng rune. Nên nếu chuỗi đến ở dạng
// tổ hợp ("e" + U+0302 + U+0301 thay vì "ế" U+1EBF) thì các dấu sẽ bị vẽ thành glyph riêng
// chồng đống ở gốc bút. Chuyện này xảy ra thường xuyên với văn bản dán từ macOS và LLM
// hoàn toàn có thể sinh ra.
//
// May mắn là cả 134 chữ cái tiếng Việt đều có dạng tiền-kết-hợp, nên NFC là đủ — không cần
// engine shaping nào. Còn sót dấu tổ hợp thì TRẢ LỖI, thà dừng còn hơn in ra rác.
func Sanitize(s string) (string, error) {
	s = norm.NFC.String(s)
	// Nháy thẳng → nháy typographic; ba chấm → ký tự ellipsis.
	s = strings.ReplaceAll(s, "...", "…")
	for _, r := range s {
		if isCombining(r) {
			return s, fmt.Errorf("chuỗi còn dấu tổ hợp U+%04X sau khi chuẩn hoá NFC — không dựng được bằng x/image", r)
		}
	}
	return s, nil
}

// MustSanitize như Sanitize nhưng thay vì lỗi thì bỏ các dấu tổ hợp còn sót.
// Dùng ở đường dựng ảnh để một câu thoại hỏng không làm sập cả lần chạy.
func MustSanitize(s string) string {
	out, err := Sanitize(s)
	if err == nil {
		return out
	}
	var b strings.Builder
	for _, r := range out {
		if !isCombining(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Line là một dòng chữ đã ngắt.
type Line struct {
	Text    string
	WidthPx float64
}

// Layout là kết quả đo + ngắt dòng, đủ để vẽ raster và xuất SVG giống hệt nhau.
type Layout struct {
	Lines      []Line
	SizePx     float64
	LineStepPx float64
	InkTop     float64 // đỉnh mực THẬT so với baseline (âm), xem inkTop
	InkBottom  float64
	W, H       float64
}

// measure đo bề rộng một chuỗi, cộng kerning và letter-spacing.
// Cộng dồn trong fixed.Int26_6 rồi mới đổi sang float MỘT LẦN: cộng float theo từng glyph
// sẽ tích luỹ sai số thấy rõ trên một dòng 40 ký tự.
func measure(face font.Face, s string, letterSpacing float64) float64 {
	var adv fixed.Int26_6
	var prev rune
	first := true
	n := 0
	for _, r := range s {
		if !first {
			adv += face.Kern(prev, r)
		}
		a, ok := face.GlyphAdvance(r)
		if !ok {
			a, _ = face.GlyphAdvance(' ')
		}
		adv += a
		prev, first = r, false
		n++
	}
	w := float64(adv) / 64
	if n > 1 {
		w += letterSpacing * float64(n-1)
	}
	return w
}

// inkTop quét GlyphBounds của CHÍNH chuỗi sắp vẽ để tìm đỉnh mực thật.
//
// Đây là điểm sửa quan trọng nhất cho tiếng Việt: face.Metrics().Ascent là ascent
// *typographic* — một hằng số thiết kế của cả font. Các chữ chồng hai dấu (Ế, Ổ, Ữ, Ẫ)
// thường xuyên VƯỢT QUÁ nó. Căn giữa bằng Ascent sẽ cắt cụt dấu trên cùng hoặc đẩy chữ
// xuống thấp — đúng là hiện tượng "chữ tiếng Việt trông lệch" ở mọi bộ dựng làm ẩu.
func inkTop(face font.Face, lines []Line) (top, bottom float64) {
	top, bottom = math.Inf(1), math.Inf(-1)
	for _, ln := range lines {
		for _, r := range ln.Text {
			b, _, ok := face.GlyphBounds(r)
			if !ok {
				continue
			}
			top = math.Min(top, float64(b.Min.Y)/64)
			bottom = math.Max(bottom, float64(b.Max.Y)/64)
		}
	}
	if math.IsInf(top, 1) {
		m := face.Metrics()
		return -float64(m.Ascent) / 64, float64(m.Descent) / 64
	}
	return top, bottom
}

// noBreakAfter là các tiếng không nên đứng cuối dòng. Tiếng Việt tách theo âm tiết nên
// ngắt dòng tham lam rất dễ để lại một tiếng phụ trợ trơ trọi cuối dòng.
var noBreakAfter = map[string]bool{
	"của": true, "và": true, "ở": true, "để": true, "là": true, "một": true,
	"cho": true, "với": true, "như": true, "thì": true, "mà": true, "từ": true,
	"trong": true, "trên": true, "dưới": true, "về": true, "khi": true, "vì": true,
}

// wrapWords ngắt dòng tham lam theo khoảng trắng.
func wrapWords(face font.Face, s string, maxW, letterSpacing float64) []Line {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var lines []Line
	cur := ""
	flush := func() {
		if cur != "" {
			lines = append(lines, Line{Text: cur, WidthPx: measure(face, cur, letterSpacing)})
			cur = ""
		}
	}
	for i, w := range words {
		cand := w
		if cur != "" {
			cand = cur + " " + w
		}
		if cur != "" && measure(face, cand, letterSpacing) > maxW {
			// Không để tiếng phụ trợ trơ trọi cuối dòng: kéo nó xuống dòng sau.
			parts := strings.Fields(cur)
			if len(parts) > 1 && noBreakAfter[strings.ToLower(parts[len(parts)-1])] {
				last := parts[len(parts)-1]
				cur = strings.Join(parts[:len(parts)-1], " ")
				flush()
				cur = last + " " + w
			} else {
				flush()
				cur = w
			}
		} else {
			cur = cand
		}
		if i == len(words)-1 {
			flush()
		}
	}
	return lines
}

// balanceLines cân bằng độ dài các dòng.
//
// Ngắt tham lam cho ra dạng [.......dài.......][một từ] — trong một bong bóng nhìn như lỗi.
// Sau khi biết cần N dòng, ngắt lại với bề rộng hẹp hơn để các dòng đều nhau, giữ kết quả
// nếu vẫn đúng N dòng. Vài chục dòng code nhưng là khác biệt giữa "giống truyện tranh" và
// "giống output của một chương trình".
func balanceLines(face font.Face, s string, maxW, letterSpacing float64) []Line {
	lines := wrapWords(face, s, maxW, letterSpacing)
	if len(lines) < 2 {
		return lines
	}
	total := 0.0
	for _, l := range lines {
		total += l.WidthPx
	}
	target := total/float64(len(lines))*1.05 + letterSpacing
	if target >= maxW {
		return lines
	}
	if alt := wrapWords(face, s, target, letterSpacing); len(alt) == len(lines) {
		return alt
	}
	return lines
}

// FitText dò cỡ chữ lớn nhất mà khối chữ đã ngắt dòng vẫn lọt trong hộp boxW×boxH.
func FitText(s string, boxW, boxH float64, st TextStyle) (Layout, font.Face, error) {
	st = st.withDefaults()
	if st.Family == nil {
		return Layout{}, nil, fmt.Errorf("thiếu FontFamily")
	}
	s = MustSanitize(s)
	if st.Upper {
		s = strings.ToUpper(s)
	}
	if strings.TrimSpace(s) == "" {
		return Layout{}, nil, fmt.Errorf("chuỗi rỗng")
	}
	if boxW <= 0 || boxH <= 0 {
		return Layout{}, nil, fmt.Errorf("hộp chữ rỗng")
	}

	build := func(size float64) (Layout, font.Face, bool) {
		face, err := st.Family.Face(size, st.Weight)
		if err != nil {
			return Layout{}, nil, false
		}
		lines := balanceLines(face, s, boxW, st.LetterSpacing)
		if len(lines) == 0 {
			return Layout{}, nil, false
		}
		step := size * st.LineHeight
		maxW := 0.0
		for _, l := range lines {
			maxW = math.Max(maxW, l.WidthPx)
		}
		top, bottom := inkTop(face, lines)
		// Chiều cao thật = (N-1) bước dòng + chiều cao mực của dòng đầu và cuối.
		h := float64(len(lines)-1)*step + (bottom - top)
		lay := Layout{Lines: lines, SizePx: size, LineStepPx: step, InkTop: top, InkBottom: bottom, W: maxW, H: h}
		return lay, face, maxW <= boxW && h <= boxH
	}

	if lay, face, ok := build(st.SizePx); ok {
		return lay, face, nil
	}
	lo, hi := st.MinSizePx, st.SizePx
	var best Layout
	var bestFace font.Face
	for i := 0; i < 7 && hi-lo > 0.25; i++ {
		mid := (lo + hi) / 2
		if lay, face, ok := build(mid); ok {
			best, bestFace, lo = lay, face, mid
		} else {
			hi = mid
		}
	}
	if bestFace != nil {
		return best, bestFace, nil
	}
	// Không cỡ nào vừa — dùng cỡ sàn và chấp nhận tràn, vẫn hơn là không vẽ gì.
	lay, face, _ := build(st.MinSizePx)
	if face == nil {
		return Layout{}, nil, fmt.Errorf("không dựng được face")
	}
	return lay, face, nil
}

// DrawLayout vẽ khối chữ đã đo vào hộp box (pixel thiết bị), căn giữa theo chiều dọc
// bằng ĐỈNH MỰC THẬT chứ không phải ascent typographic.
func DrawLayout(dst draw.Image, clip image.Rectangle, lay Layout, face font.Face,
	boxX, boxY, boxW, boxH float64, st TextStyle) {

	st = st.withDefaults()
	src := image.NewUniform(st.Color)
	// baseline dòng đầu: đẩy khối chữ xuống sao cho mực nằm giữa hộp theo chiều dọc.
	baseY := boxY + (boxH-lay.H)/2 - lay.InkTop

	for i, ln := range lay.Lines {
		x := boxX
		switch st.Align {
		case AlignCenter:
			x = boxX + (boxW-ln.WidthPx)/2
		}
		y := baseY + float64(i)*lay.LineStepPx
		drawLine(dst, clip, face, src, ln.Text, x, y, st.LetterSpacing)
	}
}

// drawLine vẽ một dòng chữ với baseline tại y.
func drawLine(dst draw.Image, clip image.Rectangle, face font.Face, src image.Image,
	s string, x, y, letterSpacing float64) {

	dot := fixed.Point26_6{X: fixed.Int26_6(x * 64), Y: fixed.Int26_6(y * 64)}
	var prev rune
	first := true
	for _, r := range s {
		if !first {
			dot.X += face.Kern(prev, r)
			dot.X += fixed.Int26_6(letterSpacing * 64)
		}
		dr, mask, maskp, adv, ok := face.Glyph(dot, r)
		if !ok {
			dot.X += fixed.Int26_6(face.Metrics().Height / 2)
			prev, first = r, false
			continue
		}
		if r != ' ' {
			// DrawMask giữ đúng khử răng cưa của glyph; blit thẳng sẽ ra chữ răng cưa.
			draw.DrawMask(dst, dr.Intersect(clip), src, image.Point{}, mask, maskp, draw.Over)
		}
		dot.X += adv
		prev, first = r, false
	}
}

// drawTextOutlined vẽ chữ có viền bằng cách vẽ mặt nạ lệch 8 hướng theo màu nét rồi vẽ
// ruột đè lên. Ở 300 DPI cách này nhìn không khác gì stroker thật, mà không cần stroker.
func drawTextOutlined(dst draw.Image, clip image.Rectangle, lay Layout, face font.Face,
	boxX, boxY, boxW, boxH float64, st TextStyle, outline float64, stroke color.RGBA) {

	if outline > 0.5 {
		os := st
		os.Color = stroke
		for i := 0; i < 8; i++ {
			ang := float64(i) * math.Pi / 4
			dx, dy := math.Cos(ang)*outline, math.Sin(ang)*outline
			DrawLayout(dst, clip, lay, face, boxX+dx, boxY+dy, boxW, boxH, os)
		}
	}
	DrawLayout(dst, clip, lay, face, boxX, boxY, boxW, boxH, st)
}

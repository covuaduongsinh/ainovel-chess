package comic

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // đăng ký bộ giải mã JPEG cho image.Decode
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/voocel/ainovel-cli/internal/comicdraw"
)

// computeAndWriteLayout tính hình học mọi trang của một chương rồi ghi ra đĩa.
func (rc *runCtx) computeAndWriteLayout(ch int, script *ScriptResult) ([]PageLayout, error) {
	layouts := make([]PageLayout, 0, len(script.Pages))
	for i := range script.Pages {
		layouts = append(layouts, ComputeLayout(rc.spec, script.Pages[i]))
	}
	// Bố cục là dữ liệu phái sinh thuần Go, rẻ — luôn ghi lại cho khớp kịch bản hiện tại.
	if err := rc.writeAlwaysJSON(ProductLayout, fmt.Sprintf("bo-cuc/%02d.json", ch), layouts); err != nil {
		return nil, err
	}
	return layouts, nil
}

func (rc *runCtx) writeAlwaysJSON(product Product, rel string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return rc.writeAlways(product, rel, data)
}

func (rc *runCtx) loadLayouts(ch int, script *ScriptResult) []PageLayout {
	var l []PageLayout
	if rc.loadArtifact(fmt.Sprintf("bo-cuc/%02d.json", ch), &l) && len(l) == len(script.Pages) {
		return l
	}
	// Thiếu hoặc lệch số trang thì tính lại — rẻ và luôn đúng.
	out := make([]PageLayout, 0, len(script.Pages))
	for i := range script.Pages {
		out = append(out, ComputeLayout(rc.spec, script.Pages[i]))
	}
	return out
}

// artFile là đường dẫn TƯƠNG ĐỐI của ảnh một khung. Đây là HỢP ĐỒNG giữa hai giai đoạn:
// giai đoạn 1 ghi đường dẫn này vào bảng prompt kể cả khi tệp chưa tồn tại; giai đoạn 2
// (hoặc chính người dùng vẽ tay) chỉ việc đặt tệp vào đúng chỗ.
func artFile(ch, page, panel int) string {
	return fmt.Sprintf("art/chuong-%03d/t%02d-k%02d.png", ch, page, panel)
}

// aspectForPanel chọn tỉ lệ khung hình gần nhất mà bộ sinh ảnh hỗ trợ.
// Luôn chọn tỉ lệ RỘNG HƠN hoặc bằng khung thật rồi cắt phủ — kéo méo sẽ bóp mặt nhân vật.
func aspectForPanel(box comicdraw.Rect, spec comicdraw.PageSpec) string {
	w := box.W * spec.MM(spec.TrimW)
	h := box.H * spec.MM(spec.TrimH)
	if h <= 0 {
		return "4:3"
	}
	r := w / h
	switch {
	case r >= 3.0:
		return "4:1"
	case r >= 1.9:
		return "21:9"
	case r >= 1.5:
		return "16:9"
	case r >= 1.25:
		return "3:2"
	case r >= 1.1:
		return "4:3"
	case r >= 0.9:
		return "1:1"
	case r >= 0.72:
		return "3:4"
	default:
		return "2:3"
	}
}

// buildPanelPrompt ghép prompt khung hoàn chỉnh: mô tả khung + mô tả chuẩn từng nhân vật
// có mặt + token phong cách + yêu cầu chừa chỗ đặt bong bóng.
func (rc *runCtx) buildPanelPrompt(p Panel) string {
	parts := []string{strings.TrimSpace(p.ImagePrompt)}

	// Chèn NGUYÊN VĂN canonical prompt của nhân vật — đây là cơ chế giữ nhất quán.
	byName := map[string]CharacterSheet{}
	for _, c := range rc.ensureCharacters() {
		byName[strings.ToLower(strings.TrimSpace(c.Name))] = c
	}
	for _, name := range p.Characters {
		if c, ok := byName[strings.ToLower(strings.TrimSpace(name))]; ok && c.CanonicalPrompt != "" {
			parts = append(parts, c.CanonicalPrompt)
		}
	}
	if tk := rc.styleTokens(); len(tk) > 0 {
		parts = append(parts, strings.Join(tk, ", "))
	}
	// Chừa chỗ cho bong bóng: nếu không nói, mô hình sẽ lấp kín khung và bong bóng đè lên mặt.
	if len(p.Balloons) > 0 {
		if area := reserveClause(p.ReserveFor, p.Balloons); area != "" {
			parts = append(parts, area)
		}
	}
	return strings.Join(filterEmpty(parts), ", ")
}

// reserveClause sinh mệnh đề yêu cầu chừa khoảng trống trong khung.
func reserveClause(reserve string, balloons []Balloon) string {
	key := strings.TrimSpace(reserve)
	if key == "" && len(balloons) > 0 {
		key = balloons[0].Anchor
	}
	m := map[string]string{
		"tren-trai":  "leave empty negative space in the upper left",
		"tren-giua":  "leave empty negative space along the top",
		"tren-phai":  "leave empty negative space in the upper right",
		"giua-trai":  "leave empty negative space on the left",
		"giua-phai":  "leave empty negative space on the right",
		"duoi-trai":  "leave empty negative space in the lower left",
		"duoi-giua":  "leave empty negative space along the bottom",
		"duoi-phai":  "leave empty negative space in the lower right",
	}
	if s, ok := m[key]; ok {
		return s + " for a speech balloon"
	}
	return "leave some empty negative space for a speech balloon"
}

func filterEmpty(in []string) []string {
	out := in[:0]
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

// imageSize trả về độ phân giải yêu cầu: 2K nếu có xuất PDF (in ấn), ngược lại 1K.
func (rc *runCtx) imageSize() string {
	if s := strings.TrimSpace(rc.opts.ImageSize); s != "" {
		return s
	}
	for _, f := range rc.formats() {
		if f == FormatPDF {
			return "2K"
		}
	}
	return "1K"
}

func (rc *runCtx) formats() []Format {
	if len(rc.opts.Formats) == 0 {
		return DefaultFormats()
	}
	return rc.opts.Formats
}

// writePanelPrompts ghi bảng prompt khung của một chương.
func (rc *runCtx) writePanelPrompts(ch int, script *ScriptResult) error {
	var rows []PanelPrompt
	for pi := range script.Pages {
		page := &script.Pages[pi]
		lay := ComputeLayout(rc.spec, *page)
		for ki := range page.Panels {
			k := &page.Panels[ki]
			box := comicdraw.Rect{W: 1, H: 1}
			if ki < len(lay.Boxes) {
				box = lay.Boxes[ki]
			}
			rows = append(rows, PanelPrompt{
				Chapter:  ch,
				Page:     page.PageNo,
				Panel:    k.PanelNo,
				ArtFile:  artFile(ch, page.PageNo, k.PanelNo),
				Prompt:   rc.buildPanelPrompt(*k),
				Negative: rc.negativeTokens(k.NegativePrompt),
				Refs:     rc.refsFor(*k),
				Aspect:   aspectForPanel(box, rc.spec),
				Size:     rc.imageSize(),
			})
		}
	}
	if err := rc.writeAlwaysJSON(ProductPanelPrompt, fmt.Sprintf("prompts/chuong-%03d.json", ch), rows); err != nil {
		return err
	}
	return rc.writeAlways(ProductPanelPrompt, fmt.Sprintf("prompts/chuong-%03d.md", ch),
		[]byte(renderPanelPromptMarkdown(ch, script.Title, rows)))
}

// refsFor trả về đường dẫn ảnh model sheet của các nhân vật trong khung (nếu đã sinh).
func (rc *runCtx) refsFor(p Panel) []string {
	var out []string
	for _, name := range p.Characters {
		rel := "nhan-vat/" + slug(name) + ".png"
		if exists(rc.path(rel)) {
			out = append(out, rel)
		}
	}
	return out
}

// renderChapterPages dựng mọi trang của một chương thành PNG (+ SVG).
func (rc *runCtx) renderChapterPages(ch, volIndex int, script *ScriptResult, layouts []PageLayout) error {
	for pi := range script.Pages {
		page := &script.Pages[pi]
		var lay PageLayout
		if pi < len(layouts) {
			lay = layouts[pi]
		} else {
			lay = ComputeLayout(rc.spec, *page)
		}

		cp, err := rc.buildDrawPage(ch, page, lay)
		if err != nil {
			rc.emit(Event{Stage: StagePage, Message: fmt.Sprintf("Bỏ qua trang %d chương %d: %v", page.PageNo, ch, err)})
			continue
		}
		img, err := comicdraw.Render(cp)
		if err != nil {
			return fmt.Errorf("dựng trang %d chương %d: %w", page.PageNo, ch, err)
		}
		rel := fmt.Sprintf("trang/chuong-%03d/%02d.png", ch, page.PageNo)
		data, err := encodePNG(img)
		if err != nil {
			return err
		}
		if err := rc.writeAlways(ProductPage, rel, data); err != nil {
			return err
		}
		// Bản SVG vector để tút lại bằng tay trước khi in.
		svg := comicdraw.RenderSVG(cp, comicdraw.SVGOptions{ImageHrefPrefix: "../../"})
		if err := rc.writeAlways(ProductPage, fmt.Sprintf("trang/chuong-%03d/%02d.svg", ch, page.PageNo), []byte(svg)); err != nil {
			return err
		}
	}
	return rc.writeChapterIndex(ch, volIndex, script)
}

func encodePNG(img image.Image) ([]byte, error) {
	var b strings.Builder
	bw := &byteWriter{}
	if err := png.Encode(bw, img); err != nil {
		return nil, err
	}
	_ = b
	return bw.buf, nil
}

type byteWriter struct{ buf []byte }

func (w *byteWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}

// buildDrawPage dịch mô hình nghiệp vụ sang mô hình đồ hoạ của comicdraw.
func (rc *runCtx) buildDrawPage(ch int, page *PageSpec, lay PageLayout) (*comicdraw.Page, error) {
	fonts := rc.deps.Fonts
	spec := rc.spec
	ink := color.RGBA{0x1A, 0x1A, 0x1A, 0xFF}

	dlg := comicdraw.TextStyle{Family: fonts.Dialogue, SizePx: spec.MM(4.4), MinSizePx: spec.MM(2.4),
		Color: ink, Align: comicdraw.AlignCenter}
	narr := comicdraw.TextStyle{Family: fonts.Narration, SizePx: spec.MM(3.5), MinSizePx: spec.MM(2.2),
		Color: ink, Align: comicdraw.AlignLeft}
	sfxSt := comicdraw.TextStyle{Family: fonts.SFX, SizePx: spec.MM(14), MinSizePx: spec.MM(7),
		Color: color.RGBA{0xFF, 0xD5, 0x4F, 0xFF}, Align: comicdraw.AlignCenter, Upper: true}
	if rc.bible.Preset.IsMonochrome() {
		sfxSt.Color = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	}

	cp := &comicdraw.Page{
		Index:      page.PageNo,
		Spec:       spec,
		Seed:       int64(ch)*1000 + int64(page.PageNo),
		PageNumber: fmt.Sprintf("%d", page.PageNo),
	}

	for ki := range page.Panels {
		k := &page.Panels[ki]
		box := comicdraw.Rect{W: 1, H: 1}
		if ki < len(lay.Boxes) {
			box = lay.Boxes[ki]
		}
		pn := comicdraw.Panel{
			ID:        fmt.Sprintf("k%02d", k.PanelNo),
			Rect:      box,
			Shape:     comicdraw.ShapeRect,
			FullBleed: true,
			Border:    comicdraw.Border{Width: 0.7, Color: ink},
			Placeholder: comicdraw.Placeholder{
				Label: fmt.Sprintf("T%d·K%d", page.PageNo, k.PanelNo),
				Note:  k.Description,
			},
		}
		// Ảnh thật nếu có trên đĩa; thiếu thì Art=nil và comicdraw vẽ ô giữ chỗ.
		if img, rel := rc.loadArt(ch, page.PageNo, k.PanelNo); img != nil {
			pn.Art = img
			pn.ArtHref = rel
		}
		cp.Panels = append(cp.Panels, pn)

		for _, s := range k.SFX {
			st := sfxSt
			switch s.Scale {
			case "nho":
				st.SizePx = spec.MM(9)
			case "lon":
				st.SizePx = spec.MM(20)
			}
			fx, fy := anchorFractions(s.Anchor)
			cp.SFX = append(cp.SFX, comicdraw.SFX{
				At:      comicdraw.Point{X: box.X + box.W*fx, Y: box.Y + box.H*fy},
				Text:    s.Text,
				Style:   st,
				Outline: 1.3,
				Fill:    st.Color,
				Stroke:  ink,
			})
		}
	}

	// Bong bóng theo thứ tự đọc, dùng vùng neo đã tính ở bước bố cục.
	for _, a := range lay.Anchor {
		if a.PanelIndex >= len(page.Panels) {
			continue
		}
		panel := page.Panels[a.PanelIndex]
		if a.Order >= len(panel.Balloons) {
			continue
		}
		b := panel.Balloons[a.Order]
		kind := comicdraw.ParseBalloonKind(b.Kind)
		st := dlg
		if kind == comicdraw.BalloonBox {
			st = narr
		}
		clip := lay.Boxes[a.PanelIndex]
		cp.Balloons = append(cp.Balloons, comicdraw.Balloon{
			ID:          fmt.Sprintf("k%02d-b%d", panel.PanelNo, a.Order),
			Kind:        kind,
			Anchor:      a.Rect,
			ClipTo:      &clip,
			Tail:        a.Tail,
			HasTail:     a.HasTail && kind != comicdraw.BalloonBox,
			TailWidth:   6,
			Text:        b.Text,
			Style:       st,
			PadH:        3.6,
			PadV:        2.8,
			StrokeWidth: strokeFor(kind),
		})
	}
	return cp, nil
}

func strokeFor(k comicdraw.BalloonKind) comicdraw.Millimeter {
	switch k {
	case comicdraw.BalloonShout:
		return 0.9
	case comicdraw.BalloonBox:
		return 0.5
	default:
		return 0.7
	}
}

// loadArt nạp ảnh một khung từ đĩa (nil nếu chưa có — bộ dựng sẽ vẽ ô giữ chỗ).
//
// Đọc THEO ĐƯỜNG DẪN chứ không nhận ảnh qua bộ nhớ là quyết định then chốt: nhờ đó dựng
// lại trang sau khi có tranh là thao tác thuần giai đoạn 1, không phải sửa dòng code nào,
// và người dùng tự vẽ tay thả vào thư mục cũng chạy được ngay.
// Trả về (ảnh, đường dẫn tương đối) — đường dẫn dùng cho bản SVG.
func (rc *runCtx) loadArt(ch, page, panel int) (image.Image, string) {
	base := strings.TrimSuffix(artFile(ch, page, panel), ".png")
	for _, ext := range []string{".png", ".jpg", ".jpeg"} {
		rel := base + ext
		f, err := os.Open(rc.path(rel))
		if err != nil {
			continue
		}
		img, _, err := image.Decode(f)
		_ = f.Close()
		if err == nil {
			return img, rel
		}
	}
	return nil, ""
}

// generatePanelArt sinh ảnh cho các khung còn thiếu (chỉ chạy khi có Deps.Img — giai đoạn 2).
func (rc *runCtx) generatePanelArt(ctx context.Context, ch int) error {
	if rc.deps.Img == nil {
		return nil
	}
	var rows []PanelPrompt
	if !rc.loadArtifact(fmt.Sprintf("prompts/chuong-%03d.json", ch), &rows) {
		return nil
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		if rc.opts.MaxImages > 0 && rc.imagesMade >= rc.opts.MaxImages {
			rc.emit(Event{Stage: StagePanelArt,
				Message: fmt.Sprintf("Đã chạm trần %d ảnh cho lần chạy này — dừng sinh ảnh", rc.opts.MaxImages)})
			return nil
		}
		if !rc.opts.Overwrite && exists(rc.path(row.ArtFile)) {
			continue
		}
		req := PanelRequest{Prompt: row.Prompt, Negative: row.Negative, Aspect: row.Aspect, Size: row.Size}
		for _, ref := range row.Refs {
			if data, err := os.ReadFile(rc.path(ref)); err == nil {
				req.Refs = append(req.Refs, RefImage{MimeType: "image/png", Data: data, Label: filepath.Base(ref)})
			}
		}
		img, err := rc.deps.Img.Panel(ctx, req)
		if err != nil {
			// Bỏ qua mềm từng khung: một khung bị chặn không được làm hỏng cả lần chạy.
			rc.emit(Event{Stage: StagePanelArt,
				Message: fmt.Sprintf("Bỏ qua khung T%d·K%d chương %d: %v", row.Page, row.Panel, ch, err)})
			continue
		}
		if err := rc.writeAlways(ProductPanelArt, row.ArtFile, img.Data); err != nil {
			return err
		}
		rc.imagesMade++
	}
	return nil
}

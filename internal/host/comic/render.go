package comic

import (
	"fmt"
	"sort"
	"strings"
)

// Các hàm render dưới đây thuần Go, không gọi LLM — chúng chỉ trình bày lại dữ liệu đã có
// dưới dạng Markdown cho người đọc.

func renderStyleMarkdown(novel string, s StyleResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Định hướng mỹ thuật truyện tranh — %s\n\n", titleOr(novel, "Không tên"))
	writeField(&b, "Phong cách tổng thể", s.Overall)
	writeField(&b, "Nét vẽ", s.LineArt)
	writeField(&b, "Quy ước bong bóng & chữ", s.Lettering)
	writeList(&b, "Bảng màu", s.Palette)

	if len(s.StyleTokens) > 0 {
		b.WriteString("\n## Token phong cách (chèn vào mọi prompt khung)\n\n```\n")
		b.WriteString(strings.Join(s.StyleTokens, ", "))
		b.WriteString("\n```\n")
	}
	if len(s.Negative) > 0 {
		b.WriteString("\n## Negative dùng chung\n\n```\n")
		b.WriteString(strings.Join(s.Negative, ", "))
		b.WriteString("\n```\n")
	}
	if len(s.Locations) > 0 {
		b.WriteString("\n## Bối cảnh\n")
		for _, l := range s.Locations {
			fmt.Fprintf(&b, "\n### %s\n\n", titleOr(l.Name, "Không tên"))
			writeField(&b, "Mô tả", l.Description)
			writePrompt(&b, "Prompt ảnh", l.ImagePrompt)
		}
	}
	return b.String()
}

func renderCharactersMarkdown(novel string, cs []CharacterSheet) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Model sheet nhân vật — %s\n\n", titleOr(novel, "Không tên"))
	b.WriteString("> `canonical_prompt` được chèn **nguyên văn** vào prompt của mọi khung có\n" +
		"> nhân vật đó. Sửa nó là đổi diện mạo nhân vật trên toàn bộ cuốn sách.\n")
	for _, c := range cs {
		fmt.Fprintf(&b, "\n## %s\n\n", titleOr(c.Name, "Không tên"))
		writeField(&b, "Vai trò", c.Role)
		writeField(&b, "Ngoại hình", c.Appearance)
		writeField(&b, "Trang phục", c.Wardrobe)
		writeList(&b, "Bảng màu", c.Palette)
		writePrompt(&b, "Prompt chuẩn (khoá diện mạo)", c.CanonicalPrompt)
		writePrompt(&b, "Prompt model sheet", c.SheetPrompt)
		writePrompt(&b, "Negative", c.NegativePrompt)
	}
	return b.String()
}

func renderScriptMarkdown(s ScriptResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Kịch bản truyện tranh — Chương %d — %s\n\n", s.Chapter, titleOr(s.Title, "Không tên"))
	fmt.Fprintf(&b, "Tổng cộng **%d trang**.\n", len(s.Pages))

	for _, p := range s.Pages {
		fmt.Fprintf(&b, "\n## Trang %d", p.PageNo)
		if p.Spread {
			b.WriteString(" · *trang đôi*")
		}
		b.WriteString("\n\n")
		if p.Beat != "" {
			fmt.Fprintf(&b, "*Nhịp:* %s\n\n", p.Beat)
		}
		for _, k := range p.Panels {
			fmt.Fprintf(&b, "### Khung %d · %s", k.PanelNo, sizeLabel(k.Size))
			if k.Shot != "" {
				fmt.Fprintf(&b, " · %s", shotLabel(k.Shot))
			}
			b.WriteString("\n\n")
			writeField(&b, "Diễn biến", k.Description)
			if len(k.Characters) > 0 {
				fmt.Fprintf(&b, "- **Nhân vật:** %s\n", strings.Join(k.Characters, ", "))
			}
			for _, bl := range k.Balloons {
				label := balloonLabel(bl.Kind)
				if bl.Speaker != "" && bl.Kind != "thuyet-minh" {
					fmt.Fprintf(&b, "- **%s** — %s: “%s”\n", label, bl.Speaker, bl.Text)
				} else {
					fmt.Fprintf(&b, "- **%s:** “%s”\n", label, bl.Text)
				}
			}
			for _, fx := range k.SFX {
				fmt.Fprintf(&b, "- **Tượng thanh:** %s\n", fx.Text)
			}
			writePrompt(&b, "Prompt ảnh", k.ImagePrompt)
		}
		if p.Cliff != "" {
			fmt.Fprintf(&b, "\n> **Cú hích lật trang:** %s\n", p.Cliff)
		}
	}
	return b.String()
}

func renderPanelPromptMarkdown(ch int, title string, rows []PanelPrompt) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Prompt khung — Chương %d — %s\n\n", ch, titleOr(title, "Không tên"))
	b.WriteString("> Đây là **hợp đồng giữa hai giai đoạn**: mỗi dòng chỉ rõ ảnh phải nằm ở đâu.\n" +
		"> Bạn có thể tự vẽ hoặc tự chạy bộ sinh ảnh rồi thả tệp vào đúng `art_file`, chạy lại\n" +
		"> `/truyentranh page` là trang sẽ có tranh — không cần sửa dòng code nào.\n")

	for _, r := range rows {
		fmt.Fprintf(&b, "\n## Trang %d · Khung %d\n\n", r.Page, r.Panel)
		fmt.Fprintf(&b, "- **Tệp ảnh:** `%s`\n", r.ArtFile)
		fmt.Fprintf(&b, "- **Tỉ lệ:** %s · **Độ phân giải:** %s\n", r.Aspect, r.Size)
		if len(r.Refs) > 0 {
			fmt.Fprintf(&b, "- **Ảnh tham chiếu:** %s\n", strings.Join(r.Refs, ", "))
		}
		writePrompt(&b, "Prompt", r.Prompt)
		writePrompt(&b, "Negative", r.Negative)
	}
	return b.String()
}

// writeChapterIndex ghi mục lục một chương vào gói lồng tap-VV/chuong-NNN/.
func (rc *runCtx) writeChapterIndex(ch, volIndex int, script *ScriptResult) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Chương %d — %s\n\n", ch, titleOr(script.Title, "Không tên"))
	fmt.Fprintf(&b, "%d trang truyện tranh.\n\n", len(script.Pages))
	for _, p := range script.Pages {
		rel := fmt.Sprintf("../../trang/chuong-%03d/%02d.png", ch, p.PageNo)
		fmt.Fprintf(&b, "## Trang %d\n\n![Trang %d](%s)\n\n", p.PageNo, p.PageNo, rel)
		if p.Beat != "" {
			fmt.Fprintf(&b, "*%s*\n\n", p.Beat)
		}
	}
	dir := fmt.Sprintf("tap-%02d/chuong-%03d", maxInt(volIndex, 1), ch)
	if err := rc.writeAlways(ProductPage, dir+"/muc-luc.md", []byte(b.String())); err != nil {
		return err
	}
	// Kịch bản chương cũng được sao vào gói cho tiện đọc offline.
	if data, err := rc.readOut(fmt.Sprintf("kich-ban/%02d.md", ch)); err == nil {
		return rc.writeAlways(ProductPage, dir+"/kich-ban-tranh.md", data)
	}
	return nil
}

// writeIndex ghi mục lục toàn bộ sản phẩm.
func (rc *runCtx) writeIndex() error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Truyện tranh — %s\n\n", titleOr(rc.bible.NovelName, "Không tên"))
	fmt.Fprintf(&b, "- **Phong cách:** %s\n", rc.bible.Preset.Label)
	if rc.bible.StyleHint != "" {
		fmt.Fprintf(&b, "- **Tinh chỉnh:** %s\n", rc.bible.StyleHint)
	}
	fmt.Fprintf(&b, "- **Khổ trang:** %s (%d×%d px ở %d DPI, tràn lề %gmm)\n",
		strings.ToUpper(orDefault(rc.opts.PageSize, "a4")),
		rc.spec.PxW(), rc.spec.PxH(), rc.spec.DPI, float64(rc.spec.Bleed))

	pages := rc.countPages()
	fmt.Fprintf(&b, "- **Tổng số trang đã dựng:** %d\n", pages)

	missing := rc.countMissingArt()
	if missing > 0 {
		fmt.Fprintf(&b, "\n> ⚠ Còn **%d khung chưa có tranh** — các khung đó đang dùng ô giữ chỗ.\n"+
			"> Xem `prompts/` để biết prompt và đường dẫn tệp ảnh cần đặt.\n", missing)
	}

	b.WriteString("\n## Thư mục\n\n")
	b.WriteString("| Thư mục | Nội dung |\n|---|---|\n")
	b.WriteString("| `style/` | Định hướng mỹ thuật, token phong cách đã khoá |\n")
	b.WriteString("| `nhan-vat/` | Model sheet nhân vật (JSON + ảnh nếu đã sinh) |\n")
	b.WriteString("| `kich-ban/` | Kịch bản truyện tranh từng chương (trang → khung) |\n")
	b.WriteString("| `bo-cuc/` | Hình học từng trang (toạ độ khung, vùng bong bóng) |\n")
	b.WriteString("| `prompts/` | Bảng prompt khung — hợp đồng để sinh/vẽ tranh |\n")
	b.WriteString("| `art/` | Tranh từng khung |\n")
	b.WriteString("| `trang/` | **Trang đã dàn** — PNG để xem/in, SVG để sửa tay |\n")
	b.WriteString("| `xuat-ban/` | Gói xuất bản: PDF, CBZ, EPUB |\n")
	b.WriteString("| `tap-*/` | Đóng gói lồng theo tập → chương |\n")

	return rc.writeAlways(ProductPage, "index.md", []byte(b.String()))
}

// countPages đếm số trang PNG đã dựng.
func (rc *runCtx) countPages() int {
	files, _ := rc.globPages()
	return len(files)
}

// countMissingArt đếm số khung có prompt nhưng chưa có tệp ảnh.
func (rc *runCtx) countMissingArt() int {
	rows := rc.allPanelPrompts()
	n := 0
	for _, r := range rows {
		if !exists(rc.path(r.ArtFile)) {
			n++
		}
	}
	return n
}

// allPanelPrompts gom bảng prompt của mọi chương, sắp theo chương → trang → khung.
func (rc *runCtx) allPanelPrompts() []PanelPrompt {
	var out []PanelPrompt
	entries, err := readDirNames(rc.path("prompts"))
	if err != nil {
		return nil
	}
	for _, name := range entries {
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		var rows []PanelPrompt
		if rc.loadArtifact("prompts/"+name, &rows) {
			out = append(out, rows...)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Chapter != b.Chapter {
			return a.Chapter < b.Chapter
		}
		if a.Page != b.Page {
			return a.Page < b.Page
		}
		return a.Panel < b.Panel
	})
	return out
}

// --- tiện ích trình bày ---

func titleOr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func writeField(b *strings.Builder, label, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fmt.Fprintf(b, "- **%s:** %s\n", label, value)
}

func writeList(b *strings.Builder, label string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "- **%s:** %s\n", label, strings.Join(items, " · "))
}

func writePrompt(b *strings.Builder, label, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fmt.Fprintf(b, "- **%s:**\n\n  ```\n  %s\n  ```\n", label, value)
}

func sizeLabel(s string) string {
	switch s {
	case "nho":
		return "khung nhỏ"
	case "lon":
		return "khung lớn"
	case "tran-trang":
		return "tràn trang"
	default:
		return "khung vừa"
	}
}

func shotLabel(s string) string {
	switch s {
	case "toan":
		return "toàn cảnh"
	case "trung":
		return "trung cảnh"
	case "can":
		return "cận cảnh"
	case "dac-ta":
		return "đặc tả"
	default:
		return s
	}
}

func balloonLabel(kind string) string {
	switch kind {
	case "doc-thoai":
		return "Độc thoại"
	case "het":
		return "Hét"
	case "thi-tham":
		return "Thì thầm"
	case "thuyet-minh":
		return "Thuyết minh"
	default:
		return "Thoại"
	}
}

package comic

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"html"
	"image"
	"image/jpeg"
	"os"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/comicdraw"
)

// runPublish đóng gói các trang đã dựng thành sản phẩm xuất bản.
func runPublish(ctx context.Context, rc *runCtx) error {
	pages, err := rc.globPages()
	if err != nil || len(pages) == 0 {
		rc.emit(Event{Stage: StagePublish, Message: "Chưa có trang nào để đóng gói"})
		return nil
	}
	name := sanitizeFileName(titleOr(rc.bible.NovelName, "truyen-tranh"))

	for _, f := range rc.formats() {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch f {
		case FormatPNG, FormatSVG:
			// Đã có sẵn từ bước dàn trang.
			continue
		case FormatCBZ:
			rc.emit(Event{Stage: StagePublish, Product: ProductPublish, Message: "Đóng gói CBZ..."})
			data, err := buildCBZ(pages, rc.bible.NovelName, len(pages))
			if err != nil {
				return err
			}
			if err := rc.writeAlways(ProductPublish, "xuat-ban/"+name+".cbz", data); err != nil {
				return err
			}
		case FormatPDF:
			rc.emit(Event{Stage: StagePublish, Product: ProductPublish,
				Message: fmt.Sprintf("Đóng gói PDF in ấn %d DPI (%d trang)...", rc.spec.DPI, len(pages))})
			data, err := buildPDF(pages, rc.spec, rc.bible.NovelName)
			if err != nil {
				return err
			}
			if err := rc.writeAlways(ProductPublish, "xuat-ban/"+name+".pdf", data); err != nil {
				return err
			}
		case FormatEPUB:
			rc.emit(Event{Stage: StagePublish, Product: ProductPublish, Message: "Đóng gói EPUB3 fixed-layout..."})
			data, err := buildEPUB(pages, rc.spec, rc.bible.NovelName)
			if err != nil {
				return err
			}
			if err := rc.writeAlways(ProductPublish, "xuat-ban/"+name+".epub", data); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------- CBZ

// buildCBZ đóng gói các trang PNG thành tệp CBZ (zip chứa ảnh đánh số).
func buildCBZ(pages []string, novel string, count int) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// Mốc thời gian CỐ ĐỊNH để chạy lại cho ra tệp giống hệt (tiện so sánh, tiện test).
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i, p := range pages {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		// Đánh số 3 chữ số: nhiều trình đọc sắp xếp theo thứ tự ASCII, "10.png" mà đứng
		// trước "2.png" là lỗi thật chứ không phải tiểu tiết.
		h := &zip.FileHeader{Name: fmt.Sprintf("%03d.png", i+1), Method: zip.Store, Modified: fixed}
		w, err := zw.CreateHeader(h)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
	}

	// ComicInfo.xml — lược đồ ComicRack, được Komga/Kavita/YACReader đọc.
	info := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<ComicInfo xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <Series>%s</Series>
  <Title>%s</Title>
  <PageCount>%d</PageCount>
  <LanguageISO>vi</LanguageISO>
  <Manga>No</Manga>
</ComicInfo>`, html.EscapeString(novel), html.EscapeString(novel), count)
	h := &zip.FileHeader{Name: "ComicInfo.xml", Method: zip.Deflate, Modified: fixed}
	w, err := zw.CreateHeader(h)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write([]byte(info)); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---------------------------------------------------------------- PDF

// jpegQuality cho bản in. 92 là mức nhà in chấp nhận mà tệp không phình vô lý.
const jpegQuality = 92

// buildPDF dựng PDF 1.7, mỗi trang một ảnh XObject tràn kín khổ.
//
// ⚠ PDF KHÔNG có thuộc tính DPI. Kích thước vật lý hoàn toàn do ma trận biến đổi trong
// content stream quyết định: "W 0 0 H 0 0 cm" trải ảnh lên hình chữ nhật W×H point.
// DPI hiệu dụng = pixel / (point/72). Toàn bộ yêu cầu "300 DPI in được" nằm ở đúng dòng đó.
//
// Ảnh JPEG được nhét thẳng qua /DCTDecode, KHÔNG giải nén rồi nén lại.
func buildPDF(pages []string, spec comicdraw.PageSpec, novel string) ([]byte, error) {
	// Điểm (point) = pixel / DPI * 72.
	wPt := float64(spec.PxW()) / float64(spec.DPI) * 72
	hPt := float64(spec.PxH()) / float64(spec.DPI) * 72
	bleedPt := float64(spec.Bleed) / 25.4 * 72

	var buf bytes.Buffer
	offsets := map[int]int{}
	obj := func(id int, body string) {
		offsets[id] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", id, body)
	}

	buf.WriteString("%PDF-1.7\n")
	// Bốn byte ≥128 đánh dấu đây là tệp nhị phân — công cụ truyền tệp dựa vào đó.
	buf.Write([]byte{'%', 0xE2, 0xE3, 0xCF, 0xD3, '\n'})

	n := len(pages)
	// id: 1 catalog, 2 pages, rồi mỗi trang 3 object (page, image, content).
	pageIDs := make([]int, n)
	for i := 0; i < n; i++ {
		pageIDs[i] = 3 + 3*i
	}
	infoID := 3 + 3*n

	obj(1, "<< /Type /Catalog /Pages 2 0 R >>")

	kids := make([]string, n)
	for i, id := range pageIDs {
		kids[i] = fmt.Sprintf("%d 0 R", id)
	}
	obj(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), n))

	for i, p := range pages {
		pageID := pageIDs[i]
		imgID, contID := pageID+1, pageID+2

		img, err := loadImageFile(p)
		if err != nil {
			return nil, fmt.Errorf("đọc trang %s: %w", p, err)
		}
		var jbuf bytes.Buffer
		if err := jpeg.Encode(&jbuf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return nil, err
		}
		b := img.Bounds()

		obj(pageID, fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /BleedBox [0 0 %.2f %.2f] "+
				"/TrimBox [%.2f %.2f %.2f %.2f] /Resources << /XObject << /Im0 %d 0 R >> >> /Contents %d 0 R >>",
			wPt, hPt, wPt, hPt,
			bleedPt, bleedPt, wPt-bleedPt, hPt-bleedPt, imgID, contID))

		// Ảnh: dict + stream nhị phân.
		offsets[imgID] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n<< /Type /XObject /Subtype /Image /Width %d /Height %d "+
			"/ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n",
			imgID, b.Dx(), b.Dy(), jbuf.Len())
		buf.Write(jbuf.Bytes())
		buf.WriteString("\nendstream\nendobj\n")

		content := fmt.Sprintf("q %.2f 0 0 %.2f 0 0 cm /Im0 Do Q", wPt, hPt)
		obj(contID, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
	}

	obj(infoID, fmt.Sprintf("<< /Title %s /Producer (ainovel-cli) /CreationDate (D:20260101000000+07'00') >>",
		pdfTextUTF16(novel)))

	// Bảng xref: mỗi mục ĐÚNG 20 byte, kể cả dấu cách trước xuống dòng.
	size := infoID + 1
	xrefPos := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", size)
	buf.WriteString("0000000000 65535 f \n")
	for id := 1; id < size; id++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[id])
	}
	docID := pdfDocID(novel, n)
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R /Info %d 0 R /ID [<%s> <%s>] >>\nstartxref\n%d\n%%%%EOF\n",
		size, infoID, docID, docID, xrefPos)
	return buf.Bytes(), nil
}

// pdfTextUTF16 mã hoá chuỗi thành hex UTF-16BE có BOM.
// Chuỗi literal trong PDF mặc định là PDFDocEncoding nên "(Cờ vua)" sẽ hỏng dấu — đây là
// lỗi tiếng Việt hay gặp nhất khi tự sinh PDF.
func pdfTextUTF16(s string) string {
	var b strings.Builder
	b.WriteString("<FEFF")
	for _, r := range s {
		if r > 0xFFFF {
			r -= 0x10000
			fmt.Fprintf(&b, "%04X%04X", 0xD800+(r>>10), 0xDC00+(r&0x3FF))
			continue
		}
		fmt.Fprintf(&b, "%04X", r)
	}
	b.WriteString(">")
	return b.String()
}

// pdfDocID sinh /ID ổn định từ tên sách + số trang (nhiều RIP đòi trường này).
func pdfDocID(novel string, pages int) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%d", novel, pages)))
	return strings.ToUpper(hex.EncodeToString(sum[:16]))
}

func loadImageFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

// ---------------------------------------------------------------- EPUB3 fixed-layout

// buildEPUB dựng EPUB3 fixed-layout cho truyện tranh.
//
// Khác bản EPUB chữ ở internal/host/exp/epub.go: có rendition:layout pre-paginated, mỗi
// trang một XHTML với viewport ĐÚNG BẰNG kích thước pixel của ảnh, và ảnh bọc trong <svg>
// để tỉ lệ luôn đúng trên mọi màn hình — chính vì bọc svg nên manifest phải khai
// properties="svg", thiếu là epubcheck báo lỗi.
func buildEPUB(pages []string, spec comicdraw.PageSpec, novel string) ([]byte, error) {
	// Ảnh cho màn hình dùng khổ THÀNH PHẨM, không có tràn lề: tràn lề là thứ của nhà in,
	// để nguyên thì mỗi trang trên máy đọc sẽ thừa 3mm viền chết.
	trimW, trimH := spec.TrimPxW(), spec.TrimPxH()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// mimetype PHẢI là mục đầu tiên và KHÔNG nén.
	mt, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store, Modified: fixed})
	if err != nil {
		return nil, err
	}
	if _, err := mt.Write([]byte("application/epub+zip")); err != nil {
		return nil, err
	}

	add := func(name string, data []byte) error {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: fixed})
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}

	if err := add("META-INF/container.xml", []byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`)); err != nil {
		return nil, err
	}
	if err := add("OEBPS/style.css", []byte("html,body{margin:0;padding:0;height:100%}\nsvg{display:block}\n")); err != nil {
		return nil, err
	}

	var manifest, spine, nav strings.Builder
	for i, p := range pages {
		img, err := loadImageFile(p)
		if err != nil {
			return nil, err
		}
		// Cắt bỏ tràn lề nếu ảnh đúng khổ có bleed.
		if sub, ok := img.(interface {
			SubImage(image.Rectangle) image.Image
		}); ok && img.Bounds().Dx() == spec.PxW() {
			b := int(spec.BleedPx() + 0.5)
			img = sub.SubImage(image.Rect(b, b, b+trimW, b+trimH))
		}
		var jbuf bytes.Buffer
		if err := jpeg.Encode(&jbuf, img, &jpeg.Options{Quality: 88}); err != nil {
			return nil, err
		}
		w, h := img.Bounds().Dx(), img.Bounds().Dy()

		imgName := fmt.Sprintf("images/trang-%03d.jpg", i+1)
		pgName := fmt.Sprintf("trang-%03d.xhtml", i+1)
		if err := add("OEBPS/"+imgName, jbuf.Bytes()); err != nil {
			return nil, err
		}

		xhtml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" xml:lang="vi">
<head><meta charset="utf-8"/><title>Trang %d</title>
<meta name="viewport" content="width=%d, height=%d"/>
<link rel="stylesheet" type="text/css" href="style.css"/></head>
<body><svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"
 version="1.1" viewBox="0 0 %d %d" width="100%%" height="100%%" preserveAspectRatio="xMidYMid meet">
<image width="%d" height="%d" xlink:href="%s"/></svg></body></html>`,
			i+1, w, h, w, h, w, h, imgName)
		if err := add("OEBPS/"+pgName, []byte(xhtml)); err != nil {
			return nil, err
		}

		coverProp := ""
		if i == 0 {
			coverProp = ` properties="cover-image"`
		}
		fmt.Fprintf(&manifest, `    <item id="img%03d" href="%s" media-type="image/jpeg"%s/>`+"\n", i+1, imgName, coverProp)
		fmt.Fprintf(&manifest, `    <item id="pg%03d" href="%s" media-type="application/xhtml+xml" properties="svg"/>`+"\n", i+1, pgName)

		spread := "page-spread-right"
		if i%2 == 1 {
			spread = "page-spread-left"
		}
		fmt.Fprintf(&spine, `    <itemref idref="pg%03d" properties="rendition:layout-pre-paginated %s"/>`+"\n", i+1, spread)
		fmt.Fprintf(&nav, `      <li><a href="%s">Trang %d</a></li>`+"\n", pgName, i+1)
	}

	navDoc := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" xml:lang="vi">
<head><meta charset="utf-8"/><title>Mục lục</title></head>
<body><nav epub:type="toc" id="toc"><h1>Mục lục</h1><ol>
%s    </ol></nav></body></html>`, nav.String())
	if err := add("OEBPS/nav.xhtml", []byte(navDoc)); err != nil {
		return nil, err
	}

	opf := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="bookid" xml:lang="vi"
 prefix="rendition: http://www.idpf.org/vocab/rendition/#">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="bookid">urn:uuid:%s</dc:identifier>
    <dc:title>%s</dc:title>
    <dc:language>vi</dc:language>
    <meta property="dcterms:modified">2026-01-01T00:00:00Z</meta>
    <meta property="rendition:layout">pre-paginated</meta>
    <meta property="rendition:orientation">portrait</meta>
    <meta property="rendition:spread">auto</meta>
    <meta name="cover" content="img001"/>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="css" href="style.css" media-type="text/css"/>
%s  </manifest>
  <spine page-progression-direction="ltr">
%s  </spine>
</package>`, epubUUID(novel, len(pages)), html.EscapeString(novel), manifest.String(), spine.String())
	if err := add("OEBPS/content.opf", []byte(opf)); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// epubUUID sinh định danh ổn định từ tên sách (giống exp/epub.go:bookIdentifier).
func epubUUID(novel string, pages int) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("comic|%s|%d", novel, pages)))
	h := hex.EncodeToString(sum[:16])
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}

// sanitizeFileName lọc tên tệp an toàn trên mọi hệ điều hành, giữ dấu tiếng Việt.
func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	repl := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "*", "-", "?", "-",
		"\"", "-", "<", "-", ">", "-", "|", "-", "\n", " ", "\r", " ")
	out := strings.TrimSpace(repl.Replace(name))
	out = strings.Trim(out, " .")
	if out == "" {
		return "truyen-tranh"
	}
	return out
}

package exp

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// chapterTitleIndex Tra ve tieu de theo so chuong, neu khong co tra ve chuoi rong.
type chapterTitleIndex map[int]string

func buildTitleIndex(outline []domain.OutlineEntry) chapterTitleIndex {
	idx := make(chapterTitleIndex, len(outline))
	for _, e := range outline {
		if e.Title != "" {
			idx[e.Chapter] = e.Title
		}
	}
	return idx
}

// chapterLocation là vị trí của một chương trong đề cương phân tầng. Chỉ giữ lại thông tin tập cần cho định dạng xuất --
// cung không vào xuất (cung là cấu trúc nội bộ, qua chi tiết từ góc nhìn độc giả).
type chapterLocation struct {
	VolumeIdx       int
	VolumeTitle     string
	IsFirstOfVolume bool
}

// buildLocations Xay dung {chapter -> location} theo thu tu chuong toan cuc cua de cuong phan tang.
// So chuong duoc xay lai theo cung quy tac voi FlattenOutline (tich luy theo thu tu trong cung cua tap),
// de giu nhat quan voi so chuong cua Progress.CompletedChapters. Tang cung van phai duyet (bat buoc de tinh so chuong toan cuc),
// nhung khong vao location -- xuat chi chen phan cach o dau tap.
func buildLocations(volumes []domain.VolumeOutline) map[int]chapterLocation {
	if len(volumes) == 0 {
		return nil
	}
	locs := make(map[int]chapterLocation)
	ch := 0
	for _, v := range volumes {
		firstOfVol := true
		for _, a := range v.Arcs {
			for range a.Chapters {
				ch++
				locs[ch] = chapterLocation{
					VolumeIdx:       v.Index,
					VolumeTitle:     v.Title,
					IsFirstOfVolume: firstOfVol,
				}
				firstOfVol = false
			}
		}
	}
	return locs
}

// chapterHeaderRe Khop dong tieu de Markdown dau tien co so chuong (# Chuong N / ## Chuong 12 ...).
var chapterHeaderRe = regexp.MustCompile(`^#+\s+第.+?章`)

// atxTitleRe Trich xuat phan van ban cua tieu de ATX (# tieu de).
var atxTitleRe = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*$`)

// stripChapterTitleHeader Bo dong dau tieu de neu dong do se trung lap voi tieu de thong nhat cua trinh xuat.
// Hai truong hop: (1) "# Chuong N ..." (co so chuong); (2) Tieu de markdown ma van ban chinh la tieu de chuong nay
// (writer thuong viet ten chuong thuan tuy lam tieu de vao dong dau noi dung, nhu "# lang que yeu thuong", trung lap voi
// "Chuong N lang que yeu thuong" do trinh xuat tao ra). Cac h1 khac (nhu "# Loi mo dau") duoc coi la mot phan noi dung, giu lai.
// Caller chiu trach nhiem TrimSpace truoc, nen cac dong trong dau khong nam trong pham vi xem xet.
func stripChapterTitleHeader(content, title string) string {
	first, rest, hasNewline := strings.Cut(content, "\n")
	if !isChapterTitleLine(first, title) {
		return content
	}
	if !hasNewline {
		return ""
	}
	return strings.TrimLeft(rest, "\n")
}

func isChapterTitleLine(line, title string) bool {
	if chapterHeaderRe.MatchString(line) {
		return true
	}
	if title = strings.TrimSpace(title); title == "" {
		return false
	}
	m := atxTitleRe.FindStringSubmatch(line)
	return len(m) == 2 && strings.TrimSpace(m[1]) == title
}

// renderTXT Ghep noi van ban cuoi cung.
//
// Thu tu chuong do chapters quyet dinh (caller da sap xep tang dan theo so chuong va loai trung lap). bodies/titleIdx/locations
// deu xu ly theo "khong co thi giam cap": thieu tieu de chi xuat "Chuong N"; thieu vi tri phan tang thi coi nhu de cuong phang.
func renderTXT(
	novelName string,
	chapters []int,
	titleIdx chapterTitleIndex,
	locations map[int]chapterLocation,
	bodies map[int]string,
) string {
	var b strings.Builder

	if name := strings.TrimSpace(novelName); name != "" {
		b.WriteString("《")
		b.WriteString(name)
		b.WriteString("》\n\n")
	}

	useLayered := len(locations) > 0

	for i, ch := range chapters {
		if useLayered {
			if loc, ok := locations[ch]; ok && loc.IsFirstOfVolume {
				b.WriteString("\n═══════════════════════════════════════════\n")
				fmt.Fprintf(&b, "           Tap %d  %s\n", loc.VolumeIdx, strings.TrimSpace(loc.VolumeTitle))
				b.WriteString("═══════════════════════════════════════════\n\n")
			}
		}

		title := strings.TrimSpace(titleIdx[ch])
		if title != "" {
			fmt.Fprintf(&b, "Chuong %d  %s\n\n", ch, title)
		} else {
			fmt.Fprintf(&b, "Chuong %d\n\n", ch)
		}

		body := stripChapterTitleHeader(strings.TrimSpace(bodies[ch]), title)
		b.WriteString(body)
		b.WriteString("\n")
		if i < len(chapters)-1 {
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

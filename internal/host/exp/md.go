package exp

import (
	"fmt"
	"strings"
)

// renderMD Ghep noi cac chuong thanh Markdown.
//
// Cau truc song song voi renderTXT (txt.go) nhung dung tieu de ATX:
//   - Ten sach   -> "# Ten"
//   - Dau tap    -> "## Tap N — Title" (chi khi de cuong phan tang)
//   - Moi chuong -> "## Chuong N — Title" (hoac "## Chuong N")
//
// Thu tu chuong do chapters quyet dinh; bodies/titleIdx/locations xu ly theo "khong co thi
// giam cap". Body duoc TrimSpace + stripChapterTitleHeader de tranh lap tieu de chuong.
func renderMD(
	novelName string,
	chapters []int,
	titleIdx chapterTitleIndex,
	locations map[int]chapterLocation,
	bodies map[int]string,
) string {
	var b strings.Builder

	if name := strings.TrimSpace(novelName); name != "" {
		fmt.Fprintf(&b, "# %s\n\n", name)
	}

	useLayered := len(locations) > 0

	for i, ch := range chapters {
		if useLayered {
			if loc, ok := locations[ch]; ok && loc.IsFirstOfVolume {
				if vt := strings.TrimSpace(loc.VolumeTitle); vt != "" {
					fmt.Fprintf(&b, "## Tap %d — %s\n\n", loc.VolumeIdx, vt)
				} else {
					fmt.Fprintf(&b, "## Tap %d\n\n", loc.VolumeIdx)
				}
			}
		}

		title := strings.TrimSpace(titleIdx[ch])
		if title != "" {
			fmt.Fprintf(&b, "## Chuong %d — %s\n\n", ch, title)
		} else {
			fmt.Fprintf(&b, "## Chuong %d\n\n", ch)
		}

		body := stripChapterTitleHeader(strings.TrimSpace(bodies[ch]), title)
		b.WriteString(body)
		b.WriteString("\n")
		if i < len(chapters)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

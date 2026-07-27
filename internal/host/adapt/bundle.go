package adapt

import (
	"fmt"
	"strings"
)

// bundleDir trả về đường dẫn tương đối thư mục đóng gói lồng của một chương:
// tap-{VV}/chuong-{NN} (VV = số tập, NN = số chương toàn cục).
func bundleDir(volIndex, chapter int) string {
	if volIndex <= 0 {
		volIndex = 1
	}
	return fmt.Sprintf("tap-%02d/chuong-%03d", volIndex, chapter)
}

// writeChapterBundle gom đủ mọi loại file của MỘT chương vào tap-{VV}/chuong-{NN}/:
// kịch bản, phân cảnh (json+md), animation, prompt ảnh/video của riêng chương.
// Render thuần từ kết quả có sẵn (không gọi LLM). Tôn trọng Overwrite. sp/sb có thể nil.
func writeChapterBundle(rc *runCtx, ch, volIndex int, sp *ScreenplayResult, sb *StoryboardResult) error {
	dir := bundleDir(volIndex, ch)
	if sp != nil {
		md := renderScreenplayMarkdown(ch, sp.Title, *sp)
		if _, err := rc.write(ProductScreenplay, dir+"/kich-ban.md", []byte(md)); err != nil {
			return err
		}
	} else if data, ok := rc.readOut(fmt.Sprintf("screenplay/%02d.md", ch)); ok {
		// Không sinh kịch bản lần này nhưng đã có sẵn trên đĩa → sao chép vào gói.
		if _, err := rc.write(ProductScreenplay, dir+"/kich-ban.md", data); err != nil {
			return err
		}
	}
	if sb == nil {
		sb = rc.loadStoryboardForChapter(ch)
	}
	if sb != nil {
		if _, err := rc.writeJSON(ProductStoryboard, dir+"/phan-canh.json", *sb); err != nil {
			return err
		}
		if _, err := rc.write(ProductStoryboard, dir+"/phan-canh.md", []byte(renderStoryboardMarkdown(*sb))); err != nil {
			return err
		}
		if _, err := rc.write(ProductAnimation, dir+"/animation.md", []byte(renderAnimationMarkdown(*sb))); err != nil {
			return err
		}
		if _, err := rc.write(ProductImagePrompt, dir+"/prompt-anh.md", []byte(chapterImagePromptMarkdown(*sb))); err != nil {
			return err
		}
		if _, err := rc.write(ProductVideoPrompt, dir+"/prompt-video.md", []byte(chapterVideoPromptMarkdown(*sb))); err != nil {
			return err
		}
	}
	return nil
}

// writeVolumeIndex ghi mục lục một tập: tap-{VV}/_tap-{VV}.md liệt kê các chương + liên kết
// tương đối tới thư mục đóng gói từng chương (để dựng video cả tập).
func writeVolumeIndex(rc *runCtx, vg volumeGroup) error {
	var b strings.Builder
	title := vg.Title
	if strings.TrimSpace(title) == "" {
		title = fmt.Sprintf("Tập %d", vg.Index)
	} else {
		title = fmt.Sprintf("Tập %d — %s", vg.Index, title)
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "Gồm %d chương. Mỗi chương có thư mục đóng gói đủ liệu làm video.\n\n", len(vg.Chapters))
	for _, ch := range vg.Chapters {
		entry := rc.bible.outlineFor(ch)
		fmt.Fprintf(&b, "- [Chương %d — %s](chuong-%03d/)\n", ch, titleOr(entry.Title), ch)
	}
	rel := fmt.Sprintf("tap-%02d/_tap-%02d.md", vg.Index, vg.Index)
	if _, err := rc.write(ProductStoryboard, rel, []byte(b.String())); err != nil {
		return err
	}
	return nil
}

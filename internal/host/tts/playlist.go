package tts

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// writeIndexes ghi playlist.m3u cho từng tập, playlist.m3u cả sách và index.md.
//
// Ba tệp này LUÔN ghi đè: chúng là bản tóm tắt dẫn xuất, không phải sản phẩm tốn tiền —
// cờ Overwrite chỉ bảo vệ tệp âm thanh. Nhờ vậy một lần chạy resume vẫn dựng lại được
// danh sách phát đầy đủ thay vì làm nó ngắn đi.
func (rc *runCtx) writeIndexes(groups []volumeGroup) error {
	if len(rc.outputs) == 0 {
		return nil
	}
	rc.emit(Event{Stage: StageIndex, Message: "Ghi danh sách phát và mục lục..."})

	outs := sortedOutputs(rc.outputs)

	// Danh sách phát cả sách.
	if _, err := atomicWrite(filepath.Join(rc.outDir, "playlist.m3u"),
		[]byte(renderPlaylist(rc.book.NovelName, outs, ""))); err != nil {
		return err
	}

	// Danh sách phát từng tập (đường dẫn tương đối so với chính thư mục tập).
	byVolume := map[int][]Output{}
	for _, o := range outs {
		byVolume[o.Volume] = append(byVolume[o.Volume], o)
	}
	for _, g := range groups {
		items := byVolume[g.Index]
		if len(items) == 0 {
			continue
		}
		name := rc.book.NovelName
		if g.Title != "" {
			name = fmt.Sprintf("%s — Tập %d: %s", rc.book.NovelName, g.Index, g.Title)
		}
		prefix := fmt.Sprintf("tap-%02d/", g.Index)
		path := filepath.Join(rc.outDir, fmt.Sprintf("tap-%02d", g.Index), "playlist.m3u")
		if _, err := atomicWrite(path, []byte(renderPlaylist(name, items, prefix))); err != nil {
			return err
		}
	}

	// Mục lục.
	md := rc.renderIndex(groups, outs)
	if _, err := atomicWrite(filepath.Join(rc.outDir, "index.md"), []byte(md)); err != nil {
		return err
	}
	return nil
}

// sortedOutputs sắp theo tập → chương → phần, để danh sách phát đúng thứ tự nghe.
func sortedOutputs(in []Output) []Output {
	out := append([]Output(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Volume != b.Volume {
			return a.Volume < b.Volume
		}
		if a.Chapter != b.Chapter {
			return a.Chapter < b.Chapter
		}
		return a.Part < b.Part
	})
	return out
}

// renderPlaylist dựng nội dung Extended M3U (UTF-8 không BOM, xuống dòng \n).
//
// Thời lượng để -1 (không rõ): công cụ không phân tích khung MP3 và không dùng ffmpeg;
// mọi trình phát phổ thông đều chấp nhận giá trị này.
//
// stripPrefix cắt bớt tiền tố thư mục để danh sách phát cấp tập trỏ đúng tệp bên cạnh nó.
func renderPlaylist(name string, outs []Output, stripPrefix string) string {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	if name != "" {
		fmt.Fprintf(&b, "#PLAYLIST:%s\n", name)
	}
	for _, o := range outs {
		fmt.Fprintf(&b, "#EXTINF:-1,%s\n", trackTitle(o))
		b.WriteString(strings.TrimPrefix(o.Rel, stripPrefix))
		b.WriteString("\n")
	}
	return b.String()
}

// trackTitle dựng nhãn hiển thị của một bài trong danh sách phát.
func trackTitle(o Output) string {
	s := fmt.Sprintf("Chương %d", o.Chapter)
	if o.Title != "" {
		s += " — " + o.Title
	}
	if o.Part > 0 {
		s += fmt.Sprintf(" (phần %d)", o.Part)
	}
	return s
}

// renderIndex dựng index.md — mục lục kèm số ký tự đã gửi Vbee để đối chiếu tín dụng.
func (rc *runCtx) renderIndex(groups []volumeGroup, outs []Output) string {
	var b strings.Builder

	name := rc.book.NovelName
	if name == "" {
		name = "Sách nói"
	}
	fmt.Fprintf(&b, "# Sách nói — %s\n\n", name)

	totalChars, totalBytes := 0, 0
	chapters := map[int]bool{}
	for _, o := range outs {
		totalChars += o.Chars
		totalBytes += o.Bytes
		chapters[o.Chapter] = true
	}

	fmt.Fprintf(&b, "- **Giọng đọc:** `%s`\n", rc.opts.VoiceCode)
	fmt.Fprintf(&b, "- **Tốc độ:** %g · **Định dạng:** %s · **Bitrate:** %d kbps · **Tần số:** %d Hz\n",
		rc.opts.Speed, rc.opts.OutputFormat, rc.opts.Bitrate, rc.opts.SampleRate)
	fmt.Fprintf(&b, "- **Tạo lúc:** %s\n", time.Now().Format("02/01/2006 15:04"))
	fmt.Fprintf(&b, "- **Số chương:** %d thành công, %d bỏ qua\n", len(chapters), len(rc.skipped))
	fmt.Fprintf(&b, "- **Tổng ký tự đã gửi Vbee:** %d (dùng để đối chiếu tín dụng bị trừ)\n", totalChars)
	fmt.Fprintf(&b, "- **Tổng dung lượng:** %s\n", humanSize(totalBytes))
	b.WriteString("- **Danh sách phát:** [playlist.m3u](playlist.m3u)\n\n")
	b.WriteString("> Thời lượng từng tệp không được đo (công cụ không dùng ffmpeg).\n")

	byVolume := map[int][]Output{}
	for _, o := range outs {
		byVolume[o.Volume] = append(byVolume[o.Volume], o)
	}
	for _, g := range groups {
		items := byVolume[g.Index]
		if len(items) == 0 {
			continue
		}
		if g.Title != "" {
			fmt.Fprintf(&b, "\n## Tập %d: %s\n\n", g.Index, g.Title)
		} else {
			fmt.Fprintf(&b, "\n## Tập %d\n\n", g.Index)
		}
		b.WriteString("| Chương | Tên chương | Tệp | Ký tự | Dung lượng |\n")
		b.WriteString("|---:|---|---|---:|---:|\n")
		for _, o := range items {
			label := fmt.Sprintf("%d", o.Chapter)
			if o.Part > 0 {
				label = fmt.Sprintf("%d.%d", o.Chapter, o.Part)
			}
			fmt.Fprintf(&b, "| %s | %s | [%s](%s) | %d | %s |\n",
				label, o.Title, o.Rel, o.Rel, o.Chars, humanSize(o.Bytes))
		}
	}

	if len(rc.skipped) > 0 {
		b.WriteString("\n## Chương bị bỏ qua\n\n")
		for _, s := range rc.skipped {
			if s.Title != "" {
				fmt.Fprintf(&b, "- Chương %d (%s) — %s\n", s.Chapter, s.Title, s.Reason)
			} else {
				fmt.Fprintf(&b, "- Chương %d — %s\n", s.Chapter, s.Reason)
			}
		}
	}
	return b.String()
}

// humanSize định dạng dung lượng cho người đọc.
func humanSize(n int) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

package tts

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Run khởi động một lần tạo sách nói từ các chương đã hoàn thành.
// Trả về kênh Event (caller chịu trách nhiệm tiêu thụ; hủy ctx để dừng sạch).
//
// Giống adapt: nhiều bước, chỉ đọc store rồi ghi file ngoài. Khác ở chỗ chi phí là
// TÍN DỤNG VBEE tính theo số ký tự, nên các chương chạy TUẦN TỰ — mỗi lỗi đồng thời
// là tiền thật, và hủy giữa chừng vẫn bị tính mọi yêu cầu đang bay.
func Run(ctx context.Context, deps Deps, opts Options) (<-chan Event, error) {
	if deps.Store == nil || deps.Vbee == nil {
		return nil, fmt.Errorf("deps chưa đầy đủ")
	}
	opts = opts.normalize()
	if strings.TrimSpace(opts.VoiceCode) == "" {
		return nil, fmt.Errorf("chưa chọn giọng đọc")
	}

	events := make(chan Event, 32)
	go func() {
		defer close(events)
		emit := func(ev Event) {
			ev.Time = time.Now()
			// Ưu tiên gửi ngay khi bộ đệm còn chỗ. Nếu dùng thẳng select có nhánh
			// ctx.Done() (như adapt/runner.go:33-39) thì sau khi người dùng bấm Dừng,
			// select có thể chọn nhánh Done và NUỐT MẤT chính sự kiện kết thúc — giao
			// diện sẽ treo ở trạng thái "đang chạy" vì không bao giờ nhận done=true.
			select {
			case events <- ev:
				return
			default:
			}
			// Bộ đệm đầy: chờ có chỗ, nhưng vẫn thoát được khi ctx hủy để không kẹt.
			select {
			case events <- ev:
			case <-ctx.Done():
			}
		}

		emit(Event{Stage: StageContext, Message: "Đang đọc dữ liệu dự án..."})
		book, err := loadBookIndex(deps.Store)
		if err != nil {
			emit(Event{Stage: StageError, Message: "Đọc dữ liệu dự án thất bại", Err: err})
			return
		}

		outDir := opts.OutDir
		if outDir == "" {
			outDir = filepath.Join(deps.Store.Dir(), "audio")
		}
		rc := &runCtx{
			deps:   deps,
			book:   book,
			opts:   opts,
			timing: deps.Timing.normalize(),
			outDir: outDir,
			emit:   emit,
		}
		rc.run(ctx)
	}()
	return events, nil
}

// runCtx là trạng thái phạm vi một lần chạy.
type runCtx struct {
	deps   Deps
	book   *bookIndex
	opts   Options
	timing Timing
	outDir string
	emit   func(Event)

	outputs []Output
	skipped []Skipped
	// anySucceeded quyết định cách phân loại ErrBadRequest: trước khi có phần nào
	// thành công thì đó là lỗi cấu hình chung (dừng); sau đó là lỗi riêng chương (bỏ qua).
	anySucceeded bool
}

// emitJob phát một sự kiện gắn với một phần cụ thể, đã gắn sẵn nhãn "Chương N (phần P)".
func (rc *runCtx) emitJob(job partJob, stage Stage, msg string, err error) {
	label := fmt.Sprintf("Chương %d", job.Chapter)
	if job.Part > 0 {
		label = fmt.Sprintf("Chương %d (phần %d/%d)", job.Chapter, job.Part, job.Total)
	}
	rc.emit(Event{
		Stage: stage, Chapter: job.Chapter, Part: job.Part,
		Message: label + ": " + msg, Err: err,
	})
}

func (rc *runCtx) run(ctx context.Context) {
	groups, notDone := rc.book.volumeGroupsForRange(rc.opts.From, rc.opts.To)
	total := 0
	for _, g := range groups {
		total += len(g.Chapters)
	}
	if total == 0 {
		rc.emit(Event{Stage: StageError, Message: "Không có chương nào đã hoàn thành trong phạm vi đã chọn",
			Err: fmt.Errorf("phạm vi rỗng")})
		return
	}
	if len(notDone) > 0 {
		rc.emit(Event{Stage: StageContext, Total: total, Message: fmt.Sprintf(
			"Bỏ qua %d chương chưa hoàn thành trong phạm vi", len(notDone))})
	}
	rc.emit(Event{Stage: StageContext, Total: total, Message: fmt.Sprintf(
		"Sẽ đọc %d chương bằng giọng %s", total, rc.opts.VoiceCode)})

	done := 0
	for _, g := range groups {
		for _, ch := range g.Chapters {
			if err := ctx.Err(); err != nil {
				rc.emit(Event{Stage: StageError, Message: "Người dùng đã dừng tạo sách nói", Err: err})
				return
			}
			done++
			fatalErr := rc.doChapter(ctx, g, ch, done, total)
			if fatalErr != nil {
				if errors.Is(fatalErr, context.Canceled) || errors.Is(fatalErr, context.DeadlineExceeded) {
					rc.emit(Event{Stage: StageError, Chapter: ch, Message: "Người dùng đã dừng tạo sách nói", Err: fatalErr})
				} else {
					rc.emit(Event{Stage: StageError, Chapter: ch, Current: done, Total: total,
						Message: "Dừng tạo sách nói", Err: fatalErr})
				}
				return
			}
		}
	}

	if err := rc.writeIndexes(groups); err != nil {
		rc.emit(Event{Stage: StageError, Message: "Ghi danh sách phát / mục lục thất bại", Err: err})
		return
	}

	rc.finish(total)
}

// finish phát sự kiện kết thúc. Toàn bộ chương lỗi → StageError, để giao diện không
// hiện màu xanh "Hoàn thành" giả.
func (rc *runCtx) finish(total int) {
	okChapters := map[int]bool{}
	for _, o := range rc.outputs {
		okChapters[o.Chapter] = true
	}
	if len(okChapters) == 0 {
		rc.emit(Event{Stage: StageError, Total: total,
			Message: fmt.Sprintf("Không tạo được chương nào (%d chương lỗi)", len(rc.skipped)),
			Err:     fmt.Errorf("tất cả các chương đều thất bại")})
		return
	}
	msg := fmt.Sprintf("Hoàn thành sách nói: %d chương, %d tệp trong %s",
		len(okChapters), len(rc.outputs), rc.outDir)
	if len(rc.skipped) > 0 {
		msg += fmt.Sprintf(" (bỏ qua %d chương lỗi: %s)", len(rc.skipped), skippedList(rc.skipped))
	}
	rc.emit(Event{Stage: StageDone, Current: len(okChapters), Total: total, Message: msg})
}

func skippedList(items []Skipped) string {
	var b strings.Builder
	for i, s := range items {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%d", s.Chapter)
	}
	return b.String()
}

// doChapter xử lý trọn một chương. Trả về lỗi KHÁC NIL chỉ khi phải dừng cả run;
// lỗi riêng chương được ghi vào rc.skipped rồi đi tiếp (fail-soft giống adapt).
func (rc *runCtx) doChapter(ctx context.Context, g volumeGroup, chapter, current, total int) error {
	title := rc.book.titleFor(chapter)

	raw, err := rc.deps.Store.Drafts.LoadChapterText(chapter)
	if err != nil {
		rc.skip(chapter, title, "đọc bản thảo thất bại: "+err.Error(), current, total)
		return nil
	}
	clean := CleanChapter(chapter, title, raw, rc.opts.SceneBreakText)
	parts := SplitForTTS(clean, rc.opts.MaxChars)
	if len(parts) == 0 {
		rc.skip(chapter, title, "chương rỗng sau khi làm sạch", current, total)
		return nil
	}

	rc.emit(Event{Stage: StagePrepare, Chapter: chapter, Current: current, Total: total,
		Message: fmt.Sprintf("Chương %d: %s", chapter, describeParts(parts))})

	// Ghi tệp của một chương theo kiểu tất cả-hoặc-không: chỉ ghi sau khi MỌI phần đã
	// tải xong, để chương lỗi giữa chừng không để lại nửa bộ tệp lẫn vào danh sách phát.
	type pending struct {
		job  partJob
		data []byte
		rel  string
		path string
	}
	var ready []pending

	for i, text := range parts {
		job := partJob{Chapter: chapter, Total: len(parts), Text: text, Chars: runeLen(text)}
		if len(parts) > 1 {
			job.Part = i + 1
		}
		rel := rc.relPath(g.Index, chapter, job.Part)
		path := filepath.Join(rc.outDir, filepath.FromSlash(rel))

		if !rc.opts.Overwrite && exists(path) {
			rc.emitJob(job, StageWrite, "đã có tệp, bỏ qua", nil)
			rc.outputs = append(rc.outputs, Output{
				Chapter: chapter, Part: job.Part, Volume: g.Index, Title: title,
				Path: path, Rel: rel, Bytes: fileSize(path),
			})
			rc.anySucceeded = true
			continue
		}

		data, err := rc.synthesize(ctx, job)
		if err != nil {
			if isFatal(err) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			rc.skip(chapter, title, err.Error(), current, total)
			return nil
		}
		ready = append(ready, pending{job: job, data: data, rel: rel, path: path})
	}

	for _, p := range ready {
		n, err := atomicWrite(p.path, p.data)
		if err != nil {
			// Lỗi ghi đĩa là lỗi cục bộ, không phải lỗi của Vbee — dừng cả run
			// giống adapt, vì mọi chương sau cũng sẽ hỏng như thế.
			return fmt.Errorf("ghi %s: %w", p.path, err)
		}
		rc.outputs = append(rc.outputs, Output{
			Chapter: p.job.Chapter, Part: p.job.Part, Volume: g.Index, Title: title,
			Path: p.path, Rel: p.rel, Bytes: n, Chars: p.job.Chars,
		})
		rc.anySucceeded = true
		rc.emitJob(p.job, StageWrite, fmt.Sprintf("đã ghi %s", p.rel), nil)
	}

	rc.emit(Event{Stage: StageWrite, Chapter: chapter, Current: current, Total: total,
		Message: fmt.Sprintf("Xong chương %d/%d", current, total)})
	return nil
}

// skip ghi nhận một chương bị bỏ qua và báo ra giao diện.
func (rc *runCtx) skip(chapter int, title, reason string, current, total int) {
	rc.skipped = append(rc.skipped, Skipped{Chapter: chapter, Title: title, Reason: reason})
	rc.emit(Event{
		Stage: StageWrite, Chapter: chapter, Current: current, Total: total,
		Message: fmt.Sprintf("Bỏ qua chương %d (%s)", chapter, reason),
		Err:     fmt.Errorf("%s", reason),
	})
}

// relPath dựng đường dẫn tương đối của một tệp âm thanh. LUÔN dùng dấu / — giá trị này
// đi thẳng vào playlist.m3u, mà dấu \ của Windows sẽ làm danh sách phát mất tính di động.
func (rc *runCtx) relPath(volume, chapter, part int) string {
	name := fmt.Sprintf("chuong-%03d", chapter)
	if part > 0 {
		name += fmt.Sprintf("-p%d", part)
	}
	return fmt.Sprintf("tap-%02d/%s.%s", volume, name, rc.opts.OutputFormat)
}

func describeParts(parts []string) string {
	chars := 0
	for _, p := range parts {
		chars += runeLen(p)
	}
	if len(parts) == 1 {
		return fmt.Sprintf("%d ký tự", chars)
	}
	return fmt.Sprintf("%d ký tự, chia %d phần", chars, len(parts))
}

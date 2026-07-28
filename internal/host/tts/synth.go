package tts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/voocel/ainovel-cli/internal/vbee"
)

// fatalError bọc một lỗi khiến CẢ RUN phải dừng (sai thông tin xác thực, sai hình
// dạng yêu cầu khi chưa phần nào thành công). Phân biệt với lỗi thường — chỉ bỏ phần.
type fatalError struct{ err error }

func (e *fatalError) Error() string { return e.err.Error() }
func (e *fatalError) Unwrap() error { return e.err }

func fatal(err error) error { return &fatalError{err: err} }

// isFatal cho biết lỗi có buộc dừng cả run không.
func isFatal(err error) bool {
	var fe *fatalError
	return errors.As(err, &fe)
}

// sleepCtx ngủ d, hoặc thoát ngay khi ctx bị hủy. Đây là điểm DUY NHẤT vòng chờ có thể
// "kẹt", nên mọi chỗ chờ đều phải đi qua đây để nút Dừng / phím Esc phản hồi tức thì.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// partJob là một đơn vị công việc gửi Vbee: trọn một chương, hoặc một phần của chương
// quá dài.
type partJob struct {
	Chapter int
	Part    int // 0 = chương không chia phần
	Total   int // tổng số phần của chương này
	Text    string
	Chars   int
}

// synthesize thực hiện trọn vòng GỬI → CHỜ → TẢI cho một phần và trả về byte âm thanh.
//
// Trả về *fatalError khi lỗi buộc dừng cả run; lỗi thường nghĩa là "bỏ phần này, đi tiếp".
func (rc *runCtx) synthesize(ctx context.Context, job partJob) ([]byte, error) {
	requestID, err := rc.submit(ctx, job)
	if err != nil {
		return nil, err
	}
	link, err := rc.waitForAudio(ctx, job, requestID)
	if err != nil {
		return nil, err
	}
	return rc.download(ctx, job, requestID, link)
}

// submit gửi yêu cầu, thử lại có lùi với lỗi tạm thời.
func (rc *runCtx) submit(ctx context.Context, job partJob) (string, error) {
	req := vbee.SpeechRequest{
		Text:         job.Text,
		VoiceCode:    rc.opts.VoiceCode,
		WebhookURL:   rc.opts.WebhookURL,
		OutputFormat: rc.opts.OutputFormat,
		Bitrate:      rc.opts.Bitrate,
		Speed:        rc.opts.Speed,
		SampleRate:   rc.opts.SampleRate,
		ClientPause:  rc.opts.Pause.toVbee(),
	}

	var lastErr error
	for attempt := 1; attempt <= rc.timing.SubmitAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		rc.emitJob(job, StageSubmit, fmt.Sprintf("gửi %d ký tự tới Vbee", job.Chars), nil)

		id, err := rc.deps.Vbee.Submit(ctx, req)
		if err == nil {
			return id, nil
		}
		lastErr = err

		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return "", err
		case errors.Is(err, vbee.ErrUnauthorized):
			// Sai thông tin xác thực: mọi chương sau cũng hỏng y hệt, dừng ngay.
			return "", fatal(err)
		case errors.Is(err, vbee.ErrBadRequest) && !rc.anySucceeded:
			// Lỗi hình dạng yêu cầu (webhookUrl bị từ chối, voiceCode lạ, speed ngoài
			// khoảng) là lỗi đồng nhất — lộ ra ngay ở chương đầu thay vì phí cả cuốn.
			return "", fatal(err)
		case !vbee.IsRetryable(err):
			return "", err
		}

		if attempt < rc.timing.SubmitAttempts {
			rc.emitJob(job, StageSubmit, fmt.Sprintf("gửi thất bại, thử lại lần %d", attempt+1), err)
			if err := sleepCtx(ctx, rc.timing.RetryBase<<(attempt-1)); err != nil {
				return "", err
			}
		}
	}
	return "", fmt.Errorf("gửi yêu cầu thất bại sau %d lần: %w", rc.timing.SubmitAttempts, lastErr)
}

// waitForAudio poll trạng thái tới khi COMPLETED và trả về audioLink.
// Link chỉ sống 3 phút nên người gọi phải tải NGAY.
func (rc *runCtx) waitForAudio(ctx context.Context, job partJob, requestID string) (string, error) {
	if err := sleepCtx(ctx, rc.timing.InitialDelay); err != nil {
		return "", err
	}

	deadline := time.Now().Add(rc.timing.ChapterTimeout)
	interval := rc.timing.MinInterval
	pollErrs := 0

	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("quá hạn chờ %s (requestId=%s) — tra cứu được trong bảng điều khiển Vbee",
				rc.timing.ChapterTimeout, requestID)
		}

		st, err := rc.deps.Vbee.Status(ctx, requestID)
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return "", err
		case errors.Is(err, vbee.ErrUnauthorized):
			return "", fatal(err)
		case err != nil:
			pollErrs++
			if pollErrs > rc.timing.MaxPollErrors {
				return "", fmt.Errorf("mất liên lạc với Vbee sau %d lần hỏi liên tiếp (requestId=%s): %w",
					pollErrs, requestID, err)
			}
			rc.emitJob(job, StageWait, fmt.Sprintf("hỏi trạng thái lỗi (%d/%d)", pollErrs, rc.timing.MaxPollErrors), err)
		default:
			pollErrs = 0
			switch st.Status {
			case vbee.StatusCompleted:
				if st.AudioLink == "" {
					return "", fmt.Errorf("Vbee báo hoàn thành nhưng không có đường dẫn tệp (requestId=%s)", requestID)
				}
				return st.AudioLink, nil
			case vbee.StatusFailed:
				return "", fmt.Errorf("Vbee xử lý thất bại (requestId=%s)", requestID)
			default:
				rc.emitJob(job, StageWait, "Vbee đang xử lý...", nil)
			}
		}

		if err := sleepCtx(ctx, interval); err != nil {
			return "", err
		}
		interval = time.Duration(float64(interval) * rc.timing.BackoffFactor)
		if interval > rc.timing.MaxInterval {
			interval = rc.timing.MaxInterval
		}
	}
}

// download tải tệp âm thanh. audioLink hết hạn sau 3 phút, nên mỗi lần tải hỏng ta xin
// LINK MỚI qua Status rồi thử lại, thay vì thử lại chính link đã chết.
func (rc *runCtx) download(ctx context.Context, job partJob, requestID, link string) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= rc.timing.DownloadAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rc.emitJob(job, StageDownload, "tải tệp âm thanh", nil)

		data, err := rc.deps.Vbee.Download(ctx, link)
		if err == nil && len(data) > 0 {
			return data, nil
		}
		if err == nil {
			err = fmt.Errorf("tệp âm thanh rỗng")
		}
		lastErr = err
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}

		if attempt < rc.timing.DownloadAttempts {
			rc.emitJob(job, StageDownload, fmt.Sprintf("tải thất bại, xin đường dẫn mới (lần %d)", attempt+1), err)
			if st, serr := rc.deps.Vbee.Status(ctx, requestID); serr == nil &&
				st.Status == vbee.StatusCompleted && st.AudioLink != "" {
				link = st.AudioLink
			} else if errors.Is(serr, vbee.ErrUnauthorized) {
				return nil, fatal(serr)
			}
			if err := sleepCtx(ctx, rc.timing.RetryBase<<(attempt-1)); err != nil {
				return nil, err
			}
		}
	}
	return nil, fmt.Errorf("tải tệp âm thanh thất bại sau %d lần: %w", rc.timing.DownloadAttempts, lastErr)
}

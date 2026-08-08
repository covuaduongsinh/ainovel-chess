package imggen

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// Sentinel để bên gọi phân loại lỗi mà không phải so chuỗi.
var (
	ErrUnauthorized  = errors.New("imggen: khoá API không hợp lệ hoặc bị từ chối")
	ErrBadRequest    = errors.New("imggen: yêu cầu không hợp lệ")
	ErrSafetyBlocked = errors.New("imggen: nội dung bị bộ lọc an toàn chặn")
	ErrNoImage       = errors.New("imggen: phản hồi không chứa ảnh nào")
	ErrQuota         = errors.New("imggen: đã cạn hạn mức hoặc chưa bật thanh toán")
)

// Err là lỗi có cấu trúc từ API.
type Err struct {
	Code       string        // INVALID_ARGUMENT | RESOURCE_EXHAUSTED | IMAGE_SAFETY | HTTP_502…
	Message    string
	StatusCode int           // 200 với lỗi mức-ứng-dụng (bị chặn an toàn)
	RetryAfter time.Duration // từ header Retry-After hoặc google.rpc.RetryInfo; 0 = không rõ
}

func (e *Err) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("imggen: %s (%s, HTTP %d)", e.Message, e.Code, e.StatusCode)
	}
	return fmt.Sprintf("imggen: %s (HTTP %d)", e.Message, e.StatusCode)
}

// Unwrap ánh xạ về sentinel để errors.Is dùng được.
func (e *Err) Unwrap() error {
	switch {
	case e.StatusCode == 401 || e.StatusCode == 403 ||
		e.Code == "PERMISSION_DENIED" || e.Code == "UNAUTHENTICATED":
		return ErrUnauthorized
	case isSafetyCode(e.Code):
		return ErrSafetyBlocked
	case e.Code == "NO_CANDIDATES" || e.Code == "NO_IMAGE":
		return ErrNoImage
	case e.StatusCode == 400 || e.Code == "INVALID_ARGUMENT":
		return ErrBadRequest
	}
	return nil
}

func isSafetyCode(code string) bool {
	switch code {
	case "IMAGE_SAFETY", "PROHIBITED_CONTENT", "SAFETY", "BLOCKLIST", "RECITATION", "SPII":
		return true
	}
	return false
}

// IsRetryable cho biết có nên thử lại không.
//
// 429 và 5xx thì có; lỗi mạng/timeout tầng transport thì có; còn lại thì không.
// CHẶN AN TOÀN KHÔNG thử lại: cùng một prompt sẽ bị chặn y hệt, thử lại chỉ tốn thời gian.
//
// ⚠ 429 nhập nhằng: nó vừa là "quá nhanh, chờ chút" (nên thử lại) vừa là "cạn hạn mức
// ngày / chưa bật thanh toán" (chí tử). Tầng transport không phân biệt được, nên bên điều
// phối phải đếm số 429 liên tiếp và tự nâng thành lỗi chí tử — xem comic/imagesource.go.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var e *Err
	if errors.As(err, &e) {
		if isSafetyCode(e.Code) {
			return false
		}
		return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
	}
	// Lỗi mạng thô: thử lại.
	var ne net.Error
	return errors.As(err, &ne)
}

// parseError dựng *Err từ phản hồi lỗi.
func parseError(status int, body []byte, header http.Header) *Err {
	e := &Err{StatusCode: status, Message: strings.TrimSpace(string(body))}
	if len(e.Message) > maxErrorBody {
		e.Message = e.Message[:maxErrorBody] + "…"
	}
	var env errorEnvelope
	if jsonUnmarshal(body, &env) && env.Error.Message != "" {
		e.Message = env.Error.Message
		e.Code = env.Error.Status
		for _, d := range env.Error.Details {
			if strings.Contains(d.Type, "RetryInfo") && d.RetryDelay != "" {
				if dur, err := time.ParseDuration(d.RetryDelay); err == nil {
					e.RetryAfter = dur
				}
			}
		}
	}
	if e.Code == "" {
		e.Code = fmt.Sprintf("HTTP_%d", status)
	}
	if e.RetryAfter == 0 {
		if ra := header.Get("Retry-After"); ra != "" {
			if secs, err := time.ParseDuration(ra + "s"); err == nil {
				e.RetryAfter = secs
			}
		}
	}
	if e.Message == "" {
		e.Message = http.StatusText(status)
	}
	return e
}

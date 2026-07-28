package vbee

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// ErrUnauthorized đánh dấu lỗi xác thực: sai App-Id / Access Token, hoặc token hết hạn.
// Người gọi nên DỪNG HẲN chứ không thử lại — mọi yêu cầu tiếp theo cũng hỏng y hệt.
var ErrUnauthorized = errors.New("vbee: thông tin xác thực không hợp lệ")

// ErrBadRequest đánh dấu yêu cầu sai hình dạng (voiceCode lạ, tham số ngoài khoảng,
// webhookUrl bị từ chối...). Không thử lại được.
var ErrBadRequest = errors.New("vbee: yêu cầu không hợp lệ")

// Err là lỗi có mã do Vbee trả về. Giữ nguyên message gốc để hiển thị cho người dùng —
// thông báo của Vbee thường đã chỉ đúng trường sai.
type Err struct {
	Code       string // BAD_REQUEST | UNAUTHORIZED | INTERNAL_SERVER_ERROR | HTTP_429...
	Message    string
	StatusCode int
}

func (e *Err) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("vbee %s (HTTP %d): %s", e.Code, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("vbee %s (HTTP %d)", e.Code, e.StatusCode)
}

// Unwrap cho phép errors.Is(err, ErrUnauthorized) / errors.Is(err, ErrBadRequest).
func (e *Err) Unwrap() error {
	switch {
	case e.StatusCode == http.StatusUnauthorized || e.Code == "UNAUTHORIZED":
		return ErrUnauthorized
	case e.StatusCode == http.StatusBadRequest || e.Code == "BAD_REQUEST":
		return ErrBadRequest
	}
	return nil
}

// IsRetryable cho biết lỗi này có đáng thử lại không: 429 và 5xx thì có, lỗi mạng /
// timeout thì có, 4xx còn lại thì không. Hủy ngữ cảnh không bao giờ đáng thử lại.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Sentinel trần (do kiểm tra tham số phía client sinh ra, không kèm *Err) cũng là
	// lỗi vĩnh viễn — thiếu nhánh này thì thử lại vô ích và tốn thêm tín dụng.
	if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrBadRequest) {
		return false
	}
	var e *Err
	if errors.As(err, &e) {
		return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
	}
	// Lỗi mạng thuần (DNS, reset, timeout tầng transport) — thử lại được.
	return true
}

// errorEnvelope là phong bì lỗi chuẩn của Vbee.
type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

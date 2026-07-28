package vbee

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Submit gửi một yêu cầu tổng hợp BẤT ĐỒNG BỘ và trả về requestId để poll sau.
// req.Mode được ép về ModeAsync; webhookUrl rỗng sẽ dùng DefaultWebhookURL vì tài
// liệu Vbee ghi trường này là bắt buộc.
func (c *Client) Submit(ctx context.Context, req SpeechRequest) (string, error) {
	req.Mode = ModeAsync
	if strings.TrimSpace(req.WebhookURL) == "" {
		req.WebhookURL = DefaultWebhookURL
	}
	if strings.TrimSpace(req.Text) == "" {
		return "", fmt.Errorf("%w: text rỗng", ErrBadRequest)
	}
	if strings.TrimSpace(req.VoiceCode) == "" {
		return "", fmt.Errorf("%w: thiếu voiceCode", ErrBadRequest)
	}

	resp, err := c.postJSON(ctx, "/v1/tts", req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var out submitResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("đọc phản hồi Vbee thất bại: %w", err)
	}
	if strings.TrimSpace(out.RequestID) == "" {
		return "", fmt.Errorf("Vbee không trả về requestId")
	}
	return out.RequestID, nil
}

// Status truy vấn trạng thái một yêu cầu đã gửi.
// Khi Status == StatusCompleted thì AudioLink chỉ còn sống 3 phút.
func (c *Client) Status(ctx context.Context, requestID string) (RequestStatus, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return RequestStatus{}, fmt.Errorf("thiếu requestId")
	}
	url := c.baseURL + "/v1/tts/requests/" + requestID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return RequestStatus{}, err
	}
	c.setAuth(req)

	resp, err := c.do(c.http, req)
	if err != nil {
		return RequestStatus{}, err
	}
	defer resp.Body.Close()

	var out RequestStatus
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return RequestStatus{}, fmt.Errorf("đọc trạng thái Vbee thất bại: %w", err)
	}
	if out.RequestID == "" {
		out.RequestID = requestID
	}
	return out, nil
}

// Download tải tệp âm thanh về bộ nhớ. audioURL chỉ sống 3 phút kể từ lúc COMPLETED;
// nếu tải hỏng thì người gọi nên gọi lại Status để xin link mới thay vì thử lại link cũ.
//
// Dùng downloader riêng (timeout 10 phút) chứ không dùng client API 60 giây.
func (c *Client) Download(ctx context.Context, audioURL string) ([]byte, error) {
	audioURL = strings.TrimSpace(audioURL)
	if audioURL == "" {
		return nil, fmt.Errorf("thiếu đường dẫn tệp âm thanh")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, audioURL, nil)
	if err != nil {
		return nil, err
	}
	// audioLink là URL đã ký sẵn trên kho lưu trữ, không cần (và không nên) gắn
	// header xác thực của Vbee vào.
	resp, err := c.do(c.downloader, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tải tệp âm thanh thất bại: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("tệp âm thanh rỗng")
	}
	return data, nil
}

// SpeakSync tổng hợp ĐỒNG BỘ (tối đa MaxSyncChars ký tự) và trả thẳng byte audio.
// Dùng cho nút "Kiểm tra kết nối / nghe thử": KHÔNG cần webhookUrl, tốn rất ít tín dụng.
func (c *Client) SpeakSync(ctx context.Context, req SpeechRequest) ([]byte, error) {
	req.Mode = ModeSync
	req.WebhookURL = ""
	if strings.TrimSpace(req.Text) == "" {
		return nil, fmt.Errorf("%w: text rỗng", ErrBadRequest)
	}
	if n := len([]rune(req.Text)); n > MaxSyncChars {
		return nil, fmt.Errorf("%w: chế độ nghe thử tối đa %d ký tự (đang có %d)", ErrBadRequest, MaxSyncChars, n)
	}
	if strings.TrimSpace(req.VoiceCode) == "" {
		return nil, fmt.Errorf("%w: thiếu voiceCode", ErrBadRequest)
	}

	resp, err := c.postJSON(ctx, "/v1/tts", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("đọc âm thanh nghe thử thất bại: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("Vbee trả về âm thanh rỗng")
	}
	return data, nil
}

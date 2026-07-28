package vbee

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// apiTimeout là hạn cho các lời gọi API thuần JSON (gửi yêu cầu, hỏi trạng thái).
const apiTimeout = 60 * time.Second

// downloadTimeout là hạn cho việc TẢI tệp âm thanh. Phải rộng hơn apiTimeout rất
// nhiều: một chương ~25.000 ký tự cho ra khoảng 25 phút audio ≈ 24 MB ở 128 kbps,
// mà audioLink chỉ sống 3 phút nên không có chỗ cho việc thử lại chậm chạp.
const downloadTimeout = 10 * time.Minute

// maxErrorBody là số byte tối đa đọc từ thân phản hồi lỗi (phòng máy chủ trả HTML dài).
const maxErrorBody = 8 << 10

// Options là tham số dựng Client.
type Options struct {
	AppID       string
	AccessToken string
	BaseURL     string // rỗng = DefaultBaseURL
	VoicesURL   string // rỗng = DefaultVoicesURL
	// Client dùng cho các lời gọi API JSON; rỗng = timeout apiTimeout.
	// Đây là khe tiêm cho test, giống version.UpdateOptions.Client.
	Client *http.Client
	// Downloader dùng riêng cho việc tải audio; rỗng = timeout downloadTimeout.
	Downloader *http.Client
}

// Client là client HTTP tới API Vbee. An toàn khi dùng đồng thời (chỉ đọc trường).
type Client struct {
	appID      string
	token      string
	baseURL    string
	voicesURL  string
	http       *http.Client
	downloader *http.Client
}

// NewClient dựng Client với các mặc định đã điền.
func NewClient(o Options) *Client {
	c := &Client{
		appID:      strings.TrimSpace(o.AppID),
		token:      strings.TrimSpace(o.AccessToken),
		baseURL:    strings.TrimRight(strings.TrimSpace(o.BaseURL), "/"),
		voicesURL:  strings.TrimRight(strings.TrimSpace(o.VoicesURL), "/"),
		http:       o.Client,
		downloader: o.Downloader,
	}
	if c.baseURL == "" {
		c.baseURL = DefaultBaseURL
	}
	if c.voicesURL == "" {
		c.voicesURL = DefaultVoicesURL
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: apiTimeout}
	}
	if c.downloader == nil {
		c.downloader = &http.Client{Timeout: downloadTimeout}
	}
	return c
}

// setAuth gắn hai header xác thực bắt buộc của Vbee.
func (c *Client) setAuth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("App-Id", c.appID)
}

// do gửi yêu cầu và trả về phản hồi đã kiểm mã trạng thái. Người gọi phải đóng Body.
func (c *Client) do(client *http.Client, req *http.Request) (*http.Response, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gọi Vbee thất bại: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, parseError(resp)
	}
	return resp, nil
}

// parseError dựng *Err từ một phản hồi lỗi. Nếu thân không đúng phong bì chuẩn của
// Vbee thì vẫn tạo được lỗi có mã dạng "HTTP_502" để phía trên phân loại được.
func parseError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	e := &Err{StatusCode: resp.StatusCode}

	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err == nil && env.Error.Code != "" {
		e.Code = env.Error.Code
		e.Message = env.Error.Message
		return e
	}
	e.Code = "HTTP_" + strconv.Itoa(resp.StatusCode)
	if msg := strings.TrimSpace(string(body)); msg != "" {
		e.Message = msg
	}
	return e
}

// postJSON gửi một yêu cầu POST với thân JSON tới đường dẫn tương đối trên baseURL.
func (c *Client) postJSON(ctx context.Context, path string, payload any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("đóng gói yêu cầu Vbee thất bại: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setAuth(req)
	req.Header.Set("Content-Type", "application/json")
	return c.do(c.http, req)
}

package imggen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // đăng ký bộ giải mã để image.DecodeConfig đo được ảnh JPEG
	_ "image/png"  // ...và PNG
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// Client gọi API sinh ảnh.
type Client struct {
	apiKey     string
	baseURL    string
	model      string
	keyInQuery bool
	dialect    dialect
	http       *http.Client
	timing     Timing
}

// Timing gom mọi tham số chờ vào một chỗ để test tiêm được và chạy bằng mili-giây.
type Timing struct {
	MaxAttempts  int
	RetryBase    time.Duration
	RetryMax     time.Duration
	RetryCapHint time.Duration // trần cho Retry-After do máy chủ đề nghị
}

func (t Timing) normalize() Timing {
	if t.MaxAttempts <= 0 {
		t.MaxAttempts = 4
	}
	if t.RetryBase <= 0 {
		t.RetryBase = 2 * time.Second
	}
	if t.RetryMax <= 0 {
		t.RetryMax = 30 * time.Second
	}
	if t.RetryCapHint <= 0 {
		t.RetryCapHint = 60 * time.Second
	}
	return t
}

// dialect tách phần DỰNG/ĐỌC gói tin khỏi phần chính sách.
// Google đang có hai bề mặt API song song nên đây là chỗ đổi khi cần.
type dialect interface {
	name() string
	path(model string) string
	build(model string, r Request) ([]byte, error)
	parse(raw []byte) (*Result, error)
}

// NewClient dựng client từ Options.
func NewClient(o Options) *Client {
	c := &Client{
		apiKey:     strings.TrimSpace(o.APIKey),
		baseURL:    strings.TrimRight(strings.TrimSpace(o.BaseURL), "/"),
		model:      strings.TrimSpace(o.Model),
		keyInQuery: o.KeyInQuery,
		http:       o.Client,
		timing:     Timing{}.normalize(),
	}
	if c.baseURL == "" {
		c.baseURL = DefaultBaseURL
	}
	if c.model == "" {
		c.model = DefaultModel
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: defaultTimeout}
	}
	switch strings.ToLower(strings.TrimSpace(o.Dialect)) {
	case "interactions":
		c.dialect = interactionsDialect{}
	default:
		c.dialect = generateContentDialect{}
	}
	return c
}

// SetTiming ghi đè tham số chờ (test dùng để chạy nhanh).
func (c *Client) SetTiming(t Timing) { c.timing = t.normalize() }

// Configured cho biết đã có khoá API chưa.
func (c *Client) Configured() bool { return c.apiKey != "" }

// Model trả về model hiệu lực.
func (c *Client) Model() string { return c.model }

// Dialect trả về tên phương ngữ đang dùng.
func (c *Client) Dialect() string { return c.dialect.name() }

// Generate sinh MỘT ảnh, có thử lại theo chính sách.
func (c *Client) Generate(ctx context.Context, r Request) (*Result, error) {
	if !c.Configured() {
		return nil, &Err{Code: "UNAUTHENTICATED", Message: "chưa cấu hình khoá API Gemini", StatusCode: 401}
	}
	if strings.TrimSpace(r.Prompt) == "" {
		return nil, &Err{Code: "INVALID_ARGUMENT", Message: "prompt rỗng", StatusCode: 400}
	}
	// Kiểm phía client trước khi tốn một vòng mạng (và tiền).
	if len(r.Refs) > MaxRefImages {
		return nil, &Err{Code: "INVALID_ARGUMENT", StatusCode: 400,
			Message: fmt.Sprintf("quá nhiều ảnh tham chiếu: %d (trần %d) — vượt ngân sách ảnh nhân vật thì mô hình sẽ trộn lẫn các nhân vật", len(r.Refs), MaxRefImages)}
	}
	for _, ref := range r.Refs {
		if len(ref.Data) > MaxRefBytes {
			return nil, &Err{Code: "INVALID_ARGUMENT", StatusCode: 400,
				Message: fmt.Sprintf("ảnh tham chiếu %q nặng %d byte, vượt trần %d — hãy thu nhỏ trước", ref.Label, len(ref.Data), MaxRefBytes)}
		}
	}

	model := strings.TrimSpace(r.Model)
	if model == "" {
		model = c.model
	}

	// Model không làm được độ phân giải đã đặt: BỎ tham số đi thay vì gửi để bị lờ.
	// Gửi đi rồi bị lờ vẫn mất đúng số tiền đó, mà không ai biết.
	wantSize := normalizeSize(r.ImageSize)
	var warn string
	if wantSize != "" && !SupportsSize(model, wantSize) {
		caps, _ := CapsFor(model)
		warn = fmt.Sprintf("model %s không làm được %s (chỉ %s) — đã bỏ yêu cầu độ phân giải; muốn in 300 DPI hãy đổi sang %s",
			model, wantSize, strings.Join(caps.Sizes, "/"), ModelFlash)
		r.ImageSize = ""
	}

	body, err := c.dialect.build(model, r)
	if err != nil {
		return nil, err
	}

	var last error
	for attempt := 1; attempt <= c.timing.MaxAttempts; attempt++ {
		res, err := c.once(ctx, model, body)
		if err == nil {
			res.Model = model
			c.inspectImage(res, wantSize, warn)
			return res, nil
		}
		last = err
		if !IsRetryable(err) || attempt == c.timing.MaxAttempts {
			break
		}
		if werr := sleepCtx(ctx, c.backoff(attempt, err)); werr != nil {
			return nil, werr
		}
	}
	return nil, fmt.Errorf("sinh ảnh thất bại sau %d lần thử: %w", c.timing.MaxAttempts, last)
}

// inspectImage đo kích thước THẬT của ảnh trả về và đối chiếu với thứ đã đặt hàng.
//
// Bước này không thể bỏ: máy chủ vẫn trả HTTP 200 và vẫn tính tiền đủ khi nó lờ đi tham số
// imageSize. Chỉ có cách tự đo pixel mới biết mình đã trả tiền 2K để nhận 1K.
func (c *Client) inspectImage(res *Result, wantSize, preWarn string) {
	res.Warning = preWarn
	if len(res.Image) == 0 {
		return
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(res.Image))
	if err != nil {
		return // định dạng lạ (vd webp) — không suy đoán
	}
	res.Width, res.Height = cfg.Width, cfg.Height

	if wantSize == "" {
		return
	}
	wantMP := megapixelFor(wantSize)
	gotMP := float64(cfg.Width) * float64(cfg.Height) / 1e6
	if wantMP > 0 && gotMP < wantMP*0.75 {
		msg := fmt.Sprintf("đã yêu cầu %s nhưng ảnh trả về chỉ %dx%d (%.1f MP) — máy chủ bỏ qua tham số độ phân giải, tiền vẫn tính đủ",
			wantSize, cfg.Width, cfg.Height, gotMP)
		if res.Warning == "" {
			res.Warning = msg
		} else {
			res.Warning += "; " + msg
		}
	}
}

// backoff tính thời gian chờ: luỹ thừa 2 có TRẦN, rồi jitter toàn phần.
//
// Jitter toàn phần (chứ không phải một nửa) là cố ý: sinh ảnh chạy theo vòng lặp hàng trăm
// khung, nếu nhiều yêu cầu cùng bị 429 rồi cùng chờ đúng một khoảng thì chúng sẽ va nhau lại.
func (c *Client) backoff(attempt int, err error) time.Duration {
	d := c.timing.RetryBase << (attempt - 1)
	if d > c.timing.RetryMax {
		d = c.timing.RetryMax
	}
	if d > 0 {
		d = time.Duration(rand.Int63n(int64(d)))
	}
	var e *Err
	if asErr(err, &e) && e.RetryAfter > d {
		d = e.RetryAfter
		if d > c.timing.RetryCapHint {
			d = c.timing.RetryCapHint
		}
	}
	return d
}

// once thực hiện đúng một lượt gọi.
func (c *Client) once(ctx context.Context, model string, body []byte) (*Result, error) {
	url := c.baseURL + c.dialect.path(model)
	if c.keyInQuery {
		url += "?key=" + c.apiKey
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if !c.keyInQuery {
		req.Header.Set("x-goog-api-key", c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseError(resp.StatusCode, raw, resp.Header)
	}
	return c.dialect.parse(raw)
}

// sleepCtx chờ nhưng vẫn hủy được ngay khi ctx đóng (Esc/Dừng phải ăn liền).
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func jsonUnmarshal(data []byte, v any) bool { return json.Unmarshal(data, v) == nil }

func asErr(err error, target **Err) bool {
	for err != nil {
		if e, ok := err.(*Err); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

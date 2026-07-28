package imggen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fastTiming để test chạy bằng mili-giây thay vì giây.
func fastTiming() Timing {
	return Timing{MaxAttempts: 4, RetryBase: time.Millisecond, RetryMax: 2 * time.Millisecond, RetryCapHint: 5 * time.Millisecond}
}

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewClient(Options{APIKey: "k-test", BaseURL: srv.URL, Client: srv.Client()})
	c.SetTiming(fastTiming())
	return c
}

// okResponse dựng phản hồi generateContent hợp lệ có kèm ảnh.
func okResponse(png []byte) string {
	body := map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{"role": "model", "parts": []any{
				map[string]any{"text": "Đây là ảnh."},
				map[string]any{"inlineData": map[string]any{
					"mimeType": "image/png",
					"data":     base64.StdEncoding.EncodeToString(png),
				}},
			}},
			"finishReason": "STOP",
		}},
		"usageMetadata": map[string]any{"totalTokenCount": 1234},
	}
	b, _ := json.Marshal(body)
	return string(b)
}

func TestGenerateHappyPath(t *testing.T) {
	want := []byte("\x89PNG-giả-lập")
	var gotPath, gotKey, gotCT string
	var gotBody []byte
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKey, gotCT = r.URL.Path, r.Header.Get("x-goog-api-key"), r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, okResponse(want))
	})

	// Dùng ModelFlash vì nó THẬT SỰ làm được 2K; model mặc định chỉ 1K nên tham số 2K
	// sẽ bị bỏ đi (hành vi đó có test riêng bên dưới).
	res, err := c.Generate(context.Background(), Request{
		Prompt: "a boy at a chessboard", Negative: "text, watermark",
		AspectRatio: "4:3", ImageSize: "2K", Model: ModelFlash,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if string(res.Image) != string(want) {
		t.Errorf("byte ảnh sai: %q", res.Image)
	}
	if res.MimeType != "image/png" || res.FinishReason != "STOP" || res.TotalTokens != 1234 {
		t.Errorf("metadata sai: %+v", res)
	}
	if res.Model != ModelFlash {
		t.Errorf("model = %q, muốn %q", res.Model, ModelFlash)
	}
	if want := "/v1beta/models/" + ModelFlash + ":generateContent"; gotPath != want {
		t.Errorf("path = %q, muốn %q", gotPath, want)
	}
	// Khoá phải đi qua HEADER, không nằm trong URL — tránh lọt vào log proxy.
	if gotKey != "k-test" {
		t.Errorf("thiếu header x-goog-api-key, nhận %q", gotKey)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q", gotCT)
	}

	var req genRequest
	if err := json.Unmarshal(gotBody, &req); err != nil {
		t.Fatalf("thân yêu cầu không phải JSON hợp lệ: %v", err)
	}
	if req.GenerationConfig == nil || len(req.GenerationConfig.ResponseModalities) == 0 {
		t.Fatal("thiếu responseModalities — không có nó thì model 2.5 sẽ chỉ trả chữ")
	}
	if req.GenerationConfig.ImageConfig == nil ||
		req.GenerationConfig.ImageConfig.AspectRatio != "4:3" ||
		req.GenerationConfig.ImageConfig.ImageSize != "2K" {
		t.Errorf("imageConfig sai: %+v", req.GenerationConfig.ImageConfig)
	}
	// Negative phải được diễn đạt thành câu cấm trong prompt (Gemini không có trường riêng).
	last := req.Contents[0].Parts[len(req.Contents[0].Parts)-1].Text
	if !strings.Contains(last, "Do NOT include") || !strings.Contains(last, "watermark") {
		t.Errorf("negative không được nối vào prompt: %q", last)
	}
}

// TestRefImagesOrdering khẳng định ảnh tham chiếu đứng TRƯỚC và chỉ thị chính đứng CUỐI,
// và byte ảnh đi vòng base64 không sai lệch.
func TestRefImagesOrdering(t *testing.T) {
	refA, refB := []byte("ảnh-A"), []byte("ảnh-B")
	var gotBody []byte
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, okResponse([]byte("x")))
	})
	if _, err := c.Generate(context.Background(), Request{
		Prompt: "CHỈ-THỊ-CHÍNH",
		Refs: []RefImage{
			{MimeType: "image/png", Data: refA, Label: "Wolf"},
			{MimeType: "image/png", Data: refB, Label: "Josef"},
		},
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var req genRequest
	if err := json.Unmarshal(gotBody, &req); err != nil {
		t.Fatal(err)
	}
	parts := req.Contents[0].Parts
	if n := len(parts); n != 5 { // 2 ref × (nhãn + blob) + 1 chỉ thị
		t.Fatalf("số part = %d, muốn 5: %+v", n, parts)
	}
	if parts[len(parts)-1].Text != "CHỈ-THỊ-CHÍNH" {
		t.Errorf("chỉ thị chính phải đứng CUỐI, nhận %q", parts[len(parts)-1].Text)
	}
	if !strings.Contains(parts[0].Text, "Wolf") {
		t.Errorf("nhãn ảnh tham chiếu đầu sai: %q", parts[0].Text)
	}
	got, err := base64.StdEncoding.DecodeString(parts[1].InlineData.Data)
	if err != nil || string(got) != string(refA) {
		t.Errorf("byte ảnh tham chiếu bị sai lệch: %q (%v)", got, err)
	}
}

// TestRejectsTooManyRefs khẳng định chặn phía client, KHÔNG tốn vòng mạng.
func TestRejectsTooManyRefs(t *testing.T) {
	var hits int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = io.WriteString(w, okResponse([]byte("x")))
	})
	refs := make([]RefImage, MaxRefImages+1)
	for i := range refs {
		refs[i] = RefImage{Data: []byte("x")}
	}
	_, err := c.Generate(context.Background(), Request{Prompt: "p", Refs: refs})
	if !errors.Is(err, ErrBadRequest) {
		t.Fatalf("phải trả ErrBadRequest, nhận %v", err)
	}
	if hits != 0 {
		t.Errorf("không được gọi mạng khi đã sai từ đầu, nhận %d lượt", hits)
	}
}

// TestSafetyBlockNoRetry — bị chặn an toàn thì KHÔNG thử lại: cùng prompt sẽ chặn y hệt.
func TestSafetyBlockNoRetry(t *testing.T) {
	var hits int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[]},"finishReason":"IMAGE_SAFETY"}]}`)
	})
	_, err := c.Generate(context.Background(), Request{Prompt: "p"})
	if !errors.Is(err, ErrSafetyBlocked) {
		t.Fatalf("phải trả ErrSafetyBlocked, nhận %v", err)
	}
	if hits != 1 {
		t.Errorf("chặn an toàn phải gọi ĐÚNG 1 lần, nhận %d", hits)
	}
}

func TestPromptFeedbackBlock(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"promptFeedback":{"blockReason":"PROHIBITED_CONTENT"}}`)
	})
	_, err := c.Generate(context.Background(), Request{Prompt: "p"})
	if !errors.Is(err, ErrSafetyBlocked) {
		t.Fatalf("phải trả ErrSafetyBlocked, nhận %v", err)
	}
}

// TestNoImageMentionsText — model trả chữ mà không trả ảnh là ca hay gặp; thông báo phải
// kèm đoạn chữ đó, nếu không thì không biết vì sao hỏng.
func TestNoImageMentionsText(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"Tôi không thể vẽ nội dung này."}]},"finishReason":"STOP"}]}`)
	})
	_, err := c.Generate(context.Background(), Request{Prompt: "p"})
	if !errors.Is(err, ErrNoImage) {
		t.Fatalf("phải trả ErrNoImage, nhận %v", err)
	}
	if !strings.Contains(err.Error(), "không thể vẽ") {
		t.Errorf("thông báo phải kèm chữ model trả về: %v", err)
	}
}

// TestRetryOn429ThenSuccess — 429 thì thử lại và cuối cùng thành công.
func TestRetryOn429ThenSuccess(t *testing.T) {
	var hits int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"quá nhanh"}}`)
			return
		}
		_, _ = io.WriteString(w, okResponse([]byte("ok")))
	})
	res, err := c.Generate(context.Background(), Request{Prompt: "p"})
	if err != nil {
		t.Fatalf("phải thành công sau khi thử lại: %v", err)
	}
	if string(res.Image) != "ok" {
		t.Errorf("ảnh sai: %q", res.Image)
	}
	if hits != 2 {
		t.Errorf("phải gọi 2 lượt, nhận %d", hits)
	}
}

// TestNoRetryOn403 — sai khoá thì thử lại vô ích.
func TestNoRetryOn403(t *testing.T) {
	var hits int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"code":403,"status":"PERMISSION_DENIED","message":"khoá sai"}}`)
	})
	_, err := c.Generate(context.Background(), Request{Prompt: "p"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("phải trả ErrUnauthorized, nhận %v", err)
	}
	if hits != 1 {
		t.Errorf("403 phải gọi ĐÚNG 1 lần, nhận %d", hits)
	}
}

// TestRetryExhausted — 5xx liên tục thì cạn lượt và báo rõ số lần đã thử.
func TestRetryExhausted(t *testing.T) {
	var hits int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadGateway)
	})
	_, err := c.Generate(context.Background(), Request{Prompt: "p"})
	if err == nil {
		t.Fatal("phải báo lỗi")
	}
	if hits != 4 {
		t.Errorf("phải thử đủ 4 lượt, nhận %d", hits)
	}
	if !strings.Contains(err.Error(), "sau 4 lần thử") {
		t.Errorf("thông báo phải nói rõ số lần đã thử: %v", err)
	}
}

// TestContextCancelStopsImmediately — Esc/Dừng phải ăn liền, không chờ hết backoff.
func TestContextCancelStopsImmediately(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	c.SetTiming(Timing{MaxAttempts: 4, RetryBase: 5 * time.Second, RetryMax: 5 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()

	start := time.Now()
	_, err := c.Generate(ctx, Request{Prompt: "p"})
	if err == nil {
		t.Fatal("phải báo lỗi khi bị hủy")
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Errorf("hủy phải ăn ngay, nhưng mất %v", el)
	}
}

// TestKeyInQuery — đường thoát cho proxy chỉ nhận ?key=.
func TestKeyInQuery(t *testing.T) {
	var gotQuery, gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery, gotHeader = r.URL.Query().Get("key"), r.Header.Get("x-goog-api-key")
		_, _ = io.WriteString(w, okResponse([]byte("x")))
	}))
	defer srv.Close()
	c := NewClient(Options{APIKey: "k-q", BaseURL: srv.URL, Client: srv.Client(), KeyInQuery: true})
	c.SetTiming(fastTiming())
	if _, err := c.Generate(context.Background(), Request{Prompt: "p"}); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "k-q" || gotHeader != "" {
		t.Errorf("KeyInQuery: query=%q header=%q", gotQuery, gotHeader)
	}
}

// TestInteractionsDialect — phương ngữ thứ hai đọc được ảnh từ steps[].content[].
func TestInteractionsDialect(t *testing.T) {
	want := []byte("ảnh-interactions")
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body := map[string]any{"steps": []any{map[string]any{
			"type": "model_output",
			"content": []any{map[string]any{
				"type": "image", "mime_type": "image/png",
				"data": base64.StdEncoding.EncodeToString(want),
			}},
		}}}
		b, _ := json.Marshal(body)
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	c := NewClient(Options{APIKey: "k", BaseURL: srv.URL, Client: srv.Client(), Dialect: "interactions"})
	c.SetTiming(fastTiming())
	res, err := c.Generate(context.Background(), Request{Prompt: "p", AspectRatio: "1:1"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if string(res.Image) != string(want) {
		t.Errorf("ảnh sai: %q", res.Image)
	}
	if gotPath != "/v1beta/interactions" {
		t.Errorf("path = %q", gotPath)
	}
	if c.Dialect() != "interactions" {
		t.Errorf("Dialect() = %q", c.Dialect())
	}
}

// pngBytes dựng một PNG thật cỡ w×h để test đo được kích thước.
func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("mã hoá PNG: %v", err)
	}
	return buf.Bytes()
}

func TestModelCaps(t *testing.T) {
	if SupportsSize(ModelFlashLite, "2K") {
		t.Error("flash-lite chỉ làm được 1K, không được báo là hỗ trợ 2K")
	}
	if !SupportsSize(ModelFlashLite, "1K") {
		t.Error("flash-lite phải hỗ trợ 1K")
	}
	if !SupportsSize(ModelFlash, "2K") {
		t.Error("flash phải hỗ trợ 2K")
	}
	// Model lạ: không chặn thứ mình không biết.
	if !SupportsSize("model-la-hoac-moi", "4K") {
		t.Error("model lạ phải được cho qua, không suy đoán")
	}
	if p, ok := PriceFor(ModelFlash, "2K"); !ok || p != 0.101 {
		t.Errorf("giá flash 2K = %v (ok=%v), muốn 0.101", p, ok)
	}
}

// TestDropsUnsupportedImageSize là test then chốt của việc cắt lãng phí: xin 2K từ model
// chỉ-1K thì tham số phải bị BỎ (gửi đi cũng bị lờ mà vẫn mất tiền) và phải có cảnh báo.
func TestDropsUnsupportedImageSize(t *testing.T) {
	var gotBody []byte
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, okResponse(pngBytes(t, 1024, 1024)))
	})
	res, err := c.Generate(context.Background(), Request{
		Prompt: "p", ImageSize: "2K", AspectRatio: "1:1", Model: ModelFlashLite,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var req genRequest
	if err := json.Unmarshal(gotBody, &req); err != nil {
		t.Fatal(err)
	}
	if req.GenerationConfig.ImageConfig.ImageSize != "" {
		t.Errorf("phải BỎ imageSize khi model không hỗ trợ, nhưng vẫn gửi %q",
			req.GenerationConfig.ImageConfig.ImageSize)
	}
	if req.GenerationConfig.ImageConfig.AspectRatio != "1:1" {
		t.Error("tỉ lệ khung hình vẫn phải được gửi")
	}
	if !strings.Contains(res.Warning, "không làm được 2K") {
		t.Errorf("thiếu cảnh báo về độ phân giải: %q", res.Warning)
	}
	if !strings.Contains(res.Warning, ModelFlash) {
		t.Errorf("cảnh báo phải chỉ ra model nào in được: %q", res.Warning)
	}
}

// TestWarnsWhenServerIgnoresSize bắt đúng kiểu lỗi đã gặp thật: máy chủ nhận 2K, trả 200,
// tính tiền đủ, nhưng ảnh chỉ ~1 megapixel.
func TestWarnsWhenServerIgnoresSize(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, okResponse(pngBytes(t, 1024, 1024))) // 1 MP dù xin 2K
	})
	res, err := c.Generate(context.Background(), Request{
		Prompt: "p", ImageSize: "2K", Model: ModelFlash, // model NÀY hỗ trợ 2K
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if res.Width != 1024 || res.Height != 1024 {
		t.Errorf("phải đo được kích thước thật, nhận %dx%d", res.Width, res.Height)
	}
	if !strings.Contains(res.Warning, "bỏ qua tham số độ phân giải") {
		t.Errorf("phải cảnh báo khi ảnh nhỏ hơn mức đã đặt: %q", res.Warning)
	}
}

// TestNoWarningWhenSizeHonoured — đúng cỡ thì không được cảnh báo bừa.
func TestNoWarningWhenSizeHonoured(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, okResponse(pngBytes(t, 2048, 1600))) // 3,3 MP ≈ 2K
	})
	res, err := c.Generate(context.Background(), Request{Prompt: "p", ImageSize: "2K", Model: ModelFlash})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if res.Warning != "" {
		t.Errorf("không được cảnh báo khi độ phân giải đúng: %q", res.Warning)
	}
}

func TestNotConfigured(t *testing.T) {
	c := NewClient(Options{})
	_, err := c.Generate(context.Background(), Request{Prompt: "p"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("thiếu khoá phải trả ErrUnauthorized, nhận %v", err)
	}
	if c.Configured() {
		t.Error("Configured() phải false khi chưa có khoá")
	}
}

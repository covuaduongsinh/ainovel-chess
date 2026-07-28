package vbee

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestClient dựng Client trỏ vào một máy chủ giả cho cả TTS lẫn danh sách giọng.
func newTestClient(t *testing.T, h http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewClient(Options{
		AppID:       "app-1",
		AccessToken: "tok-1",
		BaseURL:     srv.URL,
		VoicesURL:   srv.URL,
		Client:      srv.Client(),
		Downloader:  srv.Client(),
	})
	return c, srv
}

func TestSubmit_GuiDungHeaderVaThanCamelCase(t *testing.T) {
	var gotBody map[string]any
	var gotAuth, gotAppID, gotType, gotMethod, gotPath string

	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAppID = r.Header.Get("App-Id")
		gotType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = io.WriteString(w, `{"requestId":"req-1","status":"PROCESSING"}`)
	}))

	_, err := c.Submit(context.Background(), SpeechRequest{
		Text:         "Xin chào",
		VoiceCode:    "hn_female_ngochuyen_full_48k-fhg",
		OutputFormat: "mp3",
		Bitrate:      128,
		Speed:        1.0,
		SampleRate:   24000,
	})
	if err != nil {
		t.Fatalf("Submit lỗi: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/v1/tts" {
		t.Errorf("gọi sai endpoint: %s %s", gotMethod, gotPath)
	}
	if gotAuth != "Bearer tok-1" {
		t.Errorf("Authorization sai: %q", gotAuth)
	}
	if gotAppID != "app-1" {
		t.Errorf("App-Id sai: %q", gotAppID)
	}
	if gotType != "application/json" {
		t.Errorf("Content-Type sai: %q", gotType)
	}
	for _, key := range []string{"text", "mode", "voiceCode", "webhookUrl", "outputFormat", "bitrate", "speed", "sampleRate"} {
		if _, ok := gotBody[key]; !ok {
			t.Errorf("thân yêu cầu thiếu trường camelCase %q; có: %v", key, gotBody)
		}
	}
	if gotBody["mode"] != ModeAsync {
		t.Errorf("mode phải là %q, nhận %v", ModeAsync, gotBody["mode"])
	}
	if gotBody["webhookUrl"] != DefaultWebhookURL {
		t.Errorf("webhookUrl rỗng phải được điền mặc định, nhận %v", gotBody["webhookUrl"])
	}
}

func TestSubmit_TraVeRequestId(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"requestId":"eb75e2b0-ce65","status":"PROCESSING"}`)
	}))
	id, err := c.Submit(context.Background(), SpeechRequest{Text: "a", VoiceCode: "v"})
	if err != nil {
		t.Fatalf("Submit lỗi: %v", err)
	}
	if id != "eb75e2b0-ce65" {
		t.Errorf("requestId sai: %q", id)
	}
}

func TestSubmit_ThieuRequestIdThiBaoLoi(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"status":"PROCESSING"}`)
	}))
	if _, err := c.Submit(context.Background(), SpeechRequest{Text: "a", VoiceCode: "v"}); err == nil {
		t.Fatal("thiếu requestId mà không báo lỗi")
	}
}

func TestSubmit_401TraVeErrUnauthorized(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"code":"UNAUTHORIZED","message":"token không hợp lệ"}}`)
	}))
	_, err := c.Submit(context.Background(), SpeechRequest{Text: "a", VoiceCode: "v"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("401 phải cho ErrUnauthorized, nhận: %v", err)
	}
	if IsRetryable(err) {
		t.Error("lỗi xác thực không được coi là thử lại được")
	}
	if !strings.Contains(err.Error(), "token không hợp lệ") {
		t.Errorf("phải giữ nguyên message của Vbee, nhận: %v", err)
	}
}

func TestSubmit_400TraVeErrBadRequestVaKhongThuLai(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"code":"BAD_REQUEST","message":"webhookUrl must be defined at path webhookUrl"}}`)
	}))
	_, err := c.Submit(context.Background(), SpeechRequest{Text: "a", VoiceCode: "v"})
	if !errors.Is(err, ErrBadRequest) {
		t.Fatalf("400 phải cho ErrBadRequest, nhận: %v", err)
	}
	if IsRetryable(err) {
		t.Error("400 không được coi là thử lại được")
	}
}

func TestSubmit_429Va500CoTheThuLai(t *testing.T) {
	for _, code := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway} {
		c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			_, _ = io.WriteString(w, `{"error":{"code":"INTERNAL_SERVER_ERROR","message":"thử lại sau"}}`)
		}))
		_, err := c.Submit(context.Background(), SpeechRequest{Text: "a", VoiceCode: "v"})
		if err == nil {
			t.Fatalf("HTTP %d phải báo lỗi", code)
		}
		if !IsRetryable(err) {
			t.Errorf("HTTP %d phải thử lại được, nhận: %v", code, err)
		}
		if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrBadRequest) {
			t.Errorf("HTTP %d không được phân loại thành lỗi vĩnh viễn", code)
		}
	}
}

func TestSubmit_BodyLoiSaiHinhDangVanTaoDuocErr(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `<html>502 Bad Gateway</html>`)
	}))
	_, err := c.Submit(context.Background(), SpeechRequest{Text: "a", VoiceCode: "v"})
	var e *Err
	if !errors.As(err, &e) {
		t.Fatalf("phải dựng được *Err, nhận: %v", err)
	}
	if e.Code != "HTTP_502" || e.StatusCode != 502 {
		t.Errorf("mã lỗi dự phòng sai: %+v", e)
	}
	if !IsRetryable(err) {
		t.Error("502 phải thử lại được")
	}
}

func TestSubmit_TuChoiThamSoRong(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("không được gửi request khi tham số rỗng")
	}))
	if _, err := c.Submit(context.Background(), SpeechRequest{Text: "  ", VoiceCode: "v"}); !errors.Is(err, ErrBadRequest) {
		t.Errorf("text rỗng phải cho ErrBadRequest, nhận: %v", err)
	}
	if _, err := c.Submit(context.Background(), SpeechRequest{Text: "a", VoiceCode: " "}); !errors.Is(err, ErrBadRequest) {
		t.Errorf("thiếu voiceCode phải cho ErrBadRequest, nhận: %v", err)
	}
}

func TestStatus_PhanTichCompletedVaAudioLink(t *testing.T) {
	var gotPath, gotAuth string
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"requestId":"req-1","status":"COMPLETED","audioLink":"https://cdn/a.mp3"}`)
	}))
	st, err := c.Status(context.Background(), "req-1")
	if err != nil {
		t.Fatalf("Status lỗi: %v", err)
	}
	if gotPath != "/v1/tts/requests/req-1" {
		t.Errorf("đường dẫn sai: %q", gotPath)
	}
	if gotAuth != "Bearer tok-1" {
		t.Errorf("Status phải gắn header xác thực, nhận %q", gotAuth)
	}
	if st.Status != StatusCompleted || st.AudioLink != "https://cdn/a.mp3" {
		t.Errorf("phân tích sai: %+v", st)
	}
}

func TestStatus_PhanTichProcessingVaFailed(t *testing.T) {
	for _, want := range []string{StatusProcessing, StatusFailed} {
		body := `{"requestId":"req-1","status":"` + want + `"}`
		c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, body)
		}))
		st, err := c.Status(context.Background(), "req-1")
		if err != nil {
			t.Fatalf("Status lỗi: %v", err)
		}
		if st.Status != want {
			t.Errorf("trạng thái sai: muốn %q nhận %q", want, st.Status)
		}
	}
}

func TestDownload_TraVeByteVaBaoLoiKhi404(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/het-han.mp3" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("không được gắn header xác thực khi tải link đã ký, nhận %q", got)
		}
		_, _ = w.Write([]byte("ID3-DU-LIEU-MP3"))
	}))
	srvURL := c.baseURL

	data, err := c.Download(context.Background(), srvURL+"/ok.mp3")
	if err != nil {
		t.Fatalf("Download lỗi: %v", err)
	}
	if string(data) != "ID3-DU-LIEU-MP3" {
		t.Errorf("nội dung tải về sai: %q", data)
	}

	if _, err := c.Download(context.Background(), srvURL+"/het-han.mp3"); err == nil {
		t.Fatal("link hết hạn (404) phải báo lỗi")
	}
}

func TestDownload_TepRongBaoLoi(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	if _, err := c.Download(context.Background(), c.baseURL+"/rong.mp3"); err == nil {
		t.Fatal("tệp rỗng phải báo lỗi")
	}
}

func TestSpeakSync_TraVeByteThoAudio(t *testing.T) {
	var gotBody map[string]any
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte("AUDIO-NGHE-THU"))
	}))
	data, err := c.SpeakSync(context.Background(), SpeechRequest{Text: "Xin chào", VoiceCode: "v"})
	if err != nil {
		t.Fatalf("SpeakSync lỗi: %v", err)
	}
	if string(data) != "AUDIO-NGHE-THU" {
		t.Errorf("nội dung sai: %q", data)
	}
	if gotBody["mode"] != ModeSync {
		t.Errorf("mode phải là %q, nhận %v", ModeSync, gotBody["mode"])
	}
	if _, ok := gotBody["webhookUrl"]; ok {
		t.Error("chế độ sync không được gửi webhookUrl")
	}
}

func TestSpeakSync_TuChoiVanBanQuaDai(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("không được gửi request khi vượt giới hạn ký tự")
	}))
	long := strings.Repeat("ă", MaxSyncChars+1)
	if _, err := c.SpeakSync(context.Background(), SpeechRequest{Text: long, VoiceCode: "v"}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("vượt %d ký tự phải cho ErrBadRequest, nhận: %v", MaxSyncChars, err)
	}
}

func TestVoices_BocPhongBiResultVaSnakeCase(t *testing.T) {
	var gotQuery string
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = io.WriteString(w, `{"result":{"pagination":{"has_next_page":false,"next_cursor":null},
			"voices":[{"code":"hn_female_ngochuyen_full_48k-fhg","credit_factor":1,
			"demo":"https://cdn/demo.mp3","gender":"female","language_code":"vi-VN","name":"HN - Ngọc Huyền"}]},"status":1}`)
	}))
	p, err := c.Voices(context.Background(), VoiceQuery{Ownership: "VBEE", LanguageCode: "vi-VN", Limit: 100})
	if err != nil {
		t.Fatalf("Voices lỗi: %v", err)
	}
	for _, want := range []string{"voiceOwnership=VBEE", "languageCode=vi-VN", "limit=100"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query thiếu %q; nhận %q", want, gotQuery)
		}
	}
	if len(p.Voices) != 1 {
		t.Fatalf("muốn 1 giọng, nhận %d", len(p.Voices))
	}
	v := p.Voices[0]
	if v.Code != "hn_female_ngochuyen_full_48k-fhg" || v.Name != "HN - Ngọc Huyền" ||
		v.LanguageCode != "vi-VN" || v.Gender != "female" || v.CreditFactor != 1 || v.Demo == "" {
		t.Errorf("phân tích snake_case sai: %+v", v)
	}
	if p.HasNextPage {
		t.Error("has_next_page=false mà báo còn trang")
	}
}

func TestListAllVoices_LapConTroRoiDung(t *testing.T) {
	calls := 0
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("cursor") == "" {
			_, _ = io.WriteString(w, `{"result":{"pagination":{"has_next_page":true,"next_cursor":"c2"},
				"voices":[{"code":"a"},{"code":"b"}]},"status":1}`)
			return
		}
		_, _ = io.WriteString(w, `{"result":{"pagination":{"has_next_page":false,"next_cursor":null},
			"voices":[{"code":"b"},{"code":"c"}]},"status":1}`)
	}))
	all, err := c.ListAllVoices(context.Background(), VoiceQuery{LanguageCode: "vi-VN"})
	if err != nil {
		t.Fatalf("ListAllVoices lỗi: %v", err)
	}
	if calls != 2 {
		t.Errorf("muốn gọi 2 trang, nhận %d", calls)
	}
	if len(all) != 3 {
		t.Fatalf("muốn 3 giọng sau khi khử trùng lặp, nhận %d: %+v", len(all), all)
	}
}

func TestListAllVoices_ChanSoTrangToiDa(t *testing.T) {
	calls := 0
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		// Máy chủ hỏng: luôn báo còn trang với con trỏ MỚI → phải bị chặn.
		_, _ = io.WriteString(w, `{"result":{"pagination":{"has_next_page":true,"next_cursor":"`+
			strings.Repeat("x", calls)+`"},"voices":[{"code":"v`+strings.Repeat("x", calls)+`"}]},"status":1}`)
	}))
	if _, err := c.ListAllVoices(context.Background(), VoiceQuery{}); err != nil {
		t.Fatalf("ListAllVoices lỗi: %v", err)
	}
	if calls != maxVoicePages {
		t.Errorf("muốn dừng ở %d trang, nhận %d", maxVoicePages, calls)
	}
}

func TestListAllVoices_LoiTrangDauThiBaoLoi(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"code":"UNAUTHORIZED","message":"nope"}}`)
	}))
	if _, err := c.ListAllVoices(context.Background(), VoiceQuery{}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("muốn ErrUnauthorized, nhận: %v", err)
	}
}

func TestClient_HuyContextThiThoatNgay(t *testing.T) {
	// release để dọn dẹp gỡ kẹt handler. Không dựa vào r.Context() vì máy chủ chỉ nhận
	// ra client đã ngắt khi thử ghi phản hồi — handler sẽ treo và Server.Close() kẹt theo.
	release := make(chan struct{})
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	// Đăng ký SAU newTestClient nên chạy TRƯỚC srv.Close (cleanup theo thứ tự ngược).
	t.Cleanup(func() { close(release) })

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := c.Submit(ctx, SpeechRequest{Text: "a", VoiceCode: "v"})
	if err == nil {
		t.Fatal("hủy ngữ cảnh phải báo lỗi")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("muốn context.Canceled, nhận: %v", err)
	}
	if IsRetryable(err) {
		t.Error("hủy ngữ cảnh không được coi là thử lại được")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("thoát quá chậm: %v", d)
	}
}

func TestIsRetryable_SentinelTranLaVinhVien(t *testing.T) {
	// Kiểm tra tham số phía client sinh sentinel trần, không kèm *Err. Thử lại những
	// lỗi này là vô ích và với API tính tiền theo ký tự thì còn tốn thêm tín dụng.
	for _, err := range []error{
		errors.New("mạng hỏng"), // lỗi mạng thuần → thử lại được
	} {
		if !IsRetryable(err) {
			t.Errorf("lỗi mạng thuần phải thử lại được: %v", err)
		}
	}
	for _, err := range []error{
		ErrUnauthorized,
		ErrBadRequest,
		errors.New("bọc: " + ErrBadRequest.Error()), // chuỗi giống nhưng không bọc → vẫn thử lại
	} {
		if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrBadRequest) {
			if IsRetryable(err) {
				t.Errorf("sentinel vĩnh viễn không được thử lại: %v", err)
			}
		}
	}
	if IsRetryable(nil) {
		t.Error("nil không phải lỗi")
	}
}

func TestNewClient_DienMacDinh(t *testing.T) {
	c := NewClient(Options{AppID: " app ", AccessToken: " tok "})
	if c.baseURL != DefaultBaseURL || c.voicesURL != DefaultVoicesURL {
		t.Errorf("URL mặc định sai: %q / %q", c.baseURL, c.voicesURL)
	}
	if c.appID != "app" || c.token != "tok" {
		t.Errorf("phải cắt khoảng trắng thông tin xác thực: %q / %q", c.appID, c.token)
	}
	if c.http == nil || c.downloader == nil {
		t.Fatal("thiếu http client mặc định")
	}
	if c.http.Timeout == c.downloader.Timeout {
		t.Error("client tải phải có hạn riêng, rộng hơn client API")
	}
}

func TestNewClient_CatDauGachCheoCuoiBaseURL(t *testing.T) {
	c := NewClient(Options{BaseURL: "https://api.vbee.vn/", VoicesURL: "https://vbee.vn/"})
	if c.baseURL != "https://api.vbee.vn" || c.voicesURL != "https://vbee.vn" {
		t.Errorf("chưa cắt dấu / cuối: %q / %q", c.baseURL, c.voicesURL)
	}
}

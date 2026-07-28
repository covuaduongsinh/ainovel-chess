package tts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
	"github.com/voocel/ainovel-cli/internal/vbee"
)

// ---------- Hạ tầng test ----------

// fakeSynth thay cho *vbee.Client: không mạng, không ngủ, kịch bản hóa được từng chương.
type fakeSynth struct {
	mu sync.Mutex

	submits   []vbee.SpeechRequest
	statusN   map[string]int // số lần đã hỏi trạng thái mỗi requestId
	downloads []string

	// submitErr trả lỗi cho lần gửi thứ n (1-based); nil = thành công.
	submitErr func(n int) error
	// statusFor quyết định trạng thái trả về theo (requestId, lần hỏi thứ mấy).
	statusFor func(id string, n int) (vbee.RequestStatus, error)
	// downloadErr trả lỗi cho lần tải thứ n (1-based) của một link.
	downloadErr func(link string, n int) error

	downloadN map[string]int
	nextID    int
}

func newFakeSynth() *fakeSynth {
	return &fakeSynth{statusN: map[string]int{}, downloadN: map[string]int{}}
}

func (f *fakeSynth) Submit(ctx context.Context, req vbee.SpeechRequest) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submits = append(f.submits, req)
	if f.submitErr != nil {
		if err := f.submitErr(len(f.submits)); err != nil {
			return "", err
		}
	}
	f.nextID++
	return fmt.Sprintf("req-%d", f.nextID), nil
}

func (f *fakeSynth) Status(ctx context.Context, id string) (vbee.RequestStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statusN[id]++
	n := f.statusN[id]
	if f.statusFor != nil {
		return f.statusFor(id, n)
	}
	return vbee.RequestStatus{RequestID: id, Status: vbee.StatusCompleted, AudioLink: "https://cdn/" + id + ".mp3"}, nil
}

func (f *fakeSynth) Download(ctx context.Context, link string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.downloads = append(f.downloads, link)
	f.downloadN[link]++
	if f.downloadErr != nil {
		if err := f.downloadErr(link, f.downloadN[link]); err != nil {
			return nil, err
		}
	}
	return []byte("ID3" + link), nil
}

func (f *fakeSynth) submitCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.submits)
}

func (f *fakeSynth) submittedTexts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.submits))
	for _, s := range f.submits {
		out = append(out, s.Text)
	}
	return out
}

// tolerantTempDir: os.RemoveAll hay hỏng trên Windows ngay sau fsync+rename, thử lại
// vài nhịp. Chép từ adapt/runner_test.go:51-66.
func tolerantTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "tts-test-*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() {
		for i := 0; i < 10; i++ {
			if err := os.RemoveAll(dir); err == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
	return dir
}

func newTestStore(t *testing.T, completed []int) (*store.Store, string) {
	t.Helper()
	dir := tolerantTempDir(t)
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	if err := s.Progress.Init("Ánh Chớp", len(completed)); err != nil {
		t.Fatalf("init progress: %v", err)
	}
	for _, ch := range completed {
		if err := s.Drafts.SaveFinalChapter(ch, fmt.Sprintf("Nội dung chương %d. Lâm bước vào tửu lâu.", ch)); err != nil {
			t.Fatalf("save chapter: %v", err)
		}
		if err := s.Progress.StartChapter(ch); err != nil {
			t.Fatalf("start chapter: %v", err)
		}
		if err := s.Progress.MarkChapterComplete(ch, 8, "cliff", "main"); err != nil {
			t.Fatalf("mark complete: %v", err)
		}
	}
	entries := make([]domain.OutlineEntry, 0, len(completed))
	for _, ch := range completed {
		entries = append(entries, domain.OutlineEntry{Chapter: ch, Title: fmt.Sprintf("Tiêu đề %d", ch)})
	}
	if err := s.Outline.SaveOutline(entries); err != nil {
		t.Fatalf("save outline: %v", err)
	}
	return s, dir
}

// fastTiming rút mọi khoảng chờ xuống mức mili-giây để cả bộ test chạy dưới một giây.
func fastTiming() Timing {
	return Timing{
		InitialDelay:     time.Millisecond,
		MinInterval:      time.Millisecond,
		MaxInterval:      2 * time.Millisecond,
		BackoffFactor:    1.5,
		ChapterTimeout:   2 * time.Second,
		RetryBase:        time.Millisecond,
		SubmitAttempts:   3,
		DownloadAttempts: 3,
		MaxPollErrors:    3,
	}
}

func baseOptions() Options {
	return Options{VoiceCode: "hn_female_ngochuyen_full_48k-fhg"}
}

func drain(ch <-chan Event) []Event {
	var out []Event
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}

func lastStage(events []Event) Stage {
	if len(events) == 0 {
		return ""
	}
	return events[len(events)-1].Stage
}

func allMessages(events []Event) string {
	var b strings.Builder
	for _, ev := range events {
		b.WriteString(ev.Message)
		b.WriteString("\n")
	}
	return b.String()
}

// runOnce chạy trọn một lần và trả về các sự kiện.
func runOnce(t *testing.T, st *store.Store, f *fakeSynth, opts Options) []Event {
	t.Helper()
	ch, err := Run(context.Background(), Deps{Store: st, Vbee: f, Timing: fastTiming()}, opts)
	if err != nil {
		t.Fatalf("Run trả lỗi ngay: %v", err)
	}
	return drain(ch)
}

// relFiles liệt kê các tệp trong dir theo đường dẫn tương đối dùng dấu /.
func relFiles(t *testing.T, dir string) []string {
	t.Helper()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil // chưa ghi gì cả — hợp lệ khi run bị hủy sớm
	}
	var out []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("duyệt thư mục: %v", err)
	}
	sort.Strings(out)
	return out
}

// ---------- Kiểm tra tham số đầu vào ----------

func TestRun_ThieuDepsTraLoiNgay(t *testing.T) {
	if _, err := Run(context.Background(), Deps{}, baseOptions()); err == nil {
		t.Fatal("thiếu deps phải báo lỗi ngay")
	}
}

func TestRun_ThieuGiongDocTraLoiNgay(t *testing.T) {
	st, _ := newTestStore(t, []int{1})
	_, err := Run(context.Background(), Deps{Store: st, Vbee: newFakeSynth()}, Options{})
	if err == nil {
		t.Fatal("thiếu giọng đọc phải báo lỗi ngay")
	}
	if !strings.Contains(err.Error(), "giọng đọc") {
		t.Errorf("thông báo phải nhắc tới giọng đọc: %v", err)
	}
}

func TestRun_KhongCoChuongHoanThanhThiStageError(t *testing.T) {
	dir := tolerantTempDir(t)
	st := store.NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	if err := st.Progress.Init("Trống", 0); err != nil {
		t.Fatalf("init progress: %v", err)
	}
	events := runOnce(t, st, newFakeSynth(), baseOptions())
	if lastStage(events) != StageError {
		t.Errorf("muốn StageError, nhận %q", lastStage(events))
	}
}

// ---------- Đường đi thành công ----------

func TestRun_MotMP3MoiChuongVaPlaylist(t *testing.T) {
	st, _ := newTestStore(t, []int{1, 2, 3})
	f := newFakeSynth()
	outDir := filepath.Join(tolerantTempDir(t), "audio")

	opts := baseOptions()
	opts.OutDir = outDir
	events := runOnce(t, st, f, opts)

	if lastStage(events) != StageDone {
		t.Fatalf("muốn StageDone, nhận %q; log:\n%s", lastStage(events), allMessages(events))
	}
	want := []string{
		"index.md",
		"playlist.m3u",
		"tap-01/chuong-001.mp3",
		"tap-01/chuong-002.mp3",
		"tap-01/chuong-003.mp3",
		"tap-01/playlist.m3u",
	}
	got := relFiles(t, outDir)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("danh sách tệp sai.\nnhận: %v\nmuốn: %v", got, want)
	}
	if f.submitCount() != 3 {
		t.Errorf("muốn 3 lần gửi Vbee, nhận %d", f.submitCount())
	}
}

func TestRun_GuiDungThamSoVoiceSpeedBitrate(t *testing.T) {
	st, _ := newTestStore(t, []int{1})
	f := newFakeSynth()

	opts := baseOptions()
	opts.OutDir = filepath.Join(tolerantTempDir(t), "audio")
	opts.Speed = 1.25
	opts.Bitrate = 64
	opts.SampleRate = 44100
	opts.OutputFormat = "wav"
	opts.WebhookURL = "https://webhook.site/abc"
	opts.Pause = PauseOptions{SentenceBreak: 0.7}
	runOnce(t, st, f, opts)

	if len(f.submits) != 1 {
		t.Fatalf("muốn 1 lần gửi, nhận %d", len(f.submits))
	}
	got := f.submits[0]
	if got.VoiceCode != opts.VoiceCode || got.Speed != 1.25 || got.Bitrate != 64 ||
		got.SampleRate != 44100 || got.OutputFormat != "wav" || got.WebhookURL != "https://webhook.site/abc" {
		t.Errorf("tham số gửi Vbee sai: %+v", got)
	}
	if got.ClientPause == nil || got.ClientPause.SentenceBreak != 0.7 {
		t.Errorf("clientPause chưa được truyền: %+v", got.ClientPause)
	}
}

func TestRun_LoiDanDuocGuiKemNoiDung(t *testing.T) {
	st, _ := newTestStore(t, []int{2})
	f := newFakeSynth()
	opts := baseOptions()
	opts.OutDir = filepath.Join(tolerantTempDir(t), "audio")
	runOnce(t, st, f, opts)

	texts := f.submittedTexts()
	if len(texts) != 1 {
		t.Fatalf("muốn 1 lần gửi, nhận %d", len(texts))
	}
	if !strings.HasPrefix(texts[0], "Chương 2. Tiêu đề 2.") {
		t.Errorf("thiếu lời dẫn đầu chương: %q", texts[0])
	}
}

func TestRun_PhamViFromTo(t *testing.T) {
	st, _ := newTestStore(t, []int{1, 2, 3, 4, 5})
	f := newFakeSynth()
	outDir := filepath.Join(tolerantTempDir(t), "audio")

	opts := baseOptions()
	opts.OutDir = outDir
	opts.From, opts.To = 2, 4
	events := runOnce(t, st, f, opts)

	if lastStage(events) != StageDone {
		t.Fatalf("muốn StageDone, nhận %q", lastStage(events))
	}
	if f.submitCount() != 3 {
		t.Errorf("muốn 3 chương trong [2,4], nhận %d", f.submitCount())
	}
	for _, banned := range []string{"tap-01/chuong-001.mp3", "tap-01/chuong-005.mp3"} {
		if exists(filepath.Join(outDir, filepath.FromSlash(banned))) {
			t.Errorf("chương ngoài phạm vi vẫn được tạo: %s", banned)
		}
	}
}

func TestRun_Overwrite(t *testing.T) {
	st, _ := newTestStore(t, []int{1, 2})
	outDir := filepath.Join(tolerantTempDir(t), "audio")
	sentinelPath := filepath.Join(outDir, "tap-01", "chuong-001.mp3")
	if _, err := atomicWrite(sentinelPath, []byte("SENTINEL")); err != nil {
		t.Fatalf("ghi sentinel: %v", err)
	}

	// Overwrite=false → phải bỏ qua tệp đã có, KHÔNG tốn thêm tín dụng Vbee.
	f := newFakeSynth()
	opts := baseOptions()
	opts.OutDir = outDir
	runOnce(t, st, f, opts)
	if f.submitCount() != 1 {
		t.Errorf("resume phải chỉ gửi 1 chương còn thiếu, nhận %d", f.submitCount())
	}
	if data, _ := os.ReadFile(sentinelPath); string(data) != "SENTINEL" {
		t.Errorf("tệp cũ bị ghi đè dù Overwrite=false: %q", data)
	}

	// Overwrite=true → làm lại tất cả.
	f2 := newFakeSynth()
	opts.Overwrite = true
	runOnce(t, st, f2, opts)
	if f2.submitCount() != 2 {
		t.Errorf("Overwrite=true phải gửi lại cả 2 chương, nhận %d", f2.submitCount())
	}
	if data, _ := os.ReadFile(sentinelPath); string(data) == "SENTINEL" {
		t.Error("Overwrite=true mà tệp cũ không bị thay")
	}
}

func TestRun_ResumeVanDungLaiPlaylistDayDu(t *testing.T) {
	st, _ := newTestStore(t, []int{1, 2})
	outDir := filepath.Join(tolerantTempDir(t), "audio")
	opts := baseOptions()
	opts.OutDir = outDir

	runOnce(t, st, newFakeSynth(), opts)
	runOnce(t, st, newFakeSynth(), opts) // lần hai: mọi tệp đã có

	data, err := os.ReadFile(filepath.Join(outDir, "playlist.m3u"))
	if err != nil {
		t.Fatalf("đọc playlist: %v", err)
	}
	for _, want := range []string{"tap-01/chuong-001.mp3", "tap-01/chuong-002.mp3"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("chạy resume làm ngắn danh sách phát, thiếu %q:\n%s", want, data)
		}
	}
}

// ---------- Chính sách lỗi ----------

func TestRun_MotChuongFAILEDThiBoQuaVaTiepTuc(t *testing.T) {
	st, _ := newTestStore(t, []int{1, 2, 3})
	f := newFakeSynth()
	f.statusFor = func(id string, n int) (vbee.RequestStatus, error) {
		if id == "req-2" {
			return vbee.RequestStatus{RequestID: id, Status: vbee.StatusFailed}, nil
		}
		return vbee.RequestStatus{RequestID: id, Status: vbee.StatusCompleted, AudioLink: "https://cdn/" + id + ".mp3"}, nil
	}
	outDir := filepath.Join(tolerantTempDir(t), "audio")
	opts := baseOptions()
	opts.OutDir = outDir
	events := runOnce(t, st, f, opts)

	if lastStage(events) != StageDone {
		t.Fatalf("một chương lỗi vẫn phải kết thúc StageDone, nhận %q; log:\n%s", lastStage(events), allMessages(events))
	}
	if !strings.Contains(allMessages(events), "bỏ qua 1 chương lỗi: 2") {
		t.Errorf("thông báo kết thúc phải nêu chương bị bỏ qua:\n%s", allMessages(events))
	}
	if exists(filepath.Join(outDir, "tap-01", "chuong-002.mp3")) {
		t.Error("chương lỗi không được để lại tệp")
	}
	for _, want := range []string{"chuong-001.mp3", "chuong-003.mp3"} {
		if !exists(filepath.Join(outDir, "tap-01", want)) {
			t.Errorf("chương lành phải được tạo: %s", want)
		}
	}
}

func TestRun_TatCaChuongLoiThiStageError(t *testing.T) {
	st, _ := newTestStore(t, []int{1, 2})
	f := newFakeSynth()
	f.statusFor = func(id string, n int) (vbee.RequestStatus, error) {
		return vbee.RequestStatus{RequestID: id, Status: vbee.StatusFailed}, nil
	}
	opts := baseOptions()
	opts.OutDir = filepath.Join(tolerantTempDir(t), "audio")
	events := runOnce(t, st, f, opts)

	if lastStage(events) != StageError {
		t.Errorf("mọi chương hỏng phải cho StageError chứ không phải xanh giả, nhận %q", lastStage(events))
	}
}

func TestRun_UnauthorizedDungNgayVaChiGuiMotLan(t *testing.T) {
	st, _ := newTestStore(t, []int{1, 2, 3})
	f := newFakeSynth()
	f.submitErr = func(n int) error { return fmt.Errorf("bọc: %w", vbee.ErrUnauthorized) }
	opts := baseOptions()
	opts.OutDir = filepath.Join(tolerantTempDir(t), "audio")
	events := runOnce(t, st, f, opts)

	if lastStage(events) != StageError {
		t.Errorf("muốn StageError, nhận %q", lastStage(events))
	}
	if f.submitCount() != 1 {
		t.Errorf("sai thông tin xác thực phải dừng sau đúng 1 lần gửi, nhận %d", f.submitCount())
	}
}

func TestRun_BadRequestChuaChuongNaoThanhCongThiDung(t *testing.T) {
	st, _ := newTestStore(t, []int{1, 2, 3})
	f := newFakeSynth()
	f.submitErr = func(n int) error {
		return fmt.Errorf("webhookUrl must be defined: %w", vbee.ErrBadRequest)
	}
	opts := baseOptions()
	opts.OutDir = filepath.Join(tolerantTempDir(t), "audio")
	events := runOnce(t, st, f, opts)

	if lastStage(events) != StageError {
		t.Errorf("muốn StageError, nhận %q", lastStage(events))
	}
	if f.submitCount() != 1 {
		t.Errorf("lỗi hình dạng yêu cầu phải lộ ngay ở chương đầu, nhận %d lần gửi", f.submitCount())
	}
}

func TestRun_BadRequestSauKhiDaCoChuongThanhCongThiBoQua(t *testing.T) {
	st, _ := newTestStore(t, []int{1, 2, 3})
	f := newFakeSynth()
	f.submitErr = func(n int) error {
		if n == 2 {
			return fmt.Errorf("text quá dài: %w", vbee.ErrBadRequest)
		}
		return nil
	}
	opts := baseOptions()
	opts.OutDir = filepath.Join(tolerantTempDir(t), "audio")
	events := runOnce(t, st, f, opts)

	if lastStage(events) != StageDone {
		t.Fatalf("lỗi riêng một chương phải fail-soft, nhận %q; log:\n%s", lastStage(events), allMessages(events))
	}
	if f.submitCount() != 3 {
		t.Errorf("phải thử đủ 3 chương, nhận %d", f.submitCount())
	}
}

func TestRun_LoiGhiDiaThiDungCaRun(t *testing.T) {
	st, _ := newTestStore(t, []int{1, 2})
	// Đặt OutDir trùng tên một TỆP đã tồn tại → MkdirAll chắc chắn hỏng.
	blocker := filepath.Join(tolerantTempDir(t), "chan")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("ghi tệp chặn: %v", err)
	}
	opts := baseOptions()
	opts.OutDir = blocker
	events := runOnce(t, st, newFakeSynth(), opts)

	if lastStage(events) != StageError {
		t.Errorf("lỗi ghi đĩa phải dừng cả run, nhận %q", lastStage(events))
	}
}

// ---------- Poll và link hết hạn ----------

func TestRun_PollProcessingRoiCompleted(t *testing.T) {
	st, _ := newTestStore(t, []int{1})
	f := newFakeSynth()
	f.statusFor = func(id string, n int) (vbee.RequestStatus, error) {
		if n < 3 {
			return vbee.RequestStatus{RequestID: id, Status: vbee.StatusProcessing}, nil
		}
		return vbee.RequestStatus{RequestID: id, Status: vbee.StatusCompleted, AudioLink: "https://cdn/" + id + ".mp3"}, nil
	}
	outDir := filepath.Join(tolerantTempDir(t), "audio")
	opts := baseOptions()
	opts.OutDir = outDir
	events := runOnce(t, st, f, opts)

	if lastStage(events) != StageDone {
		t.Fatalf("muốn StageDone, nhận %q; log:\n%s", lastStage(events), allMessages(events))
	}
	if f.statusN["req-1"] < 3 {
		t.Errorf("phải poll đủ số lần, nhận %d", f.statusN["req-1"])
	}
	if !exists(filepath.Join(outDir, "tap-01", "chuong-001.mp3")) {
		t.Error("thiếu tệp âm thanh")
	}
}

func TestRun_LinkHetHanThiPollLaiLayLinkMoi(t *testing.T) {
	st, _ := newTestStore(t, []int{1})
	f := newFakeSynth()
	f.statusFor = func(id string, n int) (vbee.RequestStatus, error) {
		// Lần hỏi đầu trả link CŨ (sẽ hết hạn), các lần sau trả link MỚI.
		link := "https://cdn/cu.mp3"
		if n > 1 {
			link = "https://cdn/moi.mp3"
		}
		return vbee.RequestStatus{RequestID: id, Status: vbee.StatusCompleted, AudioLink: link}, nil
	}
	f.downloadErr = func(link string, n int) error {
		if strings.Contains(link, "cu.mp3") {
			return fmt.Errorf("403 link đã hết hạn")
		}
		return nil
	}
	outDir := filepath.Join(tolerantTempDir(t), "audio")
	opts := baseOptions()
	opts.OutDir = outDir
	events := runOnce(t, st, f, opts)

	if lastStage(events) != StageDone {
		t.Fatalf("phải xin được link mới rồi tải xong, nhận %q; log:\n%s", lastStage(events), allMessages(events))
	}
	if len(f.downloads) < 2 {
		t.Errorf("phải tải lại sau khi link hết hạn, nhận %d lần tải", len(f.downloads))
	}
	data, err := os.ReadFile(filepath.Join(outDir, "tap-01", "chuong-001.mp3"))
	if err != nil {
		t.Fatalf("đọc tệp: %v", err)
	}
	if !strings.Contains(string(data), "moi.mp3") {
		t.Errorf("phải ghi nội dung tải từ link MỚI, nhận %q", data)
	}
}

func TestRun_QuaHanChoThiBoQuaChuong(t *testing.T) {
	st, _ := newTestStore(t, []int{1, 2})
	f := newFakeSynth()
	f.statusFor = func(id string, n int) (vbee.RequestStatus, error) {
		if id == "req-1" {
			return vbee.RequestStatus{RequestID: id, Status: vbee.StatusProcessing}, nil
		}
		return vbee.RequestStatus{RequestID: id, Status: vbee.StatusCompleted, AudioLink: "https://cdn/" + id + ".mp3"}, nil
	}
	timing := fastTiming()
	timing.ChapterTimeout = 30 * time.Millisecond

	opts := baseOptions()
	opts.OutDir = filepath.Join(tolerantTempDir(t), "audio")
	ch, err := Run(context.Background(), Deps{Store: st, Vbee: f, Timing: timing}, opts)
	if err != nil {
		t.Fatalf("Run lỗi: %v", err)
	}
	events := drain(ch)

	if lastStage(events) != StageDone {
		t.Fatalf("quá hạn một chương phải fail-soft, nhận %q; log:\n%s", lastStage(events), allMessages(events))
	}
	msgs := allMessages(events)
	if !strings.Contains(msgs, "quá hạn") || !strings.Contains(msgs, "req-1") {
		t.Errorf("thông báo phải nêu lý do quá hạn kèm requestId:\n%s", msgs)
	}
}

func TestRun_MatLienLacKhiPollThiBoQuaChuong(t *testing.T) {
	st, _ := newTestStore(t, []int{1, 2})
	f := newFakeSynth()
	f.statusFor = func(id string, n int) (vbee.RequestStatus, error) {
		if id == "req-1" {
			return vbee.RequestStatus{}, fmt.Errorf("mạng hỏng")
		}
		return vbee.RequestStatus{RequestID: id, Status: vbee.StatusCompleted, AudioLink: "https://cdn/" + id + ".mp3"}, nil
	}
	outDir := filepath.Join(tolerantTempDir(t), "audio")
	opts := baseOptions()
	opts.OutDir = outDir
	events := runOnce(t, st, f, opts)

	if lastStage(events) != StageDone {
		t.Fatalf("mất liên lạc một chương phải fail-soft, nhận %q", lastStage(events))
	}
	if !exists(filepath.Join(outDir, "tap-01", "chuong-002.mp3")) {
		t.Error("chương lành phải vẫn được tạo")
	}
}

func TestRun_GuiThatBaiTamThoiThiThuLai(t *testing.T) {
	st, _ := newTestStore(t, []int{1})
	f := newFakeSynth()
	f.submitErr = func(n int) error {
		if n == 1 {
			return &vbee.Err{Code: "INTERNAL_SERVER_ERROR", StatusCode: 500}
		}
		return nil
	}
	opts := baseOptions()
	opts.OutDir = filepath.Join(tolerantTempDir(t), "audio")
	events := runOnce(t, st, f, opts)

	if lastStage(events) != StageDone {
		t.Fatalf("lỗi 500 phải được thử lại, nhận %q; log:\n%s", lastStage(events), allMessages(events))
	}
	if f.submitCount() != 2 {
		t.Errorf("muốn 2 lần gửi (1 hỏng + 1 lại), nhận %d", f.submitCount())
	}
}

// ---------- Hủy ----------

func TestRun_HuyContextThiKenhDongVaKhongDeLaiTepDo(t *testing.T) {
	st, _ := newTestStore(t, []int{1, 2, 3})
	f := newFakeSynth()
	f.statusFor = func(id string, n int) (vbee.RequestStatus, error) {
		return vbee.RequestStatus{RequestID: id, Status: vbee.StatusProcessing}, nil
	}
	outDir := filepath.Join(tolerantTempDir(t), "audio")
	opts := baseOptions()
	opts.OutDir = outDir

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := Run(ctx, Deps{Store: st, Vbee: f, Timing: fastTiming()}, opts)
	if err != nil {
		t.Fatalf("Run lỗi: %v", err)
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	done := make(chan []Event, 1)
	go func() { done <- drain(ch) }()
	select {
	case events := <-done:
		if lastStage(events) != StageError {
			t.Errorf("hủy phải cho StageError, nhận %q", lastStage(events))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("hủy ngữ cảnh mà kênh không đóng")
	}

	for _, f := range relFiles(t, outDir) {
		if strings.Contains(f, ".tmp-") {
			t.Errorf("còn sót tệp tạm: %s", f)
		}
	}
}

// ---------- Chia phần ----------

func TestRun_ChiaPhanKhiChuongQuaDai(t *testing.T) {
	st, _ := newTestStore(t, []int{1})
	f := newFakeSynth()
	outDir := filepath.Join(tolerantTempDir(t), "audio")

	opts := baseOptions()
	opts.OutDir = outDir
	opts.MaxChars = 25 // nội dung mẫu dài hơn nhiều
	events := runOnce(t, st, f, opts)

	if lastStage(events) != StageDone {
		t.Fatalf("muốn StageDone, nhận %q; log:\n%s", lastStage(events), allMessages(events))
	}
	got := relFiles(t, outDir)
	joined := strings.Join(got, "|")
	if !strings.Contains(joined, "tap-01/chuong-001-p1.mp3") || !strings.Contains(joined, "tap-01/chuong-001-p2.mp3") {
		t.Fatalf("phải chia thành nhiều phần, nhận: %v", got)
	}
	if strings.Contains(joined, "tap-01/chuong-001.mp3") {
		t.Errorf("chương đã chia phần không được có tệp không đánh số: %v", got)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "playlist.m3u"))
	if err != nil {
		t.Fatalf("đọc playlist: %v", err)
	}
	for _, want := range []string{"chuong-001-p1.mp3", "chuong-001-p2.mp3", "(phần 1)", "(phần 2)"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("danh sách phát thiếu %q:\n%s", want, data)
		}
	}
}

func TestRun_ChuongLoiGiuaChungKhongDeLaiNuaBoPhan(t *testing.T) {
	st, _ := newTestStore(t, []int{1})
	f := newFakeSynth()
	// Phần đầu thành công, phần sau hỏng vĩnh viễn.
	f.statusFor = func(id string, n int) (vbee.RequestStatus, error) {
		if id == "req-2" {
			return vbee.RequestStatus{RequestID: id, Status: vbee.StatusFailed}, nil
		}
		return vbee.RequestStatus{RequestID: id, Status: vbee.StatusCompleted, AudioLink: "https://cdn/" + id + ".mp3"}, nil
	}
	outDir := filepath.Join(tolerantTempDir(t), "audio")
	opts := baseOptions()
	opts.OutDir = outDir
	opts.MaxChars = 25
	runOnce(t, st, f, opts)

	for _, f := range relFiles(t, outDir) {
		if strings.HasSuffix(f, ".mp3") {
			t.Errorf("chương hỏng giữa chừng không được để lại tệp phần nào: %s", f)
		}
	}
}

// ---------- Playlist / index ----------

func TestPlaylist_HeaderVaExtinf(t *testing.T) {
	outs := []Output{
		{Chapter: 1, Title: "Mở đầu", Rel: "tap-01/chuong-001.mp3"},
		{Chapter: 2, Title: "", Part: 1, Rel: "tap-01/chuong-002-p1.mp3"},
	}
	got := renderPlaylist("Ánh Chớp", outs, "")
	for _, want := range []string{
		"#EXTM3U\n",
		"#PLAYLIST:Ánh Chớp\n",
		"#EXTINF:-1,Chương 1 — Mở đầu\n",
		"tap-01/chuong-001.mp3\n",
		"#EXTINF:-1,Chương 2 (phần 1)\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("thiếu %q trong:\n%s", want, got)
		}
	}
}

func TestPlaylist_LuonDungDauGachCheoXuoi(t *testing.T) {
	got := renderPlaylist("X", []Output{{Chapter: 1, Rel: "tap-01/chuong-001.mp3"}}, "")
	if strings.Contains(got, `\`) {
		t.Errorf("danh sách phát không được chứa dấu \\ (mất tính di động trên Windows):\n%s", got)
	}
}

func TestPlaylist_CapTapCatTienTo(t *testing.T) {
	got := renderPlaylist("Tập 1", []Output{{Chapter: 1, Rel: "tap-01/chuong-001.mp3"}}, "tap-01/")
	if !strings.Contains(got, "\nchuong-001.mp3\n") {
		t.Errorf("danh sách phát cấp tập phải trỏ tệp bên cạnh nó:\n%s", got)
	}
}

func TestPlaylist_SapXepTheoTapChuongPhan(t *testing.T) {
	in := []Output{
		{Volume: 2, Chapter: 5},
		{Volume: 1, Chapter: 3, Part: 2},
		{Volume: 1, Chapter: 3, Part: 1},
		{Volume: 1, Chapter: 1},
	}
	got := sortedOutputs(in)
	want := []string{"1/1/0", "1/3/1", "1/3/2", "2/5/0"}
	for i, o := range got {
		if key := fmt.Sprintf("%d/%d/%d", o.Volume, o.Chapter, o.Part); key != want[i] {
			t.Errorf("vị trí %d: nhận %s, muốn %s", i, key, want[i])
		}
	}
}

func TestIndexMarkdown_LietKeChuongBoQuaVaTongKyTu(t *testing.T) {
	st, _ := newTestStore(t, []int{1, 2})
	f := newFakeSynth()
	f.statusFor = func(id string, n int) (vbee.RequestStatus, error) {
		if id == "req-2" {
			return vbee.RequestStatus{RequestID: id, Status: vbee.StatusFailed}, nil
		}
		return vbee.RequestStatus{RequestID: id, Status: vbee.StatusCompleted, AudioLink: "https://cdn/" + id + ".mp3"}, nil
	}
	outDir := filepath.Join(tolerantTempDir(t), "audio")
	opts := baseOptions()
	opts.OutDir = outDir
	runOnce(t, st, f, opts)

	data, err := os.ReadFile(filepath.Join(outDir, "index.md"))
	if err != nil {
		t.Fatalf("đọc index.md: %v", err)
	}
	md := string(data)
	for _, want := range []string{
		"# Sách nói — Ánh Chớp",
		"Tổng ký tự đã gửi Vbee:",
		"## Chương bị bỏ qua",
		"Chương 2",
		opts.VoiceCode,
	} {
		if !strings.Contains(md, want) {
			t.Errorf("index.md thiếu %q:\n%s", want, md)
		}
	}
}

// ---------- Phân loại lỗi ----------

func TestIsFatal(t *testing.T) {
	if isFatal(errors.New("thường")) {
		t.Error("lỗi thường không được coi là fatal")
	}
	err := fatal(fmt.Errorf("bọc: %w", vbee.ErrUnauthorized))
	if !isFatal(err) {
		t.Error("fatal() phải nhận diện được")
	}
	if !errors.Is(err, vbee.ErrUnauthorized) {
		t.Error("fatal() phải giữ được errors.Is xuống lỗi gốc")
	}
}

// Package tts là "khả năng ngang" biến một dự án sách đã hoàn thành thành SÁCH NÓI:
// mỗi chương một tệp MP3, kèm danh sách phát và mục lục.
//
// Giống adapt/exp: chỉ ĐỌC dữ liệu có sẵn trong store rồi GHI ra thư mục ngoài
// (mặc định {novelDir}/audio/). Không đụng tới Coordinator/Phase/Flow — host bọc bằng
// guardExclusive để không chạy chồng.
//
// Khác adapt ở một điểm quan trọng: KHÔNG gọi LLM. Chi phí nằm ở tín dụng Vbee, tính
// theo SỐ KÝ TỰ gửi đi, nên không đi qua UsageTracker/Budget của sách.
package tts

import (
	"context"
	"time"

	"github.com/voocel/ainovel-cli/internal/store"
	"github.com/voocel/ainovel-cli/internal/vbee"
)

// Synthesizer là giao diện tối thiểu tới nhà cung cấp TTS (mặc định *vbee.Client).
//
// Cố ý tách ba nguyên thủy chứ không gộp một lời gọi Speak(): runner cần phát sự kiện
// tiến trình gửi → chờ → tải, và test cần giả lập được cả trường hợp link hết hạn
// (Download hỏng một lần rồi Status trả link mới).
type Synthesizer interface {
	Submit(ctx context.Context, req vbee.SpeechRequest) (string, error)
	Status(ctx context.Context, requestID string) (vbee.RequestStatus, error)
	Download(ctx context.Context, audioURL string) ([]byte, error)
}

// Deps là phụ thuộc chạy do host tiêm vào.
type Deps struct {
	Store  *store.Store
	Vbee   Synthesizer
	Timing Timing // rỗng = mặc định; test thu nhỏ để chạy nhanh
}

// PauseOptions tinh chỉnh độ dài các khoảng nghỉ khi đọc (giây), ánh xạ 1-1 sang
// clientPause của Vbee. Rỗng = để Vbee tự quyết.
type PauseOptions struct {
	MajorBreak     float64 // dấu chấm phẩy
	MediumBreak    float64 // dấu phẩy
	ParagraphBreak float64 // xuống dòng
	SentenceBreak  float64 // dấu chấm
}

// IsZero cho biết người dùng có tinh chỉnh khoảng nghỉ không.
func (p PauseOptions) IsZero() bool { return p == PauseOptions{} }

// toVbee chuyển sang kiểu của client; trả nil khi người dùng không tinh chỉnh gì.
func (p PauseOptions) toVbee() *vbee.ClientPause {
	if p.IsZero() {
		return nil
	}
	return &vbee.ClientPause{
		MajorBreak:     p.MajorBreak,
		MediumBreak:    p.MediumBreak,
		ParagraphBreak: p.ParagraphBreak,
		SentenceBreak:  p.SentenceBreak,
	}
}

// DefaultMaxChars đặt dưới giới hạn 100.000 của Vbee một quãng đệm rộng: chương thực
// tế chỉ khoảng 20.000 rune nên gần như không bao giờ phải chia phần, mà nếu cách đếm
// ký tự của Vbee khác ta (họ có thể đếm sau khi chuẩn hóa số) thì vẫn còn dư địa.
const DefaultMaxChars = 90000

// Options là tham số điều khiển, dùng chung cho cả Web và TUI (mirror).
type Options struct {
	From, To int // phạm vi chương (0/0 = toàn bộ đã hoàn thành)

	// VoiceCode là giọng đọc DUY NHẤT cho toàn bộ sách (giai đoạn 1).
	VoiceCode string
	// VoiceMap dành cho tương lai: gán giọng riêng cho từng nhân vật/vai (khóa = tên
	// nhân vật hoặc "narrator"). Runner hiện CHƯA đọc trường này — giữ sẵn để thêm
	// giọng-theo-nhân-vật mà không phải đổi hình dạng Options.
	VoiceMap map[string]string

	Speed        float64 // 0.25–1.9; 0 = 1.0
	Bitrate      int     // 8|16|32|64|128; 0 = 128
	SampleRate   int     // 0 = 24000
	OutputFormat string  // "" = "mp3"
	Pause        PauseOptions

	// WebhookURL: Vbee ghi là bắt buộc với mode async. Ứng dụng chạy localhost nên
	// không có URL công khai — ta gửi URL giữ chỗ rồi CHỦ ĐỘNG poll trạng thái.
	WebhookURL string // "" = vbee.DefaultWebhookURL

	// MaxChars là ngưỡng chia phần, đếm theo RUNE; 0 = DefaultMaxChars.
	MaxChars int
	// SceneBreakText thay cho đường kẻ ngang `---` khi đọc; "" = chỉ ngắt đoạn.
	SceneBreakText string

	OutDir    string // rỗng = {novelDir}/audio/
	Overwrite bool   // ghi đè tệp đã tồn tại; false = bỏ qua (resume incremental)
}

// normalize điền mặc định cho các trường bỏ trống.
func (o Options) normalize() Options {
	if o.Speed == 0 {
		o.Speed = 1.0
	}
	if o.Bitrate == 0 {
		o.Bitrate = 128
	}
	if o.SampleRate == 0 {
		o.SampleRate = 24000
	}
	if o.OutputFormat == "" {
		o.OutputFormat = "mp3"
	}
	if o.WebhookURL == "" {
		o.WebhookURL = vbee.DefaultWebhookURL
	}
	if o.MaxChars <= 0 {
		o.MaxChars = DefaultMaxChars
	}
	return o
}

// Stage là giai đoạn tiến trình, chiếu ra UI.
type Stage string

const (
	StageContext  Stage = "context"  // đọc store, xác định phạm vi chương
	StagePrepare  Stage = "prepare"  // làm sạch markdown + chia phần
	StageSubmit   Stage = "submit"   // gửi yêu cầu tới Vbee
	StageWait     Stage = "wait"     // chờ Vbee xử lý (poll)
	StageDownload Stage = "download" // tải audioLink
	StageWrite    Stage = "write"    // ghi tệp âm thanh
	StageIndex    Stage = "index"    // ghi playlist.m3u + index.md
	StageDone     Stage = "done"
	StageError    Stage = "error"
)

// Event là một mốc tiến trình gửi qua kênh.
type Event struct {
	Time    time.Time
	Stage   Stage
	Chapter int // 0 = không thuộc chương nào
	Part    int // 0 = chương không chia phần; 1..n = phần thứ mấy
	Current int
	Total   int
	Message string // tiếng Việt có dấu
	Err     error
}

// Output mô tả một tệp âm thanh đã ghi.
type Output struct {
	Chapter int    `json:"chapter"`
	Part    int    `json:"part"` // 0 = chương không chia phần
	Volume  int    `json:"volume"`
	Title   string `json:"title"`
	Path    string `json:"path"`
	Rel     string `json:"rel"` // đường dẫn tương đối, LUÔN dùng dấu / (cho .m3u)
	Bytes   int    `json:"bytes"`
	Chars   int    `json:"chars"` // số rune đã gửi Vbee — cơ sở đối chiếu tín dụng
}

// Skipped mô tả một chương bị bỏ qua, để ghi vào index.md.
type Skipped struct {
	Chapter int    `json:"chapter"`
	Title   string `json:"title"`
	Reason  string `json:"reason"`
}

// Timing gom mọi hằng số thời gian của vòng gửi→chờ→tải. Tách ra để test rút ngắn
// xuống mức mili-giây thay vì chờ thật.
type Timing struct {
	InitialDelay     time.Duration // chờ trước lần poll đầu
	MinInterval      time.Duration
	MaxInterval      time.Duration
	BackoffFactor    float64
	ChapterTimeout   time.Duration // hạn chót cho MỘT phần
	RetryBase        time.Duration // đơn vị lùi khi thử lại submit/download
	SubmitAttempts   int
	DownloadAttempts int
	MaxPollErrors    int // số lỗi poll liên tiếp tối đa trước khi bỏ phần này
}

// normalize điền mặc định cho mọi trường bằng 0.
func (t Timing) normalize() Timing {
	if t.InitialDelay <= 0 {
		t.InitialDelay = 5 * time.Second
	}
	if t.MinInterval <= 0 {
		t.MinInterval = 3 * time.Second
	}
	if t.MaxInterval <= 0 {
		t.MaxInterval = 20 * time.Second
	}
	if t.MaxInterval < t.MinInterval {
		t.MaxInterval = t.MinInterval
	}
	if t.BackoffFactor < 1 {
		t.BackoffFactor = 1.5
	}
	if t.ChapterTimeout <= 0 {
		t.ChapterTimeout = 15 * time.Minute
	}
	if t.RetryBase <= 0 {
		t.RetryBase = 2 * time.Second
	}
	if t.SubmitAttempts <= 0 {
		t.SubmitAttempts = 3
	}
	if t.DownloadAttempts <= 0 {
		t.DownloadAttempts = 3
	}
	if t.MaxPollErrors <= 0 {
		t.MaxPollErrors = 5
	}
	return t
}

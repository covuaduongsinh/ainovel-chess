package web

import (
	"time"

	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/tools"
)

// sseMessage là phong bì thống nhất mà máy chủ đẩy cho trình duyệt qua SSE.
// Type quyết định cách frontend phân tích Data.
type sseMessage struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

// Hằng số kiểu tin nhắn SSE (tương ứng switch trong app.js của frontend).
const (
	msgEvent    = "event"    // một mục nhật ký hoạt động (eventDTO)
	msgStream   = "stream"   // một đoạn văn bản AI đang viết (streamDTO)
	msgSnapshot = "snapshot" // UISnapshot toàn phần
	msgDone     = "done"     // một lần run kết thúc
	msgAskUser  = "ask_user" // câu hỏi có cấu trúc chờ trả lời
	msgCoCreate = "cocreate" // cộng tác stream
	msgProgress = "progress" // tiến trình nhập / phỏng tác
	msgCommand  = "command"  // kết quả xuất / báo cáo chẩn đoán / lỗi lệnh
)

// eventDTO là phép chiếu host.Event hướng trình duyệt: timestamp chuyển thành mili giây, thời lượng chuyển thành mili giây,
// và mang cờ running tường minh, để frontend khỏi phải tính lại.
type eventDTO struct {
	ID         string `json:"id"`
	Time       int64  `json:"time"`       // unix mili giây
	FinishedAt int64  `json:"finishedAt"` // 0 = đang tiến hành
	Failed     bool   `json:"failed"`
	Running    bool   `json:"running"`
	Category   string `json:"category"`
	Agent      string `json:"agent"`
	Summary    string `json:"summary"`
	Level      string `json:"level"`
	Depth      int    `json:"depth"`
	DurationMs int64  `json:"durationMs"`
}

func newEventDTO(ev host.Event) eventDTO {
	dto := eventDTO{
		ID:         ev.ID,
		Time:       ev.Time.UnixMilli(),
		Failed:     ev.Failed,
		Running:    ev.Running(),
		Category:   ev.Category,
		Agent:      ev.Agent,
		Summary:    ev.Summary,
		Level:      ev.Level,
		Depth:      ev.Depth,
		DurationMs: ev.Duration.Milliseconds(),
	}
	if !ev.FinishedAt.IsZero() {
		dto.FinishedAt = ev.FinishedAt.UnixMilli()
	}
	if dto.Time == 0 {
		dto.Time = time.Now().UnixMilli()
	}
	return dto
}

// streamDTO là một phần tăng thêm của văn bản stream; Clear=true nghĩa là bắt đầu vòng mới (xóa vùng stream hiện tại).
type streamDTO struct {
	Text  string `json:"text"`
	Clear bool   `json:"clear"`
}

// askUserDTO là một tập câu hỏi chờ trả lời, id dùng để điền lại.
type askUserDTO struct {
	ID        string           `json:"id"`
	Questions []tools.Question `json:"questions"`
}

// ── Thân yêu cầu (trình duyệt → máy chủ) ──

type startRequest struct {
	Prompt string `json:"prompt"`
	// Chế độ "viết bám sát nhân vật có thật" (tùy chọn). Grounding bật thì Subject/SourceText được
	// chuẩn hóa thành hồ sơ mỏ neo sự thật trước khi sáng tác.
	Grounding  bool   `json:"grounding"`
	Subject    string `json:"subject"`
	SourceText string `json:"sourceText"`
}

// dossierDraftRequest yêu cầu AI soạn nháp hồ sơ nhân vật thật từ tên chủ thể.
type dossierDraftRequest struct {
	Subject string `json:"subject"`
}

type textRequest struct {
	Text string `json:"text"`
}

type answerRequest struct {
	ID      string            `json:"id"`
	Answers map[string]string `json:"answers"`
	Notes   map[string]string `json:"notes"`
}

type modelRequest struct {
	Role     string `json:"role"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type thinkingRequest struct {
	Role  string `json:"role"`
	Level string `json:"level"`
}

type exportRequest struct {
	Path      string `json:"path"`
	From      int    `json:"from"`
	To        int    `json:"to"`
	Overwrite bool   `json:"overwrite"`
}

type importRequest struct {
	Path string `json:"path"`
	From int    `json:"from"`
}

type cocreateOpenRequest struct {
	Stage   bool   `json:"stage"`
	Initial string `json:"initial"`
}

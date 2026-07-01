package host

import (
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// Event là sự kiện có cấu trúc mà TUI tiêu thụ.
//
// Với hai loại sự kiện gọi TOOL / DISPATCH, cùng một lần gọi dùng chung một ID cho phần bắt đầu và kết thúc:
// Khi bắt đầu, phát sự kiện với FinishedAt bằng zero (TUI hiển thị kiểu "đang tiến hành");
// Khi kết thúc, phát thêm một sự kiện cùng ID với FinishedAt + Duration (+ Failed),
// TUI định vị theo ID để cập nhật tại chỗ dòng gốc, tránh hiển thị thừa "bắt đầu một dòng, hoàn thành lại một dòng".
//
// Các sự kiện không thuộc loại gọi như SYSTEM / ERROR / CONTEXT có ID rỗng, mỗi sự kiện được thêm độc lập.
type Event struct {
	ID         string    // Dùng chung cho phần bắt đầu/kết thúc của cùng một lần gọi; sự kiện không gọi thì rỗng
	Time       time.Time // Thời điểm phát lần đầu (thời điểm bắt đầu)
	FinishedAt time.Time // Zero = đang tiến hành; khác zero = đã hoàn thành
	Failed     bool      // Đã hoàn thành nhưng thất bại (chỉ có nghĩa ở trạng thái hoàn thành)
	Category   string    // DISPATCH / TOOL / SYSTEM / REVIEW / CHECK / ERROR / CONTEXT
	Agent      string    // Agent phát sự kiện
	Summary    string
	Detail     string        // Nội dung đầy đủ, ghi log không cắt để tiện tra cứu; rỗng thì dùng Summary. UI chỉ đọc Summary
	Kind       string        // Phân loại lỗi (như stream_idle), xuất cùng log để lọc/cảnh báo; rỗng thì không xuất
	Level      string        // info / warn / error / success
	Depth      int           // 0 = tầng coordinator, 1 = tầng sub-agent
	Duration   time.Duration // Thời gian thực thi khi hoàn thành
}

// Running trả về sự kiện có đang tiến hành không.
// Chỉ sự kiện loại gọi (TOOL / DISPATCH có ID) mới có thể đang tiến hành; các loại khác luôn trả về false.
func (e Event) Running() bool {
	return e.ID != "" && e.FinishedAt.IsZero()
}

// UISnapshot là ảnh chụp trạng thái tổng hợp cần cho TUI để hiển thị.
type UISnapshot struct {
	Provider           string
	NovelName          string
	ModelName          string
	ModelContextWindow int // Cửa sổ ngữ cảnh của mô hình mặc định hiện tại (cập nhật thời gian thực khi /model thay đổi)
	ThinkingLevel      string
	Style              string
	RuntimeState       string // idle / running / pausing / paused / completed
	StatusLabel        string
	Phase              string
	Flow               string
	CurrentChapter     int
	TotalChapters      int
	CompletedCount     int
	TotalWordCount     int
	InProgressChapter  int
	PendingRewrites    []int
	RewriteReason      string
	PendingSteer       string
	RecoveryLabel      string
	IsRunning          bool
	Agents             []AgentSnapshot

	// Ngữ cảnh
	ContextTokens         int
	ContextWindow         int
	ContextPercent        float64
	ContextScope          string
	ContextStrategy       string
	ContextActiveMessages int
	ContextSummaryCount   int
	ContextCompactedCount int
	ContextKeptCount      int

	// Lượng dùng tích lũy (toàn phiên, bao gồm tất cả agent và lần chuyển mô hình)
	TotalInputTokens      int
	TotalOutputTokens     int
	TotalCacheReadTokens  int
	TotalCacheWriteTokens int
	TotalCostUSD          float64
	TotalSavedUSD         float64 // USD tiết kiệm được nhờ CacheRead (so với tính toàn bộ theo giá input không cache)
	BudgetLimitUSD        float64 // Giới hạn ngân sách (config budget.book_usd); 0 = chưa bật

	// Chẩn đoán cache
	OverallCacheCapable    bool // Ít nhất một role đã chạy mô hình hỗ trợ prompt cache (phân biệt "chưa bật" và "0% trúng cache")
	OverallRecentCacheRead int  // Tổng cacheRead của N lần gần nhất trong cửa sổ trượt
	OverallRecentInput     int  // Tổng input của N lần gần nhất trong cửa sổ trượt
	OverallRecentSamples   int  // Số mẫu trong cửa sổ trượt (≤ recentSampleCap)

	// MissingAssistantUsage > 0 thường có nghĩa là upstream streaming không gửi
	// final usage chunk theo giao thức OpenAI stream_options.include_usage (thường gặp ở proxy tự dựng),
	// khiến UsageTracker không nhận được bất kỳ dữ liệu tích lũy nào. UI theo đó
	// nhắc người dùng kiểm tra backend, tránh nhầm tưởng module cache hỏng.
	MissingAssistantUsage int

	// Cache theo chiều per-role, sắp xếp giảm dần theo CacheRead, đã lọc role không tiêu thụ token
	CachePerAgent []AgentCacheStat
	CachePerModel []AgentCacheStat

	// Thiết định cơ bản
	Premise          string
	Outline          []OutlineSnapshot
	Characters       []string
	SupportingCount  int      // Tổng số nhân vật phụ trong danh sách vai phụ
	RecentSupporting []string // Nhân vật phụ hoạt động gần đây (tối đa 5, sắp xếp giảm dần theo LastSeenChapter)
	Layered          bool
	CurrentVolumeArc string
	NextVolumeTitle  string
	CompassDirection string
	CompassScale     string

	// Chi tiết
	LastCommitSummary  string
	LastReviewSummary  string
	LastCheckpointName string
	RecentSummaries    []string
}

// OutlineSnapshot là tóm tắt hiển thị của một mục dàn ý.
type OutlineSnapshot struct {
	Chapter   int
	Title     string
	CoreEvent string
}

// AgentSnapshot là hình chiếu trạng thái Agent để hiển thị.
type AgentSnapshot struct {
	Name      string
	State     string
	TaskID    string
	TaskKind  string
	Summary   string
	Tool      string
	Turn      int
	Context   AgentContextSnapshot
	UpdatedAt time.Time
}

// AgentCacheStat là tích lũy trúng cache của một agent (chiếu sang cột trái).
// HitRate = CacheRead / Input; Input ở tầng litellm đã thống nhất nghĩa "bao gồm CacheRead".
//
// CacheCapable dùng để phân biệt hai kiểu 0% trúng cache:
//   - true  → Mô hình hỗ trợ prompt cache, 0% là do thiết kế prompt kém hoặc tiền tố không ổn định, cần tối ưu
//   - false → Mô hình/provider không hỗ trợ prompt cache, 0% là bình thường, không cần tra cứu
//
// Recent* là dữ liệu trúng cache của cửa sổ trượt (N lần gọi gần nhất), so với tích lũy có thể nhận ra "kéo lùi giai đoạn đầu" vs "trúng cache thấp ổn định".
type AgentCacheStat struct {
	Role            string
	Model           string
	Input           int
	Output          int
	CacheRead       int
	CacheWrite      int
	Cost            float64
	Saved           float64
	CacheCapable    bool
	RecentCacheRead int
	RecentInput     int
	RecentSamples   int
}

// AgentContextSnapshot là tình trạng sử dụng ngữ cảnh của Agent.
type AgentContextSnapshot struct {
	Tokens          int
	ContextWindow   int
	Percent         float64
	Scope           string
	Strategy        string
	ActiveMessages  int
	SummaryMessages int
	CompactedCount  int
	KeptCount       int
}

// CoCreateMessage là tin nhắn trong hội thoại đồng sáng tác.
type CoCreateMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CoCreateReply là phản hồi LLM của hội thoại đồng sáng tác. Raw giữ nguyên bốn đoạn đầy đủ của mô hình,
// dùng để ghi lại history cho vòng tiếp theo mô hình thấy được [DRAFT] của vòng trước, từ đó thực sự
// tích lũy cập nhật trên bản nháp đã có (chỉ dùng Message không có [DRAFT] sẽ khiến mô hình mỗi vòng
// tóm lại từ đầu dựa trên hội thoại).
// Suggestions là "những gì người dùng có thể muốn nói tiếp theo" do AI chủ động đưa ra, người dùng bí
// ý có thể nhấn phím số để điền vào ô nhập liệu.
type CoCreateReply struct {
	Message     string
	Prompt      string
	Ready       bool
	Suggestions []string
	Raw         string
}

// ReplayDeltaText trích xuất văn bản luồng có thể phát lại từ mục hàng đợi runtime.
func ReplayDeltaText(item domain.RuntimeQueueItem) string {
	if payload, ok := item.Payload.(map[string]any); ok {
		if text, ok := payload["delta"].(string); ok {
			return text
		}
	}
	return ""
}

// BuildStartPrompt gói yêu cầu của người dùng thành prompt khởi động cho Coordinator.
func BuildStartPrompt(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	return "Hãy bắt đầu sáng tác một tiểu thuyết dựa theo yêu cầu sáng tác dưới đây. Sau khi vào giai đoạn hoạch định, dòng đầu tiên của Premise bắt buộc xuất `# Tên sách`. Số lượng chương do bạn tự quyết định theo nhu cầu câu chuyện; nếu thể loại và xung đột tự nhiên phù hợp dạng dài kỳ, hãy ưu tiên lên kế hoạch theo cấu trúc phân tầng thay vì nén thành dạng tóm lược ngắn.\n\n[Yêu cầu sáng tác]\n" +
		prompt +
		"\n\nNếu một số chi tiết chưa rõ ràng, hãy tự bổ sung trong phạm vi không trái với định hướng của người dùng."
}

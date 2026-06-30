package domain

import "strings"

// Phase biểu thị giai đoạn sáng tác tiểu thuyết.
type Phase string

const (
	PhaseInit     Phase = "init"
	PhasePremise  Phase = "premise"
	PhaseOutline  Phase = "outline"
	PhaseWriting  Phase = "writing"
	PhaseComplete Phase = "complete"
)

// FlowState loại luồng đang hoạt động hiện tại, dùng để khôi phục checkpoint.
type FlowState string

const (
	FlowWriting   FlowState = "writing"
	FlowReviewing FlowState = "reviewing"
	FlowRewriting FlowState = "rewriting"
	FlowPolishing FlowState = "polishing"
	FlowSteering  FlowState = "steering"
)

// PlanningTier biểu thị cấp độ dài của hoạch định tác phẩm.
type PlanningTier string

const (
	PlanningTierShort PlanningTier = "short"
	PlanningTierMid   PlanningTier = "mid"
	PlanningTierLong  PlanningTier = "long"
)

// Progress theo dõi tiến độ, lưu bền vào meta/progress.json.
type Progress struct {
	NovelName         string      `json:"novel_name"`
	Phase             Phase       `json:"phase"`
	CurrentChapter    int         `json:"current_chapter"`
	TotalChapters     int         `json:"total_chapters"`
	CompletedChapters []int       `json:"completed_chapters"`
	TotalWordCount    int         `json:"total_word_count"`
	ChapterWordCounts map[int]int `json:"chapter_word_counts,omitempty"` // số chữ mỗi chương, hỗ trợ chỉnh tổng số chữ khi viết lại
	InProgressChapter int         `json:"in_progress_chapter,omitempty"` // chương đang viết (khôi phục cấp cảnh)
	CompletedScenes   []int       `json:"completed_scenes,omitempty"`    // số thứ tự cảnh đã hoàn thành của chương hiện tại
	Flow              FlowState   `json:"flow,omitempty"`                // luồng hiện tại
	PendingRewrites   []int       `json:"pending_rewrites,omitempty"`    // hàng đợi chương chờ viết lại
	RewriteReason     string      `json:"rewrite_reason,omitempty"`      // lý do viết lại
	StrandHistory     []string    `json:"strand_history,omitempty"`      // ghi dominant_strand theo thứ tự chương
	HookHistory       []string    `json:"hook_history,omitempty"`        // ghi hook_type theo thứ tự chương
	// Theo dõi phân tầng truyện dài (chỉ chế độ truyện dài dùng, truyện ngắn/vừa là giá trị 0)
	CurrentVolume int  `json:"current_volume,omitempty"`
	CurrentArc    int  `json:"current_arc,omitempty"`
	Layered       bool `json:"layered,omitempty"`
	// ReopenedFromComplete đánh dấu sách này đã được reopen từ trạng thái hoàn tất để vào làm lại. Làm lại
	// chỉ sửa chương đã có, không tăng giảm cấu trúc, nên sau khi xả hết hàng đợi thì theo "cấu trúc đủ là
	// hoàn tất lại" mà cho qua (tránh phục bút cuối tập cuối bị làm lại quấy nhiễu rồi kẹt ở
	// writing → vòng lặp chết viết tiếp vượt biên); viết xuôi không đặt cờ này, phán định hoàn tất giữ ngữ
	// nghĩa bảo thủ về thu tuyến.
	ReopenedFromComplete bool `json:"reopened_from_complete,omitempty"`
}

// IsResumable xét có thể khôi phục từ điểm dừng hay không.
func (p *Progress) IsResumable() bool {
	return p.Phase == PhaseWriting && p.CurrentChapter > 0
}

// NextChapter trả về số chương kế tiếp cần viết.
func (p *Progress) NextChapter() int {
	return p.LatestCompleted() + 1
}

// LatestCompleted trả về số chương đã hoàn thành lớn nhất; không có chương nào hoàn thành thì trả về 0.
func (p *Progress) LatestCompleted() int {
	max := 0
	for _, ch := range p.CompletedChapters {
		if ch > max {
			max = ch
		}
	}
	return max
}

// ExtractNovelNameFromPremise trích tên truyện từ dòng đầu premise `# Tên truyện` (có thể bọc «»).
// Mô hình đôi khi chép lại nguyên placeholder trong prompt thay vì sinh tên thật, các giá trị này coi như
// chưa trích được và trả về rỗng, để tầng trên xử lý dự phòng (UI hiển thị "Chưa đặt tên truyện"),
// tránh giao diện hiển thị thẳng hai chữ "Tên truyện".
func ExtractNovelNameFromPremise(premise string) string {
	for raw := range strings.SplitSeq(strings.ReplaceAll(premise, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "# ") {
			return ""
		}
		name := strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "# ")), "«»\"")
		switch name {
		case "Tên truyện", "Tên truyện thật", "Tên ví dụ":
			return "" // placeholder trong prompt, không phải tên truyện thật
		}
		return name
	}
	return ""
}

// ContextProfile chiến lược nạp ngữ cảnh, tự thích ứng theo tổng số chương.
type ContextProfile struct {
	SummaryWindow  int  // nạp tóm tắt N chương gần nhất
	TimelineWindow int  // nạp dòng thời gian N chương gần nhất
	Layered        bool // true = bật nạp tóm tắt phân tầng (tóm tắt tập + tóm tắt cung + tóm tắt chương)
}

// MemoryPolicy biểu thị chiến lược sử dụng bộ nhớ chia sẻ lúc chạy.
// Nó vừa dùng cho việc xuất ngữ cảnh, vừa dùng cho các quyết định handoff / reminder ở tầng host.
type MemoryPolicy struct {
	Mode                string `json:"mode,omitempty"`
	SummaryWindow       int    `json:"summary_window,omitempty"`
	TimelineWindow      int    `json:"timeline_window,omitempty"`
	LayeredSummaries    bool   `json:"layered_summaries,omitempty"`
	SummaryStrategy     string `json:"summary_strategy,omitempty"`
	WorkingRefresh      string `json:"working_refresh,omitempty"`
	EpisodicRefresh     string `json:"episodic_refresh,omitempty"`
	PlanningRefresh     string `json:"planning_refresh,omitempty"`
	FoundationRefresh   string `json:"foundation_refresh,omitempty"`
	PlanningFocus       string `json:"planning_focus,omitempty"`
	FoundationFocus     string `json:"foundation_focus,omitempty"`
	PreviousTailChars   int    `json:"previous_tail_chars,omitempty"`
	ChapterPlanEnabled  bool   `json:"chapter_plan_enabled,omitempty"`
	RelatedLookup       bool   `json:"related_chapter_lookup,omitempty"`
	CurrentOutlineBound bool   `json:"current_outline_bound,omitempty"`
	TotalChapters       int    `json:"total_chapters,omitempty"`
	HandoffPreferred    bool   `json:"handoff_preferred,omitempty"`
	ReadOnlyThreshold   int    `json:"read_only_threshold,omitempty"`
}

// NewContextProfile tính chiến lược ngữ cảnh theo tổng số chương.
func NewContextProfile(totalChapters int) ContextProfile {
	switch {
	case totalChapters <= 15:
		return ContextProfile{SummaryWindow: 10, TimelineWindow: 10}
	case totalChapters <= 50:
		return ContextProfile{SummaryWindow: 5, TimelineWindow: 8}
	default:
		return ContextProfile{SummaryWindow: 3, TimelineWindow: 5, Layered: true}
	}
}

// NewChapterMemoryPolicy sinh chiến lược bộ nhớ runtime cấp chương theo tiến độ và chiến lược ngữ cảnh.
func NewChapterMemoryPolicy(progress *Progress, profile ContextProfile, currentOutlineBound bool) MemoryPolicy {
	policy := MemoryPolicy{
		Mode:                "chapter",
		SummaryWindow:       profile.SummaryWindow,
		TimelineWindow:      profile.TimelineWindow,
		LayeredSummaries:    profile.Layered,
		WorkingRefresh:      "làm mới mỗi lần nạp theo chương",
		EpisodicRefresh:     "làm mới khi nộp chương, thẩm định và khi trạng thái truyện dài thay đổi",
		PreviousTailChars:   800,
		ChapterPlanEnabled:  true,
		CurrentOutlineBound: currentOutlineBound,
		ReadOnlyThreshold:   5,
	}
	if profile.Layered {
		policy.SummaryStrategy = "tóm tắt tập + tóm tắt cung + tóm tắt chương gần nhất"
	} else {
		policy.SummaryStrategy = "tóm tắt chương gần nhất"
	}
	if progress != nil {
		policy.TotalChapters = progress.TotalChapters
		if progress.TotalChapters > 30 {
			policy.RelatedLookup = true
		}
		if progress.Flow == FlowReviewing || progress.Flow == FlowRewriting || progress.Flow == FlowPolishing {
			policy.HandoffPreferred = true
		}
		if progress.Layered && len(progress.CompletedChapters) >= 6 {
			policy.HandoffPreferred = true
		}
		if len(progress.CompletedChapters) >= 12 {
			policy.HandoffPreferred = true
		}
		if progress.Layered && len(progress.CompletedChapters) >= 6 {
			policy.ReadOnlyThreshold = 4
		}
		if len(progress.CompletedChapters) >= 12 {
			policy.ReadOnlyThreshold = 4
		}
	}
	return policy
}

// NewArchitectMemoryPolicy trả về chiến lược bộ nhớ dùng cho giai đoạn hoạch định.
func NewArchitectMemoryPolicy() MemoryPolicy {
	return MemoryPolicy{
		Mode:               "architect",
		PlanningRefresh:    "làm mới khi cấu trúc tập-cung, la bàn hoặc tóm tắt cập nhật",
		FoundationRefresh:  "làm mới khi nhân vật, phục bút, thiết định thay đổi",
		PlanningFocus:      "dàn ý phân tầng, la bàn, tóm tắt tập",
		FoundationFocus:    "thiết định nhân vật, ảnh chụp nhân vật, sổ phục bút",
		HandoffPreferred:   true,
		ChapterPlanEnabled: false,
		ReadOnlyThreshold:  4,
	}
}

// RunMeta thông tin meta của lần chạy, lưu bền vào meta/run.json.
type RunMeta struct {
	StartedAt    string       `json:"started_at"`
	Provider     string       `json:"provider,omitempty"`
	Style        string       `json:"style"`
	Model        string       `json:"model"`
	PlanningTier PlanningTier `json:"planning_tier,omitempty"`
	SteerHistory []SteerEntry `json:"steer_history,omitempty"`
	PendingSteer string       `json:"pending_steer,omitempty"` // lệnh Steer chưa hoàn thành, tiêm lại khi khôi phục sau gián đoạn
}

// SteerEntry bản ghi can thiệp của người dùng.
type SteerEntry struct {
	Input     string `json:"input"`
	Timestamp string `json:"timestamp"`
}

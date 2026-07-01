package domain

// ChapterPlan ý tưởng viết chương, Writer tự chủ tạo ra.
// Không còn bắt buộc chia cảnh, Agent tự quyết định cách tổ chức nội dung.
type ChapterPlan struct {
	Chapter    int             `json:"chapter"`
	Title      string          `json:"title"`
	Goal       string          `json:"goal"`
	Conflict   string          `json:"conflict"`
	Hook       string          `json:"hook"`
	EmotionArc string          `json:"emotion_arc,omitempty"`
	Notes      string          `json:"notes,omitempty"` // ghi chú tự do của Agent
	Contract   ChapterContract `json:"contract,omitempty"`
}

// ChapterContract là hợp đồng nghiệm thu chương được Writer và Editor chia sẻ.
// Nó xác định các mục tiến triển bắt buộc hoàn thành trong chương này, các mục bị cấm vượt ranh giới và các điểm cần chú ý khi thẩm định.
type ChapterContract struct {
	RequiredBeats    []string `json:"required_beats,omitempty"`    // các mục tiến triển bắt buộc phải hoàn thành trong chương này
	ForbiddenMoves   []string `json:"forbidden_moves,omitempty"`   // các tiến triển rõ ràng không được xảy ra trong chương này
	ContinuityChecks []string `json:"continuity_checks,omitempty"` // các điểm nhất quán cần đặc biệt kiểm tra trong chương này
	EvaluationFocus  []string `json:"evaluation_focus,omitempty"`  // các điểm Editor cần kiểm tra trọng tâm
	EmotionTarget    string   `json:"emotion_target,omitempty"`    // tùy chọn: cảm xúc chủ yếu mà chương này muốn độc giả cảm nhận
	PayoffPoints     []string `json:"payoff_points,omitempty"`     // tùy chọn: các điểm tình tiết/điểm thực hiện mà chương quan trọng muốn hồi đáp
	HookGoal         string   `json:"hook_goal,omitempty"`         // tùy chọn: ham muốn theo dõi tiếp mà móc thu hút cuối chương muốn thúc đẩy
}

// ChapterSummary tóm tắt chương, dùng cho cửa sổ ngữ cảnh của các chương tiếp theo.
type ChapterSummary struct {
	Chapter    int      `json:"chapter"`
	Summary    string   `json:"summary"`
	Characters []string `json:"characters"`
	KeyEvents  []string `json:"key_events"`
}

// ArcSummary tóm tắt cấp cung, được Editor tạo ra khi cung kết thúc.
type ArcSummary struct {
	Volume    int      `json:"volume"`
	Arc       int      `json:"arc"`
	Title     string   `json:"title"`
	Summary   string   `json:"summary"`
	KeyEvents []string `json:"key_events"`
}

// VolumeSummary tóm tắt cấp tập, được tạo ra khi tập kết thúc.
type VolumeSummary struct {
	Volume    int      `json:"volume"`
	Title     string   `json:"title"`
	Summary   string   `json:"summary"`
	KeyEvents []string `json:"key_events"`
}

// CharacterSnapshot snapshot trạng thái nhân vật, được ghi lại tại ranh giới cung.
type CharacterSnapshot struct {
	Volume     int    `json:"volume"`
	Arc        int    `json:"arc"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Power      string `json:"power,omitempty"`
	Motivation string `json:"motivation"`
	Relations  string `json:"relations,omitempty"`
}

// OutlineFeedback phản hồi của Writer về dàn ý, tùy chọn khi giao chương.
type OutlineFeedback struct {
	Deviation  string `json:"deviation"`  // mô tả sự lệch hướng
	Suggestion string `json:"suggestion"` // đề xuất điều chỉnh
}

// WritingStyleRules các quy tắc viết được đúc kết từ các chương đã viết, được Editor tạo ra tại ranh giới cung.
// Thay thế các đoạn văn gốc (style_anchors / voice_samples), dùng quy tắc thay vì sao chép văn bản gốc.
type WritingStyleRules struct {
	Volume    int              `json:"volume"`
	Arc       int              `json:"arc"`
	Prose     []string         `json:"prose"`      // 3-5 quy tắc phong cách tường thuật, mỗi quy tắc ≤50 chữ
	Dialogue  []CharacterVoice `json:"dialogue"`   // quy tắc phong cách đối thoại nhân vật
	Taboos    []string         `json:"taboos"`     // danh sách điều cấm kỵ
	UpdatedAt string           `json:"updated_at"` // dấu thời gian ISO8601
}

// CharacterVoice quy tắc phong cách đối thoại của một nhân vật đơn lẻ.
type CharacterVoice struct {
	Name  string   `json:"name"`
	Rules []string `json:"rules"` // 2-3 quy tắc đặc trưng ngôn ngữ, mỗi quy tắc ≤30 chữ
}

// RelatedChapter chương liên quan được đề xuất đọc lại.
type RelatedChapter struct {
	Chapter int    `json:"chapter"`
	Reason  string `json:"reason"`
}

// RecallItem là thông tin dài hạn được thu hồi có chọn lọc theo nhiệm vụ hiện tại.
// Nó không thay thế các artifact chính thức, chỉ chịu trách nhiệm tái nạp một lượng nhỏ thông tin lịch sử thực sự liên quan vào model ở vòng hiện tại.
type RecallItem struct {
	Kind    string `json:"kind"`
	Key     string `json:"key,omitempty"`
	Chapter int    `json:"chapter,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Summary string `json:"summary,omitempty"`
}

// CommitResult là giá trị trả về có cấu trúc của công cụ commit_chapter.
// Chỉ chứa các trường sự thật; "bước tiếp theo làm gì" được kênh Reminder tự tạo ra dựa trên Progress hiện tại.
type CommitResult struct {
	Chapter        int              `json:"chapter"`
	Committed      bool             `json:"committed"`
	WordCount      int              `json:"word_count"`
	NextChapter    int              `json:"next_chapter"`
	ReviewRequired bool             `json:"review_required"`
	ReviewReason   string           `json:"review_reason,omitempty"`
	HookType       string           `json:"hook_type,omitempty"`
	DominantStrand string           `json:"dominant_strand,omitempty"`
	Feedback       *OutlineFeedback `json:"feedback,omitempty"`
	// tín hiệu phân lớp truyện dài
	ArcEnd         bool `json:"arc_end,omitempty"`
	VolumeEnd      bool `json:"volume_end,omitempty"`
	Volume         int  `json:"volume,omitempty"`
	Arc            int  `json:"arc,omitempty"`
	NeedsExpansion bool `json:"needs_expansion,omitempty"`  // cung tiếp theo là khung xương, cần mở rộng chương
	NeedsNewVolume bool `json:"needs_new_volume,omitempty"` // cần Architect tạo tập tiếp theo
	NextVolume     int  `json:"next_volume,omitempty"`      // số thứ tự cung/tập tiếp theo
	NextArc        int  `json:"next_arc,omitempty"`         // số thứ tự cung tiếp theo
	// sự thật trạng thái hoàn thành: sau lần commit này toàn bộ cuốn sách có hoàn thành không
	BookComplete bool `json:"book_complete,omitempty"`
	// snapshot Progress.Flow hiện tại (writing / reviewing / rewriting / polishing)
	Flow string `json:"flow,omitempty"`
}

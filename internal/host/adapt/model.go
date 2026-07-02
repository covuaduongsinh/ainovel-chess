package adapt

// Các struct dưới đây vừa là schema output LLM (bọc trong <output>), vừa là dạng
// lưu JSON trên đĩa. Quy ước song ngữ: *_prompt viết tiếng Anh (công cụ sinh
// ảnh/video); mô tả/nhãn (description, appearance...) viết tiếng Việt.

// ConceptResult — art direction toàn cục.
type ConceptResult struct {
	Style     ConceptStyle      `json:"style"`
	Locations []ConceptLocation `json:"locations"`
}

type ConceptStyle struct {
	Overall        string   `json:"overall"`         // phong cách tổng thể (VI)
	Palette        []string `json:"palette"`         // bảng màu chủ đạo
	Lighting       string   `json:"lighting"`        // ánh sáng (VI)
	CameraLanguage string   `json:"camera_language"` // ngôn ngữ máy quay (VI)
	StyleTokens    []string `json:"style_tokens"`    // token EN chèn vào mọi prompt
	References     []string `json:"references"`      // tham chiếu phong cách (tuỳ chọn)
}

type ConceptLocation struct {
	Name        string `json:"name"`         // tên địa điểm (VI)
	Description string `json:"description"`  // mô tả (VI)
	Mood        string `json:"mood"`         // không khí (VI)
	ImagePrompt string `json:"image_prompt"` // prompt sinh ảnh (EN)
}

// CharacterDesign — thiết kế trực quan một nhân vật.
type CharacterDesign struct {
	Name            string   `json:"name"`
	Appearance      string   `json:"appearance"`       // ngoại hình (VI)
	Wardrobe        string   `json:"wardrobe"`         // trang phục (VI)
	Palette         []string `json:"palette"`          // bảng màu nhân vật
	KeyArtPrompt    string   `json:"key_art_prompt"`   // prompt key-art (EN)
	TurnaroundPrompt string  `json:"turnaround_prompt"` // prompt turnaround (EN)
	NegativePrompt  string   `json:"negative_prompt"`  // negative prompt (EN)
}

// PropResult — bảng đạo cụ chủ chốt.
type PropResult struct {
	Props []PropDesign `json:"props"`
}

type PropDesign struct {
	Name           string `json:"name"`            // tên đạo cụ (VI)
	Description     string `json:"description"`     // mô tả (VI)
	Significance    string `json:"significance"`    // vai trò trong truyện (VI)
	ImagePrompt     string `json:"image_prompt"`    // prompt sinh ảnh (EN)
	NegativePrompt  string `json:"negative_prompt"` // negative prompt (EN)
}

// ConsistencyBible — khóa token trực quan chuẩn, tiêm vào mọi prompt hạ nguồn.
type ConsistencyBible struct {
	StyleTokens []string           `json:"style_tokens"` // token EN chung
	Characters  []ConsistencyToken `json:"characters"`
	Props       []ConsistencyToken `json:"props"`
	Locations   []ConsistencyToken `json:"locations"`
	Notes       string             `json:"notes"` // ghi chú nhất quán (VI)
}

type ConsistencyToken struct {
	Name            string `json:"name"`
	CanonicalPrompt string `json:"canonical_prompt"`   // mô tả cố định (EN)
	SeedHint        string `json:"seed_hint,omitempty"`
	Notes           string `json:"notes,omitempty"` // VI
}

// ScreenplayResult — kịch bản một chương (nội dung là Markdown).
type ScreenplayResult struct {
	Chapter  int    `json:"chapter"`
	Title    string `json:"title"`
	Markdown string `json:"markdown"`
}

// StoryboardResult — phân cảnh một chương.
type StoryboardResult struct {
	Chapter int               `json:"chapter"`
	Title   string            `json:"title"`
	Scenes  []StoryboardScene `json:"scenes"`
}

type StoryboardScene struct {
	SceneID string `json:"scene_id"`
	Heading string `json:"heading"` // NỘI./NGOẠI. – ĐỊA ĐIỂM – THỜI GIAN
	Summary string `json:"summary"` // tóm tắt cảnh (VI)
	Shots   []Shot `json:"shots"`
}

type Shot struct {
	ShotID         string `json:"shot_id"`
	Description    string `json:"description"`    // diễn biến shot (VI)
	CameraAngle    string `json:"camera_angle"`   // góc máy (VI/EN ngắn)
	Movement       string `json:"movement"`       // chuyển động máy (VI)
	DurationSec    int    `json:"duration_sec"`   // thời lượng ước tính
	ImagePrompt    string `json:"image_prompt"`   // prompt sinh ảnh (EN)
	VideoPrompt    string `json:"video_prompt"`   // prompt sinh video (EN)
	NegativePrompt string `json:"negative_prompt,omitempty"`
	Dialogue       string `json:"dialogue,omitempty"` // lời thoại (VI)
}

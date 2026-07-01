package domain

// Novel thông tin meta của tiểu thuyết.
type Novel struct {
	Name          string `json:"name"`
	TotalChapters int    `json:"total_chapters"`
}

// OutlineEntry mục dàn ý, tương ứng với một chương.
type OutlineEntry struct {
	Chapter   int      `json:"chapter"`
	Title     string   `json:"title"`
	CoreEvent string   `json:"core_event"`
	Hook      string   `json:"hook"`
	Scenes    []string `json:"scenes"`
}

// Character hồ sơ nhân vật.
type Character struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases,omitempty"` // bí danh/danh hiệu/biệt hiệu (ví dụ "cậu bé vô dụng", "anh Viêm")
	Role        string   `json:"role"`
	Description string   `json:"description"`
	Arc         string   `json:"arc"`
	Traits      []string `json:"traits"`
	Tier        string   `json:"tier,omitempty"` // core / important / secondary / decorative (mặc định important)
}

// VolumeOutline dàn ý cấp tập (chế độ phân lớp truyện dài).
type VolumeOutline struct {
	Index int          `json:"index"`
	Title string       `json:"title"`
	Theme string       `json:"theme"` // xung đột/chủ đề cốt lõi của tập này
	Arcs  []ArcOutline `json:"arcs"`
}

// IsExpanded kiểm tra tập đã được mở rộng chưa (có cấu trúc cung).
func (v *VolumeOutline) IsExpanded() bool { return len(v.Arcs) > 0 }

// StoryCompass la bàn hướng kết thúc, thay thế danh sách tập khung xương cố định.
// Architect có thể cập nhật ở mỗi ranh giới tập, cho phép hướng câu chuyện tiến hóa theo sáng tác.
type StoryCompass struct {
	EndingDirection string   `json:"ending_direction"`          // hướng kết thúc (mô tả theo chủ đề)
	OpenThreads     []string `json:"open_threads,omitempty"`    // mạch dài còn mở (cần thắt lại mới kết thúc được)
	EstimatedScale  string   `json:"estimated_scale,omitempty"` // quy mô ước tính mờ (ví dụ "dự kiến 4-6 tập")
	LastUpdated     int      `json:"last_updated,omitempty"`    // số chương đã hoàn thành khi cập nhật
}

// ArcOutline dàn ý cấp cung.
type ArcOutline struct {
	Index             int            `json:"index"` // số thứ tự cung trong tập
	Title             string         `json:"title"`
	Goal              string         `json:"goal"`                         // mục tiêu cung (mở - phát triển - chuyển - kết)
	EstimatedChapters int            `json:"estimated_chapters,omitempty"` // số chương ước tính của cung khung xương (xóa về 0 sau khi mở rộng)
	Chapters          []OutlineEntry `json:"chapters"`
}

// IsExpanded kiểm tra cung đã được mở rộng chưa (có chương chi tiết).
func (a *ArcOutline) IsExpanded() bool { return len(a.Chapters) > 0 }

// TotalChapters tính tổng số chương kế hoạch hiện tại của dàn ý phân lớp.
// Cung đã mở rộng tính theo số chương thực tế, cung khung xương tính theo EstimatedChapters.
// Progress.TotalChapters dùng nó để xác định chiến lược ngữ cảnh truyện dài; chương thực sự có thể viết vẫn đến từ FlattenOutline.
func TotalChapters(volumes []VolumeOutline) int {
	n := 0
	for _, v := range volumes {
		for _, a := range v.Arcs {
			if a.IsExpanded() {
				n += len(a.Chapters)
			} else {
				n += a.EstimatedChapters
			}
		}
	}
	return n
}

// FlattenOutline trải phẳng dàn ý phân lớp thành danh sách chương phẳng, giữ số chương toàn cục liên tục.
func FlattenOutline(volumes []VolumeOutline) []OutlineEntry {
	var result []OutlineEntry
	ch := 1
	for _, v := range volumes {
		for _, a := range v.Arcs {
			for _, e := range a.Chapters {
				e.Chapter = ch
				result = append(result, e)
				ch++
			}
		}
	}
	return result
}

// WorldRule mục quy tắc thế giới.
type WorldRule struct {
	Category string `json:"category"` // magic / technology / geography / society / other
	Rule     string `json:"rule"`     // mô tả quy tắc
	Boundary string `json:"boundary"` // ranh giới không được vi phạm
}

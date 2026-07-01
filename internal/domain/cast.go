package domain

// CastEntry là một bản ghi nhân vật phụ trong danh sách nhân vật phụ.
//
// Tách rời khỏi Character (characters.json, hồ sơ cốt lõi do Architect duy trì):
//   - CastEntry được công cụ commit_chapter tự động tích lũy, ghi lại "nhân vật phụ có tên đã xuất hiện"
//   - Character được Architect thiết kế rõ ràng, ghi lại cung nhân cách/đặc điểm/tier của nhân vật chính và nhân vật phụ quan trọng
//
// Khi trùng tên thì Character là chuẩn (nhân vật cốt lõi không vào cast_ledger), tránh trùng lặp.
type CastEntry struct {
	Name string `json:"name"`
	// Aliases hiện chưa có kênh ghi; dành trước cho công cụ "người dùng steer hợp nhất bí danh" trong tương lai
	// (ví dụ khai báo 'Lý chủ quán' và 'Lão Lý' là cùng một người). MergeAppearances đã hỗ trợ tra cứu bí danh.
	Aliases          []string `json:"aliases,omitempty"`
	BriefRole        string   `json:"brief_role,omitempty"` // định vị một câu (Writer điền lần đầu xuất hiện, có thể bổ sung sau; không bị ghi đè)
	FirstSeenChapter int      `json:"first_seen_chapter"`
	LastSeenChapter  int      `json:"last_seen_chapter"`
	// AppearanceCount dẫn xuất từ len(AppearanceChapters), giữ đồng bộ khi merge.
	// Giữ lại trường rõ ràng để UI/JSON đọc trực tiếp, không cần tính lại mỗi lần.
	AppearanceCount    int   `json:"appearance_count"`
	AppearanceChapters []int `json:"appearance_chapters"`
	// Promoted đánh dấu mục này đã được thăng cấp lên characters.json. RecentActive sẽ bỏ qua các mục này,
	// tránh thu hồi trùng lặp với hồ sơ cốt lõi. Kênh thăng cấp hiện chưa được triển khai, trường này là hook dành trước.
	Promoted bool `json:"promoted,omitempty"`
}

// CastIntro là khai báo giới thiệu của Writer về nhân vật mới xuất hiện khi commit_chapter.
// Chỉ được áp dụng khi tên đó xuất hiện lần đầu hoặc BriefRole trong ledger vẫn còn trống.
type CastIntro struct {
	Name      string `json:"name"`
	BriefRole string `json:"brief_role"`
}

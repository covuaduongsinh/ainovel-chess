package store

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// ProjectSummary là bản tóm tắt tĩnh của một dự án sách (một thư mục), đọc trực tiếp từ đĩa
// mà không cần khởi tạo host. Dùng cho màn chọn dự án của Web (và có thể tái dùng cho TUI).
type ProjectSummary struct {
	Dir         string    `json:"dir"`         // đường dẫn thư mục dự án
	Slug        string    `json:"slug"`        // tên thư mục (filepath.Base)
	Name        string    `json:"name"`        // tên sách hiển thị
	Phase       string    `json:"phase"`       // giai đoạn hiện tại (rỗng nếu chưa bắt đầu)
	Chapters    int       `json:"chapters"`    // số chương đã hoàn thành
	TotalChaps  int       `json:"totalChaps"`  // tổng số chương dự kiến
	Words       int       `json:"words"`       // tổng số chữ
	StartedAt   string    `json:"startedAt"`   // thời điểm chạy gần nhất (từ run.json, RFC3339)
	UpdatedAt   time.Time `json:"updatedAt"`   // mtime của meta/progress.json
	HasProgress bool      `json:"hasProgress"` // đã có tiến độ (đã bắt đầu viết) hay chưa
}

// List quét thư mục cha root và trả về tóm tắt mọi dự án sách bên trong.
// Mỗi thư mục con được coi là một dự án nếu có dấu hiệu store (thư mục meta/ hoặc premise.md).
// Sắp xếp giảm dần theo UpdatedAt (dự án sửa gần nhất lên đầu). root không tồn tại → danh sách rỗng.
func List(root string) ([]ProjectSummary, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []ProjectSummary
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if !looksLikeProject(dir) {
			continue
		}
		out = append(out, summarize(dir, e.Name()))
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

// looksLikeProject nhận diện thư mục dự án qua sự tồn tại của meta/ hoặc premise.md.
func looksLikeProject(dir string) bool {
	if fi, err := os.Stat(filepath.Join(dir, "meta")); err == nil && fi.IsDir() {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "premise.md")); err == nil {
		return true
	}
	return false
}

func summarize(dir, slug string) ProjectSummary {
	s := NewStore(dir)
	sum := ProjectSummary{Dir: dir, Slug: slug, Name: slug}

	if p, err := s.Progress.Load(); err == nil && p != nil {
		sum.HasProgress = true
		sum.Phase = string(p.Phase)
		sum.Chapters = len(p.CompletedChapters)
		sum.TotalChaps = p.TotalChapters
		sum.Words = p.TotalWordCount
		if name := strings.TrimSpace(p.NovelName); name != "" {
			sum.Name = name
		}
	}

	// Tên sách: NovelName → dòng đầu premise.md → tên thư mục.
	if sum.Name == slug {
		if premise, err := s.Outline.LoadPremise(); err == nil && premise != "" {
			if name := domain.ExtractNovelNameFromPremise(premise); name != "" {
				sum.Name = name
			}
		}
	}

	if meta, err := s.RunMeta.Load(); err == nil && meta != nil {
		sum.StartedAt = meta.StartedAt
	}

	// Thời điểm sửa cuối: mtime của progress.json, fallback về mtime thư mục.
	if fi, err := os.Stat(filepath.Join(dir, "meta", "progress.json")); err == nil {
		sum.UpdatedAt = fi.ModTime()
	} else if fi, err := os.Stat(dir); err == nil {
		sum.UpdatedAt = fi.ModTime()
	}

	return sum
}

// reservedNames là các tên thư mục bị cấm trên Windows (không phân biệt hoa thường).
var reservedNames = map[string]struct{}{
	"con": {}, "prn": {}, "aux": {}, "nul": {},
	"com1": {}, "com2": {}, "com3": {}, "com4": {}, "com5": {},
	"com6": {}, "com7": {}, "com8": {}, "com9": {},
	"lpt1": {}, "lpt2": {}, "lpt3": {}, "lpt4": {}, "lpt5": {},
	"lpt6": {}, "lpt7": {}, "lpt8": {}, "lpt9": {},
}

// Slugify chuyển tên sách do người dùng nhập thành tên thư mục an toàn (đặc biệt cho Windows):
// loại ký tự cấm \ / : * ? " < > | và ký tự điều khiển, gộp khoảng trắng thành gạch nối,
// cắt dấu chấm/khoảng trắng/gạch nối đầu-cuối, tránh tên thiết bị reserved. Giữ Unicode (tiếng Việt).
// Rỗng sau khi làm sạch → "novel".
func Slugify(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 0x20: // ký tự điều khiển
			continue
		case strings.ContainsRune(`\/:*?"<>|`, r):
			continue
		case r == ' ' || r == '\t':
			b.WriteRune('-')
		default:
			b.WriteRune(r)
		}
	}
	slug := b.String()
	// Gộp nhiều gạch nối liên tiếp.
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, " .-")
	if slug == "" {
		return "novel"
	}
	if _, bad := reservedNames[strings.ToLower(slug)]; bad {
		slug += "-novel"
	}
	return slug
}

// UniqueDir trả về đường dẫn thư mục con của root chưa tồn tại, dựa trên slug;
// khi trùng thì thêm hậu tố -2, -3… cho tới khi tìm được tên trống.
func UniqueDir(root, slug string) string {
	candidate := filepath.Join(root, slug)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}
	for i := 2; ; i++ {
		candidate = filepath.Join(root, slug+"-"+strconv.Itoa(i))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

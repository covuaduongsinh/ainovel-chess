package comic

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Estimate là số liệu ước tính chi phí, đếm từ bảng prompt đã sinh.
type Estimate struct {
	HasData       bool `json:"hasData"`  // đã chạy bước kịch bản chưa
	Chapters      int  `json:"chapters"` // số chương có dữ liệu trong phạm vi
	TotalPanels   int  `json:"totalPanels"`
	MissingPanels int  `json:"missingPanels"` // khung CHƯA có tệp ảnh — đây mới là phần phải trả tiền
}

// artFileExists dò mọi đuôi được chấp nhận — model khác nhau trả định dạng khác nhau
// (2.5 trả PNG, 3.1-flash-lite trả JPEG), chỉ dò .png sẽ đếm nhầm là còn thiếu.
func artFileExists(outDir, rel string) bool {
	base := strings.TrimSuffix(filepath.Join(outDir, filepath.FromSlash(rel)), filepath.Ext(rel))
	for _, ext := range artExts {
		if exists(base + ext) {
			return true
		}
	}
	return false
}

// EstimateCost đếm số khung thật trong phạm vi [from,to] từ prompts/chuong-*.json.
//
// Tách khỏi runCtx vì nó phải gọi được khi KHÔNG có lần chạy nào đang diễn ra (giao diện hỏi
// trước lúc bấm Chạy). Chỉ đọc đĩa, không gọi LLM, không gọi API ảnh.
//
// Điểm mấu chốt: trả về MissingPanels chứ không chỉ TotalPanels. Cơ chế bỏ-qua-nếu-đã-có
// khiến chạy lại gần như miễn phí, nên báo tổng số khung sẽ doạ người dùng bằng một con số
// mà họ không hề phải trả.
func EstimateCost(outDir string, from, to int) (Estimate, error) {
	var est Estimate
	promptDir := filepath.Join(outDir, "prompts")
	entries, err := os.ReadDir(promptDir)
	if err != nil {
		return est, nil // chưa chạy bước kịch bản — không phải lỗi
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	chapters := map[int]bool{}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(promptDir, name))
		if err != nil {
			continue
		}
		var rows []PanelPrompt
		if json.Unmarshal(data, &rows) != nil {
			continue
		}
		for _, r := range rows {
			if from > 0 && r.Chapter < from {
				continue
			}
			if to > 0 && r.Chapter > to {
				continue
			}
			est.HasData = true
			chapters[r.Chapter] = true
			est.TotalPanels++
			if !artFileExists(outDir, r.ArtFile) {
				est.MissingPanels++
			}
		}
	}
	est.Chapters = len(chapters)
	return est, nil
}

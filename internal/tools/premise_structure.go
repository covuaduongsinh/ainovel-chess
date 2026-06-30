package tools

import (
	"strings"

	"github.com/voocel/ainovel-cli/internal/domain"
)

var premiseHeadingAliases = map[string]string{
	"Định vị đề tài":             "Định vị đề tài",
	"Đề tài và tông điệu":        "Đề tài và tông điệu",
	"Xung đột cốt lõi":           "Xung đột cốt lõi",
	"Mục tiêu nhân vật chính":    "Mục tiêu nhân vật chính",
	"Hướng kết cục":              "Hướng kết cục",
	"Vùng cấm sáng tác":          "Vùng cấm sáng tác",
	"Điểm bán khác biệt":         "Điểm bán khác biệt",
	"Móc câu khác biệt":          "Móc câu khác biệt",
	"Cam kết cốt lõi":            "Cam kết cốt lõi",
	"Động cơ truyện":             "Động cơ truyện",
	"Tuyến quan hệ/trưởng thành": "Tuyến quan hệ/trưởng thành",
	"Lộ trình thăng cấp":         "Lộ trình thăng cấp",
	"Bước ngoặt giữa truyện":     "Bước ngoặt giữa truyện",
	"Mệnh đề kết cục":            "Mệnh đề kết cục",
	"Tính phù hợp truyện ngắn":   "Tính phù hợp truyện ngắn",
	"Tại sao tác phẩm phù hợp truyện ngắn/kết thúc trong một tập": "Tính phù hợp truyện ngắn",
}

func parsePremiseSections(premise string) map[string]string {
	lines := strings.Split(premise, "\n")
	sections := make(map[string]string)
	var current string
	var body []string

	flush := func() {
		if current == "" {
			return
		}
		text := strings.TrimSpace(strings.Join(body, "\n"))
		if text != "" {
			sections[current] = text
		}
		body = body[:0]
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if heading, ok := canonicalPremiseHeading(trimmed); ok {
			flush()
			current = heading
			continue
		}
		if current != "" {
			body = append(body, line)
		}
	}
	flush()
	return sections
}

func canonicalPremiseHeading(line string) (string, bool) {
	if !strings.HasPrefix(line, "#") {
		return "", false
	}
	title := strings.TrimSpace(strings.TrimLeft(line, "#"))
	if title == "" {
		return "", false
	}
	canonical, ok := premiseHeadingAliases[title]
	return canonical, ok
}

func premiseStructure(premise string, tier domain.PlanningTier) map[string]any {
	sections := parsePremiseSections(premise)
	required := requiredPremiseHeadings(tier)
	found := make([]string, 0, len(required))
	var missing []string
	for _, heading := range required {
		if _, ok := sections[heading]; ok {
			found = append(found, heading)
			continue
		}
		missing = append(missing, heading)
	}

	structure := map[string]any{
		"template_ready": len(missing) == 0,
		"found":          found,
		"missing":        missing,
	}
	if len(sections) > 0 {
		structure["section_count"] = len(sections)
	}
	return structure
}

func requiredPremiseHeadings(tier domain.PlanningTier) []string {
	common := []string{
		"Đề tài và tông điệu",
		"Định vị đề tài",
		"Xung đột cốt lõi",
		"Mục tiêu nhân vật chính",
		"Hướng kết cục",
		"Vùng cấm sáng tác",
		"Điểm bán khác biệt",
		"Móc câu khác biệt",
		"Cam kết cốt lõi",
	}

	switch tier {
	case domain.PlanningTierLong:
		return append(common,
			"Động cơ truyện",
			"Tuyến quan hệ/trưởng thành",
			"Lộ trình thăng cấp",
			"Bước ngoặt giữa truyện",
			"Mệnh đề kết cục",
		)
	case domain.PlanningTierMid:
		return append(common,
			"Động cơ truyện",
			"Bước ngoặt giữa truyện",
		)
	case domain.PlanningTierShort:
		return append(common,
			"Tính phù hợp truyện ngắn",
		)
	default:
		return common
	}
}

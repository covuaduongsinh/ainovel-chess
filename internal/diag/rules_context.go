package diag

import (
	"fmt"
	"strings"
)

// GhostCharacter phát hiện nhân vật core/important vắng mặt lâu dài.
func GhostCharacter(snap *Snapshot) []Finding {
	if snap.Progress == nil || len(snap.Characters) == 0 || len(snap.Summaries) == 0 {
		return nil
	}
	completed := snap.CompletedCount()
	if completed < 5 {
		return nil
	}

	// Tính số chương xuất hiện lần cuối của mỗi nhân vật
	lastSeen := make(map[string]int)
	for ch, s := range snap.Summaries {
		for _, name := range s.Characters {
			if ch > lastSeen[name] {
				lastSeen[name] = ch
			}
		}
	}

	threshold := completed / 3
	if threshold < 5 {
		threshold = 5
	}
	latest := snap.LatestCompleted()

	var ghosts []string
	for _, c := range snap.Characters {
		if c.Tier != "core" && c.Tier != "important" {
			continue
		}
		seen, ok := lastSeen[c.Name]
		if !ok {
			// Cũng kiểm tra bí danh
			for _, alias := range c.Aliases {
				if s, exists := lastSeen[alias]; exists && s > seen {
					seen = s
					ok = true
				}
			}
		}
		gap := latest - seen
		if !ok {
			ghosts = append(ghosts, fmt.Sprintf("%s(chưa từng xuất hiện trong tóm tắt)", c.Name))
		} else if gap > threshold {
			ghosts = append(ghosts, fmt.Sprintf("%s(xuất hiện lần cuối ch%d, đã vắng %d chương)", c.Name, seen, gap))
		}
	}
	if len(ghosts) == 0 {
		return nil
	}
	return []Finding{{
		Rule:       "GhostCharacter",
		Category:   CatContext,
		Severity:   SevInfo,
		Confidence: ConfMedium,
		AutoLevel:  AutoNone,
		Target:     "context.characters",
		Title:      fmt.Sprintf("Nhân vật biến mất: %d nhân vật cốt lõi vắng mặt lâu dài", len(ghosts)),
		Evidence:   strings.Join(ghosts, "; "),
		Suggestion: "Writer có thể đã mất theo dõi nhân vật này. Cân nhắc gửi lệnh can thiệp trực tiếp để tái giới thiệu nhân vật, hoặc hạ cấp tier của nhân vật trong characters.json.",
	}}
}

// TimelineGaps phát hiện các chương đã hoàn thành thiếu sự kiện timeline.
func TimelineGaps(snap *Snapshot) []Finding {
	if snap.Progress == nil || len(snap.Progress.CompletedChapters) == 0 {
		return nil
	}
	if len(snap.Timeline) == 0 && snap.CompletedCount() > 0 {
		return []Finding{{
			Rule:       "TimelineGaps",
			Category:   CatContext,
			Severity:   SevInfo,
			Confidence: ConfMedium,
			AutoLevel:  AutoNone,
			Target:     "context.timeline",
			Title:      "Timeline rỗng",
			Evidence:   fmt.Sprintf("completed=%d, timeline_events=0", snap.CompletedCount()),
			Suggestion: "Trích xuất timeline của commit_chapter có thể chưa hoạt động. Kiểm tra output của Writer có chứa trường timeline không.",
		}}
	}

	// Xây dựng ánh xạ chương → sự kiện
	chaptersWithEvents := make(map[int]bool)
	for _, e := range snap.Timeline {
		chaptersWithEvents[e.Chapter] = true
	}

	var missing []int
	for _, ch := range snap.Progress.CompletedChapters {
		if !chaptersWithEvents[ch] {
			missing = append(missing, ch)
		}
	}
	// Cho phép một số ít thiếu sót (một số chương chuyển tiếp có thể thực sự không có sự kiện trọng đại)
	if len(missing) == 0 || float64(len(missing))/float64(snap.CompletedCount()) < ThresholdTimelineGapRate {
		return nil
	}
	return []Finding{{
		Rule:       "TimelineGaps",
		Category:   CatContext,
		Severity:   SevInfo,
		Confidence: ConfMedium,
		AutoLevel:  AutoNone,
		Target:     "context.timeline",
		Title:      fmt.Sprintf("Khoảng trống timeline: %d chương không có sự kiện được ghi", len(missing)),
		Evidence:   fmt.Sprintf("missing=[%s]", intsToStr(missing)),
		Suggestion: "Trích xuất timeline của commit_chapter có thể bị lỗi một phần. Kiểm tra định dạng trường timeline trong output của Writer.",
	}}
}

// RelationshipStagnation phát hiện dữ liệu quan hệ ngừng cập nhật.
func RelationshipStagnation(snap *Snapshot) []Finding {
	if snap.Progress == nil || len(snap.Relationships) == 0 {
		return nil
	}
	completed := snap.CompletedCount()
	if completed < 6 {
		return nil
	}

	// Tìm chương mới nhất có dữ liệu quan hệ
	latestRelCh := 0
	for _, r := range snap.Relationships {
		if r.Chapter > latestRelCh {
			latestRelCh = r.Chapter
		}
	}

	// Nếu dữ liệu quan hệ mới nhất ở trong 1/3 đầu, đánh giá là đình trệ
	cutoff := snap.LatestCompleted() - completed/3
	if latestRelCh >= cutoff {
		return nil
	}
	return []Finding{{
		Rule:       "RelationshipStagnation",
		Category:   CatContext,
		Severity:   SevInfo,
		Confidence: ConfLow,
		AutoLevel:  AutoNone,
		Target:     "context.relationships",
		Title:      fmt.Sprintf("Dữ liệu quan hệ đình trệ: cập nhật mới nhất ở chương %d", latestRelCh),
		Evidence:   fmt.Sprintf("relationship_entries=%d, latest_update=ch%d, latest_completed=ch%d", len(snap.Relationships), latestRelCh, snap.LatestCompleted()),
		Suggestion: "Cập nhật quan hệ của commit_chapter có thể đã ngừng hoạt động, hoặc quan hệ câu chuyện thực sự không thay đổi. Kiểm tra trường relationships trong output của Writer.",
	}}
}

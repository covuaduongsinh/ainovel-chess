package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

// SaveReviewTool lưu kết quả thẩm định của Editor.
type SaveReviewTool struct {
	store *store.Store
}

func NewSaveReviewTool(store *store.Store) *SaveReviewTool {
	return &SaveReviewTool{store: store}
}

func (t *SaveReviewTool) Name() string { return "save_review" }
func (t *SaveReviewTool) Description() string {
	return "Lưu kết quả thẩm định và cập nhật trạng thái luồng. verdict là một trong accept/polish/rewrite. " +
		"Công cụ nội bộ thực thi cổng phân loại thẻ điểm (có thể nâng cấp verdict), trực tiếp cập nhật flow và pending_rewrites của Progress. " +
		"Trả về thực tế có cấu trúc: final_verdict / affected_chapters / escalation_reason / next_flow / next_chapter"
}
func (t *SaveReviewTool) Label() string { return "Lưu thẩm định" }

// Công cụ ghi (đồng thời cập nhật reviews/ và PendingRewrites/Flow của Progress), cấm song song.
func (t *SaveReviewTool) ReadOnly(_ json.RawMessage) bool        { return false }
func (t *SaveReviewTool) ConcurrencySafe(_ json.RawMessage) bool { return false }

func (t *SaveReviewTool) Schema() map[string]any {
	issueSchema := schema.Object(
		schema.Property("type", schema.Enum("chiều vấn đề", "consistency", "character", "pacing", "continuity", "foreshadow", "hook", "aesthetic")).Required(),
		schema.Property("severity", schema.Enum("mức độ nghiêm trọng", "critical", "error", "warning")).Required(),
		schema.Property("description", schema.String("mô tả vấn đề")).Required(),
		schema.Property("evidence", schema.String("bằng chứng: đoạn văn gốc, tình tiết cụ thể hoặc dữ liệu trạng thái")).Required(),
		schema.Property("suggestion", schema.String("đề xuất chỉnh sửa")),
	)
	dimensionSchema := schema.Object(
		schema.Property("dimension", schema.Enum("chiều", "consistency", "character", "pacing", "continuity", "foreshadow", "hook", "aesthetic")).Required(),
		schema.Property("score", schema.Int("điểm (0-100)")).Required(),
		schema.Property("verdict", schema.Enum("kết luận chiều (có thể bỏ qua: hệ thống tự suy theo score, ≥80 pass / ≥60 warning / <60 fail)", "pass", "warning", "fail")),
		schema.Property("comment", schema.String("kết luận ngắn gọn của chiều này; bắt buộc mỗi chiều, aesthetic phải dẫn văn gốc hoặc thực tế thống kê cụ thể")).Required(),
	)
	return schema.Object(
		schema.Property("chapter", schema.Int("số chương được thẩm định (thẩm định toàn cục điền số chương mới nhất)")).Required(),
		schema.Property("scope", schema.Enum("phạm vi thẩm định", "chapter", "global", "arc")).Required(),
		schema.Property("dimensions", schema.Array("điểm theo chiều (mỗi chiều một mục, bảy chiều)", dimensionSchema)).Required(),
		schema.Property("issues", schema.Array("các vấn đề phát hiện", issueSchema)).Required(),
		schema.Property("contract_status", schema.Enum("mức độ hoàn thành hợp đồng chương", "met", "partial", "missed")),
		schema.Property("contract_misses", schema.Array("các mục contract chưa hoàn thành hoặc vi phạm", schema.String(""))),
		schema.Property("contract_notes", schema.String("ghi chú ngắn về tình trạng thực hiện contract")),
		schema.Property("verdict", schema.Enum("kết luận thẩm định", "accept", "polish", "rewrite")).Required(),
		schema.Property("summary", schema.String("tóm tắt thẩm định")).Required(),
		schema.Property("affected_chapters", schema.Array("danh sách số chương cần viết lại hoặc đánh bóng (bắt buộc khi verdict là polish/rewrite)", schema.Int(""))),
	)
}

func (t *SaveReviewTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var r domain.ReviewEntry
	if err := json.Unmarshal(args, &r); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if r.Chapter <= 0 {
		return nil, fmt.Errorf("chapter must be > 0")
	}
	// verdict là hàm thuần túy của score (≥80 pass / ≥60 warning / <60 fail), suy ra xác định bởi code —
	// không để LLM cung cấp lại rồi kiểm tra nhất quán. Vừa loại bỏ dư thừa, vừa triệt tiêu tham số tự mâu thuẫn kiểu "score=85 nhưng cho warning".
	for i := range r.Dimensions {
		r.Dimensions[i].Verdict = expectedDimensionVerdict(r.Dimensions[i].Score)
	}
	if err := validateReviewEntry(r); err != nil {
		return nil, err
	}

	// Cổng phân loại thẻ điểm — nội tuyến logic nâng cấp từ policy/review.go
	finalVerdict := r.Verdict
	var escalationReason string

	if r.Verdict == "accept" {
		// Kiểm tra trạng thái hợp đồng
		if r.ContractStatus == "missed" {
			finalVerdict = "rewrite"
			escalationReason = "trạng thái thực hiện hợp đồng là missed, nâng cấp thành rewrite"
		} else if r.ContractStatus == "partial" {
			finalVerdict = "polish"
			escalationReason = "trạng thái thực hiện hợp đồng là partial, nâng cấp thành polish"
		}
		// Cong kiem soat the diem
		if finalVerdict == "accept" {
			if gate := evaluateScorecardGate(r.Dimensions); gate != "" {
				if strings.Contains(gate, "rewrite") {
					finalVerdict = "rewrite"
				} else {
					finalVerdict = "polish"
				}
				escalationReason = gate
			}
		}
	}

	affected := r.AffectedChapters
	if finalVerdict == "rewrite" || finalVerdict == "polish" {
		if len(affected) == 0 && r.Chapter > 0 {
			affected = []int{r.Chapter}
		}
		if err := t.store.Progress.ValidatePendingRewrites(affected); err != nil {
			return nil, fmt.Errorf("validate pending rewrites: %w", err)
		}
	}

	if err := t.store.World.SaveReview(r); err != nil {
		return nil, fmt.Errorf("save review: %w", err)
	}

	// Cập nhật Progress theo final verdict.
	// Ghi thất bại phải trả về sớm — tiếp theo sẽ append review checkpoint, nếu nuốt err ở đây
	// Coordinator sẽ thấy saved:true nhưng Store vẫn ở trạng thái trung gian thiếu Flow cũ / PendingRewrites.
	progress, _ := t.store.Progress.Load()
	if finalVerdict == "rewrite" || finalVerdict == "polish" {
		flow := domain.FlowRewriting
		if finalVerdict == "polish" {
			flow = domain.FlowPolishing
		}
		if err := t.store.Progress.SetPendingRewrites(affected, r.Summary); err != nil {
			return nil, fmt.Errorf("set pending rewrites: %w", err)
		}
		if err := t.store.Progress.SetFlow(flow); err != nil {
			return nil, fmt.Errorf("set flow %s: %w", flow, err)
		}
	} else {
		if err := t.store.Progress.SetFlow(domain.FlowWriting); err != nil {
			return nil, fmt.Errorf("set flow writing: %w", err)
		}
	}

	// Đọc ảnh chụp Progress đã cập nhật làm thực tế
	latest, _ := t.store.Progress.Load()
	nextFlow := string(domain.FlowWriting)
	nextChapter := 0
	if latest != nil {
		nextFlow = string(latest.Flow)
		nextChapter = latest.NextChapter()
	}

	// Thêm checkpoint
	scope := domain.ChapterScope(r.Chapter)
	if r.Scope == "arc" {
		vol, arc := 0, 0
		if progress != nil {
			vol, arc = progress.CurrentVolume, progress.CurrentArc
		}
		scope = domain.ArcScope(vol, arc)
	}
	artifact := fmt.Sprintf("reviews/%02d.json", r.Chapter)
	if r.Scope == "global" {
		artifact = fmt.Sprintf("reviews/%02d-global.json", r.Chapter)
	}
	if _, err := t.store.Checkpoints.AppendArtifact(scope, "review", artifact); err != nil {
		return nil, fmt.Errorf("checkpoint review: %w", err)
	}

	return json.Marshal(map[string]any{
		"saved":             true,
		"chapter":           r.Chapter,
		"scope":             r.Scope,
		"verdict":           r.Verdict,
		"final_verdict":     finalVerdict,
		"escalation_reason": escalationReason,
		"affected_chapters": affected,
		"issues":            len(r.Issues),
		"next_flow":         nextFlow,
		"next_chapter":      nextChapter,
	})
}

var expectedReviewDimensions = map[string]struct{}{
	"consistency": {},
	"character":   {},
	"pacing":      {},
	"continuity":  {},
	"foreshadow":  {},
	"hook":        {},
	"aesthetic":   {},
}

func validateReviewEntry(r domain.ReviewEntry) error {
	if strings.TrimSpace(r.Scope) == "" {
		return fmt.Errorf("scope is required")
	}
	if strings.TrimSpace(r.Summary) == "" {
		return fmt.Errorf("summary is required")
	}
	for _, issue := range r.Issues {
		if strings.TrimSpace(issue.Description) == "" {
			return fmt.Errorf("issue description is required")
		}
		if strings.TrimSpace(issue.Evidence) == "" {
			return fmt.Errorf("issue evidence is required")
		}
	}
	if err := validateDimensions(r.Dimensions); err != nil {
		return err
	}
	if (r.Verdict == "rewrite" || r.Verdict == "polish") && len(r.AffectedChapters) == 0 {
		return fmt.Errorf("affected_chapters is required when verdict=%s", r.Verdict)
	}
	return nil
}

func validateDimensions(dimensions []domain.DimensionScore) error {
	if len(dimensions) != len(expectedReviewDimensions) {
		return fmt.Errorf("dimensions must contain exactly %d entries", len(expectedReviewDimensions))
	}

	seen := make(map[string]struct{}, len(dimensions))
	for _, dim := range dimensions {
		if _, ok := expectedReviewDimensions[dim.Dimension]; !ok {
			return fmt.Errorf("unknown dimension: %s", dim.Dimension)
		}
		if _, ok := seen[dim.Dimension]; ok {
			return fmt.Errorf("duplicate dimension: %s", dim.Dimension)
		}
		seen[dim.Dimension] = struct{}{}
		if dim.Score < 0 || dim.Score > 100 {
			return fmt.Errorf("invalid score for %s: %d", dim.Dimension, dim.Score)
		}
		if strings.TrimSpace(dim.Comment) == "" {
			return fmt.Errorf("dimension comment is required: %s", dim.Dimension)
		}
	}
	return nil
}

func expectedDimensionVerdict(score int) string {
	switch {
	case score >= 80:
		return "pass"
	case score >= 60:
		return "warning"
	default:
		return "fail"
	}
}

// criticalDimensions định nghĩa các chiều quan trọng sẽ kích hoạt nâng cấp verdict.
var criticalDimensions = map[string]struct{}{
	"consistency": {},
	"character":   {},
	"continuity":  {},
}

// evaluateScorecardGate kiểm tra xem thẻ điểm có cần nâng cấp verdict không.
// Trả về chuỗi rỗng nghĩa là không nâng cấp.
func evaluateScorecardGate(dimensions []domain.DimensionScore) string {
	var criticalFails []string
	var polishIssues []string

	for _, dim := range dimensions {
		_, isCritical := criticalDimensions[dim.Dimension]
		if isCritical && (dim.Verdict == "fail" || dim.Score < 60) {
			criticalFails = append(criticalFails, fmt.Sprintf("%s(%d)", dim.Dimension, dim.Score))
		} else if dim.Verdict == "warning" || (isCritical && dim.Score < 80) {
			polishIssues = append(polishIssues, fmt.Sprintf("%s(%d)", dim.Dimension, dim.Score))
		}
	}

	if len(criticalFails) > 0 {
		return fmt.Sprintf("rewrite: chiều quan trọng không đạt %v", criticalFails)
	}
	if len(polishIssues) > 0 {
		return fmt.Sprintf("polish: một số chiều cần đánh bóng %v", polishIssues)
	}
	return ""
}

package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/rules"
	"github.com/voocel/ainovel-cli/internal/store"
)

// CommitChapterTool nộp chương: tải nội dung → lưu bản cuối → tạo tóm tắt → cập nhật trạng thái → cập nhật tiến độ.
type CommitChapterTool struct {
	store *store.Store
}

func NewCommitChapterTool(store *store.Store) *CommitChapterTool {
	return &CommitChapterTool{store: store}
}

// commitOutput nhúng trường mở rộng trên domain.CommitResult, giữ domain package không phụ thuộc rules.
// Vì trường nhúng được JSON marshaler thăng cấp (promoted), kết quả tuần tự hóa tương đương cấu trúc phẳng.
type commitOutput struct {
	domain.CommitResult
	RuleViolations []rules.Violation `json:"rule_violations,omitempty"`
}

func (t *CommitChapterTool) Name() string { return "commit_chapter" }
func (t *CommitChapterTool) Description() string {
	return "Nộp bản cuối chương. Tải nội dung bản nháp lưu thành bản cuối, cập nhật dòng thời gian, phục bút, quan hệ, trạng thái nhân vật và tiến độ." +
		"Trả về sự kiện có cấu trúc: next_chapter / review_required / arc_end / volume_end / needs_expansion / book_complete / flow v.v."
}
func (t *CommitChapterTool) Label() string { return "Nộp chương" }

// Công cụ ghi (thao tác nguyên tử xuyên miền: bản nháp→bản cuối→tóm tắt→tiến độ→checkpoint), cấm song song.
func (t *CommitChapterTool) ReadOnly(_ json.RawMessage) bool        { return false }
func (t *CommitChapterTool) ConcurrencySafe(_ json.RawMessage) bool { return false }

func (t *CommitChapterTool) Schema() map[string]any {
	timelineSchema := schema.Object(
		schema.Property("time", schema.String("thời gian trong câu chuyện")).Required(),
		schema.Property("event", schema.String("mô tả sự kiện")).Required(),
		schema.Property("characters", schema.Array("nhân vật liên quan", schema.String(""))),
	)
	foreshadowSchema := schema.Object(
		schema.Property("id", schema.String("ID phục bút")).Required(),
		schema.Property("action", schema.Enum("thao tác", "plant", "advance", "resolve")).Required(),
		schema.Property("description", schema.String("mô tả phục bút (chỉ bắt buộc khi plant)")),
	)
	relationshipSchema := schema.Object(
		schema.Property("character_a", schema.String("nhân vật A")).Required(),
		schema.Property("character_b", schema.String("nhân vật B")).Required(),
		schema.Property("relation", schema.String("mô tả quan hệ hiện tại")).Required(),
	)
	stateChangeSchema := schema.Object(
		schema.Property("entity", schema.String("tên nhân vật hoặc thực thể")).Required(),
		schema.Property("field", schema.String("thuộc tính thay đổi")).Required(),
		schema.Property("old_value", schema.String("giá trị trước khi thay đổi")),
		schema.Property("new_value", schema.String("giá trị sau khi thay đổi")).Required(),
		schema.Property("reason", schema.String("lý do thay đổi")),
	)
	feedbackSchema := schema.Object(
		schema.Property("deviation", schema.String("mô tả lệch khỏi dàn ý")).Required(),
		schema.Property("suggestion", schema.String("đề xuất điều chỉnh dàn ý tiếp theo")).Required(),
	)
	feedbackSchema["description"] = "đối tượng đề xuất cho dàn ý tiếp theo; phải truyền trực tiếp JSON object, không truyền JSON dạng chuỗi"
	return schema.Object(
		schema.Property("chapter", schema.Int("số chương")).Required(),
		schema.Property("summary", schema.String("tóm tắt nội dung chương này (trong 200 chữ)")).Required(),
		schema.Property("characters", schema.Array("tên nhân vật xuất hiện trong chương này", schema.String(""))).Required(),
		schema.Property("key_events", schema.Array("sự kiện quan trọng trong chương này", schema.String(""))).Required(),
		schema.Property("timeline_events", schema.Array("sự kiện dòng thời gian trong chương này", timelineSchema)),
		schema.Property("foreshadow_updates", schema.Array("thao tác phục bút", foreshadowSchema)),
		schema.Property("relationship_changes", schema.Array("thay đổi quan hệ", relationshipSchema)),
		schema.Property("state_changes", schema.Array("thay đổi trạng thái nhân vật/thực thể", stateChangeSchema)),
		schema.Property("cast_intros", schema.Array("giới thiệu nhân vật phụ lần đầu xuất hiện trong chương này và có thể xuất hiện lại sau (không gồm nhân vật chính và nhân vật đã có trong characters.json)", schema.Object(
			schema.Property("name", schema.String("tên nhân vật")).Required(),
			schema.Property("brief_role", schema.String("định vị một câu (ví dụ: chủ quán trọ/tay đấm sòng bạc)")).Required(),
		))),
		schema.Property("hook_type", schema.Enum("loại móc cuối chương", "crisis", "mystery", "desire", "emotion", "choice")),
		schema.Property("dominant_strand", schema.Enum("tuyến tường thuật chủ đạo chương này", "quest", "fire", "constellation")),
		schema.Property("feedback", feedbackSchema),
	)
}

func (t *CommitChapterTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Chapter             int                        `json:"chapter"`
		Summary             string                     `json:"summary"`
		Characters          []string                   `json:"characters"`
		KeyEvents           []string                   `json:"key_events"`
		TimelineEvents      []domain.TimelineEvent     `json:"timeline_events"`
		ForeshadowUpdates   []domain.ForeshadowUpdate  `json:"foreshadow_updates"`
		RelationshipChanges []domain.RelationshipEntry `json:"relationship_changes"`
		StateChanges        []domain.StateChange       `json:"state_changes"`
		CastIntros          []domain.CastIntro         `json:"cast_intros"`
		HookType            string                     `json:"hook_type"`
		DominantStrand      string                     `json:"dominant_strand"`
		Feedback            *domain.OutlineFeedback    `json:"feedback"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w: %w", errs.ErrToolArgs, err)
	}
	if a.Chapter <= 0 {
		return nil, fmt.Errorf("chapter must be > 0: %w", errs.ErrToolArgs)
	}
	if t.store.Progress.IsChapterCompleted(a.Chapter) {
		// dọn dẹp PendingCommit có thể còn sót lại (sự cố xảy ra sau ProgressMarked, trước ClearPendingCommit)
		if pending, _ := t.store.Signals.LoadPendingCommit(); pending != nil && pending.Chapter == a.Chapter {
			if err := t.appendCommitCheckpoint(a.Chapter); err != nil {
				return nil, fmt.Errorf("checkpoint commit: %w: %w", errs.ErrStoreWrite, err)
			}
			_ = t.store.Signals.ClearPendingCommit()
		}
		// đường dẫn trau chuốt/viết lại: chương tuy đã hoàn thành nhưng vẫn trong pending_rewrites, cho phép ghi đè và drain hàng đợi
		progress, _ := t.store.Progress.Load()
		if progress != nil && slices.Contains(progress.PendingRewrites, a.Chapter) {
			return t.executeRewriteCommit(a.Chapter, a.Summary, a.Characters, a.KeyEvents,
				a.HookType, a.DominantStrand, progress)
		}
		return t.buildSkipResult(a.Chapter, progress)
	}
	existingPending, err := t.store.Signals.LoadPendingCommit()
	if err != nil {
		return nil, fmt.Errorf("load pending commit: %w: %w", errs.ErrStoreRead, err)
	}
	if existingPending != nil && existingPending.Chapter != a.Chapter {
		return nil, fmt.Errorf("tồn tại nộp chương chưa được khôi phục: chương %d (giai đoạn %s), vui lòng khôi phục hoặc nộp lại chương đó trước: %w", existingPending.Chapter, existingPending.Stage, errs.ErrToolConflict)
	}
	if err := t.store.Progress.ValidateChapterWork(a.Chapter); err != nil {
		// xung đột hàng đợi giữ nguyên (đã mang phân loại ErrToolConflict); lỗi IO khác quy về Precondition.
		if errors.Is(err, errs.ErrToolConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("chương hiện tại không được phép nộp: %w: %w", errs.ErrToolPrecondition, err)
	}

	// chặn vượt biên chế độ phân tầng: phải đứng trước mọi thao tác ghi, nếu không commit vượt biên sẽ làm hỏng
	// file chương, tóm tắt và Progress. boundary tái sử dụng cho bước 6b bên dưới để tính tín hiệu cung/tập.
	var boundary *store.ArcBoundary
	if progress, perr := t.store.Progress.Load(); perr == nil && progress != nil && progress.Layered {
		b, bErr := t.store.Outline.CheckArcBoundary(a.Chapter)
		if bErr != nil {
			return nil, fmt.Errorf("phát hiện biên cung thất bại chapter=%d: %w: %w", a.Chapter, errs.ErrStoreRead, bErr)
		}
		if b == nil {
			return nil, fmt.Errorf(
				"Chương %d không nằm trong phạm vi dàn ý phân tầng: viết lách phải dùng expand_arc mở rộng cung hoặc append_volume bổ sung tập trước; nếu toàn bộ sách đã hoàn thành hãy gọi save_foundation type=complete_book: %w",
				a.Chapter, errs.ErrToolPrecondition)
		}
		boundary = b
	}

	// 1. tải nội dung chương
	content, wordCount, err := t.store.Drafts.LoadChapterContent(a.Chapter)
	if err != nil {
		return nil, fmt.Errorf("load chapter content: %w: %w", errs.ErrStoreRead, err)
	}
	if content == "" {
		return nil, fmt.Errorf("no content found for chapter %d: %w", a.Chapter, errs.ErrToolPrecondition)
	}

	now := time.Now().Format(time.RFC3339)
	pending := domain.PendingCommit{
		Chapter:        a.Chapter,
		Stage:          domain.CommitStageStarted,
		Summary:        a.Summary,
		HookType:       a.HookType,
		DominantStrand: a.DominantStrand,
		StartedAt:      now,
		UpdatedAt:      now,
	}
	if err := t.store.Signals.SavePendingCommit(pending); err != nil {
		return nil, fmt.Errorf("save pending commit: %w: %w", errs.ErrStoreWrite, err)
	}

	// 2. lưu bản cuối
	if err := t.store.Drafts.SaveFinalChapter(a.Chapter, content); err != nil {
		return nil, fmt.Errorf("save final chapter: %w: %w", errs.ErrStoreWrite, err)
	}

	// 3. lưu tóm tắt
	summary := domain.ChapterSummary{
		Chapter:    a.Chapter,
		Summary:    a.Summary,
		Characters: a.Characters,
		KeyEvents:  a.KeyEvents,
	}
	if err := t.store.Summaries.SaveSummary(summary); err != nil {
		return nil, fmt.Errorf("save summary: %w: %w", errs.ErrStoreWrite, err)
	}

	// 4. cập nhật gia số trạng thái
	if len(a.TimelineEvents) > 0 {
		for i := range a.TimelineEvents {
			a.TimelineEvents[i].Chapter = a.Chapter
		}
		if err := t.store.World.AppendTimelineEvents(a.TimelineEvents); err != nil {
			return nil, fmt.Errorf("append timeline: %w: %w", errs.ErrStoreWrite, err)
		}
	}
	if len(a.ForeshadowUpdates) > 0 {
		if err := t.store.World.UpdateForeshadow(a.Chapter, a.ForeshadowUpdates); err != nil {
			return nil, fmt.Errorf("update foreshadow: %w: %w", errs.ErrStoreWrite, err)
		}
	}
	if len(a.RelationshipChanges) > 0 {
		for i := range a.RelationshipChanges {
			a.RelationshipChanges[i].Chapter = a.Chapter
		}
		if err := t.store.World.UpdateRelationships(a.RelationshipChanges); err != nil {
			return nil, fmt.Errorf("update relationships: %w: %w", errs.ErrStoreWrite, err)
		}
	}
	if len(a.StateChanges) > 0 {
		for i := range a.StateChanges {
			a.StateChanges[i].Chapter = a.Chapter
		}
		if err := t.store.World.AppendStateChanges(a.StateChanges); err != nil {
			return nil, fmt.Errorf("append state changes: %w: %w", errs.ErrStoreWrite, err)
		}
	}

	// 4b. cộng dồn danh sách diễn viên phụ: nhân vật không cốt lõi xuất hiện chương này vào cast_ledger, để novel_context gợi nhớ.
	// Khi thất bại chỉ warn không chặn commit — danh sách là dữ liệu thứ yếu, có thể tự phục hồi qua commit chương tiếp theo.
	if len(a.Characters) > 0 {
		coreNames := loadCoreCharacterNameSet(t.store)
		if err := t.store.Cast.MergeAppearances(a.Chapter, a.Characters, a.CastIntros, coreNames); err != nil {
			slog.Warn("cộng dồn danh sách diễn viên phụ thất bại, bỏ qua", "module", "commit", "chapter", a.Chapter, "err", err)
		}
	}

	pending.Stage = domain.CommitStageStateApplied
	pending.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := t.store.Signals.SavePendingCommit(pending); err != nil {
		return nil, fmt.Errorf("update pending commit stage: %w: %w", errs.ErrStoreWrite, err)
	}

	// 5. cập nhật tiến độ
	if err := t.store.Progress.MarkChapterComplete(a.Chapter, wordCount, a.HookType, a.DominantStrand); err != nil {
		return nil, fmt.Errorf("mark chapter complete: %w: %w", errs.ErrStoreWrite, err)
	}

	// 6. xác định có cần thẩm định không
	progress, err := t.store.Progress.Load()
	if err != nil {
		return nil, fmt.Errorf("load progress: %w: %w", errs.ErrStoreRead, err)
	}
	completedCount := 0
	if progress != nil {
		completedCount = len(progress.CompletedChapters)
	}

	// 6b. tín hiệu cung/tập chế độ trường thiên: boundary đã kiểm tra trước ở đầu vào, khi Layered đảm bảo khác nil
	var arcEnd, volumeEnd, needsExpansion, needsNewVolume bool
	var vol, arc, nextVol, nextArc int
	if progress != nil && progress.Layered && boundary != nil {
		arcEnd = boundary.IsArcEnd
		volumeEnd = boundary.IsVolumeEnd
		vol = boundary.Volume
		arc = boundary.Arc
		needsExpansion = boundary.NeedsExpansion
		needsNewVolume = boundary.NeedsNewVolume
		nextVol = boundary.NextVolume
		nextArc = boundary.NextArc
		_ = t.store.Progress.UpdateVolumeArc(vol, arc)
	}

	var reviewRequired bool
	var reviewReason string
	if progress != nil && progress.Layered {
		reviewRequired, reviewReason = domain.ShouldArcReview(arcEnd, volumeEnd, vol, arc)
	} else {
		reviewRequired, reviewReason = domain.ShouldReview(completedCount)
	}

	// 7. xây dựng tín hiệu có cấu trúc
	result := domain.CommitResult{
		Chapter:        a.Chapter,
		Committed:      true,
		WordCount:      wordCount,
		NextChapter:    a.Chapter + 1,
		ReviewRequired: reviewRequired,
		ReviewReason:   reviewReason,
		HookType:       a.HookType,
		DominantStrand: a.DominantStrand,
		Feedback:       a.Feedback,
		ArcEnd:         arcEnd,
		VolumeEnd:      volumeEnd,
		Volume:         vol,
		Arc:            arc,
		NeedsExpansion: needsExpansion,
		NeedsNewVolume: needsNewVolume,
		NextVolume:     nextVol,
		NextArc:        nextArc,
	}

	// 8. xác định trạng thái hoàn thành: không phân tầng viết xong chương cuối / phân tầng chương cuối tập cuối → MarkComplete
	if t.applyCompletion(&result, progress) {
		result.BookComplete = true
	}
	if p, _ := t.store.Progress.Load(); p != nil {
		result.Flow = string(p.Flow)
	}

	pending.Stage = domain.CommitStageProgressMarked
	pending.Result = &result
	pending.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := t.store.Signals.SavePendingCommit(pending); err != nil {
		return nil, fmt.Errorf("update pending commit result: %w: %w", errs.ErrStoreWrite, err)
	}

	// 9. bổ sung checkpoint. Phải đứng trước xóa pending_commit, đảm bảo
	// pending_commit hiển thị sau khởi động lại luôn có thể chạy lại để bù checkpoint còn thiếu.
	if err := t.appendCommitCheckpoint(a.Chapter); err != nil {
		return nil, fmt.Errorf("checkpoint commit: %w: %w", errs.ErrStoreWrite, err)
	}

	// 10. xóa trạng thái trung gian tiến độ
	if err := t.store.Progress.ClearInProgress(); err != nil {
		return nil, fmt.Errorf("clear in-progress: %w: %w", errs.ErrStoreWrite, err)
	}
	if err := t.store.Signals.ClearPendingCommit(); err != nil {
		return nil, fmt.Errorf("clear pending commit: %w: %w", errs.ErrStoreWrite, err)
	}

	// 11. kiểm tra quy tắc máy móc (chỉ trả sự kiện, không chặn)
	violations := t.checkRules(content, wordCount)
	return json.Marshal(commitOutput{CommitResult: result, RuleViolations: violations})
}

func (t *CommitChapterTool) appendCommitCheckpoint(chapter int) error {
	_, err := t.store.Checkpoints.AppendArtifact(
		domain.ChapterScope(chapter), "commit",
		fmt.Sprintf("chapters/%02d.md", chapter),
	)
	return err
}

// checkRules kiểm tra máy móc nội dung chương: Lint đường cơ sở sản phẩm tích hợp sẵn (cơ chế còn lại, luôn thực thi)
// + Check quy tắc người dùng (đọc structured từ ảnh chụp sách; khi thiếu ảnh chụp quay lại mặc định tích hợp, đảm bảo đường cơ sở máy móc luôn có).
func (t *CommitChapterTool) checkRules(text string, wordCount int) []rules.Violation {
	violations := rules.Lint(text)
	structured := rules.SystemDefaults().Structured
	if snap, err := t.store.UserRules.Load(); err == nil && snap != nil {
		structured = snap.Structured
	}
	return append(violations, rules.Check(text, wordCount, structured)...)
}

// executeRewriteCommit xử lý nộp chương trau chuốt/viết lại: ghi đè bản cuối và tóm tắt, cập nhật số chữ, drain hàng đợi.
// Bỏ qua tất cả bổ sung trạng thái thế giới (timeline / foreshadow / relationship / state_changes) và phát hiện biên cung,
// những thứ này đã được áp dụng lúc nộp chương gốc.
func (t *CommitChapterTool) executeRewriteCommit(
	chapter int,
	summary string,
	characters, keyEvents []string,
	hookType, dominantStrand string,
	progress *domain.Progress,
) (json.RawMessage, error) {
	// 1. tải nội dung sau khi trau chuốt
	content, wordCount, err := t.store.Drafts.LoadChapterContent(chapter)
	if err != nil {
		return nil, fmt.Errorf("rewrite: load chapter content: %w: %w", errs.ErrStoreRead, err)
	}
	if content == "" {
		return nil, fmt.Errorf("no content found for chapter %d: %w", chapter, errs.ErrToolPrecondition)
	}

	// 2. kiểm tra cứng: drafts hoàn toàn giống bản cuối hiện tại → xác định chưa thực sự trau chuốt/viết lại (writer đã bỏ qua draft_chapter)
	// từ chối commit, buộc writer phải gọi draft_chapter(mode=write) để ghi phiên bản mới trước.
	existingFinal, _ := t.store.Drafts.LoadChapterText(chapter)
	if existingFinal != "" && existingFinal == content {
		mode := "viết lại"
		if progress != nil && progress.Flow == domain.FlowPolishing {
			mode = "trau chuốt"
		}
		return nil, fmt.Errorf("chương %d drafts và chapters hoàn toàn giống nhau, không phát hiện thay đổi %s. Vui lòng gọi draft_chapter(mode=write, chapter=%d) để ghi nội dung mới sau %s, rồi commit_chapter: %w",
			chapter, mode, chapter, mode, errs.ErrToolPrecondition)
	}

	// 3. ghi đè bản cuối
	if err := t.store.Drafts.SaveFinalChapter(chapter, content); err != nil {
		return nil, fmt.Errorf("rewrite: save final chapter: %w: %w", errs.ErrStoreWrite, err)
	}

	// 3. ghi đè tóm tắt
	if err := t.store.Summaries.SaveSummary(domain.ChapterSummary{
		Chapter:    chapter,
		Summary:    summary,
		Characters: characters,
		KeyEvents:  keyEvents,
	}); err != nil {
		return nil, fmt.Errorf("rewrite: save summary: %w: %w", errs.ErrStoreWrite, err)
	}

	// 4. cập nhật số chữ (MarkChapterComplete là idempotent với chương đã hoàn thành: thay thế word count, slice.Contains ngăn vào hàng đợi trùng)
	if err := t.store.Progress.MarkChapterComplete(chapter, wordCount, hookType, dominantStrand); err != nil {
		return nil, fmt.Errorf("rewrite: update word count: %w: %w", errs.ErrStoreWrite, err)
	}

	// 5. Drain hàng đợi chờ xử lý; khi hàng đợi trống CompleteRewrite sẽ tự động chuyển flow về writing
	if err := t.store.Progress.CompleteRewrite(chapter); err != nil {
		return nil, fmt.Errorf("rewrite: complete rewrite: %w: %w", errs.ErrStoreWrite, err)
	}

	// 6. Checkpoint (điểm kiểm tra)
	if _, err := t.store.Checkpoints.AppendArtifact(
		domain.ChapterScope(chapter), "commit",
		fmt.Sprintf("chapters/%02d.md", chapter),
	); err != nil {
		return nil, fmt.Errorf("rewrite: checkpoint commit: %w: %w", errs.ErrStoreWrite, err)
	}

	// 7. đọc ảnh chụp Progress sau khi drain, trả về như sự kiện
	mode := "rewrite"
	if progress.Flow == domain.FlowPolishing {
		mode = "polish"
	}
	latest, _ := t.store.Progress.Load()
	remaining := []int{}
	nextChapter := chapter + 1
	flow := string(domain.FlowWriting)
	if latest != nil {
		remaining = append(remaining, latest.PendingRewrites...)
		nextChapter = latest.NextChapter()
		flow = string(latest.Flow)
	}
	drained := len(remaining) == 0

	// Sau khi hàng đợi rỗng mới xét hoàn thành: nộp sau trả công không đi qua đường chính applyCompletion, hoàn thành chỉ có thể kích hoạt ở đây.
	//   - Phân tầng + viết xuôi: dùng layeredBookComplete cấp chất lượng (yêu cầu thu hồi tuyến), chưa thỏa mãn nhường kiến trúc sư.
	//   - Phân tầng + reopen trả công (ReopenedFromComplete): trả công chỉ sửa chương đã có, không tăng giảm cấu trúc, theo cấu trúc hoàn chỉnh
	//     là hoàn thành lại — nếu trả công làm xáo trộn tuyến nào đó thì kẹt ở writing, cuối tập cuối sẽ rơi vào vòng lặp viết tiếp vượt biên.
	//   - Không phân tầng: viết đủ TotalChapters là hoàn thành (trả công không tăng giảm số chương, ban đầu đã đủ).
	bookComplete := false
	if drained && latest != nil {
		reComplete := false
		switch {
		case latest.Layered && latest.ReopenedFromComplete:
			reComplete = t.layeredStructurallyComplete(latest)
		case latest.Layered:
			reComplete = t.layeredBookComplete(latest)
		default:
			reComplete = latest.TotalChapters > 0 && len(latest.CompletedChapters) >= latest.TotalChapters
		}
		if reComplete {
			if cerr := t.store.Progress.MarkComplete(); cerr == nil {
				bookComplete = true
				if p, _ := t.store.Progress.Load(); p != nil {
					flow = string(p.Flow)
				}
			}
		}
	}

	// Cùng đường chính: rewrite/polish cũng kiểm tra máy móc và đính kèm rule_violations
	violations := t.checkRules(content, wordCount)
	return json.Marshal(map[string]any{
		"chapter":         chapter,
		"rewritten":       true,
		"mode":            mode,
		"word_count":      wordCount,
		"remaining_queue": remaining,
		"queue_drained":   drained,
		"next_chapter":    nextChapter,
		"flow":            flow,
		"book_complete":   bookComplete,
		"rule_violations": violations,
	})
}

// buildSkipResult xây dựng kết quả trả về căn chỉnh với commit thông thường cho "nộp trùng chương đã hoàn thành".
// Điều phối viên dựa vào đó ra quyết định tiếp theo (phân phối writer/editor/architect), không bị ảo giác vì nhận được gợi ý prose.
func (t *CommitChapterTool) buildSkipResult(chapter int, progress *domain.Progress) (json.RawMessage, error) {
	_, wordCount, _ := t.store.Drafts.LoadChapterContent(chapter)

	result := domain.CommitResult{
		Chapter:     chapter,
		Committed:   true,
		WordCount:   wordCount,
		NextChapter: chapter + 1,
	}

	if progress != nil && progress.Layered {
		if boundary, _ := t.store.Outline.CheckArcBoundary(chapter); boundary != nil {
			result.ArcEnd = boundary.IsArcEnd
			result.VolumeEnd = boundary.IsVolumeEnd
			result.Volume = boundary.Volume
			result.Arc = boundary.Arc
			result.NeedsExpansion = boundary.NeedsExpansion
			result.NeedsNewVolume = boundary.NeedsNewVolume
			result.NextVolume = boundary.NextVolume
			result.NextArc = boundary.NextArc
		}
		result.ReviewRequired, result.ReviewReason = domain.ShouldArcReview(result.ArcEnd, result.VolumeEnd, result.Volume, result.Arc)
	} else if progress != nil {
		result.ReviewRequired, result.ReviewReason = domain.ShouldReview(len(progress.CompletedChapters))
	}

	if progress != nil {
		if progress.Phase == domain.PhaseComplete {
			result.BookComplete = true
		}
		result.Flow = string(progress.Flow)
	}

	return json.Marshal(result)
}

// loadCoreCharacterNameSet tải tập hợp tên nhân vật đã có trong characters.json (gồm cả bí danh).
// Dùng làm tập lọc "cốt lõi đã biết" cho cast_ledger — nhân vật cốt lõi không vào danh sách phụ.
// Khi tải thất bại trả về nil (khi merge tất cả characters đều vào ledger, chấp nhận được).
func loadCoreCharacterNameSet(s *store.Store) map[string]bool {
	chars, err := s.Characters.Load()
	if err != nil || len(chars) == 0 {
		return nil
	}
	set := make(map[string]bool, len(chars)*2)
	for _, c := range chars {
		if c.Name != "" {
			set[c.Name] = true
		}
		for _, alias := range c.Aliases {
			if alias != "" {
				set[alias] = true
			}
		}
	}
	return set
}

// applyCompletion xác định commit này có làm toàn bộ sách hoàn thành không, nếu có thì MarkComplete và trả về true.
//   - Không phân tầng: viết đủ tổng số chương đã thỏa thuận là hoàn thành.
//   - Phân tầng: kiến trúc sư gọi tường minh save_foundation type=complete_book là đường chính; ở đây thêm một lớp
//     dự phòng chắc chắn — khi toàn bộ sách đã khách quan thỏa mãn điều kiện hoàn thành (xem layeredBookComplete) thì tự kết thúc.
//     Ngăn mô hình ở điểm cuối không gọi append_volume cũng không complete_book, dẫn đến "writer chạy trần chương vượt biên →
//     lính canh vượt biên chặn → thử lại liên tục" livelock (căn nguyên của trường hợp ch204..347).
func (t *CommitChapterTool) applyCompletion(result *domain.CommitResult, progress *domain.Progress) bool {
	if progress == nil {
		return false
	}
	if progress.Layered {
		if t.layeredBookComplete(progress) {
			_ = t.store.Progress.MarkComplete()
			return true
		}
		return false
	}
	if progress.TotalChapters > 0 && result.NextChapter > progress.TotalChapters {
		_ = t.store.Progress.MarkComplete()
		return true
	}
	return false
}

// layeredStructurallyComplete xác định tiểu thuyết dài phân tầng có "viết xong về cấu trúc": hàng đợi trả công rỗng + không có cung khung xương chờ mở rộng
// + tất cả chương đã mở rộng đều đã viết. Đây là sự kiện chắc chắn về trạng thái cuối, không gồm phán đoán ngữ nghĩa phục bút/tuyến dài — dùng làm lưới an toàn "phòng vòng lặp chết trạng thái cuối" (hoàn thành lại sau khi hàng đợi trả công rỗng).
func (t *CommitChapterTool) layeredStructurallyComplete(progress *domain.Progress) bool {
	// 1. hàng đợi trả công phải rỗng
	if len(progress.PendingRewrites) > 0 {
		return false
	}
	volumes, err := t.store.Outline.LoadLayeredOutline()
	if err != nil || len(volumes) == 0 {
		return false
	}
	// 2. không được còn cung khung xương chờ mở rộng (trong kế hoạch vẫn còn nội dung cần viết)
	for i := range volumes {
		for j := range volumes[i].Arcs {
			if !volumes[i].Arcs[j].IsExpanded() {
				return false
			}
		}
	}
	// 3. tất cả chương đã mở rộng phải được viết xong
	expanded := len(domain.FlattenOutline(volumes))
	return expanded > 0 && len(progress.CompletedChapters) >= expanded
}

// layeredBookComplete dùng sự kiện khách quan để xác định tiểu thuyết dài phân tầng có thực sự viết xong không, đối chiếu với
// các mục có thể định lượng trong danh sách hoàn thành architect-long.md + sự kiện cấu trúc. Trên cơ sở cấu trúc hoàn chỉnh còn yêu cầu phục bút về không, tuyến dài thu hồi — bất kỳ điều kiện nào không thỏa mãn đều
// nhường kiến trúc sư tiếp tục expand_arc / append_volume, tuyệt đối không kết thúc khi câu chuyện chưa viết xong. Khi không có la bàn thì bảo thủ
// xác định là chưa hoàn thành. Đây là hoàn thành "cấp chất lượng" của viết xuôi, nghiêm ngặt hơn layeredStructurallyComplete.
func (t *CommitChapterTool) layeredBookComplete(progress *domain.Progress) bool {
	if !t.layeredStructurallyComplete(progress) {
		return false
	}
	// 4. phục bút hoạt động phải về không (cam kết đã thực hiện)
	if active, aerr := t.store.World.LoadActiveForeshadow(); aerr != nil || len(active) > 0 {
		return false
	}
	// 5. tuyến dài hoạt động của la bàn phải thu hồi (không có la bàn / tuyến dài chưa rỗng đều trả về kiến trúc sư phán quyết)
	compass, cerr := t.store.Outline.LoadCompass()
	if cerr != nil || compass == nil || len(compass.OpenThreads) > 0 {
		return false
	}
	return true
}

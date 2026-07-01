package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

// References tài liệu tham khảo nhúng kèm.
type References struct {
	// V0
	ChapterGuide      string
	HookTechniques    string
	QualityChecklist  string
	OutlineTemplate   string
	CharacterTemplate string
	ChapterTemplate   string
	// V1
	Consistency      string
	ContentExpansion string
	DialogueWriting  string
	// V2
	StyleReference   string // tham khảo bổ sung về phong cách (có thể để trống)
	LongformPlanning string // tham khảo hoạch định truyện dài chung
	Differentiation  string // tham khảo thiết kế khác biệt hóa chung
	ArcTemplates     string // mẫu cung theo thể loại (tải theo style, có thể để trống)
	AntiAITone       string // kho tiêu chí loại bỏ giọng AI (writer/editor dùng chung, tiêm xuyên suốt)
}

// ContextTool tổng hợp ngữ cảnh cần thiết cho chương hiện tại.
type ContextTool struct {
	store *store.Store
	refs  References
	style string
}

// NewContextTool tạo công cụ ngữ cảnh.
// user_rules được tiêm bởi buildUserRules đọc thẳng từ bản chụp (meta/user_rules.json), không còn phụ thuộc vào tùy chọn tải.
func NewContextTool(store *store.Store, refs References, style string) *ContextTool {
	return &ContextTool{store: store, refs: refs, style: style}
}

func (t *ContextTool) Name() string { return "novel_context" }
func (t *ContextTool) Description() string {
	return "Lấy trạng thái hiện tại của tiểu thuyết và ngữ cảnh sáng tác. " +
		"Không truyền chapter: trả về progress_status (các trường tiến độ như phase/flow/next_chapter/pending_rewrites) + thiết định cơ bản, dùng để phán đoán bước tiếp theo. " +
		"Truyền chapter=N: bổ sung trả về ngữ cảnh viết như tóm tắt trước đó, phục bút, trạng thái nhân vật, quy tắc phong cách của chương đó"
}
func (t *ContextTool) Label() string { return "Tải ngữ cảnh" }

// Công cụ chỉ đọc, có thể lên lịch song song.
func (t *ContextTool) ReadOnly(_ json.RawMessage) bool        { return true }
func (t *ContextTool) ConcurrencySafe(_ json.RawMessage) bool { return true }

func (t *ContextTool) Schema() map[string]any {
	return schema.Object(
		schema.Property("chapter", schema.Int("Số chương. Không truyền thì trả về trạng thái tiến độ và thiết định cơ bản (Coordinator dùng để phán đoán bước tiếp theo); truyền vào thì bổ sung trả về ngữ cảnh viết của chương đó (Writer dùng)")),
	)
}

func (t *ContextTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Chapter int `json:"chapter"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}

	result := make(map[string]any)
	var warnings []string
	seenWarnings := make(map[string]struct{})
	warn := func(scope string, err error) {
		if err == nil || os.IsNotExist(err) {
			return
		}
		msg := fmt.Sprintf("%s đọc thất bại: %v", scope, err)
		if _, ok := seenWarnings[msg]; ok {
			return
		}
		seenWarnings[msg] = struct{}{}
		warnings = append(warnings, msg)
	}

	if a.Chapter > 0 {
		// Đường Writer: tải toàn bộ dữ liệu cơ bản + ngữ cảnh chương
		t.buildBaseContext(result, warn)
		seed := newChapterContextEnvelope()
		state := t.prepareChapterContext(a.Chapter, &seed, warn)
		seed.apply(result)
		t.buildChapterContext(result, state, warn)
		// Đánh dấu ngữ nghĩa dữ liệu (chữa bệnh kể lại): episodic là ghi chú sự kiện đã viết vào chính văn, không phải tư liệu chờ viết.
		// Chỉ gắn vào container, không vào gương cấp trên.
		if epi, ok := result["episodic_memory"].(map[string]any); ok && len(epi) > 0 {
			epi["_usage"] = "Container này là ghi chú sự kiện đã viết vào chính văn (dùng để đối chiếu nhất quán và kết nối); kể lại nguyên xi nội dung này trong chính văn chương mới là lỗi lặp lại"
		}
	} else {
		// Đường Coordinator/Architect: chỉ trả trạng thái + dữ liệu cấu trúc, không tải toàn văn bản
		t.buildProgressStatus(result)
		t.buildArchitectContext(result, warn)
	}

	// Tiêm working_memory.user_rules (đường canonical). Đường architect vốn không có working_memory,
	// buildUserRules tạo container chỉ chứa user_rules khi cần. Khi bản chụp thiếu thì lui về mặc định tích hợp,
	// luôn xuất cấu trúc ổn định, tránh LLM thấy user_rules=null mà đi vào nhánh ngoại lệ.
	if a.Chapter > 0 {
		t.buildSimulationProfile(result, "working_memory", warn)
	} else {
		t.buildSimulationProfile(result, "planning_memory", warn)
	}

	t.buildUserRules(result)
	t.buildDossier(result)

	if len(warnings) > 0 {
		result["_warnings"] = warnings
	}

	// Ngân sách ưu tiên: tự động cắt bớt dữ liệu ưu tiên thấp khi tổng kích thước vượt ngưỡng
	if a.Chapter > 0 {
		trimByBudget(result, 100*1024) // Writer: 100KB
	} else {
		trimByBudget(result, 60*1024) // Coordinator/Architect: 60KB
	}

	result["_loading_summary"] = buildLoadingSummary(result, a.Chapter)
	return json.Marshal(result)
}

// buildLoadingSummary thống kê lượng dữ liệu từng mục trong result đã được tổng hợp, tạo ra một dòng tóm tắt dễ đọc.
func buildLoadingSummary(result map[string]any, chapter int) string {
	var parts []string

	if chapter > 0 {
		parts = append(parts, fmt.Sprintf("ch=%d", chapter))
	} else {
		parts = append(parts, "architect")
	}
	if tier, ok := result["planning_tier"].(domain.PlanningTier); ok && tier != "" {
		parts = append(parts, fmt.Sprintf("tier=%s", tier))
	}

	// vị trí tập-cung
	if pos, ok := result["position"].(map[string]any); ok {
		parts = append(parts, fmt.Sprintf("V%dA%d", pos["volume"], pos["arc"]))
	}

	var items []string
	countSlice := func(key string) int {
		if v, ok := result[key]; ok {
			if s, ok := v.([]domain.Character); ok {
				return len(s)
			}
			// phản chiếu slice tổng quát
			return sliceLen(v)
		}
		return 0
	}

	// nhân vật
	if n := countSlice("character_snapshots"); n > 0 {
		items = append(items, fmt.Sprintf("nv:%d(ảnh chụp)", n))
	} else if n := countSlice("characters"); n > 0 {
		items = append(items, fmt.Sprintf("nv:%d", n))
	}

	if working, ok := result["working_memory"].(map[string]any); ok && len(working) > 0 {
		items = append(items, fmt.Sprintf("bmLam:%d", len(working)))
	}
	if episodic, ok := result["episodic_memory"].(map[string]any); ok && len(episodic) > 0 {
		items = append(items, fmt.Sprintf("bmTinhTiet:%d", len(episodic)))
	}
	if planning, ok := result["planning_memory"].(map[string]any); ok && len(planning) > 0 {
		items = append(items, fmt.Sprintf("bmHoachDinh:%d", len(planning)))
	}
	if foundation, ok := result["foundation_memory"].(map[string]any); ok && len(foundation) > 0 {
		items = append(items, fmt.Sprintf("bmNenTang:%d", len(foundation)))
	}

	// tóm tắt phân tầng
	if n := countSlice("volume_summaries"); n > 0 {
		items = append(items, fmt.Sprintf("ttTap:%d", n))
	}
	if n := countSlice("arc_summaries"); n > 0 {
		items = append(items, fmt.Sprintf("ttCung:%d", n))
	}
	if n := countSlice("recent_summaries"); n > 0 {
		items = append(items, fmt.Sprintf("ttChuong:%d", n))
	}

	// dàn ý phân tầng
	if n := countSlice("layered_outline"); n > 0 {
		items = append(items, fmt.Sprintf("danY:%dtap", n))
	}

	// dữ liệu trạng thái
	if n := countSlice("timeline"); n > 0 {
		items = append(items, fmt.Sprintf("tgian:%d", n))
	}
	if n := countSlice("foreshadow_ledger"); n > 0 {
		items = append(items, fmt.Sprintf("phucBut:%d", n))
	}
	if n := countSlice("relationship_state"); n > 0 {
		items = append(items, fmt.Sprintf("quanHe:%d", n))
	}
	if n := countSlice("recent_state_changes"); n > 0 {
		items = append(items, fmt.Sprintf("ttThayDoi:%d", n))
	}
	if _, ok := result["previous_tail"]; ok {
		items = append(items, "duoiChuongTruoc:ok")
	}
	if _, ok := result["style_rules"]; ok {
		items = append(items, "quyTacPhong:ok")
	}
	if n := sliceLen(result["related_chapters"]); n > 0 {
		items = append(items, fmt.Sprintf("chuongLienQuan:%d", n))
	}
	if selected, ok := result["selected_memory"].(map[string]any); ok && len(selected) > 0 {
		if n := sliceLen(selected["story_threads"]); n > 0 {
			items = append(items, fmt.Sprintf("goiLaiCotTruyen:%d", n))
		}
		if n := sliceLen(selected["review_lessons"]); n > 0 {
			items = append(items, fmt.Sprintf("goiLaiThamDinh:%d", n))
		}
	}

	// tài liệu tham khảo
	if refs, ok := result["references"].(map[string]string); ok && len(refs) > 0 {
		items = append(items, fmt.Sprintf("thamKhao:%dmuc", len(refs)))
	}
	if pack, ok := result["reference_pack"].(map[string]any); ok && len(pack) > 0 {
		items = append(items, fmt.Sprintf("goiTK:%d", len(pack)))
	}
	if _, ok := result["memory_policy"]; ok {
		items = append(items, "chiLuocBM:ok")
	}
	if _, ok := result["simulation_profile"]; ok {
		items = append(items, "hoaSiMo:ok")
	}
	if warnings, ok := result["_warnings"].([]string); ok && len(warnings) > 0 {
		items = append(items, fmt.Sprintf("canhBao:%d", len(warnings)))
	}
	if trimmed, ok := result["_trimmed"].([]string); ok && len(trimmed) > 0 {
		items = append(items, fmt.Sprintf("catBot:%s", strings.Join(trimmed, ",")))
	}

	if len(items) > 0 {
		parts = append(parts, strings.Join(items, " "))
	}
	return strings.Join(parts, " | ")
}

// sliceLen cố gắng lấy độ dài slice từ kiểu any.
func sliceLen(v any) int {
	switch s := v.(type) {
	case []domain.ChapterSummary:
		return len(s)
	case []domain.ArcSummary:
		return len(s)
	case []domain.VolumeSummary:
		return len(s)
	case []domain.CharacterSnapshot:
		return len(s)
	case []domain.TimelineEvent:
		return len(s)
	case []domain.ForeshadowEntry:
		return len(s)
	case []domain.RelationshipEntry:
		return len(s)
	case []domain.StateChange:
		return len(s)
	case []domain.VolumeOutline:
		return len(s)
	case []domain.Character:
		return len(s)
	case []domain.RelatedChapter:
		return len(s)
	case []domain.RecallItem:
		return len(s)
	default:
		return 0
	}
}

// loadFilteredCharacters lọc nhân vật theo Tier và sự xuất hiện trong cảnh.
// core/important luôn được trả về; secondary/decorative chỉ trả về khi dàn ý chương hiện tại đề cập đến.
func (t *ContextTool) loadFilteredCharacters(result map[string]any, chapter int, warn func(string, error)) {
	chars, err := t.store.Characters.Load()
	if err != nil {
		warn("characters", err)
		return
	}
	if len(chars) == 0 {
		return
	}

	// lấy mô tả cảnh của dàn ý chương hiện tại để khớp nhân vật phụ
	entry, err := t.store.Outline.GetChapterOutline(chapter)
	if err != nil {
		warn("current_chapter_outline", err)
		result["characters"] = chars
		return
	}
	sceneText := strings.Join(entry.Scenes, " ") + " " + entry.CoreEvent + " " + entry.Title

	var filtered []domain.Character
	for _, c := range chars {
		switch c.Tier {
		case "secondary", "decorative":
			if matchCharacter(sceneText, c) {
				filtered = append(filtered, c)
			}
		default: // core, important, hoặc chưa đặt
			filtered = append(filtered, c)
		}
	}
	result["characters"] = filtered
}

// matchCharacter kiểm tra xem văn bản cảnh có chứa tên chính thức hoặc bất kỳ bí danh nào của nhân vật không.
func matchCharacter(text string, c domain.Character) bool {
	if strings.Contains(text, c.Name) {
		return true
	}
	for _, alias := range c.Aliases {
		if strings.Contains(text, alias) {
			return true
		}
	}
	return false
}

// loadLayeredSummaries tải tóm tắt phân tầng: tóm tắt tập + tóm tắt cung trong tập hiện tại + tóm tắt chương trong cung.
func (t *ContextTool) loadLayeredSummaries(result map[string]any, chapter, summaryWindow int, warn func(string, error)) {
	vol, arc, err := t.store.Outline.LocateChapter(chapter)
	if err != nil {
		warn("layered_outline_position", err)
		// lui về chế độ phẳng
		if summaries, err := t.store.Summaries.LoadRecentSummaries(chapter, summaryWindow); err == nil && len(summaries) > 0 {
			result["recent_summaries"] = summaries
		} else {
			warn("recent_summaries", err)
		}
		return
	}

	// 1. tóm tắt tập của các tập đã hoàn thành
	if volSummaries, err := t.store.Summaries.LoadAllVolumeSummaries(); err == nil && len(volSummaries) > 0 {
		result["volume_summaries"] = volSummaries
	} else {
		warn("volume_summaries", err)
	}

	// 2. tóm tắt cung của các cung đã hoàn thành trong tập hiện tại (không gồm cung hiện tại)
	if arcSummaries, err := t.store.Summaries.LoadArcSummaries(vol); err == nil && len(arcSummaries) > 0 {
		var prior []domain.ArcSummary
		for _, s := range arcSummaries {
			if s.Arc < arc {
				prior = append(prior, s)
			}
		}
		if len(prior) > 0 {
			result["arc_summaries"] = prior
		}
	} else {
		warn("arc_summaries", err)
	}

	// 3. tóm tắt N chương gần nhất trong cung hiện tại
	if summaries, err := t.store.Summaries.LoadRecentSummaries(chapter, summaryWindow); err == nil && len(summaries) > 0 {
		result["recent_summaries"] = summaries
	} else {
		warn("recent_summaries", err)
	}
}

// loadLayeredCharacters tải nhân vật ở chế độ Layered: ưu tiên dùng ảnh chụp gần nhất, lui về thiết định gốc + lọc Tier.
func (t *ContextTool) loadLayeredCharacters(result map[string]any, chapter int, warn func(string, error)) {
	snapshots, err := t.store.Characters.LoadLatestSnapshots()
	if err == nil && len(snapshots) > 0 {
		result["character_snapshots"] = snapshots
		// đồng thời giữ lại nhân vật core/important trong thiết định gốc (ảnh chụp có thể chưa có nhân vật mới xuất hiện)
		t.loadFilteredCharacters(result, chapter, warn)
		return
	}
	warn("character_snapshots", err)
	// không có ảnh chụp thì lui về thiết định gốc
	t.loadFilteredCharacters(result, chapter, warn)
}

// writerReferences trả về tài liệu tham khảo viết lách. Chương 1 trả về toàn bộ, các chương sau cắt bớt các mẫu không còn cần.
func (t *ContextTool) writerReferences(chapter int) map[string]string {
	refs := map[string]string{}
	add := func(k, v string) {
		if v != "" {
			refs[k] = v
		}
	}
	// tải theo tiến độ: luôn giữ tham khảo cốt lõi, 3 chương đầu tải thêm hướng dẫn viết đầy đủ
	add("consistency", t.refs.Consistency)
	add("hook_techniques", t.refs.HookTechniques)
	add("quality_checklist", t.refs.QualityChecklist)
	add("anti_ai_tone", t.refs.AntiAITone) // tiêu chí loại bỏ giọng AI tiêm xuyên suốt, không cắt theo chương
	if chapter <= 3 {
		add("chapter_guide", t.refs.ChapterGuide)
		add("dialogue_writing", t.refs.DialogueWriting)
		add("style_reference", t.refs.StyleReference)
	}

	// tham khảo bổ sung chỉ tải ở chương đầu tiên
	if chapter <= 1 {
		add("chapter_template", t.refs.ChapterTemplate)
		add("content_expansion", t.refs.ContentExpansion)
	}
	return refs
}

func (t *ContextTool) architectReferences() map[string]string {
	refs := map[string]string{}
	add := func(k, v string) {
		if v != "" {
			refs[k] = v
		}
	}
	add("outline_template", t.refs.OutlineTemplate)
	add("character_template", t.refs.CharacterTemplate)
	add("longform_planning", t.refs.LongformPlanning)
	add("differentiation", t.refs.Differentiation)
	add("style_reference", t.refs.StyleReference)
	add("arc_templates", t.refs.ArcTemplates)
	add("anti_ai_tone", t.refs.AntiAITone) // loại giọng AI cho dàn ý architect; cũng xử lý editor đi theo đường Chapter=0
	return refs
}

// foundationStatus kiểm tra tính đầy đủ của thiết định cơ bản, trả về danh sách các mục còn thiếu.
// Dùng chung logic phán định store.FoundationMissing với công cụ save_foundation, đảm bảo
// ready/missing mà LLM thấy trong novel_context luôn nhất quán với foundation_ready
// mà save_foundation trả về (các chi tiết như compass bắt buộc cho truyện dài không bị lệch).
func (t *ContextTool) foundationStatus() map[string]any {
	missing := t.store.FoundationMissing()
	status := map[string]any{"ready": len(missing) == 0}
	if len(missing) > 0 {
		status["missing"] = missing
	}
	return status
}

// ContextSummary trả về tóm tắt ngắn gọn về trạng thái hiện tại (dùng cho log).
func (t *ContextTool) ContextSummary() string {
	var parts []string
	if p, _ := t.store.Outline.LoadPremise(); p != "" {
		parts = append(parts, "premise:ok")
	}
	if o, _ := t.store.Outline.LoadOutline(); o != nil {
		parts = append(parts, fmt.Sprintf("outline:%d chapters", len(o)))
	}
	if c, _ := t.store.Characters.Load(); c != nil {
		parts = append(parts, fmt.Sprintf("characters:%d", len(c)))
	}
	if len(parts) == 0 {
		return "empty"
	}
	return strings.Join(parts, ", ")
}

// trimByBudget cắt bớt result theo ưu tiên để tổng kích thước JSON không vượt quá budget byte.
// Ưu tiên (từ thấp đến cao): references < voice_samples < style_anchors < previous_tail < timeline
//
//	< recent_state_changes < foreshadow_ledger < relationship_state < phần còn lại (không cắt)
//
// Các key đã cắt được ghi vào result["_trimmed"] để debug log.
func trimByBudget(result map[string]any, budget int) {
	// đo kích thước hiện tại trước
	data, err := json.Marshal(result)
	if err != nil || len(data) <= budget {
		return
	}

	// liệt kê các key có thể cắt từ ưu tiên thấp đến cao
	trimOrder := []string{
		"references",
		"voice_samples",
		"style_anchors",
		"style_rules",
		"style_stats",
		"previous_tail",
		"timeline",
		"recent_state_changes",
		"foreshadow_ledger",
		"relationship_state",
	}

	var trimmed []string
	for _, key := range trimOrder {
		if _, ok := result[key]; !ok {
			continue
		}
		deleteContextKey(result, key)
		trimmed = append(trimmed, key)
		data, err = json.Marshal(result)
		if err != nil || len(data) <= budget {
			break
		}
	}
	if len(trimmed) > 0 {
		result["_trimmed"] = trimmed
	}
}

func deleteContextKey(result map[string]any, key string) {
	delete(result, key)
	for _, containerKey := range []string{
		"working_memory",
		"episodic_memory",
		"planning_memory",
		"foundation_memory",
		"reference_pack",
	} {
		section, ok := result[containerKey].(map[string]any)
		if !ok {
			continue
		}
		delete(section, key)
	}
}

// buildRelatedChapters tra cứu ngược các chương lịch sử liên quan đến chương hiện tại dựa trên dữ liệu cấu trúc.
// Đề xuất từ 4 chiều: phục bút, xuất hiện nhân vật, thay đổi trạng thái và quan hệ; sau khi lọc trùng trả về tối đa 5 mục.
// Tất cả dữ liệu được truyền qua tham số, không thực hiện IO bổ sung.
func (t *ContextTool) buildRelatedChapters(
	chapter int,
	entry *domain.OutlineEntry,
	foreshadow []domain.ForeshadowEntry,
	relationships []domain.RelationshipEntry,
	stateChanges []domain.StateChange,
) []domain.RelatedChapter {
	const recentWindow = 10
	const maxResults = 5

	seen := make(map[int]struct{})
	var results []domain.RelatedChapter
	add := func(ch int, reason string) {
		if ch <= 0 || ch >= chapter {
			return
		}
		// các chương gần đây quá gần, không đề xuất
		if ch > chapter-recentWindow {
			return
		}
		if _, ok := seen[ch]; ok {
			return
		}
		seen[ch] = struct{}{}
		results = append(results, domain.RelatedChapter{Chapter: ch, Reason: reason})
	}

	// ghép văn bản dàn ý để khớp từ khóa
	outlineText := entry.Title + " " + entry.CoreEvent
	for _, s := range entry.Scenes {
		outlineText += " " + s
	}

	// 1. tra ngược phục bút: mô tả phục bút đang hoạt động có liên quan đến dàn ý chương hiện tại không
	for _, f := range foreshadow {
		if strings.Contains(outlineText, f.ID) || containsAny(outlineText, strings.Fields(f.Description)) {
			add(f.PlantedAt, fmt.Sprintf("chương cài phục bút %s(%s)", f.ID, truncateRunes(f.Description, 15)))
		}
		if len(results) >= maxResults {
			break
		}
	}

	// 2. tra ngược xuất hiện nhân vật: duyệt một lần theo lô, IO từ O(số nhân vật×số chương) xuống O(số chương)
	chars, _ := t.store.Characters.Load()
	outlineChars := matchOutlineCharacters(outlineText, chars)
	if len(outlineChars) > 0 {
		appearances := t.store.Summaries.FindCharacterAppearances(outlineChars, chapter, recentWindow)
		for _, name := range outlineChars {
			if len(results) >= maxResults {
				break
			}
			if ch, ok := appearances[name]; ok {
				add(ch, fmt.Sprintf("chương xuất hiện cuối của '%s'", name))
			}
		}
	}

	// 3. tra ngược thay đổi trạng thái: thao tác trên slice đã tải, IO bằng 0
	for _, name := range outlineChars {
		if len(results) >= maxResults {
			break
		}
		ch := findLastStateChange(stateChanges, name, chapter)
		if ch > 0 && ch <= chapter-recentWindow {
			add(ch, fmt.Sprintf("chương thay đổi trạng thái của '%s'", name))
		}
	}

	// 4. tra ngược quan hệ: biến đổi quan hệ gần nhất giữa các cặp nhân vật trong chương hiện tại
	if len(relationships) > 0 && len(outlineChars) >= 2 {
		charSet := make(map[string]struct{}, len(outlineChars))
		for _, c := range outlineChars {
			charSet[c] = struct{}{}
		}
		for _, r := range relationships {
			if len(results) >= maxResults {
				break
			}
			_, aIn := charSet[r.CharacterA]
			_, bIn := charSet[r.CharacterB]
			if aIn && bIn {
				add(r.Chapter, fmt.Sprintf("biến đổi quan hệ %s-%s", r.CharacterA, r.CharacterB))
			}
		}
	}

	return results
}

// findLastStateChange tìm số chương của lần thay đổi gần nhất của thực thể trong danh sách thay đổi trạng thái đã tải.
func findLastStateChange(changes []domain.StateChange, entity string, currentChapter int) int {
	for i := len(changes) - 1; i >= 0; i-- {
		if changes[i].Entity == entity && changes[i].Chapter < currentChapter {
			return changes[i].Chapter
		}
	}
	return 0
}

// matchOutlineCharacters khớp tên nhân vật xuất hiện từ văn bản dàn ý.
func matchOutlineCharacters(text string, chars []domain.Character) []string {
	var matched []string
	for _, c := range chars {
		if strings.Contains(text, c.Name) {
			matched = append(matched, c.Name)
			continue
		}
		for _, alias := range c.Aliases {
			if strings.Contains(text, alias) {
				matched = append(matched, c.Name)
				break
			}
		}
	}
	return matched
}

// containsAny kiểm tra text có chứa bất kỳ từ nào trong words không (ít nhất 2 ký tự mới khớp, tránh nhiễu).
func containsAny(text string, words []string) bool {
	for _, w := range words {
		if len([]rune(w)) >= 2 && strings.Contains(text, w) {
			return true
		}
	}
	return false
}

func (t *ContextTool) selectStoryThreads(state contextBuildState) []domain.RecallItem {
	if state.currentEntry == nil {
		return nil
	}
	if len(state.foreshadow) < storyThreadRecallThreshold {
		return nil
	}

	const maxThreads = 5
	var items []domain.RecallItem
	seen := make(map[string]struct{})
	picked := make(map[string]struct{}) // ID phục bút đã chọn, dùng để lọc trùng khi bổ sung theo tuổi
	add := func(item domain.RecallItem) {
		key := item.Kind + "|" + item.Key + "|" + item.Summary
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		picked[item.Key] = struct{}{}
		items = append(items, item)
	}

	// 1. gợi nhớ theo liên quan: phục bút trùng từ khóa focus của chương hiện tại.
	focusTerms := recallFocusTerms(state.currentEntry, state.chapterPlan)
	focusText := strings.Join(focusTerms, " ")
	for _, entry := range state.foreshadow {
		if !matchesRecallTerms(entry.ID+" "+entry.Description, focusTerms) && !strings.Contains(focusText, entry.ID) {
			continue
		}
		add(domain.RecallItem{
			Kind:    "story_thread",
			Key:     entry.ID,
			Chapter: entry.PlantedAt,
			Reason:  "chương hiện tại có thể cần kế thừa phục bút hiện có",
			Summary: fmt.Sprintf("Phục bút %q cài ở chương %d: %s", entry.ID, entry.PlantedAt, truncateRunes(entry.Description, 30)),
		})
		if len(items) >= maxThreads {
			return items
		}
	}

	// 2. bổ sung theo tuổi: phục bút không liên quan đến chương hiện tại nhưng treo lâu chưa thu (ưu tiên cũ nhất), bổ sung vào chỗ còn thiếu.
	//    Bù điểm mù tự nhiên của gợi nhớ theo liên quan — những tuyến treo một mình quá lâu mà không khớp từ khóa ở chương này.
	for _, entry := range agingForeshadow(state.foreshadow, state.chapter, picked) {
		add(domain.RecallItem{
			Kind:    "story_thread",
			Key:     entry.ID,
			Chapter: entry.PlantedAt,
			Reason:  "phục bút treo lâu chưa thu, chú ý đẩy tiến hoặc thu hồi đúng lúc",
			Summary: fmt.Sprintf("Phục bút %q cài chương %d, đã %d chương chưa thu: %s", entry.ID, entry.PlantedAt, state.chapter-entry.PlantedAt, truncateRunes(entry.Description, 30)),
		})
		if len(items) >= maxThreads {
			break
		}
	}

	return items
}

// agingForeshadow trả về các phục bút chưa thu có tuổi ≥ foreshadowAgingChapters, sắp xếp ưu tiên cũ nhất trước,
// bỏ qua những mục đã được gợi nhớ theo liên quan chọn vào picked. Tham số all đã là danh sách active (chưa thu), không cần lọc trạng thái lại.
func agingForeshadow(all []domain.ForeshadowEntry, chapter int, picked map[string]struct{}) []domain.ForeshadowEntry {
	var aging []domain.ForeshadowEntry
	for _, e := range all {
		if _, ok := picked[e.ID]; ok {
			continue
		}
		if e.PlantedAt <= 0 || chapter-e.PlantedAt < foreshadowAgingChapters {
			continue
		}
		aging = append(aging, e)
	}
	sort.SliceStable(aging, func(i, j int) bool {
		return aging[i].PlantedAt < aging[j].PlantedAt
	})
	return aging
}

func (t *ContextTool) selectReviewLessons(chapter int, warn func(string, error)) []domain.RecallItem {
	if chapter <= 1 {
		return nil
	}

	var items []domain.RecallItem
	seen := make(map[string]struct{})
	add := func(item domain.RecallItem) {
		key := item.Summary
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		items = append(items, item)
	}

	appendReview := func(review *domain.ReviewEntry) bool {
		if review == nil {
			return false
		}
		for i, miss := range review.ContractMisses {
			add(domain.RecallItem{
				Kind:    "review_lesson",
				Key:     fmt.Sprintf("review-%d-contract-%d", review.Chapter, i),
				Chapter: review.Chapter,
				Reason:  "thẩm định gần đây chỉ ra mục contract bị bỏ sót",
				Summary: fmt.Sprintf("chương %d thiếu contract: %s", review.Chapter, miss),
			})
			if len(items) >= 3 {
				return true
			}
		}
		for i, issue := range review.Issues {
			switch issue.Severity {
			case "", "warning", "error", "critical":
				add(domain.RecallItem{
					Kind:    "review_lesson",
					Key:     fmt.Sprintf("review-%d-issue-%d", review.Chapter, i),
					Chapter: review.Chapter,
					Reason:  "thẩm định gần đây chỉ ra vấn đề cần tránh lặp lại",
					Summary: fmt.Sprintf("nhắc nhở thẩm định chương %d: %s", review.Chapter, truncateRunes(issue.Description, 36)),
				})
			}
			if len(items) >= 3 {
				return true
			}
		}
		return false
	}

	for ch := chapter - 1; ch >= max(chapter-3, 1); ch-- {
		review, err := t.store.World.LoadReview(ch)
		if err != nil {
			warn("review", err)
			continue
		}
		if appendReview(review) {
			return items
		}
	}

	globalReview, err := t.store.World.LoadLastReview(chapter - 1)
	if err != nil {
		warn("global_review", err)
	} else if appendReview(globalReview) {
		return items
	}
	return items
}

func recallFocusTerms(entry *domain.OutlineEntry, plan *domain.ChapterPlan) []string {
	if entry == nil {
		return nil
	}
	var terms []string
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v != "" {
			terms = append(terms, v)
		}
	}

	add(entry.Title)
	add(entry.CoreEvent)
	add(entry.Hook)
	for _, scene := range entry.Scenes {
		add(scene)
	}
	if plan != nil {
		add(plan.Goal)
		add(plan.Hook)
		for _, point := range plan.Contract.PayoffPoints {
			add(point)
		}
		add(plan.Contract.HookGoal)
	}
	return terms
}

func matchesRecallTerms(text string, terms []string) bool {
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if len([]rune(term)) < 2 {
			continue
		}
		if strings.Contains(text, term) || strings.Contains(term, text) {
			return true
		}
		if hasMeaningfulOverlap(term, text) {
			return true
		}
	}
	return false
}

func hasMeaningfulOverlap(a, b string) bool {
	ar := []rune(strings.TrimSpace(a))
	br := []rune(strings.TrimSpace(b))
	if len(ar) < 5 || len(br) < 5 {
		return false
	}
	shorter := len(ar)
	if len(br) < shorter {
		shorter = len(br)
	}
	threshold := 5
	switch {
	case shorter >= 12:
		threshold = 7
	case shorter >= 9:
		threshold = 6
	}
	return longestCommonSubstringRunes(ar, br) >= threshold
}

const storyThreadRecallThreshold = 6
const storyThreadRecallMinSelected = 2

// foreshadowAgingChapters: một phục bút kể từ khi cài mà vượt quá số chương này vẫn chưa thu, coi là "treo lâu".
// Loại phục bút này dù không liên quan đến từ khóa chương hiện tại vẫn được bổ sung vào story_threads, tránh bị lãng quên trong truyện dài
// (gợi nhớ theo liên quan tự nhiên chỉ thấy tuyến liên quan đến chương này, không thấy tuyến treo một mình quá lâu).
// Tuổi là sự thật thuần code (chương hiện tại - chương cài), chỉ nêu "đã treo N chương chưa thu", không ra lệnh.
const foreshadowAgingChapters = 30

func longestCommonSubstringRunes(a, b []rune) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	prev := make([]int, len(b)+1)
	best := 0
	for i := 1; i <= len(a); i++ {
		curr := make([]int, len(b)+1)
		for j := 1; j <= len(b); j++ {
			if a[i-1] != b[j-1] {
				continue
			}
			curr[j] = prev[j-1] + 1
			if curr[j] > best {
				best = curr[j]
			}
		}
		prev = curr
	}
	return best
}

// truncateRunes cắt bớt chuỗi đến số rune chỉ định.
func truncateRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

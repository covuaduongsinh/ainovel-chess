package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

func TestContextToolInjectsStyleStats(t *testing.T) {
	dir := t.TempDir()
	st := store.NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	progress := &domain.Progress{TotalChapters: 10}
	body := "# Chuong N\nAnh khong do du, ma la so hai. Im lang mot luc. Nhu mot tia sang.\nBong dem buong xuong.\nAnh di roi."
	for ch := 1; ch <= 6; ch++ {
		if err := st.Drafts.SaveFinalChapter(ch, body); err != nil {
			t.Fatalf("SaveFinalChapter: %v", err)
		}
		progress.CompletedChapters = append(progress.CompletedChapters, ch)
	}
	if err := st.Progress.Save(progress); err != nil {
		t.Fatalf("Save progress: %v", err)
	}

	tool := NewContextTool(st, References{}, "default")
	args, _ := json.Marshal(map[string]any{"chapter": 7})
	raw, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload struct {
		Episodic map[string]json.RawMessage `json:"episodic_memory"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	statsRaw, ok := payload.Episodic["style_stats"]
	if !ok {
		t.Fatalf("expected episodic_memory.style_stats, got keys %v", keysOf(payload.Episodic))
	}
	var stats struct {
		Chapters int `json:"chapters"`
		Patterns []struct {
			Name  string `json:"name"`
			Total int    `json:"total"`
		} `json:"patterns"`
	}
	if err := json.Unmarshal(statsRaw, &stats); err != nil {
		t.Fatalf("Unmarshal stats: %v", err)
	}
	if stats.Chapters != 6 || len(stats.Patterns) == 0 {
		t.Errorf("stats content: %+v", stats)
	}
	if usage, ok := payload.Episodic["_usage"]; !ok || len(usage) == 0 {
		t.Error("expected episodic_memory._usage annotation")
	}
}

func keysOf(m map[string]json.RawMessage) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestContextToolReportsWarningsForCorruptedState(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "outline.json"), []byte("{invalid"), 0o644); err != nil {
		t.Fatalf("write outline.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta", "progress.json"), []byte("{invalid"), 0o644); err != nil {
		t.Fatalf("write progress.json: %v", err)
	}

	tool := NewContextTool(store, References{}, "default")
	args, err := json.Marshal(map[string]any{"chapter": 2})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload struct {
		Warnings []string `json:"_warnings"`
		Summary  string   `json:"_loading_summary"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(payload.Warnings) == 0 {
		t.Fatal("expected context warnings for corrupted files")
	}
	if !containsWarning(payload.Warnings, "outline") {
		t.Fatalf("expected outline warning, got %v", payload.Warnings)
	}
	if !containsWarning(payload.Warnings, "progress") {
		t.Fatalf("expected progress warning, got %v", payload.Warnings)
	}
	if !strings.Contains(payload.Summary, "canhBao:") {
		t.Fatalf("expected loading summary to contain warning count, got %q", payload.Summary)
	}
}

func containsWarning(warnings []string, key string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, key) {
			return true
		}
	}
	return false
}

func TestContextToolChapterModeIncludesWorkingAndReferenceFields(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Outline.SavePremise(`## Đề tài và tông điệu
Thiếu niên truong thanh, thiên ve cang thang ap buc.

## Định vị đề tài
The loai thang cap thieu nien

## Xung đột cốt lõi
Nhan vat chinh phai song sot trong cuoc canh tranh cua tong mon.

## Mục tiêu nhân vật chính
Gia nhap noi mon.

## Hướng kết cục
Tro thanh nguoi cam quan that su.

## Vùng cấm sáng tác
Khong tiet lo su that ve su phu som.

## Điểm bán khác biệt
Ke yeu phan bai.

## Móc câu khác biệt
Moi giai doan phai dung gia cao hon de doi lay su truong thanh.

## Cam kết cốt lõi
Lien tuc hien thuc hoa khung hoang va dot pha.

## Động cơ truyện
Thu thach, tranh gianh tai nguyen va nang cap danh tinh cung day chuyen.

## Bước ngoặt giữa truyện
Nhan vat chinh bi buoc chuyen sang mot con duong tu hanh khac.
`); err != nil {
		t.Fatalf("SavePremise: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "Nhap mon", CoreEvent: "Nhan vat chinh gia nhap tong mon", Scenes: []string{"배 su", "The nguyen"}},
		{Chapter: 2, Title: "Thu thach", CoreEvent: "Tham gia ky thi ngoai mon", Scenes: []string{"Tap hop", "Xuat phat"}},
	}); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}
	if err := s.Characters.Save([]domain.Character{
		{Name: "Lam Nghien", Role: "Nhan vat chinh", Description: "Thieu nien tu si", Arc: "Truong thanh", Traits: []string{"Diem tinh"}},
	}); err != nil {
		t.Fatalf("SaveCharacters: %v", err)
	}
	if err := s.World.SaveWorldRules([]domain.WorldRule{
		{Category: "magic", Rule: "Linh khi co the luyen hoa", Boundary: "Nguoi thuong khong the truc tiep dieu khien"},
	}); err != nil {
		t.Fatalf("SaveWorldRules: %v", err)
	}
	if err := s.Progress.Init("test", 2); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.Summaries.SaveSummary(domain.ChapterSummary{
		Chapter:    1,
		Summary:    "Nhan vat chinh gia nhap tong mon, xac lap muc tieu.",
		Characters: []string{"Lam Nghien"},
		KeyEvents:  []string{"배 su"},
	}); err != nil {
		t.Fatalf("SaveSummary: %v", err)
	}
	if err := s.Drafts.SaveFinalChapter(1, "Cuoi chuong mot, de lai huyem han thu thach."); err != nil {
		t.Fatalf("SaveFinalChapter: %v", err)
	}
	if err := s.Drafts.SaveChapterPlan(domain.ChapterPlan{
		Chapter: 2,
		Title:   "Thu thach",
		Goal:    "Vuot qua vong dau",
		Contract: domain.ChapterContract{
			RequiredBeats:    []string{"Phai de nhan vat chinh vuot qua vong dau", "Phai cai thu thach noi mon"},
			ForbiddenMoves:   []string{"Khong duoc tiet lo that su than phan su phu som"},
			ContinuityChecks: []string{"Vet thuong cu canh tay trai nhan vat chinh van chua lanh"},
			EvaluationFocus:  []string{"Uu tien kiem tra nhip do thu thach co le te khong"},
		},
	}); err != nil {
		t.Fatalf("SaveChapterPlan: %v", err)
	}
	if err := s.World.SaveStyleRules(domain.WritingStyleRules{
		Volume: 1,
		Arc:    1,
		Prose:  []string{"Ke chuyen giu su kiem che"},
	}); err != nil {
		t.Fatalf("SaveStyleRules: %v", err)
	}
	if err := s.RunMeta.SetPlanningTier(domain.PlanningTierLong); err != nil {
		t.Fatalf("SetPlanningTier: %v", err)
	}

	tool := NewContextTool(s, References{
		Consistency:      "Kiem tra nhat quan",
		HookTechniques:   "Ky thuat moc cau",
		QualityChecklist: "Danh sach kiem tra chat luong",
	}, "default")
	args, err := json.Marshal(map[string]any{"chapter": 2})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	for _, key := range []string{
		"premise",
		"premise_sections",
		"premise_structure",
		"outline",
		"world_rules",
		"memory_policy",
		"planning_tier",
		"working_memory",
		"episodic_memory",
		"reference_pack",
		"current_chapter_outline",
		"recent_summaries",
		"chapter_plan",
		"chapter_contract",
		"previous_tail",
		"style_rules",
		"references",
	} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("expected key %q in chapter context", key)
		}
	}
}

func TestContextToolArchitectModeIncludesPlanningAndFoundation(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Outline.SavePremise(`## Đề tài và tông điệu
Phieu luu quan chung, thiên ve su thi lanh lung.

## Định vị đề tài
Phieu luu quan chung truyen dai

## Xung đột cốt lõi
Moi nguoi phai tim trat tu moi trong trat tu cu lien tuc mat kiem soat.

## Mục tiêu nhân vật chính
Cham den trai tim cua su that.

## Hướng kết cục
Vach tran su that co dai va tai thiet trat tu.

## Vùng cấm sáng tác
Khong ket thuc bang thiet lap roi tren troi roi xuong.

## Điểm bán khác biệt
Quan he quan chung day tien trinh.

## Móc câu khác biệt
Moi tap deu thay doi cau truc quan he doi ngu.

## Cam kết cốt lõi
Lien tuc cung cap su kham pha, hy sinh va lua chon.

## Động cơ truyện
Tien trinh hanh trinh, dieu tra su that va quan he doi ngu cung thuc day.

## Tuyến quan hệ/trưởng thành
Doi ngu tu khong tin tuong nhau den chia re roi tai hop.

## Lộ trình thăng cấp
Tu su kien dia phuong tien den khung hoang cap the gioi.

## Bước ngoặt giữa truyện
Su that khong phai ke thu, ma chinh trat tu co van de.

## Mệnh đề kết cục
Trat tu nen do ai dinh nghia.
`); err != nil {
		t.Fatalf("SavePremise: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "Diem khoi dau", CoreEvent: "Hanh trinh bat dau"},
	}); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}
	if err := s.Characters.Save([]domain.Character{
		{Name: "Tram Dieu", Role: "Nhan vat chinh", Description: "Kiem khach lang thang", Arc: "Tim su that", Traits: []string{"Nhay ben"}},
	}); err != nil {
		t.Fatalf("SaveCharacters: %v", err)
	}
	if err := s.World.SaveWorldRules([]domain.WorldRule{
		{Category: "society", Rule: "Thanh bang nhan nhit", Boundary: "Hoang quyen khong truc tiep quan li bien dia"},
	}); err != nil {
		t.Fatalf("SaveWorldRules: %v", err)
	}
	if err := s.Outline.SaveLayeredOutline([]domain.VolumeOutline{
		{
			Index: 1, Title: "Tap mot", Theme: "Buoc len hanh trinh",
			Arcs: []domain.ArcOutline{
				{Index: 1, Title: "Len duong", Goal: "Xay dung doi ngu", Chapters: []domain.OutlineEntry{{Chapter: 1, Title: "Diem khoi dau"}}},
				{Index: 2, Title: "Suong mu", Goal: "Tiep can bi mat", EstimatedChapters: 5},
			},
		},
	}); err != nil {
		t.Fatalf("SaveLayeredOutline: %v", err)
	}
	if err := s.Outline.SaveCompass(domain.StoryCompass{
		EndingDirection: "Vach tran su that co dai",
		EstimatedScale:  "Du kien 3 tap",
	}); err != nil {
		t.Fatalf("SaveCompass: %v", err)
	}
	if err := s.World.SaveStyleRules(domain.WritingStyleRules{
		Volume: 1,
		Arc:    1,
		Prose:  []string{"Giu su lanh lung kiem che"},
	}); err != nil {
		t.Fatalf("SaveStyleRules: %v", err)
	}
	if err := s.RunMeta.SetPlanningTier(domain.PlanningTierLong); err != nil {
		t.Fatalf("SetPlanningTier: %v", err)
	}

	tool := NewContextTool(s, References{
		OutlineTemplate:   "Mau dan y",
		CharacterTemplate: "Mau nhan vat",
		LongformPlanning:  "Ke hoach truyen dai",
	}, "default")
	args, err := json.Marshal(map[string]any{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	for _, key := range []string{
		"memory_policy",
		"planning_tier",
		"planning_memory",
		"foundation_memory",
		"reference_pack",
		"premise_sections",
		"premise_structure",
		"characters",
		"layered_outline",
		"skeleton_arcs",
		"compass",
		"style_rules",
		"references",
		"foundation_status",
	} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("expected key %q in architect context", key)
		}
	}
}

func TestTrimByBudgetRemovesMirroredMemoryKeys(t *testing.T) {
	result := map[string]any{
		"references": map[string]string{
			"a": strings.Repeat("x", 200),
			"b": strings.Repeat("y", 200),
		},
		"reference_pack": map[string]any{
			"references": map[string]string{
				"a": strings.Repeat("x", 200),
				"b": strings.Repeat("y", 200),
			},
			"style_rules": []string{"Kiem che"},
		},
	}

	trimByBudget(result, 80)

	if _, ok := result["references"]; ok {
		t.Fatal("expected top-level references to be trimmed")
	}
	pack, ok := result["reference_pack"].(map[string]any)
	if !ok {
		t.Fatal("expected reference_pack to remain available")
	}
	if _, ok := pack["references"]; ok {
		t.Fatal("expected mirrored references to be trimmed from reference_pack")
	}
}

func TestContextToolSelectedMemoryRecallsStoryThreadsAndReviewLessons(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "Loi moi", CoreEvent: "Truong lao bi mat dua ra loi moi thu thach noi mon", Scenes: []string{"Dam thoai bi mat", "De lai loi moi thu thach"}},
		{Chapter: 2, Title: "Dem truoc thu thach", CoreEvent: "Lam Nghien chuan bi hoi dap loi moi thu thach noi mon", Hook: "Ai la nguoi dung sau thuc day cuoc thu thach nay", Scenes: []string{"Sap xep manh moi", "Quyet dinh nhan loi"}},
	}); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}
	if err := s.Progress.Init("test", 8); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.World.SaveForeshadowLedger([]domain.ForeshadowEntry{
		{ID: "trial_invite", Description: "Muc dich that su cua loi moi thu thach noi mon", PlantedAt: 1, Status: "planted"},
		{ID: "trial_mastermind", Description: "Ai la nguoi dung sau thuc day cuoc thu thach nay", PlantedAt: 1, Status: "planted"},
		{ID: "trial_rules", Description: "Manh van bia quy tac thu thach con sot lai", PlantedAt: 1, Status: "planted"},
		{ID: "outer_disciple", Description: "Mon no cu cua de tu ngoai mon", PlantedAt: 1, Status: "planted"},
		{ID: "elder_token", Description: "Nguon goc the bai trong tay truong lao", PlantedAt: 1, Status: "planted"},
		{ID: "hidden_gate", Description: "Con duong bi mat phia sau cong mon", PlantedAt: 1, Status: "planted"},
		{ID: "trial_bet", Description: "Nguoi dieu phoi thu cuoc thu thach ngam", PlantedAt: 1, Status: "planted"},
	}); err != nil {
		t.Fatalf("SaveForeshadowLedger: %v", err)
	}
	if err := s.Drafts.SaveChapterPlan(domain.ChapterPlan{
		Chapter: 2,
		Title:   "Dem truoc thu thach",
		Goal:    "Quyet dinh co hoi dap loi moi hay khong",
		Contract: domain.ChapterContract{
			PayoffPoints: []string{"Hoi dap loi moi thu thach noi mon"},
			HookGoal:     "Tung ra cau hoi ai dung sau thuc day thu thach",
		},
	}); err != nil {
		t.Fatalf("SaveChapterPlan: %v", err)
	}
	if err := s.World.SaveReview(domain.ReviewEntry{
		Chapter:        1,
		Scope:          "chapter",
		Verdict:        "polish",
		Summary:        "Tuyen chinh da khoi dong, nhung phuc but chua ro rang.",
		ContractStatus: "partial",
		ContractMisses: []string{"Chua ro rang cai thu thach noi mon"},
		Issues: []domain.ConsistencyIssue{
			{Type: "hook", Severity: "warning", Description: "Moc cau cuoi chuong chua cu the"},
		},
	}); err != nil {
		t.Fatalf("SaveReview: %v", err)
	}

	tool := NewContextTool(s, References{}, "default")
	args, err := json.Marshal(map[string]any{"chapter": 2})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload struct {
		Selected struct {
			StoryThreads  []domain.RecallItem `json:"story_threads"`
			ReviewLessons []domain.RecallItem `json:"review_lessons"`
		} `json:"selected_memory"`
		Summary string `json:"_loading_summary"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(payload.Selected.StoryThreads) == 0 {
		t.Fatal("expected story thread recall items")
	}
	if len(payload.Selected.ReviewLessons) == 0 {
		t.Fatal("expected review lesson recall items")
	}
	if !containsRecallSummary(payload.Selected.StoryThreads, "loi moi thu thach noi mon") {
		t.Fatalf("expected story thread recall to mention invite, got %+v", payload.Selected.StoryThreads)
	}
	if !containsRecallSummary(payload.Selected.StoryThreads, "thuc day cuoc thu thach nay") {
		t.Fatalf("expected story thread recall to mention trial mastermind, got %+v", payload.Selected.StoryThreads)
	}
	if containsRecallSummary(payload.Selected.StoryThreads, "Manh van bia quy tac thu thach con sot lai") {
		t.Fatalf("expected weak-overlap foreshadow to stay out, got %+v", payload.Selected.StoryThreads)
	}
	if containsRecallSummary(payload.Selected.StoryThreads, "de xuat xem lai chuong") {
		t.Fatalf("expected related_chapters not to be duplicated into story_threads, got %+v", payload.Selected.StoryThreads)
	}
	if !containsRecallSummary(payload.Selected.ReviewLessons, "thieu contract") {
		t.Fatalf("expected review lesson recall to mention contract miss, got %+v", payload.Selected.ReviewLessons)
	}
	if !strings.Contains(payload.Summary, "goiLaiCotTruyen:") || !strings.Contains(payload.Summary, "goiLaiThamDinh:") {
		t.Fatalf("expected loading summary to report selected memory, got %q", payload.Summary)
	}
}

// Phuc but treo lau chua thu du khong lien quan den tu khoa chuong hien tai van nen duoc bo sung vao story_threads--
// day chinh la diem mu cua goi lai theo lien quan (tuyen treo mot minh qua lau ma khong khop tu khoa o chuong nay).
// Phuc but cai gan day (tuoi < nguong) khong nen bi nhan lam "chua thu".
func TestContextToolSelectedMemorySurfacesAgingForeshadow(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Chu de chuong hien tai khong lien quan den bat ky phuc but nao, dam bao goi lai theo lien quan rong, chi con bo sung theo tuoi gio hieu luc.
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 50, Title: "Dich benh", CoreEvent: "Lam Nghien chua tri benh nhan dich benh tai y quan phia nam thanh", Scenes: []string{"Sac thuoc", "Phong toa duong pho"}},
	}); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}
	if err := s.Progress.Init("test", 60); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	// 6 muc du nguong goi lai; hai muc dau tuoi >=30 (treo lau), bon muc sau tuoi <30 (gan day).
	if err := s.World.SaveForeshadowLedger([]domain.ForeshadowEntry{
		{ID: "ancient_seal", Description: "Vet nut cua con dau co dai", PlantedAt: 3, Status: "planted"},
		{ID: "lost_bloodline", Description: "Nguon goc huyet mach mat tich cua nhan vat chinh", PlantedAt: 5, Status: "advanced"},
		{ID: "market_feud", Description: "Cai co o cho dem qua", PlantedAt: 47, Status: "planted"},
		{ID: "rumor_a", Description: "Tin don gan day A", PlantedAt: 48, Status: "planted"},
		{ID: "rumor_b", Description: "Tin don gan day B", PlantedAt: 48, Status: "planted"},
		{ID: "rumor_c", Description: "Tin don gan day C", PlantedAt: 49, Status: "planted"},
	}); err != nil {
		t.Fatalf("SaveForeshadowLedger: %v", err)
	}

	tool := NewContextTool(s, References{}, "default")
	args, err := json.Marshal(map[string]any{"chapter": 50})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload struct {
		Selected struct {
			StoryThreads []domain.RecallItem `json:"story_threads"`
		} `json:"selected_memory"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Hai phuc but treo lau nen duoc bo sung, kem chu thich tuoi "chua thu".
	if !containsRecallSummary(payload.Selected.StoryThreads, "Vet nut cua con dau co dai") {
		t.Fatalf("expected aging foreshadow to surface despite no relevance, got %+v", payload.Selected.StoryThreads)
	}
	if !containsRecallSummary(payload.Selected.StoryThreads, "huyet mach mat tich") {
		t.Fatalf("expected second aging foreshadow to surface, got %+v", payload.Selected.StoryThreads)
	}
	if !containsRecallSummary(payload.Selected.StoryThreads, "chua thu") {
		t.Fatalf("expected aging item to carry overdue annotation, got %+v", payload.Selected.StoryThreads)
	}
	// Phuc but gan day (tuoi <30 va khong lien quan) khong nen duoc bo sung.
	if containsRecallSummary(payload.Selected.StoryThreads, "Cai co o cho dem qua") {
		t.Fatalf("recent foreshadow must not be labeled overdue, got %+v", payload.Selected.StoryThreads)
	}
}

func TestContextToolSelectedMemoryIncludesGlobalReviewLessons(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "Mo dau", CoreEvent: "Cau chuyen bat dau"},
		{Chapter: 2, Title: "Day tien", CoreEvent: "Tuyen chinh tiep tuc tien trien"},
	}); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}
	if err := s.Progress.Init("test", 6); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.World.SaveReview(domain.ReviewEntry{
		Chapter: 1,
		Scope:   "global",
		Verdict: "polish",
		Summary: "Toan cuon tien trien dat yeu cau, nhung bieu dat muc tieu nhan vat van chua on dinh.",
		Issues: []domain.ConsistencyIssue{
			{Type: "character", Severity: "warning", Description: "Bieu dat muc tieu nhan vat chinh chua on dinh"},
		},
	}); err != nil {
		t.Fatalf("SaveReview(global): %v", err)
	}

	tool := NewContextTool(s, References{}, "default")
	args, err := json.Marshal(map[string]any{"chapter": 2})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload struct {
		Selected struct {
			ReviewLessons []domain.RecallItem `json:"review_lessons"`
		} `json:"selected_memory"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !containsRecallSummary(payload.Selected.ReviewLessons, "Bieu dat muc tieu nhan vat chinh chua on dinh") {
		t.Fatalf("expected global review lesson to be recalled, got %+v", payload.Selected.ReviewLessons)
	}
}

func TestContextToolKeepsFullForeshadowWhenRecallNotTriggered(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "Mo man", CoreEvent: "Cau chuyen mo man"},
		{Chapter: 2, Title: "Day tien", CoreEvent: "Tiep tuc tien trien"},
	}); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}
	if err := s.Progress.Init("test", 4); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.World.SaveForeshadowLedger([]domain.ForeshadowEntry{
		{ID: "small_1", Description: "Phuc but nho thu nhat", PlantedAt: 1, Status: "planted"},
		{ID: "small_2", Description: "Phuc but nho thu hai", PlantedAt: 1, Status: "planted"},
	}); err != nil {
		t.Fatalf("SaveForeshadowLedger: %v", err)
	}

	tool := NewContextTool(s, References{}, "default")
	args, err := json.Marshal(map[string]any{"chapter": 2})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := payload["foreshadow_ledger"]; !ok {
		t.Fatal("expected full foreshadow ledger to remain when selected recall is not triggered")
	}
	if _, ok := payload["selected_memory"]; ok {
		t.Fatalf("expected no selected_memory for small foreshadow sets, got %+v", payload["selected_memory"])
	}
}

func TestContextToolFallsBackToFullForeshadowWhenSelectionIsTooSparse(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "Loi moi", CoreEvent: "Truong lao bi mat dua ra loi moi thu thach noi mon"},
		{Chapter: 2, Title: "Dem truoc thu thach", CoreEvent: "Lam Nghien chuan bi hoi dap loi moi thu thach noi mon", Scenes: []string{"Sap xep manh moi", "Quyet dinh nhan loi"}},
	}); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}
	if err := s.Progress.Init("test", 8); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.World.SaveForeshadowLedger([]domain.ForeshadowEntry{
		{ID: "trial_invite", Description: "Muc dich that su cua loi moi thu thach noi mon", PlantedAt: 1, Status: "planted"},
		{ID: "trial_rules", Description: "Manh van bia quy tac thu thach con sot lai", PlantedAt: 1, Status: "planted"},
		{ID: "outer_disciple", Description: "Mon no cu cua de tu ngoai mon", PlantedAt: 1, Status: "planted"},
		{ID: "elder_token", Description: "Nguon goc the bai trong tay truong lao", PlantedAt: 1, Status: "planted"},
		{ID: "hidden_gate", Description: "Con duong bi mat phia sau cong mon", PlantedAt: 1, Status: "planted"},
		{ID: "trial_bet", Description: "Nguoi dieu phoi thu cuoc thu thach ngam", PlantedAt: 1, Status: "planted"},
	}); err != nil {
		t.Fatalf("SaveForeshadowLedger: %v", err)
	}

	tool := NewContextTool(s, References{}, "default")
	args, err := json.Marshal(map[string]any{"chapter": 2})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := payload["foreshadow_ledger"]; !ok {
		t.Fatal("expected full foreshadow ledger when selection is too sparse")
	}
	if selected, ok := payload["selected_memory"].(map[string]any); ok {
		if _, exists := selected["story_threads"]; exists {
			t.Fatalf("expected sparse story_threads to fall back to full ledger, got %+v", selected["story_threads"])
		}
	}
}

func containsRecallSummary(items []domain.RecallItem, want string) bool {
	for _, item := range items {
		if strings.Contains(item.Summary, want) {
			return true
		}
	}
	return false
}

func TestContextToolInjectsRewriteBriefForPendingRewriteChapter(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.Progress.MarkChapterComplete(2, 3000, "", ""); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}
	if err := s.Progress.SetPendingRewrites([]int{2}, "Nhip do le te, can nen la nua dau"); err != nil {
		t.Fatalf("SetPendingRewrites: %v", err)
	}
	if err := s.World.SaveReview(domain.ReviewEntry{
		Chapter: 2,
		Scope:   "chapter",
		Verdict: "rewrite",
		Summary: "Nua dau trai qua dai, xung dot mai khong xuat hien.",
		Issues: []domain.ConsistencyIssue{
			{Type: "pacing", Severity: "error", Description: "2000 chu dau khong co tien trien"},
		},
		ContractMisses: []string{"Chua thuc hien canh mo dau thu thach"},
	}); err != nil {
		t.Fatalf("SaveReview: %v", err)
	}

	tool := NewContextTool(s, References{}, "default")
	args, err := json.Marshal(map[string]any{"chapter": 2})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	brief, ok := payload["rewrite_brief"].(map[string]any)
	if !ok {
		t.Fatalf("expected rewrite_brief in chapter context, got %T", payload["rewrite_brief"])
	}
	if got := brief["reason"]; got != "Nhip do le te, can nen la nua dau" {
		t.Fatalf("expected rewrite reason, got %v", got)
	}
	if got, _ := brief["review_summary"].(string); !strings.Contains(got, "trai qua dai") {
		t.Fatalf("expected review summary from chapter review, got %v", brief["review_summary"])
	}
	if issues, _ := brief["issues"].([]any); len(issues) == 0 {
		t.Fatalf("expected review issues in rewrite_brief, got %v", brief["issues"])
	}
	if misses, _ := brief["contract_misses"].([]any); len(misses) == 0 {
		t.Fatalf("expected contract misses in rewrite_brief, got %v", brief["contract_misses"])
	}
}

func TestContextToolOmitsRewriteBriefForNormalChapter(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	tool := NewContextTool(s, References{}, "default")
	args, err := json.Marshal(map[string]any{"chapter": 2})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := payload["rewrite_brief"]; ok {
		t.Fatal("expected no rewrite_brief for chapter outside PendingRewrites")
	}
}

func TestContextToolDoesNotInjectUserDirectives(t *testing.T) {
	// save_directive da duoc xoa: novel_context khong con tiem working_memory.user_directives nua,
	// cac yeu cau viet lau dai duoc thong nhat qua user_rules. Khoa cung dieu nay, ngan hoi quy.
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	tool := NewContextTool(s, References{}, "default")
	for name, chapter := range map[string]int{"writer": 1, "architect": 0} {
		args, _ := json.Marshal(map[string]any{"chapter": chapter})
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("[%s] Execute: %v", name, err)
		}
		var payload map[string]any
		if err := json.Unmarshal(result, &payload); err != nil {
			t.Fatalf("[%s] Unmarshal: %v", name, err)
		}
		working, ok := payload["working_memory"].(map[string]any)
		if !ok {
			t.Fatalf("[%s] missing working_memory", name)
		}
		if _, exists := working["user_directives"]; exists {
			t.Errorf("[%s] working_memory khong nen con user_directives (da thong nhat vao user_rules)", name)
		}
		// user_rules van nen duoc tiem on dinh
		if _, ok := working["user_rules"].(map[string]any); !ok {
			t.Errorf("[%s] working_memory.user_rules nen duoc tiem on dinh", name)
		}
	}
}

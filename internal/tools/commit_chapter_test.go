package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

func TestCommitChapterSchemaDescribesFeedbackAsObject(t *testing.T) {
	tool := NewCommitChapterTool(store.NewStore(t.TempDir()))
	schema := tool.Schema()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties missing: %#v", schema["properties"])
	}
	feedback, ok := props["feedback"].(map[string]any)
	if !ok {
		t.Fatalf("feedback schema missing: %#v", props["feedback"])
	}
	desc, _ := feedback["description"].(string)
	if !strings.Contains(desc, "JSON object") || !strings.Contains(desc, "JSON dạng chuỗi") {
		t.Fatalf("feedback description should warn against stringified JSON, got %q", desc)
	}
	if got := feedback["type"]; got != "object" {
		t.Fatalf("feedback type = %v, want object", got)
	}
}

func TestCommitChapterRejectsNonPendingRewrite(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := store.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := store.Progress.MarkChapterComplete(2, 3000, "", ""); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}
	if err := store.Progress.SetPendingRewrites([]int{2}, "Kiem tra viet lai"); err != nil {
		t.Fatalf("SetPendingRewrites: %v", err)
	}
	if err := store.Progress.SetFlow(domain.FlowRewriting); err != nil {
		t.Fatalf("SetFlow: %v", err)
	}
	if err := store.Drafts.SaveDraft(3, "Day la noi dung chuong sai."); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	tool := NewCommitChapterTool(store)
	args, err := json.Marshal(map[string]any{
		"chapter":         3,
		"summary":         "Nop sai",
		"characters":      []string{"Nhan vat chinh"},
		"key_events":      []string{"Nop nham"},
		"timeline_events": []any{},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Fatal("expected commit to be rejected during rewrite flow")
	}

	if _, err := os.Stat(dir + "/chapters/03.md"); !os.IsNotExist(err) {
		t.Fatalf("chapter should not be persisted, stat err=%v", err)
	}

	progress, err := store.Progress.Load()
	if err != nil {
		t.Fatalf("LoadProgress: %v", err)
	}
	if len(progress.CompletedChapters) != 1 || progress.CompletedChapters[0] != 2 {
		t.Fatalf("completed chapters should only contain original chapter 2, got %v", progress.CompletedChapters)
	}
	if progress.CurrentChapter != 3 {
		t.Fatalf("current chapter should not advance beyond original progress, got %d", progress.CurrentChapter)
	}
}

func TestCommitChapterAllowsPendingRewrite(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := store.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := store.Progress.MarkChapterComplete(2, 3000, "", ""); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}
	if err := store.Progress.SetPendingRewrites([]int{2}, "Kiem tra viet lai"); err != nil {
		t.Fatalf("SetPendingRewrites: %v", err)
	}
	if err := store.Progress.SetFlow(domain.FlowRewriting); err != nil {
		t.Fatalf("SetFlow: %v", err)
	}
	if err := store.Drafts.SaveDraft(2, "Day la noi dung chuong dung can viet lai."); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	tool := NewCommitChapterTool(store)
	args, err := json.Marshal(map[string]any{
		"chapter":         2,
		"summary":         "Nop dung",
		"characters":      []string{"Nhan vat chinh"},
		"key_events":      []string{"Hoan thanh viet lai"},
		"timeline_events": []any{},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if _, err := os.Stat(dir + "/chapters/02.md"); err != nil {
		t.Fatalf("chapter should be persisted: %v", err)
	}

	progress, err := store.Progress.Load()
	if err != nil {
		t.Fatalf("LoadProgress: %v", err)
	}
	if len(progress.CompletedChapters) != 1 || progress.CompletedChapters[0] != 2 {
		t.Fatalf("unexpected completed chapters: %v", progress.CompletedChapters)
	}
	pending, err := store.Signals.LoadPendingCommit()
	if err != nil {
		t.Fatalf("LoadPendingCommit: %v", err)
	}
	if pending != nil {
		t.Fatalf("expected pending commit cleared, got %+v", pending)
	}
}

// TestCommitChapterUpdatesCastLedger xac minh: commit_chapter tich luy characters cua chuong nay vao cast_ledger,
// brief_role do cast_intros cung cap duoc su dung, va cac nhan vat chinh trong characters.json khong vao ledger.
func TestCommitChapterUpdatesCastLedger(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	// Thiet lap ho so nhan vat chinh (nhung nhan vat nay khong nen vao cast_ledger)
	if err := s.Characters.Save([]domain.Character{
		{Name: "Lam Mo", Role: "Nhan vat chinh", Tier: "core"},
		{Name: "Le Thanh Nghien", Role: "Su phu", Tier: "important"},
	}); err != nil {
		t.Fatalf("Save core characters: %v", err)
	}
	if err := s.Drafts.SaveDraft(1, "Noi dung chuong mot, Lam Mo gap chu quan tro Lao Chau va cau boi A Van."); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	tool := NewCommitChapterTool(s)
	args, _ := json.Marshal(map[string]any{
		"chapter":    1,
		"summary":    "Lam Mo vao tro",
		"characters": []string{"Lam Mo", "Le Thanh Nghien", "Lao Chau", "A Van"},
		"key_events": []string{"Vao tro"},
		"cast_intros": []any{
			map[string]any{"name": "Lao Chau", "brief_role": "Chu quan tro"},
			map[string]any{"name": "A Van", "brief_role": "Cau boi quan tro"},
		},
	})
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	entries, err := s.Cast.Load()
	if err != nil {
		t.Fatalf("Cast.Load: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 ledger entries (Lao Chau/A Van), got %d: %+v", len(entries), entries)
	}
	byName := map[string]domain.CastEntry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	if e, ok := byName["Lao Chau"]; !ok || e.BriefRole != "Chu quan tro" || e.FirstSeenChapter != 1 {
		t.Errorf("Lao Chau entry wrong: %+v", e)
	}
	if e, ok := byName["A Van"]; !ok || e.BriefRole != "Cau boi quan tro" || e.AppearanceCount != 1 {
		t.Errorf("A Van entry wrong: %+v", e)
	}
	if _, ok := byName["Lam Mo"]; ok {
		t.Errorf("Nhan vat chinh Lam Mo khong nen vao ledger")
	}
	if _, ok := byName["Le Thanh Nghien"]; ok {
		t.Errorf("Nhan vat chinh Le Thanh Nghien khong nen vao ledger")
	}
}

func TestCommitChapterReplayAfterPartialCommitDoesNotDuplicateWorldState(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.Drafts.SaveDraft(1, "Noi dung chuong mot, Lam Mo gap bong toi va dot pha."); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	timeline := []domain.TimelineEvent{{
		Chapter:    1,
		Time:       "Sang som",
		Event:      "Lam Mo gap bong toi",
		Characters: []string{"Lam Mo"},
	}}
	stateChanges := []domain.StateChange{{
		Chapter:  1,
		Entity:   "Lam Mo",
		Field:    "realm",
		OldValue: "Nguoi thuong",
		NewValue: "Luyen khi ky",
	}}
	foreshadow := []domain.ForeshadowUpdate{{
		ID:          "f1",
		Action:      "plant",
		Description: "Than phan bong toi",
	}}

	// Mo phong commit_chapter da ghi vao trang thai the gioi nhung tien trinh gap su co truoc khi MarkChapterComplete.
	if err := s.World.AppendTimelineEvents(timeline); err != nil {
		t.Fatalf("AppendTimelineEvents seed: %v", err)
	}
	if err := s.World.AppendStateChanges(stateChanges); err != nil {
		t.Fatalf("AppendStateChanges seed: %v", err)
	}
	if err := s.World.UpdateForeshadow(1, foreshadow); err != nil {
		t.Fatalf("UpdateForeshadow seed: %v", err)
	}
	if err := s.Signals.SavePendingCommit(domain.PendingCommit{
		Chapter: 1,
		Stage:   domain.CommitStageStateApplied,
		Summary: "Tom tat nua chung",
	}); err != nil {
		t.Fatalf("SavePendingCommit: %v", err)
	}

	tool := NewCommitChapterTool(s)
	args, _ := json.Marshal(map[string]any{
		"chapter":            1,
		"summary":            "Lam Mo gap bong toi va dot pha",
		"characters":         []string{"Lam Mo"},
		"key_events":         []string{"Gap bong toi", "Dot pha"},
		"timeline_events":    timeline,
		"state_changes":      stateChanges,
		"foreshadow_updates": foreshadow,
	})
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute replay: %v", err)
	}

	events, _ := s.World.LoadTimeline()
	if len(events) != 1 {
		t.Fatalf("timeline duplicated after replay, got %d: %+v", len(events), events)
	}
	changes, _ := s.World.LoadStateChanges()
	if len(changes) != 1 {
		t.Fatalf("state changes duplicated after replay, got %d: %+v", len(changes), changes)
	}
	ledger, _ := s.World.LoadForeshadowLedger()
	if len(ledger) != 1 {
		t.Fatalf("foreshadow duplicated after replay, got %d: %+v", len(ledger), ledger)
	}
	pending, _ := s.Signals.LoadPendingCommit()
	if pending != nil {
		t.Fatalf("pending commit should be cleared, got %+v", pending)
	}
	if cp := s.Checkpoints.LatestByStep(domain.ChapterScope(1), "commit"); cp == nil {
		t.Fatal("commit checkpoint should be written")
	}
}

// TestCommitChapterRejectsPolishWithoutDraftChange xac minh: sau khi chuong da hoan thanh vao hang doi danh bong/viet lai,
// neu writer bo qua draft_chapter va nop thang (noi dung drafts va chapters hoan toan giong nhau),
// commit_chapter phai tu choi, buoc writer goi draft_chapter truoc de ghi phien ban moi.
// TestCommitChapterNonLayeredRecompletesAfterRework xac minh sach phi phan tang sau khi hoan thanh va reopen de lam lai,
// khi nop chuong xong va hang doi trong co the tu dong quay lai trang thai complete (nhanh phi phan tang sau khi bo sung drain va ket thuc).
func TestCommitChapterNonLayeredRecompletesAfterRework(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 2); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	// Hai chuong viet xong va hoan thanh. Chuong 2 co du drafts/chapters, san sang cho nop lai.
	ch2 := "Noi dung goc chuong hai, dung de mo phong ban thao da nop."
	if err := s.Drafts.SaveDraft(2, ch2); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	if err := s.Drafts.SaveFinalChapter(2, ch2); err != nil {
		t.Fatalf("SaveFinalChapter: %v", err)
	}
	if err := s.Progress.MarkChapterComplete(1, 100, "", ""); err != nil {
		t.Fatalf("MarkChapterComplete(1): %v", err)
	}
	if err := s.Progress.MarkChapterComplete(2, len([]rune(ch2)), "", ""); err != nil {
		t.Fatalf("MarkChapterComplete(2): %v", err)
	}
	if err := s.Progress.MarkComplete(); err != nil {
		t.Fatalf("MarkComplete: %v", err)
	}

	// reopen chuong 2 → phase quay ve writing, PendingRewrites=[2], flow=rewriting
	if err := s.Progress.Reopen([]int{2}, "Lam lai"); err != nil {
		t.Fatalf("Reopen: %v", err)
	}

	// Nop lai (ban nhap can khac ban thao cuoi moi duoc chap nhan)
	if err := s.Drafts.SaveDraft(2, ch2+"\n\nDoan them moi sau khi lam lai."); err != nil {
		t.Fatalf("SaveDraft (reworked): %v", err)
	}
	tool := NewCommitChapterTool(s)
	args, _ := json.Marshal(map[string]any{
		"chapter":    2,
		"summary":    "Tom tat sau khi lam lai",
		"characters": []string{"Nhan vat chinh"},
		"key_events": []string{"Don dep"},
	})
	raw, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute rework commit: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload["book_complete"] != true {
		t.Errorf("book_complete = %v, want true", payload["book_complete"])
	}

	p, _ := s.Progress.Load()
	if p.Phase != domain.PhaseComplete {
		t.Errorf("phase = %s, want complete (nen tu dong ket lai)", p.Phase)
	}
	if len(p.PendingRewrites) != 0 {
		t.Errorf("PendingRewrites = %v, want empty", p.PendingRewrites)
	}
}

// TestCommitChapterLayeredReopenRecompletesDespiteOpenThread xac minh ket thuc: sach phan tang sau khi reopen
// lam lai, du compass van con long tuyen chua ket (lam lai co the gay nhieu loan), sau khi hang doi trong van hoan thanh theo "cau truc day du"--
// khong bi ket o writing, ngan vong lap viet tran bien gioi cuoi tap cuoi (gia dinh §6.5 / known_outline_exhaustion).
// Phan chung: neu duong dan reopen van dung layeredBookComplete cap chat luong, open thread se tra false,
// book_complete la false, kiem thu se that bai.
func TestCommitChapterLayeredReopenRecompletesDespiteOpenThread(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	// Mot tap mot cung hai chuong, tat ca da trien khai
	foundation := NewSaveFoundationTool(s)
	layeredArgs, _ := json.Marshal(map[string]any{
		"type": "layered_outline",
		"content": []map[string]any{{
			"index": 1, "title": "Tap mot", "theme": "Chu de",
			"arcs": []map[string]any{{
				"index": 1, "title": "Cung mot", "goal": "Muc tieu",
				"chapters": []map[string]any{
					{"title": "Chuong dau", "core_event": "Mo dau", "hook": "Tiep theo"},
					{"title": "Chuong hai", "core_event": "Phat trien", "hook": "Ket thuc"},
				},
			}},
		}},
		"scale": "long",
	})
	if _, err := foundation.Execute(context.Background(), layeredArgs); err != nil {
		t.Fatalf("Execute layered: %v", err)
	}

	// Hai chuong viet xong luu dia va hoan thanh
	ch2 := "Noi dung goc chuong hai, mo phong ban thao da nop."
	for ch, body := range map[int]string{1: "Noi dung chuong mot.", 2: ch2} {
		if err := s.Drafts.SaveDraft(ch, body); err != nil {
			t.Fatalf("SaveDraft %d: %v", ch, err)
		}
		if err := s.Drafts.SaveFinalChapter(ch, body); err != nil {
			t.Fatalf("SaveFinalChapter %d: %v", ch, err)
		}
		if err := s.Progress.MarkChapterComplete(ch, len([]rune(body)), "", ""); err != nil {
			t.Fatalf("MarkChapterComplete %d: %v", ch, err)
		}
	}
	if err := s.Progress.MarkComplete(); err != nil {
		t.Fatalf("MarkComplete: %v", err)
	}

	// Mo phong "lam lai lam xao tron long tuyen": compass van con open thread chua ket
	if err := s.Outline.SaveCompass(domain.StoryCompass{EndingDirection: "Nhan vat chinh ve que", OpenThreads: []string{"Ke thu chua bi tieu diet"}}); err != nil {
		t.Fatalf("SaveCompass: %v", err)
	}

	// reopen chuong 2 → nop lai (ban nhap can khac ban thao cuoi moi duoc chap nhan)
	if err := s.Progress.Reopen([]int{2}, "Lam lai"); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if err := s.Drafts.SaveDraft(2, ch2+"\n\nDoan them moi sau khi lam lai."); err != nil {
		t.Fatalf("SaveDraft reworked: %v", err)
	}
	tool := NewCommitChapterTool(s)
	args, _ := json.Marshal(map[string]any{
		"chapter": 2, "summary": "Tom tat lam lai", "characters": []string{"Nhan vat chinh"}, "key_events": []string{"Don dep"},
	})
	raw, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute rework commit: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if bc, _ := out["book_complete"].(bool); !bc {
		t.Error("reopen lam lai hang doi trong nen hoan thanh theo cau truc day du (du long tuyen chua ket)")
	}
	p, _ := s.Progress.Load()
	if p.Phase != domain.PhaseComplete {
		t.Errorf("phase = %s, want complete", p.Phase)
	}
	if p.ReopenedFromComplete {
		t.Error("Sau khi hoan thanh lai ReopenedFromComplete nen duoc xoa")
	}
}

func TestCommitChapterRejectsPolishWithoutDraftChange(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	// Mo phong chuong 2 da hoan thanh binh thuong: noi dung drafts va chapters giong nhau.
	original := "Noi dung goc chuong hai, dung de mo phong ban thao da nop."
	if err := s.Drafts.SaveDraft(2, original); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	if err := s.Drafts.SaveFinalChapter(2, original); err != nil {
		t.Fatalf("SaveFinalChapter: %v", err)
	}
	if err := s.Progress.MarkChapterComplete(2, len([]rune(original)), "mystery", "quest"); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}

	// Vao hang doi danh bong: Flow=Polishing, PendingRewrites=[2]
	if err := s.Progress.SetPendingRewrites([]int{2}, "Kiem tra danh bong"); err != nil {
		t.Fatalf("SetPendingRewrites: %v", err)
	}
	if err := s.Progress.SetFlow(domain.FlowPolishing); err != nil {
		t.Fatalf("SetFlow: %v", err)
	}

	tool := NewCommitChapterTool(s)
	args, _ := json.Marshal(map[string]any{
		"chapter":    2,
		"summary":    "Gia vo da danh bong",
		"characters": []string{"Nhan vat chinh"},
		"key_events": []string{"Khong thay doi"},
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected commit to be rejected when drafts equals final content")
	}

	// Viet them mot phien ban ban nhap khac → nen duoc chap nhan
	polished := original + "\n\nDoan them moi sau khi danh bong."
	if err := s.Drafts.SaveDraft(2, polished); err != nil {
		t.Fatalf("SaveDraft (polished): %v", err)
	}
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute after real polish: %v", err)
	}
}

// TestCommitChapterLayeredRejectsOutOfRangeChapter xac minh trong che do phan tang,
// commit voi so chuong vuot ngoai pham vi layered_outline phai that bai cung, khong the su dung slog.Warn cho qua.
// Day la phanh vat ly ngan "writer chay tran sau khi phan quyet sai" (vu "Fan Gu" ch204..347).
func TestCommitChapterLayeredRejectsOutOfRangeChapter(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	// Tao mot layered_outline chi co 1 tap 1 cung 1 chuong
	foundation := NewSaveFoundationTool(s)
	layeredArgs, _ := json.Marshal(map[string]any{
		"type": "layered_outline",
		"content": []map[string]any{{
			"index": 1, "title": "Tap mot", "theme": "Chu de",
			"arcs": []map[string]any{{
				"index": 1, "title": "Cung mot", "goal": "Muc tieu",
				"chapters": []map[string]any{
					{"title": "Chuong dau", "core_event": "Mo dau", "hook": "Tiep theo"},
				},
			}},
		}},
		"scale": "long",
	})
	if _, err := foundation.Execute(context.Background(), layeredArgs); err != nil {
		t.Fatalf("Execute layered: %v", err)
	}
	_ = s.Progress.UpdatePhase(domain.PhaseWriting)

	// Commit chuong tran bien 2 phai that bai cung
	if err := s.Drafts.SaveDraft(2, "Noi dung chuong tran bien, phai bi chan lai."); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	tool := NewCommitChapterTool(s)
	args, _ := json.Marshal(map[string]any{
		"chapter":    2,
		"summary":    "Chuong tran bien",
		"characters": []string{"Nhan vat chinh"},
		"key_events": []string{"Khong duoc phep"},
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected commit to fail when chapter out of layered outline range")
	}

	// File chuong khong nen duoc luu dia, Progress khong nen tien
	if _, statErr := os.Stat(dir + "/chapters/02.md"); !os.IsNotExist(statErr) {
		t.Fatalf("chapter 2 should not be persisted, stat err=%v", statErr)
	}
	progress, _ := s.Progress.Load()
	if len(progress.CompletedChapters) != 0 {
		t.Fatalf("CompletedChapters should stay empty, got %v", progress.CompletedChapters)
	}
}

// TestCommitChapterLayeredAutoCompletesWhenDone xac minh nen ket thuc xac dinh tinh phan tang:
// Khi toan bo dan y da trien khai va viet xong + khong co cung xuong + khong co viec lam lai + phuc but hoat dong bang khong + long tuyen compass da ket,
// commit chuong cuoi tu dong day Phase=Complete, khong can kien truc su chu dong goi complete_book.
// Day la ban sua loi livelock duoc gioi thieu sau khi 9bf26a5 xoa ket thuc tu dong phan tang (mo hinh cuoi tap cuoi khong append
// cung khong complete → writer chay tran vong lap).
func TestCommitChapterLayeredAutoCompletesWhenDone(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	// Mot tap mot cung hai chuong, tat ca da trien khai (khong co cung xuong)
	foundation := NewSaveFoundationTool(s)
	layeredArgs, _ := json.Marshal(map[string]any{
		"type": "layered_outline",
		"content": []map[string]any{{
			"index": 1, "title": "Tap mot", "theme": "Chu de",
			"arcs": []map[string]any{{
				"index": 1, "title": "Cung mot", "goal": "Muc tieu",
				"chapters": []map[string]any{
					{"title": "Chuong dau", "core_event": "Mo dau", "hook": "Tiep theo"},
					{"title": "Chuong hai", "core_event": "Phat trien", "hook": "Ket thuc"},
				},
			}},
		}},
		"scale": "long",
	})
	if _, err := foundation.Execute(context.Background(), layeredArgs); err != nil {
		t.Fatalf("Execute layered: %v", err)
	}
	// Long tuyen compass da ket (OpenThreads rong)
	if err := s.Outline.SaveCompass(domain.StoryCompass{EndingDirection: "Nhan vat chinh ve que"}); err != nil {
		t.Fatalf("SaveCompass: %v", err)
	}
	_ = s.Progress.UpdatePhase(domain.PhaseWriting)

	tool := NewCommitChapterTool(s)
	commit := func(ch int) map[string]any {
		if err := s.Drafts.SaveDraft(ch, fmt.Sprintf("Noi dung chuong %d, dung de kiem thu ket thuc xac dinh tinh.", ch)); err != nil {
			t.Fatalf("SaveDraft %d: %v", ch, err)
		}
		args, _ := json.Marshal(map[string]any{
			"chapter": ch, "summary": "Tom tat", "characters": []string{"Nhan vat chinh"}, "key_events": []string{"Su kien"},
		})
		raw, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("Execute ch%d: %v", ch, err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("Unmarshal ch%d: %v", ch, err)
		}
		return out
	}

	// Chuong 1: chua viet xong, khong nen ket thuc
	if bc, _ := commit(1)["book_complete"].(bool); bc {
		t.Fatal("Viet xong chuong 1 khong nen kich hoat ket thuc")
	}
	if p, _ := s.Progress.Load(); p.Phase == domain.PhaseComplete {
		t.Fatal("Viet xong chuong 1 phase khong nen la complete")
	}

	// Chuong 2 (chuong cuoi): nen tu dong ket thuc
	if bc, _ := commit(2)["book_complete"].(bool); !bc {
		t.Fatal("Viet xong chuong cuoi nen tu dong ket thuc")
	}
	if p, _ := s.Progress.Load(); p.Phase != domain.PhaseComplete {
		t.Fatalf("expected phase=complete, got %s", p.Phase)
	}
}

// TestCommitChapterLayeredNoAutoCompleteWithOpenThreads xac minh tinh bao thu: khi van con long tuyen hoat dong
// du chuong viet day du cung khong tu dong ket thuc, trao quyen phan quyet "co tiep tuc hay khong" cho kien truc su.
func TestCommitChapterLayeredNoAutoCompleteWithOpenThreads(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	foundation := NewSaveFoundationTool(s)
	layeredArgs, _ := json.Marshal(map[string]any{
		"type": "layered_outline",
		"content": []map[string]any{{
			"index": 1, "title": "Tap mot", "theme": "Chu de",
			"arcs": []map[string]any{{
				"index": 1, "title": "Cung mot", "goal": "Muc tieu",
				"chapters": []map[string]any{{"title": "Chuong dau", "core_event": "Mo dau", "hook": "Tiep theo"}},
			}},
		}},
		"scale": "long",
	})
	if _, err := foundation.Execute(context.Background(), layeredArgs); err != nil {
		t.Fatalf("Execute layered: %v", err)
	}
	// Van con long tuyen hoat dong chua ket
	if err := s.Outline.SaveCompass(domain.StoryCompass{EndingDirection: "Nhan vat chinh ve que", OpenThreads: []string{"Ke thu chua bi tieu diet"}}); err != nil {
		t.Fatalf("SaveCompass: %v", err)
	}
	_ = s.Progress.UpdatePhase(domain.PhaseWriting)

	if err := s.Drafts.SaveDraft(1, "Noi dung chuong duy nhat, nhung long tuyen chua ket."); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	tool := NewCommitChapterTool(s)
	args, _ := json.Marshal(map[string]any{
		"chapter": 1, "summary": "Tom tat", "characters": []string{"Nhan vat chinh"}, "key_events": []string{"Su kien"},
	})
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if p, _ := s.Progress.Load(); p.Phase == domain.PhaseComplete {
		t.Fatal("Long tuyen hoat dong chua ket khong nen tu dong hoan thanh")
	}
}

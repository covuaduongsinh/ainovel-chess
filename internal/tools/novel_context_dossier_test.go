package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

func setupDossierStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	st := store.NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	if err := st.Outline.SaveOutline([]domain.OutlineEntry{{Chapter: 1, Title: "Start", CoreEvent: "Begin"}}); err != nil {
		t.Fatal(err)
	}
	if err := st.Progress.Init("test", 1); err != nil {
		t.Fatal(err)
	}
	return st
}

func factDossier(m map[string]any) (map[string]any, bool) {
	working, ok := m["working_memory"].(map[string]any)
	if !ok {
		return nil, false
	}
	fd, ok := working["fact_dossier"].(map[string]any)
	return fd, ok
}

func TestContextToolInjectsFactDossier(t *testing.T) {
	st := setupDossierStore(t)
	if err := st.Dossier.Save(&domain.Dossier{
		Enabled:  true,
		Subject:  "Wilhelm Steinitz",
		Fidelity: domain.FidelityAnchored,
		Facts: []domain.DossierFact{
			{Category: "thành tựu", Fact: "Nhà vô địch cờ vua thế giới đầu tiên (1886)", MustHold: true},
			{Category: "chuyên môn", Fact: "Cha đẻ trường phái cờ vị thế", MustHold: false},
		},
	}); err != nil {
		t.Fatal(err)
	}

	tool := NewContextTool(st, References{}, "default")

	// Đường architect (chapter=0)
	architectRaw, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("architect Execute: %v", err)
	}
	var architect map[string]any
	if err := json.Unmarshal(architectRaw, &architect); err != nil {
		t.Fatal(err)
	}
	fd, ok := factDossier(architect)
	if !ok {
		t.Fatalf("architect context thiếu working_memory.fact_dossier")
	}
	if fd["subject"] != "Wilhelm Steinitz" {
		t.Fatalf("subject sai: %v", fd["subject"])
	}
	must, ok := fd["must_hold"].([]any)
	if !ok || len(must) != 1 {
		t.Fatalf("phải có đúng 1 mỏ neo must_hold: %v", fd["must_hold"])
	}

	// Đường writer (chapter=1)
	chapterRaw, err := tool.Execute(context.Background(), json.RawMessage(`{"chapter":1}`))
	if err != nil {
		t.Fatalf("chapter Execute: %v", err)
	}
	var chapter map[string]any
	if err := json.Unmarshal(chapterRaw, &chapter); err != nil {
		t.Fatal(err)
	}
	if _, ok := factDossier(chapter); !ok {
		t.Fatalf("writer context thiếu working_memory.fact_dossier")
	}
}

func TestContextToolSkipsDisabledDossier(t *testing.T) {
	st := setupDossierStore(t)
	if err := st.Dossier.Save(&domain.Dossier{Enabled: false, Subject: "Wilhelm Steinitz"}); err != nil {
		t.Fatal(err)
	}

	tool := NewContextTool(st, References{}, "default")
	raw, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := factDossier(m); ok {
		t.Fatalf("hồ sơ tắt không được bơm fact_dossier")
	}
}

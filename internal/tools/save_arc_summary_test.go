package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/store"
)

func TestSaveArcSummaryPersistsStyleRulesDialogueObjects(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tool := NewSaveArcSummaryTool(s)
	args, err := json.Marshal(map[string]any{
		"volume":     1,
		"arc":        2,
		"title":      "Vao Nui",
		"summary":    "Nhan vat chinh hoan thanh thu thach vao nui, xac nhan huong truy tim tiep theo.",
		"key_events": []string{"Vuot qua thu thach", "Phat hien manh moi vu an cu"},
		"character_snapshots": []map[string]any{
			{"name": "Tram Uyen", "status": "Con song", "motivation": "Truy tim vu an cu"},
		},
		"style_rules": map[string]any{
			"prose": []string{"Mo ta moi truong uu tien xuc giac va khi giac", "Canh hanh dong dung cau ngan day tien", "Mo ta tam ly khong giai thich ket luan"},
			"dialogue": []map[string]any{
				{"name": "Tram Uyen", "rules": []string{"Doi thoai cuc ngan gon", "Han che dung cau hoi"}},
			},
			"taboos": []string{"Tranh doc thoai dai o cuoi chuong"},
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	rules, err := s.World.LoadStyleRules()
	if err != nil {
		t.Fatalf("LoadStyleRules: %v", err)
	}
	if rules == nil || len(rules.Dialogue) != 1 {
		t.Fatalf("expected one dialogue rule, got %+v", rules)
	}
	if rules.Dialogue[0].Name != "Tram Uyen" || len(rules.Dialogue[0].Rules) != 2 {
		t.Fatalf("unexpected dialogue rule: %+v", rules.Dialogue[0])
	}
}

func TestSaveArcSummaryRejectsDialogueStringArray(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tool := NewSaveArcSummaryTool(s)
	args, err := json.Marshal(map[string]any{
		"volume":              1,
		"arc":                 2,
		"title":               "Vao Nui",
		"summary":             "Nhan vat chinh hoan thanh thu thach vao nui, xac nhan huong truy tim tiep theo.",
		"key_events":          []string{"Vuot qua thu thach"},
		"character_snapshots": []map[string]any{},
		"style_rules": map[string]any{
			"prose":    []string{"Mo ta moi truong uu tien xuc giac va khi giac"},
			"dialogue": []string{"Tram Uyen doi thoai cuc ngan gon"},
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil || !strings.Contains(err.Error(), "style_rules.dialogue") {
		t.Fatalf("expected style_rules.dialogue validation error, got %v", err)
	}
}

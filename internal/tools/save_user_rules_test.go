package tools

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/rules"
	"github.com/voocel/ainovel-cli/internal/store"
	"github.com/voocel/ainovel-cli/internal/userrules"
)

// Mo hinh nil → giam cap chuan hoa; LoadOptions rong → khong doc dia that.
func newDegradedTool(t *testing.T) (*SaveUserRulesTool, *store.Store) {
	t.Helper()
	st := store.NewStore(t.TempDir())
	svc := userrules.NewService(st, nil, rules.LoadOptions{})
	return NewSaveUserRulesTool(svc), st
}

// Hop dong cot loi: loi chuan hoa (chi tiet ky thuat) tuyet doi khong nem lai cho Coordinator, chi giam cap + tra ve su kien + luu dia.
func TestSaveUserRulesTool_DegradeReturnsFactsNotError(t *testing.T) {
	tool, st := newDegradedTool(t)

	out, err := tool.Execute(t.Context(), json.RawMessage(`{"text":"Moi chuong 1500 chu, han che dung an du"}`))
	if err != nil {
		t.Fatalf("Giam cap chuan hoa khong nen nem ra nhu tool error: %v", err)
	}

	var res struct {
		Saved      bool   `json:"saved"`
		Status     string `json:"status"`
		Understood struct {
			Degraded    bool   `json:"degraded"`
			Preferences string `json:"preferences"`
		} `json:"understood"`
		InEffect map[string]any `json:"in_effect"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("Ket qua phai la JSON hop le: %v", err)
	}
	if !res.Saved {
		t.Fatal("saved phai la true")
	}
	if res.Status != string(rules.StatusDegraded) {
		t.Fatalf("Khong co mo hinh phai la degraded, got %q", res.Status)
	}
	if !res.Understood.Degraded || res.Understood.Preferences != "Moi chuong 1500 chu, han che dung an du" {
		t.Fatalf("Hien thi lai phai chua dau hieu giam cap va van ban goc, got %+v", res.Understood)
	}
	if res.InEffect == nil {
		t.Fatal("Phai tra ve toan bo rang buoc dang hieu luc de hien thi lai")
	}
	// Da luu dia.
	if cur, _ := st.UserRules.Load(); cur == nil {
		t.Fatal("Quy tac phai da duoc luu tru")
	}
}

func TestSaveUserRulesTool_EmptyTextErrors(t *testing.T) {
	tool, _ := newDegradedTool(t)
	if _, err := tool.Execute(t.Context(), json.RawMessage(`{"text":"  "}`)); !errors.Is(err, errs.ErrToolArgs) {
		t.Fatalf("空 text 应返回 ErrToolArgs，got %v", err)
	}
}

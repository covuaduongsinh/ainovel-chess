package store

import (
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
)

func TestDossierStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}

	// Chưa có hồ sơ → Load trả nil, nil.
	got, err := st.Dossier.Load()
	if err != nil {
		t.Fatalf("Load rỗng: %v", err)
	}
	if got != nil {
		t.Fatalf("mong đợi nil khi chưa lưu, nhận %+v", got)
	}

	want := &domain.Dossier{
		Enabled:   true,
		Subject:   "Wilhelm Steinitz",
		Fidelity:  domain.FidelityAnchored,
		Confirmed: true,
		Facts: []domain.DossierFact{
			{Category: "thành tựu", Fact: "Nhà vô địch cờ vua thế giới đầu tiên (1886)", MustHold: true},
			{Category: "chuyên môn", Fact: "Cha đẻ trường phái cờ vị thế", MustHold: false},
		},
	}
	if err := st.Dossier.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err = st.Dossier.Load()
	if err != nil {
		t.Fatalf("Load sau Save: %v", err)
	}
	if got == nil || got.Subject != want.Subject || !got.Enabled || len(got.Facts) != 2 {
		t.Fatalf("round-trip sai: %+v", got)
	}
	if !got.Facts[0].MustHold || got.Facts[1].MustHold {
		t.Fatalf("cờ must_hold không được bảo toàn: %+v", got.Facts)
	}
}

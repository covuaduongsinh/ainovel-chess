package dossier

import (
	"context"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// Với model=nil, dịch vụ giáng cấp một cách xác định (không cần mạng), đủ để kiểm thử đơn vị.

func TestNormalizeSourceKeepsRawWhenModelNil(t *testing.T) {
	svc := NewService(nil)
	d, err := svc.NormalizeSource(context.Background(), "Wilhelm Steinitz", "Nhà vô địch cờ vua thế giới đầu tiên năm 1886.")
	if err != nil {
		t.Fatal(err)
	}
	if d.Subject != "Wilhelm Steinitz" || d.Fidelity != domain.FidelityAnchored {
		t.Fatalf("trường cơ bản sai: %+v", d)
	}
	if len(d.Facts) != 1 || !strings.Contains(d.Facts[0].Fact, "1886") {
		t.Fatalf("tư liệu thô phải được giữ làm dữ kiện: %+v", d.Facts)
	}
	if d.RawSource == "" {
		t.Fatalf("RawSource phải được lưu")
	}
}

func TestNormalizeSourceEmptySourceAnchorsName(t *testing.T) {
	svc := NewService(nil)
	d, err := svc.NormalizeSource(context.Background(), "Wilhelm Steinitz", "")
	if err != nil {
		t.Fatal(err)
	}
	must := d.MustHoldFacts()
	if len(must) == 0 {
		t.Fatalf("tên thật phải là mỏ neo must_hold khi không có tư liệu: %+v", d.Facts)
	}
	if !strings.Contains(must[0].Fact, "Wilhelm Steinitz") {
		t.Fatalf("mỏ neo phải chứa tên thật: %+v", must)
	}
}

func TestDraftFromSubjectDegradesWithDisclaimer(t *testing.T) {
	svc := NewService(nil)
	d, err := svc.DraftFromSubject(context.Background(), "Wilhelm Steinitz")
	if err != nil {
		t.Fatal(err)
	}
	if !d.Draft {
		t.Fatalf("bản nháp phải đặt Draft=true")
	}
	if strings.TrimSpace(d.Disclaimer) == "" {
		t.Fatalf("bản nháp phải có disclaimer cảnh báo kiểm chứng")
	}
}

func TestDraftFromSubjectEmptySubject(t *testing.T) {
	svc := NewService(nil)
	d, err := svc.DraftFromSubject(context.Background(), "  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Facts) != 0 {
		t.Fatalf("subject rỗng không nên sinh dữ kiện: %+v", d.Facts)
	}
}

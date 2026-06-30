package tools

import (
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
)

func TestParsePremiseSections(t *testing.T) {
	premise := `# Premise

## Đề tài và tông điệu
Huyền huyễn phương Đông, trưởng thành lạnh cứng.

## Định vị đề tài
Huyền huyễn phương Đông dòng thăng cấp, hướng đến độc giả tìm khoái cảm và đẩy tiến quan hệ.

## Xung đột cốt lõi
Nhân vật chính phải chọn giữa quy tắc tông môn và lương tri cá nhân.

## Tại sao tác phẩm phù hợp truyện ngắn/kết thúc trong một tập
Mâu thuẫn cốt lõi và cung nhân vật đều gói gọn trong một nhiệm vụ.
`

	sections := parsePremiseSections(premise)
	if sections["Đề tài và tông điệu"] == "" {
		t.Fatalf("expected Đề tài và tông điệu section, got %+v", sections)
	}
	if sections["Định vị đề tài"] == "" {
		t.Fatalf("expected Định vị đề tài section, got %+v", sections)
	}
	if sections["Xung đột cốt lõi"] == "" {
		t.Fatalf("expected Xung đột cốt lõi section, got %+v", sections)
	}
	if sections["Tính phù hợp truyện ngắn"] == "" {
		t.Fatalf("expected long-form alias normalized to Tính phù hợp truyện ngắn, got %+v", sections)
	}
}

func TestPremiseStructure(t *testing.T) {
	premise := `## Đề tài và tông điệu
Dòng thăng cấp, thiên lạnh cứng.

## Định vị đề tài
Dòng thăng cấp

## Xung đột cốt lõi
Xung đột

## Mục tiêu nhân vật chính
Mục tiêu

## Hướng kết cục
Kết cục

## Vùng cấm sáng tác
Vùng cấm

## Điểm bán khác biệt
Điểm bán

## Móc câu khác biệt
Móc câu

## Cam kết cốt lõi
Cam kết

## Động cơ truyện
Động cơ

## Bước ngoặt giữa truyện
Bước ngoặt
`

	structure := premiseStructure(premise, domain.PlanningTierMid)
	if ready, _ := structure["template_ready"].(bool); !ready {
		t.Fatalf("expected template_ready, got %+v", structure)
	}
	missing, _ := structure["missing"].([]string)
	if len(missing) != 0 {
		t.Fatalf("expected no missing headings, got %+v", missing)
	}
}

func TestPremiseStructureShortAcceptsLegacyHeadingAlias(t *testing.T) {
	premise := `## Đề tài và tông điệu
Giải cứu cao áp trong một tập.

## Định vị đề tài
Phiêu lưu mật độ cao dạng truyện ngắn.

## Xung đột cốt lõi
Nhân vật chính phải cứu con tin trong một đêm.

## Mục tiêu nhân vật chính
Cứu con tin và sống sót rời đi.

## Hướng kết cục
Hoàn thành nhiệm vụ nhưng phải trả giá.

## Vùng cấm sáng tác
Không mở rộng thành đăng dài kỳ.

## Điểm bán khác biệt
Áp lực thời hạn và những cú lật liên tiếp.

## Móc câu khác biệt
Mỗi lựa chọn đều rút ngắn thời gian giải cứu.

## Cam kết cốt lõi
Cảm giác cấp bách, sự lựa chọn và những cú lật.

## Tại sao tác phẩm phù hợp truyện ngắn/kết thúc trong một tập
Mâu thuẫn cốt lõi và cung nhân vật đều gói gọn trong một nhiệm vụ.
`

	structure := premiseStructure(premise, domain.PlanningTierShort)
	if ready, _ := structure["template_ready"].(bool); !ready {
		t.Fatalf("expected short template_ready, got %+v", structure)
	}
}

package web

import (
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/bootstrap"
)

// TestMergeSecret_GiuBiMatKhiGiaTriDaCheGuiNguocLen là chốt chặn cho luật vòng-lặp của
// hộp thoại Sách nói: máy chủ không giữ trạng thái phiên, nên trình duyệt gửi lại đúng
// chuỗi đã che mà nó vừa hiển thị. Thiếu luật này thì mỗi lần bấm "Lưu" sẽ ghi đè token
// thật bằng chuỗi "sk-1****wxyz" và người dùng mất thông tin xác thực.
func TestMergeSecret_GiuBiMatKhiGiaTriDaCheGuiNguocLen(t *testing.T) {
	const real = "sk-abcdefghijklmnop"
	masked := bootstrap.MaskSecret(real)
	if !strings.Contains(masked, maskedMarker) {
		t.Fatalf("giá trị che phải chứa %q, nhận %q", maskedMarker, masked)
	}
	if got := mergeSecret(real, masked); got != real {
		t.Errorf("gửi lại giá trị đã che phải giữ bí mật cũ, nhận %q", got)
	}
}

func TestMergeSecret_CacTruongHopConLai(t *testing.T) {
	cases := []struct {
		ten      string
		current  string
		incoming string
		muon     string
	}{
		{"để trống = không đổi", "cu", "", "cu"},
		{"chỉ khoảng trắng = không đổi", "cu", "   ", "cu"},
		{"giá trị mới thì thay", "cu", "moi", "moi"},
		{"cắt khoảng trắng thừa", "cu", "  moi  ", "moi"},
		{"dấu gạch ngang = xóa hẳn", "cu", clearMarker, ""},
		{"chưa có gì mà gửi mới", "", "moi", "moi"},
	}
	for _, tc := range cases {
		if got := mergeSecret(tc.current, tc.incoming); got != tc.muon {
			t.Errorf("%s: mergeSecret(%q, %q) = %q, muốn %q", tc.ten, tc.current, tc.incoming, got, tc.muon)
		}
	}
}

func TestToVbeeDTO_LuonCheThongTinBiMat(t *testing.T) {
	dto := toVbeeDTO(bootstrap.VbeeConfig{
		AppID:       "app-id-that-day-du",
		AccessToken: "token-that-day-du",
		VoiceCode:   "hn_female_ngochuyen_full_48k-fhg",
	})
	if strings.Contains(dto.AppID, "that-day-du") || strings.Contains(dto.AccessToken, "that-day-du") {
		t.Errorf("thông tin bí mật bị lộ nguyên văn: %+v", dto)
	}
	if !strings.Contains(dto.AppID, maskedMarker) || !strings.Contains(dto.AccessToken, maskedMarker) {
		t.Errorf("thông tin bí mật phải được che: %+v", dto)
	}
	if !dto.Configured {
		t.Error("đủ app_id + token thì Configured phải là true")
	}
	if dto.VoiceCode != "hn_female_ngochuyen_full_48k-fhg" {
		t.Errorf("trường không bí mật phải giữ nguyên: %q", dto.VoiceCode)
	}
}

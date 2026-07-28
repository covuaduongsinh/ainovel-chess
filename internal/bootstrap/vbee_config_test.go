package bootstrap

import (
	"errors"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/errs"
)

// baseValidConfig là cấu hình tối thiểu hợp lệ để ValidateBase đi qua được phần LLM,
// nhờ đó test bên dưới chỉ còn phản ánh đúng phần vbee.
func baseValidConfig() Config {
	return Config{
		Provider:  "openrouter",
		ModelName: "google/gemini-2.5-flash",
		Providers: map[string]ProviderConfig{
			"openrouter": {APIKey: "sk-test-123456"},
		},
	}
}

func TestValidateBase_VbeeRongLaHopLe(t *testing.T) {
	c := baseValidConfig()
	if err := c.ValidateBase(); err != nil {
		t.Fatalf("mục vbee bỏ trống phải hợp lệ, nhận: %v", err)
	}
}

func TestValidateBase_VbeeDayDuLaHopLe(t *testing.T) {
	c := baseValidConfig()
	c.Vbee = VbeeConfig{
		AppID:        "app-1",
		AccessToken:  "tok-1",
		VoiceCode:    "hn_female_ngochuyen_full_48k-fhg",
		Speed:        1.0,
		Bitrate:      128,
		SampleRate:   24000,
		OutputFormat: "mp3",
	}
	if err := c.ValidateBase(); err != nil {
		t.Fatalf("cấu hình vbee hợp lệ mà báo lỗi: %v", err)
	}
}

func TestValidateBase_VbeeGiaTriNgoaiKhoang(t *testing.T) {
	cases := []struct {
		ten   string
		sua   func(*VbeeConfig)
		chuoi string
	}{
		{"speed quá nhỏ", func(v *VbeeConfig) { v.Speed = 0.1 }, "vbee.speed"},
		{"speed quá lớn", func(v *VbeeConfig) { v.Speed = 2.5 }, "vbee.speed"},
		{"bitrate lạ", func(v *VbeeConfig) { v.Bitrate = 96 }, "vbee.bitrate"},
		{"sample_rate lạ", func(v *VbeeConfig) { v.SampleRate = 12345 }, "vbee.sample_rate"},
		{"output_format lạ", func(v *VbeeConfig) { v.OutputFormat = "ogg" }, "vbee.output_format"},
		{"app_id có ký tự điều khiển", func(v *VbeeConfig) { v.AppID = "app\x00id" }, "vbee.app_id"},
	}
	for _, tc := range cases {
		t.Run(tc.ten, func(t *testing.T) {
			c := baseValidConfig()
			tc.sua(&c.Vbee)
			err := c.ValidateBase()
			if err == nil {
				t.Fatalf("%s phải báo lỗi", tc.ten)
			}
			if !errors.Is(err, errs.ErrConfig) {
				t.Errorf("lỗi phải bọc errs.ErrConfig, nhận: %v", err)
			}
			if !strings.Contains(err.Error(), tc.chuoi) {
				t.Errorf("thông báo phải nhắc tới %q, nhận: %v", tc.chuoi, err)
			}
		})
	}
}

func TestVbeeConfig_Configured(t *testing.T) {
	cases := []struct {
		ten  string
		v    VbeeConfig
		muon bool
	}{
		{"rỗng", VbeeConfig{}, false},
		{"chỉ có app_id", VbeeConfig{AppID: "a"}, false},
		{"chỉ có token", VbeeConfig{AccessToken: "t"}, false},
		{"chỉ có khoảng trắng", VbeeConfig{AppID: "  ", AccessToken: "  "}, false},
		{"đủ đôi", VbeeConfig{AppID: "a", AccessToken: "t"}, true},
	}
	for _, tc := range cases {
		if got := tc.v.Configured(); got != tc.muon {
			t.Errorf("%s: Configured() = %v, muốn %v", tc.ten, got, tc.muon)
		}
	}
}

func TestFillDefaults_VbeeChiDienKhiDaCauHinh(t *testing.T) {
	// Chưa cấu hình → phải giữ nguyên rỗng, để không vẽ khối "vbee" thừa vào
	// config.json của người không dùng tính năng sách nói.
	trong := baseValidConfig()
	trong.FillDefaults()
	if trong.Vbee != (VbeeConfig{}) {
		t.Errorf("chưa cấu hình mà đã điền mặc định: %+v", trong.Vbee)
	}

	// Đã cấu hình → điền đủ mặc định.
	day := baseValidConfig()
	day.Vbee = VbeeConfig{AppID: "app-1", AccessToken: "tok-1"}
	day.FillDefaults()
	v := day.Vbee
	if v.BaseURL != DefaultVbeeBaseURL || v.VoicesURL != DefaultVbeeVoicesURL || v.WebhookURL != DefaultVbeeWebhookURL {
		t.Errorf("URL mặc định chưa được điền: %+v", v)
	}
	if v.Speed != 1.0 || v.Bitrate != 128 || v.SampleRate != 24000 || v.OutputFormat != "mp3" {
		t.Errorf("tham số mặc định chưa được điền: %+v", v)
	}
}

func TestFillDefaults_VbeeKhongDeGiaTriNguoiDungBiGhiDe(t *testing.T) {
	c := baseValidConfig()
	c.Vbee = VbeeConfig{
		AppID: "app-1", AccessToken: "tok-1",
		BaseURL: "https://proxy.noi.bo", Speed: 1.3, Bitrate: 64, OutputFormat: "wav",
	}
	c.FillDefaults()
	if c.Vbee.BaseURL != "https://proxy.noi.bo" || c.Vbee.Speed != 1.3 ||
		c.Vbee.Bitrate != 64 || c.Vbee.OutputFormat != "wav" {
		t.Errorf("giá trị người dùng bị ghi đè: %+v", c.Vbee)
	}
}

// TestMergeConfig_VbeeGhiDeTungTruong là chốt chặn cho cái bẫy lớn nhất của mergeConfig:
// nó ghép TỪNG TRƯỜNG thủ công, nên quên một nhánh là cấu hình cấp dự án biến mất
// không báo lỗi. Ở đây: toàn cục giữ thông tin xác thực, dự án chỉ đổi giọng đọc —
// cả hai credential phải sống sót.
func TestMergeConfig_VbeeGhiDeTungTruong(t *testing.T) {
	base := Config{Vbee: VbeeConfig{
		AppID:        "app-toan-cuc",
		AccessToken:  "tok-toan-cuc",
		VoiceCode:    "giong-cu",
		Speed:        1.0,
		Bitrate:      128,
		OutputFormat: "mp3",
	}}
	overlay := Config{Vbee: VbeeConfig{VoiceCode: "giong-moi", Speed: 1.25}}

	got := mergeConfig(base, overlay).Vbee

	if got.AppID != "app-toan-cuc" || got.AccessToken != "tok-toan-cuc" {
		t.Errorf("thông tin xác thực toàn cục bị mất: %+v", got)
	}
	if got.VoiceCode != "giong-moi" {
		t.Errorf("voice_code cấp dự án không được áp dụng: %q", got.VoiceCode)
	}
	if got.Speed != 1.25 {
		t.Errorf("speed cấp dự án không được áp dụng: %v", got.Speed)
	}
	if got.Bitrate != 128 || got.OutputFormat != "mp3" {
		t.Errorf("trường không khai báo ở overlay phải giữ giá trị cũ: %+v", got)
	}
}

func TestMergeConfig_VbeeOverlayRongThiGiuNguyen(t *testing.T) {
	base := Config{Vbee: VbeeConfig{AppID: "a", AccessToken: "t", VoiceCode: "v", Bitrate: 64}}
	got := mergeConfig(base, Config{}).Vbee
	if got != base.Vbee {
		t.Errorf("overlay rỗng không được thay đổi gì: %+v", got)
	}
}

func TestMaskSecret(t *testing.T) {
	cases := []struct {
		vao, ra string
	}{
		{"", "****"},
		{"ngan", "****"},
		{"12345678", "****"},
		{"123456789", "1234****6789"},
		{"sk-abcdefghijklmnop", "sk-a****mnop"},
	}
	for _, tc := range cases {
		if got := MaskSecret(tc.vao); got != tc.ra {
			t.Errorf("MaskSecret(%q) = %q, muốn %q", tc.vao, got, tc.ra)
		}
	}
}

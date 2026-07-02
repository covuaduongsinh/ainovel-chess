package bootstrap

import "strings"

// ClaudeCodeProvider là tên key provider mặc định cho tích hợp Claude Code.
// Đây là một "con trỏ" trỏ tới một proxy nội bộ tương thích giao thức Anthropic
// (dùng đăng nhập Claude Code của người dùng). litellm chỉ xác thực bằng x-api-key,
// nên không thể đăng nhập gói thuê bao trực tiếp trong binary — luôn đi qua proxy nội bộ.
const ClaudeCodeProvider = "claude-code"

// ClaudeCodeDefaultBaseURL là địa chỉ proxy nội bộ gợi ý mặc định trong wizard.
// Mặc định khớp Meridian (cầu nối hợp lệ qua Agent SDK chính thức, cổng 3456).
// Nếu dùng cầu nối/proxy khác thì đổi lại cho khớp cổng thực tế.
const ClaudeCodeDefaultBaseURL = "http://127.0.0.1:3456"

// ClaudeDefaultModel là model mặc định cân bằng khi khởi tạo provider Claude Code.
const ClaudeDefaultModel = "claude-sonnet-4-6"

// ClaudeCodeLocalKeyPlaceholder là api_key giữ chỗ khi dùng cầu nối nội bộ không cần key thật.
// litellm (provider anthropic) BẮT BUỘC x-api-key khác rỗng, nhưng cầu nối như Meridian bỏ qua
// giá trị này (nó xác thực bằng đăng nhập Claude Code). KHÔNG dùng cho api.anthropic.com thật —
// ở đó api_key phải là khóa sk-ant... thật, nếu không sẽ 401.
const ClaudeCodeLocalKeyPlaceholder = "sk-local"

// ClaudeModelCatalog là danh mục model Claude (đúng bộ model trong Claude Code).
// ID dạng gạch ngang — chính là chuỗi gửi thẳng lên API Anthropic. Dùng để điền
// providers.<name>.models cho bảng chọn /model, và làm cơ sở cho preset tự-chọn.
var ClaudeModelCatalog = []string{
	"claude-opus-4-8",
	"claude-opus-4-7",
	"claude-sonnet-4-6",
	"claude-haiku-4-5",
}

// ClaudeCodeProviderConfig dựng ProviderConfig trỏ tới proxy Claude Code nội bộ:
// type=anthropic (giao thức Anthropic), api_key tùy chọn (proxy nội bộ thường không cần),
// kèm sẵn danh mục model để bảng chọn /model dùng được ngay.
func ClaudeCodeProviderConfig(baseURL, apiKey string) ProviderConfig {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = ClaudeCodeDefaultBaseURL
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		// Cầu nối nội bộ (Meridian…) không cần key thật, nhưng litellm vẫn đòi x-api-key khác rỗng → giữ chỗ.
		apiKey = ClaudeCodeLocalKeyPlaceholder
	}
	return ProviderConfig{
		Type:    "anthropic",
		APIKey:  apiKey,
		BaseURL: baseURL,
		Models:  append([]string(nil), ClaudeModelCatalog...),
	}
}

// RolePreset là một mục trong preset gán model theo vai.
type RolePreset struct {
	Role   string // coordinator / architect / writer / editor
	Model  string // ID model Claude (dạng gạch ngang)
	Effort string // off/low/medium/high/xhigh/max — rỗng = kế thừa mặc định
}

// Preset keys cho tự-chọn model Claude.
const (
	PresetStandard = "standard" // "Chuẩn": Opus cho writer/architect, Sonnet cho coordinator/editor
	PresetEconomy  = "economy"  // "Tiết kiệm": bỏ Opus — Sonnet cho writer/architect/editor, Haiku cho coordinator
)

// ClaudePreset là một cấu hình gán model theo vai có tên, dùng cho tự-chọn nhanh.
type ClaudePreset struct {
	Key   string // định danh máy (standard/economy)
	Label string // nhãn hiển thị (thông báo TUI, nút Web)
	Desc  string // mô tả ngắn
	Roles []RolePreset
}

// ClaudePresets trả về các preset tự-chọn theo thứ tự cố định (standard trước).
// Đây là nguồn duy nhất cho setup wizard, lệnh /model auto và API web.
func ClaudePresets() []ClaudePreset {
	return []ClaudePreset{
		{
			Key:   PresetStandard,
			Label: "Chuẩn (cân bằng)",
			Desc:  "Opus cho Writer/Architect, Sonnet cho Coordinator/Editor — chất lượng cao nhất",
			Roles: []RolePreset{
				{Role: "coordinator", Model: "claude-sonnet-4-6", Effort: "medium"},
				{Role: "architect", Model: "claude-opus-4-8", Effort: "high"},
				{Role: "writer", Model: "claude-opus-4-8", Effort: "high"},
				{Role: "editor", Model: "claude-sonnet-4-6", Effort: "medium"},
			},
		},
		{
			Key:   PresetEconomy,
			Label: "Tiết kiệm (vẫn tối ưu chất lượng)",
			Desc:  "Bỏ Opus: Sonnet cho Writer/Architect/Editor, Haiku cho Coordinator — rẻ hơn nhưng giữ prose ở Sonnet",
			Roles: []RolePreset{
				{Role: "coordinator", Model: "claude-haiku-4-5", Effort: "medium"},
				{Role: "architect", Model: "claude-sonnet-4-6", Effort: "medium"},
				{Role: "writer", Model: "claude-sonnet-4-6", Effort: "high"},
				{Role: "editor", Model: "claude-sonnet-4-6", Effort: "medium"},
			},
		},
	}
}

// ClaudePresetByKey giải quyết preset theo key, chấp nhận một số bí danh thân thiện.
// Key rỗng → preset "standard" (mặc định). Trả về false nếu key không nhận ra.
func ClaudePresetByKey(key string) (ClaudePreset, bool) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "", PresetStandard, "chuan", "balanced", "canbang", "default":
		key = PresetStandard
	case PresetEconomy, "tietkiem", "tiet-kiem", "tiet_kiem", "save", "cheap", "tk":
		key = PresetEconomy
	default:
		return ClaudePreset{}, false
	}
	for _, p := range ClaudePresets() {
		if p.Key == key {
			return p, true
		}
	}
	return ClaudePreset{}, false
}

// PresetKeysHint trả về danh sách key hợp lệ để hiển thị trong thông báo lỗi.
func PresetKeysHint() string {
	keys := make([]string, 0, 2)
	for _, p := range ClaudePresets() {
		keys = append(keys, p.Key)
	}
	return strings.Join(keys, " / ")
}

// RoleConfigs chuyển preset thành map[string]RoleConfig sẵn sàng ghi vào Config.Roles cho provider chỉ định.
func (p ClaudePreset) RoleConfigs(provider string) map[string]RoleConfig {
	roles := make(map[string]RoleConfig, len(p.Roles))
	for _, r := range p.Roles {
		roles[r.Role] = RoleConfig{
			Provider:        provider,
			Model:           r.Model,
			ReasoningEffort: r.Effort,
		}
	}
	return roles
}

// BalancedClaudeRoles trả về roles của preset mặc định ("Chuẩn"). Giữ để tương thích
// với setup wizard + test hiện có (cài đặt lần đầu luôn dùng preset Chuẩn).
func BalancedClaudeRoles() []RolePreset {
	p, _ := ClaudePresetByKey(PresetStandard)
	return p.Roles
}

// BalancedRoleConfigs = RoleConfigs của preset "Chuẩn" cho provider chỉ định.
func BalancedRoleConfigs(provider string) map[string]RoleConfig {
	p, _ := ClaudePresetByKey(PresetStandard)
	return p.RoleConfigs(provider)
}

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

// BalancedClaudeRoles trả về preset "cân bằng chất lượng/chi phí": Opus cho công việc
// sáng tạo nặng (writer/architect), Sonnet cho điều phối và xét duyệt. Trả về theo thứ tự
// cố định để lặp xác định. Đây là nguồn duy nhất cho setup wizard, lệnh /model auto và API web.
func BalancedClaudeRoles() []RolePreset {
	return []RolePreset{
		{Role: "coordinator", Model: "claude-sonnet-4-6", Effort: "medium"},
		{Role: "architect", Model: "claude-opus-4-8", Effort: "high"},
		{Role: "writer", Model: "claude-opus-4-8", Effort: "high"},
		{Role: "editor", Model: "claude-sonnet-4-6", Effort: "medium"},
	}
}

// BalancedRoleConfigs chuyển preset cân bằng thành map[string]RoleConfig sẵn sàng ghi
// vào Config.Roles cho provider chỉ định (mặc định là ClaudeCodeProvider).
func BalancedRoleConfigs(provider string) map[string]RoleConfig {
	roles := make(map[string]RoleConfig, 4)
	for _, p := range BalancedClaudeRoles() {
		roles[p.Role] = RoleConfig{
			Provider:        provider,
			Model:           p.Model,
			ReasoningEffort: p.Effort,
		}
	}
	return roles
}

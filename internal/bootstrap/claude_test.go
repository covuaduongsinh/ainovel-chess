package bootstrap

import (
	"strings"
	"testing"
)

// Cấu hình Claude Code do wizard dựng (provider giao thức anthropic không api_key +
// roles cân bằng trỏ về chính nó) phải qua được ValidateBase — nếu không người dùng
// vừa setup xong đã kẹt "chưa cấu hình thông tin xác thực".
func TestClaudeCodeConfigValidates(t *testing.T) {
	cfg := Config{
		Provider:        ClaudeCodeProvider,
		ModelName:       ClaudeDefaultModel,
		ReasoningEffort: "medium",
		Providers: map[string]ProviderConfig{
			ClaudeCodeProvider: ClaudeCodeProviderConfig("", ""),
		},
		Roles: BalancedRoleConfigs(ClaudeCodeProvider),
		Style: "default",
	}
	cfg.FillDefaults()
	if err := cfg.ValidateBase(); err != nil {
		t.Fatalf("cấu hình Claude Code không hợp lệ: %v", err)
	}

	pc := cfg.Providers[ClaudeCodeProvider]
	if pc.Type != "anthropic" {
		t.Errorf("type = %q, muốn anthropic", pc.Type)
	}
	if pc.RequiresAPIKey(ClaudeCodeProvider) {
		t.Error("Claude Code (type anthropic) không nên bắt buộc api_key")
	}
	// litellm (provider anthropic) từ chối x-api-key rỗng ngay cả khi trỏ cầu nối nội bộ,
	// nên helper phải tự chèn key giữ chỗ khi để trống — nếu không request đầu tiên sẽ lỗi "api key is required".
	if pc.APIKey == "" {
		t.Error("api_key rỗng: litellm sẽ từ chối; helper phải chèn key giữ chỗ")
	}
	if pc.BaseURL != ClaudeCodeDefaultBaseURL {
		t.Errorf("base_url = %q, muốn mặc định %q", pc.BaseURL, ClaudeCodeDefaultBaseURL)
	}
	if len(pc.Models) != len(ClaudeModelCatalog) {
		t.Errorf("models = %v, muốn danh mục %v", pc.Models, ClaudeModelCatalog)
	}
}

// Danh mục model phải là ID dạng gạch ngang (đúng chuỗi gửi lên API Anthropic), không dùng dấu chấm.
func TestClaudeModelCatalogUsesDashIDs(t *testing.T) {
	for _, id := range ClaudeModelCatalog {
		if strings.Contains(id, ".") {
			t.Errorf("model %q dùng dấu chấm — API Anthropic cần dạng gạch ngang (vd claude-opus-4-8)", id)
		}
		if !strings.HasPrefix(id, "claude-") {
			t.Errorf("model %q không phải model Claude", id)
		}
	}
}

// Preset cân bằng phải phủ đủ 4 vai đã biết, mỗi model nằm trong danh mục và effort hợp lệ.
func TestBalancedClaudeRolesCoverKnownRoles(t *testing.T) {
	inCatalog := make(map[string]bool, len(ClaudeModelCatalog))
	for _, id := range ClaudeModelCatalog {
		inCatalog[id] = true
	}
	validEffort := map[string]bool{"": true, "off": true, "low": true, "medium": true, "high": true, "xhigh": true, "max": true}

	seen := make(map[string]bool)
	for _, p := range BalancedClaudeRoles() {
		if !knownRoles[p.Role] {
			t.Errorf("vai %q không nằm trong knownRoles", p.Role)
		}
		if !inCatalog[p.Model] {
			t.Errorf("vai %q dùng model %q ngoài danh mục", p.Role, p.Model)
		}
		if !validEffort[p.Effort] {
			t.Errorf("vai %q có effort không hợp lệ %q", p.Role, p.Effort)
		}
		seen[p.Role] = true
	}
	for role := range knownRoles {
		if !seen[role] {
			t.Errorf("preset cân bằng thiếu vai %q", role)
		}
	}
}

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

// Registry preset: cả hai preset (Chuẩn + Tiết kiệm) phải hợp lệ, và mỗi preset thể hiện đúng
// ý đồ — Chuẩn dùng Opus cho writer/architect; Tiết kiệm bỏ Opus nhưng giữ writer ở Sonnet.
func TestClaudePresets(t *testing.T) {
	inCatalog := make(map[string]bool, len(ClaudeModelCatalog))
	for _, id := range ClaudeModelCatalog {
		inCatalog[id] = true
	}
	validEffort := map[string]bool{"": true, "off": true, "low": true, "medium": true, "high": true, "xhigh": true, "max": true}

	presets := ClaudePresets()
	if len(presets) < 2 {
		t.Fatalf("cần ít nhất 2 preset (Chuẩn + Tiết kiệm), có %d", len(presets))
	}
	for _, p := range presets {
		if p.Key == "" || p.Label == "" {
			t.Errorf("preset thiếu Key/Label: %+v", p)
		}
		seen := make(map[string]bool)
		for _, r := range p.Roles {
			if !knownRoles[r.Role] {
				t.Errorf("preset %q: vai %q không hợp lệ", p.Key, r.Role)
			}
			if !inCatalog[r.Model] {
				t.Errorf("preset %q: vai %q model %q ngoài danh mục", p.Key, r.Role, r.Model)
			}
			if !validEffort[r.Effort] {
				t.Errorf("preset %q: vai %q effort %q không hợp lệ", p.Key, r.Role, r.Effort)
			}
			seen[r.Role] = true
		}
		for role := range knownRoles {
			if !seen[role] {
				t.Errorf("preset %q thiếu vai %q", p.Key, role)
			}
		}
	}

	// Tiết kiệm: không dùng Opus, writer vẫn Sonnet (giữ chất lượng prose).
	eco, ok := ClaudePresetByKey(PresetEconomy)
	if !ok {
		t.Fatal("không tìm thấy preset economy")
	}
	for _, r := range eco.Roles {
		if strings.Contains(r.Model, "opus") {
			t.Errorf("preset Tiết kiệm không nên dùng Opus, nhưng vai %q = %q", r.Role, r.Model)
		}
		if r.Role == "writer" && r.Model != "claude-sonnet-4-6" {
			t.Errorf("preset Tiết kiệm: writer nên là Sonnet (giữ chất lượng), nhận %q", r.Model)
		}
	}

	// Chuẩn: writer + architect dùng Opus.
	std, ok := ClaudePresetByKey(PresetStandard)
	if !ok {
		t.Fatal("không tìm thấy preset standard")
	}
	for _, r := range std.Roles {
		if (r.Role == "writer" || r.Role == "architect") && !strings.Contains(r.Model, "opus") {
			t.Errorf("preset Chuẩn: vai %q nên dùng Opus, nhận %q", r.Role, r.Model)
		}
	}

	// Bí danh + mặc định + key lạ.
	if p, ok := ClaudePresetByKey(""); !ok || p.Key != PresetStandard {
		t.Errorf("key rỗng phải về standard, nhận %q ok=%v", p.Key, ok)
	}
	if p, ok := ClaudePresetByKey("tietkiem"); !ok || p.Key != PresetEconomy {
		t.Errorf("alias 'tietkiem' phải về economy, nhận %q ok=%v", p.Key, ok)
	}
	if _, ok := ClaudePresetByKey("khong-ton-tai"); ok {
		t.Error("key lạ phải trả về ok=false")
	}
}

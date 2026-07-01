package bootstrap

import "testing"

func TestConfigResolveReasoningEffort(t *testing.T) {
	cfg := Config{
		ReasoningEffort: "low", // mặc định cấp cao nhất
		Roles: map[string]RoleConfig{
			"writer":    {Provider: "p", Model: "m", ReasoningEffort: "high"}, // ghi đè cấp vai
			"architect": {Provider: "p", Model: "m"},                          // không có reasoning_effort, nên dùng mặc định
		},
	}

	cases := []struct {
		role string
		want string
	}{
		{"writer", "high"},     // ghi đè cấp vai ưu tiên
		{"architect", "low"},   // vai chưa cấu hình → dùng mặc định cấp cao nhất
		{"editor", "low"},      // vai không tồn tại → mặc định cấp cao nhất
		{"", "low"},            // rỗng → mặc định cấp cao nhất
		{"default", "low"},     // default → mặc định cấp cao nhất
		{"coordinator", "low"}, // chưa cấu hình → mặc định cấp cao nhất
	}
	for _, c := range cases {
		if got := cfg.ResolveReasoningEffort(c.role); got != c.want {
			t.Errorf("ResolveReasoningEffort(%q) = %q, want %q", c.role, got, c.want)
		}
	}

	// Khi mặc định cấp cao nhất cũng rỗng, vai chưa ghi đè trả về "" (không ghi đè).
	empty := Config{Roles: map[string]RoleConfig{"writer": {ReasoningEffort: "xhigh"}}}
	if got := empty.ResolveReasoningEffort("editor"); got != "" {
		t.Errorf("khi mặc định rỗng, editor nên trả về \"\", nhận được %q", got)
	}
	if got := empty.ResolveReasoningEffort("writer"); got != "xhigh" {
		t.Errorf("khi mặc định rỗng, ghi đè writer nên có hiệu lực, nhận được %q", got)
	}
}

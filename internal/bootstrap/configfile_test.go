package bootstrap

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/voocel/ainovel-cli/internal/errs"
)

const validGlobal = `{
  "provider": "openrouter",
  "model": "google/gemini-2.5-flash",
  "providers": { "openrouter": { "api_key": "sk-test-123456" } }
}`

// writeGlobal ghi cấu hình toàn cục vào HOME được cách ly và trả về HOME đó.
func writeGlobal(t *testing.T, content string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".ainovel")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if content != "" {
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0o644); err != nil {
			t.Fatalf("write global: %v", err)
		}
	}
	return home
}

// writeProjectConfig ghi cấu hình cấp dự án vào ./.ainovel/ của thư mục làm việc hiện tại.
// Cần t.Chdir đến thư mục đích trước khi gọi.
func writeProjectConfig(t *testing.T, content string) {
	t.Helper()
	if err := os.MkdirAll(".ainovel", 0o755); err != nil {
		t.Fatalf("mkdir .ainovel: %v", err)
	}
	if err := os.WriteFile(filepath.Join(".ainovel", "config.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}
}

// Nguyên nhân gốc 3: ./.ainovel/config.json cấp dự án tồn tại nhưng là JSON lỗi, phải báo lỗi, không thể im lặng bỏ qua quay về toàn cục.
func TestLoadConfig_CorruptProjectFailsLoud(t *testing.T) {
	writeGlobal(t, validGlobal)
	proj := t.TempDir()
	t.Chdir(proj)
	// Ví dụ chép tay bị thêm dấu phẩy cuối — JSON lỗi phổ biến nhất.
	writeProjectConfig(t, `{ "model": "x", }`)

	if _, err := LoadConfig(""); err == nil {
		t.Fatal("./.ainovel/config.json lỗi phải báo lỗi nhưng đã bị im lặng bỏ qua")
	}
}

// Toàn cục là nền tảng ưu tiên thấp nhất: file lỗi không được chặn ghi đè --config ưu tiên cao hơn (bảo vệ hồi quy —
// phiên trước nhầm làm toàn cục cũng fail-loud, khiến người dùng "toàn cục lỗi + --config hợp lệ" bị chặn bởi file không liên quan).
func TestLoadConfig_CorruptGlobalDoesNotBlockOverride(t *testing.T) {
	writeGlobal(t, `{ not json`)
	proj := t.TempDir()
	t.Chdir(proj)
	good := filepath.Join(proj, "good.json")
	if err := os.WriteFile(good, []byte(validGlobal), 0o644); err != nil {
		t.Fatalf("write override: %v", err)
	}

	cfg, err := LoadConfig(good)
	if err != nil {
		t.Fatalf("toàn cục lỗi không nên chặn --config hợp lệ, nhận được: %v", err)
	}
	if cfg.Provider != "openrouter" {
		t.Errorf("nên dùng giá trị --config, nhận được provider=%q", cfg.Provider)
	}
}

// File không tồn tại là tình huống bình thường (portable/lần đầu), không được báo lỗi.
func TestLoadConfig_MissingFilesNoError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home) // ~/.ainovel/config.json không tồn tại
	t.Chdir(t.TempDir())   // cũng không có ./.ainovel/config.json

	if _, err := LoadConfig(""); err != nil {
		t.Fatalf("thiếu file cấu hình không nên báo lỗi, nhận được: %v", err)
	}
}

// Đường dẫn bình thường: toàn cục + cấp dự án hợp nhất có hiệu lực.
func TestLoadConfig_ValidMergeWorks(t *testing.T) {
	writeGlobal(t, validGlobal)
	proj := t.TempDir()
	t.Chdir(proj)
	writeProjectConfig(t, `{
  "model": "google/gemini-2.5-pro",
  "reasoning_effort": "high",
  "roles": {
    "writer": {
      "provider": "openrouter",
      "model": "google/gemini-2.5-flash",
      "reasoning_effort": "low"
    }
  }
}`)

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("cấu hình hợp lệ không nên báo lỗi: %v", err)
	}
	if cfg.Provider != "openrouter" {
		t.Errorf("provider nên giữ giá trị toàn cục openrouter, nhận được %q", cfg.Provider)
	}
	if cfg.ModelName != "google/gemini-2.5-pro" {
		t.Errorf("model nên bị ghi đè bởi cấp dự án, nhận được %q", cfg.ModelName)
	}
	if cfg.ReasoningEffort != "high" {
		t.Errorf("reasoning_effort nên bị ghi đè bởi cấp dự án, nhận được %q", cfg.ReasoningEffort)
	}
	if got := cfg.Roles["writer"].ReasoningEffort; got != "low" {
		t.Errorf("roles.writer.reasoning_effort nên bị ghi đè bởi cấp dự án, nhận được %q", got)
	}
}

func TestMergeConfig_ProviderExtraFields(t *testing.T) {
	base := Config{
		Provider:  "openrouter",
		ModelName: "google/gemini-2.5-flash",
		Providers: map[string]ProviderConfig{
			"openrouter": {
				API:    "chat",
				APIKey: "sk-test-123456",
				ExtraBody: map[string]any{
					"temperature": 0.8,
				},
				Extra: map[string]any{
					"user_agent": "base-client/1.0",
				},
			},
		},
	}
	overlay := Config{
		Providers: map[string]ProviderConfig{
			"openrouter": {
				API:     "responses",
				BaseURL: "https://proxy.example.com/v1",
				ExtraBody: map[string]any{
					"min_p": 0.05,
				},
				Extra: map[string]any{
					"user_agent": "override-client/1.0",
					"headers": map[string]any{
						"X-Custom-Client": "ainovel",
					},
				},
			},
		},
	}

	cfg := mergeConfig(base, overlay)
	pc := cfg.Providers["openrouter"]
	if pc.APIKey != "sk-test-123456" {
		t.Fatalf("APIKey = %q, want inherited key", pc.APIKey)
	}
	if pc.API != "responses" {
		t.Fatalf("API = %q, want responses", pc.API)
	}
	if pc.BaseURL != "https://proxy.example.com/v1" {
		t.Fatalf("BaseURL = %q, want overlay URL", pc.BaseURL)
	}
	if _, ok := pc.ExtraBody["temperature"]; ok {
		t.Fatalf("ExtraBody should be replaced by overlay, got %#v", pc.ExtraBody)
	}
	if got := pc.ExtraBody["min_p"]; got != 0.05 {
		t.Fatalf("ExtraBody[min_p] = %#v, want 0.05", got)
	}
	if got := pc.Extra["user_agent"]; got != "override-client/1.0" {
		t.Fatalf("Extra[user_agent] = %#v, want override-client/1.0", got)
	}
	headers, ok := pc.Extra["headers"].(map[string]any)
	if !ok {
		t.Fatalf("Extra[headers] missing or invalid: %#v", pc.Extra["headers"])
	}
	if got := headers["X-Custom-Client"]; got != "ainovel" {
		t.Fatalf("Extra.headers[X-Custom-Client] = %#v, want ainovel", got)
	}
}

// Nguyên nhân gốc 2 (tái hiện cốt lõi issue #37): cấp dự án ghi đè provider nhưng không khai báo thông tin xác thực providers tương ứng,
// ValidateBase phải báo lỗi config (chứ không phải để qua rồi crash sâu hơn).
func TestValidateBase_ProviderOverrideWithoutCredentials(t *testing.T) {
	cfg := Config{
		Provider:  "mimo",
		ModelName: "mimo-v2.5-pro",
		Providers: map[string]ProviderConfig{
			"openrouter": {APIKey: "sk-test-123456"},
		},
	}
	cfg.FillDefaults()
	err := cfg.ValidateBase()
	if err == nil {
		t.Fatal("provider thiếu thông tin xác thực phải báo lỗi")
	}
	if !errors.Is(err, errs.ErrConfig) {
		t.Errorf("nên bọc errs.ErrConfig, nhận được: %v", err)
	}
}

func TestValidateBaseRejectsInvalidProviderAPI(t *testing.T) {
	cfg := Config{
		Provider:  "openai",
		ModelName: "gpt-5.1",
		Providers: map[string]ProviderConfig{
			"openai": {APIKey: "sk-test-123456", API: "legacy"},
		},
	}
	cfg.FillDefaults()
	err := cfg.ValidateBase()
	if err == nil {
		t.Fatal("provider api không hợp lệ phải báo lỗi")
	}
	if !errors.Is(err, errs.ErrConfig) {
		t.Errorf("nên bọc errs.ErrConfig, nhận được: %v", err)
	}
}

func TestValidateBaseRejectsProviderAPIOnNonOpenAIProvider(t *testing.T) {
	cfg := Config{
		Provider:  "anthropic",
		ModelName: "claude-sonnet-4",
		Providers: map[string]ProviderConfig{
			"anthropic": {APIKey: "sk-test-123456", API: "responses"},
		},
	}
	cfg.FillDefaults()
	err := cfg.ValidateBase()
	if err == nil {
		t.Fatal("provider không phải OpenAI cấu hình api phải báo lỗi")
	}
	if !errors.Is(err, errs.ErrConfig) {
		t.Errorf("nên bọc errs.ErrConfig, nhận được: %v", err)
	}
}

// Cấu hình mẫu phải tự nhất quán: sau khi bỏ chú thích là JSON hợp lệ,
// con trỏ provider cấp cao nhất không được treo, và phải nêu rõ tư duy "con trỏ" -- đây là mẫu để người dùng sao chép, tự nó lỗi sẽ hại người.
func TestExampleConfigIsValidAndSelfConsistent(t *testing.T) {
	if exampleConfig == "" {
		t.Fatal("go:embed không có hiệu lực, exampleConfig rỗng")
	}
	rootExample, err := os.ReadFile(filepath.Join("..", "..", "config.example.jsonc"))
	if err != nil {
		t.Fatalf("đọc config.example.jsonc thư mục gốc: %v", err)
	}
	if string(rootExample) != exampleConfig {
		t.Fatal("config.example.jsonc thư mục gốc và internal/bootstrap/config.example.jsonc không nhất quán")
	}
	var cfg Config
	if err := json.Unmarshal(stripJSONComments([]byte(exampleConfig)), &cfg); err != nil {
		t.Fatalf("ví dụ nội tích sau khi bỏ chú thích không phải JSON hợp lệ (người dùng sao chép sẽ bị lỗi): %v", err)
	}
	if cfg.Provider == "" || cfg.ModelName == "" {
		t.Fatal("ví dụ nên cung cấp provider/model mặc định")
	}
	if _, ok := cfg.Providers[cfg.Provider]; !ok {
		t.Errorf("provider %q cấp cao nhất của ví dụ không trỏ đến mục trong providers — mẫu con trỏ chính mình treo rồi", cfg.Provider)
	}
	if !contains(exampleConfig, "con trỏ") {
		t.Error("ví dụ nên nêu rõ 'provider là con trỏ' — đừng để bẫy nhận thức của #37 quay lại")
	}
}

func TestWriteStartupError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path := WriteStartupError("boom: provider not configured")
	if path == "" {
		t.Fatal("nên trả về đường dẫn ghi đĩa")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc last-error.log: %v", err)
	}
	if want := "boom: provider not configured"; !contains(string(data), want) {
		t.Errorf("nhật ký nên chứa %q, thực tế: %s", want, data)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

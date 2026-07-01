package web

import (
	"net/http"
	"strings"

	"github.com/voocel/ainovel-cli/internal/bootstrap"
)

// setupProviderInfo là thông tin meta của provider cần thiết cho form thiết lập lần đầu (phản chiếu bootstrap.setupProviders).
type setupProviderInfo struct {
	Name           string `json:"name"`
	Label          string `json:"label"`
	BaseURL        string `json:"baseURL"`
	NeedType       bool   `json:"needType"`
	APIKeyOptional bool   `json:"apiKeyOptional"`
	// SampleModel là ví dụ tên mô hình của provider, dùng làm placeholder form.
	// Quan trọng: provider gốc (như gemini/anthropic) dùng tên mô hình "trần",
	// chỉ OpenRouter mới dùng tiền tố "nhà sản xuất/mô hình". Placeholder sai sẽ khiến người dùng điền
	// tên OpenRouter như google/gemini-2.5-flash vào Gemini gốc,
	// dẫn đến .../models/google/gemini-2.5-flash:generateContent mà 404.
	SampleModel string `json:"sampleModel"`
}

var setupCatalog = []setupProviderInfo{
	{Name: "openrouter", Label: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1", SampleModel: "google/gemini-2.5-flash"},
	{Name: "anthropic", Label: "Anthropic", SampleModel: "claude-sonnet-4-5"},
	{Name: "gemini", Label: "Gemini", SampleModel: "gemini-2.5-flash"},
	{Name: "openai", Label: "OpenAI", SampleModel: "gpt-4o"},
	{Name: "deepseek", Label: "DeepSeek", SampleModel: "deepseek-chat"},
	{Name: "qwen", Label: "Qwen", SampleModel: "qwen-plus"},
	{Name: "glm", Label: "GLM", SampleModel: "glm-4-plus"},
	{Name: "grok", Label: "Grok", SampleModel: "grok-2-latest"},
	{Name: "ollama", Label: "Ollama", BaseURL: "http://localhost:11434/v1", APIKeyOptional: true, SampleModel: "llama3.1"},
	{Name: "bedrock", Label: "Bedrock", APIKeyOptional: true, SampleModel: "anthropic.claude-3-5-sonnet-20241022-v2:0"},
	{Name: "custom", Label: "Custom Proxy", NeedType: true, APIKeyOptional: true, SampleModel: "gpt-4o"},
}

type setupRequest struct {
	Provider   string `json:"provider"`   // name trong danh mục
	CustomName string `json:"customName"` // tên provider khi là custom
	Type       string `json:"type"`       // loại giao thức khi là custom
	APIKey     string `json:"apiKey"`
	BaseURL    string `json:"baseURL"`
	Model      string `json:"model"`
}

// newSetupMux xây dựng mux cho giai đoạn thiết lập lần đầu: trang thiết lập + API danh mục + API lưu.
// Sau khi lưu thành công thì ghi vào done, để RunWeb chuyển sang bàn làm việc chính thức.
func newSetupMux(configPath string, done chan<- struct{}) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, err := staticFS.ReadFile("static/setup.html")
		if err != nil {
			http.Error(w, "setup page missing", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})

	mux.HandleFunc("GET /styles.css", func(w http.ResponseWriter, _ *http.Request) {
		data, _ := staticFS.ReadFile("static/styles.css")
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(data)
	})

	mux.HandleFunc("GET /api/setup/providers", func(w http.ResponseWriter, _ *http.Request) {
		writeOK(w, setupCatalog)
	})

	mux.HandleFunc("POST /api/setup", func(w http.ResponseWriter, r *http.Request) {
		var req setupRequest
		if err := decodeBody(r, &req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		cfg, err := buildSetupConfig(req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		path := configPath
		if path == "" {
			path = bootstrap.DefaultConfigPath()
		}
		if err := bootstrap.SaveConfig(path, cfg); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeOK(w, map[string]any{"ok": true})
		// Thông báo cho RunWeb chuyển sang bàn làm việc chính thức (không chặn).
		select {
		case done <- struct{}{}:
		default:
		}
	})

	return mux
}

func buildSetupConfig(req setupRequest) (bootstrap.Config, error) {
	var info *setupProviderInfo
	for i := range setupCatalog {
		if setupCatalog[i].Name == req.Provider {
			info = &setupCatalog[i]
			break
		}
	}
	if info == nil {
		return bootstrap.Config{}, errMsg("nhà cung cấp không hợp lệ")
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return bootstrap.Config{}, errMsg("vui lòng nhập tên mô hình")
	}

	providerName := info.Name
	var pc bootstrap.ProviderConfig
	if info.NeedType {
		providerName = strings.TrimSpace(req.CustomName)
		if providerName == "" {
			return bootstrap.Config{}, errMsg("vui lòng nhập tên nhà cung cấp tùy chỉnh")
		}
		pc.Type = strings.TrimSpace(req.Type)
	}
	if !info.APIKeyOptional && strings.TrimSpace(req.APIKey) == "" {
		return bootstrap.Config{}, errMsg("vui lòng nhập API key")
	}
	pc.APIKey = strings.TrimSpace(req.APIKey)

	baseURL := strings.TrimSpace(req.BaseURL)
	if baseURL == "" {
		baseURL = info.BaseURL
	}
	pc.BaseURL = baseURL

	return bootstrap.Config{
		Provider:  providerName,
		ModelName: model,
		Providers: map[string]bootstrap.ProviderConfig{providerName: pc},
		Roles:     map[string]bootstrap.RoleConfig{},
		Style:     "default",
	}, nil
}

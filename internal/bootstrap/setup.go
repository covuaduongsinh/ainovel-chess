package bootstrap

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/voocel/ainovel-cli/internal/rules"
	"github.com/voocel/ainovel-cli/internal/utils"
)

// exampleConfig là template có chú thích được ghi vào ~/.ainovel/config.example.jsonc sau khi hướng dẫn.
// File nhúng phải nhất quán với config.example.jsonc ở thư mục gốc repo, test sẽ ngăn sự trôi dạt.
//
//go:embed config.example.jsonc
var exampleConfig string

// NeedsSetup kiểm tra có cần hướng dẫn lần đầu không (kích hoạt khi file cấu hình không tồn tại).
func NeedsSetup(flagPath string) bool {
	if flagPath != "" {
		_, err := os.Stat(flagPath)
		return os.IsNotExist(err)
	}
	if p := DefaultConfigPath(); p != "" {
		if _, err := os.Stat(p); err == nil {
			return false
		}
	}
	if _, err := os.Stat(projectConfigPath()); err == nil {
		return false
	}
	return true
}

type setupProvider struct {
	name           string
	label          string
	baseURL        string // base_url được điền sẵn
	needType       bool   // proxy tùy chỉnh cần hỏi thêm type và base_url
	apiKeyOptional bool   // true nghĩa là API Key cho phép để trống
}

var setupProviders = []setupProvider{
	{name: "openrouter", label: "OpenRouter", baseURL: "https://openrouter.ai/api/v1"},
	{name: ClaudeCodeProvider, label: "Claude Code (thuê bao qua proxy nội bộ)", baseURL: ClaudeCodeDefaultBaseURL, apiKeyOptional: true},
	{name: "anthropic", label: "Anthropic"},
	{name: "gemini", label: "Gemini"},
	{name: "openai", label: "OpenAI"},
	{name: "deepseek", label: "DeepSeek"},
	{name: "qwen", label: "Qwen"},
	{name: "glm", label: "GLM"},
	{name: "grok", label: "Grok"},
	{name: "ollama", label: "Ollama", baseURL: "http://localhost:11434/v1", apiKeyOptional: true},
	{name: "bedrock", label: "Bedrock", apiKeyOptional: true},
	{name: "custom", label: "Custom Proxy", needType: true, apiKeyOptional: true},
}

// RunSetup chạy hướng dẫn lần đầu, trả về cấu hình đã tạo.
func RunSetup() (Config, error) {
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).
		Render("Không tìm thấy file cấu hình, bắt đầu thiết lập ban đầu..."))
	fmt.Fprintf(os.Stderr, "  Đường dẫn file cấu hình: %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(DefaultConfigPath()))
	fmt.Fprintf(os.Stderr, "  Sau khi hoàn thành có thể chỉnh sửa file này bất kỳ lúc nào để điều chỉnh cài đặt nâng cao.\n")
	fmt.Fprintln(os.Stderr)

	// Bước 1: Chọn Provider
	sp, err := runProviderSelect()
	if err != nil {
		return Config{}, err
	}

	providerName := sp.name
	var pc ProviderConfig
	printStepDone("Provider", sp.label)

	// Proxy tùy chỉnh: hỏi thêm tên và loại giao thức API
	if sp.needType {
		providerName, err = runTextInput("Tên Provider", "my-proxy")
		if err != nil {
			return Config{}, err
		}
		providerType, err := runTypeSelect()
		if err != nil {
			return Config{}, err
		}
		pc.Type = providerType
	}

	// Bước 2: Nhập API Key
	var apiKey string
	if sp.apiKeyOptional {
		apiKey, err = runOptionalTextInput("[2/4] API Key (có thể để trống)", "Để trống nghĩa là không dùng API Key")
	} else {
		apiKey, err = runTextInput("[2/4] API Key", "sk-xxx")
	}
	if err != nil {
		return Config{}, err
	}
	pc.APIKey = apiKey
	if apiKey == "" {
		printStepDone("API Key", "chưa đặt")
	} else {
		printStepDone("API Key", maskKey(apiKey))
	}

	// Bước 3: Base URL (nhấn Enter để dùng địa chỉ chính thức mặc định)
	baseDefault := sp.baseURL
	baseHint := "Để trống dùng địa chỉ chính thức"
	if baseDefault != "" {
		baseHint = baseDefault
	}
	baseURL, err := runTextInputWithDefault("[3/4] Base URL (nhấn Enter dùng mặc định, người dùng proxy điền địa chỉ proxy)", baseHint, baseDefault)
	if err != nil {
		return Config{}, err
	}
	pc.BaseURL = baseURL
	if baseURL != "" {
		printStepDone("Base URL", baseURL)
	} else {
		printStepDone("Base URL", "mặc định")
	}

	// Claude Code: giao thức anthropic + danh mục model dựng sẵn (không cần gõ tên model),
	// tùy chọn bật preset tự-chọn cân bằng thay cho bước nhập model tự do.
	if sp.name == ClaudeCodeProvider {
		// Dùng chung helper: type anthropic + danh mục model + tự chèn api_key giữ chỗ khi để trống
		// (litellm đòi x-api-key khác rỗng, cầu nối nội bộ bỏ qua giá trị này).
		pc = ClaudeCodeProviderConfig(baseURL, apiKey)

		useBalanced, err := runConfirm("Bật tự-chọn model cân bằng? (Opus 4.8 cho Writer/Architect, Sonnet 4.6 cho Coordinator/Editor)")
		if err != nil {
			return Config{}, err
		}
		cfg := Config{
			Provider:  ClaudeCodeProvider,
			ModelName: ClaudeDefaultModel,
			Providers: map[string]ProviderConfig{ClaudeCodeProvider: pc},
			Roles:     map[string]RoleConfig{},
			Style:     "default",
		}
		if useBalanced {
			cfg.Roles = BalancedRoleConfigs(ClaudeCodeProvider)
			cfg.ReasoningEffort = "medium"
			printStepDone("Tự-chọn model", "Cân bằng (Writer/Architect: Opus 4.8; Coordinator/Editor: Sonnet 4.6)")
		} else {
			printStepDone("Model", ClaudeDefaultModel)
		}
		if err := writeSetupConfig(cfg); err != nil {
			return cfg, err
		}
		return cfg, nil
	}

	// Bước 4: Tên mô hình (bắt buộc)
	modelName, err := runTextInput("[4/4] Tên mô hình", "Ví dụ: gpt-4o / claude-sonnet-4 / gemini-2.5-pro")
	if err != nil {
		return Config{}, err
	}
	printStepDone("Model", modelName)

	cfg := Config{
		Provider:  providerName,
		ModelName: modelName,
		Providers: map[string]ProviderConfig{providerName: pc},
		Roles:     map[string]RoleConfig{},
		Style:     "default",
	}

	if err := writeSetupConfig(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// writeSetupConfig lưu cấu hình wizard xuống đĩa, tạo template chú thích và in tóm tắt.
func writeSetupConfig(cfg Config) error {
	path := DefaultConfigPath()
	if err := SaveConfig(path, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	// Tạo template có chú thích
	saveExampleConfig()

	// Thư mục tùy chọn toàn cục được tạo thống nhất bởi quy trình khởi động (runWithConfig), ở đây chỉ lấy đường dẫn để hiển thị
	rulesDir := rules.DefaultHomeRulesDir()

	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "%s Cấu hình đã được lưu vào %s\n",
		lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✓"), path)
	if len(cfg.Roles) > 0 {
		fmt.Fprintf(os.Stderr, "  Model mặc định: %s (một số vai dùng model riêng — xem /model)\n", cfg.ModelName)
	} else {
		fmt.Fprintf(os.Stderr, "  Mô hình mặc định: %s\n", cfg.ModelName)
	}
	fmt.Fprintln(os.Stderr, "  Nếu muốn cấu hình mô hình khác nhau theo vai, chỉnh sửa file cấu hình là được.")
	if rulesDir != "" {
		fmt.Fprintf(os.Stderr, "  Tùy chọn viết toàn cục có thể đặt file .md trong %s (xem README.txt trong đó)\n", rulesDir)
	}
	fmt.Fprintln(os.Stderr)
	return nil
}

func saveExampleConfig() {
	dir, err := configDir()
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "config.example.jsonc"), []byte(exampleConfig), 0o644)
}

// printStepDone in dòng xác nhận hoàn thành một bước.
func printStepDone(label, value string) {
	fmt.Fprintf(os.Stderr, "  %s %s: %s\n",
		lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✓"),
		label,
		lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(value))
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// ---------- Thành phần TUI ----------

func runProviderSelect() (setupProvider, error) {
	m := setupSelectModel{
		title: "[1/4] Chọn Provider",
		items: setupProviders,
	}
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		return setupProvider{}, err
	}
	result := final.(setupSelectModel)
	if result.cancelled {
		return setupProvider{}, fmt.Errorf("setup cancelled")
	}
	return result.items[result.cursor], nil
}

var apiTypeOptions = []setupProvider{
	{name: "openai", label: "Tương thích OpenAI"},
	{name: "anthropic", label: "Tương thích Anthropic"},
	{name: "gemini", label: "Tương thích Gemini"},
}

func runTypeSelect() (string, error) {
	m := setupSelectModel{
		title: "Loại giao thức API",
		items: apiTypeOptions,
	}
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	result := final.(setupSelectModel)
	if result.cancelled {
		return "", fmt.Errorf("setup cancelled")
	}
	return result.items[result.cursor].name, nil
}

// runConfirm hiển thị lựa chọn Có/Không, trả về true khi người dùng chọn "Có".
func runConfirm(title string) (bool, error) {
	m := setupSelectModel{
		title: title,
		items: []setupProvider{{name: "yes", label: "Có"}, {name: "no", label: "Không"}},
	}
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		return false, err
	}
	result := final.(setupSelectModel)
	if result.cancelled {
		return false, fmt.Errorf("setup cancelled")
	}
	return result.items[result.cursor].name == "yes", nil
}

func runTextInput(label, placeholder string) (string, error) {
	return runTextInputWithDefault(label, placeholder, "")
}

func runOptionalTextInput(label, placeholder string) (string, error) {
	m := setupInputModel{label: label, placeholder: placeholder, allowEmpty: true}
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	result := final.(setupInputModel)
	if result.cancelled {
		return "", fmt.Errorf("setup cancelled")
	}
	return utils.CleanInputLine(result.value), nil
}

func runTextInputWithDefault(label, placeholder, defaultValue string) (string, error) {
	m := setupInputModel{label: label, placeholder: placeholder, defaultValue: defaultValue}
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	result := final.(setupInputModel)
	if result.cancelled {
		return "", fmt.Errorf("setup cancelled")
	}
	if result.value == "" && result.defaultValue != "" {
		return result.defaultValue, nil
	}
	return utils.CleanInputLine(result.value), nil
}

// ---------- Bộ chọn ----------

var (
	setupCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	setupDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	setupHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	setupInputStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
)

type setupSelectModel struct {
	title     string
	items     []setupProvider
	cursor    int
	cancelled bool
}

func (m setupSelectModel) Init() tea.Cmd { return nil }

func (m setupSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter":
			return m, tea.Quit
		case "q", "esc", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m setupSelectModel) View() string {
	var b strings.Builder
	b.WriteString(setupHeaderStyle.Render(m.title))
	b.WriteString("\n\n")
	for i, item := range m.items {
		cursor := "  "
		label := item.label
		if i == m.cursor {
			cursor = setupCursorStyle.Render("❯ ")
			label = setupCursorStyle.Render(label)
		}
		b.WriteString(cursor + label + "\n")
	}
	b.WriteString(setupDimStyle.Render("\n  ↑↓ Chọn  Enter Xác nhận  Esc Hủy"))
	return b.String()
}

// ---------- Nhập văn bản ----------

type setupInputModel struct {
	label        string
	placeholder  string
	defaultValue string // giá trị mặc định khi nhấn Enter trực tiếp
	allowEmpty   bool   // cho phép nhập giá trị rỗng trực tiếp
	value        string
	cancelled    bool
}

func (m setupInputModel) Init() tea.Cmd { return nil }

func (m setupInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "enter":
			if utils.CleanInputLine(m.value) != "" || m.defaultValue != "" || m.allowEmpty {
				return m, tea.Quit
			}
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "backspace":
			if len(m.value) > 0 {
				runes := []rune(m.value)
				m.value = string(runes[:len(runes)-1])
			}
		default:
			if msg.Type == tea.KeyRunes {
				m.value += utils.CleanInputRunes(msg.Runes)
			} else if msg.Type == tea.KeySpace {
				m.value += " "
			}
		}
	}
	return m, nil
}

func (m setupInputModel) View() string {
	var b strings.Builder
	b.WriteString(setupHeaderStyle.Render(m.label))
	b.WriteString("\n\n")
	b.WriteString(setupInputStyle.Render("❯ "))
	if m.value == "" {
		b.WriteString(setupCursorStyle.Render("▌"))
		b.WriteString(setupDimStyle.Render(m.placeholder))
	} else {
		b.WriteString(m.value)
		b.WriteString(setupCursorStyle.Render("▌"))
	}
	b.WriteString(setupDimStyle.Render("  (Enter xác nhận, Esc hủy)"))
	b.WriteString("\n")
	return b.String()
}

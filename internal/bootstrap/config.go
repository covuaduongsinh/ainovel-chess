package bootstrap

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/voocel/agentcore/llm"
	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/models"
	"github.com/voocel/ainovel-cli/internal/utils"
)

// DefaultContextWindow là kích thước cửa sổ dự phòng khi mô hình chưa được đăng ký trong registry.
const DefaultContextWindow = 200000

// CompactRatio là ngưỡng tương đối kích hoạt nén ngữ cảnh: nén khi tokens >= window * CompactRatio.
// 0.85 là giá trị kinh nghiệm, để lại 15% không gian đầu cho "prompt vòng tiếp theo + kết quả tool lớn",
// đồng thời cho phép mô hình cửa sổ lớn nén chủ động ở 85%, tránh chờ đầy mới nén dưới cửa sổ danh nghĩa 1M (vùng suy giảm chú ý).
//
// Không phơi bày cho người dùng cấu hình: cùng nguồn gốc với context_window đã xóa —
// trong kiến trúc đa mô hình, để người dùng vặn núm số qua lại không bằng cố định một giá trị hợp lý trong code.
const CompactRatio = 0.85

// MinCompactReserve là giới hạn dưới của ReserveTokens. Mô hình cửa sổ nhỏ (như qwen3:8b 32k cục bộ)
// theo tỷ lệ 0.15 chỉ có reserve 4800, một lần phản hồi tool commit_chapter có thể chiếm 5-8k,
// một chương văn bản 8-15k — sẽ xảy ra "vừa nén xong đã vượt lại". 8000 dự phòng đảm bảo kịch bản tệ nhất vẫn còn nửa vòng đệm.
const MinCompactReserve = 8000

// CompactReserveTokens tính ngược ReserveTokens theo CompactRatio và áp dụng sàn MinCompactReserve:
//
//	threshold = window - reserve = window * CompactRatio
//	reserve   = max(MinCompactReserve, window * (1 - CompactRatio))
//
// Dùng cho EngineConfig.ReserveTokens của agentcore.context.Engine.
func CompactReserveTokens(window int) int {
	if window <= 0 {
		return 0
	}
	reserve := window - int(float64(window)*CompactRatio)
	if reserve < MinCompactReserve {
		return MinCompactReserve
	}
	return reserve
}

// ProviderConfig định nghĩa thông tin xác thực của một nhà cung cấp LLM.
type ProviderConfig struct {
	Type    string   `json:"type,omitempty"`     // loại giao thức API (openai/anthropic/gemini), chỉ định khi dùng proxy tùy chỉnh
	API     string   `json:"api,omitempty"`      // endpoint giao thức OpenAI: chat (mặc định) / responses
	APIKey  string   `json:"api_key,omitempty"`  // khóa API
	BaseURL string   `json:"base_url,omitempty"` // API Base URL
	Models  []string `json:"models,omitempty"`   // danh sách mô hình tùy chọn, hiển thị khi TUI chuyển đổi
	// ExtraBody chuyển tiếp các tham số bổ sung cho mỗi yêu cầu của provider (như temperature/top_p/min_p/
	// presence_penalty, hoặc khóa đặc thù của nhà sản xuất như chat_template_kwargs của nvidia để bật think).
	// Phía tương thích OpenAI sẽ hợp nhất từng chữ vào thân yêu cầu (tức quy ước extra_body); giá trị do người dùng tự chịu trách nhiệm.
	ExtraBody map[string]any `json:"extra_body,omitempty"`
	// Extra chuyển tiếp cấu hình cấp provider (litellm.ProviderConfig.Extra), dùng cho HTTP
	// headers, user_agent, anthropic_beta và các tùy chọn client/tầng truyền tải.
	Extra map[string]any `json:"extra,omitempty"`
}

// RequiresAPIKey trả về provider này có cần cấu hình api_key tường minh không.
// Quy ước:
// 1. ollama / bedrock cho phép không có key;
// 2. Cấu hình chỉ định Type tường minh được coi là proxy tùy chỉnh, cho phép không có key;
// 3. Các provider khác mặc định yêu cầu key, duy trì kiểm tra bảo thủ với giao diện được lưu trữ chính thức.
func (pc ProviderConfig) RequiresAPIKey(name string) bool {
	switch name {
	case "ollama", "bedrock":
		return false
	}
	return pc.Type == ""
}

// ProviderType trả về loại giao thức API hợp lệ.
// Ưu tiên dùng Type tường minh; nếu không thì yêu cầu tên provider đã có trong registry litellm.
func (pc ProviderConfig) ProviderType(name string) (string, error) {
	if pc.Type != "" {
		return pc.Type, nil
	}
	if llm.IsProviderRegistered(name) {
		return name, nil
	}
	return "", fmt.Errorf("provider %q thiếu type và không có trong danh sách provider đã biết của litellm: %w", name, errs.ErrConfig)
}

// ModelRef biểu thị một tổ hợp provider/model.
type ModelRef struct {
	Provider string `json:"provider"` // tên provider (key trong map Providers)
	Model    string `json:"model"`    // tên mô hình (truyền nguyên vẹn, không phân tích)
}

// RoleConfig định nghĩa ghi đè mô hình cho một vai cụ thể.
type RoleConfig struct {
	Provider  string     `json:"provider"`            // tên provider chính (key trong map Providers)
	Model     string     `json:"model"`               // tên mô hình chính (truyền nguyên vẹn, không phân tích)
	Fallbacks []ModelRef `json:"fallbacks,omitempty"` // danh sách provider/model dự phòng tường minh
	// ReasoningEffort mức độ suy luận của vai này (off/low/medium/high/xhigh/max), rỗng=kế thừa mặc định cấp cao nhất.
	// Được xác thực bởi agents.ParseThinkingLevel rồi áp dụng, giá trị vượt cấp coi như rỗng.
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

// knownRoles các tên vai được hỗ trợ.
var knownRoles = map[string]bool{
	"coordinator": true,
	"architect":   true,
	"writer":      true,
	"editor":      true,
}

// Config cấu hình ứng dụng tiểu thuyết.
type Config struct {
	// Trường runtime (không serialize sang JSON)
	OutputDir string `json:"-"` // thư mục gốc đầu ra

	// Cấu hình LLM mặc định
	Provider  string `json:"provider"` // provider mặc định (key trong map Providers)
	ModelName string `json:"model"`    // tên mô hình mặc định
	// ReasoningEffort mức độ suy luận mặc định cấp cao nhất (off/low/medium/high/xhigh/max), rỗng=không ghi đè (dùng mặc định mô hình/provider).
	// Khi vai chưa cấu hình reasoning_effort riêng thì dùng giá trị này.
	ReasoningEffort string `json:"reasoning_effort,omitempty"`

	// Kho thông tin xác thực provider
	Providers map[string]ProviderConfig `json:"providers,omitempty"`

	// Ghi đè mô hình cấp vai
	Roles map[string]RoleConfig `json:"roles,omitempty"`

	// Tham số sáng tác
	Style string `json:"style,omitempty"`

	// ContextWindow kích thước cửa sổ dùng để nén ngữ cảnh. Khi để trống (0) sẽ tự động giải quyết theo tên mô hình:
	// registry trúng thì dùng cửa sổ thực của mô hình, không trúng thì dùng DefaultContextWindow dự phòng.
	// Khi cấu hình tường minh thì ưu tiên áp dụng — dùng để chỉ định cửa sổ thực cho mô hình tùy chỉnh registry không tìm thấy,
	// hoặc ghim mô hình cửa sổ lớn ở giá trị nhỏ hơn để kích hoạt nén sớm hơn (cửa sổ danh nghĩa 1M thường đã suy giảm chú ý ở 200k+).
	// Chỉ ảnh hưởng ngưỡng nén, không thay đổi độ dài yêu cầu thực tế tới LLM API; giá trị cấu hình do người dùng tự chịu trách nhiệm.
	ContextWindow int `json:"context_window,omitempty"`

	// Budget chính sách ngân sách chi phí cho mỗi cuốn sách; chỉ kích hoạt khi book_usd > 0.
	Budget BudgetConfig `json:"budget,omitzero"`

	// Notify cấu hình cảnh báo không người giám sát; mặc định kích hoạt (kênh system dự phòng).
	Notify NotifyConfig `json:"notify,omitzero"`
}

// BudgetConfig là khai báo chính sách ví tiền cho mỗi cuốn sách của người dùng. Dừng khi vượt ngưỡng tương đương người dùng
// thủ công Abort tại thời điểm đó — Host chỉ thực thi thay, không đánh giá hành vi mô hình (ranh giới hợp hiến kiến trúc §10).
type BudgetConfig struct {
	BookUSD   float64 `json:"book_usd,omitempty"`   // phải điền mới kích hoạt; 0/thiếu = không giới hạn
	WarnRatio float64 `json:"warn_ratio,omitempty"` // mức cảnh báo, mặc định 0.8
	HardStop  bool    `json:"hard_stop,omitempty"`  // true=dừng ngay khi vượt; mặc định chờ tác vụ sub-agent hiện tại kết thúc
}

// Enabled trả về chính sách ngân sách có được kích hoạt không.
func (b BudgetConfig) Enabled() bool { return b.BookUSD > 0 }

// NotifyConfig cấu hình kênh cảnh báo không người giám sát.
type NotifyConfig struct {
	Enabled *bool    `json:"enabled,omitempty"` // mặc định true (kênh system dùng được mà không cần cấu hình)
	Command string   `json:"command,omitempty"` // tùy chọn, khi cấu hình sẽ thay thế kênh system (push điện thoại đi qua đây)
	Events  []string `json:"events,omitempty"`  // tùy chọn, lọc kind (run_end/repeat/budget), mặc định mở tất cả
}

// IsEnabled trả về cảnh báo có được kích hoạt không (mặc định true).
func (n NotifyConfig) IsEnabled() bool { return n.Enabled == nil || *n.Enabled }

// ValidateBase kiểm tra cấu hình cơ bản.
func (c *Config) ValidateBase() error {
	if err := validateConfigText("provider", c.Provider); err != nil {
		return err
	}
	if err := validateConfigText("model", c.ModelName); err != nil {
		return err
	}

	if c.Provider == "" {
		return fmt.Errorf("provider is required: %w", errs.ErrConfig)
	}
	if c.ModelName == "" {
		return fmt.Errorf("model is required: %w", errs.ErrConfig)
	}

	// Provider mặc định phải có thông tin xác thực
	pc, ok := c.Providers[c.Provider]
	if !ok {
		return fmt.Errorf("provider %q chưa cấu hình thông tin xác thực trong providers; nếu ghi đè provider trong ./.ainovel/config.json, cần khai báo đồng thời providers.%s (có api_key/base_url), không thể chỉ đổi provider cấp cao nhất: %w", c.Provider, c.Provider, errs.ErrConfig)
	}
	if pc.RequiresAPIKey(c.Provider) && pc.APIKey == "" {
		return fmt.Errorf("provider %q has no api_key configured: %w", c.Provider, errs.ErrConfig)
	}
	if err := validateProviderConfigText(c.Provider, pc); err != nil {
		return err
	}
	if err := c.validateProviderAPI("default", c.Provider, pc); err != nil {
		return err
	}
	for name, provider := range c.Providers {
		if err := validateConfigText("provider name", name); err != nil {
			return err
		}
		if err := validateProviderConfigText(name, provider); err != nil {
			return err
		}
		if err := c.validateProviderAPI(fmt.Sprintf("provider %q", name), name, provider); err != nil {
			return err
		}
	}

	// Kiểm tra ghi đè vai
	for role, rc := range c.Roles {
		if err := validateConfigText("role name", role); err != nil {
			return err
		}
		if err := validateConfigText(fmt.Sprintf("role %q provider", role), rc.Provider); err != nil {
			return err
		}
		if err := validateConfigText(fmt.Sprintf("role %q model", role), rc.Model); err != nil {
			return err
		}
		if !knownRoles[role] {
			return fmt.Errorf("unknown role %q in roles config (valid: coordinator/architect/writer/editor): %w", role, errs.ErrConfig)
		}
		if rc.Provider == "" || rc.Model == "" {
			return fmt.Errorf("role %q must have both provider and model: %w", role, errs.ErrConfig)
		}
		if err := c.validateModelRef(
			fmt.Sprintf("role %q", role),
			ModelRef{Provider: rc.Provider, Model: rc.Model},
		); err != nil {
			return err
		}
		for i, fallback := range rc.Fallbacks {
			if err := validateConfigText(fmt.Sprintf("role %q fallback[%d] provider", role, i), fallback.Provider); err != nil {
				return err
			}
			if err := validateConfigText(fmt.Sprintf("role %q fallback[%d] model", role, i), fallback.Model); err != nil {
				return err
			}
			if err := c.validateModelRef(
				fmt.Sprintf("role %q fallback[%d]", role, i),
				fallback,
			); err != nil {
				return err
			}
		}
	}

	// Kiểm tra chính sách ngân sách
	if c.Budget.BookUSD < 0 {
		return fmt.Errorf("budget.book_usd must be >= 0: %w", errs.ErrConfig)
	}
	if c.Budget.Enabled() && (c.Budget.WarnRatio <= 0 || c.Budget.WarnRatio >= 1) {
		return fmt.Errorf("budget.warn_ratio must be in (0, 1): %w", errs.ErrConfig)
	}

	// Kiểm tra cấu hình cảnh báo
	if err := validateConfigText("notify.command", c.Notify.Command); err != nil {
		return err
	}
	for _, ev := range c.Notify.Events {
		if !knownNotifyEvents[ev] {
			return fmt.Errorf("unknown notify event %q (valid: run_end/repeat/budget): %w", ev, errs.ErrConfig)
		}
	}

	return nil
}

var knownNotifyEvents = map[string]bool{"run_end": true, "repeat": true, "budget": true}

func validateProviderConfigText(name string, pc ProviderConfig) error {
	fields := []struct {
		label string
		value string
	}{
		{label: fmt.Sprintf("provider %q type", name), value: pc.Type},
		{label: fmt.Sprintf("provider %q api", name), value: pc.API},
		{label: fmt.Sprintf("provider %q api_key", name), value: pc.APIKey},
		{label: fmt.Sprintf("provider %q base_url", name), value: pc.BaseURL},
	}
	for _, field := range fields {
		if err := validateConfigText(field.label, field.value); err != nil {
			return err
		}
	}
	for i, model := range pc.Models {
		if err := validateConfigText(fmt.Sprintf("provider %q models[%d]", name, i), model); err != nil {
			return err
		}
	}
	switch pc.API {
	case "", "chat", "responses":
	default:
		return fmt.Errorf("provider %q api must be chat or responses: %w", name, errs.ErrConfig)
	}
	return nil
}

func validateConfigText(name, value string) error {
	if utils.ContainsControl(value) {
		return fmt.Errorf("%s contains control character: %w", name, errs.ErrConfig)
	}
	return nil
}

// DefaultProviderConfig trả về cấu hình thông tin xác thực của provider mặc định.
func (c *Config) DefaultProviderConfig() ProviderConfig {
	if c.Providers == nil {
		return ProviderConfig{}
	}
	return c.Providers[c.Provider]
}

// FillDefaults điền các giá trị mặc định.
func (c *Config) FillDefaults() {
	if c.OutputDir == "" {
		c.OutputDir = filepath.Join("output", "novel")
	}
	if c.Providers == nil {
		c.Providers = make(map[string]ProviderConfig)
	}
	if c.Roles == nil {
		c.Roles = make(map[string]RoleConfig)
	}
	if c.Style == "" {
		c.Style = "default"
	}
	if c.Budget.Enabled() && c.Budget.WarnRatio == 0 {
		c.Budget.WarnRatio = 0.8
	}
}

// ContextWindowSource đánh dấu nguồn gốc giá trị cửa sổ, dùng cho nhật ký/chẩn đoán.
type ContextWindowSource string

const (
	CtxWindowConfig   ContextWindowSource = "config"   // context_window trong file cấu hình chỉ định tường minh
	CtxWindowRegistry ContextWindowSource = "registry" // trúng baseline OpenRouter
	CtxWindowDefault  ContextWindowSource = "default"  // dự phòng (proxy tùy chỉnh/mô hình chưa biết)
)

// ResolveContextWindow giải quyết cửa sổ hợp lệ dùng để nén ngữ cảnh, theo thứ tự ưu tiên:
//  1. ContextWindow > 0 trong file cấu hình → dùng trực tiếp (ưu tiên cao nhất, có thể vượt cửa sổ thực của mô hình)
//  2. models.DefaultRegistry truy vấn theo tên mô hình (baseline OpenRouter + làm mới 24h)
//  3. Dự phòng DefaultContextWindow (proxy tùy chỉnh / mô hình chưa biết)
//
// Lưu ý: giá trị trả về chỉ dùng để tính ngưỡng nén, không thu hẹp độ dài yêu cầu thực tế tới LLM API.
func (c Config) ResolveContextWindow(modelName string) (int, ContextWindowSource) {
	if c.ContextWindow > 0 {
		return c.ContextWindow, CtxWindowConfig
	}
	if rw := models.DefaultRegistry().ResolveContextWindow(modelName); rw > 0 {
		return rw, CtxWindowRegistry
	}
	return DefaultContextWindow, CtxWindowDefault
}

// ResolveReasoningEffort trả về chuỗi mức độ suy luận có hiệu lực của một vai (off/low/medium/high/xhigh/max hoặc rỗng).
// Thứ tự ưu tiên: Roles[role].ReasoningEffort cấp vai → ReasoningEffort mặc định cấp cao nhất → "" (không ghi đè, dùng mặc định mô hình/provider).
// Khi role rỗng hoặc "default" thì lấy trực tiếp mặc định cấp cao nhất. Tính hợp lệ của giá trị do agents.ParseThinkingLevel kiểm soát.
func (c Config) ResolveReasoningEffort(role string) string {
	if role != "" && role != "default" {
		if rc, ok := c.Roles[role]; ok && rc.ReasoningEffort != "" {
			return rc.ReasoningEffort
		}
	}
	return c.ReasoningEffort
}

// LogContextWindowChoice in quyết định cửa sổ của một vai. Khi source=default thì phát Warn
// thông báo mô hình chưa trúng registry (OpenRouter cũng chưa thu thập), việc nén ngữ cảnh tiếp theo
// sẽ kích hoạt theo cửa sổ dự phòng — nếu cửa sổ thực của mô hình lớn hơn, có thể dùng context_window tường minh trong file cấu hình để tránh bị nén sớm, mất lịch sử.
func LogContextWindowChoice(role, model string, window int, source ContextWindowSource) {
	attrs := []any{"module", "context", "role", role, "model", model, "window", window, "source", source}
	switch source {
	case CtxWindowDefault:
		slog.Warn("Mô hình chưa được nhận dạng, dùng cửa sổ dự phòng (proxy tùy chỉnh hoặc OpenRouter chưa thu thập, có thể dùng context_window để chỉ định tường minh)", attrs...)
	case CtxWindowConfig:
		slog.Info("Cửa sổ ngữ cảnh (từ context_window trong file cấu hình)", attrs...)
	default:
		slog.Info("Cửa sổ ngữ cảnh", attrs...)
	}
}

// CandidateModels trả về danh sách mô hình có thể chuyển đổi của một provider.
// Ưu tiên dùng models mà provider khai báo tường minh; đồng thời bổ sung các mô hình của provider đó đã xuất hiện trong cấu hình hiện tại.
func (c Config) CandidateModels(provider string) []string {
	if provider == "" {
		return nil
	}

	seen := make(map[string]bool)
	models := make([]string, 0, 4)
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			return
		}
		seen[model] = true
		models = append(models, model)
	}

	if pc, ok := c.Providers[provider]; ok {
		for _, model := range pc.Models {
			add(model)
		}
	}
	if c.Provider == provider {
		add(c.ModelName)
	}
	for _, rc := range c.Roles {
		if rc.Provider == provider {
			add(rc.Model)
		}
		for _, fallback := range rc.Fallbacks {
			if fallback.Provider == provider {
				add(fallback.Model)
			}
		}
	}
	return models
}

func (c Config) validateModelRef(owner string, ref ModelRef) error {
	if ref.Provider == "" || ref.Model == "" {
		return fmt.Errorf("%s must have both provider and model: %w", owner, errs.ErrConfig)
	}

	pc, ok := c.Providers[ref.Provider]
	if !ok {
		return fmt.Errorf("%s references provider %q which is not configured: %w", owner, ref.Provider, errs.ErrConfig)
	}
	if pc.RequiresAPIKey(ref.Provider) && pc.APIKey == "" {
		return fmt.Errorf("%s references provider %q which has no api_key: %w", owner, ref.Provider, errs.ErrConfig)
	}
	if err := c.validateProviderAPI(owner, ref.Provider, pc); err != nil {
		return err
	}
	return nil
}

func (c Config) validateProviderAPI(owner, providerName string, pc ProviderConfig) error {
	if pc.API == "" {
		return nil
	}
	providerType, err := pc.ProviderType(providerName)
	if err != nil {
		return fmt.Errorf("%s cấu hình api của provider %q không thể giải quyết loại giao thức: %w", owner, providerName, err)
	}
	if strings.ToLower(strings.TrimSpace(providerType)) != "openai" {
		return fmt.Errorf("%s api của provider %q chỉ hỗ trợ provider giao thức OpenAI: %w", owner, providerName, errs.ErrConfig)
	}
	return nil
}

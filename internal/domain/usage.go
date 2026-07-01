package domain

import "time"

// UsageSchemaVersion là số phiên bản tương thích của meta/usage.json.
// Nếu ngữ nghĩa các trường AgentUsageTotals thay đổi trong tương lai, hãy tăng giá trị này;
// UsageStore.Load khi thấy phiên bản khác nên bỏ qua và kích hoạt replay để xây dựng lại.
const UsageSchemaVersion = 2

// UsageState là snapshot có thể lưu trữ của tổng lượng token / chi phí tích lũy.
// Được duy trì trong bộ nhớ bởi UsageTracker, định kỳ debounce ghi xuống meta/usage.json.
//
// Lưu ý: các mẫu cửa sổ trượt (sliding window samples) bên trong UsageTracker ("tỷ lệ trúng N lần gần đây")
// **không được lưu trữ** — chúng chỉ phục vụ chẩn đoán ngắn hạn trên UI,
// khởi động lại tiến trình từ đầu và tích lũy lại vài vòng là có thể khôi phục ngữ nghĩa.
// MissingAssistantUsage được giữ lại để lưu trữ, tích lũy xuyên khởi động lại có giá trị chẩn đoán hơn.
type UsageState struct {
	Schema       int                         `json:"schema"`
	UpdatedAt    time.Time                   `json:"updated_at"`
	Overall      AgentUsageTotals            `json:"overall"`
	PerAgent     map[string]AgentUsageTotals `json:"per_agent"`
	PerModel     map[string]AgentUsageTotals `json:"per_model,omitempty"`
	MissingUsage int                         `json:"missing_assistant_usage"`
}

// AgentUsageTotals là dạng có thể lưu trữ của tổng số đếm tích lũy cho một nhân vật (hoặc overall).
type AgentUsageTotals struct {
	Input        int     `json:"input"`
	Output       int     `json:"output"`
	CacheRead    int     `json:"cache_read"`
	CacheWrite   int     `json:"cache_write"`
	Cost         float64 `json:"cost_usd"`
	Saved        float64 `json:"saved_usd"`
	CacheCapable bool    `json:"cache_capable"`
}

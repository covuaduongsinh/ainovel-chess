package userrules

import (
	"context"
	"strings"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/rules"
	"github.com/voocel/ainovel-cli/internal/store"
)

// Service điều phối việc tạo và cập nhật ảnh chụp quy tắc người dùng: chuẩn hóa từng nguồn → hợp nhất có tính xác định → lưu xuống đĩa.
//
// Hai bên gọi dùng chung một bộ logic:
//   - Mở sách/làm mới (phía khởi động, có tính xác định): Build / GetOrBuild, Host gọi trực tiếp, không qua Coordinator.
//   - Cập nhật lúc chạy (công cụ Coordinator): AddRuntimeRule, vỏ công cụ save_user_rules tái dùng.
type Service struct {
	store     *store.Store
	norm      *Normalizer
	rulesOpts rules.LoadOptions
}

// NewService tạo dịch vụ. model dùng cho chuẩn hóa (nên là mô hình có năng lực mạnh hơn); khi model là nil
// tất cả nguồn giáng cấp thành raw preferences (vẫn có thể tạo ảnh chụp, kiểm tra cơ học do system_defaults đảm bảo).
func NewService(st *store.Store, model agentcore.ChatModel, opts rules.LoadOptions) *Service {
	return &Service{store: st, norm: NewNormalizer(model), rulesOpts: opts}
}

// Build tạo ảnh chụp bằng cách chuẩn hóa từ nguồn tĩnh (system_defaults + tệp rules + startup prompt) và lưu xuống đĩa.
// Gọi khi mở sách/làm mới. startupPrompt có thể rỗng.
func (s *Service) Build(ctx context.Context, startupPrompt string) (*rules.Snapshot, error) {
	cands := []rules.Candidate{rules.SystemDefaults()}
	for _, rs := range rules.RawFileSources(s.rulesOpts) {
		cands = append(cands, s.norm.Normalize(ctx, rs.Label, rs.Text))
	}
	if strings.TrimSpace(startupPrompt) != "" {
		cands = append(cands, s.norm.Normalize(ctx, "startup_prompt", startupPrompt))
	}
	snap := rules.BuildSnapshot(cands)
	if err := s.store.UserRules.Save(&snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

// GetOrBuild trả về ảnh chụp hiện tại; khi sách cũ chưa có ảnh chụp thì tạo lười biếng (không có văn bản startup prompt gốc, nên chỉ chứa
// system_defaults + tệp rules). Đường đọc lúc chạy đều đi qua đây.
func (s *Service) GetOrBuild(ctx context.Context) (*rules.Snapshot, error) {
	cur, err := s.store.UserRules.Load()
	if err != nil {
		return nil, err
	}
	if cur != nil {
		return cur, nil
	}
	return s.Build(ctx, "")
}

// AddRuntimeRule chuẩn hóa một quy tắc lâu dài lúc chạy, chồng lên ảnh chụp hiện tại với ưu tiên cao nhất rồi lưu xuống đĩa.
// Không bao giờ báo lỗi do chuẩn hóa thất bại — khi thất bại, quy tắc đó giáng cấp thành raw preferences.
// Trả về ảnh chụp sau khi chồng và ứng viên chuẩn hóa lần này (cái sau dùng để save_user_rules phản hồi lại "đã hiểu thành gì" cho người dùng xác nhận).
func (s *Service) AddRuntimeRule(ctx context.Context, text string) (*rules.Snapshot, rules.Candidate, error) {
	cur, err := s.GetOrBuild(ctx)
	if err != nil {
		return nil, rules.Candidate{}, err
	}
	cand := s.norm.Normalize(ctx, "runtime_update", text)
	merged := rules.OverlaySnapshot(*cur, cand)
	if err := s.store.UserRules.Save(&merged); err != nil {
		return nil, cand, err
	}
	return &merged, cand, nil
}

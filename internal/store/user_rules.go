package store

import (
	"os"

	"github.com/voocel/ainovel-cli/internal/rules"
)

// UserRulesStore quản lý ảnh chụp quy tắc người dùng đã chuẩn hóa của cuốn sách này (meta/user_rules.json).
//
// Nguồn sự thật duy nhất tại thời gian chạy: novel_context nhúng và commit_chapter kiểm tra đều chỉ đọc bản này,
// không còn đọc lại tệp rules nhiều lần (tránh trôi dạt và phân kỳ giữa hai người đọc). Ảnh chụp được tạo khi mở sách/nhập/làm mới.
type UserRulesStore struct{ io *IO }

func NewUserRulesStore(io *IO) *UserRulesStore { return &UserRulesStore{io: io} }

// Load đọc meta/user_rules.json. Trả về nil nếu không tồn tại (bên gọi dựa vào đó để tạo theo kiểu lazy).
func (s *UserRulesStore) Load() (*rules.Snapshot, error) {
	s.io.mu.RLock()
	defer s.io.mu.RUnlock()
	var snap rules.Snapshot
	if err := s.io.ReadJSONUnlocked("meta/user_rules.json", &snap); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return &snap, nil
}

// Save lưu ảnh chụp.
func (s *UserRulesStore) Save(snap *rules.Snapshot) error {
	s.io.mu.Lock()
	defer s.io.mu.Unlock()
	return s.io.WriteJSONUnlocked("meta/user_rules.json", snap)
}

package store

import (
	"os"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// UsageStore lưu trữ bền vững lượng dùng tích lũy token/cost vào meta/usage.json.
// Ghi qua atomic write của IO (tmp + rename), đường dẫn Save mỗi lần ghi đè hoàn toàn toàn bộ state.
type UsageStore struct{ io *IO }

func NewUsageStore(io *IO) *UsageStore { return &UsageStore{io: io} }

// Load đọc usage.json. Khi tệp không tồn tại hoặc phiên bản schema không khớp thì trả về (nil, nil),
// bên gọi quyết định có đi theo session replay để bổ sung một lần không.
func (s *UsageStore) Load() (*domain.UsageState, error) {
	var state domain.UsageState
	if err := s.io.ReadJSON("meta/usage.json", &state); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if state.Schema != domain.UsageSchemaVersion {
		return nil, nil
	}
	return &state, nil
}

// Save ghi đè hoàn toàn state xuống đĩa. Bên gọi chịu trách nhiệm debounce / throttle.
func (s *UsageStore) Save(state domain.UsageState) error {
	state.Schema = domain.UsageSchemaVersion
	return s.io.WriteJSON("meta/usage.json", state)
}

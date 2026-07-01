package store

import (
	"os"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// DossierStore quản lý hồ sơ nhân vật thật (mỏ neo sự thật) của cuốn sách này (meta/dossier.json).
//
// Nguồn sự thật duy nhất tại thời gian chạy cho chế độ "viết bám sát nhân vật có thật":
// novel_context đọc bản này để bơm working_memory.fact_dossier cho architect/writer.
// Ảnh chụp được tạo khi bắt đầu sáng tác (Host.PrepareDossier).
type DossierStore struct{ io *IO }

func NewDossierStore(io *IO) *DossierStore { return &DossierStore{io: io} }

// Load đọc meta/dossier.json. Trả về nil nếu không tồn tại (chế độ grounding chưa bật).
func (s *DossierStore) Load() (*domain.Dossier, error) {
	s.io.mu.RLock()
	defer s.io.mu.RUnlock()
	var d domain.Dossier
	if err := s.io.ReadJSONUnlocked("meta/dossier.json", &d); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

// Save lưu hồ sơ.
func (s *DossierStore) Save(d *domain.Dossier) error {
	s.io.mu.Lock()
	defer s.io.mu.Unlock()
	return s.io.WriteJSONUnlocked("meta/dossier.json", d)
}

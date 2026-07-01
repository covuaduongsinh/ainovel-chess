package userrules

import (
	"testing"

	"github.com/voocel/ainovel-cli/internal/rules"
	"github.com/voocel/ainovel-cli/internal/store"
)

// Mô hình nil + thư mục quy tắc rỗng: chuẩn hóa toàn bộ giáng cấp, nhưng ảnh chụp vẫn có thể tạo ra (system_defaults đảm bảo) và lưu xuống đĩa.
// Hai thư mục của LoadOptions{} là chuỗi rỗng, RawFileSources trả về nil, kiểm tra không chạm vào đĩa thật.
func newDegradedService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	st := store.NewStore(t.TempDir())
	return NewService(st, nil, rules.LoadOptions{}), st
}

func TestService_Build_DegradesButPersists(t *testing.T) {
	svc, st := newDegradedService(t)

	snap, err := svc.Build(t.Context(), "mỗi chương 1200 chữ, nhân vật chính bình tĩnh kiềm chế")
	if err != nil {
		t.Fatalf("Build không nên báo lỗi (giáng cấp chứ không chặn): %v", err)
	}
	if snap.Status != rules.StatusDegraded {
		t.Fatalf("không có mô hình nên giáng cấp, status=%q", snap.Status)
	}
	// system_defaults luôn đảm bảo đường đáy cơ học.
	if snap.Structured.ChapterWords == nil || snap.Structured.ChapterWords.Min != 3000 {
		t.Fatalf("nên giữ đường đáy số chữ system_defaults, got %+v", snap.Structured.ChapterWords)
	}
	// startup prompt giáng cấp thành raw preferences, văn bản gốc không mất.
	if snap.Preferences == "" {
		t.Fatal("giáng cấp nên ghi văn bản gốc startup prompt vào preferences")
	}

	// Đã lưu xuống đĩa: GetOrBuild đọc lại cùng bản chứ không tạo lại.
	reloaded, err := st.UserRules.Load()
	if err != nil || reloaded == nil {
		t.Fatalf("ảnh chụp nên đã được lưu: err=%v snap=%v", err, reloaded)
	}
	if reloaded.Preferences != snap.Preferences {
		t.Fatal("nội dung lưu không nhất quán với kết quả trả về")
	}
}

func TestService_GetOrBuild_LazyForOldBook(t *testing.T) {
	svc, st := newDegradedService(t)

	if cur, _ := st.UserRules.Load(); cur != nil {
		t.Fatal("ban đầu không nên có ảnh chụp")
	}
	snap, err := svc.GetOrBuild(t.Context())
	if err != nil {
		t.Fatalf("GetOrBuild không nên báo lỗi: %v", err)
	}
	if snap.Structured.ChapterWords == nil {
		t.Fatal("tạo lười biếng nên chứa system_defaults")
	}
	if cur, _ := st.UserRules.Load(); cur == nil {
		t.Fatal("GetOrBuild nên đồng thời lưu xuống đĩa")
	}
}

func TestService_AddRuntimeRule_PersistsAndReturnsCandidate(t *testing.T) {
	svc, st := newDegradedService(t)

	const text = "ít dùng ẩn dụ sau này"
	merged, cand, err := svc.AddRuntimeRule(t.Context(), text)
	if err != nil {
		t.Fatalf("AddRuntimeRule không nên báo lỗi: %v", err)
	}
	// Ứng viên dùng để phản hồi lại: không có mô hình thì giáng cấp, văn bản gốc vào preferences.
	if !cand.Degraded {
		t.Fatal("không có mô hình, ứng viên lần này nên giáng cấp")
	}
	if cand.Preferences != text {
		t.Fatalf("ứng viên nên giữ văn bản gốc, got %q", cand.Preferences)
	}
	// Ảnh chụp sau khi chồng chứa mục đó và đã được lưu.
	if merged.Preferences == "" {
		t.Fatal("preferences sau khi chồng không nên rỗng")
	}
	reloaded, err := st.UserRules.Load()
	if err != nil || reloaded == nil {
		t.Fatalf("sau khi chồng nên lưu xuống đĩa: err=%v", err)
	}
	if reloaded.Status != rules.StatusDegraded {
		t.Fatalf("chứa nguồn giáng cấp, status nên là degraded, got %q", reloaded.Status)
	}
}

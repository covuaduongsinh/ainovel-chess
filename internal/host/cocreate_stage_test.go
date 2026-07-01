package host

import (
	"context"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host/imp"
	"github.com/voocel/ainovel-cli/internal/store"
)

// newFlagTestHost tao mot Host toi thieu, du de dieu khien may trang thai co cocreating va guard dong thoi.
// emitEvent dung recover + select khong chan, dem may kenh events la du, khong can coordinator/observer.
// Nhanh trang thai chay cua PauseForCoCreate se goi coordinator.Abort (tai su dung duong tam dung Esc da kiem tra),
// khong o day trong unit test; o day chi bao phu trang thai khong chay va logic co/guard khong phu thuoc coordinator.
func newFlagTestHost(lc lifecycle, cocreating bool) *Host {
	return &Host{
		lifecycle:  lc,
		cocreating: cocreating,
		events:     make(chan Event, 16),
	}
}

func TestPauseForCoCreate_NonRunningSetsFlag(t *testing.T) {
	h := newFlagTestHost(lifecycleIdle, false)
	if !h.PauseForCoCreate() {
		t.Fatal("Trang thai idle nen cho phep vao dong sang tao giai doan")
	}
	if !h.cocreating {
		t.Error("Sau khi vao, cocreating nen la true")
	}
	if h.lifecycle != lifecycleIdle {
		t.Errorf("Vao khi khong chay khong nen thay doi lifecycle, got %s", h.lifecycle)
	}
}

func TestPauseForCoCreate_RejectsCompleted(t *testing.T) {
	h := newFlagTestHost(lifecycleCompleted, false)
	if h.PauseForCoCreate() {
		t.Error("Sau khi ca cuon sach hoan thanh khong nen cho phep vao dong sang tao giai doan")
	}
	if h.cocreating {
		t.Error("Sau khi tu choi khong nen dat cocreating")
	}
}

func TestPauseForCoCreate_RejectsReentrant(t *testing.T) {
	h := newFlagTestHost(lifecyclePaused, true)
	if h.PauseForCoCreate() {
		t.Error("Da trong dong sang tao nen tu choi tai nhap")
	}
}

func TestCancelCoCreate_ClearsFlag(t *testing.T) {
	h := newFlagTestHost(lifecyclePaused, true)
	h.CancelCoCreate()
	if h.cocreating {
		t.Error("Sau khi huy, cocreating nen duoc xoa")
	}
	if h.lifecycle != lifecyclePaused {
		t.Errorf("Huy khong nen thay doi lifecycle, got %s", h.lifecycle)
	}
}

func TestCancelCoCreate_NoopWhenNotCocreating(t *testing.T) {
	h := newFlagTestHost(lifecycleRunning, false)
	h.CancelCoCreate() // Khong nen panic, khong nen thay doi trang thai
	if h.cocreating || h.lifecycle != lifecycleRunning {
		t.Error("CancelCoCreate khi khong trong dong sang tao nen la no-op")
	}
}

func TestResumeFromCoCreate_RejectsEmptyDraft(t *testing.T) {
	h := newFlagTestHost(lifecyclePaused, true)
	if err := h.ResumeFromCoCreate("   "); err == nil {
		t.Fatal("Draft rong nen bao loi")
	}
	if !h.cocreating {
		t.Error("Draft rong tra ve truoc khi xoa co, cocreating nen giu true")
	}
}

func TestResumeFromCoCreate_RejectsWhenNotCocreating(t *testing.T) {
	h := newFlagTestHost(lifecyclePaused, false)
	err := h.ResumeFromCoCreate("## Hướng đi tiếp theo\n- Bước vào tập 2")
	if err == nil || !strings.Contains(err.Error(), "not in co-create") {
		t.Fatalf("Khi khong trong dong sang tao nen bao not in co-create, got %v", err)
	}
}

func TestGuardExclusive(t *testing.T) {
	cases := []struct {
		name       string
		lc         lifecycle
		cocreating bool
		wantErr    string // Rong = ky vong cho phep
	}{
		{"running", lifecycleRunning, false, "dang chay"},
		{"cocreating", lifecyclePaused, true, "dong sang tao"},
		{"idle free", lifecycleIdle, false, ""},
		{"paused free", lifecyclePaused, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newFlagTestHost(c.lc, c.cocreating)
			err := h.guardExclusive("nhap")
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("Nen cho phep, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("Nen chua %q, got %v", c.wantErr, err)
			}
			if !strings.Contains(err.Error(), "nhap") {
				t.Errorf("Thong bao loi nen chua action %q, got %v", "nhap", err)
			}
		})
	}
}

// TestStageCoCreate_OccupancyBlocksConcurrentEntries kiem tra tat ca diem vao doc quyen trong cua so dong sang tao deu bi chan:
// import/start/resume/continue trong khi cocreating deu nen bi tu choi, bu vao khe chi kiem tra ==running trong khi paused.
func TestStageCoCreate_OccupancyBlocksConcurrentEntries(t *testing.T) {
	h := newFlagTestHost(lifecycleIdle, false)
	if !h.PauseForCoCreate() {
		t.Fatal("Vao dong sang tao giai doan that bai")
	}

	if _, err := h.ImportFrom(context.Background(), imp.Options{}); err == nil {
		t.Error("ImportFrom trong cua so dong sang tao nen bi tu choi")
	}
	if err := h.StartPrepared("viet mot cau chuyen moi"); err == nil {
		t.Error("StartPrepared trong cua so dong sang tao nen bi tu choi")
	}
	if _, err := h.Resume(); err == nil {
		t.Error("Resume trong cua so dong sang tao nen bi tu choi")
	}
	if err := h.Continue("tiep tuc viet"); err == nil {
		t.Error("Continue trong cua so dong sang tao nen bi tu choi")
	}

	// Giai phong chiem dung sau khi thoat dong sang tao (o day dung Cancel; duong nap Resume can coordinator, kiem tra o lop tich hop)
	h.CancelCoCreate()
	if h.cocreating {
		t.Fatal("Sau khi thoat co chiem dung nen duoc giai phong")
	}
}

func TestBuildStoryStateSummary_NilStore(t *testing.T) {
	if got := buildStoryStateSummary(nil); got != "" {
		t.Errorf("nil store nen tra ve chuoi rong, got %q", got)
	}
}

func TestBuildStoryStateSummary_Populated(t *testing.T) {
	dir := t.TempDir()
	st := store.NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	if err := st.Progress.Init("Bài thơ bóng tối", 100); err != nil {
		t.Fatal(err)
	}
	p, _ := st.Progress.Load()
	p.CompletedChapters = []int{1, 2, 3}
	p.TotalWordCount = 12000
	if err := st.Progress.Save(p); err != nil {
		t.Fatal(err)
	}
	if err := st.Outline.SaveCompass(domain.StoryCompass{
		EndingDirection: "Nhân vật chính leo đến đỉnh tuyệt đối",
		OpenThreads:     []string{"Mối thù huyết môn phái chưa trả"},
		EstimatedScale:  "Dự kiến 4-6 tập",
	}); err != nil {
		t.Fatal(err)
	}

	got := buildStoryStateSummary(st)
	for _, want := range []string{"Bài thơ bóng tối", "đã hoàn thành 3 chương", "chương tiếp theo là chương 4", "Nhân vật chính leo đến đỉnh tuyệt đối", "Mối thù huyết môn phái chưa trả", "Dự kiến 4-6 tập"} {
		if !strings.Contains(got, want) {
			t.Errorf("Tom tat nen chua %q, thuc te:\n%s", want, got)
		}
	}
}

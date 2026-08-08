package comic

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writePrompts ghi một bảng prompt giả cho chương ch, và tạo sẵn artHave tệp ảnh đầu tiên.
func writePrompts(t *testing.T, outDir string, ch, panels, artHave int) {
	t.Helper()
	rows := make([]PanelPrompt, 0, panels)
	for i := 1; i <= panels; i++ {
		rows = append(rows, PanelPrompt{
			Chapter: ch, Page: 1, Panel: i,
			ArtFile: artFile(ch, 1, i),
		})
	}
	data, _ := json.MarshalIndent(rows, "", "  ")
	p := filepath.Join(outDir, "prompts")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "chuong-"+pad3(ch)+".json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= artHave; i++ {
		ap := filepath.Join(outDir, filepath.FromSlash(artFile(ch, 1, i)))
		if err := os.MkdirAll(filepath.Dir(ap), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(ap, []byte("PNG-giả"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func pad3(n int) string {
	s := "00" + itoa3(n)
	return s[len(s)-3:]
}

func itoa3(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// TestEstimateCountsMissingNotTotal là điểm mấu chốt: chi phí thật là số khung CÒN THIẾU.
// Báo tổng số khung sẽ doạ người dùng bằng tiền họ không hề phải trả, vì cơ chế
// bỏ-qua-nếu-đã-có làm việc chạy lại gần như miễn phí.
func TestEstimateCountsMissingNotTotal(t *testing.T) {
	out := tolerantTempDir(t)
	writePrompts(t, out, 1, 10, 4) // 10 khung, đã có 4 ảnh

	est, err := EstimateCost(out, 0, 0)
	if err != nil {
		t.Fatalf("EstimateCost: %v", err)
	}
	if !est.HasData {
		t.Fatal("phải báo đã có dữ liệu")
	}
	if est.TotalPanels != 10 {
		t.Errorf("tổng khung = %d, muốn 10", est.TotalPanels)
	}
	if est.MissingPanels != 6 {
		t.Errorf("khung còn thiếu = %d, muốn 6 (đây mới là phần phải trả tiền)", est.MissingPanels)
	}
	if est.Chapters != 1 {
		t.Errorf("số chương = %d, muốn 1", est.Chapters)
	}
}

func TestEstimateRespectsRange(t *testing.T) {
	out := tolerantTempDir(t)
	writePrompts(t, out, 1, 10, 0)
	writePrompts(t, out, 2, 5, 0)
	writePrompts(t, out, 3, 7, 0)

	est, err := EstimateCost(out, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if est.Chapters != 2 || est.TotalPanels != 12 {
		t.Errorf("phạm vi 2–3: %d chương / %d khung, muốn 2 / 12", est.Chapters, est.TotalPanels)
	}
}

// TestEstimateNoDataIsNotAnError — chưa chạy bước kịch bản thì trả rỗng, KHÔNG phải lỗi;
// giao diện dựa vào đó để rơi về giả định thô và nói rõ đó là giả định.
func TestEstimateNoDataIsNotAnError(t *testing.T) {
	est, err := EstimateCost(tolerantTempDir(t), 0, 0)
	if err != nil {
		t.Fatalf("thiếu dữ liệu không được coi là lỗi: %v", err)
	}
	if est.HasData || est.TotalPanels != 0 {
		t.Errorf("phải trả rỗng: %+v", est)
	}
}

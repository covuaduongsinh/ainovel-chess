package tts

import (
	"strings"
	"testing"
)

// ---------- CleanChapter: lời dẫn đầu chương ----------

func TestCleanChapter_TieuDeATXCoChuChuong(t *testing.T) {
	got := CleanChapter(1, "", "# Chương 1: Berlinchen — Cậu Bé Tò Mò\n\nNội dung.", "")
	want := "Chương 1. Berlinchen — Cậu Bé Tò Mò.\n\nNội dung."
	if got != want {
		t.Errorf("lời dẫn sai.\nnhận: %q\nmuốn: %q", got, want)
	}
}

func TestCleanChapter_TieuDeH2CoChuChuong(t *testing.T) {
	got := CleanChapter(3, "", "## Chương 3: Những con số biết nhảy múa\n\nNội dung.", "")
	if !strings.HasPrefix(got, "Chương 3. Những con số biết nhảy múa.") {
		t.Errorf("lời dẫn sai: %q", got)
	}
}

func TestCleanChapter_TieuDeThuongKhongCoThang(t *testing.T) {
	// Dạng có thật trong output/Alexander-Alekhine-32c/chapters/01.md
	got := CleanChapter(1, "", "Chương 1: Bàn cờ trong phòng khách\n\nỞ Moscow, mùa đông...", "")
	if !strings.HasPrefix(got, "Chương 1. Bàn cờ trong phòng khách.") {
		t.Errorf("lời dẫn sai: %q", got)
	}
	if strings.Contains(got, "Chương 1: Bàn cờ") {
		t.Errorf("dòng tiêu đề trần phải bị nuốt vào lời dẫn, không được đọc lại: %q", got)
	}
}

func TestCleanChapter_TieuDeATXKhongCoChuChuongThiGhepSoChuong(t *testing.T) {
	// Dạng có thật trong .../chapters/02.md
	got := CleanChapter(2, "", "# Đêm quân cờ thức giấc\n\nSasha đứng giữa phòng khách.", "")
	if !strings.HasPrefix(got, "Chương 2. Đêm quân cờ thức giấc.") {
		t.Errorf("lời dẫn sai: %q", got)
	}
}

func TestCleanChapter_KhongCoTieuDeThiLayTuDanY(t *testing.T) {
	// Dạng PHỔ BIẾN NHẤT trong repo: vào thẳng văn xuôi.
	got := CleanChapter(3, "Những con số biết nhảy múa", "Suốt cả ngày hôm ấy...", "")
	if !strings.HasPrefix(got, "Chương 3. Những con số biết nhảy múa.") {
		t.Errorf("phải lấy tiêu đề từ dàn ý: %q", got)
	}
	if !strings.Contains(got, "Suốt cả ngày hôm ấy") {
		t.Errorf("mất thân chương: %q", got)
	}
}

func TestCleanChapter_KhongTieuDeKhongDanYVanCoLoiDan(t *testing.T) {
	got := CleanChapter(7, "", "Văn xuôi.", "")
	if !strings.HasPrefix(got, "Chương 7.") {
		t.Errorf("phải có lời dẫn tối thiểu: %q", got)
	}
}

func TestCleanChapter_TieuDeGiuaChuongDuocDoc(t *testing.T) {
	got := CleanChapter(1, "T", "Mở đầu.\n\n## Phần hai\n\nTiếp theo.", "")
	if !strings.Contains(got, "Phần hai.") {
		t.Errorf("tiêu đề giữa chương phải được đọc thành câu: %q", got)
	}
}

// ---------- CleanChapter: gỡ ký hiệu ----------

func TestCleanChapter_BoDamNghiengMaNguonVaLienKet(t *testing.T) {
	md := "Đây là **đậm**, *nghiêng*, `mã`, ~~gạch~~ và [liên kết](https://vd.vn) cùng ![ảnh](a.png)."
	got := CleanChapter(0, "", md, "")
	for _, banned := range []string{"**", "`", "~~", "](", "!["} {
		if strings.Contains(got, banned) {
			t.Errorf("còn sót ký hiệu %q: %q", banned, got)
		}
	}
	for _, want := range []string{"đậm", "nghiêng", "mã", "gạch", "liên kết"} {
		if !strings.Contains(got, want) {
			t.Errorf("mất nội dung %q: %q", want, got)
		}
	}
	if strings.Contains(got, "https://vd.vn") || strings.Contains(got, "a.png") {
		t.Errorf("URL/ảnh phải bị bỏ: %q", got)
	}
}

func TestCleanChapter_GiuGachDuoiTrongDinhDanh(t *testing.T) {
	got := CleanChapter(0, "", "Mã giọng là hn_female_ngochuyen chứ không phải gì khác.", "")
	if !strings.Contains(got, "hn_female_ngochuyen") {
		t.Errorf("gạch dưới trong định danh bị xé: %q", got)
	}
}

func TestCleanChapter_BoNghiengBangGachDuoi(t *testing.T) {
	got := CleanChapter(0, "", "Cậu ấy _thật sự_ đã đi.", "")
	if strings.Contains(got, "_") {
		t.Errorf("nghiêng bằng gạch dưới chưa được gỡ: %q", got)
	}
	if !strings.Contains(got, "thật sự") {
		t.Errorf("mất nội dung: %q", got)
	}
}

func TestCleanChapter_BoTrichDanVaGachDauDong(t *testing.T) {
	got := CleanChapter(0, "", "> Một câu trích.\n\n- Mục một\n- Mục hai", "")
	if strings.Contains(got, ">") || strings.Contains(got, "- Mục") {
		t.Errorf("chưa gỡ tiền tố trích dẫn/gạch đầu dòng: %q", got)
	}
	if !strings.Contains(got, "Một câu trích") || !strings.Contains(got, "Mục một") {
		t.Errorf("mất nội dung: %q", got)
	}
}

func TestCleanChapter_BoDuongKeNganGiuNgatDoan(t *testing.T) {
	got := CleanChapter(0, "", "Cảnh một.\n\n---\n\nCảnh hai.", "")
	if strings.Contains(got, "---") {
		t.Errorf("còn đường kẻ ngang: %q", got)
	}
	if !strings.Contains(got, "Cảnh một") || !strings.Contains(got, "Cảnh hai") {
		t.Errorf("mất nội dung: %q", got)
	}
}

func TestCleanChapter_SceneBreakTextDuocDoc(t *testing.T) {
	got := CleanChapter(0, "", "Cảnh một.\n\n---\n\nCảnh hai.", "Chuyển cảnh")
	if !strings.Contains(got, "Chuyển cảnh.") {
		t.Errorf("câu chuyển cảnh phải được chèn và có dấu kết: %q", got)
	}
}

func TestCleanChapter_BoEmojiNhungGiuGachNgangHoiThoai(t *testing.T) {
	got := CleanChapter(0, "", "🔑 Bí mật ở đây.\n\n— Sasha, lại đây với mẹ nào.", "")
	if strings.Contains(got, "🔑") {
		t.Errorf("emoji phải bị bỏ: %q", got)
	}
	if !strings.Contains(got, "— Sasha") {
		t.Errorf("gạch ngang mở thoại PHẢI được giữ (Vbee đọc thành nhịp nghỉ): %q", got)
	}
}

func TestCleanChapter_GomKhoangTrangVaDongTrong(t *testing.T) {
	got := CleanChapter(0, "", "Một    câu.\n\n\n\n\nCâu   khác.", "")
	if strings.Contains(got, "  ") {
		t.Errorf("chưa gom khoảng trắng: %q", got)
	}
	if strings.Contains(got, "\n\n\n") {
		t.Errorf("chưa gộp dòng trống: %q", got)
	}
}

func TestCleanChapter_BoKhoiMaNguon(t *testing.T) {
	got := CleanChapter(0, "", "Trước.\n\n```go\nfmt.Println(\"x\")\n```\n\nSau.", "")
	if strings.Contains(got, "fmt.Println") || strings.Contains(got, "```") {
		t.Errorf("khối mã nguồn phải bị bỏ: %q", got)
	}
}

func TestCleanChapter_NoiDungRongTraVeRong(t *testing.T) {
	if got := CleanChapter(0, "", "   \n\n  \n", ""); got != "" {
		t.Errorf("nội dung rỗng phải cho chuỗi rỗng, nhận %q", got)
	}
}

// ---------- SplitForTTS ----------

func TestSplitForTTS_DuoiNguongThiMotPhan(t *testing.T) {
	parts := SplitForTTS("Một câu ngắn.", 1000)
	if len(parts) != 1 || parts[0] != "Một câu ngắn." {
		t.Errorf("muốn 1 phần nguyên vẹn, nhận %d: %q", len(parts), parts)
	}
}

func TestSplitForTTS_RongTraVeNil(t *testing.T) {
	if parts := SplitForTTS("   ", 100); parts != nil {
		t.Errorf("văn bản rỗng phải trả nil, nhận %q", parts)
	}
}

func TestSplitForTTS_ChiaTheoRanhGioiDoanVan(t *testing.T) {
	a := strings.Repeat("a", 40)
	b := strings.Repeat("b", 40)
	c := strings.Repeat("c", 40)
	parts := SplitForTTS(a+"\n\n"+b+"\n\n"+c, 90)
	if len(parts) != 2 {
		t.Fatalf("muốn 2 phần, nhận %d: %v", len(parts), parts)
	}
	if parts[0] != a+"\n\n"+b || parts[1] != c {
		t.Errorf("chia sai ranh giới đoạn: %q", parts)
	}
}

func TestSplitForTTS_ChiaTheoCauKhiDoanQuaDai(t *testing.T) {
	para := "Câu một dài dài dài. Câu hai dài dài dài. Câu ba dài dài dài."
	parts := SplitForTTS(para, 30)
	if len(parts) < 2 {
		t.Fatalf("đoạn quá dài phải bị tách theo câu, nhận %d phần", len(parts))
	}
	for _, p := range parts {
		if n := len([]rune(p)); n > 30 {
			t.Errorf("phần dài %d rune, vượt ngưỡng 30: %q", n, p)
		}
	}
	joined := strings.Join(parts, " ")
	for _, want := range []string{"Câu một", "Câu hai", "Câu ba"} {
		if !strings.Contains(joined, want) {
			t.Errorf("mất nội dung %q sau khi chia: %v", want, parts)
		}
	}
}

func TestSplitForTTS_KhongCatGiuaDauNhay(t *testing.T) {
	// Dấu chấm nằm TRONG cặp nháy cong không được coi là ranh giới câu.
	para := `“Đi nào. Tớ nghe thấy rồi. Tớ biết cậu ở đây.” Sasha thì thầm rất khẽ.`
	parts := SplitForTTS(para, 50)
	if len(parts) != 2 {
		t.Fatalf("muốn cắt ngay sau lời thoại thành 2 phần, nhận %d: %q", len(parts), parts)
	}
	for _, p := range parts {
		open := strings.Count(p, "“")
		close := strings.Count(p, "”")
		if open != close {
			t.Errorf("phần bị cắt giữa cặp nháy (mở %d, đóng %d): %q", open, close, p)
		}
	}
}

func TestSplitForTTS_CatCungKhiMotCauQuaDai(t *testing.T) {
	long := strings.Repeat("từ ", 60) // không có dấu kết câu nào
	parts := SplitForTTS(strings.TrimSpace(long), 50)
	if len(parts) < 2 {
		t.Fatalf("câu quá dài phải bị cắt cứng, nhận %d phần", len(parts))
	}
	for _, p := range parts {
		if n := len([]rune(p)); n > 50 {
			t.Errorf("phần dài %d rune, vượt ngưỡng 50: %q", n, p)
		}
	}
}

func TestSplitForTTS_CatCungKhiKhongCoKhoangTrang(t *testing.T) {
	parts := SplitForTTS(strings.Repeat("x", 130), 50)
	if len(parts) != 3 {
		t.Fatalf("muốn 3 phần, nhận %d: %v", len(parts), parts)
	}
	for _, p := range parts {
		if n := len([]rune(p)); n > 50 {
			t.Errorf("phần dài %d rune: %q", n, p)
		}
	}
}

func TestSplitForTTS_DemTheoRuneKhongPhaiByte(t *testing.T) {
	// 100 rune tiếng Việt có dấu ≈ 300 byte. Đếm byte sẽ chia nhầm thành nhiều phần.
	text := strings.Repeat("ế", 100)
	parts := SplitForTTS(text, 120)
	if len(parts) != 1 {
		t.Errorf("100 rune dưới ngưỡng 120 phải là 1 phần, nhận %d (đang đếm byte?)", len(parts))
	}
}

func TestSplitForTTS_TatDinh(t *testing.T) {
	// Chia phải tất định để resume với Overwrite=false sinh đúng tên phần cũ.
	text := strings.Repeat("Một câu ngắn. ", 40)
	a := SplitForTTS(text, 100)
	b := SplitForTTS(text, 100)
	if len(a) != len(b) {
		t.Fatalf("số phần không ổn định: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("phần %d khác nhau giữa hai lần chia", i)
		}
	}
}

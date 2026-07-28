package comic

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/comicdraw"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

// fakeLLM trả JSON cố định theo marker trong system prompt.
type fakeLLM struct{ calls int }

func (f *fakeLLM) Generate(_ context.Context, messages []agentcore.Message, _ []agentcore.ToolSpec, _ ...agentcore.CallOption) (*agentcore.LLMResponse, error) {
	f.calls++
	sys := messages[0].TextContent()
	var payload string
	switch {
	case strings.Contains(sys, "COMICSTYLE"):
		payload = `{"overall":"màu nước ấm","palette":["vàng mật ong"],"line_art":"nét mềm",
			"lettering":"bong bóng bo tròn","style_tokens":["warm watercolor storybook"],
			"negative":["modern objects"],"locations":[{"name":"Phố Havana","image_prompt":"a Havana street"}]}`
	case strings.Contains(sys, "COMICCHARACTER"):
		payload = `{"characters":[
			{"name":"Lâm","role":"chính","appearance":"thiếu niên gầy","wardrobe":"áo lanh",
			 "palette":["nâu gỗ"],"canonical_prompt":"a lean Vietnamese boy in a linen shirt",
			 "sheet_prompt":"character turnaround sheet","negative_prompt":"adult proportions"}]}`
	case strings.Contains(sys, "COMICSCRIPT"):
		payload = `{"chapter":0,"title":"Mở đầu","pages":[
			{"page_no":1,"beat":"giới thiệu","cliff":"Ai đang đợi sau cánh cửa?","panels":[
			  {"panel_no":1,"size":"lon","shot":"toan","description":"Toàn cảnh phố",
			   "characters":["Lâm"],"image_prompt":"a wide street view","reserve_for":"tren-trai",
			   "balloons":[{"order":0,"kind":"thuyet-minh","text":"Havana, một chiều tháng Chín.","anchor":"tren-trai"}]},
			  {"panel_no":2,"size":"nho","shot":"can","description":"Cận mặt Lâm",
			   "characters":["Lâm"],"image_prompt":"close up of a boy",
			   "balloons":[{"order":0,"kind":"thoai","speaker":"Lâm","text":"Mình sẽ thắng ván này!","anchor":"tren-phai","tail_to":"duoi-trai"}],
			   "sfx":[{"text":"CẠCH!","anchor":"duoi-phai","scale":"vua"}]},
			  {"panel_no":3,"size":"vua","shot":"trung","description":"Bàn cờ",
			   "characters":[],"image_prompt":"a chessboard",
			   "balloons":[{"order":0,"kind":"doc-thoai","speaker":"Lâm","text":"Nước đi ấy mình đã thấy từ lâu rồi.","anchor":"duoi-trai","tail_to":"tren"}]}]},
			{"page_no":2,"beat":"cao trào","panels":[
			  {"panel_no":1,"size":"tran-trang","shot":"toan","description":"Toàn cảnh ván cờ",
			   "characters":["Lâm"],"image_prompt":"an epic chess match",
			   "balloons":[{"order":0,"kind":"het","speaker":"Lâm","text":"CHIẾU TƯỚNG!","anchor":"tren-giua","tail_to":"duoi"}]}]}]}`
	default:
		payload = `{}`
	}
	return &agentcore.LLMResponse{
		Message: agentcore.Message{
			Role:    agentcore.RoleAssistant,
			Content: []agentcore.ContentBlock{agentcore.TextBlock("<output>\n" + payload + "\n</output>")},
		},
	}, nil
}

// tolerantTempDir tạo thư mục tạm với dọn dẹp khoan dung — RemoveAll trên Windows hay báo
// "directory not empty" ngay sau fsync/rename.
func tolerantTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "comic-test-*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() {
		for i := 0; i < 10; i++ {
			if err := os.RemoveAll(dir); err == nil {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	})
	return dir
}

func newTestStore(t *testing.T, completed []int) *store.Store {
	t.Helper()
	dir := tolerantTempDir(t)
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	if err := s.Progress.Init("Ván Cờ Havana", len(completed)); err != nil {
		t.Fatalf("init progress: %v", err)
	}
	for _, ch := range completed {
		if err := s.Drafts.SaveFinalChapter(ch, fmt.Sprintf("Nội dung chương %d. Lâm bước tới bàn cờ.", ch)); err != nil {
			t.Fatalf("save chapter: %v", err)
		}
		if err := s.Progress.StartChapter(ch); err != nil {
			t.Fatalf("start chapter: %v", err)
		}
		if err := s.Progress.MarkChapterComplete(ch, 8, "cliff", "main"); err != nil {
			t.Fatalf("mark complete: %v", err)
		}
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "Mở đầu", Scenes: []string{"Vào quán"}},
	}); err != nil {
		t.Fatalf("save outline: %v", err)
	}
	if err := s.Characters.Save([]domain.Character{
		{Name: "Lâm", Role: "chính", Description: "thiếu niên", Tier: "core"},
	}); err != nil {
		t.Fatalf("save characters: %v", err)
	}
	return s
}

func testFonts(t *testing.T) *comicdraw.FontSet {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "assets", "fonts")
	rd := func(n string) []byte {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatalf("đọc font %s: %v", n, err)
		}
		return b
	}
	fs, err := comicdraw.NewFontSet(rd("PatrickHand-Regular.ttf"), rd("Bangers-Regular.ttf"),
		rd("BeVietnamPro-Regular.ttf"), rd("BeVietnamPro-Bold.ttf"))
	if err != nil {
		t.Fatalf("dựng bộ font: %v", err)
	}
	return fs
}

func testPrompts() Prompts {
	return Prompts{Style: "COMICSTYLE", Character: "COMICCHARACTER", Script: "COMICSCRIPT"}
}

func drain(t *testing.T, ch <-chan Event) []Event {
	t.Helper()
	var out []Event
	for ev := range ch {
		out = append(out, ev)
		if ev.Stage == StageError {
			t.Errorf("lỗi trong đường ống: %s — %v", ev.Message, ev.Err)
		}
	}
	return out
}

// TestRunFullPipeline chạy TRỌN đường ống giai đoạn 1 và kiểm chứng mọi sản phẩm.
//
// Giai đoạn 1 không có nguồn sinh ảnh nên khung dùng ảnh giữ chỗ — nhưng trang, PDF, CBZ,
// EPUB đều là thật. Đó chính là điều khiến giai đoạn 1 kiểm được bố cục và typography
// tiếng Việt trước khi tiêu một đồng nào cho việc sinh ảnh.
func TestRunFullPipeline(t *testing.T) {
	st := newTestStore(t, []int{1})
	out := tolerantTempDir(t)

	deps := Deps{Store: st, LLM: &fakeLLM{}, Prompts: testPrompts(), Fonts: testFonts(t)}
	ch, err := Run(context.Background(), deps, Options{OutDir: out, PageSize: "a4"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	events := drain(t, ch)
	if len(events) == 0 || events[len(events)-1].Stage != StageDone {
		t.Fatalf("đường ống không kết thúc bằng StageDone, sự kiện cuối = %+v", events[len(events)-1])
	}

	must := []string{
		"style/art-direction.json",
		"style/art-direction.md",
		"nhan-vat/characters.json",
		"nhan-vat/characters.md",
		"kich-ban/01.json",
		"kich-ban/01.md",
		"bo-cuc/01.json",
		"prompts/chuong-001.json",
		"prompts/chuong-001.md",
		"trang/chuong-001/01.png",
		"trang/chuong-001/01.svg",
		"trang/chuong-001/02.png",
		"tap-01/chuong-001/muc-luc.md",
		"index.md",
		"xuat-ban/Ván Cờ Havana.pdf",
		"xuat-ban/Ván Cờ Havana.cbz",
		"xuat-ban/Ván Cờ Havana.epub",
	}
	for _, rel := range must {
		p := filepath.Join(out, filepath.FromSlash(rel))
		fi, err := os.Stat(p)
		if err != nil {
			t.Errorf("thiếu sản phẩm %s: %v", rel, err)
			continue
		}
		if fi.Size() == 0 {
			t.Errorf("sản phẩm %s rỗng", rel)
		}
	}

	// Đặt COMIC_DEMO_OUT=<thư mục> để giữ lại toàn bộ sản phẩm mà xem bằng mắt.
	// Bố cục và typography là thứ chỉ mắt người nghiệm thu được.
	if dir := os.Getenv("COMIC_DEMO_OUT"); dir != "" {
		if err := copyTree(out, filepath.Join(dir, "truyen-tranh-demo")); err != nil {
			t.Fatalf("sao chép sản phẩm: %v", err)
		}
		t.Logf("đã giữ sản phẩm tại %s", filepath.Join(dir, "truyen-tranh-demo"))
	}
}

// copyTree sao chép đệ quy một cây thư mục.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// TestPDFStructure kiểm tra bộ ghi PDF tự viết: đúng header, đúng số trang, và — quan
// trọng nhất — MỌI offset trong bảng xref phải trỏ đúng vào "<id> 0 obj".
// Một phép kiểm này bắt được toàn bộ lỗi lệch byte của bộ ghi.
func TestPDFStructure(t *testing.T) {
	st := newTestStore(t, []int{1})
	out := tolerantTempDir(t)
	deps := Deps{Store: st, LLM: &fakeLLM{}, Prompts: testPrompts(), Fonts: testFonts(t)}
	ch, err := Run(context.Background(), deps, Options{OutDir: out, Formats: []Format{FormatPDF}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	drain(t, ch)

	data, err := os.ReadFile(filepath.Join(out, "xuat-ban", "Ván Cờ Havana.pdf"))
	if err != nil {
		t.Fatalf("đọc PDF: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-1.7")) {
		t.Error("thiếu header %PDF-1.7")
	}
	if !bytes.Contains(data, []byte("/Type /Pages")) || !bytes.Contains(data, []byte("/DCTDecode")) {
		t.Error("thiếu cây trang hoặc ảnh DCTDecode")
	}
	if !bytes.HasSuffix(bytes.TrimRight(data, "\r\n"), []byte("%%EOF")) {
		t.Error("thiếu dấu kết thúc tệp PDF")
	}

	// Kiểm tra bảng xref.
	i := bytes.LastIndex(data, []byte("\nxref\n"))
	if i < 0 {
		t.Fatal("không tìm thấy bảng xref")
	}
	body := data[i+len("\nxref\n"):]
	var start, count int
	if _, err := fmt.Sscanf(string(body[:bytes.IndexByte(body, '\n')]), "%d %d", &start, &count); err != nil {
		t.Fatalf("đọc đầu mục xref: %v", err)
	}
	rows := body[bytes.IndexByte(body, '\n')+1:]
	const entryLen = 20
	for id := 1; id < count; id++ {
		entry := rows[id*entryLen : (id+1)*entryLen]
		var off int
		if _, err := fmt.Sscanf(string(entry[:10]), "%d", &off); err != nil {
			t.Fatalf("đọc offset của object %d: %v", id, err)
		}
		want := []byte(fmt.Sprintf("%d 0 obj", id))
		if off <= 0 || off+len(want) > len(data) || !bytes.HasPrefix(data[off:], want) {
			t.Errorf("offset xref của object %d sai: trỏ tới %q", id, snippet(data, off))
		}
	}
}

func snippet(data []byte, off int) string {
	if off < 0 || off >= len(data) {
		return "<ngoài phạm vi>"
	}
	end := off + 20
	if end > len(data) {
		end = len(data)
	}
	return string(data[off:end])
}

// TestCBZStructure kiểm tra CBZ: ảnh đánh số 3 chữ số, không nén, có ComicInfo.xml.
func TestCBZStructure(t *testing.T) {
	st := newTestStore(t, []int{1})
	out := tolerantTempDir(t)
	deps := Deps{Store: st, LLM: &fakeLLM{}, Prompts: testPrompts(), Fonts: testFonts(t)}
	ch, err := Run(context.Background(), deps, Options{OutDir: out, Formats: []Format{FormatCBZ}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	drain(t, ch)

	data, err := os.ReadFile(filepath.Join(out, "xuat-ban", "Ván Cờ Havana.cbz"))
	if err != nil {
		t.Fatalf("đọc CBZ: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("mở CBZ: %v", err)
	}
	var pages, info int
	for _, f := range zr.File {
		switch {
		case strings.HasSuffix(f.Name, ".png"):
			pages++
			if len(f.Name) != len("001.png") {
				t.Errorf("tên ảnh phải là 3 chữ số, nhận %q", f.Name)
			}
			if f.Method != zip.Store {
				t.Errorf("ảnh %s phải để Store (PNG đã nén sẵn), nhận method %d", f.Name, f.Method)
			}
		case f.Name == "ComicInfo.xml":
			info++
		}
	}
	if pages != 2 {
		t.Errorf("CBZ có %d trang, muốn 2", pages)
	}
	if info != 1 {
		t.Error("CBZ thiếu ComicInfo.xml")
	}
}

// TestEPUBStructure kiểm tra EPUB3 fixed-layout: mimetype đứng đầu + không nén, và OPF
// khai đủ rendition:layout pre-paginated cùng properties="svg".
func TestEPUBStructure(t *testing.T) {
	st := newTestStore(t, []int{1})
	out := tolerantTempDir(t)
	deps := Deps{Store: st, LLM: &fakeLLM{}, Prompts: testPrompts(), Fonts: testFonts(t)}
	ch, err := Run(context.Background(), deps, Options{OutDir: out, Formats: []Format{FormatEPUB}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	drain(t, ch)

	data, err := os.ReadFile(filepath.Join(out, "xuat-ban", "Ván Cờ Havana.epub"))
	if err != nil {
		t.Fatalf("đọc EPUB: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("mở EPUB: %v", err)
	}
	if len(zr.File) == 0 || zr.File[0].Name != "mimetype" {
		t.Fatal("mimetype phải là mục đầu tiên trong zip")
	}
	if zr.File[0].Method != zip.Store {
		t.Error("mimetype phải để Store, không nén")
	}
	var opf string
	for _, f := range zr.File {
		if f.Name == "OEBPS/content.opf" {
			rc, _ := f.Open()
			b, _ := os.ReadFile(os.DevNull)
			_ = b
			var sb strings.Builder
			buf := make([]byte, 4096)
			for {
				n, err := rc.Read(buf)
				sb.Write(buf[:n])
				if err != nil {
					break
				}
			}
			rc.Close()
			opf = sb.String()
		}
	}
	if opf == "" {
		t.Fatal("thiếu OEBPS/content.opf")
	}
	for _, want := range []string{
		`<meta property="rendition:layout">pre-paginated</meta>`,
		`properties="svg"`,
		`page-progression-direction="ltr"`,
		`properties="cover-image"`,
	} {
		if !strings.Contains(opf, want) {
			t.Errorf("OPF thiếu %q", want)
		}
	}
}

// TestResumeSkipsExisting khẳng định chạy lại KHÔNG gọi lại LLM cho phần đã có —
// đây là thứ khiến chạy lại rẻ và là điều kiện để giai đoạn 2 không đốt tiền lặp lại.
func TestResumeSkipsExisting(t *testing.T) {
	st := newTestStore(t, []int{1})
	out := tolerantTempDir(t)

	first := &fakeLLM{}
	ch, err := Run(context.Background(), Deps{Store: st, LLM: first, Prompts: testPrompts(), Fonts: testFonts(t)},
		Options{OutDir: out, Formats: []Format{FormatPNG}})
	if err != nil {
		t.Fatalf("Run lần 1: %v", err)
	}
	drain(t, ch)
	if first.calls == 0 {
		t.Fatal("lần chạy đầu phải gọi LLM")
	}

	second := &fakeLLM{}
	ch, err = Run(context.Background(), Deps{Store: st, LLM: second, Prompts: testPrompts(), Fonts: testFonts(t)},
		Options{OutDir: out, Formats: []Format{FormatPNG}})
	if err != nil {
		t.Fatalf("Run lần 2: %v", err)
	}
	drain(t, ch)
	if second.calls != 0 {
		t.Errorf("chạy lại phải bỏ qua mọi bước LLM đã có, nhưng đã gọi %d lần", second.calls)
	}
}

// TestSkipsImageStepsWithoutSource khẳng định giai đoạn 1 bỏ qua mềm các bước cần ảnh,
// coi đó là thông tin chứ KHÔNG phải lỗi.
func TestSkipsImageStepsWithoutSource(t *testing.T) {
	st := newTestStore(t, []int{1})
	out := tolerantTempDir(t)
	deps := Deps{Store: st, LLM: &fakeLLM{}, Prompts: testPrompts(), Fonts: testFonts(t)}
	ch, err := Run(context.Background(), deps,
		Options{OutDir: out, Products: []Product{ProductRefSheet, ProductPanelArt}, Formats: []Format{FormatPNG}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	events := drain(t, ch)
	var mentioned int
	for _, ev := range events {
		if strings.Contains(ev.Message, "chưa cấu hình nguồn sinh ảnh") {
			mentioned++
		}
	}
	if mentioned != 2 {
		t.Errorf("phải báo bỏ qua 2 bước cần ảnh, nhận %d", mentioned)
	}
	if events[len(events)-1].Stage != StageDone {
		t.Error("thiếu nguồn ảnh không được coi là lỗi")
	}
}

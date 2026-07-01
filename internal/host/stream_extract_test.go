package host

import (
	"strings"
	"testing"
)

// feedAll nap toan bo mot lan, tra ve tich luy dau ra.
func feedAll(t *testing.T, tool, input string) string {
	t.Helper()
	e := newToolExtractor(tool)
	if e == nil {
		t.Fatalf("no extractor for tool %q", tool)
	}
	return e.Feed(input)
}

// feedChunked nap tung manh theo so byte chi dinh, kiem tra luong va nap mot lan cho ket qua giong nhau.
func feedChunked(t *testing.T, tool, input string, chunk int) string {
	t.Helper()
	e := newToolExtractor(tool)
	if e == nil {
		t.Fatalf("no extractor for tool %q", tool)
	}
	var b strings.Builder
	for i := 0; i < len(input); i += chunk {
		end := min(i+chunk, len(input))
		b.WriteString(e.Feed(input[i:end]))
	}
	return b.String()
}

func mustContain(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("expected substring %q in:\n---\n%s\n---", want, got)
	}
}

func mustNotContain(t *testing.T, got, want string) {
	t.Helper()
	if strings.Contains(got, want) {
		t.Errorf("unexpected substring %q in:\n---\n%s\n---", want, got)
	}
}

// ── Che do chung: obj phang ──

func TestExtract_PlanChapter(t *testing.T) {
	in := `{"chapter":1,"title":"Bán mình","goal":"Thiết lập cơ bản mỏ khoáng","conflict":"món nợ cha","hook":"mỏ tro","emotion_arc":"u uất"}`
	out := feedAll(t, "plan_chapter", in)
	mustContain(t, out, "✻ Lập kế hoạch")
	mustContain(t, out, "chapter: 1")
	mustContain(t, out, "title: Bán mình")
	mustContain(t, out, "goal: Thiết lập cơ bản mỏ khoáng")
	mustContain(t, out, "conflict: món nợ cha")
	mustContain(t, out, "hook: mỏ tro")
	mustContain(t, out, "emotion_arc: u uất")
}

// ── Che do chung: obj long nhau + mang ──

func TestExtract_FoundationCharacters(t *testing.T) {
	in := `{"type":"characters","scale":"long","content":[` +
		`{"name":"Trần Lệ","role":"nhân vật chính","aliases":["Tro Mạch","Trần Thất"],"description":"thiếu niên vùng hoang biên。","traits":["kiềm chế","đa nghi"]},` +
		`{"name":"Cố Tiểu Đăng","role":"nhân vật phụ quan trọng","description":"cô bé thử thuốc ở hiệu thuốc。"}` +
		`]}`
	out := feedAll(t, "save_foundation", in)
	mustContain(t, out, "✻ Thiết định")
	mustContain(t, out, "type: characters")
	mustContain(t, out, "scale: long")
	// Hien thi chung: tat ca truong deu hien thi, ke ca aliases / traits truoc day bi bo qua theo whitelist
	mustContain(t, out, "name: Trần Lệ")
	mustContain(t, out, "role: nhân vật chính")
	mustContain(t, out, "aliases:")
	mustContain(t, out, "- Tro Mạch")
	mustContain(t, out, "- Trần Thất")
	mustContain(t, out, "description: thiếu niên vùng hoang biên。")
	mustContain(t, out, "traits:")
	mustContain(t, out, "- kiềm chế")
	mustContain(t, out, "- đa nghi")
	mustContain(t, out, "name: Cố Tiểu Đăng")
	mustContain(t, out, "role: nhân vật phụ quan trọng")
}

func TestExtract_FoundationLayeredOutline(t *testing.T) {
	in := `{"type":"layered_outline","content":[` +
		`{"index":1,"title":"Mỏ Lửa Lờ Mờ","arcs":[` +
		`{"index":1,"title":"Lao Dịch Mỏ Vảy Đen","goal":"cầu sinh tồn","chapters":[` +
		`{"chapter":1,"title":"Bán mình","core_event":"Bị bán vào mỏ khoáng。"}` +
		`]}]}]}`
	out := feedAll(t, "save_foundation", in)
	mustContain(t, out, "type: layered_outline")
	// Tap
	mustContain(t, out, "index: 1")
	mustContain(t, out, "title: Mỏ Lửa Lờ Mờ")
	// Cung
	mustContain(t, out, "title: Lao Dịch Mỏ Vảy Đen")
	mustContain(t, out, "goal: cầu sinh tồn")
	// Chuong
	mustContain(t, out, "chapter: 1")
	mustContain(t, out, "title: Bán mình")
	mustContain(t, out, "core_event: Bị bán vào mỏ khoáng。")
	// Thut hang long nhau the hien cap do
	mustContain(t, out, "arcs:\n")
	mustContain(t, out, "chapters:\n")
}

func TestExtract_FoundationUpdateCompass(t *testing.T) {
	in := `{"type":"update_compass","content":{"ending_direction":"Một mình thăng thiên vs chặt đứt tế máu","open_threads":["Chìa khóa Tro Mạch","Sổ vé người sống"],"estimated_scale":"5-6 tập"}}`
	out := feedAll(t, "save_foundation", in)
	mustContain(t, out, "type: update_compass")
	mustContain(t, out, "ending_direction: Một mình thăng thiên vs chặt đứt tế máu")
	mustContain(t, out, "estimated_scale: 5-6 tập")
	mustContain(t, out, "open_threads:")
	mustContain(t, out, "- Chìa khóa Tro Mạch")
	mustContain(t, out, "- Sổ vé người sống")
}

// ── save_review: chua mang doi tuong + mang so ──

func TestExtract_SaveReview(t *testing.T) {
	in := `{"chapter":3,"scope":"chapter","verdict":"polish","summary":"nhịp điệu hơi chậm。","dimensions":[{"dimension":"hook","score":55,"verdict":"fail"}],"issues":[{"type":"hook","severity":"error","description":"thiếu móc câu cuối chương。"}],"affected_chapters":[3,4]}`
	out := feedAll(t, "save_review", in)
	mustContain(t, out, "✻ Xem xét")
	mustContain(t, out, "verdict: polish")
	mustContain(t, out, "summary: nhịp điệu hơi chậm。")
	mustContain(t, out, "dimension: hook")
	mustContain(t, out, "score: 55")
	mustContain(t, out, "verdict: fail")
	mustContain(t, out, "type: hook")
	mustContain(t, out, "severity: error")
	mustContain(t, out, "description: thiếu móc câu cuối chương。")
	mustContain(t, out, "- 3")
	mustContain(t, out, "- 4")
}

// ── commit_chapter: long nhau phuc tap ──

func TestExtract_CommitChapter(t *testing.T) {
	in := `{"chapter":1,"summary":"Bị bán vào mỏ khoáng。","characters":["Trần Lệ","mẹ"],"key_events":["ký hợp đồng bán mình"],"foreshadow_updates":[{"id":"f1","action":"plant","description":"mỏ tro nóng bỏng。"}],"state_changes":[{"entity":"Trần Lệ","field":"danh phận","old_value":"thiếu niên hái thuốc","new_value":"tạp dịch mỏ khoáng"}]}`
	out := feedAll(t, "commit_chapter", in)
	mustContain(t, out, "✻ Nộp chương")
	mustContain(t, out, "summary: Bị bán vào mỏ khoáng。")
	mustContain(t, out, "- Trần Lệ")
	mustContain(t, out, "- mẹ")
	mustContain(t, out, "- ký hợp đồng bán mình")
	mustContain(t, out, "id: f1")
	mustContain(t, out, "action: plant")
	mustContain(t, out, "description: mỏ tro nóng bỏng。")
	mustContain(t, out, "entity: Trần Lệ")
	mustContain(t, out, "field: danh phận")
	mustContain(t, out, "old_value: thiếu niên hái thuốc")
	mustContain(t, out, "new_value: tạp dịch mỏ khoáng")
}

// ── edit_chapter: che do chung + string nhieu dong ──

func TestExtract_EditChapter(t *testing.T) {
	in := `{"chapter":24,"old_string":"Trần Lệ cúi đầu không nói。\nHắn nắm chặt nắm đấm。","new_string":"Trần Lệ không ngẩng đầu, quả hầu chuyển động。\nkhớp ngón tay nắm đến tái trắng。","replace_all":false}`
	out := feedAll(t, "edit_chapter", in)
	mustContain(t, out, "✻ Chỉnh sửa")
	mustContain(t, out, "chapter: 24")
	mustContain(t, out, "old_string: Trần Lệ cúi đầu không nói。\nHắn nắm chặt nắm đấm。")
	mustContain(t, out, "new_string: Trần Lệ không ngẩng đầu, quả hầu chuyển động。\nkhớp ngón tay nắm đến tái trắng。")
	mustContain(t, out, "replace_all: false")
}

// ── Cong cu doc: mat do thong tin args thap nhung header + truong chinh van phai hien thi ──

func TestExtract_ReadChapter(t *testing.T) {
	in := `{"chapter":234,"source":"final"}`
	out := feedAll(t, "read_chapter", in)
	mustContain(t, out, "✻ Đọc chương")
	mustContain(t, out, "chapter: 234")
	mustContain(t, out, "source: final")
}

func TestExtract_CheckConsistency(t *testing.T) {
	out := feedAll(t, "check_consistency", `{"chapter":234}`)
	mustContain(t, out, "✻ Kiểm tra nhất quán")
	mustContain(t, out, "chapter: 234")
}

// Du phong args rong: khi coordinator goi novel_context khong truyen tham so thi args la {},
// khong the im lang hoan toan, it nhat phai xuat header de nguoi dung nhan thay cuoc goi.
func TestExtract_NovelContextEmptyArgs(t *testing.T) {
	out := feedAll(t, "novel_context", `{}`)
	mustContain(t, out, "✻ Tra cứu ngữ cảnh")
}

func TestExtract_NovelContextWithChapter(t *testing.T) {
	out := feedAll(t, "novel_context", `{"chapter":234}`)
	mustContain(t, out, "✻ Tra cứu ngữ cảnh")
	mustContain(t, out, "chapter: 234")
}

// ── Che do luong tran ──

func TestExtract_DraftChapterRawMarkdown(t *testing.T) {
	in := `{"chapter":1,"content":"# Chương 1\n\nTrần Lệ đứng ở cửa mỏ。\n"}`
	out := feedAll(t, "draft_chapter", in)
	// Luong tran: khong trang tri, khong tien to key
	mustNotContain(t, out, "【")
	mustNotContain(t, out, "content:")
	mustNotContain(t, out, "chapter:")
	mustContain(t, out, "# Chương 1")
	mustContain(t, out, "Trần Lệ đứng ở cửa mỏ。")
}

func TestExtract_DraftChapterIgnoresOtherFields(t *testing.T) {
	// Cac truong ngoai content phai bi bo qua im lang, khong lam o nhiem dau ra
	in := `{"chapter":7,"summary":"meta","content":"nội dung","extra_array":[1,2,3]}`
	out := feedAll(t, "draft_chapter", in)
	mustContain(t, out, "nội dung")
	mustNotContain(t, out, "meta")
	mustNotContain(t, out, "summary")
	mustNotContain(t, out, "7")
	mustNotContain(t, out, "1")
}

// ── Bat bien hanh vi ──

func TestExtract_UnknownTool(t *testing.T) {
	if e := newToolExtractor("nonexistent_tool"); e != nil {
		t.Errorf("expected nil for unknown tool")
	}
}

func TestExtract_DoneAfterClose(t *testing.T) {
	e := newToolExtractor("plan_chapter")
	e.Feed(`{"title":"x"}`)
	if !e.Done() {
		t.Error("expected Done after closing brace")
	}
}

// ── Bat bien phan manh luong ──

// Cung mot dau vao chia tung 1/3/7/13 byte, dau ra phai giong hoan toan voi nap mot lan.
func TestExtract_ChunkedEqualsWhole(t *testing.T) {
	cases := []struct {
		tool  string
		input string
	}{
		{"plan_chapter", `{"title":"Bán mình","goal":"mục tiêu","conflict":"món nợ cha","hook":"mỏ tro","emotion_arc":"u uất"}`},
		{"save_foundation", `{"type":"characters","content":[{"name":"Trần Lệ","role":"nhân vật chính","aliases":["Tro Mạch","Trần Thất"]}]}`},
		{"save_foundation", `{"type":"layered_outline","content":[{"index":1,"title":"Mỏ Lửa","arcs":[{"index":1,"title":"Lao Dịch Mỏ","goal":"cầu sinh tồn","chapters":[{"chapter":1,"title":"Bán mình"}]}]}]}`},
		{"save_review", `{"verdict":"accept","summary":"good","dimensions":[{"dimension":"hook","score":85,"verdict":"pass"}],"issues":[]}`},
		{"draft_chapter", `{"chapter":1,"content":"# Chương 1\n\nnội dung。\n"}`},
	}
	for _, tc := range cases {
		whole := feedAll(t, tc.tool, tc.input)
		for _, chunk := range []int{1, 3, 7, 13} {
			got := feedChunked(t, tc.tool, tc.input, chunk)
			if got != whole {
				t.Errorf("tool=%s chunk=%d differs from whole\n--- whole ---\n%s\n--- chunked ---\n%s", tc.tool, chunk, whole, got)
			}
		}
	}
}

// ── Escape va Unicode ──

func TestExtract_EscapeSequences(t *testing.T) {
	in := `{"goal":"行1\n行2 \"引号\" \\反斜线 中字"}`
	out := feedAll(t, "plan_chapter", in)
	mustContain(t, out, "行1\n行2")
	mustContain(t, out, `"引号"`)
	mustContain(t, out, `\反斜线`)
	mustContain(t, out, "中字")
}

func TestExtract_UnicodeEscape(t *testing.T) {
	// 中 = 中 (escape Unicode)
	in := `{"goal":"中文"}`
	out := feedAll(t, "plan_chapter", in)
	mustContain(t, out, "中文")
}

// ── Container rong / cau truc don gian ──

func TestExtract_EmptyArrays(t *testing.T) {
	in := `{"key_events":[],"characters":["Trần Lệ"]}`
	out := feedAll(t, "commit_chapter", in)
	mustContain(t, out, "key_events:")
	mustContain(t, out, "characters:")
	mustContain(t, out, "- Trần Lệ")
}

func TestExtract_BoolAndNull(t *testing.T) {
	in := `{"foreshadow_updates":[{"id":"f1","action":"plant","description":null}],"chapter":1,"summary":"x","characters":["a"],"key_events":["b"]}`
	out := feedAll(t, "commit_chapter", in)
	mustContain(t, out, "id: f1")
	mustContain(t, out, "action: plant")
	mustContain(t, out, "description: null")
}

// ── Truong hop bien: mang long mang, long nhau sau ──

func TestExtract_NestedArrays(t *testing.T) {
	// affected_chapters la mang int; o day doi thanh mang long mang de kiem tra
	in := `{"summary":"x","key_events":[],"characters":["a"],"foreshadow_updates":[],"relationship_changes":[]}`
	out := feedAll(t, "commit_chapter", in)
	mustContain(t, out, "summary: x")
	mustContain(t, out, "key_events:")
	mustContain(t, out, "- a")
}

func TestExtract_DeeplyNested(t *testing.T) {
	in := `{"a":{"b":{"c":{"d":"deep"}}}}`
	e := newToolExtractor("plan_chapter")
	out := e.Feed(in)
	mustContain(t, out, "a:")
	mustContain(t, out, "b:")
	mustContain(t, out, "c:")
	mustContain(t, out, "d: deep")
	if !e.Done() {
		t.Error("expected Done after final closing brace")
	}
}

// ── chunk cat vao giua da byte utf-8 ──

func TestExtract_ChunkSplitInUTF8(t *testing.T) {
	// "中" la 3 byte (E4 B8 AD). Dat kich thuoc manh la 1, dam bao moi byte duoc nap rieng le.
	in := `{"goal":"中文测试"}`
	whole := feedAll(t, "plan_chapter", in)
	chunked := feedChunked(t, "plan_chapter", in, 1)
	if whole != chunked {
		t.Errorf("byte-by-byte chunked output differs from whole:\n--- whole ---\n%s\n--- chunked ---\n%s", whole, chunked)
	}
	mustContain(t, chunked, "中文测试")
}

// ── Che do luong tran: key cung ten trong obj long nhau khong duoc trung khop nham ──

func TestExtract_NakedKeyOnlyTopLevel(t *testing.T) {
	// "content" xuat hien o hai noi: trong doi tuong long nhau + cap cao nhat. Chi cai cap cao nhat moi duoc xuat luong.
	in := `{"meta":{"content":"嵌套不应输出"},"content":"顶层应输出"}`
	out := feedAll(t, "draft_chapter", in)
	mustContain(t, out, "顶层应输出")
	mustNotContain(t, out, "嵌套不应输出")
}

// ── Che do luong tran: bo qua hoan toan khi content khong phai string ──

func TestExtract_NakedKeyNonStringValue(t *testing.T) {
	// content viet nham thanh doi tuong (khong nen xay ra nhung phai chap nhan)
	in := `{"content":{"unexpected":true}}`
	out := feedAll(t, "draft_chapter", in)
	if out != "" {
		t.Errorf("expected empty output, got: %q", out)
	}
}

// ── Sau khi container cap cao nhat dong lai, Feed nua khong phat them ──

func TestExtract_FeedAfterDone(t *testing.T) {
	e := newToolExtractor("plan_chapter")
	e.Feed(`{"title":"x"}`)
	if !e.Done() {
		t.Fatal("expected Done")
	}
	if got := e.Feed(`junk`); got != "" {
		t.Errorf("expected empty output after Done, got: %q", got)
	}
}

// ── Chunk rong / input rong ──

func TestExtract_EmptyFeed(t *testing.T) {
	e := newToolExtractor("plan_chapter")
	if got := e.Feed(""); got != "" {
		t.Errorf("expected empty output for empty feed, got: %q", got)
	}
	if e.Done() {
		t.Error("Done should be false before any input")
	}
}

// ── Trong mang long mang truc tiep (khong qua obj) ──

func TestExtract_ArrayOfArrays(t *testing.T) {
	in := `{"matrix":[[1,2],[3,4]]}`
	out := feedAll(t, "plan_chapter", in)
	mustContain(t, out, "matrix:")
	mustContain(t, out, "- 1")
	mustContain(t, out, "- 2")
	mustContain(t, out, "- 3")
	mustContain(t, out, "- 4")
}

// ── number sau do khoang trang roi phan cach ──

func TestExtract_NumberWithTrailingSpace(t *testing.T) {
	// "chapter": 1 ,  <- nhieu khoang trang truoc va sau so
	in := `{ "chapter" : 1 , "title" : "x" }`
	out := feedAll(t, "plan_chapter", in)
	mustContain(t, out, "chapter: 1")
	mustContain(t, out, "title: x")
}

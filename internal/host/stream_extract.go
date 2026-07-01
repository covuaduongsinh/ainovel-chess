package host

import (
	"strings"
	"unicode/utf8"
)

// toolDisplays cau hinh chien luoc hien thi cua moi cong cu tren panel luong. Cong cu khong co trong bang nay
// khong tham gia hien thi luong (observer huy bo DeltaToolCall ngay).
//
// Che do chung (nakedKey rong): tokenizer hien thi args JSON ma LLM xuat ra thanh van ban thut hang
// "key: value", doi tuong/mang long nhau thut hang theo cap do, string/number/bool xuat luong.
// Hoan toan tach roi khoi schema -- LLM xuat them mot truong thi panel them mot dong, khong can thay doi code.
//
// Che do luong tran (nakedKey khong rong): chi xuat nguyen xi gia tri string cua truong dich o cap cao nhat,
// cac truong khac bo qua het. Dung cho draft_chapter, de toan bo chuong markdown khong bi trang tri thanh "content: # ...".
// Header luon bat dau bang "Lam" ": TUI renderStreamContent di qua renderAgentBlock
// duong lam noi bat (vang Lam + nen xanh chu thich gach chan xanh + duong dim) la tien to quy uoc,
// nhat quan voi fallback header (streamHeaderFallback); doi thanh chu binh thuong se roi vao duong van ban
// dung mau mac dinh cua terminal, title khong con noi bat.
var toolDisplays = map[string]toolDisplay{
	"draft_chapter": {nakedKey: "content"},

	"plan_chapter":        {header: "✻ Lập kế hoạch"},
	"edit_chapter":        {header: "✻ Chỉnh sửa"},
	"commit_chapter":      {header: "✻ Nộp chương"},
	"save_review":         {header: "✻ Xem xét"},
	"save_arc_summary":    {header: "✻ Tóm tắt cung"},
	"save_volume_summary": {header: "✻ Tóm tắt tập"},
	"save_foundation":     {header: "✻ Thiết định"},
	"read_chapter":        {header: "✻ Đọc chương"},
	"check_consistency":   {header: "✻ Kiểm tra nhất quán"},
	"novel_context":       {header: "✻ Tra cứu ngữ cảnh"},
}

type toolDisplay struct {
	header   string
	nakedKey string
}

// jsonFieldExtractor la bo tach truong JSON luong. Dieu khien may trang thai tung byte, bien doi
// args cong cu cua LLM thanh van ban co the doc duoc. Mot instance chi phuc vu mot lan goi cong cu, Done()=true sau khi container cap cao nhat dong lai.
type jsonFieldExtractor struct {
	cfg toolDisplay

	state pState
	stack []byte // Ngan xep container: 'O' obj / 'A' arr

	keyBuf strings.Builder

	escape bool
	uHex   []byte

	started bool // Da emit bat ky ky tu nao chua (dung cho xuong dong giua header va key dau tien)

	done bool
}

type pState int

const (
	psRoot         pState = iota
	psBeforeKey           // Trong obj: cho key tiep theo hoac }
	psInKey               // Trong obj: phan tich key
	psAfterKey            // Trong obj: cho :
	psBeforeValue         // Cho ky tu bat dau value
	psStringStream        // Gia tri string, emit ky tu da xu ly theo luong
	psStringSkip          // Gia tri string, bo qua (truong khong phai muc tieu trong che do luong tran)
	psNumberStream        // So, emit theo luong
	psNumberSkip          // So, bo qua
	psPrimStream          // true/false/null, emit theo luong
	psPrimSkip            // true/false/null, bo qua
	psDone                // Container cap cao nhat da dong
)

func newToolExtractor(tool string) *jsonFieldExtractor {
	cfg, ok := toolDisplays[tool]
	if !ok {
		return nil
	}
	return &jsonFieldExtractor{cfg: cfg}
}

func (e *jsonFieldExtractor) Done() bool { return e.done }

func (e *jsonFieldExtractor) Feed(chunk string) string {
	if e.done || chunk == "" {
		return ""
	}
	var out strings.Builder
	for i := 0; i < len(chunk); i++ {
		e.step(chunk[i], &out)
		if e.done {
			break
		}
	}
	return out.String()
}

// ── Ngan xep container / Thut hang ──

func (e *jsonFieldExtractor) push(kind byte) {
	e.stack = append(e.stack, kind)
}

func (e *jsonFieldExtractor) pop() {
	if len(e.stack) == 0 {
		return
	}
	e.stack = e.stack[:len(e.stack)-1]
}

func (e *jsonFieldExtractor) parent() byte {
	if len(e.stack) == 0 {
		return 0
	}
	return e.stack[len(e.stack)-1]
}

// writeIndent ghi thut hang hien tai. Do sau = so cap long nhau = len(stack)-1 (noi bo container goc khong thut hang).
func (e *jsonFieldExtractor) writeIndent(out *strings.Builder) {
	depth := len(e.stack) - 1
	for range depth {
		out.WriteString("  ")
	}
}

// ── May trang thai ──

func (e *jsonFieldExtractor) step(c byte, out *strings.Builder) {
	switch e.state {
	case psRoot:
		switch c {
		case '{':
			e.push('O')
			e.state = psBeforeKey
		case '[':
			// Thuc te khong xay ra (tool args luon la obj); chap nhan: khi la root arr
			e.push('A')
			e.state = psBeforeValue
		}
	case psBeforeKey:
		switch c {
		case '"':
			e.keyBuf.Reset()
			e.escape = false
			e.state = psInKey
		case '}':
			e.closeContainer(out)
		case ' ', '\t', '\n', '\r', ',':
		}
	case psInKey:
		if e.escape {
			e.keyBuf.WriteByte(c)
			e.escape = false
			return
		}
		if c == '\\' {
			e.escape = true
			return
		}
		if c == '"' {
			e.emitKeyLine(out, e.keyBuf.String())
			e.state = psAfterKey
			return
		}
		e.keyBuf.WriteByte(c)
	case psAfterKey:
		if c == ':' {
			e.state = psBeforeValue
		}
	case psBeforeValue:
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ',' {
			return
		}
		switch c {
		case '"':
			e.beginString(out)
		case '{':
			e.beginNested('O', out)
		case '[':
			e.beginNested('A', out)
		case ']', '}':
			e.closeContainer(out)
		case 't', 'f', 'n':
			e.beginPrim(c, out)
		default:
			if c == '-' || (c >= '0' && c <= '9') {
				e.beginNumber(c, out)
			}
		}
	case psStringStream:
		e.handleStringByte(c, out, false)
	case psStringSkip:
		e.handleStringByte(c, out, true)
	case psNumberStream:
		if isNumberByte(c) {
			out.WriteByte(c)
			return
		}
		e.afterValueChar(c, out)
	case psNumberSkip:
		if isNumberByte(c) {
			return
		}
		e.afterValueChar(c, out)
	case psPrimStream:
		if c >= 'a' && c <= 'z' {
			out.WriteByte(c)
			return
		}
		e.afterValueChar(c, out)
	case psPrimSkip:
		if c >= 'a' && c <= 'z' {
			return
		}
		e.afterValueChar(c, out)
	case psDone:
	}
}

// ── Hien thi dong ──

// emitKeyLine duoc goi khi phan tich xong key trong obj, ghi ra tien to "<lf><indent>key:".
// Trong che do luong tran khong ghi tien to key (key duoc luu trong keyBuf de beginString kiem tra).
func (e *jsonFieldExtractor) emitKeyLine(out *strings.Builder, key string) {
	if e.cfg.nakedKey != "" {
		return
	}
	if !e.started {
		if e.cfg.header != "" {
			out.WriteString(e.cfg.header)
			out.WriteByte('\n')
		}
		e.started = true
	} else {
		out.WriteByte('\n')
	}
	e.writeIndent(out)
	out.WriteString(key)
	out.WriteByte(':')
}

// emitArrayItem duoc goi khi bat dau moi phan tu trong arr, ghi ra "<lf><indent>-". Phan tu primitive
// tiep theo la khoang trang roi emit gia tri; phan tu struct duoc xu ly bang cach long nhau tu nhien xuong dong.
func (e *jsonFieldExtractor) emitArrayItem(out *strings.Builder) {
	if e.cfg.nakedKey != "" {
		return
	}
	if !e.started {
		if e.cfg.header != "" {
			out.WriteString(e.cfg.header)
			out.WriteByte('\n')
		}
		e.started = true
	} else {
		out.WriteByte('\n')
	}
	e.writeIndent(out)
	out.WriteByte('-')
}

// ── Bat dau value ──

func (e *jsonFieldExtractor) beginString(out *strings.Builder) {
	if e.cfg.nakedKey != "" {
		// Luong tran: chi xuat gia tri string cua key dich trong obj cap cao nhat
		if e.cfg.nakedKey == e.keyBuf.String() && len(e.stack) == 1 && e.stack[0] == 'O' {
			e.state = psStringStream
		} else {
			e.state = psStringSkip
		}
		e.escape = false
		e.uHex = nil
		return
	}
	// Chung: truong obj tiep theo "key: " (da emit "key:", bo sung them khoang trang); phan tu arr tiep theo "- "
	if e.parent() == 'A' {
		e.emitArrayItem(out)
		out.WriteByte(' ')
	} else {
		out.WriteByte(' ')
	}
	e.state = psStringStream
	e.escape = false
	e.uHex = nil
}

func (e *jsonFieldExtractor) beginNumber(first byte, out *strings.Builder) {
	if e.cfg.nakedKey != "" {
		e.state = psNumberSkip
		return
	}
	if e.parent() == 'A' {
		e.emitArrayItem(out)
		out.WriteByte(' ')
	} else {
		out.WriteByte(' ')
	}
	out.WriteByte(first)
	e.state = psNumberStream
}

func (e *jsonFieldExtractor) beginPrim(first byte, out *strings.Builder) {
	if e.cfg.nakedKey != "" {
		e.state = psPrimSkip
		return
	}
	if e.parent() == 'A' {
		e.emitArrayItem(out)
		out.WriteByte(' ')
	} else {
		out.WriteByte(' ')
	}
	out.WriteByte(first)
	e.state = psPrimStream
}

func (e *jsonFieldExtractor) beginNested(kind byte, out *strings.Builder) {
	if e.cfg.nakedKey != "" {
		// Che do luong tran khong mo rong long nhau; dung do sau ngan xep de theo doi den } / ] tuong ung
		e.push(kind)
		if kind == 'O' {
			e.state = psBeforeKey
		} else {
			e.state = psBeforeValue
		}
		return
	}
	// Che do chung: khi phan tu arr la cau truc long nhau, emit rieng mot dong "<indent>-" truoc
	// (sau ":" cua obj key khong co khoang trang, de key con long nhau tu nhien xuong dong tiep theo)
	if e.parent() == 'A' {
		e.emitArrayItem(out)
	}
	e.push(kind)
	if kind == 'O' {
		e.state = psBeforeKey
	} else {
		e.state = psBeforeValue
	}
}

// closeContainer xu ly } hoac ].
func (e *jsonFieldExtractor) closeContainer(out *strings.Builder) {
	e.pop()
	if len(e.stack) == 0 {
		// Du phong args rong (vi du novel_context khong truyen tham so): emitKeyLine khong co co hoi xuat header,
		// bo sung o day mot lan, tranh roi vao "khong co tieu de cung khong co noi dung".
		if !e.started && e.cfg.nakedKey == "" && e.cfg.header != "" {
			out.WriteString(e.cfg.header)
			out.WriteByte('\n')
			e.started = true
		}
		// Xuong dong cuoi de co ranh gioi ro rang giua panel va doan xuat tiep theo
		if e.started {
			out.WriteByte('\n')
		}
		e.state = psDone
		e.done = true
		return
	}
	if e.parent() == 'O' {
		e.state = psBeforeKey
	} else {
		e.state = psBeforeValue
	}
}

// ── String theo luong ──

func (e *jsonFieldExtractor) handleStringByte(c byte, out *strings.Builder, skipping bool) {
	if e.uHex != nil {
		e.uHex = append(e.uHex, c)
		if len(e.uHex) == 4 {
			if r, ok := parseHex4(e.uHex); ok && !skipping {
				var buf [4]byte
				n := utf8.EncodeRune(buf[:], r)
				out.Write(buf[:n])
			}
			e.uHex = nil
		}
		return
	}
	if e.escape {
		e.escape = false
		if !skipping {
			writeEscapedByte(out, c)
		}
		if c == 'u' {
			e.uHex = make([]byte, 0, 4)
		}
		return
	}
	if c == '\\' {
		e.escape = true
		return
	}
	if c == '"' {
		e.afterValueDone()
		return
	}
	if !skipping {
		out.WriteByte(c)
	}
}

func writeEscapedByte(out *strings.Builder, c byte) {
	switch c {
	case 'n':
		out.WriteByte('\n')
	case 't':
		out.WriteByte('\t')
	case 'r':
		out.WriteByte('\r')
	case '"':
		out.WriteByte('"')
	case '\\':
		out.WriteByte('\\')
	case '/':
		out.WriteByte('/')
	case 'b', 'f':
		// Backspace / form feed: bo qua
	case 'u':
		// Caller tao bo dem uHex; khong xuat o day
	default:
		out.WriteByte('\\')
		out.WriteByte(c)
	}
}

// ── Ket thuc ──

// afterValueDone chuyen sang trang thai tiep theo sau khi string dong lai (doc duoc `"` ket thuc).
func (e *jsonFieldExtractor) afterValueDone() {
	e.escape = false
	e.uHex = nil
	if len(e.stack) == 0 {
		e.state = psDone
		e.done = true
		return
	}
	if e.parent() == 'O' {
		e.state = psBeforeKey
	} else {
		e.state = psBeforeValue
	}
}

// afterValueChar quyet dinh trang thai tiep theo dua vao ky tu khi "ky tu ket thuc" cua number/primitive da duoc doc.
// Ky tu nay co the la , / } / ] / khoang trang, ham nay chuyen tiep va phan phoi.
func (e *jsonFieldExtractor) afterValueChar(c byte, out *strings.Builder) {
	switch c {
	case '}', ']':
		e.closeContainer(out)
	case ',', ' ', '\t', '\n', '\r':
		if len(e.stack) == 0 {
			e.state = psDone
			e.done = true
			return
		}
		if e.parent() == 'O' {
			e.state = psBeforeKey
		} else {
			e.state = psBeforeValue
		}
	}
}

// ── Cong cu ──

func isNumberByte(c byte) bool {
	switch c {
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
		'-', '+', '.', 'e', 'E':
		return true
	}
	return false
}

func parseHex4(b []byte) (rune, bool) {
	var r rune
	for _, d := range b {
		var v rune
		switch {
		case d >= '0' && d <= '9':
			v = rune(d - '0')
		case d >= 'a' && d <= 'f':
			v = rune(d-'a') + 10
		case d >= 'A' && d <= 'F':
			v = rune(d-'A') + 10
		default:
			return 0, false
		}
		r = r*16 + v
	}
	return r, true
}

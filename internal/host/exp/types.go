// Package exp thuc hien kha nang xuat cac chuong da hoan thanh.
//
// Doi xung voi imp/: IO chi cuc bo, khong phu thuoc LLM, khong thay doi trang thai store. Xuat co the chay
// dong thoi voi Coordinator (chi doc Progress + ban thao cuoi chuong), thuoc kha nang ngang.
//
// Phien ban dau chi ho tro TXT; EPUB de lai vong tiep theo.
package exp

import "github.com/voocel/ainovel-cli/internal/store"

// Format xac dinh dinh dang xuat.
type Format string

const (
	// FormatTXT Dau ra van ban thuan.
	FormatTXT Format = "txt"
	// FormatEPUB Container EPUB 3 chuan (zip + xhtml).
	FormatEPUB Format = "epub"
	// FormatMD Dau ra Markdown (tieu de #/##/### + noi dung chuong).
	FormatMD Format = "md"
)

// DefaultFormats la bo dinh dang xuat mac dinh khi caller khong chi dinh hau to cu the:
// Markdown + TXT + EPUB (theo yeu cau "mac dinh xuat 3 dang file").
func DefaultFormats() []Format {
	return []Format{FormatMD, FormatTXT, FormatEPUB}
}

// FormatsForPath quyet dinh danh sach dinh dang tu duong dan nguoi dung nhap.
// Neu hau to duoc nhan biet (.md/.txt/.epub) -> xuat dung 1 dinh dang do (giu kha nang
// go hau to cu the de chi xuat 1 file). Nguoc lai (khong hau to hoac rong) -> DefaultFormats().
// Web va TUI dung chung ham nay de hanh vi mirror nhau.
func FormatsForPath(path string) []Format {
	if f, ok := recognizedExt(path); ok {
		return []Format{f}
	}
	return DefaultFormats()
}

// Options kiem soat hanh vi xuat. zero-value tuong duong "xuat toan bo sang duong dan mac dinh, bao loi neu file ton tai".
//
// Dinh dang: {{TenSach}} -> phan cach tap -> noi dung chuong. Hai loai du lieu noi bo khong vao xuat: premise (ban do sang tac,
// chua doc gia muc tieu / diem tieu thu cot loi / vung cam viet v.v. la meta thong tin hau truong, cho tac gia va engine xem, khong phai loi noi dau cho doc gia);
// phan cach cung (cung la cau truc noi bo qua chi tiet tu goc nhin doc gia). Ten sach va phan cach tap luon giu lai.
type Options struct {
	// Format Khi chuoi rong thi suy ra tu hau to OutPath (.txt -> TXT, .epub -> EPUB, .md -> MD);
	// Khi OutPath cung rong thi tro ve FormatTXT. SDK caller co the chi dinh tuong minh de bo qua suy ra.
	// Bi bo qua khi Formats non-empty.
	Format Format

	// Formats Danh sach dinh dang can xuat trong mot lan (vd DefaultFormats() = md/txt/epub).
	// Khi non-empty: OutPath duoc coi la duong dan base (hau to nhan biet duoc se bi bo), moi dinh
	// dang ghi ra base+"."+ext. Khi rong: giu hanh vi 1-dinh-dang cu theo Format/OutPath.
	Formats []Format

	// OutPath Duong dan file dau ra (hoac base khi Formats non-empty); rong thi dung
	// {novelDir}/{TenSach}.{ext}, ext do Format quyet dinh (TenSach rong thi dung ten thu muc).
	OutPath string

	// From / To Pham vi chuong, dong kin. 0 co nghia la tu chuong 1 / den chuong cuoi.
	// Cac chuong chua hoan thanh trong pham vi se bi bo qua va ghi vao Result.Skipped, khong coi la loi.
	From, To int

	// Overwrite Khi file da ton tai co ghi de khong; mac dinh tu choi.
	Overwrite bool
}

// Deps la phu thuoc can thiet cua Run. Chi store; xuat khong can LLM, prompt, bundle.
type Deps struct {
	Store *store.Store
}

// Output mo ta mot file da ghi trong mot lan xuat.
type Output struct {
	// Format dinh dang cua file.
	Format Format
	// Path duong dan file thuc te duoc ghi.
	Path string
	// Bytes so byte file.
	Bytes int
}

// Result la tom tat san pham cua mot lan xuat thanh cong.
type Result struct {
	// Outputs danh sach file da ghi (mot phan tu khi xuat 1 dinh dang, nhieu khi xuat nhieu).
	Outputs []Output
	// Chapters So chuong thuc te duoc ghi (chung cho moi dinh dang).
	Chapters int
	// Skipped Cac so chuong nam trong pham vi yeu cau nhung chua hoan thanh.
	Skipped []int

	// Path / Bytes la file dau tien trong Outputs, giu de tuong thich voi caller/test cu.
	Path  string
	Bytes int
}

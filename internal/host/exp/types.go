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
)

// Options kiem soat hanh vi xuat. zero-value tuong duong "xuat toan bo sang duong dan mac dinh, bao loi neu file ton tai".
//
// Dinh dang: {{TenSach}} -> phan cach tap -> noi dung chuong. Hai loai du lieu noi bo khong vao xuat: premise (ban do sang tac,
// chua doc gia muc tieu / diem tieu thu cot loi / vung cam viet v.v. la meta thong tin hau truong, cho tac gia va engine xem, khong phai loi noi dau cho doc gia);
// phan cach cung (cung la cau truc noi bo qua chi tiet tu goc nhin doc gia). Ten sach va phan cach tap luon giu lai.
type Options struct {
	// Format Khi chuoi rong thi suy ra tu hau to OutPath (.txt -> TXT, .epub -> EPUB);
	// Khi OutPath cung rong thi tro ve FormatTXT. SDK caller co the chi dinh tuong minh de bo qua suy ra.
	Format Format

	// OutPath Duong dan file dau ra; rong thi dung {novelDir}/{TenSach}.{ext},
	// ext do Format quyet dinh (TenSach rong thi dung ten thu muc).
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

// Result la tom tat san pham cua mot lan xuat thanh cong.
type Result struct {
	// Path Duong dan file thuc te duoc ghi (tuyet doi hoac tuong doi do caller truyen vao).
	Path string
	// Chapters So chuong thuc te duoc ghi.
	Chapters int
	// Bytes So byte file (UTF-8).
	Bytes int
	// Skipped Cac so chuong nam trong pham vi yeu cau nhung chua hoan thanh.
	Skipped []int
}

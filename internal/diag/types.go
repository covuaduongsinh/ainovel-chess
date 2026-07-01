package diag

// Severity bieu thi muc do nghiem trong cua phat hien.
type Severity string

const (
	SevCritical Severity = "critical" // Chan tien do hoac lam hong du lieu
	SevWarning  Severity = "warning"  // Co the lam giam chat luong hoac lang phi token
	SevInfo     Severity = "info"     // Muc co the toi uu
)

// Category nhom cac phat hien theo chieu.
type Category string

const (
	CatFlow     Category = "flow"     // Quy trinh bi ket, trang thai bat thuong, van de phuc hoi
	CatQuality  Category = "quality"  // Diem tham dinh, thuc thi hop dong, nhat quan
	CatPlanning Category = "planning" // Thieu dai cuong, dich chuyen foreshadow, la ban co han
	CatContext  Category = "context"  // Bat thuong nhan vat/dong thoi gian/moi quan he
)

// Confidence bieu thi do tin cay cua phan xet quy tac.
type Confidence string

const (
	ConfHigh   Confidence = "high"   // Do chinh xac cao, dang tin cay
	ConfMedium Confidence = "medium" // Phan xet heuristic, co the co sai sot
	ConfLow    Confidence = "low"    // Tin hieu tho, chi tham khao
)

// AutoLevel bieu thi lieu Finding co the chuyen thanh dong tac tu dong hay khong.
type AutoLevel string

const (
	AutoNone    AutoLevel = "none"    // Chi bao cao, khong tu dong
	AutoSuggest AutoLevel = "suggest" // De xuat dong tac nhung can xac nhan thu cong
	AutoSafe    AutoLevel = "safe"    // Co the tu dong thuc thi an toan
)

// Finding la mot ket qua chan doan co the hanh dong.
type Finding struct {
	Rule       string     // Ten quy tac, vi du "StaleForeshadow"
	Category   Category   // Phan loai
	Severity   Severity   // Muc do nghiem trong
	Confidence Confidence // Do tin cay phan xet
	AutoLevel  AutoLevel  // Cap do tu dong hoa
	Target     string     // Mat tac dong de xuat, vi du "runtime.flow"
	Title      string     // Tom tat mot dong
	Evidence   string     // Bang chung du lieu cu the
	Suggestion string     // De xuat cai thien (tro den prompt/flow/config)
}

// RuleFunc la chu ky thong nhat cua quy tac chan doan.
type RuleFunc func(snap *Snapshot) []Finding

// ActionKind bieu thi loai dong tac chan doan.
type ActionKind string

const (
	ActionEmitNotice      ActionKind = "emit_notice"       // Phat thong bao he thong
	ActionEnqueueFollowUp ActionKind = "enqueue_follow_up" // Nhet follow-up vao coordinator
)

// Action la dong tac co the thuc thi duoc Planner tao ra dua tren Finding do tin cay cao.
type Action struct {
	SourceRule  string     // Ten quy tac nguon goc
	Kind        ActionKind // Loai dong tac
	Severity    Severity   // Ke thua tu Finding
	Summary     string     // Mo ta ngan gon
	Message     string     // Tin nhan truyen cho luong dieu khien
	Fingerprint string     // Dau an on dinh cua Finding nguon, dung de loc trung lap thoi gian chay
}

// Stats la cac chi so tong quan hien thi song song voi cac phat hien.
type Stats struct {
	CompletedChapters int
	TotalChapters     int
	TotalWords        int
	AvgWordsPerCh     int
	Phase             string
	Flow              string
	PlanningTier      string
	ReviewCount       int
	RewriteCount      int
	AvgReviewScore    float64
	ForeshadowOpen    int
	ForeshadowStale   int
}

// Report la dau ra day du cua mot lan chay chan doan.
type Report struct {
	Stats    Stats
	Findings []Finding
	Actions  []Action
}

package diag

import (
	"fmt"
	"strings"

	"github.com/voocel/ainovel-cli/internal/store"
)

// Nguong phat hien thoi gian chay.
const (
	repeatCritical = 8 // lap phan gan cuoi dat so lan nay thi nang cap len critical
	streamIdleWarn = 3 // nguong canh bao tich luy stream_idle
)

// RuntimeRuleFunc la chu ky thong nhat cua quy tac chan doan thoi gian chay (tuong ung voi RuleFunc phia sang tac).
// Tham so dau vao la RuntimeCapture sau khi da tong hop khu nhan dang, dau ra la Finding kieu bao cao -- tat ca AutoNone,
// chi chan doan, khong tao Action (nguyen tac quan sat, xem architecture.md §2.3).
type RuntimeRuleFunc func(rc *RuntimeCapture) []Finding

var runtimeRules = []RuntimeRuleFunc{
	repeatedErrors,
	stuckStep,
	streamIdleStorm,
}

// runtimeFindings chay tat ca cac quy tac thoi gian chay.
func runtimeFindings(rc *RuntimeCapture) []Finding {
	var out []Finding
	for _, rule := range runtimeRules {
		out = append(out, rule(rc)...)
	}
	return out
}

// Diagnose la diem vao chan doan day du cua /diag: chan doan sang tac + tin hieu thoi gian chay + phat hien thoi gian chay,
// tra ve Report da hop nhat va RuntimeCapture thu so (de tai su dung khi xuat, tranh bat giu trung lap).
// Finding thoi gian chay chi duoc hop nhat vao Findings de hien thi, khong thay doi Actions -- giu quan sat thuan tuy.
func Diagnose(s *store.Store) (Report, RuntimeCapture) {
	rep := Analyze(s)
	rc := CaptureRuntime(s)
	rep.Findings = append(rep.Findings, runtimeFindings(&rc)...)
	sortFindings(rep.Findings)
	return rep, rc
}

// repeatedErrors chi danh gia "loi / tham so khong hop le xuat hien lap lai o phan gan cuoi" thanh Finding.
// Khong can thiep vao lap cong cu binh thuong -- subagent/novel_context/read_chapter trong luong chay dai
// tu nhien co tan so cao, so lan tich luy khong phai la tin hieu vong lap; "lap lai ma khong tien trien" that su duoc stuckStep dam nhan.
func repeatedErrors(rc *RuntimeCapture) []Finding {
	var out []Finding
	for _, r := range rc.Repeats {
		var rule, title, sugg string
		switch {
		case strings.Contains(r.Sig, " · err: "):
			rule = "RepeatedToolError"
			title = "Cong cu lien tuc bao cung mot loi"
			sugg = "Cung mot cong cu o phan gan cuoi lien tuc tra ve cung mot loi, thuong do tham so mo hinh khong hop le hoac hop dong cong cu khong khop; kiem tra xac thuc cong cu agentcore / quy uoc tham so prompt (xem #34)."
		case strings.Contains(r.Sig, "(args invalid)"):
			rule = "ArgsInvalidLoop"
			title = "Tham so lien tuc khong the phan tich"
			sugg = "Tham so tu mo hinh gui den khong the phan tich nhung cu thu lai; xem agentcore co thuc hien ep kieu long leo cho kieu nay khong (xem #34)."
		default:
			continue // Lap cong cu binh thuong khong tao Finding
		}
		sev := SevWarning
		if r.Count >= repeatCritical {
			sev = SevCritical
		}
		out = append(out, Finding{
			Rule:       rule,
			Category:   CatFlow,
			Severity:   sev,
			Confidence: ConfHigh,
			AutoLevel:  AutoNone,
			Target:     "runtime.flow",
			Title:      title,
			Evidence:   fmt.Sprintf("`%s` ×%d", r.Sig, r.Count),
			Suggestion: sugg,
		})
	}
	return out
}

// stuckStep phat hien checkpoint lien tuc dung tai cung mot step.
func stuckStep(rc *RuntimeCapture) []Finding {
	if rc.StuckStep == "" {
		return nil
	}
	sev := SevWarning
	if rc.StuckCount >= repeatCritical {
		sev = SevCritical
	}
	return []Finding{{
		Rule:       "StuckStep",
		Category:   CatFlow,
		Severity:   sev,
		Confidence: ConfHigh,
		AutoLevel:  AutoNone,
		Target:     "runtime.flow",
		Title:      "checkpoint bi ket tai cung mot step",
		Evidence:   fmt.Sprintf("lien tuc dung tai `%s` x%d", rc.StuckStep, rc.StuckCount),
		Suggestion: "Cung mot step bi ghi di ghi lai ma khong tien trien; ket hop voi chu ky lap o tren de xac dinh sub-agent nao bi ket.",
	}}
}

// streamIdleStorm phat hien ngat luong phat truc tuyen xay ra qua thuong (#32).
func streamIdleStorm(rc *RuntimeCapture) []Finding {
	n := rc.LogKinds["stream_idle"]
	if n < streamIdleWarn {
		return nil
	}
	return []Finding{{
		Rule:       "StreamIdleStorm",
		Category:   CatFlow,
		Severity:   SevWarning,
		Confidence: ConfHigh,
		AutoLevel:  AutoNone,
		Target:     "runtime.provider",
		Title:      "Ngat luong phat truc tuyen xay ra qua thuong (stream_idle)",
		Evidence:   fmt.Sprintf("stream_idle x%d", n),
		Suggestion: "Luong tu phia tren lau khong xuat token bi watchdog nham giet; tang streamIdleTimeout cho mo hinh suy nghi cham, hoac kiem tra do on dinh ket noi provider (xem #32).",
	}}
}

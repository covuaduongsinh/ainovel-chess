package diag

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

const (
	logTailCap   = 200 << 10 // chi lay phan duoi 200KB cua log (vong lap la hien tuong gan cuoi)
	sessionTail  = 80        // so dong xuong bo (xem thu tu phan phat)
	repeatWindow = 150       // tong hop lap chi xet nhieu nhat so su kien gan cuoi nay -- trong luong chay dai cong cu binh thuong tich luy hang tram lan,
	// vong lap that su la tap trung cao do o phan gan cuoi; dung cua so thay vi tich luy, tranh nhan dinh nham "tien trien binh thuong" thanh "vong lap chet".
	recentAgents = 2  // so phien sub-agent hoat dong gan day can quet bo sung
	repeatMin    = 3  // lap may lan moi tinh la "tin hieu tan so cao"
	repeatTopN   = 12 // so chu ky lap toi da liet ke
)

// RuntimeCapture la ket qua khu nhan dang cua mot lan bat giu thoi gian chay. Chi mang tin hieu thoi gian chay;
// trang thai sang tac nhu phase/flow/chuong duoc Report.Stats mang, khong lap lai o day.
type RuntimeCapture struct {
	GoOS, GoArch  string
	Models        []RoleModel  // provider/model thuc su co hieu luc trong moi phien (thu thap tu _meta)
	CurrentStep   string       // checkpoint moi nhat: scope.step
	StuckStep     string       // cung step lien tuc o phan duoi; "" = khong bi ket
	StuckCount    int          // so lan lien tiep
	Repeats       []RepeatStat // chu ky lap top-N (tin hieu vong lap)
	DupContent    []DupStat    // cung sha xuat hien nhieu lan (tao di tao lai cung doan)
	LogKinds      map[string]int
	LogErrors     int
	LogWarns      int
	StopGuard     int
	Tail          []SkelEvent // N dong xuong cuoi cung (xem thu tu)
	RedactedTexts int         // tong so khoi van ban da khu nhan dang (tu kiem tra khu nhan dang)
	Sources       []string    // cac nguon doc duoc thuc te (tu kiem tra)
}

// RoleModel ghi lai provider/model thuc su su dung trong mot phien nhat dinh.
type RoleModel struct {
	Agent, Provider, Model string
}

// RepeatStat la mot chu ky lap cung voi so lan cua no.
type RepeatStat struct {
	Sig   string
	Count int
}

// DupStat la so lan cung mot doan van ban da khu nhan dang xuat hien lap lai.
type DupStat struct {
	Sha   string
	Count int
}

// sessionLine phan tich mot dong cua sessions/*.jsonl: nhung agentcore.Message + _meta tuy chon.
type sessionLine struct {
	agentcore.Message
	Meta *struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	} `json:"_meta"`
}

var kindRe = regexp.MustCompile(`kind=(\S+)`)

// CaptureRuntime chi doc bat giu tin hieu thoi gian chay tu thu muc output va tong hop khu nhan dang.
// Bat ky nguon nao bi thieu deu giam cap an toan (khong bao loi), co gang het suc.
func CaptureRuntime(s *store.Store) RuntimeCapture {
	rc := RuntimeCapture{GoOS: runtime.GOOS, GoArch: runtime.GOARCH, LogKinds: map[string]int{}}

	rc.CurrentStep, rc.StuckStep, rc.StuckCount = analyzeCheckpoints(s.Checkpoints.All())
	captureSessions(s.Dir(), &rc)
	captureLog(s.Dir(), &rc)
	return rc
}

// analyzeCheckpoints lay step moi nhat va tinh cung step lien tiep o phan duoi (tin hieu bi ket).
func analyzeCheckpoints(cps []domain.Checkpoint) (current, stuck string, count int) {
	if len(cps) == 0 {
		return "", "", 0
	}
	key := func(c domain.Checkpoint) string { return fmt.Sprintf("%s.%s", c.Scope, c.Step) }
	current = key(cps[len(cps)-1])
	n := 1
	for i := len(cps) - 2; i >= 0; i-- {
		if key(cps[i]) == current {
			n++
		} else {
			break
		}
	}
	if n >= repeatMin {
		stuck, count = current, n
	}
	return current, stuck, count
}

// captureSessions quet coordinator + cac phien sub-agent gan day, tong hop khu nhan dang.
func captureSessions(dir string, rc *RuntimeCapture) {
	sessDir := filepath.Join(dir, "meta", "sessions")
	files := sessionFiles(sessDir)

	repeats := map[string]int{}
	dups := map[string]int{}
	models := map[string]RoleModel{}

	for _, f := range files {
		evs := scanSession(filepath.Join(sessDir, f.path), f.agent, rc, models)
		// Tong hop chi xet cua so gan cuoi: trong luong chay dai subagent/novel_context tich luy hang tram lan la tien trien binh thuong,
		// khong phai vong lap; vong lap chet that su la tap trung cao do o phan gan cuoi.
		aggregateRepeats(f.agent, tailEvents(evs, repeatWindow), repeats, dups)
		// Xuong bo uu tien lay tu coordinator -- vong lap phan phat ro rang nhat o day.
		if f.agent == "coordinator" && len(evs) > 0 {
			rc.Tail = tailEvents(evs, sessionTail)
		}
		rc.Sources = append(rc.Sources, "sessions/"+f.path)
	}
	if len(rc.Tail) == 0 {
		// Khi khong co phien coordinator, lui ve sub-agent gan day nhat.
		for _, f := range files {
			if evs := scanSessionTailOnly(filepath.Join(sessDir, f.path), f.agent); len(evs) > 0 {
				rc.Tail = tailEvents(evs, sessionTail)
				break
			}
		}
	}

	rc.Repeats = topRepeats(repeats)
	rc.DupContent = topDups(dups)
	rc.Models = sortedModels(models)
}

type sessionFile struct {
	path  string // tuong doi voi sessDir
	agent string
}

// sessionFiles tra ve coordinator.jsonl + cac phien sub-agent hoat dong gan day nhat.
func sessionFiles(sessDir string) []sessionFile {
	var out []sessionFile
	if _, err := os.Stat(filepath.Join(sessDir, "coordinator.jsonl")); err == nil {
		out = append(out, sessionFile{path: "coordinator.jsonl", agent: "coordinator"})
	}

	agentsDir := filepath.Join(sessDir, "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return out
	}
	type withTime struct {
		name string
		mod  int64
	}
	var agents []withTime
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		if info, err := e.Info(); err == nil {
			agents = append(agents, withTime{e.Name(), info.ModTime().UnixNano()})
		}
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].mod > agents[j].mod })
	for i, a := range agents {
		if i >= recentAgents {
			break
		}
		stem := strings.TrimSuffix(a.name, ".jsonl")
		out = append(out, sessionFile{path: filepath.Join("agents", a.name), agent: stem})
	}
	return out
}

// scanSession doc mot tep phien, khu nhan dang tung dong, thu thap chuoi su kien va mo hinh per-agent.
// Tong hop lap/cung doan khong thuc hien o day -- giao cho aggregateRepeats tinh tren cua so gan cuoi.
func scanSession(path, agent string, rc *RuntimeCapture, models map[string]RoleModel) []SkelEvent {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var evs []SkelEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for sc.Scan() {
		var sl sessionLine
		if json.Unmarshal(sc.Bytes(), &sl) != nil {
			continue
		}
		ev := redactMessage(agent, sl.Message)
		evs = append(evs, ev)
		rc.RedactedTexts += ev.Redacted
		if sl.Meta != nil && (sl.Meta.Provider != "" || sl.Meta.Model != "") {
			models[agent] = RoleModel{Agent: agent, Provider: sl.Meta.Provider, Model: sl.Meta.Model}
		}
	}
	return evs
}

// aggregateRepeats tich luy chu ky lap va cung doan van ban tren cua so su kien da cho.
func aggregateRepeats(agent string, evs []SkelEvent, repeats, dups map[string]int) {
	for _, ev := range evs {
		for _, t := range ev.Tools {
			sig := agent + " · " + t.Name
			if t.Invalid {
				sig += " (args invalid)"
			}
			repeats[sig]++
		}
		if ev.ErrClass != "" {
			repeats[agent+" · err: "+ev.ErrClass]++
		}
		if ev.TextSha != "" {
			dups[ev.TextSha]++
		}
	}
}

// scanSessionTailOnly chi lay xuong (khong tinh tong hop), dung de du phong khi coordinator vang mat.
func scanSessionTailOnly(path, agent string) []SkelEvent {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var evs []SkelEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for sc.Scan() {
		var sl sessionLine
		if json.Unmarshal(sc.Bytes(), &sl) != nil {
			continue
		}
		evs = append(evs, redactMessage(agent, sl.Message))
	}
	return evs
}

func tailEvents(evs []SkelEvent, n int) []SkelEvent {
	if len(evs) <= n {
		return evs
	}
	return evs[len(evs)-n:]
}

// captureLog doc phan duoi cua log, chi tong hop tin hieu co cau truc (kind/error/warn/stop_guard),
// khong dua dong log thu so vao goi -- Detail co the chua noi dung chinh.
func captureLog(dir string, rc *RuntimeCapture) {
	path := filepath.Join(dir, "logs", "tui.log")
	tail, ok := readTail(path)
	if !ok {
		path = filepath.Join(dir, "logs", "headless.log")
		tail, ok = readTail(path)
	}
	if !ok {
		return
	}
	rc.Sources = append(rc.Sources, "logs/"+filepath.Base(path)+" (phan duoi)")

	sc := bufio.NewScanner(bytes.NewReader(tail))
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.Contains(line, "level=ERROR"):
			rc.LogErrors++
		case strings.Contains(line, "level=WARN"):
			rc.LogWarns++
		}
		if m := kindRe.FindStringSubmatch(line); m != nil {
			rc.LogKinds[m[1]]++
		}
		if strings.Contains(line, "stop_guard") {
			rc.StopGuard++
		}
	}
}

// readTail doc logTailCap byte phan duoi cua tep va bo dong nua co the bi cat dau tien.
func readTail(path string) ([]byte, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, false
	}
	size := info.Size()
	var off int64
	if size > logTailCap {
		off = size - logTailCap
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return nil, false
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, false
	}
	if off > 0 {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	return data, true
}

func topRepeats(m map[string]int) []RepeatStat {
	var out []RepeatStat
	for sig, c := range m {
		if c >= repeatMin {
			out = append(out, RepeatStat{Sig: sig, Count: c})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Sig < out[j].Sig
	})
	if len(out) > repeatTopN {
		out = out[:repeatTopN]
	}
	return out
}

func topDups(m map[string]int) []DupStat {
	var out []DupStat
	for sha, c := range m {
		if c >= repeatMin {
			out = append(out, DupStat{Sha: sha, Count: c})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Sha < out[j].Sha
	})
	return out
}

func sortedModels(m map[string]RoleModel) []RoleModel {
	out := make([]RoleModel, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Agent < out[j].Agent })
	return out
}

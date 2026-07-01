package notify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAllowsFilter(t *testing.T) {
	if New("", nil).allows("repeat") != true {
		t.Error("events mac dinh nen cho qua tat ca")
	}
	n := New("", []string{"run_end", "budget"})
	if !n.allows("run_end") || !n.allows("budget") {
		t.Error("kind da liet ke nen duoc cho qua")
	}
	if n.allows("repeat") {
		t.Error("kind chua liet ke nen bi chan")
	}
	var nilN *Notifier
	if nilN.allows("run_end") {
		t.Error("nil Notifier nen chan tat ca")
	}
	nilN.Send(Notification{Kind: "run_end"}) // khong duoc panic
}

func TestCommandChannelEnvAndStdin(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "env.txt")
	jsonFile := filepath.Join(dir, "stdin.json")

	n := New(`echo "$NOTIFY_KIND|$NOTIFY_LEVEL|$NOTIFY_TITLE|$NOTIFY_BODY" > `+envFile+` && cat > `+jsonFile, nil)
	nt := Notification{Kind: "budget", Level: "warn", Title: "ainovel: ngan sach", Body: "Da chi $8.00"}
	n.deliver(nt) // goi dong bo de xac nhan

	env, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("command chua duoc thuc thi: %v", err)
	}
	if got := strings.TrimSpace(string(env)); got != "budget|warn|ainovel: ngan sach|Da chi $8.00" {
		t.Errorf("truyen bien moi truong khong khop: %q", got)
	}

	raw, err := os.ReadFile(jsonFile)
	if err != nil {
		t.Fatalf("stdin chua duoc truyen: %v", err)
	}
	var decoded Notification
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("stdin khong phai JSON hop le: %v", err)
	}
	if decoded != nt {
		t.Errorf("stdin JSON khong khop: %+v", decoded)
	}
}

func TestCommandChannelTimeoutKill(t *testing.T) {
	n := New("sleep 30", nil)
	n.timeout = 200 * time.Millisecond

	start := time.Now()
	n.deliver(Notification{Kind: "run_end"})
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("het thoi gian chua giet cung buoc, bi chan %v", elapsed)
	}
}

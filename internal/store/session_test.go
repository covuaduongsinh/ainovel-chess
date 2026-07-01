package store

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/voocel/agentcore"
)

// TestSessionStore_MetaInjected_AssistantWithUsage xac nhan chi co tin nhan "assistant + co Usage"
// moi duoc gan them _meta, day la dieu kien tien quyet de tinh gia chinh xac tren duong dan replay.
func TestSessionStore_MetaInjected_AssistantWithUsage(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStore(newIO(dir))
	lookup := ModelLookup(func(agentName string) (string, string) {
		return "meme", "gpt-5.4"
	})
	logger := s.SubAgentLogger(lookup)

	logger("writer", "viet chuong 1", agentcore.Message{
		Role:  agentcore.RoleUser,
		Usage: nil,
	})
	logger("writer", "viet chuong 1", agentcore.Message{
		Role: agentcore.RoleAssistant,
		Usage: &agentcore.Usage{
			Input: 1000, Output: 200, CacheRead: 800, TotalTokens: 1200,
		},
	})
	logger("writer", "viet chuong 1", agentcore.Message{
		Role:  agentcore.RoleAssistant,
		Usage: nil, // assistant nhung khong co usage (luong phat truc tuyen chua mang final usage chunk)
	})

	entries := readJSONL(t, filepath.Join(dir, "meta/sessions/agents/writer-ch01.jsonl"))
	if len(entries) != 3 {
		t.Fatalf("entries=%d want 3", len(entries))
	}
	if _, has := entries[0]["_meta"]; has {
		t.Errorf("user message should NOT have _meta")
	}
	if _, has := entries[2]["_meta"]; has {
		t.Errorf("assistant without Usage should NOT have _meta")
	}
	meta, ok := entries[1]["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("assistant+Usage should have _meta map, got %T %v", entries[1]["_meta"], entries[1]["_meta"])
	}
	if meta["provider"] != "meme" || meta["model"] != "gpt-5.4" {
		t.Errorf("_meta = %v want provider=meme model=gpt-5.4", meta)
	}
}

// TestSessionStore_MetaModelSwitch xac nhan sau khi chuyen doi mo hinh trong qua trinh chay,
// _meta cua cac tin nhan tiep theo cung thay doi theo. Day la ho tro chinh xac cua phuong an B
// cho viec "chuyen doi /model trong cung tien trinh".
func TestSessionStore_MetaModelSwitch(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStore(newIO(dir))

	current := "model-a"
	lookup := ModelLookup(func(agentName string) (string, string) {
		return "meme", current
	})
	logger := s.SubAgentLogger(lookup)

	logger("writer", "viet chuong 1", makeAssistantWithUsage())
	current = "model-b" // mo phong chuyen doi /model
	logger("writer", "viet chuong 1", makeAssistantWithUsage())

	entries := readJSONL(t, filepath.Join(dir, "meta/sessions/agents/writer-ch01.jsonl"))
	if len(entries) != 2 {
		t.Fatalf("entries=%d want 2", len(entries))
	}
	for i, want := range []string{"model-a", "model-b"} {
		meta, ok := entries[i]["_meta"].(map[string]any)
		if !ok {
			t.Fatalf("entry[%d] missing _meta", i)
		}
		if got := meta["model"]; got != want {
			t.Errorf("entry[%d] model = %v want %s", i, got, want)
		}
	}
}

// TestSessionStore_NilLookup xac nhan khi lookup=nil (nhu duong dan cocreate) viec ghi van binh thuong,
// chi la khong co _meta, giu tuong thich nguoc.
func TestSessionStore_NilLookup(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStore(newIO(dir))
	logger := s.CoordinatorLogger(nil)
	logger(makeAssistantWithUsage())

	entries := readJSONL(t, filepath.Join(dir, "meta/sessions/coordinator.jsonl"))
	if len(entries) != 1 {
		t.Fatalf("entries=%d want 1", len(entries))
	}
	if _, has := entries[0]["_meta"]; has {
		t.Errorf("nil lookup should not produce _meta")
	}
	// Nhung cac truong khac (role/usage) phai binh thuong
	if entries[0]["role"] != "assistant" {
		t.Errorf("role lost: %v", entries[0]["role"])
	}
}

func makeAssistantWithUsage() agentcore.Message {
	return agentcore.Message{
		Role:  agentcore.RoleAssistant,
		Usage: &agentcore.Usage{Input: 1000, Output: 200, TotalTokens: 1200},
	}
}

func readJSONL(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	var out []map[string]any
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatalf("unmarshal line: %v\n%s", err, string(line))
		}
		out = append(out, m)
	}
	return out
}

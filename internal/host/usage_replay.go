package host

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/voocel/agentcore"
)

// sessionRecord la dang phan tich nhe cua ban ghi don trong meta/sessions/*.jsonl -- chi lay
// cac truong can thiet cho tich luy usage. Truong lon nhu Content bo qua phan tich, tiet kiem IO luc khoi dong.
//
// Ba cap giangiam mo hinh:
//  1. Usage.Provider/Model -- mo hinh phan hoi thuc tu agentcore/litellm (uu tien)
//  2. Meta(_meta)          -- khi upstream khong truyen, phia ghi duoc ModelLookup bo sung mo hinh "hien tai hieu luc"
//  3. Ca hai deu khong co  -- replay lui ve effectiveModel dung ModelSet hien tai suy nguoc (do chinh xac giam)
type sessionRecord struct {
	Role  agentcore.Role     `json:"role"`
	Usage *agentcore.Usage   `json:"usage,omitempty"`
	Meta  *sessionRecordMeta `json:"_meta,omitempty"`
}

type sessionRecordMeta struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// ReplaySessions quet meta/sessions/coordinator.jsonl va meta/sessions/agents/*.jsonl,
// tich luy lai usage cua moi tin nhan assistant vao tracker. Tra ve so ban ghi da dien lai.
//
// Rang buoc goi: chi goi mot lan khi meta/usage.json vang (lan dau nang cap hoac thay doi schema),
// de dien lai du lieu lich su. Luu tru hang ngay dung SaveNow / autoSaveLoop.
//
// Do chinh xac phu thuoc ba cap giam cap trong sessionRecord -- cap 3 (Usage va _meta deu vang)
// chi kich hoat voi log cu hon hoac upstream bat thuong.
func (t *UsageTracker) ReplaySessions(rootDir string) (int, error) {
	if t == nil {
		return 0, nil
	}
	sessionsDir := filepath.Join(rootDir, "meta", "sessions")
	info, err := os.Stat(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if !info.IsDir() {
		return 0, nil
	}

	total := 0
	if n, err := t.replayFile(filepath.Join(sessionsDir, "coordinator.jsonl"), "coordinator"); err != nil {
		slog.Warn("replay coordinator session failed", "module", "usage", "err", err)
	} else {
		total += n
	}

	agentsDir := filepath.Join(sessionsDir, "agents")
	walkErr := filepath.WalkDir(agentsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			return nil
		}
		agentName := parseAgentNameFromFile(name)
		if agentName == "" {
			return nil
		}
		n, fileErr := t.replayFile(path, agentName)
		if fileErr != nil {
			slog.Warn("replay agent session failed", "module", "usage", "file", name, "err", fileErr)
			return nil
		}
		total += n
		return nil
	})
	if walkErr != nil && !os.IsNotExist(walkErr) {
		return total, walkErr
	}
	return total, nil
}

// replayFile quet mot file jsonl, nap tat ca tin nhan assistant co Usage vao accumulate.
// agentName duoc caller truyen vao (coordinator hoac ten sub-agent phan tich tu ten file).
func (t *UsageTracker) replayFile(path, agentName string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()

	role := agentRoleName(agentName)
	count := 0
	scanner := bufio.NewScanner(f)
	// Mot dong co the rat dai (tin nhan assistant + tool args deu phang hoa), mo rong len 4MB.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec sessionRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec.Role != agentcore.RoleAssistant || rec.Usage == nil {
			continue
		}
		provider, modelName := usageActualModel(rec.Usage)
		if rec.Meta != nil {
			if provider == "" {
				provider = rec.Meta.Provider
			}
			if modelName == "" {
				modelName = rec.Meta.Model
			}
		}
		t.accumulate(role, provider, modelName, *rec.Usage)
		count++
	}
	if err := scanner.Err(); err != nil {
		return count, fmt.Errorf("scan %s: %w", path, err)
	}
	return count, nil
}

// parseAgentNameFromFile trich xuat ten agent tu "writer-ch01.jsonl" / "architect_short-001.jsonl"
// (phan truoc "-"). Quy uoc dat ten xem store/session.go::subAgentPath:
// agentName khong chua dash, suffix la ch<n> hoac so thu tu tang dan.
func parseAgentNameFromFile(name string) string {
	base := strings.TrimSuffix(name, ".jsonl")
	if i := strings.Index(base, "-"); i > 0 {
		return base[:i]
	}
	return ""
}

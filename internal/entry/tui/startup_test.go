package tui

import (
	"strings"
	"testing"
)

func TestEnterStartingSwitchesToWorkbenchImmediately(t *testing.T) {
	m := NewModel(nil, nil, "")
	m.width = 120
	m.height = 40
	m.resizeTextarea()
	m.updateViewportSize()

	m.enterStarting("Viết một bộ tiểu thuyết huyền huyễn phương Đông dài tập")

	if m.mode != modeRunning {
		t.Fatalf("mode = %v, want modeRunning", m.mode)
	}
	if !m.starting {
		t.Fatal("starting should be true while host startup command is running")
	}
	if !m.snapshot.IsRunning {
		t.Fatal("snapshot should render as running during local startup")
	}
	if got := m.textarea.Placeholder; got != "Đang khởi tạo sáng tác..." {
		t.Fatalf("placeholder = %q", got)
	}
	if len(m.events) != 2 {
		t.Fatalf("events = %+v, want startup user + system events", m.events)
	}
	if m.events[0].Category != "USER" || !strings.HasPrefix(m.events[0].Summary, "Yêu cầu sáng tác: ") {
		t.Fatalf("first event = %+v, want USER prompt event", m.events[0])
	}
}

func TestApplyStartupPromptEventTruncatesSummaryButKeepsDetail(t *testing.T) {
	m := NewModel(nil, nil, "")
	prompt := strings.Repeat("s", maxPromptEventRunes+50)

	m.applyStartupPromptEvent(prompt)

	if len(m.events) != 1 {
		t.Fatalf("events = %+v, want one event", m.events)
	}
	ev := m.events[0]
	if ev.Detail != prompt {
		t.Fatalf("detail should keep full prompt, got len=%d want=%d", len([]rune(ev.Detail)), len([]rune(prompt)))
	}
	maxSummaryRunes := len([]rune("Yêu cầu sáng tác: ")) + maxPromptEventRunes
	if got := len([]rune(ev.Summary)); got > maxSummaryRunes {
		t.Fatalf("summary runes = %d, want <= %d", got, maxSummaryRunes)
	}
	if !strings.HasSuffix(ev.Summary, "...") {
		t.Fatalf("summary should be truncated with ellipsis, got %q", ev.Summary)
	}
}

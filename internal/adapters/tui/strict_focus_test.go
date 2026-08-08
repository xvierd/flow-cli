package tui

// Strict focus mode key-flow tests: when strict is enabled, early disengagement
// from an active work session (pause, finish, void, break, mode-switch cancel)
// is locked in both Model and InlineModel, and a STRICT badge is rendered.

import (
	"strings"
	"testing"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

func breakSession() *domain.CurrentState {
	return &domain.CurrentState{ActiveSession: domain.NewBreakSession(domain.DefaultPomodoroConfig(), 1)}
}

func TestModel_Strict_LocksDisengagement(t *testing.T) {
	cb, cmds := commandTracker()
	m := NewModel(stateWithSession(), nil, nil)
	m.strict = true
	m.commandCallback = cb

	for _, k := range []string{"p", "f", "v", "b"} {
		result, _ := m.Update(key(k))
		updated := result.(Model)
		if !updated.strictLocked {
			t.Errorf("[%s] should set strictLocked under strict focus", k)
		}
		if len(*cmds) != 0 {
			t.Fatalf("[%s] should not dispatch any command, got %v", k, *cmds)
		}
	}
}

func TestModel_Strict_BadgeRendered(t *testing.T) {
	m := NewModel(stateWithSession(), nil, nil)
	m.strict = true
	m.width = 80
	m.height = 24
	view := m.View()
	if !strings.Contains(view, "🔒 STRICT") {
		t.Error("View should render the STRICT badge when strict + work session active")
	}
}

func TestModel_NoStrict_NoBadge(t *testing.T) {
	m := NewModel(stateWithSession(), nil, nil)
	m.strict = false
	m.width = 80
	m.height = 24
	if strings.Contains(m.View(), "🔒 STRICT") {
		t.Error("View should not render the STRICT badge when strict is off")
	}
}

func TestModel_Strict_DoesNotBlockBreakSession(t *testing.T) {
	cb, cmds := commandTracker()
	m := NewModel(breakSession(), nil, nil)
	m.strict = true
	m.commandCallback = cb

	result, _ := m.Update(key("p"))
	updated := result.(Model)
	if updated.strictLocked {
		t.Error("break sessions should not be locked in strict mode")
	}
	if len(*cmds) == 0 || (*cmds)[0] != ports.CmdPause {
		t.Errorf("expected CmdPause on break, got %v", *cmds)
	}
}

func TestModel_Strict_AllowsResumeWhenPaused(t *testing.T) {
	s := activeSession()
	s.Pause()
	cb, cmds := commandTracker()
	m := NewModel(&domain.CurrentState{ActiveSession: s}, nil, nil)
	m.strict = true
	m.commandCallback = cb

	result, _ := m.Update(key("p"))
	updated := result.(Model)
	if updated.strictLocked {
		t.Error("resuming an already-paused work session should not be locked")
	}
	if len(*cmds) == 0 || (*cmds)[0] != ports.CmdResume {
		t.Errorf("expected CmdResume for paused session, got %v", *cmds)
	}
}

func TestInlineModel_Strict_LocksDisengagement(t *testing.T) {
	cb, cmds := commandTracker()
	m := baseInlineModel()
	m.strict = true
	m.commandCallback = cb

	for _, k := range []string{"p", "f", "v", "b", "m"} {
		result, _ := m.Update(key(k))
		updated := result.(InlineModel)
		if !updated.strictLocked {
			t.Errorf("[%s] should set strictLocked under strict focus", k)
		}
		if len(*cmds) != 0 {
			t.Fatalf("[%s] should not dispatch any command, got %v", k, *cmds)
		}
	}
}

func TestInlineModel_Strict_BadgeRendered(t *testing.T) {
	m := baseInlineModel()
	m.strict = true
	view := m.View()
	if !strings.Contains(view, "🔒 STRICT") {
		t.Error("InlineModel view should render the STRICT badge when strict + work session active")
	}
}

func TestInlineModel_NoStrict_NoBadge(t *testing.T) {
	m := baseInlineModel()
	m.strict = false
	if strings.Contains(m.View(), "🔒 STRICT") {
		t.Error("InlineModel view should not render the STRICT badge when strict is off")
	}
}

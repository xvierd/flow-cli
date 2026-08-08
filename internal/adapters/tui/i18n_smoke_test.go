package tui

import (
	"strings"
	"testing"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
)

// TestSpanishSmoke renders views with the Spanish catalog active and asserts
// known translations appear. The default (English) rendering is covered by
// the rest of the package tests.
func TestSpanishSmoke(t *testing.T) {
	i18n.SetLanguage("es")
	defer i18n.SetLanguage("en")

	state := &domain.CurrentState{}
	state.TodayStats.WorkSessions = 3
	if got := viewIdlePomodoro(state); !strings.Contains(got, "3 sesiones hoy") {
		t.Errorf("viewIdlePomodoro in es should contain '3 sesiones hoy', got %q", got)
	}

	state.TodayStats.WorkSessions = 1
	if got := viewIdlePomodoro(state); !strings.Contains(got, "1 sesión hoy") {
		t.Errorf("viewIdlePomodoro singular in es should contain '1 sesión hoy', got %q", got)
	}

	if got := i18n.T("[n]ew session  [b]reak  [q]uit"); !strings.Contains(got, "nueva sesión") {
		t.Errorf("hotkey help line should be translated, got %q", got)
	}

	// Domain labels are translated inside domain (leaf i18n import).
	if got := domain.GetSessionTypeLabel(domain.SessionTypeWork); got != "Trabajo" {
		t.Errorf("GetSessionTypeLabel(Work) in es = %q, want %q", got, "Trabajo")
	}

	// The strict lock message keeps the fullscreen/inline distinction (inline adds mode).
	if got := strictLockMessage(false); !strings.Contains(got, "bloqueados") {
		t.Errorf("strict lock message should be translated, got %q", got)
	}
	if strictLockMessage(true) == strictLockMessage(false) {
		t.Error("inline strict lock message should differ from the fullscreen one (it also locks mode)")
	}
}

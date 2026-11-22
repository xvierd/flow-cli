package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/xvierd/flow-cli/internal/domain"
)

func TestCompletionViews_ShowTopInsight(t *testing.T) {
	const msg = "Protect 09–12h for your hardest work."
	info := &domain.CompletionInfo{NextBreakType: domain.SessionTypeShortBreak, TopInsight: msg}
	state := &domain.CurrentState{TodayStats: domain.DailyStats{WorkSessions: 2}}

	m := NewModel(state, info, nil)
	im := NewInlineModel(state, info, nil)
	plain := lipgloss.NewStyle()
	views := map[string]string{
		"default":         strings.Join(m.viewDefaultWorkComplete(nil), "\n"),
		"deepwork":        strings.Join(m.viewDeepWorkComplete(nil), "\n"),
		"maketime":        strings.Join(m.viewMakeTimeComplete(nil), "\n"),
		"inline default":  im.viewInlineDefaultComplete(plain, plain),
		"inline deepwork": im.viewInlineDeepWorkComplete(plain, plain),
		"inline maketime": im.viewInlineMakeTimeComplete(plain, plain),
	}
	for name, v := range views {
		if strings.Count(v, "💡 "+msg) != 1 {
			t.Errorf("%s view should show the insight once:\n%s", name, v)
		}
	}

	info.TopInsight = ""
	m = NewModel(state, info, nil)
	if v := strings.Join(m.viewDefaultWorkComplete(nil), "\n"); strings.Contains(v, "💡") {
		t.Errorf("no insight: line should be hidden:\n%s", v)
	}
}

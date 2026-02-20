package tui

import (
	"fmt"
	"strings"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/methodology"
)

// viewIdleMethodologyInfo returns 2-4 lines of methodology-specific content
// to display when no session is active.
func viewIdleMethodologyInfo(state *domain.CurrentState, mode methodology.Mode, completionInfo *domain.CompletionInfo) string {
	if mode == nil {
		return ""
	}

	switch mode.Name() {
	case domain.MethodologyPomodoro:
		return viewIdlePomodoro(state)
	case domain.MethodologyMakeTime:
		return viewIdleMakeTime(state)
	case domain.MethodologyDeepWork:
		return viewIdleDeepWork(state, mode, completionInfo)
	default:
		return ""
	}
}

func viewIdlePomodoro(state *domain.CurrentState) string {
	sessions := state.TodayStats.WorkSessions
	label := "sessions"
	if sessions == 1 {
		label = "session"
	}
	return fmt.Sprintf("  🍅 %d %s today", sessions, label)
}

func viewIdleMakeTime(state *domain.CurrentState) string {
	if state.ActiveTask != nil && state.ActiveTask.IsTodayHighlight() {
		return fmt.Sprintf("  ★ Highlight: \"%s\"", state.ActiveTask.Title)
	}
	return "  No Highlight set for today"
}

func viewIdleDeepWork(state *domain.CurrentState, mode methodology.Mode, completionInfo *domain.CompletionInfo) string {
	goalHours := mode.DeepWorkGoalHours()
	if goalHours <= 0 {
		goalHours = 4.0
	}
	currentHours := state.TodayStats.TotalWorkTime.Hours()
	pct := currentHours / goalHours
	if pct > 1.0 {
		pct = 1.0
	}

	// Build ASCII progress bar
	barWidth := 20
	filled := int(pct * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	empty := barWidth - filled
	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)

	var b strings.Builder
	fmt.Fprintf(&b, "  %s  %.1fh / %.1fh  (%d%%)", bar, currentHours, goalHours, int(pct*100))

	// Streak and philosophy
	streak := 0
	if completionInfo != nil {
		streak = completionInfo.DeepWorkStreak
	}
	philosophy := mode.DeepWorkPhilosophy()

	var parts []string
	if streak > 0 {
		parts = append(parts, fmt.Sprintf("Streak: %d days", streak))
	}
	if philosophy != "" {
		// Capitalize first letter
		parts = append(parts, strings.ToUpper(philosophy[:1])+philosophy[1:])
	}
	if len(parts) > 0 {
		b.WriteString("\n")
		b.WriteString("  " + strings.Join(parts, " · "))
	}

	return b.String()
}

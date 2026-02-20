package tui

import (
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/methodology"
)

// shutdownStepLabels are the prompt labels for the 4-step Deep Work shutdown ritual (Cal Newport).
var shutdownStepLabels = [4]string{
	"Review pending tasks — anything urgent?",
	"Review tomorrow's calendar — any conflicts?",
	"Plan for tomorrow:",
	"Closing phrase (e.g. 'Shutdown complete'):",
}

// completionViewData holds pre-computed values used when rendering completion screens.
// It avoids repeating the same derivations in both Model and InlineModel view functions.
type completionViewData struct {
	// Session
	elapsed         time.Duration
	intendedOutcome string

	// Distractions
	distractionCount int

	// Deep Work stats
	deepWorkHours     float64
	deepWorkGoalHours float64
	deepWorkPct       float64
	deepWorkStreak    int

	// Next break
	hasBreakInfo bool
	breakLabel   string
	breakDur     string
	isLongBreak  bool

	// Break countdown for sessions_before_long display
	sessionsBeforeLong int
	sessionsUntilLong  int

	// Daily stats
	statsWorkSessions  int
	statsBreaksTaken   int
	statsTotalWorkTime time.Duration

	// Make Time
	hasHighlightTask bool
}

// buildCompletionViewData derives all values needed for completion screens from
// the live model state, so neither Model nor InlineModel has to repeat these
// computations in their own view functions.
func buildCompletionViewData(
	cs *completionState,
	mode methodology.Mode,
	state *domain.CurrentState,
	completionInfo *domain.CompletionInfo,
	completedElapsed time.Duration,
) completionViewData {
	stats := state.TodayStats

	goalHours := 4.0
	if mode != nil && mode.DeepWorkGoalHours() > 0 {
		goalHours = mode.DeepWorkGoalHours()
	}
	deepWorkHours := stats.TotalWorkTime.Hours()

	var breakLabel, breakDur string
	var isLongBreak bool
	var sessionsBeforeLong, sessionsUntilLong int
	if completionInfo != nil {
		breakLabel = domain.GetSessionTypeLabel(completionInfo.NextBreakType)
		breakDur = formatDuration(completionInfo.NextBreakDuration)
		isLongBreak = completionInfo.NextBreakType == domain.SessionTypeLongBreak
		sessionsBeforeLong = completionInfo.SessionsBeforeLong
		sessionsUntilLong = completionInfo.SessionsUntilLong
	}

	streak := 0
	if completionInfo != nil {
		streak = completionInfo.DeepWorkStreak
	}

	hasHighlight := state.ActiveTask != nil && state.ActiveTask.HighlightDate != nil

	return completionViewData{
		elapsed:            completedElapsed,
		intendedOutcome:    cs.completedIntendedOutcome,
		distractionCount:   len(cs.distractions),
		deepWorkHours:      deepWorkHours,
		deepWorkGoalHours:  goalHours,
		deepWorkPct:        deepWorkHours / goalHours * 100,
		deepWorkStreak:     streak,
		hasBreakInfo:       completionInfo != nil,
		breakLabel:         breakLabel,
		breakDur:           breakDur,
		isLongBreak:        isLongBreak,
		sessionsBeforeLong: sessionsBeforeLong,
		sessionsUntilLong:  sessionsUntilLong,
		statsWorkSessions:  stats.WorkSessions,
		statsBreaksTaken:   stats.BreaksTaken,
		statsTotalWorkTime: stats.TotalWorkTime,
		hasHighlightTask:   hasHighlight,
	}
}

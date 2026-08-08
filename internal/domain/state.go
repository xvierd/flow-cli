package domain

import (
	"time"

	"github.com/xvierd/flow-cli/internal/i18n"
)

// CurrentState represents the global application state.
type CurrentState struct {
	ActiveTask    *Task
	ActiveSession *PomodoroSession
	TodayStats    DailyStats
}

// DailyStats aggregates pomodoro statistics for a day.
type DailyStats struct {
	Date           time.Time
	WorkSessions   int
	BreaksTaken    int
	TotalWorkTime  time.Duration
	TasksCompleted int
}

// CompletionInfo holds pre-computed context about what break comes next
// after a work session completes, or signals that a break just ended.
type CompletionInfo struct {
	NextBreakType      SessionType
	NextBreakDuration  time.Duration
	SessionsUntilLong  int
	SessionsBeforeLong int
	DeepWorkStreak     int
}

// MethodologyBreakdown holds session counts and total time per methodology.
type MethodologyBreakdown struct {
	Methodology  Methodology
	SessionCount int
	TotalTime    time.Duration
}

// PeriodStats holds aggregated statistics for a time period (week or month).
type PeriodStats struct {
	Label            string
	Start            time.Time
	End              time.Time
	TotalSessions    int
	TotalWorkTime    time.Duration
	ByMethodology    []MethodologyBreakdown
	AvgFocusScore    float64
	FocusScoreCount  int
	DistractionCount int
}

// EnergizeStat holds aggregated focus score data for a specific energize activity.
type EnergizeStat struct {
	Activity      string
	SessionCount  int
	AvgFocusScore float64
}

// StateSnapshot captures the complete system state at a point in time.
type StateSnapshot struct {
	Timestamp      time.Time
	CurrentState   CurrentState
	PendingTasks   []*Task
	RecentSessions []*PomodoroSession
}

// FocusReport summarizes a single day of sessions for AI consumers and the MCP get_focus_report tool.
type FocusReport struct {
	Date             string
	WorkSessions     int
	TotalWorkTime    time.Duration
	AvgFocusScore    float64
	FocusScoreCount  int
	DistractionCount int
	DeepWorkStreak   int
	HighlightID      *string
	HighlightTitle   string
}

// IsSessionActive returns true if there's a running or paused session.
func (cs *CurrentState) IsSessionActive() bool {
	return cs.ActiveSession != nil &&
		(cs.ActiveSession.Status == SessionStatusRunning ||
			cs.ActiveSession.Status == SessionStatusPaused)
}

// CanStartSession returns true if a new session can be started.
func (cs *CurrentState) CanStartSession() bool {
	return cs.ActiveSession == nil ||
		cs.ActiveSession.Status == SessionStatusCompleted ||
		cs.ActiveSession.Status == SessionStatusCancelled
}

// GetSessionTypeLabel returns a human-readable label for the session type.
func GetSessionTypeLabel(t SessionType) string {
	switch t {
	case SessionTypeWork:
		return i18n.T("Work")
	case SessionTypeShortBreak:
		return i18n.T("Short Break")
	case SessionTypeLongBreak:
		return i18n.T("Long Break")
	default:
		return i18n.T("Unknown")
	}
}

// GetStatusLabel returns a human-readable label for the session status.
func GetStatusLabel(s SessionStatus) string {
	switch s {
	case SessionStatusRunning:
		return i18n.T("Running")
	case SessionStatusPaused:
		return i18n.T("Paused")
	case SessionStatusCompleted:
		return i18n.T("Completed")
	case SessionStatusCancelled:
		return i18n.T("Cancelled")
	case SessionStatusInterrupted:
		return i18n.T("Interrupted")
	default:
		return i18n.T("Unknown")
	}
}

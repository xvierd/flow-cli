package domain

import (
	"time"
)

// CurrentState represents the global application state.
type CurrentState struct {
	ActiveTask    *Task
	ActiveSession *PomodoroSession
	TodayStats    DailyStats
}

// DailyStats aggregates pomodoro statistics for a day.
type DailyStats struct {
	Date              time.Time
	WorkSessions      int
	BreaksTaken       int
	TotalWorkTime     time.Duration
	TasksCompleted    int
}

// StateSnapshot captures the complete system state at a point in time.
type StateSnapshot struct {
	Timestamp     time.Time
	CurrentState  CurrentState
	PendingTasks  []*Task
	RecentSessions []*PomodoroSession
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
		return "Work"
	case SessionTypeShortBreak:
		return "Short Break"
	case SessionTypeLongBreak:
		return "Long Break"
	default:
		return "Unknown"
	}
}

// GetStatusLabel returns a human-readable label for the session status.
func GetStatusLabel(s SessionStatus) string {
	switch s {
	case SessionStatusRunning:
		return "Running"
	case SessionStatusPaused:
		return "Paused"
	case SessionStatusCompleted:
		return "Completed"
	case SessionStatusCancelled:
		return "Cancelled"
	default:
		return "Unknown"
	}
}

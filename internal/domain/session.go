package domain

import (
	"strings"
	"time"
)

// SessionType represents the type of work session.
type SessionType string

const (
	SessionTypeWork       SessionType = "work"
	SessionTypeShortBreak SessionType = "short_break"
	SessionTypeLongBreak  SessionType = "long_break"
)

// SessionStatus represents the current state of a session.
type SessionStatus string

const (
	SessionStatusRunning   SessionStatus = "running"
	SessionStatusPaused    SessionStatus = "paused"
	SessionStatusCompleted SessionStatus = "completed"
	SessionStatusCancelled SessionStatus = "cancelled"
)

// ShutdownRitual captures the structured end-of-session reflection for Deep Work mode.
// Cal Newport's 4-step shutdown ritual:
// 1. Review pending tasks
// 2. Review tomorrow's calendar
// 3. Plan for tomorrow
// 4. Closing phrase
type ShutdownRitual struct {
	PendingTasksReview string
	CalendarReview     string
	TomorrowPlan       string
	ClosingPhrase      string
}

// Distraction represents a logged distraction during a session.
type Distraction struct {
	Text     string
	Category string // "internal", "external", or ""
}

// PomodoroSession represents a single work or break interval.
type PomodoroSession struct {
	ID               string
	TaskID           *string
	Type             SessionType
	Status           SessionStatus
	Duration         time.Duration
	StartedAt        time.Time
	PausedAt         *time.Time
	CompletedAt      *time.Time
	GitBranch        string
	GitCommit        string
	GitModified      []string
	Notes            string
	Methodology      Methodology
	FocusScore       *int
	Distractions     []Distraction
	ShutdownRitual   *ShutdownRitual
	Accomplishment   string
	IntendedOutcome  string
	Tags             []string
	EnergizeActivity string
	OutcomeAchieved  string // y/p/n for Deep Work outcome review
}

// PomodoroConfig holds configuration for pomodoro sessions.
type PomodoroConfig struct {
	WorkDuration       time.Duration
	ShortBreakDuration time.Duration
	LongBreakDuration  time.Duration
	SessionsBeforeLong int
}

// DefaultPomodoroConfig returns the standard pomodoro configuration.
func DefaultPomodoroConfig() PomodoroConfig {
	return PomodoroConfig{
		WorkDuration:       25 * time.Minute,
		ShortBreakDuration: 5 * time.Minute,
		LongBreakDuration:  15 * time.Minute,
		SessionsBeforeLong: 4,
	}
}

// NewPomodoroSession creates a new work session.
func NewPomodoroSession(config PomodoroConfig, taskID *string) *PomodoroSession {
	return &PomodoroSession{
		ID:        generateID(),
		TaskID:    taskID,
		Type:      SessionTypeWork,
		Status:    SessionStatusRunning,
		Duration:  config.WorkDuration,
		StartedAt: time.Now(),
	}
}

// NewBreakSession creates a new break session.
func NewBreakSession(config PomodoroConfig, sessionCount int) *PomodoroSession {
	duration := config.ShortBreakDuration
	sessionType := SessionTypeShortBreak

	if sessionCount%config.SessionsBeforeLong == 0 {
		duration = config.LongBreakDuration
		sessionType = SessionTypeLongBreak
	}

	return &PomodoroSession{
		ID:        generateID(),
		Type:      sessionType,
		Status:    SessionStatusRunning,
		Duration:  duration,
		StartedAt: time.Now(),
	}
}

// Pause marks the session as paused.
func (s *PomodoroSession) Pause() {
	if s.Status != SessionStatusRunning {
		return
	}
	now := time.Now()
	s.PausedAt = &now
	s.Status = SessionStatusPaused
}

// Resume continues a paused session.
func (s *PomodoroSession) Resume() {
	if s.Status != SessionStatusPaused || s.PausedAt == nil {
		return
	}

	pausedDuration := time.Since(*s.PausedAt)
	s.StartedAt = s.StartedAt.Add(pausedDuration)
	s.PausedAt = nil
	s.Status = SessionStatusRunning
}

// Complete marks the session as finished.
func (s *PomodoroSession) Complete() {
	now := time.Now()
	s.CompletedAt = &now
	s.Status = SessionStatusCompleted
}

// Cancel aborts the session.
func (s *PomodoroSession) Cancel() {
	s.Status = SessionStatusCancelled
}

// RemainingTime returns how much time is left in the session.
func (s *PomodoroSession) RemainingTime() time.Duration {
	if s.Status == SessionStatusPaused {
		if s.PausedAt == nil {
			return s.Duration
		}
		elapsed := s.PausedAt.Sub(s.StartedAt)
		remaining := s.Duration - elapsed
		if remaining < 0 {
			return 0
		}
		return remaining
	}

	if s.Status != SessionStatusRunning {
		return 0
	}

	elapsed := time.Since(s.StartedAt)
	remaining := s.Duration - elapsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ElapsedTime returns how much time has passed since session started.
func (s *PomodoroSession) ElapsedTime() time.Duration {
	if s.Status == SessionStatusPaused && s.PausedAt != nil {
		return s.PausedAt.Sub(s.StartedAt)
	}
	return time.Since(s.StartedAt)
}

// Progress returns the completion percentage (0.0 to 1.0).
func (s *PomodoroSession) Progress() float64 {
	if s.Duration == 0 {
		return 0
	}

	elapsed := s.ElapsedTime()
	progress := float64(elapsed) / float64(s.Duration)
	if progress > 1 {
		return 1
	}
	return progress
}

// IsWorkSession returns true if this is a work session.
func (s *PomodoroSession) IsWorkSession() bool {
	return s.Type == SessionTypeWork
}

// IsBreakSession returns true if this is a break session.
func (s *PomodoroSession) IsBreakSession() bool {
	return s.Type == SessionTypeShortBreak || s.Type == SessionTypeLongBreak
}

// SetGitContext stores git information for the session.
func (s *PomodoroSession) SetGitContext(branch, commit string, modified []string) {
	s.GitBranch = branch
	s.GitCommit = commit
	s.GitModified = modified
}

// AddNotes adds notes to the session.
func (s *PomodoroSession) AddNotes(notes string) {
	s.Notes = notes
}

// ParseTagsFromInput extracts #tags from input text.
// Returns the cleaned text (without tags) and the list of tags.
// For example: "Build API #coding #backend" returns ("Build API", ["coding", "backend"]).
func ParseTagsFromInput(input string) (string, []string) {
	words := strings.Fields(input)
	var clean []string
	var tags []string
	for _, w := range words {
		if strings.HasPrefix(w, "#") && len(w) > 1 {
			tags = append(tags, strings.TrimPrefix(w, "#"))
		} else {
			clean = append(clean, w)
		}
	}
	return strings.Join(clean, " "), tags
}

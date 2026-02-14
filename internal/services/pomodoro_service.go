package services

import (
	"context"
	"fmt"
	"time"

	"github.com/dvidx/flow-cli/internal/domain"
	"github.com/dvidx/flow-cli/internal/ports"
)

// PomodoroService handles pomodoro session use cases.
type PomodoroService struct {
	storage    ports.Storage
	gitDetector ports.GitDetector
	config     domain.PomodoroConfig
}

// NewPomodoroService creates a new pomodoro service.
func NewPomodoroService(storage ports.Storage, gitDetector ports.GitDetector) *PomodoroService {
	return &PomodoroService{
		storage:     storage,
		gitDetector: gitDetector,
		config:      domain.DefaultPomodoroConfig(),
	}
}

// SetConfig updates the pomodoro configuration.
func (s *PomodoroService) SetConfig(config domain.PomodoroConfig) {
	s.config = config
}

// StartPomodoroRequest contains data to start a work session.
type StartPomodoroRequest struct {
	TaskID    *string
	WorkingDir string
}

// StartPomodoro begins a new pomodoro work session.
func (s *PomodoroService) StartPomodoro(ctx context.Context, req StartPomodoroRequest) (*domain.PomodoroSession, error) {
	// Check if there's already an active session
	active, err := s.storage.Sessions().FindActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check active sessions: %w", err)
	}
	if active != nil {
		return nil, domain.ErrSessionAlreadyActive
	}

	// If task ID provided, verify it exists and mark it as active
	if req.TaskID != nil {
		task, err := s.storage.Tasks().FindByID(ctx, *req.TaskID)
		if err != nil {
			return nil, fmt.Errorf("task not found: %w", err)
		}
		task.Start()
		if err := s.storage.Tasks().Update(ctx, task); err != nil {
			return nil, fmt.Errorf("failed to update task: %w", err)
		}
	}

	// Create and save the session
	session := domain.NewPomodoroSession(s.config, req.TaskID)

	// Detect git context if available
	if s.gitDetector != nil && s.gitDetector.IsAvailable() {
		gitInfo, err := s.gitDetector.Detect(ctx, req.WorkingDir)
		if err == nil && gitInfo != nil {
			session.SetGitContext(gitInfo.Branch, gitInfo.Commit, gitInfo.Modified)
		}
	}

	if err := s.storage.Sessions().Save(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to save session: %w", err)
	}

	return session, nil
}

// StartBreak begins a new break session.
func (s *PomodoroService) StartBreak(ctx context.Context, workingDir string) (*domain.PomodoroSession, error) {
	// Check if there's already an active session
	active, err := s.storage.Sessions().FindActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check active sessions: %w", err)
	}
	if active != nil {
		return nil, domain.ErrSessionAlreadyActive
	}

	// Get today's session count to determine short vs long break
	stats, err := s.storage.Sessions().GetDailyStats(ctx, time.Now())
	if err != nil {
		stats = &domain.DailyStats{}
	}

	session := domain.NewBreakSession(s.config, stats.WorkSessions)

	if err := s.storage.Sessions().Save(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to save break session: %w", err)
	}

	return session, nil
}

// PauseSession pauses the active session.
func (s *PomodoroService) PauseSession(ctx context.Context) (*domain.PomodoroSession, error) {
	session, err := s.storage.Sessions().FindActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to find active session: %w", err)
	}
	if session == nil {
		return nil, domain.ErrNoActiveSession
	}

	session.Pause()
	if err := s.storage.Sessions().Update(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to update session: %w", err)
	}

	return session, nil
}

// ResumeSession resumes a paused session.
func (s *PomodoroService) ResumeSession(ctx context.Context) (*domain.PomodoroSession, error) {
	session, err := s.storage.Sessions().FindActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to find active session: %w", err)
	}
	if session == nil {
		return nil, domain.ErrNoActiveSession
	}

	session.Resume()
	if err := s.storage.Sessions().Update(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to update session: %w", err)
	}

	return session, nil
}

// StopSession completes the active session.
func (s *PomodoroService) StopSession(ctx context.Context) (*domain.PomodoroSession, error) {
	session, err := s.storage.Sessions().FindActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to find active session: %w", err)
	}
	if session == nil {
		return nil, domain.ErrNoActiveSession
	}

	session.Complete()
	if err := s.storage.Sessions().Update(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to update session: %w", err)
	}

	return session, nil
}

// CancelSession aborts the active session.
func (s *PomodoroService) CancelSession(ctx context.Context) error {
	session, err := s.storage.Sessions().FindActive(ctx)
	if err != nil {
		return fmt.Errorf("failed to find active session: %w", err)
	}
	if session == nil {
		return domain.ErrNoActiveSession
	}

	session.Cancel()
	return s.storage.Sessions().Update(ctx, session)
}

// GetCurrentState retrieves the complete current application state.
func (s *PomodoroService) GetCurrentState(ctx context.Context) (*domain.CurrentState, error) {
	activeTask, _ := s.storage.Tasks().FindActive(ctx)
	activeSession, _ := s.storage.Sessions().FindActive(ctx)

	stats, err := s.storage.Sessions().GetDailyStats(ctx, time.Now())
	if err != nil {
		stats = &domain.DailyStats{Date: time.Now()}
	}

	return &domain.CurrentState{
		ActiveTask:    activeTask,
		ActiveSession: activeSession,
		TodayStats:    *stats,
	}, nil
}

// GetTaskHistory retrieves session history for a specific task.
func (s *PomodoroService) GetTaskHistory(ctx context.Context, taskID string) ([]*domain.PomodoroSession, error) {
	return s.storage.Sessions().FindByTask(ctx, taskID)
}

// GetRecentSessions retrieves recent pomodoro sessions.
func (s *PomodoroService) GetRecentSessions(ctx context.Context, limit int) ([]*domain.PomodoroSession, error) {
	// Get sessions from the last 7 days
	since := time.Now().AddDate(0, 0, -7)
	sessions, err := s.storage.Sessions().FindRecent(ctx, since)
	if err != nil {
		return nil, err
	}

	if len(sessions) > limit {
		return sessions[:limit], nil
	}
	return sessions, nil
}

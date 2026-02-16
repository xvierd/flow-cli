package services

import (
	"context"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

// StateService implements the MCPStateProvider interface.
type StateService struct {
	storage       ports.Storage
	taskService   *TaskService
	pomodoroSvc   *PomodoroService
}

// NewStateService creates a new state service.
func NewStateService(storage ports.Storage) *StateService {
	return &StateService{storage: storage}
}

// SetTaskService sets the task service for write operations.
func (s *StateService) SetTaskService(taskService *TaskService) {
	s.taskService = taskService
}

// SetPomodoroService sets the pomodoro service for write operations.
func (s *StateService) SetPomodoroService(pomodoroSvc *PomodoroService) {
	s.pomodoroSvc = pomodoroSvc
}

// GetCurrentState implements ports.MCPStateProvider.
func (s *StateService) GetCurrentState(ctx context.Context) (*domain.CurrentState, error) {
	activeTask, _ := s.storage.Tasks().FindActive(ctx)
	activeSession, _ := s.storage.Sessions().FindActive(ctx)

	// Auto-complete expired sessions that are still marked as running
	if activeSession != nil && activeSession.Status == domain.SessionStatusRunning && activeSession.RemainingTime() == 0 {
		activeSession.Complete()
		_ = s.storage.Sessions().Update(ctx, activeSession)
		activeSession = nil
	}

	todayStats, err := s.storage.Sessions().GetDailyStats(ctx, time.Now())
	if err != nil {
		todayStats = &domain.DailyStats{}
	}

	return &domain.CurrentState{
		ActiveTask:    activeTask,
		ActiveSession: activeSession,
		TodayStats:    *todayStats,
	}, nil
}

// ListTasks implements ports.MCPStateProvider.
func (s *StateService) ListTasks(ctx context.Context, status *domain.TaskStatus) ([]*domain.Task, error) {
	return s.storage.Tasks().FindAll(ctx, status)
}

// GetTaskHistory implements ports.MCPStateProvider.
func (s *StateService) GetTaskHistory(ctx context.Context, taskID string) ([]*domain.PomodoroSession, error) {
	return s.storage.Sessions().FindByTask(ctx, taskID)
}

// GetRecentSessions implements ports.MCPStateProvider.
func (s *StateService) GetRecentSessions(ctx context.Context, limit int) ([]*domain.PomodoroSession, error) {
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

// StartPomodoro implements ports.MCPStateProvider.
func (s *StateService) StartPomodoro(ctx context.Context, taskID *string, durationMinutes *int) (*domain.PomodoroSession, error) {
	if s.pomodoroSvc == nil {
		return nil, domain.ErrNoActiveSession
	}
	req := StartPomodoroRequest{
		TaskID:     taskID,
		WorkingDir: "",
	}
	if durationMinutes != nil {
		req.Duration = time.Duration(*durationMinutes) * time.Minute
	}
	return s.pomodoroSvc.StartPomodoro(ctx, req)
}

// StopPomodoro implements ports.MCPStateProvider.
func (s *StateService) StopPomodoro(ctx context.Context) (*domain.PomodoroSession, error) {
	if s.pomodoroSvc == nil {
		return nil, domain.ErrNoActiveSession
	}
	return s.pomodoroSvc.StopSession(ctx)
}

// PausePomodoro implements ports.MCPStateProvider.
func (s *StateService) PausePomodoro(ctx context.Context) (*domain.PomodoroSession, error) {
	if s.pomodoroSvc == nil {
		return nil, domain.ErrNoActiveSession
	}
	return s.pomodoroSvc.PauseSession(ctx)
}

// ResumePomodoro implements ports.MCPStateProvider.
func (s *StateService) ResumePomodoro(ctx context.Context) (*domain.PomodoroSession, error) {
	if s.pomodoroSvc == nil {
		return nil, domain.ErrNoActiveSession
	}
	return s.pomodoroSvc.ResumeSession(ctx)
}

// CreateTask implements ports.MCPStateProvider.
func (s *StateService) CreateTask(ctx context.Context, title string, description *string, tags []string) (*domain.Task, error) {
	if s.taskService == nil {
		return nil, domain.ErrTaskNotFound
	}
	req := AddTaskRequest{
		Title:       title,
		Description: "",
		Tags:        tags,
	}
	if description != nil {
		req.Description = *description
	}
	return s.taskService.AddTask(ctx, req)
}

// CompleteTask implements ports.MCPStateProvider.
func (s *StateService) CompleteTask(ctx context.Context, taskID string) (*domain.Task, error) {
	if s.taskService == nil {
		return nil, domain.ErrTaskNotFound
	}
	if err := s.taskService.CompleteTask(ctx, taskID); err != nil {
		return nil, err
	}
	return s.storage.Tasks().FindByID(ctx, taskID)
}

// AddSessionNotes implements ports.MCPStateProvider.
func (s *StateService) AddSessionNotes(ctx context.Context, sessionID string, notes string) (*domain.PomodoroSession, error) {
	if s.pomodoroSvc == nil {
		return nil, domain.ErrNoActiveSession
	}
	return s.pomodoroSvc.AddSessionNotes(ctx, sessionID, notes)
}

// Ensure StateService implements MCPStateProvider.
var _ ports.MCPStateProvider = (*StateService)(nil)

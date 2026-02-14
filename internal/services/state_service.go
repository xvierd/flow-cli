package services

import (
	"context"
	"time"

	"github.com/dvidx/flow-cli/internal/domain"
	"github.com/dvidx/flow-cli/internal/ports"
)

// StateService implements the MCPStateProvider interface.
type StateService struct {
	storage ports.Storage
}

// NewStateService creates a new state service.
func NewStateService(storage ports.Storage) *StateService {
	return &StateService{storage: storage}
}

// GetCurrentState implements ports.MCPStateProvider.
func (s *StateService) GetCurrentState(ctx context.Context) (*domain.CurrentState, error) {
	activeTask, _ := s.storage.Tasks().FindActive(ctx)
	activeSession, _ := s.storage.Sessions().FindActive(ctx)

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

// Ensure StateService implements MCPStateProvider.
var _ ports.MCPStateProvider = (*StateService)(nil)

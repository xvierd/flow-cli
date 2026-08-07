package services

import (
	"context"
	"strings"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

// StateService implements the MCPStateProvider interface.
type StateService struct {
	storage     ports.Storage
	taskService *TaskService
	pomodoroSvc *PomodoroService
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
// Deprecated: kept for backward compatibility; prefer StartSession.
func (s *StateService) StartPomodoro(ctx context.Context, taskID *string, durationMinutes *int) (*domain.PomodoroSession, error) {
	return s.StartSession(ctx, ports.StartSessionRequest{
		TaskID:          taskID,
		DurationMinutes: durationMinutes,
	})
}

// StartSession implements ports.MCPStateProvider.
func (s *StateService) StartSession(ctx context.Context, req ports.StartSessionRequest) (*domain.PomodoroSession, error) {
	if s.pomodoroSvc == nil {
		return nil, domain.ErrNoActiveSession
	}

	if req.Methodology != "" {
		if _, err := domain.ValidateMethodology(string(req.Methodology)); err != nil {
			return nil, err
		}
	}

	// Resolve the task: explicit ID wins, then a title match, then a new task.
	if req.TaskID == nil && strings.TrimSpace(req.TaskTitle) != "" {
		matches, err := s.storage.Tasks().FindByTitle(ctx, strings.TrimSpace(req.TaskTitle))
		if err == nil {
			for _, m := range matches {
				if strings.EqualFold(m.Title, strings.TrimSpace(req.TaskTitle)) {
					id := m.ID
					req.TaskID = &id
					break
				}
			}
		}
		if req.TaskID == nil {
			if s.taskService == nil {
				return nil, domain.ErrTaskNotFound
			}
			task, err := s.taskService.AddTask(ctx, AddTaskRequest{
				Title: strings.TrimSpace(req.TaskTitle),
				Tags:  req.Tags,
			})
			if err != nil {
				return nil, err
			}
			req.TaskID = &task.ID
		}
	}

	svcReq := StartPomodoroRequest{
		TaskID:          req.TaskID,
		Methodology:     req.Methodology,
		IntendedOutcome: req.IntendedOutcome,
		Tags:            req.Tags,
	}
	if req.DurationMinutes != nil && *req.DurationMinutes > 0 {
		svcReq.Duration = time.Duration(*req.DurationMinutes) * time.Minute
	}
	return s.pomodoroSvc.StartPomodoro(ctx, svcReq)
}

// StartBreak implements ports.MCPStateProvider.
func (s *StateService) StartBreak(ctx context.Context) (*domain.PomodoroSession, error) {
	if s.pomodoroSvc == nil {
		return nil, domain.ErrNoActiveSession
	}
	return s.pomodoroSvc.StartBreak(ctx, "")
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

// CancelSession implements ports.MCPStateProvider.
func (s *StateService) CancelSession(ctx context.Context) error {
	if s.pomodoroSvc == nil {
		return domain.ErrNoActiveSession
	}
	return s.pomodoroSvc.CancelSession(ctx)
}

// VoidSession implements ports.MCPStateProvider.
func (s *StateService) VoidSession(ctx context.Context) (*domain.PomodoroSession, error) {
	if s.pomodoroSvc == nil {
		return nil, domain.ErrNoActiveSession
	}
	return s.pomodoroSvc.VoidSession(ctx)
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

// GetTask implements ports.MCPStateProvider.
func (s *StateService) GetTask(ctx context.Context, taskID string) (*domain.Task, error) {
	return s.taskService.GetTask(ctx, taskID)
}

// DeleteTask implements ports.MCPStateProvider.
func (s *StateService) DeleteTask(ctx context.Context, taskID string) error {
	if s.taskService == nil {
		return domain.ErrTaskNotFound
	}
	return s.taskService.DeleteTask(ctx, taskID)
}

// StartTask implements ports.MCPStateProvider.
func (s *StateService) StartTask(ctx context.Context, taskID string) error {
	if s.taskService == nil {
		return domain.ErrTaskNotFound
	}
	return s.taskService.StartTask(ctx, taskID)
}

// AddSessionNotes implements ports.MCPStateProvider.
func (s *StateService) AddSessionNotes(ctx context.Context, sessionID string, notes string) (*domain.PomodoroSession, error) {
	if s.pomodoroSvc == nil {
		return nil, domain.ErrNoActiveSession
	}
	return s.pomodoroSvc.AddSessionNotes(ctx, sessionID, notes)
}

// LogDistraction implements ports.MCPStateProvider.
func (s *StateService) LogDistraction(ctx context.Context, sessionID string, text string, category string) error {
	if s.pomodoroSvc == nil {
		return domain.ErrNoActiveSession
	}
	return s.pomodoroSvc.LogDistraction(ctx, sessionID, text, category)
}

// SetFocusScore implements ports.MCPStateProvider.
func (s *StateService) SetFocusScore(ctx context.Context, sessionID string, score int) error {
	if s.pomodoroSvc == nil {
		return domain.ErrNoActiveSession
	}
	return s.pomodoroSvc.SetFocusScore(ctx, sessionID, score)
}

// SetAccomplishment implements ports.MCPStateProvider.
func (s *StateService) SetAccomplishment(ctx context.Context, sessionID string, text string) error {
	if s.pomodoroSvc == nil {
		return domain.ErrNoActiveSession
	}
	return s.pomodoroSvc.SetAccomplishment(ctx, sessionID, text)
}

// SetShutdownRitual implements ports.MCPStateProvider.
func (s *StateService) SetShutdownRitual(ctx context.Context, sessionID string, ritual domain.ShutdownRitual) error {
	if s.pomodoroSvc == nil {
		return domain.ErrNoActiveSession
	}
	return s.pomodoroSvc.SetShutdownRitual(ctx, sessionID, ritual)
}

// SetEnergizeActivity implements ports.MCPStateProvider.
func (s *StateService) SetEnergizeActivity(ctx context.Context, sessionID string, activity string) error {
	if s.pomodoroSvc == nil {
		return domain.ErrNoActiveSession
	}
	return s.pomodoroSvc.SetEnergizeActivity(ctx, sessionID, activity)
}

// SetOutcomeAchieved implements ports.MCPStateProvider.
func (s *StateService) SetOutcomeAchieved(ctx context.Context, sessionID string, achieved string) error {
	if s.pomodoroSvc == nil {
		return domain.ErrNoActiveSession
	}
	return s.pomodoroSvc.SetOutcomeAchieved(ctx, sessionID, achieved)
}

// GetTodayHighlight implements ports.MCPStateProvider.
func (s *StateService) GetTodayHighlight(ctx context.Context) (*domain.Task, error) {
	return s.storage.Tasks().FindTodayHighlight(ctx, time.Now())
}

// SetHighlight implements ports.MCPStateProvider.
func (s *StateService) SetHighlight(ctx context.Context, taskID string) (*domain.Task, error) {
	task, err := s.storage.Tasks().FindByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	task.SetAsHighlight()
	if err := s.storage.Tasks().Update(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

// GetDailySummary implements ports.MCPStateProvider.
func (s *StateService) GetDailySummary(ctx context.Context, date time.Time) (*domain.DailyStats, error) {
	return s.storage.Sessions().GetDailyStats(ctx, date)
}

// GetPeriodStats implements ports.MCPStateProvider.
func (s *StateService) GetPeriodStats(ctx context.Context, start, end time.Time) (*domain.PeriodStats, error) {
	return s.storage.Sessions().GetPeriodStats(ctx, start, end)
}

// GetFocusReport implements ports.MCPStateProvider.
func (s *StateService) GetFocusReport(ctx context.Context) (*domain.FocusReport, error) {
	now := time.Now()
	sessions, err := s.storage.Sessions().FindRecent(ctx, startOfDay(now))
	if err != nil {
		return nil, err
	}

	report := &domain.FocusReport{
		Date: now.Format(time.RFC3339),
	}

	var totalFocus int
	for _, sess := range sessions {
		if !sess.IsWorkSession() {
			continue
		}
		if sess.Status == domain.SessionStatusCompleted {
			report.WorkSessions++
			report.TotalWorkTime += sess.Duration
		}
		if sess.FocusScore != nil {
			report.FocusScoreCount++
			totalFocus += *sess.FocusScore
		}
		report.DistractionCount += len(sess.Distractions)
	}
	if report.FocusScoreCount > 0 {
		report.AvgFocusScore = float64(totalFocus) / float64(report.FocusScoreCount)
	}

	report.DeepWorkStreak, _ = s.storage.Sessions().GetDeepWorkStreak(ctx, 4*time.Hour)

	hl, err := s.storage.Tasks().FindTodayHighlight(ctx, now)
	if err == nil && hl != nil {
		report.HighlightID = &hl.ID
		report.HighlightTitle = hl.Title
	}

	return report, nil
}

// ensure StateService implements MCPStateProvider.
var _ ports.MCPStateProvider = (*StateService)(nil)

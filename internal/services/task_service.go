// Package services implements the application layer (use cases)
// following hexagonal architecture principles.
package services

import (
	"context"
	"fmt"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

// TaskService handles task-related use cases.
type TaskService struct {
	storage ports.Storage
}

// NewTaskService creates a new task service.
func NewTaskService(storage ports.Storage) *TaskService {
	return &TaskService{storage: storage}
}

// AddTaskRequest contains the data needed to create a new task.
type AddTaskRequest struct {
	Title       string
	Description string
	Tags        []string
}

// AddTask creates a new task.
func (s *TaskService) AddTask(ctx context.Context, req AddTaskRequest) (*domain.Task, error) {
	return s.addTask(ctx, s.storage, req)
}

// AddTaskWith is AddTask against the given storage, which may be
// transaction-scoped, so callers can compose it into a larger WithTx
// transaction.
func (s *TaskService) AddTaskWith(ctx context.Context, storage ports.Storage, req AddTaskRequest) (*domain.Task, error) {
	return s.addTask(ctx, storage, req)
}

// addTask implements AddTask against the given storage.
func (s *TaskService) addTask(ctx context.Context, storage ports.Storage, req AddTaskRequest) (*domain.Task, error) {
	task, err := domain.NewTask(req.Title)
	if err != nil {
		return nil, fmt.Errorf("invalid task: %w", err)
	}

	task.Description = req.Description
	for _, tag := range req.Tags {
		task.AddTag(tag)
	}

	if err := storage.Tasks().Save(ctx, task); err != nil {
		return nil, fmt.Errorf("failed to save task: %w", err)
	}

	return task, nil
}

// ListTasksRequest contains filters for listing tasks.
type ListTasksRequest struct {
	Status      *domain.TaskStatus
	OnlyPending bool
}

// ListTasks retrieves tasks based on filters.
func (s *TaskService) ListTasks(ctx context.Context, req ListTasksRequest) ([]*domain.Task, error) {
	if req.OnlyPending {
		return s.storage.Tasks().FindPending(ctx)
	}
	return s.storage.Tasks().FindAll(ctx, req.Status)
}

// GetTask retrieves a single task by ID.
func (s *TaskService) GetTask(ctx context.Context, id string) (*domain.Task, error) {
	return s.storage.Tasks().FindByID(ctx, id)
}

// CompleteTask marks a task as completed.
func (s *TaskService) CompleteTask(ctx context.Context, id string) error {
	task, err := s.storage.Tasks().FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to find task: %w", err)
	}

	task.Complete()
	return s.storage.Tasks().Update(ctx, task)
}

// DeleteTask removes a task.
func (s *TaskService) DeleteTask(ctx context.Context, id string) error {
	return s.storage.Tasks().Delete(ctx, id)
}

// StartTask marks a task as in progress.
func (s *TaskService) StartTask(ctx context.Context, id string) error {
	task, err := s.storage.Tasks().FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to find task: %w", err)
	}

	task.Start()
	return s.storage.Tasks().Update(ctx, task)
}

// SetHighlight makes the task today's single highlight: any other task
// highlighted for today is cleared first. Clearing and setting run in one
// transaction.
func (s *TaskService) SetHighlight(ctx context.Context, taskID string) (*domain.Task, error) {
	var task *domain.Task
	err := s.storage.WithTx(ctx, func(tx ports.Storage) error {
		var err error
		task, err = setHighlight(ctx, tx, taskID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return task, nil
}

// SetHighlightWith is SetHighlight without its own transaction: it runs
// against the given storage, which may be transaction-scoped, so callers can
// compose it into a larger WithTx transaction.
func (s *TaskService) SetHighlightWith(ctx context.Context, storage ports.Storage, taskID string) (*domain.Task, error) {
	return setHighlight(ctx, storage, taskID)
}

// setHighlight clears every highlight for today (the target included) and
// then marks the target task as today's highlight, against the given storage.
func setHighlight(ctx context.Context, storage ports.Storage, taskID string) (*domain.Task, error) {
	task, err := storage.Tasks().FindByID(ctx, taskID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	cleared := make(map[string]bool)
	for {
		prev, err := storage.Tasks().FindTodayHighlight(ctx, now)
		if err != nil {
			return nil, err
		}
		if prev == nil {
			break
		}
		if cleared[prev.ID] {
			return nil, fmt.Errorf("failed to clear highlight of task %s", prev.ID)
		}
		cleared[prev.ID] = true
		prev.HighlightDate = nil
		if err := storage.Tasks().Update(ctx, prev); err != nil {
			return nil, err
		}
	}

	task.SetAsHighlight()
	if err := storage.Tasks().Update(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

// Package services implements the application layer (use cases)
// following hexagonal architecture principles.
package services

import (
	"context"
	"fmt"
	"strings"
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

// FindOrAddTaskWith returns the pending task titled title (compared without
// regard to case), creating it when there is none. It runs against the given
// storage, which may be transaction-scoped. Starting several sessions from
// the same branch thus reuses one task instead of piling up duplicates.
func (s *TaskService) FindOrAddTaskWith(ctx context.Context, storage ports.Storage, title string) (*domain.Task, error) {
	pending, err := storage.Tasks().FindPending(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending tasks: %w", err)
	}
	for _, task := range pending {
		if strings.EqualFold(strings.TrimSpace(task.Title), strings.TrimSpace(title)) {
			return task, nil
		}
	}
	return s.addTask(ctx, storage, AddTaskRequest{Title: title})
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
		return err
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
		return err
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

// UpdateTaskRequest lists the task fields to change; nil fields are kept.
type UpdateTaskRequest struct {
	Title       *string
	Description *string
	// Tags, when set, replaces the task's tags (an empty slice clears them).
	Tags *[]string
	// Reopen moves a completed or cancelled task back to pending.
	Reopen bool
}

// UpdateTask edits a task (id may be a unique ID prefix) and returns it.
func (s *TaskService) UpdateTask(ctx context.Context, id string, req UpdateTaskRequest) (*domain.Task, error) {
	var task *domain.Task
	err := s.storage.WithTx(ctx, func(tx ports.Storage) error {
		var err error
		task, err = tx.Tasks().FindByID(ctx, id)
		if err != nil {
			return err
		}
		if req.Title != nil {
			title := strings.TrimSpace(*req.Title)
			if title == "" {
				return domain.ErrEmptyTaskTitle
			}
			task.Title = title
		}
		if req.Description != nil {
			task.Description = *req.Description
		}
		if req.Tags != nil {
			task.Tags = nil
			for _, tag := range *req.Tags {
				if tag = strings.TrimSpace(tag); tag != "" {
					task.AddTag(tag)
				}
			}
		}
		if req.Reopen && (task.Status == domain.StatusCompleted || task.Status == domain.StatusCancelled) {
			task.Status = domain.StatusPending
			task.CompletedAt = nil
		}
		return tx.Tasks().Update(ctx, task)
	})
	if err != nil {
		return nil, err
	}
	return task, nil
}

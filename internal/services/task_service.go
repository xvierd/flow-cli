// Package services implements the application layer (use cases)
// following hexagonal architecture principles.
package services

import (
	"context"
	"fmt"

	"github.com/xavier/flow/internal/domain"
	"github.com/xavier/flow/internal/ports"
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
	task, err := domain.NewTask(req.Title)
	if err != nil {
		return nil, fmt.Errorf("invalid task: %w", err)
	}

	task.Description = req.Description
	for _, tag := range req.Tags {
		task.AddTag(tag)
	}

	if err := s.storage.Tasks().Save(ctx, task); err != nil {
		return nil, fmt.Errorf("failed to save task: %w", err)
	}

	return task, nil
}

// ListTasksRequest contains filters for listing tasks.
type ListTasksRequest struct {
	Status *domain.TaskStatus
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

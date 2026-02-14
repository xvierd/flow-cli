// Package ports defines the interfaces (driven and driving ports)
// for the Flow application following hexagonal architecture principles.
// These interfaces define the contracts between the domain layer and
// external infrastructure.
package ports

import (
	"context"
	"time"

	"github.com/xavier/flow/internal/domain"
)

// TaskRepository defines the interface for task persistence.
// This is a driven port (implemented by adapters).
type TaskRepository interface {
	// Save persists a task to storage.
	Save(ctx context.Context, task *domain.Task) error

	// FindByID retrieves a task by its unique identifier.
	FindByID(ctx context.Context, id string) (*domain.Task, error)

	// FindAll retrieves all tasks, optionally filtered by status.
	FindAll(ctx context.Context, status *domain.TaskStatus) ([]*domain.Task, error)

	// FindPending returns all tasks that are not completed or cancelled.
	FindPending(ctx context.Context) ([]*domain.Task, error)

	// FindActive returns the currently active task (in_progress).
	FindActive(ctx context.Context) (*domain.Task, error)

	// Delete removes a task from storage.
	Delete(ctx context.Context, id string) error

	// Update modifies an existing task.
	Update(ctx context.Context, task *domain.Task) error
}

// SessionRepository defines the interface for pomodoro session persistence.
// This is a driven port (implemented by adapters).
type SessionRepository interface {
	// Save persists a session to storage.
	Save(ctx context.Context, session *domain.PomodoroSession) error

	// FindByID retrieves a session by its unique identifier.
	FindByID(ctx context.Context, id string) (*domain.PomodoroSession, error)

	// FindActive retrieves the currently running or paused session.
	FindActive(ctx context.Context) (*domain.PomodoroSession, error)

	// FindRecent retrieves sessions within a time range.
	FindRecent(ctx context.Context, since time.Time) ([]*domain.PomodoroSession, error)

	// FindByTask retrieves all sessions associated with a task.
	FindByTask(ctx context.Context, taskID string) ([]*domain.PomodoroSession, error)

	// Update modifies an existing session.
	Update(ctx context.Context, session *domain.PomodoroSession) error

	// GetDailyStats returns aggregated statistics for a specific date.
	GetDailyStats(ctx context.Context, date time.Time) (*domain.DailyStats, error)
}

// Storage is the combined repository interface.
// This is a driven port (implemented by adapters).
type Storage interface {
	// Tasks provides access to task operations.
	Tasks() TaskRepository

	// Sessions provides access to session operations.
	Sessions() SessionRepository

	// Close closes the storage connection.
	Close() error

	// Migrate runs database migrations.
	Migrate() error
}

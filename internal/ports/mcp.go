package ports

import (
	"context"

	"github.com/dvidx/flow-cli/internal/domain"
)

// MCPHandler defines the interface for MCP server operations.
// This is a driving port (called by the application layer).
type MCPHandler interface {
	// Start begins serving MCP requests.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the server.
	Stop() error

	// IsRunning returns true if the server is active.
	IsRunning() bool
}

// MCPStateProvider provides state information to the MCP server.
// This is a driven port (implemented by services layer).
type MCPStateProvider interface {
	// GetCurrentState returns the current application state.
	GetCurrentState(ctx context.Context) (*domain.CurrentState, error)

	// ListTasks returns all tasks, optionally filtered.
	ListTasks(ctx context.Context, status *domain.TaskStatus) ([]*domain.Task, error)

	// GetTaskHistory returns session history for a specific task.
	GetTaskHistory(ctx context.Context, taskID string) ([]*domain.PomodoroSession, error)

	// GetRecentSessions returns recent pomodoro sessions.
	GetRecentSessions(ctx context.Context, limit int) ([]*domain.PomodoroSession, error)
}

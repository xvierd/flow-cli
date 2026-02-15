// Package storage provides SQLite implementations of the storage ports.
package storage

import (
	"database/sql"
	"fmt"

	"github.com/xvierd/flow-cli/internal/ports"
	"modernc.org/sqlite"
)

// sqliteStorage implements the ports.Storage interface using SQLite.
type sqliteStorage struct {
	db            *sql.DB
	taskRepo      ports.TaskRepository
	sessionRepo   ports.SessionRepository
}

// Ensure sqliteStorage implements ports.Storage.
var _ ports.Storage = (*sqliteStorage)(nil)

// New creates a new SQLite storage instance.
func New(dbPath string) (ports.Storage, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign keys and WAL mode for better performance
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}

	storage := &sqliteStorage{
		db:          db,
		taskRepo:    newTaskRepository(db),
		sessionRepo: newSessionRepository(db),
	}

	if err := storage.Migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return storage, nil
}

// NewMemory creates a new in-memory SQLite storage instance for testing.
func NewMemory() (ports.Storage, error) {
	return New(":memory:")
}

// Tasks returns the task repository.
func (s *sqliteStorage) Tasks() ports.TaskRepository {
	return s.taskRepo
}

// Sessions returns the session repository.
func (s *sqliteStorage) Sessions() ports.SessionRepository {
	return s.sessionRepo
}

// Close closes the database connection.
func (s *sqliteStorage) Close() error {
	return s.db.Close()
}

// Migrate creates the database schema.
func (s *sqliteStorage) Migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT,
		status TEXT NOT NULL,
		tags TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		completed_at DATETIME
	);

	CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
	CREATE INDEX IF NOT EXISTS idx_tasks_updated ON tasks(updated_at);

	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		task_id TEXT,
		type TEXT NOT NULL,
		status TEXT NOT NULL,
		duration_ms INTEGER NOT NULL,
		started_at DATETIME NOT NULL,
		paused_at DATETIME,
		completed_at DATETIME,
		git_branch TEXT,
		git_commit TEXT,
		git_modified TEXT,
		notes TEXT,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE SET NULL
	);

	CREATE INDEX IF NOT EXISTS idx_sessions_task ON sessions(task_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_started ON sessions(started_at);
	CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
	`

	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}

	return nil
}

// isUniqueConstraintError checks if an error is a unique constraint violation.
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	sqliteErr, ok := err.(*sqlite.Error)
	return ok && sqliteErr.Code() == 2067 // SQLITE_CONSTRAINT_UNIQUE
}

package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/dvidx/flow-cli/internal/domain"
	"github.com/dvidx/flow-cli/internal/ports"
)

// sessionRepository implements ports.SessionRepository using SQLite.
type sessionRepository struct {
	db *sql.DB
}

// newSessionRepository creates a new session repository.
func newSessionRepository(db *sql.DB) ports.SessionRepository {
	return &sessionRepository{db: db}
}

// Save persists a session to storage.
func (r *sessionRepository) Save(ctx context.Context, session *domain.PomodoroSession) error {
	query := `
		INSERT INTO sessions (
			id, task_id, type, status, duration_ms, started_at, paused_at, 
			completed_at, git_branch, git_commit, git_modified, notes
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	modified := strings.Join(session.GitModified, ",")

	_, err := r.db.ExecContext(ctx, query,
		session.ID,
		session.TaskID,
		string(session.Type),
		string(session.Status),
		session.Duration.Milliseconds(),
		session.StartedAt,
		session.PausedAt,
		session.CompletedAt,
		session.GitBranch,
		session.GitCommit,
		modified,
		session.Notes,
	)

	if err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	return nil
}

// FindByID retrieves a session by its unique identifier.
func (r *sessionRepository) FindByID(ctx context.Context, id string) (*domain.PomodoroSession, error) {
	query := `
		SELECT
			id, task_id, type, status, duration_ms, started_at, paused_at,
			completed_at, git_branch, git_commit, git_modified, notes
		FROM sessions
		WHERE id = ?
	`

	return r.scanSession(r.db.QueryRowContext(ctx, query, id))
}

// FindActive retrieves the currently running or paused session.
func (r *sessionRepository) FindActive(ctx context.Context) (*domain.PomodoroSession, error) {
	query := `
		SELECT
			id, task_id, type, status, duration_ms, started_at, paused_at,
			completed_at, git_branch, git_commit, git_modified, notes
		FROM sessions
		WHERE status IN (?, ?)
		ORDER BY started_at DESC
		LIMIT 1
	`

	return r.scanSession(r.db.QueryRowContext(ctx, query,
		string(domain.SessionStatusRunning),
		string(domain.SessionStatusPaused)))
}

// FindRecent retrieves sessions within a time range.
func (r *sessionRepository) FindRecent(ctx context.Context, since time.Time) ([]*domain.PomodoroSession, error) {
	query := `
		SELECT
			id, task_id, type, status, duration_ms, started_at, paused_at,
			completed_at, git_branch, git_commit, git_modified, notes
		FROM sessions
		WHERE started_at >= ?
		ORDER BY started_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, since)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent sessions: %w", err)
	}
	defer rows.Close()

	return r.scanSessions(rows)
}

// FindByTask retrieves all sessions associated with a task.
func (r *sessionRepository) FindByTask(ctx context.Context, taskID string) ([]*domain.PomodoroSession, error) {
	query := `
		SELECT
			id, task_id, type, status, duration_ms, started_at, paused_at,
			completed_at, git_branch, git_commit, git_modified, notes
		FROM sessions
		WHERE task_id = ?
		ORDER BY started_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions by task: %w", err)
	}
	defer rows.Close()

	return r.scanSessions(rows)
}

// Update modifies an existing session.
func (r *sessionRepository) Update(ctx context.Context, session *domain.PomodoroSession) error {
	query := `
		UPDATE sessions
		SET task_id = ?, type = ?, status = ?, duration_ms = ?, started_at = ?,
		    paused_at = ?, completed_at = ?, git_branch = ?, git_commit = ?, git_modified = ?, notes = ?
		WHERE id = ?
	`

	modified := strings.Join(session.GitModified, ",")

	result, err := r.db.ExecContext(ctx, query,
		session.TaskID,
		session.Type,
		session.Status,
		session.Duration.Milliseconds(),
		session.StartedAt,
		session.PausedAt,
		session.CompletedAt,
		session.GitBranch,
		session.GitCommit,
		modified,
		session.Notes,
		session.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("session not found: %s", session.ID)
	}

	return nil
}

// GetDailyStats returns aggregated statistics for a specific date.
func (r *sessionRepository) GetDailyStats(ctx context.Context, date time.Time) (*domain.DailyStats, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	query := `
		SELECT 
			COUNT(CASE WHEN type = 'work' AND status = 'completed' THEN 1 END) as work_sessions,
			COUNT(CASE WHEN type IN ('short_break', 'long_break') AND status = 'completed' THEN 1 END) as breaks,
			COALESCE(SUM(CASE WHEN type = 'work' AND status = 'completed' THEN duration_ms END), 0) as total_work_ms
		FROM sessions
		WHERE started_at >= ? AND started_at < ?
	`

	stats := &domain.DailyStats{
		Date: startOfDay,
	}

	var totalWorkMs int64
	err := r.db.QueryRowContext(ctx, query, startOfDay, endOfDay).Scan(
		&stats.WorkSessions,
		&stats.BreaksTaken,
		&totalWorkMs,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get daily stats: %w", err)
	}

	stats.TotalWorkTime = time.Duration(totalWorkMs) * time.Millisecond

	return stats, nil
}

// scanSession scans a single session row.
func (r *sessionRepository) scanSession(row *sql.Row) (*domain.PomodoroSession, error) {
	var session domain.PomodoroSession
	var taskID sql.NullString
	var pausedAt sql.NullTime
	var completedAt sql.NullTime
	var durationMs int64
	var modifiedStr string
	var notes sql.NullString

	err := row.Scan(
		&session.ID,
		&taskID,
		&session.Type,
		&session.Status,
		&durationMs,
		&session.StartedAt,
		&pausedAt,
		&completedAt,
		&session.GitBranch,
		&session.GitCommit,
		&modifiedStr,
		&notes,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	session.Duration = time.Duration(durationMs) * time.Millisecond

	if taskID.Valid {
		session.TaskID = &taskID.String
	}
	if pausedAt.Valid {
		session.PausedAt = &pausedAt.Time
	}
	if completedAt.Valid {
		session.CompletedAt = &completedAt.Time
	}
	if modifiedStr != "" {
		session.GitModified = strings.Split(modifiedStr, ",")
	}
	if notes.Valid {
		session.Notes = notes.String
	}

	return &session, nil
}

// scanSessions scans multiple session rows.
func (r *sessionRepository) scanSessions(rows *sql.Rows) ([]*domain.PomodoroSession, error) {
	var sessions []*domain.PomodoroSession

	for rows.Next() {
		var session domain.PomodoroSession
		var taskID sql.NullString
		var pausedAt sql.NullTime
		var completedAt sql.NullTime
		var durationMs int64
		var modifiedStr string
		var notes sql.NullString

		err := rows.Scan(
			&session.ID,
			&taskID,
			&session.Type,
			&session.Status,
			&durationMs,
			&session.StartedAt,
			&pausedAt,
			&completedAt,
			&session.GitBranch,
			&session.GitCommit,
			&modifiedStr,
			&notes,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		session.Duration = time.Duration(durationMs) * time.Millisecond

		if taskID.Valid {
			session.TaskID = &taskID.String
		}
		if pausedAt.Valid {
			session.PausedAt = &pausedAt.Time
		}
		if completedAt.Valid {
			session.CompletedAt = &completedAt.Time
		}
		if modifiedStr != "" {
			session.GitModified = strings.Split(modifiedStr, ",")
		}
		if notes.Valid {
			session.Notes = notes.String
		}

		sessions = append(sessions, &session)
	}

	return sessions, rows.Err()
}

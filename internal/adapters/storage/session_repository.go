package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
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
			completed_at, git_branch, git_commit, git_modified, notes,
			methodology, focus_score, distractions, accomplishment, intended_outcome, tags,
			energize_activity, shutdown_ritual, outcome_achieved
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	modified := strings.Join(session.GitModified, ",")
	distractionsJSON, _ := json.Marshal(session.Distractions)
	methodology := string(session.Methodology)
	if methodology == "" {
		methodology = string(domain.MethodologyPomodoro)
	}
	tags := strings.Join(session.Tags, ",")

	var shutdownRitualJSON []byte
	if session.ShutdownRitual != nil {
		shutdownRitualJSON, _ = json.Marshal(session.ShutdownRitual)
	}

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
		methodology,
		session.FocusScore,
		string(distractionsJSON),
		session.Accomplishment,
		session.IntendedOutcome,
		tags,
		session.EnergizeActivity,
		nullableString(shutdownRitualJSON),
		session.OutcomeAchieved,
	)

	if err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	return nil
}

// nullableString returns a *string from bytes, or nil if empty.
func nullableString(b []byte) *string {
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	return &s
}

// unmarshalDistractions deserializes distractions with backward compat for old string format.
func unmarshalDistractions(data string) []domain.Distraction {
	if data == "" {
		return nil
	}
	// Try new JSON format first
	var distractions []domain.Distraction
	if err := json.Unmarshal([]byte(data), &distractions); err == nil {
		return distractions
	}
	// Fall back to old newline-separated string format
	parts := strings.Split(data, "\n")
	result := make([]domain.Distraction, 0, len(parts))
	for _, s := range parts {
		if s != "" {
			result = append(result, domain.Distraction{Text: s})
		}
	}
	return result
}

// FindByID retrieves a session by its unique identifier.
func (r *sessionRepository) FindByID(ctx context.Context, id string) (*domain.PomodoroSession, error) {
	query := `
		SELECT
			id, task_id, type, status, duration_ms, started_at, paused_at,
			completed_at, git_branch, git_commit, git_modified, notes,
			methodology, focus_score, distractions, accomplishment, intended_outcome, tags,
			energize_activity, shutdown_ritual, outcome_achieved
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
			completed_at, git_branch, git_commit, git_modified, notes,
			methodology, focus_score, distractions, accomplishment, intended_outcome, tags,
			energize_activity, shutdown_ritual, outcome_achieved
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
			completed_at, git_branch, git_commit, git_modified, notes,
			methodology, focus_score, distractions, accomplishment, intended_outcome, tags,
			energize_activity, shutdown_ritual, outcome_achieved
		FROM sessions
		WHERE started_at >= ?
		ORDER BY started_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, since)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanSessions(rows)
}

// FindByTask retrieves all sessions associated with a task.
func (r *sessionRepository) FindByTask(ctx context.Context, taskID string) ([]*domain.PomodoroSession, error) {
	query := `
		SELECT
			id, task_id, type, status, duration_ms, started_at, paused_at,
			completed_at, git_branch, git_commit, git_modified, notes,
			methodology, focus_score, distractions, accomplishment, intended_outcome, tags,
			energize_activity, shutdown_ritual, outcome_achieved
		FROM sessions
		WHERE task_id = ?
		ORDER BY started_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions by task: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanSessions(rows)
}

// Update modifies an existing session.
func (r *sessionRepository) Update(ctx context.Context, session *domain.PomodoroSession) error {
	query := `
		UPDATE sessions
		SET task_id = ?, type = ?, status = ?, duration_ms = ?, started_at = ?,
		    paused_at = ?, completed_at = ?, git_branch = ?, git_commit = ?, git_modified = ?, notes = ?,
		    methodology = ?, focus_score = ?, distractions = ?, accomplishment = ?, intended_outcome = ?,
		    tags = ?, energize_activity = ?, shutdown_ritual = ?, outcome_achieved = ?
		WHERE id = ?
	`

	modified := strings.Join(session.GitModified, ",")
	distractionsJSON, _ := json.Marshal(session.Distractions)
	methodology := string(session.Methodology)
	if methodology == "" {
		methodology = string(domain.MethodologyPomodoro)
	}
	tags := strings.Join(session.Tags, ",")

	var shutdownRitualJSON []byte
	if session.ShutdownRitual != nil {
		shutdownRitualJSON, _ = json.Marshal(session.ShutdownRitual)
	}

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
		methodology,
		session.FocusScore,
		string(distractionsJSON),
		session.Accomplishment,
		session.IntendedOutcome,
		tags,
		session.EnergizeActivity,
		nullableString(shutdownRitualJSON),
		session.OutcomeAchieved,
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

// GetPeriodStats returns aggregated statistics for a time range.
func (r *sessionRepository) GetPeriodStats(ctx context.Context, start, end time.Time) (*domain.PeriodStats, error) {
	stats := &domain.PeriodStats{
		Start: start,
		End:   end,
	}

	// Aggregate totals by methodology
	query := `
		SELECT
			COALESCE(methodology, 'pomodoro') as meth,
			COUNT(*) as cnt,
			COALESCE(SUM(duration_ms), 0) as total_ms
		FROM sessions
		WHERE type = 'work' AND status = 'completed'
		  AND started_at >= ? AND started_at < ?
		GROUP BY meth
	`
	rows, err := r.db.QueryContext(ctx, query, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to query period stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var methStr string
		var cnt int
		var totalMs int64
		if err := rows.Scan(&methStr, &cnt, &totalMs); err != nil {
			return nil, fmt.Errorf("failed to scan period stats: %w", err)
		}
		stats.ByMethodology = append(stats.ByMethodology, domain.MethodologyBreakdown{
			Methodology:  domain.Methodology(methStr),
			SessionCount: cnt,
			TotalTime:    time.Duration(totalMs) * time.Millisecond,
		})
		stats.TotalSessions += cnt
		stats.TotalWorkTime += time.Duration(totalMs) * time.Millisecond
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Average focus score (Make Time sessions)
	scoreQuery := `
		SELECT AVG(focus_score), COUNT(focus_score)
		FROM sessions
		WHERE type = 'work' AND status = 'completed'
		  AND focus_score IS NOT NULL
		  AND started_at >= ? AND started_at < ?
	`
	var avgScore sql.NullFloat64
	var scoreCount int
	if err := r.db.QueryRowContext(ctx, scoreQuery, start, end).Scan(&avgScore, &scoreCount); err != nil {
		// Non-fatal
		avgScore.Float64 = 0
	}
	if avgScore.Valid {
		stats.AvgFocusScore = avgScore.Float64
	}
	stats.FocusScoreCount = scoreCount

	// Count distractions (Deep Work sessions).
	// Distractions are stored as JSON arrays (new format) or newline-separated strings (legacy).
	distractQuery := `
		SELECT COALESCE(SUM(
			CASE
				WHEN distractions IS NULL OR distractions = '' THEN 0
				WHEN distractions LIKE '[%' THEN json_array_length(distractions)
				ELSE LENGTH(distractions) - LENGTH(REPLACE(distractions, CHAR(10), '')) + 1
			END
		), 0)
		FROM sessions
		WHERE type = 'work' AND status = 'completed'
		  AND started_at >= ? AND started_at < ?
	`
	if err := r.db.QueryRowContext(ctx, distractQuery, start, end).Scan(&stats.DistractionCount); err != nil {
		stats.DistractionCount = 0
	}

	return stats, nil
}

// GetDeepWorkStreak returns consecutive days (ending today) with >= threshold deep work hours.
func (r *sessionRepository) GetDeepWorkStreak(ctx context.Context, threshold time.Duration) (int, error) {
	thresholdMs := threshold.Milliseconds()
	streak := 0

	// Walk backward from today, checking each day
	now := time.Now()
	for i := 0; i < 365; i++ {
		day := now.AddDate(0, 0, -i)
		startOfDay := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
		endOfDay := startOfDay.Add(24 * time.Hour)

		query := `
			SELECT COALESCE(SUM(duration_ms), 0)
			FROM sessions
			WHERE type = 'work' AND status = 'completed'
			  AND methodology = 'deepwork'
			  AND started_at >= ? AND started_at < ?
		`

		var totalMs int64
		if err := r.db.QueryRowContext(ctx, query, startOfDay, endOfDay).Scan(&totalMs); err != nil {
			return 0, fmt.Errorf("failed to query deep work streak: %w", err)
		}

		if totalMs >= thresholdMs {
			streak++
		} else {
			// If today has no sessions yet, skip it (don't break streak for today)
			if i == 0 {
				continue
			}
			break
		}
	}

	return streak, nil
}

// GetHourlyProductivity returns total work minutes per hour-of-day for the last N days.
func (r *sessionRepository) GetHourlyProductivity(ctx context.Context, days int) (map[int]time.Duration, error) {
	since := time.Now().AddDate(0, 0, -days)

	// Go stores time.Time as RFC3339 (e.g. "2024-01-15T09:30:00Z").
	// SQLite's strftime('%H', ...) cannot parse the 'T' separator or trailing timezone,
	// so we extract the hour by byte position: chars 12-13 in the RFC3339 string.
	query := `
		SELECT
			CAST(substr(started_at, 12, 2) AS INTEGER) as hour,
			SUM(duration_ms) as total_ms
		FROM sessions
		WHERE type = 'work' AND status = 'completed'
		  AND started_at >= ?
		GROUP BY hour
		ORDER BY hour
	`

	rows, err := r.db.QueryContext(ctx, query, since)
	if err != nil {
		return nil, fmt.Errorf("failed to query hourly productivity: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int]time.Duration)
	for rows.Next() {
		var hour int
		var totalMs int64
		if err := rows.Scan(&hour, &totalMs); err != nil {
			return nil, fmt.Errorf("failed to scan hourly productivity: %w", err)
		}
		result[hour] = time.Duration(totalMs) * time.Millisecond
	}

	return result, rows.Err()
}

// GetEnergizeStats returns avg focus score per energize activity for a time range.
func (r *sessionRepository) GetEnergizeStats(ctx context.Context, start, end time.Time) ([]domain.EnergizeStat, error) {
	query := `
		SELECT
			energize_activity,
			COUNT(*) as session_count,
			AVG(focus_score) as avg_focus
		FROM sessions
		WHERE type = 'work' AND status = 'completed'
		  AND energize_activity IS NOT NULL AND energize_activity != ''
		  AND focus_score IS NOT NULL
		  AND started_at >= ? AND started_at < ?
		GROUP BY energize_activity
		ORDER BY avg_focus DESC
	`

	rows, err := r.db.QueryContext(ctx, query, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to query energize stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var stats []domain.EnergizeStat
	for rows.Next() {
		var s domain.EnergizeStat
		if err := rows.Scan(&s.Activity, &s.SessionCount, &s.AvgFocusScore); err != nil {
			return nil, fmt.Errorf("failed to scan energize stat: %w", err)
		}
		stats = append(stats, s)
	}

	return stats, rows.Err()
}

// GetDeepWorkHours returns total deep work hours for a date range.
func (r *sessionRepository) GetDeepWorkHours(ctx context.Context, start, end time.Time) (time.Duration, error) {
	query := `
		SELECT COALESCE(SUM(duration_ms), 0)
		FROM sessions
		WHERE type = 'work' AND status = 'completed'
		  AND methodology = 'deepwork'
		  AND started_at >= ? AND started_at < ?
	`

	var totalMs int64
	if err := r.db.QueryRowContext(ctx, query, start, end).Scan(&totalMs); err != nil {
		return 0, fmt.Errorf("failed to get deep work hours: %w", err)
	}

	return time.Duration(totalMs) * time.Millisecond, nil
}

// GetDeepWorkDays returns the number of days with at least one deep work session in the range.
func (r *sessionRepository) GetDeepWorkDays(ctx context.Context, start, end time.Time) (int, error) {
	// Extract date from started_at (RFC3339 format: 2006-01-02T15:04:05Z)
	// Use substr to get YYYY-MM-DD part and count distinct dates
	query := `
		SELECT COUNT(DISTINCT substr(started_at, 1, 10))
		FROM sessions
		WHERE type = 'work' AND status = 'completed'
		  AND methodology = 'deepwork'
		  AND started_at >= ? AND started_at < ?
	`

	var days int
	if err := r.db.QueryRowContext(ctx, query, start, end).Scan(&days); err != nil {
		return 0, fmt.Errorf("failed to get deep work days: %w", err)
	}

	return days, nil
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
	var methodology sql.NullString
	var focusScore sql.NullInt64
	var distractionsStr sql.NullString
	var accomplishment sql.NullString
	var intendedOutcome sql.NullString
	var tagsStr sql.NullString
	var energizeActivity sql.NullString
	var shutdownRitualStr sql.NullString
	var outcomeAchieved sql.NullString

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
		&methodology,
		&focusScore,
		&distractionsStr,
		&accomplishment,
		&intendedOutcome,
		&tagsStr,
		&energizeActivity,
		&shutdownRitualStr,
		&outcomeAchieved,
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
	if methodology.Valid && methodology.String != "" {
		session.Methodology = domain.Methodology(methodology.String)
	} else {
		session.Methodology = domain.MethodologyPomodoro
	}
	if focusScore.Valid {
		score := int(focusScore.Int64)
		session.FocusScore = &score
	}
	if distractionsStr.Valid && distractionsStr.String != "" {
		session.Distractions = unmarshalDistractions(distractionsStr.String)
	}
	if accomplishment.Valid {
		session.Accomplishment = accomplishment.String
	}
	if intendedOutcome.Valid {
		session.IntendedOutcome = intendedOutcome.String
	}
	if tagsStr.Valid && tagsStr.String != "" {
		session.Tags = strings.Split(tagsStr.String, ",")
	}
	if energizeActivity.Valid {
		session.EnergizeActivity = energizeActivity.String
	}
	if shutdownRitualStr.Valid && shutdownRitualStr.String != "" {
		var ritual domain.ShutdownRitual
		if err := json.Unmarshal([]byte(shutdownRitualStr.String), &ritual); err == nil {
			session.ShutdownRitual = &ritual
		}
	}
	if outcomeAchieved.Valid {
		session.OutcomeAchieved = outcomeAchieved.String
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
		var methodology sql.NullString
		var focusScore sql.NullInt64
		var distractionsStr sql.NullString
		var accomplishment sql.NullString
		var intendedOutcome sql.NullString
		var tagsStr sql.NullString
		var energizeActivity sql.NullString
		var shutdownRitualStr sql.NullString
		var outcomeAchieved sql.NullString

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
			&methodology,
			&focusScore,
			&distractionsStr,
			&accomplishment,
			&intendedOutcome,
			&tagsStr,
			&energizeActivity,
			&shutdownRitualStr,
			&outcomeAchieved,
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
		if methodology.Valid && methodology.String != "" {
			session.Methodology = domain.Methodology(methodology.String)
		} else {
			session.Methodology = domain.MethodologyPomodoro
		}
		if focusScore.Valid {
			score := int(focusScore.Int64)
			session.FocusScore = &score
		}
		if distractionsStr.Valid && distractionsStr.String != "" {
			session.Distractions = unmarshalDistractions(distractionsStr.String)
		}
		if accomplishment.Valid {
			session.Accomplishment = accomplishment.String
		}
		if intendedOutcome.Valid {
			session.IntendedOutcome = intendedOutcome.String
		}
		if tagsStr.Valid && tagsStr.String != "" {
			session.Tags = strings.Split(tagsStr.String, ",")
		}
		if energizeActivity.Valid {
			session.EnergizeActivity = energizeActivity.String
		}
		if shutdownRitualStr.Valid && shutdownRitualStr.String != "" {
			var ritual domain.ShutdownRitual
			if err := json.Unmarshal([]byte(shutdownRitualStr.String), &ritual); err == nil {
				session.ShutdownRitual = &ritual
			}
		}
		if outcomeAchieved.Valid {
			session.OutcomeAchieved = outcomeAchieved.String
		}

		sessions = append(sessions, &session)
	}

	return sessions, rows.Err()
}

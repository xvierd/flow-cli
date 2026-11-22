package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

// sessionRepository implements ports.SessionRepository using SQLite.
type sessionRepository struct {
	db executor
}

// newSessionRepository creates a new session repository.
func newSessionRepository(db executor) ports.SessionRepository {
	return &sessionRepository{db: db}
}

// Save persists a session to storage.
func (r *sessionRepository) Save(ctx context.Context, session *domain.PomodoroSession) error {
	query := `
		INSERT INTO sessions (
			id, task_id, type, status, duration_ms, started_at, paused_at,
			completed_at, git_branch, git_commit, git_modified, notes,
			methodology, focus_score, distractions, accomplishment, intended_outcome,
			energize_activity, shutdown_ritual, outcome_achieved
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	modified := strings.Join(session.GitModified, ",")
	distractionsJSON, _ := json.Marshal(session.Distractions)
	methodology := string(session.Methodology)
	if methodology == "" {
		methodology = string(domain.MethodologyPomodoro)
	}

	var shutdownRitualJSON []byte
	if session.ShutdownRitual != nil {
		shutdownRitualJSON, _ = json.Marshal(session.ShutdownRitual)
	}

	// The row and its tags commit together.
	return atomically(ctx, r.db, func(ex executor) error {
		_, err := ex.ExecContext(ctx, query,
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
			session.EnergizeActivity,
			nullableString(shutdownRitualJSON),
			session.OutcomeAchieved,
		)

		if err != nil {
			return fmt.Errorf("failed to save session: %w", err)
		}

		return replaceSessionTags(ctx, ex, session.ID, session.Tags)
	})
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
// A unique ID prefix is accepted too.
func (r *sessionRepository) FindByID(ctx context.Context, id string) (*domain.PomodoroSession, error) {
	id, err := resolveID(ctx, r.db, "sessions", id, domain.ErrSessionNotFound)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT
			s.id, s.task_id, s.type, s.status, s.duration_ms, s.started_at, s.paused_at,
			s.completed_at, s.git_branch, s.git_commit, s.git_modified, s.notes,
			s.methodology, s.focus_score, s.distractions, s.accomplishment, s.intended_outcome,
			COALESCE(GROUP_CONCAT(st.tag), ''),
			s.energize_activity, s.shutdown_ritual, s.outcome_achieved, s.git_summary
		FROM sessions s
		LEFT JOIN session_tags st ON st.session_id = s.id
		WHERE s.id = ?
		GROUP BY s.id
	`

	return r.scanSession(r.db.QueryRowContext(ctx, query, id))
}

// FindActive retrieves the currently running or paused session.
func (r *sessionRepository) FindActive(ctx context.Context) (*domain.PomodoroSession, error) {
	query := `
		SELECT
			s.id, s.task_id, s.type, s.status, s.duration_ms, s.started_at, s.paused_at,
			s.completed_at, s.git_branch, s.git_commit, s.git_modified, s.notes,
			s.methodology, s.focus_score, s.distractions, s.accomplishment, s.intended_outcome,
			COALESCE(GROUP_CONCAT(st.tag), ''),
			s.energize_activity, s.shutdown_ritual, s.outcome_achieved, s.git_summary
		FROM sessions s
		LEFT JOIN session_tags st ON st.session_id = s.id
		WHERE s.status IN (?, ?)
		GROUP BY s.id
		ORDER BY s.started_at DESC
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
			s.id, s.task_id, s.type, s.status, s.duration_ms, s.started_at, s.paused_at,
			s.completed_at, s.git_branch, s.git_commit, s.git_modified, s.notes,
			s.methodology, s.focus_score, s.distractions, s.accomplishment, s.intended_outcome,
			COALESCE(GROUP_CONCAT(st.tag), ''),
			s.energize_activity, s.shutdown_ritual, s.outcome_achieved, s.git_summary
		FROM sessions s
		LEFT JOIN session_tags st ON st.session_id = s.id
		WHERE s.started_at >= ?
		GROUP BY s.id
		ORDER BY s.started_at DESC
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
			s.id, s.task_id, s.type, s.status, s.duration_ms, s.started_at, s.paused_at,
			s.completed_at, s.git_branch, s.git_commit, s.git_modified, s.notes,
			s.methodology, s.focus_score, s.distractions, s.accomplishment, s.intended_outcome,
			COALESCE(GROUP_CONCAT(st.tag), ''),
			s.energize_activity, s.shutdown_ritual, s.outcome_achieved, s.git_summary
		FROM sessions s
		LEFT JOIN session_tags st ON st.session_id = s.id
		WHERE s.task_id = ?
		GROUP BY s.id
		ORDER BY s.started_at DESC
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
		    energize_activity = ?, shutdown_ritual = ?, outcome_achieved = ?
		WHERE id = ?
	`

	modified := strings.Join(session.GitModified, ",")
	distractionsJSON, _ := json.Marshal(session.Distractions)
	methodology := string(session.Methodology)
	if methodology == "" {
		methodology = string(domain.MethodologyPomodoro)
	}

	var shutdownRitualJSON []byte
	if session.ShutdownRitual != nil {
		shutdownRitualJSON, _ = json.Marshal(session.ShutdownRitual)
	}

	// The row and its tags commit together.
	return atomically(ctx, r.db, func(ex executor) error {
		result, err := ex.ExecContext(ctx, query,
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
			return fmt.Errorf("session not found: %s: %w", session.ID, domain.ErrSessionNotFound)
		}

		return replaceSessionTags(ctx, ex, session.ID, session.Tags)
	})
}

// UpdateGitSummary stores the git summary of a session. Session Update leaves
// the column alone, so a summary written after the session ended survives
// later updates (focus score, shutdown ritual, …).
func (r *sessionRepository) UpdateGitSummary(ctx context.Context, sessionID string, summary *domain.GitSummary) error {
	var value *string
	if summary != nil {
		data, err := json.Marshal(summary)
		if err != nil {
			return fmt.Errorf("failed to encode git summary: %w", err)
		}
		value = nullableString(data)
	}
	result, err := r.db.ExecContext(ctx, "UPDATE sessions SET git_summary = ? WHERE id = ?", value, sessionID)
	if err != nil {
		return fmt.Errorf("failed to update git summary: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return fmt.Errorf("session not found: %s: %w", sessionID, domain.ErrSessionNotFound)
	}
	return nil
}

// unmarshalGitSummary decodes the git_summary column; NULL, empty or
// unreadable JSON yields nil (no summary).
func unmarshalGitSummary(col sql.NullString) *domain.GitSummary {
	if !col.Valid || col.String == "" {
		return nil
	}
	var summary domain.GitSummary
	if err := json.Unmarshal([]byte(col.String), &summary); err != nil {
		return nil
	}
	return &summary
}

// GetDailyStats returns aggregated session statistics for a specific date.
// Session data only: TasksCompleted is populated by the service layer.
func (r *sessionRepository) GetDailyStats(ctx context.Context, date time.Time) (*domain.DailyStats, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	// Calendar day, not 24h: DST days are 23 or 25 hours long.
	endOfDay := startOfDay.AddDate(0, 0, 1)

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
	// Distractions are stored as JSON arrays (new format), JSON 'null' (no
	// distractions) or newline-separated strings (legacy, which may start
	// with '[' without being JSON). The nested CASE keeps json_type and
	// json_array_length away from invalid JSON, which would raise an error.
	distractQuery := `
		SELECT COALESCE(SUM(
			CASE
				WHEN distractions IS NULL OR distractions = '' OR distractions = 'null' THEN 0
				WHEN json_valid(distractions) THEN
					CASE json_type(distractions)
						WHEN 'array' THEN json_array_length(distractions)
						ELSE LENGTH(distractions) - LENGTH(REPLACE(distractions, CHAR(10), '')) + 1
					END
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

	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -365)

	// Load deep work sessions in a single query and aggregate by day in Go.
	query := `
		SELECT started_at, duration_ms
		FROM sessions
		WHERE type = 'work' AND status = 'completed'
		  AND methodology = 'deepwork'
		  AND started_at >= ?
	`

	rows, err := r.db.QueryContext(ctx, query, start)
	if err != nil {
		return 0, fmt.Errorf("failed to query deep work streak: %w", err)
	}
	defer func() { _ = rows.Close() }()

	daySums := make(map[string]int64)
	for rows.Next() {
		var startedAt time.Time
		var durationMs int64
		if err := rows.Scan(&startedAt, &durationMs); err != nil {
			return 0, fmt.Errorf("failed to scan deep work streak: %w", err)
		}
		daySums[startedAt.Format("2006-01-02")] += durationMs
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("failed to iterate deep work streak: %w", err)
	}

	streak := 0
	for i := 0; i < 366; i++ {
		day := now.AddDate(0, 0, -i)
		if daySums[day.Format("2006-01-02")] >= thresholdMs {
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
// Hours are aggregated using the session's local time (consistent with the report heatmap).
func (r *sessionRepository) GetHourlyProductivity(ctx context.Context, days int) (map[int]time.Duration, error) {
	since := time.Now().AddDate(0, 0, -days)

	query := `
		SELECT started_at, duration_ms
		FROM sessions
		WHERE type = 'work' AND status = 'completed'
		  AND started_at >= ?
	`

	rows, err := r.db.QueryContext(ctx, query, since)
	if err != nil {
		return nil, fmt.Errorf("failed to query hourly productivity: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int]time.Duration)
	for rows.Next() {
		var startedAt time.Time
		var totalMs int64
		if err := rows.Scan(&startedAt, &totalMs); err != nil {
			return nil, fmt.Errorf("failed to scan hourly productivity: %w", err)
		}
		result[startedAt.Local().Hour()] += time.Duration(totalMs) * time.Millisecond
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

// GetTagStats returns aggregated tag statistics for work sessions in a time range.
// Tags live in the session_tags table (normalized at write time); a session
// counts fully toward each of its tags. Only focus scores > 0 feed averages.
func (r *sessionRepository) GetTagStats(ctx context.Context, start, end time.Time) ([]domain.TagStat, error) {
	query := `
		SELECT
			st.tag,
			COUNT(*) as session_count,
			COALESCE(SUM(s.duration_ms), 0) as total_ms,
			COALESCE(SUM(CASE WHEN s.focus_score > 0 THEN 1 ELSE 0 END), 0) as focus_count,
			COALESCE(SUM(CASE WHEN s.focus_score > 0 THEN s.focus_score ELSE 0 END), 0) as focus_sum
		FROM session_tags st
		JOIN sessions s ON s.id = st.session_id
		WHERE s.type = 'work' AND s.status = 'completed'
		  AND s.started_at >= ? AND s.started_at < ?
		GROUP BY st.tag
	`

	rows, err := r.db.QueryContext(ctx, query, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to query tag stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make([]domain.TagStat, 0)
	for rows.Next() {
		var stat domain.TagStat
		var totalMs int64
		var focusSum int64
		if err := rows.Scan(&stat.Tag, &stat.SessionCount, &totalMs, &stat.FocusScoreCount, &focusSum); err != nil {
			return nil, fmt.Errorf("failed to scan tag stats: %w", err)
		}
		stat.TotalTime = time.Duration(totalMs) * time.Millisecond
		if stat.FocusScoreCount > 0 {
			stat.AvgFocusScore = float64(focusSum) / float64(stat.FocusScoreCount)
		}
		result = append(result, stat)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sortTags(result)
	return result, nil
}

// sortTags orders tag stats by total time, descending.
func sortTags(stats []domain.TagStat) {
	sort.Slice(stats, func(i, j int) bool { return stats[i].TotalTime > stats[j].TotalTime })
}

// scanSession scans a single session row.
func (r *sessionRepository) scanSession(row *sql.Row) (*domain.PomodoroSession, error) {
	var session domain.PomodoroSession
	var taskID sql.NullString
	var pausedAt sql.NullTime
	var completedAt sql.NullTime
	var durationMs int64
	var gitBranch, gitCommit, modifiedStr sql.NullString
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
	var gitSummaryStr sql.NullString

	err := row.Scan(
		&session.ID,
		&taskID,
		&session.Type,
		&session.Status,
		&durationMs,
		&session.StartedAt,
		&pausedAt,
		&completedAt,
		&gitBranch,
		&gitCommit,
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
		&gitSummaryStr,
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
	// The git columns are NULL on rows written by very old versions.
	session.GitBranch = gitBranch.String
	session.GitCommit = gitCommit.String
	if modifiedStr.String != "" {
		session.GitModified = strings.Split(modifiedStr.String, ",")
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
	session.GitSummary = unmarshalGitSummary(gitSummaryStr)

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
		var gitBranch, gitCommit, modifiedStr sql.NullString
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
		var gitSummaryStr sql.NullString

		err := rows.Scan(
			&session.ID,
			&taskID,
			&session.Type,
			&session.Status,
			&durationMs,
			&session.StartedAt,
			&pausedAt,
			&completedAt,
			&gitBranch,
			&gitCommit,
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
			&gitSummaryStr,
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
		// The git columns are NULL on rows written by very old versions.
		session.GitBranch = gitBranch.String
		session.GitCommit = gitCommit.String
		if modifiedStr.String != "" {
			session.GitModified = strings.Split(modifiedStr.String, ",")
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
		session.GitSummary = unmarshalGitSummary(gitSummaryStr)

		sessions = append(sessions, &session)
	}

	return sessions, rows.Err()
}

// GetInsightSessions returns the finished work sessions started in
// [start, end) for the insight detectors, oldest first. StartedAt is
// converted to local time so hour-of-day and weekday buckets match the
// user's clock, not UTC. Tags are the union of session and task tags.
func (r *sessionRepository) GetInsightSessions(ctx context.Context, start, end time.Time) ([]domain.InsightSession, error) {
	query := `
		SELECT
			s.id, COALESCE(s.task_id, ''), COALESCE(t.status, ''), s.status,
			COALESCE(s.methodology, ''), s.started_at, s.duration_ms,
			s.focus_score, COALESCE(s.distractions, ''),
			COALESCE((SELECT GROUP_CONCAT(tag) FROM session_tags WHERE session_id = s.id), ''),
			COALESCE((SELECT GROUP_CONCAT(tag) FROM task_tags WHERE task_id = s.task_id), '')
		FROM sessions s
		LEFT JOIN tasks t ON t.id = s.task_id
		WHERE s.type = 'work'
		  AND s.status IN ('completed', 'interrupted', 'cancelled')
		  AND s.started_at >= ? AND s.started_at < ?
		ORDER BY s.started_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to query insight sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make([]domain.InsightSession, 0)
	for rows.Next() {
		var is domain.InsightSession
		var taskStatus, status, methodology, distractions, sessionTags, taskTags string
		var durationMs int64
		var focus sql.NullInt64
		if err := rows.Scan(&is.ID, &is.TaskID, &taskStatus, &status, &methodology,
			&is.StartedAt, &durationMs, &focus, &distractions, &sessionTags, &taskTags); err != nil {
			return nil, fmt.Errorf("failed to scan insight session: %w", err)
		}
		is.TaskDone = taskStatus == string(domain.StatusCompleted)
		is.Status = domain.SessionStatus(status)
		is.Methodology = domain.Methodology(methodology)
		if is.Methodology == "" {
			is.Methodology = domain.MethodologyPomodoro
		}
		is.StartedAt = is.StartedAt.Local()
		is.Duration = time.Duration(durationMs) * time.Millisecond
		if focus.Valid && focus.Int64 > 0 {
			v := int(focus.Int64)
			is.FocusScore = &v
		}
		is.Distractions = unmarshalDistractions(distractions)
		is.Tags = mergeTagLists(sessionTags, taskTags)
		result = append(result, is)
	}
	return result, rows.Err()
}

// mergeTagLists unions comma-joined tag lists, keeping first-seen order.
func mergeTagLists(lists ...string) []string {
	var tags []string
	seen := map[string]bool{}
	for _, list := range lists {
		for _, tag := range strings.Split(list, ",") {
			if tag == "" || seen[tag] {
				continue
			}
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	return tags
}

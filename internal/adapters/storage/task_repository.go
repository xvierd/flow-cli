package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/sahilm/fuzzy"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

// taskRepository implements ports.TaskRepository using SQLite.
type taskRepository struct {
	db executor
}

// newTaskRepository creates a new task repository.
func newTaskRepository(db executor) ports.TaskRepository {
	return &taskRepository{db: db}
}

// Save persists a task to storage.
func (r *taskRepository) Save(ctx context.Context, task *domain.Task) error {
	query := `
		INSERT INTO tasks (id, title, description, status, created_at, updated_at, completed_at, highlight_date)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(ctx, query,
		task.ID,
		task.Title,
		task.Description,
		string(task.Status),
		task.CreatedAt,
		task.UpdatedAt,
		task.CompletedAt,
		task.HighlightDate,
	)

	if err != nil {
		return fmt.Errorf("failed to save task: %w", err)
	}

	return replaceTaskTags(ctx, r.db, task.ID, task.Tags)
}

// FindByID retrieves a task by its unique identifier.
func (r *taskRepository) FindByID(ctx context.Context, id string) (*domain.Task, error) {
	query := `
		SELECT t.id, t.title, t.description, t.status, COALESCE(GROUP_CONCAT(tt.tag), ''), t.created_at, t.updated_at, t.completed_at, t.highlight_date
		FROM tasks t
		LEFT JOIN task_tags tt ON tt.task_id = t.id
		WHERE t.id = ?
		GROUP BY t.id
	`

	var task domain.Task
	var tagsStr string
	var completedAt sql.NullTime
	var highlightDate sql.NullTime

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&task.ID,
		&task.Title,
		&task.Description,
		&task.Status,
		&tagsStr,
		&task.CreatedAt,
		&task.UpdatedAt,
		&completedAt,
		&highlightDate,
	)

	if err == sql.ErrNoRows {
		return nil, domain.ErrTaskNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find task: %w", err)
	}

	if completedAt.Valid {
		task.CompletedAt = &completedAt.Time
	}
	if highlightDate.Valid {
		task.HighlightDate = &highlightDate.Time
	}

	if tagsStr != "" {
		task.Tags = strings.Split(tagsStr, ",")
	}

	return &task, nil
}

// FindAll retrieves all tasks, optionally filtered by status.
func (r *taskRepository) FindAll(ctx context.Context, status *domain.TaskStatus) ([]*domain.Task, error) {
	var query string
	var args []interface{}

	if status != nil {
		query = `
			SELECT t.id, t.title, t.description, t.status, COALESCE(GROUP_CONCAT(tt.tag), ''), t.created_at, t.updated_at, t.completed_at, t.highlight_date
			FROM tasks t
			LEFT JOIN task_tags tt ON tt.task_id = t.id
			WHERE t.status = ?
			GROUP BY t.id
			ORDER BY t.created_at DESC
		`
		args = append(args, string(*status))
	} else {
		query = `
			SELECT t.id, t.title, t.description, t.status, COALESCE(GROUP_CONCAT(tt.tag), ''), t.created_at, t.updated_at, t.completed_at, t.highlight_date
			FROM tasks t
			LEFT JOIN task_tags tt ON tt.task_id = t.id
			GROUP BY t.id
			ORDER BY t.created_at DESC
		`
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanTasks(rows)
}

// FindPending returns all tasks that are not completed or cancelled.
func (r *taskRepository) FindPending(ctx context.Context) ([]*domain.Task, error) {
	query := `
		SELECT t.id, t.title, t.description, t.status, COALESCE(GROUP_CONCAT(tt.tag), ''), t.created_at, t.updated_at, t.completed_at, t.highlight_date
		FROM tasks t
		LEFT JOIN task_tags tt ON tt.task_id = t.id
		WHERE t.status NOT IN (?, ?)
		GROUP BY t.id
		ORDER BY
			CASE t.status
				WHEN 'in_progress' THEN 0
				WHEN 'pending' THEN 1
				ELSE 2
			END,
			t.updated_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, string(domain.StatusCompleted), string(domain.StatusCancelled))
	if err != nil {
		return nil, fmt.Errorf("failed to query pending tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanTasks(rows)
}

// FindActive returns the currently active task (in_progress).
func (r *taskRepository) FindActive(ctx context.Context) (*domain.Task, error) {
	query := `
		SELECT t.id, t.title, t.description, t.status, COALESCE(GROUP_CONCAT(tt.tag), ''), t.created_at, t.updated_at, t.completed_at, t.highlight_date
		FROM tasks t
		LEFT JOIN task_tags tt ON tt.task_id = t.id
		WHERE t.status = ?
		GROUP BY t.id
		ORDER BY t.updated_at DESC
		LIMIT 1
	`

	var task domain.Task
	var tagsStr string
	var completedAt sql.NullTime
	var highlightDate sql.NullTime

	err := r.db.QueryRowContext(ctx, query, string(domain.StatusInProgress)).Scan(
		&task.ID,
		&task.Title,
		&task.Description,
		&task.Status,
		&tagsStr,
		&task.CreatedAt,
		&task.UpdatedAt,
		&completedAt,
		&highlightDate,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find active task: %w", err)
	}

	if completedAt.Valid {
		task.CompletedAt = &completedAt.Time
	}
	if highlightDate.Valid {
		task.HighlightDate = &highlightDate.Time
	}

	// Initialize tags as empty slice to avoid null in JSON
	task.Tags = []string{}
	if tagsStr != "" {
		task.Tags = strings.Split(tagsStr, ",")
	}

	return &task, nil
}

// FindByTitle does a fuzzy search for tasks by title.
func (r *taskRepository) FindByTitle(ctx context.Context, query string) ([]*domain.Task, error) {
	// First get all tasks
	tasks, err := r.FindAll(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get tasks for fuzzy search: %w", err)
	}

	// Prepare titles for fuzzy search
	titles := make([]string, len(tasks))
	for i, task := range tasks {
		titles[i] = task.Title
	}

	// Perform fuzzy search
	matches := fuzzy.Find(query, titles)

	// Collect matching tasks
	var result []*domain.Task
	for _, match := range matches {
		if match.Score > 0 {
			result = append(result, tasks[match.Index])
		}
	}

	return result, nil
}

// Delete removes a task from storage.
func (r *taskRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM tasks WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return domain.ErrTaskNotFound
	}

	return nil
}

// Update modifies an existing task.
func (r *taskRepository) Update(ctx context.Context, task *domain.Task) error {
	query := `
		UPDATE tasks
		SET title = ?, description = ?, status = ?, updated_at = ?, completed_at = ?, highlight_date = ?
		WHERE id = ?
	`

	task.UpdatedAt = time.Now()

	result, err := r.db.ExecContext(ctx, query,
		task.Title,
		task.Description,
		string(task.Status),
		task.UpdatedAt,
		task.CompletedAt,
		task.HighlightDate,
		task.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return domain.ErrTaskNotFound
	}

	return replaceTaskTags(ctx, r.db, task.ID, task.Tags)
}

// CountCompleted returns how many tasks were completed on the given date.
func (r *taskRepository) CountCompleted(ctx context.Context, date time.Time) (int, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM tasks
		WHERE status = ? AND completed_at >= ? AND completed_at < ?
	`, string(domain.StatusCompleted), startOfDay, endOfDay).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count completed tasks: %w", err)
	}

	return count, nil
}

// scanTasks scans multiple task rows.
func (r *taskRepository) scanTasks(rows *sql.Rows) ([]*domain.Task, error) {
	var tasks []*domain.Task

	for rows.Next() {
		var task domain.Task
		var tagsStr string
		var completedAt sql.NullTime
		var highlightDate sql.NullTime

		err := rows.Scan(
			&task.ID,
			&task.Title,
			&task.Description,
			&task.Status,
			&tagsStr,
			&task.CreatedAt,
			&task.UpdatedAt,
			&completedAt,
			&highlightDate,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan task: %w", err)
		}

		if completedAt.Valid {
			task.CompletedAt = &completedAt.Time
		}
		if highlightDate.Valid {
			task.HighlightDate = &highlightDate.Time
		}

		// Initialize tags as empty slice to avoid null in JSON
		task.Tags = []string{}
		if tagsStr != "" {
			task.Tags = strings.Split(tagsStr, ",")
		}

		tasks = append(tasks, &task)
	}

	return tasks, rows.Err()
}

// FindRecentTasks returns the last N distinct tasks that had sessions,
// ordered by most recent session start time.
func (r *taskRepository) FindRecentTasks(ctx context.Context, limit int) ([]*domain.Task, error) {
	query := `
		SELECT t.id, t.title, t.description, t.status, COALESCE(GROUP_CONCAT(tt.tag), ''), t.created_at, t.updated_at, t.completed_at, t.highlight_date
		FROM tasks t
		INNER JOIN (
			SELECT task_id, MAX(started_at) AS last_session
			FROM sessions
			WHERE task_id IS NOT NULL
			GROUP BY task_id
		) s ON t.id = s.task_id
		LEFT JOIN task_tags tt ON tt.task_id = t.id
		GROUP BY t.id
		ORDER BY s.last_session DESC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanTasks(rows)
}

// FindTodayHighlight returns the task marked as today's highlight, if any.
func (r *taskRepository) FindTodayHighlight(ctx context.Context, date time.Time) (*domain.Task, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	query := `
		SELECT t.id, t.title, t.description, t.status, COALESCE(GROUP_CONCAT(tt.tag), ''), t.created_at, t.updated_at, t.completed_at, t.highlight_date
		FROM tasks t
		LEFT JOIN task_tags tt ON tt.task_id = t.id
		WHERE t.highlight_date >= ? AND t.highlight_date < ?
		GROUP BY t.id
		ORDER BY t.updated_at DESC
		LIMIT 1
	`

	var task domain.Task
	var tagsStr string
	var completedAt sql.NullTime
	var highlightDate sql.NullTime

	err := r.db.QueryRowContext(ctx, query, startOfDay, endOfDay).Scan(
		&task.ID,
		&task.Title,
		&task.Description,
		&task.Status,
		&tagsStr,
		&task.CreatedAt,
		&task.UpdatedAt,
		&completedAt,
		&highlightDate,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find today's highlight: %w", err)
	}

	if completedAt.Valid {
		task.CompletedAt = &completedAt.Time
	}
	if highlightDate.Valid {
		task.HighlightDate = &highlightDate.Time
	}

	task.Tags = []string{}
	if tagsStr != "" {
		task.Tags = strings.Split(tagsStr, ",")
	}

	return &task, nil
}

// FindYesterdayHighlight returns yesterday's highlight task if it wasn't completed.
func (r *taskRepository) FindYesterdayHighlight(ctx context.Context, today time.Time) (*domain.Task, error) {
	yesterday := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location()).Add(-24 * time.Hour)
	endOfYesterday := yesterday.Add(24 * time.Hour)

	query := `
		SELECT t.id, t.title, t.description, t.status, COALESCE(GROUP_CONCAT(tt.tag), ''), t.created_at, t.updated_at, t.completed_at, t.highlight_date
		FROM tasks t
		LEFT JOIN task_tags tt ON tt.task_id = t.id
		WHERE t.highlight_date >= ? AND t.highlight_date < ?
		  AND t.status != ?
		GROUP BY t.id
		ORDER BY t.updated_at DESC
		LIMIT 1
	`

	var task domain.Task
	var tagsStr string
	var completedAt sql.NullTime
	var highlightDate sql.NullTime

	err := r.db.QueryRowContext(ctx, query, yesterday, endOfYesterday, string(domain.StatusCompleted)).Scan(
		&task.ID,
		&task.Title,
		&task.Description,
		&task.Status,
		&tagsStr,
		&task.CreatedAt,
		&task.UpdatedAt,
		&completedAt,
		&highlightDate,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find yesterday's highlight: %w", err)
	}

	if completedAt.Valid {
		task.CompletedAt = &completedAt.Time
	}
	if highlightDate.Valid {
		task.HighlightDate = &highlightDate.Time
	}

	task.Tags = []string{}
	if tagsStr != "" {
		task.Tags = strings.Split(tagsStr, ",")
	}

	return &task, nil
}

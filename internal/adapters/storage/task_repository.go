package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
	"github.com/sahilm/fuzzy"
)

// taskRepository implements ports.TaskRepository using SQLite.
type taskRepository struct {
	db *sql.DB
}

// newTaskRepository creates a new task repository.
func newTaskRepository(db *sql.DB) ports.TaskRepository {
	return &taskRepository{db: db}
}

// Save persists a task to storage.
func (r *taskRepository) Save(ctx context.Context, task *domain.Task) error {
	query := `
		INSERT INTO tasks (id, title, description, status, tags, created_at, updated_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	tags := strings.Join(task.Tags, ",")

	_, err := r.db.ExecContext(ctx, query,
		task.ID,
		task.Title,
		task.Description,
		string(task.Status),
		tags,
		task.CreatedAt,
		task.UpdatedAt,
		task.CompletedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save task: %w", err)
	}

	return nil
}

// FindByID retrieves a task by its unique identifier.
func (r *taskRepository) FindByID(ctx context.Context, id string) (*domain.Task, error) {
	query := `
		SELECT id, title, description, status, tags, created_at, updated_at, completed_at
		FROM tasks
		WHERE id = ?
	`

	var task domain.Task
	var tagsStr string
	var completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&task.ID,
		&task.Title,
		&task.Description,
		&task.Status,
		&tagsStr,
		&task.CreatedAt,
		&task.UpdatedAt,
		&completedAt,
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
			SELECT id, title, description, status, tags, created_at, updated_at, completed_at
			FROM tasks
			WHERE status = ?
			ORDER BY created_at DESC
		`
		args = append(args, string(*status))
	} else {
		query = `
			SELECT id, title, description, status, tags, created_at, updated_at, completed_at
			FROM tasks
			ORDER BY created_at DESC
		`
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query tasks: %w", err)
	}
	defer rows.Close()

	return r.scanTasks(rows)
}

// FindPending returns all tasks that are not completed or cancelled.
func (r *taskRepository) FindPending(ctx context.Context) ([]*domain.Task, error) {
	query := `
		SELECT id, title, description, status, tags, created_at, updated_at, completed_at
		FROM tasks
		WHERE status NOT IN (?, ?)
		ORDER BY 
			CASE status
				WHEN 'in_progress' THEN 0
				WHEN 'pending' THEN 1
				ELSE 2
			END,
			updated_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, string(domain.StatusCompleted), string(domain.StatusCancelled))
	if err != nil {
		return nil, fmt.Errorf("failed to query pending tasks: %w", err)
	}
	defer rows.Close()

	return r.scanTasks(rows)
}

// FindActive returns the currently active task (in_progress).
func (r *taskRepository) FindActive(ctx context.Context) (*domain.Task, error) {
	query := `
		SELECT id, title, description, status, tags, created_at, updated_at, completed_at
		FROM tasks
		WHERE status = ?
		ORDER BY updated_at DESC
		LIMIT 1
	`

	var task domain.Task
	var tagsStr string
	var completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, string(domain.StatusInProgress)).Scan(
		&task.ID,
		&task.Title,
		&task.Description,
		&task.Status,
		&tagsStr,
		&task.CreatedAt,
		&task.UpdatedAt,
		&completedAt,
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
		SET title = ?, description = ?, status = ?, tags = ?, updated_at = ?, completed_at = ?
		WHERE id = ?
	`

	tags := strings.Join(task.Tags, ",")
	task.UpdatedAt = time.Now()

	result, err := r.db.ExecContext(ctx, query,
		task.Title,
		task.Description,
		string(task.Status),
		tags,
		task.UpdatedAt,
		task.CompletedAt,
		task.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return domain.ErrTaskNotFound
	}

	return nil
}

// scanTasks scans multiple task rows.
func (r *taskRepository) scanTasks(rows *sql.Rows) ([]*domain.Task, error) {
	var tasks []*domain.Task

	for rows.Next() {
		var task domain.Task
		var tagsStr string
		var completedAt sql.NullTime

		err := rows.Scan(
			&task.ID,
			&task.Title,
			&task.Description,
			&task.Status,
			&tagsStr,
			&task.CreatedAt,
			&task.UpdatedAt,
			&completedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan task: %w", err)
		}

		if completedAt.Valid {
			task.CompletedAt = &completedAt.Time
		}

		if tagsStr != "" {
			task.Tags = strings.Split(tagsStr, ",")
		}

		tasks = append(tasks, &task)
	}

	return tasks, rows.Err()
}

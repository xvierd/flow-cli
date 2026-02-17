// Package domain contains the core business entities for Flow.
// These entities represent the fundamental concepts of the task tracking system
// and are independent of any external frameworks or infrastructure.
package domain

import (
	"errors"
	"time"
)

// Common domain errors.
var (
	ErrInvalidTaskID        = errors.New("invalid task ID")
	ErrEmptyTaskTitle       = errors.New("task title cannot be empty")
	ErrTaskNotFound         = errors.New("task not found")
	ErrInvalidDuration      = errors.New("invalid duration")
	ErrSessionAlreadyActive = errors.New("session already active")
	ErrNoActiveSession      = errors.New("no active session")
)

// TaskStatus represents the current state of a task.
type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"
	StatusInProgress TaskStatus = "in_progress"
	StatusCompleted  TaskStatus = "completed"
	StatusCancelled  TaskStatus = "cancelled"
)

// Task represents a unit of work to be tracked.
type Task struct {
	ID            string
	Title         string
	Description   string
	Status        TaskStatus
	Tags          []string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	CompletedAt   *time.Time
	HighlightDate *time.Time
}

// SetAsHighlight marks this task as today's highlight.
func (t *Task) SetAsHighlight() {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	t.HighlightDate = &today
	t.UpdatedAt = now
}

// IsHighlightForDate returns true if this task is the highlight for the given date.
func (t *Task) IsHighlightForDate(date time.Time) bool {
	if t.HighlightDate == nil {
		return false
	}
	d := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	h := time.Date(t.HighlightDate.Year(), t.HighlightDate.Month(), t.HighlightDate.Day(), 0, 0, 0, 0, t.HighlightDate.Location())
	return d.Equal(h)
}

// NewTask creates a new task with the given title.
func NewTask(title string) (*Task, error) {
	if err := validateTaskTitle(title); err != nil {
		return nil, err
	}

	now := time.Now()
	return &Task{
		ID:        generateID(),
		Title:     title,
		Status:    StatusPending,
		Tags:      []string{},
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// validateTaskTitle ensures the title is not empty.
func validateTaskTitle(title string) error {
	if title == "" {
		return ErrEmptyTaskTitle
	}
	return nil
}

// Start marks the task as in progress.
func (t *Task) Start() {
	t.Status = StatusInProgress
	t.UpdatedAt = time.Now()
}

// Complete marks the task as completed.
func (t *Task) Complete() {
	now := time.Now()
	t.Status = StatusCompleted
	t.CompletedAt = &now
	t.UpdatedAt = now
}

// Cancel marks the task as cancelled.
func (t *Task) Cancel() {
	t.Status = StatusCancelled
	t.UpdatedAt = time.Now()
}

// AddTag adds a tag to the task.
func (t *Task) AddTag(tag string) {
	for _, existing := range t.Tags {
		if existing == tag {
			return
		}
	}
	t.Tags = append(t.Tags, tag)
	t.UpdatedAt = time.Now()
}

// IsActive returns true if the task is currently being worked on.
func (t *Task) IsActive() bool {
	return t.Status == StatusInProgress
}

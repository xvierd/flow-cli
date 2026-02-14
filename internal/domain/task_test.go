package domain

import (
	"testing"
	"time"
)

func TestNewTask(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		wantErr     bool
		errExpected error
	}{
		{
			name:    "valid task",
			title:   "Implement feature X",
			wantErr: false,
		},
		{
			name:        "empty title",
			title:       "",
			wantErr:     true,
			errExpected: ErrEmptyTaskTitle,
		},
		{
			name:    "title with spaces",
			title:   "   Valid Title   ",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, err := NewTask(tt.title)

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewTask() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.errExpected != nil && err != tt.errExpected {
					t.Errorf("NewTask() error = %v, want %v", err, tt.errExpected)
				}
				return
			}

			if err != nil {
				t.Errorf("NewTask() unexpected error = %v", err)
				return
			}

			if task == nil {
				t.Error("NewTask() returned nil task")
				return
			}

			if task.Title != tt.title {
				t.Errorf("NewTask() title = %v, want %v", task.Title, tt.title)
			}

			if task.Status != StatusPending {
				t.Errorf("NewTask() status = %v, want %v", task.Status, StatusPending)
			}

			if task.ID == "" {
				t.Error("NewTask() ID is empty")
			}

			if task.CreatedAt.IsZero() {
				t.Error("NewTask() CreatedAt is zero")
			}
		})
	}
}

func TestTask_Start(t *testing.T) {
	task, _ := NewTask("Test task")
	originalUpdate := task.UpdatedAt

	time.Sleep(10 * time.Millisecond)
	task.Start()

	if task.Status != StatusInProgress {
		t.Errorf("Start() status = %v, want %v", task.Status, StatusInProgress)
	}

	if !task.UpdatedAt.After(originalUpdate) {
		t.Error("Start() should update UpdatedAt")
	}
}

func TestTask_Complete(t *testing.T) {
	task, _ := NewTask("Test task")

	task.Complete()

	if task.Status != StatusCompleted {
		t.Errorf("Complete() status = %v, want %v", task.Status, StatusCompleted)
	}

	if task.CompletedAt == nil {
		t.Error("Complete() CompletedAt should not be nil")
	}
}

func TestTask_Cancel(t *testing.T) {
	task, _ := NewTask("Test task")

	task.Cancel()

	if task.Status != StatusCancelled {
		t.Errorf("Cancel() status = %v, want %v", task.Status, StatusCancelled)
	}
}

func TestTask_AddTag(t *testing.T) {
	tests := []struct {
		name     string
		tags     []string
		newTag   string
		expected int
	}{
		{
			name:     "add first tag",
			tags:     []string{},
			newTag:   "urgent",
			expected: 1,
		},
		{
			name:     "add unique tag",
			tags:     []string{"work"},
			newTag:   "urgent",
			expected: 2,
		},
		{
			name:     "add duplicate tag",
			tags:     []string{"work", "urgent"},
			newTag:   "urgent",
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, _ := NewTask("Test")
			task.Tags = tt.tags

			task.AddTag(tt.newTag)

			if len(task.Tags) != tt.expected {
				t.Errorf("AddTag() len(tags) = %v, want %v", len(task.Tags), tt.expected)
			}
		})
	}
}

func TestTask_IsActive(t *testing.T) {
	tests := []struct {
		name   string
		status TaskStatus
		want   bool
	}{
		{
			name:   "in_progress is active",
			status: StatusInProgress,
			want:   true,
		},
		{
			name:   "pending is not active",
			status: StatusPending,
			want:   false,
		},
		{
			name:   "completed is not active",
			status: StatusCompleted,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, _ := NewTask("Test")
			task.Status = tt.status

			if got := task.IsActive(); got != tt.want {
				t.Errorf("IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

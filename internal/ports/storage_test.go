package ports

import (
	"context"
	"errors"
	"testing"

	"github.com/xavier/flow/internal/domain"
)

// Mock implementations for testing interfaces.

type mockTaskRepository struct {
	tasks map[string]*domain.Task
}

func (m *mockTaskRepository) Save(ctx context.Context, task *domain.Task) error {
	m.tasks[task.ID] = task
	return nil
}

func (m *mockTaskRepository) FindByID(ctx context.Context, id string) (*domain.Task, error) {
	task, ok := m.tasks[id]
	if !ok {
		return nil, domain.ErrTaskNotFound
	}
	return task, nil
}

func (m *mockTaskRepository) FindAll(ctx context.Context, status *domain.TaskStatus) ([]*domain.Task, error) {
	var result []*domain.Task
	for _, task := range m.tasks {
		if status == nil || task.Status == *status {
			result = append(result, task)
		}
	}
	return result, nil
}

func (m *mockTaskRepository) FindPending(ctx context.Context) ([]*domain.Task, error) {
	var result []*domain.Task
	for _, task := range m.tasks {
		if task.Status != domain.StatusCompleted && task.Status != domain.StatusCancelled {
			result = append(result, task)
		}
	}
	return result, nil
}

func (m *mockTaskRepository) FindActive(ctx context.Context) (*domain.Task, error) {
	for _, task := range m.tasks {
		if task.Status == domain.StatusInProgress {
			return task, nil
		}
	}
	return nil, nil
}

func (m *mockTaskRepository) Delete(ctx context.Context, id string) error {
	delete(m.tasks, id)
	return nil
}

func (m *mockTaskRepository) Update(ctx context.Context, task *domain.Task) error {
	m.tasks[task.ID] = task
	return nil
}

func TestMockTaskRepository(t *testing.T) {
	repo := &mockTaskRepository{tasks: make(map[string]*domain.Task)}
	ctx := context.Background()

	t.Run("save and find task", func(t *testing.T) {
		task, _ := domain.NewTask("Test task")
		err := repo.Save(ctx, task)
		if err != nil {
			t.Errorf("Save() error = %v", err)
		}

		found, err := repo.FindByID(ctx, task.ID)
		if err != nil {
			t.Errorf("FindByID() error = %v", err)
		}
		if found.Title != task.Title {
			t.Errorf("Found task title = %v, want %v", found.Title, task.Title)
		}
	})

	t.Run("find non-existent task", func(t *testing.T) {
		_, err := repo.FindByID(ctx, "non-existent")
		if !errors.Is(err, domain.ErrTaskNotFound) {
			t.Errorf("FindByID() error = %v, want ErrTaskNotFound", err)
		}
	})

	t.Run("find all tasks", func(t *testing.T) {
		tasks, err := repo.FindAll(ctx, nil)
		if err != nil {
			t.Errorf("FindAll() error = %v", err)
		}
		if len(tasks) != 1 {
			t.Errorf("FindAll() returned %d tasks, want 1", len(tasks))
		}
	})
}

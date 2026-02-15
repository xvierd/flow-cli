package services

import (
	"context"
	"testing"

	"github.com/xvierd/flow-cli/internal/adapters/storage"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

func setupTestStorage(t *testing.T) (ports.Storage, func()) {
	store, err := storage.NewMemory()
	if err != nil {
		t.Fatalf("Failed to create test storage: %v", err)
	}
	return store, func() { store.Close() }
}

func TestTaskService_AddTask(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	service := NewTaskService(store)
	ctx := context.Background()

	t.Run("add valid task", func(t *testing.T) {
		req := AddTaskRequest{
			Title:       "Test Task",
			Description: "A test task",
			Tags:        []string{"test", "example"},
		}

		task, err := service.AddTask(ctx, req)
		if err != nil {
			t.Errorf("AddTask() error = %v", err)
		}
		if task == nil {
			t.Fatal("AddTask() returned nil")
		}
		if task.Title != req.Title {
			t.Errorf("AddTask() title = %v, want %v", task.Title, req.Title)
		}
		if len(task.Tags) != 2 {
			t.Errorf("AddTask() tags = %v, want 2 tags", len(task.Tags))
		}
	})

	t.Run("add task with empty title", func(t *testing.T) {
		req := AddTaskRequest{Title: ""}
		_, err := service.AddTask(ctx, req)
		if err == nil {
			t.Error("AddTask() should return error for empty title")
		}
	})
}

func TestTaskService_ListTasks(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	service := NewTaskService(store)
	ctx := context.Background()

	// Create test tasks
	service.AddTask(ctx, AddTaskRequest{Title: "Task 1"})
	service.AddTask(ctx, AddTaskRequest{Title: "Task 2"})

	t.Run("list all tasks", func(t *testing.T) {
		tasks, err := service.ListTasks(ctx, ListTasksRequest{})
		if err != nil {
			t.Errorf("ListTasks() error = %v", err)
		}
		if len(tasks) != 2 {
			t.Errorf("ListTasks() returned %d tasks, want 2", len(tasks))
		}
	})

	t.Run("list pending only", func(t *testing.T) {
		tasks, err := service.ListTasks(ctx, ListTasksRequest{OnlyPending: true})
		if err != nil {
			t.Errorf("ListTasks() error = %v", err)
		}
		if len(tasks) != 2 {
			t.Errorf("ListTasks() returned %d tasks, want 2", len(tasks))
		}
	})

	t.Run("list by status", func(t *testing.T) {
		status := domain.StatusPending
		tasks, err := service.ListTasks(ctx, ListTasksRequest{Status: &status})
		if err != nil {
			t.Errorf("ListTasks() error = %v", err)
		}
		if len(tasks) != 2 {
			t.Errorf("ListTasks() returned %d tasks, want 2", len(tasks))
		}
	})
}

func TestTaskService_GetTask(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	service := NewTaskService(store)
	ctx := context.Background()

	task, _ := service.AddTask(ctx, AddTaskRequest{Title: "Get Me"})

	t.Run("get existing task", func(t *testing.T) {
		found, err := service.GetTask(ctx, task.ID)
		if err != nil {
			t.Errorf("GetTask() error = %v", err)
		}
		if found.ID != task.ID {
			t.Error("GetTask() returned wrong task")
		}
	})

	t.Run("get non-existent task", func(t *testing.T) {
		_, err := service.GetTask(ctx, "non-existent")
		if err == nil {
			t.Error("GetTask() should return error for non-existent task")
		}
	})
}

func TestTaskService_CompleteTask(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	service := NewTaskService(store)
	ctx := context.Background()

	task, _ := service.AddTask(ctx, AddTaskRequest{Title: "Complete Me"})

	err := service.CompleteTask(ctx, task.ID)
	if err != nil {
		t.Errorf("CompleteTask() error = %v", err)
	}

	completed, _ := service.GetTask(ctx, task.ID)
	if completed.Status != domain.StatusCompleted {
		t.Errorf("CompleteTask() status = %v, want completed", completed.Status)
	}
}

func TestTaskService_DeleteTask(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	service := NewTaskService(store)
	ctx := context.Background()

	task, _ := service.AddTask(ctx, AddTaskRequest{Title: "Delete Me"})

	err := service.DeleteTask(ctx, task.ID)
	if err != nil {
		t.Errorf("DeleteTask() error = %v", err)
	}

	_, err = service.GetTask(ctx, task.ID)
	if err == nil {
		t.Error("DeleteTask() should remove task")
	}
}

func TestTaskService_StartTask(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	service := NewTaskService(store)
	ctx := context.Background()

	task, _ := service.AddTask(ctx, AddTaskRequest{Title: "Start Me"})

	err := service.StartTask(ctx, task.ID)
	if err != nil {
		t.Errorf("StartTask() error = %v", err)
	}

	started, _ := service.GetTask(ctx, task.ID)
	if started.Status != domain.StatusInProgress {
		t.Errorf("StartTask() status = %v, want in_progress", started.Status)
	}
}

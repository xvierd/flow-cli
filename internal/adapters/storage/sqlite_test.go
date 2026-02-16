package storage

import (
	"context"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
)

func TestNewMemory(t *testing.T) {
	storage, err := NewMemory()
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	defer storage.Close()

	if storage == nil {
		t.Error("NewMemory() returned nil storage")
	}
}

func TestTaskRepository_SaveAndFind(t *testing.T) {
	storage, err := NewMemory()
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	repo := storage.Tasks()

	t.Run("save new task", func(t *testing.T) {
		task, _ := domain.NewTask("Test Task")
		err := repo.Save(ctx, task)
		if err != nil {
			t.Errorf("Save() error = %v", err)
		}
	})

	t.Run("find by id", func(t *testing.T) {
		task, _ := domain.NewTask("Find Me")
		err := repo.Save(ctx, task)
		if err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		found, err := repo.FindByID(ctx, task.ID)
		if err != nil {
			t.Errorf("FindByID() error = %v", err)
		}
		if found == nil {
			t.Fatal("FindByID() returned nil")
		}
		if found.Title != task.Title {
			t.Errorf("Found task title = %v, want %v", found.Title, task.Title)
		}
	})

	t.Run("find non-existent", func(t *testing.T) {
		_, err := repo.FindByID(ctx, "non-existent-id")
		if err != domain.ErrTaskNotFound {
			t.Errorf("FindByID() error = %v, want ErrTaskNotFound", err)
		}
	})
}

func TestTaskRepository_FindAll(t *testing.T) {
	storage, _ := NewMemory()
	defer storage.Close()

	ctx := context.Background()
	repo := storage.Tasks()

	// Create test tasks
	task1, _ := domain.NewTask("Task 1")
	task2, _ := domain.NewTask("Task 2")
	task3, _ := domain.NewTask("Task 3")
	task3.Complete()

	_ = repo.Save(ctx, task1)
	_ = repo.Save(ctx, task2)
	_ = repo.Save(ctx, task3)

	t.Run("find all without filter", func(t *testing.T) {
		tasks, err := repo.FindAll(ctx, nil)
		if err != nil {
			t.Errorf("FindAll() error = %v", err)
		}
		if len(tasks) != 3 {
			t.Errorf("FindAll() returned %d tasks, want 3", len(tasks))
		}
	})

	t.Run("find with status filter", func(t *testing.T) {
		status := domain.StatusCompleted
		tasks, err := repo.FindAll(ctx, &status)
		if err != nil {
			t.Errorf("FindAll() error = %v", err)
		}
		if len(tasks) != 1 {
			t.Errorf("FindAll() returned %d tasks, want 1", len(tasks))
		}
	})
}

func TestTaskRepository_FindPending(t *testing.T) {
	storage, _ := NewMemory()
	defer storage.Close()

	ctx := context.Background()
	repo := storage.Tasks()

	task1, _ := domain.NewTask("Pending Task")
	task2, _ := domain.NewTask("In Progress Task")
	task2.Start()
	task3, _ := domain.NewTask("Completed Task")
	task3.Complete()

	_ = repo.Save(ctx, task1)
	_ = repo.Save(ctx, task2)
	_ = repo.Save(ctx, task3)

	tasks, err := repo.FindPending(ctx)
	if err != nil {
		t.Errorf("FindPending() error = %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("FindPending() returned %d tasks, want 2", len(tasks))
	}
}

func TestTaskRepository_FindActive(t *testing.T) {
	storage, _ := NewMemory()
	defer storage.Close()

	ctx := context.Background()
	repo := storage.Tasks()

	t.Run("no active task", func(t *testing.T) {
		active, err := repo.FindActive(ctx)
		if err != nil {
			t.Errorf("FindActive() error = %v", err)
		}
		if active != nil {
			t.Error("FindActive() should return nil when no active task")
		}
	})

	t.Run("with active task", func(t *testing.T) {
		task, _ := domain.NewTask("Active Task")
		task.Start()
		repo.Save(ctx, task)

		active, err := repo.FindActive(ctx)
		if err != nil {
			t.Errorf("FindActive() error = %v", err)
		}
		if active == nil {
			t.Fatal("FindActive() returned nil")
		}
		if active.ID != task.ID {
			t.Errorf("FindActive() returned wrong task")
		}
	})
}

func TestTaskRepository_Update(t *testing.T) {
	storage, _ := NewMemory()
	defer storage.Close()

	ctx := context.Background()
	repo := storage.Tasks()

	task, _ := domain.NewTask("Original Title")
	repo.Save(ctx, task)

	task.Title = "Updated Title"
	task.Start()

	err := repo.Update(ctx, task)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	found, _ := repo.FindByID(ctx, task.ID)
	if found.Title != "Updated Title" {
		t.Errorf("Update() title = %v, want 'Updated Title'", found.Title)
	}
	if found.Status != domain.StatusInProgress {
		t.Errorf("Update() status = %v, want in_progress", found.Status)
	}
}

func TestTaskRepository_Delete(t *testing.T) {
	storage, _ := NewMemory()
	defer storage.Close()

	ctx := context.Background()
	repo := storage.Tasks()

	task, _ := domain.NewTask("To Delete")
	repo.Save(ctx, task)

	err := repo.Delete(ctx, task.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = repo.FindByID(ctx, task.ID)
	if err != domain.ErrTaskNotFound {
		t.Errorf("FindByID() after delete error = %v, want ErrTaskNotFound", err)
	}
}

func TestSessionRepository_SaveAndFind(t *testing.T) {
	storage, _ := NewMemory()
	defer storage.Close()

	ctx := context.Background()
	repo := storage.Sessions()
	config := domain.DefaultPomodoroConfig()

	t.Run("save work session", func(t *testing.T) {
		session := domain.NewPomodoroSession(config, nil)
		err := repo.Save(ctx, session)
		if err != nil {
			t.Errorf("Save() error = %v", err)
		}
	})

	t.Run("save break session", func(t *testing.T) {
		session := domain.NewBreakSession(config, 1)
		err := repo.Save(ctx, session)
		if err != nil {
			t.Errorf("Save() error = %v", err)
		}
	})

	t.Run("find by id", func(t *testing.T) {
		session := domain.NewPomodoroSession(config, nil)
		repo.Save(ctx, session)

		found, err := repo.FindByID(ctx, session.ID)
		if err != nil {
			t.Errorf("FindByID() error = %v", err)
		}
		if found == nil {
			t.Fatal("FindByID() returned nil")
		}
		if found.Type != session.Type {
			t.Errorf("Found session type = %v, want %v", found.Type, session.Type)
		}
	})
}

func TestSessionRepository_FindActive(t *testing.T) {
	storage, _ := NewMemory()
	defer storage.Close()

	ctx := context.Background()
	repo := storage.Sessions()
	config := domain.DefaultPomodoroConfig()

	t.Run("find running session", func(t *testing.T) {
		session := domain.NewPomodoroSession(config, nil)
		repo.Save(ctx, session)

		active, err := repo.FindActive(ctx)
		if err != nil {
			t.Errorf("FindActive() error = %v", err)
		}
		if active == nil {
			t.Fatal("FindActive() returned nil")
		}
		if active.ID != session.ID {
			t.Error("FindActive() returned wrong session")
		}
	})

	t.Run("find paused session", func(t *testing.T) {
		session := domain.NewPomodoroSession(config, nil)
		session.Pause()
		repo.Save(ctx, session)

		active, err := repo.FindActive(ctx)
		if err != nil {
			t.Errorf("FindActive() error = %v", err)
		}
		if active == nil {
			t.Fatal("FindActive() returned nil for paused session")
		}
	})
}

func TestSessionRepository_Update(t *testing.T) {
	storage, _ := NewMemory()
	defer storage.Close()

	ctx := context.Background()
	repo := storage.Sessions()
	config := domain.DefaultPomodoroConfig()

	session := domain.NewPomodoroSession(config, nil)
	repo.Save(ctx, session)

	session.Complete()
	err := repo.Update(ctx, session)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	found, _ := repo.FindByID(ctx, session.ID)
	if found.Status != domain.SessionStatusCompleted {
		t.Errorf("Update() status = %v, want completed", found.Status)
	}
	if found.CompletedAt == nil {
		t.Error("Update() completed_at should not be nil")
	}
}

func TestSessionRepository_FindRecent(t *testing.T) {
	storage, _ := NewMemory()
	defer storage.Close()

	ctx := context.Background()
	repo := storage.Sessions()
	config := domain.DefaultPomodoroConfig()

	// Create sessions
	session1 := domain.NewPomodoroSession(config, nil)
	session1.Complete()
	repo.Save(ctx, session1)

	sessions, err := repo.FindRecent(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Errorf("FindRecent() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("FindRecent() returned %d sessions, want 1", len(sessions))
	}
}

func TestSessionRepository_GetDailyStats(t *testing.T) {
	storage, _ := NewMemory()
	defer storage.Close()

	ctx := context.Background()
	repo := storage.Sessions()
	config := domain.DefaultPomodoroConfig()

	// Create completed work session
	session := domain.NewPomodoroSession(config, nil)
	session.Complete()
	repo.Save(ctx, session)

	// Create completed break session
	breakSession := domain.NewBreakSession(config, 1)
	breakSession.Complete()
	repo.Save(ctx, breakSession)

	stats, err := repo.GetDailyStats(ctx, time.Now())
	if err != nil {
		t.Errorf("GetDailyStats() error = %v", err)
	}
	if stats.WorkSessions != 1 {
		t.Errorf("WorkSessions = %d, want 1", stats.WorkSessions)
	}
	if stats.BreaksTaken != 1 {
		t.Errorf("BreaksTaken = %d, want 1", stats.BreaksTaken)
	}
	if stats.TotalWorkTime == 0 {
		t.Error("TotalWorkTime should not be zero")
	}
}

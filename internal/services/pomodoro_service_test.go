package services

import (
	"context"
	"testing"

	"github.com/dvidx/flow-cli/internal/domain"
	"github.com/dvidx/flow-cli/internal/ports"
)

func clearSessions(t *testing.T, store ports.Storage, ctx context.Context) {
	session, _ := store.Sessions().FindActive(ctx)
	if session != nil {
		session.Complete()
		store.Sessions().Update(ctx, session)
	}
}

func TestPomodoroService_StartPomodoro(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	service := NewPomodoroService(store, nil)
	ctx := context.Background()

	t.Run("start pomodoro without task", func(t *testing.T) {
		clearSessions(t, store, ctx)

		req := StartPomodoroRequest{
			TaskID:     nil,
			WorkingDir: ".",
		}

		session, err := service.StartPomodoro(ctx, req)
		if err != nil {
			t.Errorf("StartPomodoro() error = %v", err)
		}
		if session == nil {
			t.Fatal("StartPomodoro() returned nil")
		}
		if session.Type != domain.SessionTypeWork {
			t.Errorf("StartPomodoro() type = %v, want work", session.Type)
		}
	})

	t.Run("start pomodoro with task", func(t *testing.T) {
		clearSessions(t, store, ctx)

		// Create a task first
		taskService := NewTaskService(store)
		task, _ := taskService.AddTask(ctx, AddTaskRequest{Title: "Test Task"})

		req := StartPomodoroRequest{
			TaskID:     &task.ID,
			WorkingDir: ".",
		}

		session, err := service.StartPomodoro(ctx, req)
		if err != nil {
			t.Errorf("StartPomodoro() error = %v", err)
		}
		if session.TaskID == nil || *session.TaskID != task.ID {
			t.Error("StartPomodoro() should link to task")
		}
	})

	t.Run("start when already active", func(t *testing.T) {
		// Don't clear sessions - keep the one from previous test
		req := StartPomodoroRequest{
			TaskID:     nil,
			WorkingDir: ".",
		}

		_, err := service.StartPomodoro(ctx, req)
		if err != domain.ErrSessionAlreadyActive {
			t.Errorf("StartPomodoro() error = %v, want ErrSessionAlreadyActive", err)
		}
	})
}

func TestPomodoroService_StartBreak(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	service := NewPomodoroService(store, nil)
	ctx := context.Background()

	// Complete existing session
	clearSessions(t, store, ctx)

	t.Run("start break", func(t *testing.T) {
		session, err := service.StartBreak(ctx, ".")
		if err != nil {
			t.Errorf("StartBreak() error = %v", err)
		}
		// Break type depends on number of completed sessions
		// First break after 0 sessions = short break
		// First break after 4 sessions = long break
		if session.Type != domain.SessionTypeShortBreak && session.Type != domain.SessionTypeLongBreak {
			t.Errorf("StartBreak() type = %v, want short_break or long_break", session.Type)
		}
	})
}

func TestPomodoroService_PauseAndResume(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	service := NewPomodoroService(store, nil)
	ctx := context.Background()

	// Complete existing and start new
	clearSessions(t, store, ctx)
	service.StartPomodoro(ctx, StartPomodoroRequest{})

	t.Run("pause session", func(t *testing.T) {
		session, err := service.PauseSession(ctx)
		if err != nil {
			t.Errorf("PauseSession() error = %v", err)
		}
		if session.Status != domain.SessionStatusPaused {
			t.Errorf("PauseSession() status = %v, want paused", session.Status)
		}
	})

	t.Run("resume session", func(t *testing.T) {
		session, err := service.ResumeSession(ctx)
		if err != nil {
			t.Errorf("ResumeSession() error = %v", err)
		}
		if session.Status != domain.SessionStatusRunning {
			t.Errorf("ResumeSession() status = %v, want running", session.Status)
		}
	})
}

func TestPomodoroService_StopSession(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	service := NewPomodoroService(store, nil)
	ctx := context.Background()

	// Complete existing and start new
	clearSessions(t, store, ctx)
	service.StartPomodoro(ctx, StartPomodoroRequest{})

	t.Run("stop session", func(t *testing.T) {
		session, err := service.StopSession(ctx)
		if err != nil {
			t.Errorf("StopSession() error = %v", err)
		}
		if session.Status != domain.SessionStatusCompleted {
			t.Errorf("StopSession() status = %v, want completed", session.Status)
		}
		if session.CompletedAt == nil {
			t.Error("StopSession() completed_at should not be nil")
		}
	})
}

func TestPomodoroService_CancelSession(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	service := NewPomodoroService(store, nil)
	ctx := context.Background()

	clearSessions(t, store, ctx)
	service.StartPomodoro(ctx, StartPomodoroRequest{})

	t.Run("cancel session", func(t *testing.T) {
		err := service.CancelSession(ctx)
		if err != nil {
			t.Errorf("CancelSession() error = %v", err)
		}

		session, _ := store.Sessions().FindActive(ctx)
		if session != nil {
			t.Error("CancelSession() should remove active session")
		}
	})
}

func TestPomodoroService_GetCurrentState(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	service := NewPomodoroService(store, nil)
	ctx := context.Background()

	// Complete all sessions
	clearSessions(t, store, ctx)

	// Create task and start pomodoro
	taskService := NewTaskService(store)
	task, _ := taskService.AddTask(ctx, AddTaskRequest{Title: "Active Task"})
	service.StartPomodoro(ctx, StartPomodoroRequest{TaskID: &task.ID})

	t.Run("get current state", func(t *testing.T) {
		state, err := service.GetCurrentState(ctx)
		if err != nil {
			t.Errorf("GetCurrentState() error = %v", err)
		}
		if state.ActiveTask == nil {
			t.Error("GetCurrentState() should have active task")
		}
		if state.ActiveSession == nil {
			t.Error("GetCurrentState() should have active session")
		}
	})
}

func TestPomodoroService_GetTaskHistory(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	service := NewPomodoroService(store, nil)
	ctx := context.Background()

	// Create task and complete session
	taskService := NewTaskService(store)
	task, _ := taskService.AddTask(ctx, AddTaskRequest{Title: "History Task"})

	// Complete existing
	clearSessions(t, store, ctx)
	service.StartPomodoro(ctx, StartPomodoroRequest{TaskID: &task.ID})
	service.StopSession(ctx)

	t.Run("get task history", func(t *testing.T) {
		history, err := service.GetTaskHistory(ctx, task.ID)
		if err != nil {
			t.Errorf("GetTaskHistory() error = %v", err)
		}
		if len(history) != 1 {
			t.Errorf("GetTaskHistory() returned %d sessions, want 1", len(history))
		}
	})
}

func TestPomodoroService_GetRecentSessions(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	service := NewPomodoroService(store, nil)
	ctx := context.Background()

	// Complete existing
	clearSessions(t, store, ctx)

	// Create multiple sessions
	for i := 0; i < 3; i++ {
		service.StartPomodoro(ctx, StartPomodoroRequest{})
		service.StopSession(ctx)
	}

	t.Run("get recent sessions", func(t *testing.T) {
		sessions, err := service.GetRecentSessions(ctx, 10)
		if err != nil {
			t.Errorf("GetRecentSessions() error = %v", err)
		}
		if len(sessions) != 3 {
			t.Errorf("GetRecentSessions() returned %d sessions, want 3", len(sessions))
		}
	})

	t.Run("limit results", func(t *testing.T) {
		sessions, err := service.GetRecentSessions(ctx, 2)
		if err != nil {
			t.Errorf("GetRecentSessions() error = %v", err)
		}
		if len(sessions) != 2 {
			t.Errorf("GetRecentSessions() returned %d sessions, want 2", len(sessions))
		}
	})
}

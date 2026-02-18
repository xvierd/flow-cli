package services

import (
	"context"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

func clearSessions(t *testing.T, store ports.Storage, ctx context.Context) {
	session, _ := store.Sessions().FindActive(ctx)
	if session != nil {
		session.Complete()
		_ = store.Sessions().Update(ctx, session)
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
	_, _ = service.StartPomodoro(ctx, StartPomodoroRequest{})

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
	_, _ = service.StartPomodoro(ctx, StartPomodoroRequest{})

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
	_, _ = service.StartPomodoro(ctx, StartPomodoroRequest{})

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
	if _, err := service.StartPomodoro(ctx, StartPomodoroRequest{TaskID: &task.ID}); err != nil {
		t.Fatalf("StartPomodoro() error = %v", err)
	}

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
	_, _ = service.StartPomodoro(ctx, StartPomodoroRequest{TaskID: &task.ID})
	_, _ = service.StopSession(ctx)

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
		_, _ = service.StartPomodoro(ctx, StartPomodoroRequest{})
		_, _ = service.StopSession(ctx)
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

func TestPomodoroService_LogDistraction_WithCategory(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	service := NewPomodoroService(store, nil)
	ctx := context.Background()

	clearSessions(t, store, ctx)
	session, err := service.StartPomodoro(ctx, StartPomodoroRequest{})
	if err != nil {
		t.Fatalf("StartPomodoro() error = %v", err)
	}

	err = service.LogDistraction(ctx, session.ID, "checked email", "external")
	if err != nil {
		t.Fatalf("LogDistraction() error = %v", err)
	}

	found, _ := store.Sessions().FindByID(ctx, session.ID)
	if len(found.Distractions) != 1 {
		t.Fatalf("got %d distractions, want 1", len(found.Distractions))
	}
	if found.Distractions[0].Text != "checked email" {
		t.Errorf("distraction text = %q, want 'checked email'", found.Distractions[0].Text)
	}
	if found.Distractions[0].Category != "external" {
		t.Errorf("distraction category = %q, want 'external'", found.Distractions[0].Category)
	}
}

func TestPomodoroService_SetShutdownRitual(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	service := NewPomodoroService(store, nil)
	ctx := context.Background()

	clearSessions(t, store, ctx)
	session, err := service.StartPomodoro(ctx, StartPomodoroRequest{
		Methodology: domain.MethodologyDeepWork,
	})
	if err != nil {
		t.Fatalf("StartPomodoro() error = %v", err)
	}

	ritual := domain.ShutdownRitual{
		PendingTasksReview: "all clear",
		TomorrowPlan:       "write tests",
		ClosingPhrase:      "shutdown complete",
	}
	err = service.SetShutdownRitual(ctx, session.ID, ritual)
	if err != nil {
		t.Fatalf("SetShutdownRitual() error = %v", err)
	}

	found, _ := store.Sessions().FindByID(ctx, session.ID)
	if found.ShutdownRitual == nil {
		t.Fatal("ShutdownRitual should not be nil")
	}
	if found.ShutdownRitual.TomorrowPlan != "write tests" {
		t.Errorf("TomorrowPlan = %q, want 'write tests'", found.ShutdownRitual.TomorrowPlan)
	}
}

func TestPomodoroService_GetDeepWorkStreak_CustomThreshold(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	service := NewPomodoroService(store, nil)
	ctx := context.Background()

	// Create a deep work session for today with 90 min duration
	session := &domain.PomodoroSession{
		ID:          "dw-streak-test",
		Type:        domain.SessionTypeWork,
		Status:      domain.SessionStatusCompleted,
		Duration:    90 * time.Minute,
		StartedAt:   time.Now().Add(-2 * time.Hour),
		Methodology: domain.MethodologyDeepWork,
	}
	now := time.Now()
	session.CompletedAt = &now
	if err := store.Sessions().Save(ctx, session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// With 1 hour threshold, should have streak >= 1
	streak, err := service.GetDeepWorkStreak(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("GetDeepWorkStreak() error = %v", err)
	}
	if streak < 1 {
		t.Errorf("streak = %d, want >= 1 with 1h threshold", streak)
	}

	// With 2 hour threshold, streak should be 0 (only 90 min)
	streak, err = service.GetDeepWorkStreak(ctx, 2*time.Hour)
	if err != nil {
		t.Fatalf("GetDeepWorkStreak() error = %v", err)
	}
	if streak != 0 {
		t.Errorf("streak = %d, want 0 with 2h threshold", streak)
	}

	// With 0 threshold, should default to 4h, so streak should be 0
	streak, err = service.GetDeepWorkStreak(ctx, 0)
	if err != nil {
		t.Fatalf("GetDeepWorkStreak() error = %v", err)
	}
	if streak != 0 {
		t.Errorf("streak = %d, want 0 with default 4h threshold", streak)
	}
}

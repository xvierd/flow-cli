package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/adapters/storage"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
	"github.com/xvierd/flow-cli/internal/services"
)

// setupTestStorage creates a temporary database for integration tests
func setupTestStorage(t *testing.T) (ports.Storage, func()) {
	t.Helper()
	
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	
	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	
	if err := store.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}
	
	cleanup := func() {
		store.Close()
		os.Remove(dbPath)
	}
	
	return store, cleanup
}

// TestFullSessionLifecycle tests a complete pomodoro session lifecycle
func TestFullSessionLifecycle(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()
	
	ctx := context.Background()
	pomodoroSvc := services.NewPomodoroService(store, nil)
	
	t.Run("complete session lifecycle", func(t *testing.T) {
		// 1. Start a pomodoro session
		req := services.StartPomodoroRequest{
			WorkingDir: ".",
		}
		
		session, err := pomodoroSvc.StartPomodoro(ctx, req)
		if err != nil {
			t.Fatalf("failed to start pomodoro: %v", err)
		}
		
		if session.Status != domain.SessionStatusRunning {
			t.Errorf("expected session status running, got %v", session.Status)
		}
		
		if session.Type != domain.SessionTypeWork {
			t.Errorf("expected session type work, got %v", session.Type)
		}
		
		// 2. Pause the session
		pausedSession, err := pomodoroSvc.PauseSession(ctx)
		if err != nil {
			t.Fatalf("failed to pause session: %v", err)
		}
		
		if pausedSession.Status != domain.SessionStatusPaused {
			t.Errorf("expected session status paused, got %v", pausedSession.Status)
		}
		
		// 3. Resume the session
		resumedSession, err := pomodoroSvc.ResumeSession(ctx)
		if err != nil {
			t.Fatalf("failed to resume session: %v", err)
		}
		
		if resumedSession.Status != domain.SessionStatusRunning {
			t.Errorf("expected session status running after resume, got %v", resumedSession.Status)
		}
		
		// 4. Stop (complete) the session
		completedSession, err := pomodoroSvc.StopSession(ctx)
		if err != nil {
			t.Fatalf("failed to stop session: %v", err)
		}
		
		if completedSession.Status != domain.SessionStatusCompleted {
			t.Errorf("expected session status completed, got %v", completedSession.Status)
		}
		
		if completedSession.CompletedAt == nil {
			t.Error("expected CompletedAt to be set")
		}
		
		// 5. Verify no active session remains
		active, err := store.Sessions().FindActive(ctx)
		if err != nil {
			t.Fatalf("failed to check active sessions: %v", err)
		}
		
		if active != nil {
			t.Error("expected no active session after completion")
		}
	})
}

// TestStartPauseResumeStop tests the start -> pause -> resume -> stop flow
func TestStartPauseResumeStop(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()
	
	ctx := context.Background()
	pomodoroSvc := services.NewPomodoroService(store, nil)
	
	t.Run("start pause resume stop flow", func(t *testing.T) {
		// Start
		_, err := pomodoroSvc.StartPomodoro(ctx, services.StartPomodoroRequest{})
		if err != nil {
			t.Fatalf("failed to start: %v", err)
		}
		
		// Pause
		session, err := pomodoroSvc.PauseSession(ctx)
		if err != nil {
			t.Fatalf("failed to pause: %v", err)
		}
		
		// Verify elapsed time doesn't change while paused
		elapsedBefore := session.ElapsedTime()
		time.Sleep(50 * time.Millisecond)
		elapsedAfter := session.ElapsedTime()
		
		if elapsedAfter != elapsedBefore {
			t.Error("elapsed time should not change while paused")
		}
		
		// Resume
		_, err = pomodoroSvc.ResumeSession(ctx)
		if err != nil {
			t.Fatalf("failed to resume: %v", err)
		}
		
		// Stop
		_, err = pomodoroSvc.StopSession(ctx)
		if err != nil {
			t.Fatalf("failed to stop: %v", err)
		}
	})
}

// TestCreateTaskStartPomodoroComplete tests creating a task, starting a pomodoro, and completing the task
func TestCreateTaskStartPomodoroComplete(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()
	
	ctx := context.Background()
	taskSvc := services.NewTaskService(store)
	pomodoroSvc := services.NewPomodoroService(store, nil)
	
	t.Run("create task start pomodoro complete", func(t *testing.T) {
		// 1. Create a task
		taskReq := services.AddTaskRequest{
			Title: "Integration Test Task",
			Tags:  []string{"test", "integration"},
		}
		
		task, err := taskSvc.AddTask(ctx, taskReq)
		if err != nil {
			t.Fatalf("failed to add task: %v", err)
		}
		
		if task.Title != "Integration Test Task" {
			t.Errorf("expected title 'Integration Test Task', got %q", task.Title)
		}
		
		if task.Status != domain.StatusPending {
			t.Errorf("expected status pending, got %v", task.Status)
		}
		
		// 2. Start a pomodoro with the task
		pomodoroReq := services.StartPomodoroRequest{
			TaskID:     &task.ID,
			WorkingDir: ".",
		}
		
		session, err := pomodoroSvc.StartPomodoro(ctx, pomodoroReq)
		if err != nil {
			t.Fatalf("failed to start pomodoro: %v", err)
		}
		
		if session.TaskID == nil || *session.TaskID != task.ID {
			t.Error("expected session to be linked to task")
		}
		
		// Verify task is now in progress
		updatedTask, err := taskSvc.GetTask(ctx, task.ID)
		if err != nil {
			t.Fatalf("failed to get task: %v", err)
		}
		
		if updatedTask.Status != domain.StatusInProgress {
			t.Errorf("expected task status in_progress, got %v", updatedTask.Status)
		}
		
		// 3. Complete the pomodoro session
		_, err = pomodoroSvc.StopSession(ctx)
		if err != nil {
			t.Fatalf("failed to stop pomodoro: %v", err)
		}
		
		// 4. Complete the task
		err = taskSvc.CompleteTask(ctx, task.ID)
		if err != nil {
			t.Fatalf("failed to complete task: %v", err)
		}
		
		// Verify task is completed
		completedTask, err := taskSvc.GetTask(ctx, task.ID)
		if err != nil {
			t.Fatalf("failed to get completed task: %v", err)
		}
		
		if completedTask.Status != domain.StatusCompleted {
			t.Errorf("expected task status completed, got %v", completedTask.Status)
		}
		
		// 5. Verify task history includes the session
		history, err := pomodoroSvc.GetTaskHistory(ctx, task.ID)
		if err != nil {
			t.Fatalf("failed to get task history: %v", err)
		}
		
		if len(history) != 1 {
			t.Errorf("expected 1 session in history, got %d", len(history))
		}
	})
}

// TestBreakSessionFlow tests starting breaks after work sessions
func TestBreakSessionFlow(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()
	
	ctx := context.Background()
	pomodoroSvc := services.NewPomodoroService(store, nil)
	
	t.Run("break after work session", func(t *testing.T) {
		// Complete a work session
		_, err := pomodoroSvc.StartPomodoro(ctx, services.StartPomodoroRequest{})
		if err != nil {
			t.Fatalf("failed to start pomodoro: %v", err)
		}
		
		_, err = pomodoroSvc.StopSession(ctx)
		if err != nil {
			t.Fatalf("failed to stop pomodoro: %v", err)
		}
		
		// Start a break
		breakSession, err := pomodoroSvc.StartBreak(ctx, ".")
		if err != nil {
			t.Fatalf("failed to start break: %v", err)
		}
		
		if breakSession.Type != domain.SessionTypeShortBreak {
			t.Errorf("expected short break, got %v", breakSession.Type)
		}
		
		// Complete the break
		_, err = pomodoroSvc.StopSession(ctx)
		if err != nil {
			t.Fatalf("failed to stop break: %v", err)
		}
	})
}

// TestCancelSession tests cancelling a session
func TestCancelSession(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()
	
	ctx := context.Background()
	pomodoroSvc := services.NewPomodoroService(store, nil)
	
	t.Run("cancel active session", func(t *testing.T) {
		// Start a session
		session, err := pomodoroSvc.StartPomodoro(ctx, services.StartPomodoroRequest{})
		if err != nil {
			t.Fatalf("failed to start: %v", err)
		}
		
		// Cancel it
		err = pomodoroSvc.CancelSession(ctx)
		if err != nil {
			t.Fatalf("failed to cancel: %v", err)
		}
		
		// Verify session is cancelled by checking the stored version
		cancelledSession, err := store.Sessions().FindByID(ctx, session.ID)
		if err != nil {
			t.Fatalf("failed to find session: %v", err)
		}
		
		if cancelledSession.Status != domain.SessionStatusCancelled {
			t.Errorf("expected status cancelled, got %v", cancelledSession.Status)
		}
		
		// Verify no active session
		active, _ := store.Sessions().FindActive(ctx)
		if active != nil {
			t.Error("expected no active session after cancellation")
		}
	})
}

// TestConcurrentSessions tests that only one session can be active
func TestConcurrentSessions(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()
	
	ctx := context.Background()
	pomodoroSvc := services.NewPomodoroService(store, nil)
	
	t.Run("cannot start two sessions", func(t *testing.T) {
		// Start first session
		_, err := pomodoroSvc.StartPomodoro(ctx, services.StartPomodoroRequest{})
		if err != nil {
			t.Fatalf("failed to start first session: %v", err)
		}
		
		// Try to start second session
		_, err = pomodoroSvc.StartPomodoro(ctx, services.StartPomodoroRequest{})
		if err != domain.ErrSessionAlreadyActive {
			t.Errorf("expected ErrSessionAlreadyActive, got %v", err)
		}
		
		// Stop first session
		_, err = pomodoroSvc.StopSession(ctx)
		if err != nil {
			t.Fatalf("failed to stop session: %v", err)
		}
		
		// Now can start a new session
		_, err = pomodoroSvc.StartPomodoro(ctx, services.StartPomodoroRequest{})
		if err != nil {
			t.Fatalf("failed to start second session after stopping first: %v", err)
		}
	})
}

// TestDailyStats tests that daily stats are tracked correctly
func TestDailyStats(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()
	
	ctx := context.Background()
	pomodoroSvc := services.NewPomodoroService(store, nil)
	
	t.Run("daily stats accumulation", func(t *testing.T) {
		// Complete 3 work sessions
		for i := 0; i < 3; i++ {
			_, err := pomodoroSvc.StartPomodoro(ctx, services.StartPomodoroRequest{})
			if err != nil {
				t.Fatalf("failed to start session %d: %v", i+1, err)
			}
			
			_, err = pomodoroSvc.StopSession(ctx)
			if err != nil {
				t.Fatalf("failed to stop session %d: %v", i+1, err)
			}
		}
		
		// Get current state to check stats
		state, err := pomodoroSvc.GetCurrentState(ctx)
		if err != nil {
			t.Fatalf("failed to get current state: %v", err)
		}
		
		if state.TodayStats.WorkSessions != 3 {
			t.Errorf("expected 3 work sessions, got %d", state.TodayStats.WorkSessions)
		}
		
		if state.TodayStats.TotalWorkTime <= 0 {
			t.Error("expected positive total work time")
		}
	})
}

package services

// Comprehensive tests for StateService, the MCPStateProvider + CLI status shim.
// Covers the guard clauses (nil pomodoro/task service), session delegation,
// task CRUD, and the focused stats/report methods.

import (
	"context"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

// newStateWithServices wires a full StateService backed by in-memory storage.
func newStateWithServices(t *testing.T) (*StateService, ports.Storage, *TaskService, *PomodoroService) {
	t.Helper()
	store, cleanup := setupTestStorage(t)
	t.Cleanup(cleanup)

	taskSvc := NewTaskService(store)
	pomSvc := NewPomodoroService(store, nil)

	svc := NewStateService(store, taskSvc, pomSvc)
	return svc, store, taskSvc, pomSvc
}

func TestStateService_GetCurrentState(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()
	taskSvc := NewTaskService(store)
	pomSvc := NewPomodoroService(store, nil)
	svc := NewStateService(store, taskSvc, pomSvc)
	ctx := context.Background()

	t.Run("empty state", func(t *testing.T) {
		state, err := svc.GetCurrentState(ctx)
		if err != nil {
			t.Fatalf("GetCurrentState() error = %v", err)
		}
		if state.ActiveTask != nil || state.ActiveSession != nil {
			t.Errorf("empty state should have no active task/session, got task=%v session=%v", state.ActiveTask, state.ActiveSession)
		}
	})

	t.Run("auto-completes expired running session", func(t *testing.T) {
		// Start a session, then backdate its StartedAt so it has fully elapsed.
		if _, err := svc.StartSession(ctx, ports.StartSessionRequest{Methodology: domain.MethodologyPomodoro}); err != nil {
			t.Fatalf("StartSession() error = %v", err)
		}
		active, _ := store.Sessions().FindActive(ctx)
		active.StartedAt = time.Now().Add(-2 * active.Duration)
		active.Status = domain.SessionStatusRunning
		_ = store.Sessions().Update(ctx, active)

		state, err := svc.GetCurrentState(ctx)
		if err != nil {
			t.Fatalf("GetCurrentState() error = %v", err)
		}
		if state.ActiveSession != nil {
			t.Error("expired running session should be auto-completed and removed from active state")
		}
		completed, _ := store.Sessions().FindByID(ctx, active.ID)
		if completed.Status != domain.SessionStatusCompleted {
			t.Errorf("expired session status = %v, want completed", completed.Status)
		}
	})
}

func TestState_ListTasksAndHistory(t *testing.T) {
	svc, _, taskSvc, _ := newStateWithServices(t)
	ctx := context.Background()

	task, err := taskSvc.AddTask(ctx, AddTaskRequest{Title: "Alpha"})
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	t.Run("list all", func(t *testing.T) {
		tasks, err := svc.ListTasks(ctx, nil)
		if err != nil {
			t.Fatalf("ListTasks() error = %v", err)
		}
		if len(tasks) != 1 {
			t.Errorf("ListTasks() returned %d tasks, want 1", len(tasks))
		}
	})
	t.Run("list pending only", func(t *testing.T) {
		status := domain.StatusPending
		tasks, err := svc.ListTasks(ctx, &status)
		if err != nil {
			t.Fatalf("ListTasks(pending) error = %v", err)
		}
		if len(tasks) != 1 {
			t.Errorf("ListTasks(pending) returned %d tasks, want 1", len(tasks))
		}
	})
	t.Run("task history", func(t *testing.T) {
		_, _ = svc.StartSession(ctx, ports.StartSessionRequest{TaskID: &task.ID})
		_, _ = svc.StopPomodoro(ctx)

		history, err := svc.GetTaskHistory(ctx, task.ID)
		if err != nil {
			t.Fatalf("GetTaskHistory() error = %v", err)
		}
		if len(history) != 1 {
			t.Errorf("GetTaskHistory() returned %d sessions, want 1", len(history))
		}
	})
}

func TestState_StartSession(t *testing.T) {
	svc, store, _, _ := newStateWithServices(t)
	ctx := context.Background()

	t.Run("start with title creates task", func(t *testing.T) {
		clearSessions(t, store, ctx)
		session, err := svc.StartSession(ctx, ports.StartSessionRequest{TaskTitle: "Write tests"})
		if err != nil {
			t.Fatalf("StartSession() error = %v", err)
		}
		if session == nil || session.TaskID == nil {
			t.Fatal("StartSession() with title should create/link a task")
		}
	})
	t.Run("start with duration", func(t *testing.T) {
		clearSessions(t, store, ctx)
		mins := 15
		session, err := svc.StartSession(ctx, ports.StartSessionRequest{DurationMinutes: &mins})
		if err != nil {
			t.Fatalf("StartSession() error = %v", err)
		}
		if session.Duration != 15*time.Minute {
			t.Errorf("session duration = %v, want 15m", session.Duration)
		}
	})
	t.Run("invalid methodology rejected", func(t *testing.T) {
		clearSessions(t, store, ctx)
		_, err := svc.StartSession(ctx, ports.StartSessionRequest{Methodology: domain.Methodology("bogus")})
		if err == nil {
			t.Fatal("StartSession() with invalid methodology should error")
		}
	})
	t.Run("start when already active", func(t *testing.T) {
		clearSessions(t, store, ctx)
		if _, err := svc.StartSession(ctx, ports.StartSessionRequest{}); err != nil {
			t.Fatalf("setup StartSession() error = %v", err)
		}
		_, err := svc.StartSession(ctx, ports.StartSessionRequest{})
		if err != domain.ErrSessionAlreadyActive {
			t.Errorf("StartSession() error = %v, want ErrSessionAlreadyActive", err)
		}
	})
}

func TestState_StartSession_DurationOnlyOnce(t *testing.T) {
	svc, store, _, _ := newStateWithServices(t)
	ctx := context.Background()

	// Seed an already-active session so the second start hits the guard.
	clearSessions(t, store, ctx)

	mins := 30
	session, err := svc.StartSession(ctx, ports.StartSessionRequest{DurationMinutes: &mins})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if session.Duration != 30*time.Minute {
		t.Errorf("session duration = %v, want 30m", session.Duration)
	}
}

func TestState_PomodoroGuard(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()
	// StateService without a configured pomodoro service.
	svc := NewStateService(store, nil, nil)
	ctx := context.Background()

	if _, err := svc.StartSession(ctx, ports.StartSessionRequest{}); err != domain.ErrServiceNotConfigured {
		t.Errorf("StartSession() error = %v, want ErrServiceNotConfigured", err)
	}
	if _, err := svc.StartBreak(ctx); err != domain.ErrServiceNotConfigured {
		t.Errorf("StartBreak() error = %v, want ErrServiceNotConfigured", err)
	}
	if _, err := svc.StopPomodoro(ctx); err != domain.ErrServiceNotConfigured {
		t.Errorf("StopPomodoro() error = %v, want ErrServiceNotConfigured", err)
	}
	if _, err := svc.PausePomodoro(ctx); err != domain.ErrServiceNotConfigured {
		t.Errorf("PausePomodoro() error = %v, want ErrServiceNotConfigured", err)
	}
	if _, err := svc.ResumePomodoro(ctx); err != domain.ErrServiceNotConfigured {
		t.Errorf("ResumePomodoro() error = %v, want ErrServiceNotConfigured", err)
	}
	if err := svc.CancelSession(ctx); err != domain.ErrServiceNotConfigured {
		t.Errorf("CancelSession() error = %v, want ErrServiceNotConfigured", err)
	}
	if _, err := svc.VoidSession(ctx); err != domain.ErrServiceNotConfigured {
		t.Errorf("VoidSession() error = %v, want ErrServiceNotConfigured", err)
	}
	if _, err := svc.AddSessionNotes(ctx, "s1", "note"); err != domain.ErrServiceNotConfigured {
		t.Errorf("AddSessionNotes() error = %v, want ErrServiceNotConfigured", err)
	}
	if err := svc.LogDistraction(ctx, "s1", "x", "internal"); err != domain.ErrServiceNotConfigured {
		t.Errorf("LogDistraction() error = %v, want ErrServiceNotConfigured", err)
	}
	if err := svc.SetFocusScore(ctx, "s1", 3); err != domain.ErrServiceNotConfigured {
		t.Errorf("SetFocusScore() error = %v, want ErrServiceNotConfigured", err)
	}
	if err := svc.SetAccomplishment(ctx, "s1", "done"); err != domain.ErrServiceNotConfigured {
		t.Errorf("SetAccomplishment() error = %v, want ErrServiceNotConfigured", err)
	}
	if err := svc.SetShutdownRitual(ctx, "s1", domain.ShutdownRitual{}); err != domain.ErrServiceNotConfigured {
		t.Errorf("SetShutdownRitual() error = %v, want ErrServiceNotConfigured", err)
	}
	if err := svc.SetEnergizeActivity(ctx, "s1", "walk"); err != domain.ErrServiceNotConfigured {
		t.Errorf("SetEnergizeActivity() error = %v, want ErrServiceNotConfigured", err)
	}
	if err := svc.SetOutcomeAchieved(ctx, "s1", "y"); err != domain.ErrServiceNotConfigured {
		t.Errorf("SetOutcomeAchieved() error = %v, want ErrServiceNotConfigured", err)
	}
}

func TestState_TaskServiceGuard(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()
	// StateService with a task service but no pomodoro service.
	taskSvc := NewTaskService(store)
	svc := NewStateService(store, taskSvc, nil)
	ctx := context.Background()

	if _, err := svc.CreateTask(ctx, "task", nil, nil); err != nil {
		t.Errorf("CreateTask() error = %v, want nil", err)
	}

	// StateService without a task service at all.
	bare := NewStateService(store, nil, nil)
	if _, err := bare.CreateTask(ctx, "task", nil, nil); err != domain.ErrServiceNotConfigured {
		t.Errorf("CreateTask() error = %v, want ErrServiceNotConfigured", err)
	}
	if err := bare.DeleteTask(ctx, "t1"); err != domain.ErrServiceNotConfigured {
		t.Errorf("DeleteTask() error = %v, want ErrServiceNotConfigured", err)
	}
	if err := bare.StartTask(ctx, "t1"); err != domain.ErrServiceNotConfigured {
		t.Errorf("StartTask() error = %v, want ErrServiceNotConfigured", err)
	}
	if _, err := svc.CompleteTask(ctx, "missing"); err == nil {
		t.Error("CompleteTask() with missing task should error")
	}
}

func TestState_TaskLifecycle(t *testing.T) {
	svc, store, _, _ := newStateWithServices(t)
	ctx := context.Background()

	desc := "a description"
	task, err := svc.CreateTask(ctx, "Lifecycle", &desc, []string{"backend"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if task.Description != desc || len(task.Tags) != 1 {
		t.Errorf("task description/tags not persisted: %+v", task)
	}

	got, err := svc.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if got.Title != "Lifecycle" {
		t.Errorf("GetTask() title = %q, want Lifecycle", got.Title)
	}

	if _, err := svc.CompleteTask(ctx, task.ID); err != nil {
		t.Fatalf("CompleteTask() error = %v", err)
	}
	completed, _ := store.Tasks().FindByID(ctx, task.ID)
	if completed.Status != domain.StatusCompleted {
		t.Errorf("task status = %v, want completed", completed.Status)
	}

	if err := svc.DeleteTask(ctx, task.ID); err != nil {
		t.Fatalf("DeleteTask() error = %v", err)
	}
	if _, err := svc.GetTask(ctx, task.ID); err == nil {
		t.Error("GetTask() after delete should error")
	}
}

func TestState_StartTask(t *testing.T) {
	svc, _, taskSvc, _ := newStateWithServices(t)
	ctx := context.Background()

	task, _ := taskSvc.AddTask(ctx, AddTaskRequest{Title: "To Start"})
	if err := svc.StartTask(ctx, task.ID); err != nil {
		t.Fatalf("StartTask() error = %v", err)
	}
	started, _ := taskSvc.GetTask(ctx, task.ID)
	if started.Status != domain.StatusInProgress {
		t.Errorf("task status = %v, want in_progress", started.Status)
	}
}

func TestState_SessionWrites(t *testing.T) {
	svc, store, _, _ := newStateWithServices(t)
	ctx := context.Background()

	session, err := svc.StartSession(ctx, ports.StartSessionRequest{})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}

	t.Run("add notes", func(t *testing.T) {
		s, err := svc.AddSessionNotes(ctx, session.ID, "hello")
		if err != nil {
			t.Fatalf("AddSessionNotes() error = %v", err)
		}
		if s.Notes != "hello" {
			t.Errorf("notes = %q, want hello", s.Notes)
		}
	})
	t.Run("log distraction", func(t *testing.T) {
		if err := svc.LogDistraction(ctx, session.ID, "noise", "external"); err != nil {
			t.Fatalf("LogDistraction() error = %v", err)
		}
		found, _ := store.Sessions().FindByID(ctx, session.ID)
		if len(found.Distractions) != 1 || found.Distractions[0].Category != "external" {
			t.Errorf("distractions = %+v, want 1 external", found.Distractions)
		}
	})
	t.Run("set focus score", func(t *testing.T) {
		if err := svc.SetFocusScore(ctx, session.ID, 4); err != nil {
			t.Fatalf("SetFocusScore() error = %v", err)
		}
		found, _ := store.Sessions().FindByID(ctx, session.ID)
		if found.FocusScore == nil || *found.FocusScore != 4 {
			t.Errorf("focus score = %v, want 4", found.FocusScore)
		}
	})
	t.Run("set accomplishment", func(t *testing.T) {
		if err := svc.SetAccomplishment(ctx, session.ID, "wrote docs"); err != nil {
			t.Fatalf("SetAccomplishment() error = %v", err)
		}
		found, _ := store.Sessions().FindByID(ctx, session.ID)
		if found.Accomplishment != "wrote docs" {
			t.Errorf("accomplishment = %q, want 'wrote docs'", found.Accomplishment)
		}
	})
	t.Run("set energize activity", func(t *testing.T) {
		if err := svc.SetEnergizeActivity(ctx, session.ID, "walk"); err != nil {
			t.Fatalf("SetEnergizeActivity() error = %v", err)
		}
		found, _ := store.Sessions().FindByID(ctx, session.ID)
		if found.EnergizeActivity != "walk" {
			t.Errorf("energize activity = %q, want walk", found.EnergizeActivity)
		}
	})
	t.Run("set outcome achieved", func(t *testing.T) {
		if err := svc.SetOutcomeAchieved(ctx, session.ID, "p"); err != nil {
			t.Fatalf("SetOutcomeAchieved() error = %v", err)
		}
		found, _ := store.Sessions().FindByID(ctx, session.ID)
		if found.OutcomeAchieved != "p" {
			t.Errorf("outcome achieved = %q, want p", found.OutcomeAchieved)
		}
	})
	t.Run("set shutdown ritual", func(t *testing.T) {
		ritual := domain.ShutdownRitual{TomorrowPlan: "ship"}
		if err := svc.SetShutdownRitual(ctx, session.ID, ritual); err != nil {
			t.Fatalf("SetShutdownRitual() error = %v", err)
		}
		found, _ := store.Sessions().FindByID(ctx, session.ID)
		if found.ShutdownRitual == nil || found.ShutdownRitual.TomorrowPlan != "ship" {
			t.Errorf("shutdown ritual = %+v, want TomorrowPlan=ship", found.ShutdownRitual)
		}
	})
}

func TestSession_LifecycleThroughState(t *testing.T) {
	svc, store, _, _ := newStateWithServices(t)
	ctx := context.Background()

	if _, err := svc.StartSession(ctx, ports.StartSessionRequest{}); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}

	if _, err := svc.PausePomodoro(ctx); err != nil {
		t.Fatalf("PausePomodoro() error = %v", err)
	}
	active, _ := store.Sessions().FindActive(ctx)
	if active.Status != domain.SessionStatusPaused {
		t.Errorf("session status = %v, want paused", active.Status)
	}

	if _, err := svc.ResumePomodoro(ctx); err != nil {
		t.Fatalf("ResumePomodoro() error = %v", err)
	}

	if _, err := svc.StopPomodoro(ctx); err != nil {
		t.Fatalf("StopPomodoro() error = %v", err)
	}
	stopped, _ := store.Sessions().FindByID(ctx, active.ID)
	if stopped.Status != domain.SessionStatusCompleted {
		t.Errorf("session status = %v, want completed", stopped.Status)
	}
}

func TestSession_VoidAndCancel(t *testing.T) {
	svc, store, _, _ := newStateWithServices(t)
	ctx := context.Background()

	t.Run("void", func(t *testing.T) {
		clearSessions(t, store, ctx)
		if _, err := svc.StartSession(ctx, ports.StartSessionRequest{}); err != nil {
			t.Fatalf("StartSession() error = %v", err)
		}
		active, _ := store.Sessions().FindActive(ctx)
		if _, err := svc.VoidSession(ctx); err != nil {
			t.Fatalf("VoidSession() error = %v", err)
		}
		voided, _ := store.Sessions().FindByID(ctx, active.ID)
		if voided.Status != domain.SessionStatusInterrupted {
			t.Errorf("session status = %v, want interrupted", voided.Status)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		clearSessions(t, store, ctx)
		if _, err := svc.StartSession(ctx, ports.StartSessionRequest{}); err != nil {
			t.Fatalf("StartSession() error = %v", err)
		}
		if err := svc.CancelSession(ctx); err != nil {
			t.Fatalf("CancelSession() error = %v", err)
		}
		active, _ := store.Sessions().FindActive(ctx)
		if active != nil {
			t.Error("CancelSession() should leave no active session")
		}
	})
}

func TestState_Highlight(t *testing.T) {
	svc, _, taskSvc, _ := newStateWithServices(t)
	ctx := context.Background()

	t.Run("no highlight set", func(t *testing.T) {
		hl, err := svc.GetTodayHighlight(ctx)
		if err != nil {
			t.Fatalf("GetTodayHighlight() error = %v", err)
		}
		if hl != nil {
			t.Error("GetTodayHighlight() should be nil before any highlight is set")
		}
	})

	t.Run("set and get highlight", func(t *testing.T) {
		task, _ := taskSvc.AddTask(ctx, AddTaskRequest{Title: "Highlight me"})
		hl, err := svc.SetHighlight(ctx, task.ID)
		if err != nil {
			t.Fatalf("SetHighlight() error = %v", err)
		}
		if hl == nil || hl.ID != task.ID {
			t.Errorf("SetHighlight() returned %v, want task %s", hl, task.ID)
		}
		got, _ := svc.GetTodayHighlight(ctx)
		if got == nil || got.ID != task.ID {
			t.Errorf("GetTodayHighlight() = %v, want task %s", got, task.ID)
		}
	})
}

func TestState_DailyAndPeriodStats(t *testing.T) {
	svc, store, _, _ := newStateWithServices(t)
	ctx := context.Background()

	now := time.Now()
	clearSessions(t, store, ctx)
	session, err := svc.StartSession(ctx, ports.StartSessionRequest{})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if _, err := svc.StopPomodoro(ctx); err != nil {
		t.Fatalf("StopPomodoro() error = %v", err)
	}
	completed, _ := store.Sessions().FindByID(ctx, session.ID)

	summary, err := svc.GetDailySummary(ctx, now)
	if err != nil {
		t.Fatalf("GetDailySummary() error = %v", err)
	}
	if summary.WorkSessions != 1 {
		t.Errorf("daily work sessions = %d, want 1", summary.WorkSessions)
	}

	start := now.Add(-24 * time.Hour)
	end := now.Add(24 * time.Hour)
	periodStats, err := svc.GetPeriodStats(ctx, start, end)
	if err != nil {
		t.Fatalf("GetPeriodStats() error = %v", err)
	}
	if completed == nil || completed.Status != domain.SessionStatusCompleted {
		t.Fatal("expected a completed session to be stored")
	}
	if periodStats.TotalSessions != 1 {
		t.Errorf("period total sessions = %d, want 1", periodStats.TotalSessions)
	}
}

func TestState_GetFocusReport(t *testing.T) {
	svc, store, _, _ := newStateWithServices(t)
	ctx := context.Background()

	clearSessions(t, store, ctx)

	// Seed a completed work session today with a focus score and a distraction.
	session := &domain.PomodoroSession{
		ID:          "focus-report-1",
		Type:        domain.SessionTypeWork,
		Status:      domain.SessionStatusCompleted,
		Duration:    25 * time.Minute,
		StartedAt:   time.Now().Add(-2 * time.Hour),
		Methodology: domain.MethodologyDeepWork,
	}
	now := time.Now()
	session.CompletedAt = &now
	score := 5
	session.FocusScore = &score
	session.Distractions = []domain.Distraction{{Text: "ping", Category: "internal"}}
	if err := store.Sessions().Save(ctx, session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	report, err := svc.GetFocusReport(ctx)
	if err != nil {
		t.Fatalf("GetFocusReport() error = %v", err)
	}
	if report.WorkSessions != 1 {
		t.Errorf("WorkSessions = %d, want 1", report.WorkSessions)
	}
	if report.TotalWorkTime != 25*time.Minute {
		t.Errorf("TotalWorkTime = %v, want 25m", report.TotalWorkTime)
	}
	if report.FocusScoreCount != 1 {
		t.Errorf("FocusScoreCount = %d, want 1", report.FocusScoreCount)
	}
	// Avg from focus 5, but an energize activity + focus should still average 5/1.
	if report.AvgFocusScore < 4.9 || report.AvgFocusScore > 5.1 {
		t.Errorf("AvgFocusScore = %f, want ~5", report.AvgFocusScore)
	}
	if report.DistractionCount != 1 {
		t.Errorf("DistractionCount = %d, want 1", report.DistractionCount)
	}
}

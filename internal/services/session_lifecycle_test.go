package services

import (
	"context"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

// startWithTask creates a task and starts a work session on it.
func startWithTask(t *testing.T, store ports.Storage, service *PomodoroService, ctx context.Context) *domain.Task {
	t.Helper()
	task, err := NewTaskService(store).AddTask(ctx, AddTaskRequest{Title: "write report"})
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	if _, err := service.StartPomodoro(ctx, StartPomodoroRequest{TaskID: &task.ID, Duration: 25 * time.Minute}); err != nil {
		t.Fatalf("StartPomodoro() error = %v", err)
	}
	return task
}

func taskStatus(t *testing.T, store ports.Storage, ctx context.Context, id string) domain.TaskStatus {
	t.Helper()
	task, err := store.Tasks().FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	return task.Status
}

// expire backdates the active session so its timer has fully elapsed.
func expire(t *testing.T, store ports.Storage, ctx context.Context) {
	t.Helper()
	session, err := store.Sessions().FindActive(ctx)
	if err != nil || session == nil {
		t.Fatalf("FindActive() = %v, %v", session, err)
	}
	session.StartedAt = time.Now().Add(-3 * time.Hour)
	if err := store.Sessions().Update(ctx, session); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
}

// TestPomodoroService_EndingSessionReleasesTask guards that every way a
// session ends (stop, cancel, void, and the auto-complete of an expired
// session, whichever caller triggers it, MCP included) moves its task from
// in_progress back to pending.
func TestPomodoroService_EndingSessionReleasesTask(t *testing.T) {
	ctx := context.Background()
	ends := map[string]func(*PomodoroService, *StateService) error{
		"stop": func(p *PomodoroService, _ *StateService) error {
			_, err := p.StopSession(ctx)
			return err
		},
		"cancel": func(p *PomodoroService, _ *StateService) error {
			return p.CancelSession(ctx)
		},
		"void": func(p *PomodoroService, _ *StateService) error {
			_, err := p.VoidSession(ctx)
			return err
		},
		"mcp stop": func(_ *PomodoroService, s *StateService) error {
			_, err := s.StopPomodoro(ctx)
			return err
		},
		"mcp cancel": func(_ *PomodoroService, s *StateService) error {
			return s.CancelSession(ctx)
		},
		"mcp void": func(_ *PomodoroService, s *StateService) error {
			_, err := s.VoidSession(ctx)
			return err
		},
	}
	for name, end := range ends {
		t.Run(name, func(t *testing.T) {
			store, cleanup := setupTestStorage(t)
			defer cleanup()
			service := NewPomodoroService(store, nil)
			state := NewStateService(store, NewTaskService(store), service)
			task := startWithTask(t, store, service, ctx)
			if got := taskStatus(t, store, ctx, task.ID); got != domain.StatusInProgress {
				t.Fatalf("task status while running = %q, want in_progress", got)
			}
			if err := end(service, state); err != nil {
				t.Fatalf("%s error = %v", name, err)
			}
			if got := taskStatus(t, store, ctx, task.ID); got != domain.StatusPending {
				t.Errorf("task status after %s = %q, want pending", name, got)
			}
		})
	}

	autoCompletes := map[string]func(*PomodoroService, *StateService) error{
		"get current state": func(_ *PomodoroService, s *StateService) error {
			_, err := s.GetCurrentState(ctx)
			return err
		},
		"start break": func(p *PomodoroService, _ *StateService) error {
			_, err := p.StartBreak(ctx, "")
			return err
		},
		"pause": func(p *PomodoroService, _ *StateService) error {
			if _, err := p.PauseSession(ctx); err != domain.ErrNoActiveSession {
				t.Errorf("PauseSession() error = %v, want ErrNoActiveSession", err)
			}
			return nil
		},
		"start pomodoro": func(p *PomodoroService, _ *StateService) error {
			_, err := p.StartPomodoro(ctx, StartPomodoroRequest{})
			return err
		},
	}
	for name, trigger := range autoCompletes {
		t.Run("expired via "+name, func(t *testing.T) {
			store, cleanup := setupTestStorage(t)
			defer cleanup()
			service := NewPomodoroService(store, nil)
			state := NewStateService(store, NewTaskService(store), service)
			task := startWithTask(t, store, service, ctx)
			expire(t, store, ctx)
			if err := trigger(service, state); err != nil {
				t.Fatalf("%s error = %v", name, err)
			}
			if got := taskStatus(t, store, ctx, task.ID); got != domain.StatusPending {
				t.Errorf("task status after auto-complete via %s = %q, want pending", name, got)
			}
		})
	}

	t.Run("completed task stays completed", func(t *testing.T) {
		store, cleanup := setupTestStorage(t)
		defer cleanup()
		service := NewPomodoroService(store, nil)
		tasks := NewTaskService(store)
		task := startWithTask(t, store, service, ctx)
		if err := tasks.CompleteTask(ctx, task.ID); err != nil {
			t.Fatalf("CompleteTask() error = %v", err)
		}
		if _, err := service.StopSession(ctx); err != nil {
			t.Fatalf("StopSession() error = %v", err)
		}
		if got := taskStatus(t, store, ctx, task.ID); got != domain.StatusCompleted {
			t.Errorf("task status after stop = %q, want completed", got)
		}
	})

	t.Run("deleted task does not fail the stop", func(t *testing.T) {
		store, cleanup := setupTestStorage(t)
		defer cleanup()
		service := NewPomodoroService(store, nil)
		task := startWithTask(t, store, service, ctx)
		if err := NewTaskService(store).DeleteTask(ctx, task.ID); err != nil {
			t.Fatalf("DeleteTask() error = %v", err)
		}
		if _, err := service.StopSession(ctx); err != nil {
			t.Errorf("StopSession() error = %v, want nil for a deleted task", err)
		}
	})
}

// TestStateService_StartSession_MethodologyDefaults guards that MCP sessions
// started without a methodology or duration use the configured methodology
// and that methodology's default duration, while explicit arguments win.
func TestStateService_StartSession_MethodologyDefaults(t *testing.T) {
	ctx := context.Background()
	durations := map[domain.Methodology]time.Duration{
		domain.MethodologyPomodoro: 25 * time.Minute,
		domain.MethodologyDeepWork: 90 * time.Minute,
		domain.MethodologyMakeTime: 60 * time.Minute,
	}
	thirty := 30
	tests := []struct {
		name     string
		req      ports.StartSessionRequest
		wantMeth domain.Methodology
		wantDur  time.Duration
	}{
		{"no arguments", ports.StartSessionRequest{}, domain.MethodologyDeepWork, 90 * time.Minute},
		{"explicit methodology", ports.StartSessionRequest{Methodology: domain.MethodologyMakeTime}, domain.MethodologyMakeTime, 60 * time.Minute},
		{"explicit duration", ports.StartSessionRequest{DurationMinutes: &thirty}, domain.MethodologyDeepWork, 30 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, cleanup := setupTestStorage(t)
			defer cleanup()
			service := NewPomodoroService(store, nil)
			service.SetMethodologyDefaults(domain.MethodologyDeepWork, durations)
			state := NewStateService(store, NewTaskService(store), service)

			session, err := state.StartSession(ctx, tt.req)
			if err != nil {
				t.Fatalf("StartSession() error = %v", err)
			}
			stored, err := store.Sessions().FindByID(ctx, session.ID)
			if err != nil {
				t.Fatalf("FindByID() error = %v", err)
			}
			if stored.Methodology != tt.wantMeth || stored.Duration != tt.wantDur {
				t.Errorf("stored session = %s/%v, want %s/%v", stored.Methodology, stored.Duration, tt.wantMeth, tt.wantDur)
			}
		})
	}

	t.Run("no defaults configured", func(t *testing.T) {
		store, cleanup := setupTestStorage(t)
		defer cleanup()
		service := NewPomodoroService(store, nil)
		state := NewStateService(store, NewTaskService(store), service)
		session, err := state.StartSession(ctx, ports.StartSessionRequest{})
		if err != nil {
			t.Fatalf("StartSession() error = %v", err)
		}
		if session.Duration != domain.DefaultPomodoroConfig().WorkDuration {
			t.Errorf("duration = %v, want the configured work duration", session.Duration)
		}
	})
}

// TestPomodoroService_StartBreak_RecordsMethodology guards that breaks read
// back with the methodology they were started in, not as Pomodoro.
func TestPomodoroService_StartBreak_RecordsMethodology(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	service := NewPomodoroService(store, nil)
	service.SetMethodologyDefaults(domain.MethodologyMakeTime, nil)

	session, err := service.StartBreak(ctx, "")
	if err != nil {
		t.Fatalf("StartBreak() error = %v", err)
	}
	stored, err := store.Sessions().FindByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if stored.Methodology != domain.MethodologyMakeTime {
		t.Errorf("break methodology = %q, want maketime", stored.Methodology)
	}
}

// txFlagStorage records whether a WithTx transaction is open.
type txFlagStorage struct {
	ports.Storage
	inTx *bool
}

func (s txFlagStorage) WithTx(ctx context.Context, fn func(tx ports.Storage) error) error {
	*s.inTx = true
	defer func() { *s.inTx = false }()
	return s.Storage.WithTx(ctx, fn)
}

// txCheckingDetector fails the test when git runs inside a transaction.
type txCheckingDetector struct {
	t     *testing.T
	inTx  *bool
	calls int
}

func (d *txCheckingDetector) IsAvailable() bool { return true }

func (d *txCheckingDetector) Detect(context.Context, string) (*ports.GitInfo, error) {
	d.calls++
	if *d.inTx {
		d.t.Error("git detection ran inside a transaction")
	}
	return &ports.GitInfo{Branch: "main", Commit: "abc123"}, nil
}

// TestPomodoroService_GitDetectedBeforeTransaction guards that git runs
// before the write-locking transaction opens, and that the gathered context
// is still recorded on the session.
func TestPomodoroService_GitDetectedBeforeTransaction(t *testing.T) {
	ctx := context.Background()
	starts := map[string]func(*PomodoroService, *StateService, ports.Storage) (*domain.PomodoroSession, error){
		"start pomodoro": func(p *PomodoroService, _ *StateService, _ ports.Storage) (*domain.PomodoroSession, error) {
			return p.StartPomodoro(ctx, StartPomodoroRequest{WorkingDir: "."})
		},
		"state start session": func(_ *PomodoroService, s *StateService, _ ports.Storage) (*domain.PomodoroSession, error) {
			return s.StartSession(ctx, ports.StartSessionRequest{TaskTitle: "t"})
		},
		"start pomodoro with, caller tx": func(p *PomodoroService, _ *StateService, store ports.Storage) (*domain.PomodoroSession, error) {
			req := StartPomodoroRequest{WorkingDir: ".", Git: p.DetectGit(ctx, ".")}
			var session *domain.PomodoroSession
			err := store.WithTx(ctx, func(tx ports.Storage) error {
				var err error
				session, err = p.StartPomodoroWith(ctx, tx, req)
				return err
			})
			return session, err
		},
	}
	for name, start := range starts {
		t.Run(name, func(t *testing.T) {
			base, cleanup := setupTestStorage(t)
			defer cleanup()
			inTx := false
			store := txFlagStorage{Storage: base, inTx: &inTx}
			detector := &txCheckingDetector{t: t, inTx: &inTx}
			service := NewPomodoroService(store, detector)
			state := NewStateService(store, NewTaskService(store), service)

			session, err := start(service, state, store)
			if err != nil {
				t.Fatalf("start error = %v", err)
			}
			if detector.calls != 1 {
				t.Errorf("git detected %d times, want 1", detector.calls)
			}
			stored, err := base.Sessions().FindByID(ctx, session.ID)
			if err != nil {
				t.Fatalf("FindByID() error = %v", err)
			}
			if stored.GitBranch != "main" || stored.GitCommit != "abc123" {
				t.Errorf("stored git context = %q/%q, want main/abc123", stored.GitBranch, stored.GitCommit)
			}
		})
	}
}

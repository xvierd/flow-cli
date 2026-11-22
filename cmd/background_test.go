package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/services"
)

// setupBackgroundApp initializes the services on a temp database with
// notifications off and the on_end hook writing its environment to a file,
// whose path it returns.
func setupBackgroundApp(t *testing.T) string {
	t.Helper()
	isolateApp(t)
	dbPath = filepath.Join(t.TempDir(), "flow.db")
	if err := initializeServices(); err != nil {
		t.Fatalf("initializeServices() error = %v", err)
	}
	app.config.Notifications.Enabled = false
	out := filepath.Join(t.TempDir(), "hook.env")
	app.config.Hooks.OnEnd = `env | grep '^FLOW_' > "` + out + `"`
	return out
}

// saveExpiredSession stores a running work session whose timer ran out.
func saveExpiredSession(t *testing.T, taskID *string) *domain.PomodoroSession {
	t.Helper()
	s := domain.NewPomodoroSession(domain.DefaultPomodoroConfig(), taskID)
	s.Duration = time.Minute
	s.StartedAt = time.Now().Add(-2 * time.Minute)
	if err := app.storage.Sessions().Save(context.Background(), s); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	return s
}

func TestWatchActiveSessions_CompletesAndAnnounces(t *testing.T) {
	out := setupBackgroundApp(t)
	ctx := context.Background()

	task, err := app.tasks.AddTask(ctx, servicesAddTask("Write docs"))
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	s := saveExpiredSession(t, &task.ID)

	var sleeps int
	watchActiveSessions(ctx, heartbeatPath("watcher"), func(time.Duration) { sleeps++ })

	got, err := app.storage.Sessions().FindByID(ctx, s.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if got.Status != domain.SessionStatusCompleted {
		t.Errorf("session status = %s, want completed", got.Status)
	}
	if sleeps != 0 {
		t.Errorf("watcher slept %d times for an already expired session", sleeps)
	}

	env, err := os.ReadFile(out) // #nosec G304 -- test temp file
	if err != nil {
		t.Fatalf("on_end hook did not run: %v", err)
	}
	for _, want := range []string{"FLOW_EVENT=end", "FLOW_SESSION_ID=" + s.ID, "FLOW_SESSION_TYPE=work", "FLOW_TASK=Write docs", "FLOW_METHODOLOGY=pomodoro"} {
		if !strings.Contains(string(env), want) {
			t.Errorf("hook env missing %q:\n%s", want, env)
		}
	}
}

func TestWatchActiveSessions_TUIAttachedSkipsAnnouncement(t *testing.T) {
	out := setupBackgroundApp(t)
	ctx := context.Background()
	s := saveExpiredSession(t, nil)
	touchHeartbeat(heartbeatPath("tui"))

	watchActiveSessions(ctx, heartbeatPath("watcher"), func(time.Duration) {})

	got, _ := app.storage.Sessions().FindByID(ctx, s.ID)
	if got.Status != domain.SessionStatusCompleted {
		t.Errorf("session status = %s, want completed", got.Status)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("on_end hook ran although a TUI owns the announcement")
	}
}

func TestWatchActiveSessions_WaitsForRunningSession(t *testing.T) {
	setupBackgroundApp(t)
	ctx := context.Background()
	s := domain.NewPomodoroSession(domain.DefaultPomodoroConfig(), nil)
	if err := app.storage.Sessions().Save(ctx, s); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var slept []time.Duration
	watchActiveSessions(ctx, heartbeatPath("watcher"), func(d time.Duration) {
		slept = append(slept, d)
		// Stand in for the time passing: the user stops it meanwhile.
		if _, err := app.pomodoro.StopSession(ctx); err != nil {
			t.Fatalf("StopSession() error = %v", err)
		}
	})
	if len(slept) != 1 || slept[0] != watchPollInterval {
		t.Errorf("slept %v, want one poll interval (%v)", slept, watchPollInterval)
	}
}

func TestEnsureWatcher(t *testing.T) {
	setupBackgroundApp(t)
	t.Setenv(noWatcherEnv, "")

	var spawned int
	old := spawnWatcher
	spawnWatcher = func() error { spawned++; return nil }
	t.Cleanup(func() { spawnWatcher = old })

	ensureWatcher()
	if spawned != 1 {
		t.Fatalf("spawned = %d, want 1", spawned)
	}

	touchHeartbeat(heartbeatPath("watcher"))
	ensureWatcher()
	if spawned != 1 {
		t.Errorf("spawned again with a live watcher: %d", spawned)
	}

	t.Setenv(noWatcherEnv, "1")
	_ = os.Remove(heartbeatPath("watcher"))
	ensureWatcher()
	if spawned != 1 {
		t.Errorf("spawned with %s set: %d", noWatcherEnv, spawned)
	}
}

func TestRunHook_UnsetIsNoop(t *testing.T) {
	setupBackgroundApp(t)
	app.config.Hooks.OnStart = ""
	if err := runHook(context.Background(), "start", domain.NewPomodoroSession(domain.DefaultPomodoroConfig(), nil)); err != nil {
		t.Errorf("runHook() with no hook = %v, want nil", err)
	}
}

func TestSplitTags(t *testing.T) {
	got := splitTags([]string{"a, b", "", " c ", "d,"})
	want := []string{"a", "b", "c", "d"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("splitTags() = %v, want %v", got, want)
	}
	if got := splitTags([]string{""}); len(got) != 0 {
		t.Errorf("splitTags(\"\") = %v, want empty", got)
	}
}

func servicesAddTask(title string) services.AddTaskRequest {
	return services.AddTaskRequest{Title: title}
}

// TestWatchActiveSessions_AnnouncesSessionCompletedElsewhere covers a state
// read (flow status, the status line, MCP) completing the expired session
// before the watcher wakes up: the watcher must still announce it.
func TestWatchActiveSessions_AnnouncesSessionCompletedElsewhere(t *testing.T) {
	out := setupBackgroundApp(t)
	ctx := context.Background()
	s := domain.NewPomodoroSession(domain.DefaultPomodoroConfig(), nil)
	s.Duration = 300 * time.Millisecond
	if err := app.storage.Sessions().Save(ctx, s); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	watchActiveSessions(ctx, heartbeatPath("watcher"), func(d time.Duration) {
		// The session runs out while the watcher sleeps, and another
		// process reads the state first, which completes it.
		time.Sleep(d + 50*time.Millisecond)
		if _, err := app.state.GetCurrentState(ctx); err != nil {
			t.Fatalf("GetCurrentState() error = %v", err)
		}
	})

	env, err := os.ReadFile(out) // #nosec G304 -- test temp file
	if err != nil {
		t.Fatalf("on_end hook did not run for a session completed elsewhere: %v", err)
	}
	if !strings.Contains(string(env), "FLOW_SESSION_ID="+s.ID) {
		t.Errorf("hook env = %s, want session %s", env, s.ID)
	}
}

// TestWatchActiveSessions_ManualStopNotAnnounced: a session stopped before
// its time is announced by whoever stopped it, never by the watcher.
func TestWatchActiveSessions_ManualStopNotAnnounced(t *testing.T) {
	out := setupBackgroundApp(t)
	ctx := context.Background()
	s := domain.NewPomodoroSession(domain.DefaultPomodoroConfig(), nil)
	if err := app.storage.Sessions().Save(ctx, s); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	watchActiveSessions(ctx, heartbeatPath("watcher"), func(time.Duration) {
		if _, err := app.pomodoro.StopSession(ctx); err != nil {
			t.Fatalf("StopSession() error = %v", err)
		}
	})
	if _, err := os.Stat(out); err == nil {
		t.Error("watcher announced a session that was stopped early")
	}
}

// TestAnnounceSessionEnd_DefersToTUI: with a TUI open, stops from another
// process (flow stop, MCP) leave the announcement to the TUI.
func TestAnnounceSessionEnd_DefersToTUI(t *testing.T) {
	out := setupBackgroundApp(t)
	s := domain.NewPomodoroSession(domain.DefaultPomodoroConfig(), nil)

	touchHeartbeat(heartbeatPath("tui"))
	announceSessionEnd(context.Background(), s, s.Duration)
	if _, err := os.Stat(out); err == nil {
		t.Fatal("on_end hook ran although a TUI is open")
	}

	_ = os.Remove(heartbeatPath("tui"))
	announceSessionEnd(context.Background(), s, s.Duration)
	if _, err := os.Stat(out); err != nil {
		t.Errorf("on_end hook did not run without a TUI: %v", err)
	}
}

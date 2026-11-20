package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/services"
)

func workState(m domain.Methodology) *domain.CurrentState {
	return &domain.CurrentState{ActiveSession: &domain.PomodoroSession{
		ID: "s", Type: domain.SessionTypeWork, Methodology: m,
	}}
}

// TestChainSessions_NewSessionRequestStartsNext guards T4: pressing 'n' on the
// completion screen must start the new-session flow for every caller.
func TestChainSessions_NewSessionRequestStartsNext(t *testing.T) {
	isolateApp(t)
	var launched []*domain.CurrentState
	requests := []bool{true, false}
	nexts := 0
	second := &domain.CurrentState{}

	err := chainSessions(context.Background(), &domain.CurrentState{},
		func(_ context.Context, s *domain.CurrentState) error {
			launched = append(launched, s)
			return nil
		},
		func() bool { r := requests[0]; requests = requests[1:]; return r },
		func(context.Context) (*domain.CurrentState, error) { nexts++; return second, nil },
	)
	if err != nil {
		t.Fatalf("chainSessions() error = %v", err)
	}
	if len(launched) != 2 || launched[1] != second || nexts != 1 {
		t.Fatalf("launches = %d (second ok: %v), next calls = %d; want 2 launches, 1 next",
			len(launched), len(launched) == 2 && launched[1] == second, nexts)
	}
}

func TestChainSessions_StopsOnAbortCancelAndError(t *testing.T) {
	isolateApp(t)
	launches := 0
	launch := func(context.Context, *domain.CurrentState) error { launches++; return nil }
	always := func() bool { return true }

	// Aborted prompts (nil state) end the chain.
	if err := chainSessions(context.Background(), &domain.CurrentState{}, launch, always,
		func(context.Context) (*domain.CurrentState, error) { return nil, nil }); err != nil || launches != 1 {
		t.Fatalf("abort: err = %v, launches = %d; want nil, 1", err, launches)
	}

	// A cancelled context (signal) never prompts for another session.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	launches = 0
	if err := chainSessions(ctx, &domain.CurrentState{}, launch, always,
		func(context.Context) (*domain.CurrentState, error) {
			t.Fatal("next called after cancellation")
			return nil, nil
		}); err != nil || launches != 1 {
		t.Fatalf("cancel: err = %v, launches = %d; want nil, 1", err, launches)
	}

	// Errors from next propagate.
	boom := errors.New("boom")
	if err := chainSessions(context.Background(), &domain.CurrentState{}, launch, always,
		func(context.Context) (*domain.CurrentState, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Fatalf("next error = %v, want %v", err, boom)
	}
}

// TestChainSessions_AdoptsSessionMethodology guards T5: launching for an
// active session uses the session's methodology, not the configured one.
func TestChainSessions_AdoptsSessionMethodology(t *testing.T) {
	isolateApp(t)
	dbPath = filepath.Join(t.TempDir(), "flow.db")
	modeFlag = "pomodoro"
	if err := initializeServices(); err != nil {
		t.Fatalf("initializeServices() error = %v", err)
	}

	var seen domain.Methodology
	err := chainSessions(context.Background(), workState(domain.MethodologyMakeTime),
		func(context.Context, *domain.CurrentState) error {
			seen = app.mode.Name()
			return nil
		},
		func() bool { return false }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if seen != domain.MethodologyMakeTime || app.methodology != domain.MethodologyMakeTime {
		t.Fatalf("mode at launch = %q, methodology = %q; want maketime", seen, app.methodology)
	}
}

func TestSessionMethodology(t *testing.T) {
	brk := func(m domain.Methodology) *domain.CurrentState {
		return &domain.CurrentState{ActiveSession: &domain.PomodoroSession{
			Type: domain.SessionTypeShortBreak, Methodology: m,
		}}
	}
	tests := []struct {
		name  string
		state *domain.CurrentState
		want  domain.Methodology
		ok    bool
	}{
		{"nil state", nil, "", false},
		{"no session", &domain.CurrentState{}, "", false},
		{"work session", workState(domain.MethodologyDeepWork), domain.MethodologyDeepWork, true},
		{"empty methodology", workState(""), "", false},
		// Breaks record their methodology, but old break rows read back as
		// pomodoro whatever their methodology was: a pomodoro break is not
		// trusted, any other methodology is.
		{"pomodoro break", brk(domain.MethodologyPomodoro), "", false},
		{"deep work break", brk(domain.MethodologyDeepWork), domain.MethodologyDeepWork, true},
		{"make time break", brk(domain.MethodologyMakeTime), domain.MethodologyMakeTime, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := sessionMethodology(tt.state)
			if got != tt.want || ok != tt.ok {
				t.Errorf("sessionMethodology() = %q, %v; want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

// TestSetupSignalHandler_SingleRegistration guards T13a: repeated launches
// share one handler instead of registering a new one each time.
func TestSetupSignalHandler_SingleRegistration(t *testing.T) {
	if a, b := setupSignalHandler(), setupSignalHandler(); a != b {
		t.Fatal("setupSignalHandler returned different contexts; handler registered more than once")
	}
}

func TestCancelOnSignal(t *testing.T) {
	sig := make(chan os.Signal, 1)
	stopped := make(chan struct{})
	ctx := cancelOnSignal(sig, func() { close(stopped) })
	if ctx.Err() != nil {
		t.Fatal("context cancelled before any signal")
	}
	sig <- syscall.SIGINT
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("context not cancelled after signal")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("signal registration not released after the first signal")
	}
}

// TestReadLine_Cancellation guards T13a: Ctrl+C at a plain-terminal prompt
// (context cancelled by the signal handler) must not leave it waiting.
func TestReadLine_Cancellation(t *testing.T) {
	r, w := io.Pipe()
	t.Cleanup(func() { _ = w.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := readLine(ctx, r)
		done <- err
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("readLine() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("readLine did not return after cancellation")
	}
}

func TestReadLine_Input(t *testing.T) {
	for in, want := range map[string]string{"  ship it \nrest\n": "ship it", "no newline": "no newline"} {
		got, err := readLine(context.Background(), strings.NewReader(in))
		if err != nil || got != want {
			t.Errorf("readLine(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := readLine(context.Background(), strings.NewReader("")); !errors.Is(err, io.EOF) {
		t.Errorf("readLine(empty) error = %v, want EOF", err)
	}
}

// TestNewTomorrowHighlight_FoundTomorrow guards T13b: the task entered as
// tomorrow's Highlight is tomorrow's highlight (and not today's).
func TestNewTomorrowHighlight_FoundTomorrow(t *testing.T) {
	isolateApp(t)
	dbPath = filepath.Join(t.TempDir(), "flow.db")
	if err := initializeServices(); err != nil {
		t.Fatalf("initializeServices() error = %v", err)
	}
	ctx := context.Background()
	now := time.Now()

	task, err := newTomorrowHighlight("Write the proposal", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.storage.Tasks().Save(ctx, task); err != nil {
		t.Fatal(err)
	}

	got, err := app.storage.Tasks().FindTodayHighlight(ctx, now.AddDate(0, 0, 1))
	if err != nil || got == nil || got.ID != task.ID {
		t.Fatalf("FindTodayHighlight(tomorrow) = %v, %v; want task %s", got, err, task.ID)
	}
	if today, _ := app.storage.Tasks().FindTodayHighlight(ctx, now); today != nil {
		t.Errorf("tomorrow's highlight reported as today's: %v", today.Title)
	}
	if !task.IsHighlightForDate(now.AddDate(0, 0, 1)) {
		t.Errorf("HighlightDate = %v, not tomorrow", task.HighlightDate)
	}
}

// TestSaveConfig_DoesNotOverwriteBrokenConfig guards P1: after a config parse
// failure the defaults in memory must never be written over the user's file.
func TestSaveConfig_DoesNotOverwriteBrokenConfig(t *testing.T) {
	home := isolateApp(t)
	dbPath = filepath.Join(t.TempDir(), "flow.db")
	cfgPath := filepath.Join(home, ".flow", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o750); err != nil {
		t.Fatal(err)
	}
	broken := []byte("not = = valid [[[ toml")
	if err := os.WriteFile(cfgPath, broken, 0o600); err != nil {
		t.Fatal(err)
	}

	_ = captureStderr(t, func() {
		if err := initializeServices(); err != nil {
			t.Fatalf("initializeServices() error = %v", err)
		}
	})
	if app.config.FirstRun {
		t.Error("FirstRun = true after a config load failure; the wizard would show the welcome screen and save")
	}
	if err := saveConfig(app.config); err == nil {
		t.Error("saveConfig() succeeded over a config file that failed to load")
	}
	got, err := os.ReadFile(cfgPath) // #nosec G304 -- temp path built by the test
	if err != nil || string(got) != string(broken) {
		t.Fatalf("config file changed: %q (err %v)", got, err)
	}
}

func TestSaveConfig_SavesValidConfig(t *testing.T) {
	home := isolateApp(t)
	dbPath = filepath.Join(t.TempDir(), "flow.db")
	if err := initializeServices(); err != nil {
		t.Fatalf("initializeServices() error = %v", err)
	}
	app.config.Language = "es"
	if err := saveConfig(app.config); err != nil {
		t.Fatalf("saveConfig() error = %v", err)
	}
	got, err := os.ReadFile(filepath.Join(home, ".flow", "config.toml")) // #nosec G304 -- temp path built by the test
	if err != nil || !regexp.MustCompile(`(?m)^language\s*=\s*["']es["']`).Match(got) {
		t.Fatalf("saved config missing change: %q (err %v)", got, err)
	}
}

// TestCarryForwardHighlight_AskedOncePerRun guards T15: the carry-forward
// question is asked once per run, and Yes returns yesterday's task so the
// session uses it without a task picker.
func TestCarryForwardHighlight_AskedOncePerRun(t *testing.T) {
	isolateApp(t)
	dbPath = filepath.Join(t.TempDir(), "flow.db")
	if err := initializeServices(); err != nil {
		t.Fatalf("initializeServices() error = %v", err)
	}
	ctx := context.Background()
	task, err := newTomorrowHighlight("Yesterday's thing", time.Now().AddDate(0, 0, -2))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.storage.Tasks().Save(ctx, task); err != nil {
		t.Fatal(err)
	}

	asks := 0
	no := func(*domain.Task) bool { asks++; return false }
	w := newSessionWizard(t.TempDir())
	for i := 0; i < 3; i++ { // first session plus two chained ones
		if got := w.carryForwardHighlight(ctx, no); got != nil {
			t.Fatalf("No answer returned task %v", got.Title)
		}
	}
	if asks != 1 {
		t.Fatalf("carry-forward asked %d times in one run, want 1", asks)
	}

	yes := func(*domain.Task) bool { return true }
	got := newSessionWizard(t.TempDir()).carryForwardHighlight(ctx, yes)
	if got == nil || got.ID != task.ID {
		t.Fatalf("Yes returned %v, want yesterday's task %s", got, task.ID)
	}
	today, _ := app.storage.Tasks().FindTodayHighlight(ctx, time.Now())
	if today == nil || today.ID != task.ID {
		t.Fatalf("today's highlight = %v, want carried task", today)
	}
}

// TestEndSessionReleasesTask guards P2: a session stopped, cancelled or voided
// from the CLI or TUI must not leave its task listed as in progress. The
// release lives in PomodoroService, so no cmd-level step is needed.
func TestEndSessionReleasesTask(t *testing.T) {
	isolateApp(t)
	dbPath = filepath.Join(t.TempDir(), "flow.db")
	if err := initializeServices(); err != nil {
		t.Fatalf("initializeServices() error = %v", err)
	}
	ctx := context.Background()

	for _, end := range []string{"stop", "cancel", "void"} {
		t.Run(end, func(t *testing.T) {
			task, err := app.tasks.AddTask(ctx, services.AddTaskRequest{Title: "task " + end})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := app.pomodoro.StartPomodoro(ctx, services.StartPomodoroRequest{TaskID: &task.ID}); err != nil {
				t.Fatal(err)
			}
			switch end {
			case "stop":
				_, err = app.pomodoro.StopSession(ctx)
			case "cancel":
				err = app.pomodoro.CancelSession(ctx)
			default:
				_, err = app.pomodoro.VoidSession(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}

			got, err := app.storage.Tasks().FindByID(ctx, task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != domain.StatusPending {
				t.Errorf("task status after %s = %q, want pending", end, got.Status)
			}
		})
	}
}

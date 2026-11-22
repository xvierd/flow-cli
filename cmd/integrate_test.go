package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/xvierd/flow-cli/internal/adapters/integrations"
	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/services"
)

// recordIntegrations replaces the dispatcher with one that records events.
func recordIntegrations(t *testing.T) func() []integrations.Event {
	t.Helper()
	var mu sync.Mutex
	var events []integrations.Event
	old := dispatchIntegrations
	dispatchIntegrations = func(_ context.Context, ev integrations.Event) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	}
	t.Cleanup(func() { dispatchIntegrations = old })
	return func() []integrations.Event {
		mu.Lock()
		defer mu.Unlock()
		return append([]integrations.Event(nil), events...)
	}
}

func setupIntegrateApp(t *testing.T) string {
	t.Helper()
	home := isolateApp(t)
	dbPath = filepath.Join(t.TempDir(), "flow.db")
	if err := initializeServices(); err != nil {
		t.Fatalf("initializeServices() error = %v", err)
	}
	app.config.Notifications.Enabled = false
	oldDry, oldYes, oldConfirm := integrateDryRun, integrateYes, confirmChange
	t.Cleanup(func() { integrateDryRun, integrateYes, confirmChange = oldDry, oldYes, oldConfirm })
	integrateDryRun, integrateYes = false, false
	confirmChange = func(string) bool { return false }
	return home
}

func TestRunHook_DispatchesIntegrationsWithoutUserHook(t *testing.T) {
	setupIntegrateApp(t)
	events := recordIntegrations(t)
	ctx := context.Background()

	s := domain.NewPomodoroSession(domain.DefaultPomodoroConfig(), nil)
	// Nothing enabled: the dispatcher is not even called.
	_ = runHook(ctx, "start", s)
	if got := events(); len(got) != 0 {
		t.Fatalf("dispatched %v with no integration enabled", got)
	}

	task, err := app.tasks.AddTask(ctx, services.AddTaskRequest{Title: "Write docs"})
	if err != nil {
		t.Fatal(err)
	}
	s.TaskID = &task.ID
	app.config.Integrations.DND = true
	app.config.Hooks.OnStart = ""
	if err := runHook(ctx, "start", s); err != nil {
		t.Fatalf("runHook() = %v", err)
	}
	_ = runHook(ctx, "end", s)
	got := events()
	if len(got) != 2 {
		t.Fatalf("events = %+v, want start and end", got)
	}
	if got[0].Kind != "start" || got[0].SessionType != "work" || got[0].Task != "Write docs" || got[0].Duration != s.Duration {
		t.Errorf("start event = %+v", got[0])
	}
	if got[1].Kind != "end" {
		t.Errorf("end event = %+v", got[1])
	}
}

func TestVoid_UndoesIntegrations(t *testing.T) {
	setupIntegrateApp(t)
	events := recordIntegrations(t)
	ctx := context.Background()
	app.config.Integrations.Slack = true

	if _, err := app.pomodoro.StartPomodoro(ctx, services.StartPomodoroRequest{}); err != nil {
		t.Fatal(err)
	}
	if err := voidCmd.RunE(voidCmd, nil); err != nil {
		t.Fatalf("void = %v", err)
	}
	got := events()
	if len(got) != 1 || got[0].Kind != "end" {
		t.Errorf("events = %+v, want one end", got)
	}
}

func TestIntegrate_EnableDisableXbar(t *testing.T) {
	home := setupIntegrateApp(t)
	plugin := filepath.Join(home, "Library", "Application Support", "xbar", "plugins", "flow.5s.sh")

	integrateDryRun = true
	if err := runIntegrateToggle(context.Background(), "xbar", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plugin); err == nil {
		t.Fatal("--dry-run wrote the plugin")
	}
	if app.config.Integrations.Xbar {
		t.Fatal("--dry-run enabled the integration")
	}

	integrateDryRun = false
	if err := runIntegrateToggle(context.Background(), "xbar", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plugin); err != nil {
		t.Fatalf("plugin not written: %v", err)
	}
	loaded, err := config.Load()
	if err != nil || !loaded.Integrations.Xbar {
		t.Fatalf("xbar not saved in config: %+v %v", loaded.Integrations, err)
	}

	if err := runIntegrateToggle(context.Background(), "xbar", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plugin); !os.IsNotExist(err) {
		t.Errorf("plugin not removed: %v", err)
	}
	if loaded, _ := config.Load(); loaded.Integrations.Xbar {
		t.Error("xbar still enabled after disable")
	}
}

func TestIntegrate_TmuxNeedsConfirmation(t *testing.T) {
	home := setupIntegrateApp(t)
	conf := filepath.Join(home, ".tmux.conf")
	if err := os.WriteFile(conf, []byte("set -g mouse on\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var asked []string
	confirmChange = func(q string) bool { asked = append(asked, q); return false }
	if err := runIntegrateToggle(context.Background(), "tmux", true); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(conf); string(data) != "set -g mouse on\n" || len(asked) != 1 {
		t.Fatalf("declined edit changed .tmux.conf (%q) or asked %v", data, asked)
	}
	if app.config.Integrations.Tmux {
		t.Fatal("declined edit still enabled tmux")
	}

	integrateYes = true
	if err := runIntegrateToggle(context.Background(), "tmux", true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(conf)
	if !strings.Contains(string(data), "tmux-status") || !strings.HasPrefix(string(data), "set -g mouse on\n") {
		t.Errorf(".tmux.conf =\n%s", data)
	}
	if len(asked) != 1 {
		t.Errorf("--yes still asked: %v", asked)
	}
}

func TestIntegrate_UnknownName(t *testing.T) {
	setupIntegrateApp(t)
	err := runIntegrateToggle(context.Background(), "fax", true)
	if err == nil || !strings.Contains(err.Error(), "dnd") {
		t.Errorf("err = %v, want the list of integrations", err)
	}
}

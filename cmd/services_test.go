package cmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
)

// isolateApp points HOME at a temp dir, resets the package-level flag
// globals and tears down the services afterwards. It never touches the real
// ~/.flow.
func isolateApp(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FLOW_LANG", "en")

	oldDB, oldMode, oldStrict := dbPath, modeFlag, strictFlag
	t.Cleanup(func() {
		_ = cleanupServices()
		app = appDeps{}
		dbPath, modeFlag, strictFlag = oldDB, oldMode, oldStrict
		i18n.SetLanguage("en")
	})
	dbPath, modeFlag, strictFlag = "", "", false
	return home
}

// TestInitializeServices_BreakConfigUsesResolvedMethodology guards the
// ordering bug where the break config was built before app.methodology was
// resolved, so Deep Work breaks used Pomodoro durations.
func TestInitializeServices_BreakConfigUsesResolvedMethodology(t *testing.T) {
	isolateApp(t)
	dbPath = filepath.Join(t.TempDir(), "flow.db")
	modeFlag = "deepwork"

	if err := initializeServices(); err != nil {
		t.Fatalf("initializeServices() error = %v", err)
	}
	if app.methodology != domain.MethodologyDeepWork {
		t.Fatalf("methodology = %q, want deepwork", app.methodology)
	}

	session, err := app.pomodoro.StartBreak(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("StartBreak() error = %v", err)
	}
	want := time.Duration(app.config.DeepWork.BreakDuration)
	if session.Duration != want {
		t.Errorf("deep work break duration = %v, want %v", session.Duration, want)
	}
}

// TestSetMethodology_ReappliesBreakConfig covers mode switches after startup
// (TUI mode picker, wizard): the service must follow the new methodology.
func TestSetMethodology_ReappliesBreakConfig(t *testing.T) {
	isolateApp(t)
	dbPath = filepath.Join(t.TempDir(), "flow.db")
	modeFlag = "pomodoro"

	if err := initializeServices(); err != nil {
		t.Fatalf("initializeServices() error = %v", err)
	}
	setMethodology(domain.MethodologyMakeTime)
	if app.mode == nil || app.mode.Name() != domain.MethodologyMakeTime {
		t.Fatalf("app.mode not rebuilt for maketime: %v", app.mode)
	}

	session, err := app.pomodoro.StartBreak(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("StartBreak() error = %v", err)
	}
	want := time.Duration(app.config.MakeTime.BreakDuration)
	if session.Duration != want {
		t.Errorf("make time break duration = %v, want %v", session.Duration, want)
	}
}

func TestEffectiveSessionsBeforeLong(t *testing.T) {
	cfg := config.DefaultConfig()

	cfg.Pomodoro.SessionsBeforeLong = 3
	if got := effectiveSessionsBeforeLong(cfg, domain.MethodologyPomodoro); got != 4 {
		t.Errorf("pomodoro = %d, want 4 (technique rule)", got)
	}
	if got := effectiveSessionsBeforeLong(cfg, domain.MethodologyDeepWork); got != 3 {
		t.Errorf("deepwork = %d, want configured 3", got)
	}
	for _, n := range []int{0, -2} {
		cfg.Pomodoro.SessionsBeforeLong = n
		if got := effectiveSessionsBeforeLong(cfg, domain.MethodologyMakeTime); got <= 0 {
			t.Errorf("sessions_before_long=%d gave %d, must be > 0 (used as modulus)", n, got)
		}
	}
}

// TestInitializeServices_InvalidConfigFallback guards the fallback path: a
// config file that fails to parse must produce a stderr warning, and the
// default data dir must expand to $HOME instead of creating ./~/.flow.
func TestInitializeServices_InvalidConfigFallback(t *testing.T) {
	home := isolateApp(t)
	cwd := t.TempDir()
	t.Chdir(cwd)

	cfgPath := filepath.Join(home, ".flow", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte("not = = valid [[[ toml"), 0o600); err != nil {
		t.Fatal(err)
	}

	stderr := captureStderr(t, func() {
		if err := initializeServices(); err != nil {
			t.Fatalf("initializeServices() error = %v", err)
		}
	})

	if !strings.Contains(stderr, "config file could not be read") {
		t.Errorf("expected a stderr warning about the broken config, got %q", stderr)
	}
	if _, err := os.Stat(filepath.Join(cwd, "~")); !os.IsNotExist(err) {
		t.Errorf("a literal ~ directory was created in the cwd (stat err = %v)", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".flow", "flow.db")); err != nil {
		t.Errorf("expected the DB under $HOME/.flow: %v", err)
	}
}

// TestInitializeServices_MissingConfigIsSilent: a first run (no config file)
// must not print any warning.
func TestInitializeServices_MissingConfigIsSilent(t *testing.T) {
	isolateApp(t)
	dbPath = filepath.Join(t.TempDir(), "flow.db")

	stderr := captureStderr(t, func() {
		if err := initializeServices(); err != nil {
			t.Fatalf("initializeServices() error = %v", err)
		}
	})
	if strings.Contains(stderr, "config file could not be read") {
		t.Errorf("expected no config warning on first run, got %q", stderr)
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() { os.Stderr = old }()
	fn()
	_ = w.Close()
	return <-done
}

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/xvierd/flow-cli/internal/adapters/git"
	"github.com/xvierd/flow-cli/internal/adapters/notification"
	"github.com/xvierd/flow-cli/internal/adapters/storage"
	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/methodology"
	"github.com/xvierd/flow-cli/internal/ports"
	"github.com/xvierd/flow-cli/internal/services"
)

// appDeps groups all service-layer dependencies initialized at startup.
type appDeps struct {
	storage     ports.Storage
	tasks       *services.TaskService
	pomodoro    *services.PomodoroService
	state       *services.StateService
	git         ports.GitDetector
	notifier    *notification.Notifier
	config      *config.Config
	methodology domain.Methodology
	mode        methodology.Mode
}

// app holds all initialized service dependencies.
// Populated by initializeServices() and accessible to all commands.
var app appDeps

// configLoadFailed records that the config could not be loaded and app.config
// holds defaults instead. Saving then would overwrite the user's (broken but
// recoverable) config file with those defaults, so saveConfig refuses.
var configLoadFailed bool

// saveConfig persists cfg unless the config file failed to load at startup.
// Every config write in cmd must go through here instead of config.Save.
func saveConfig(cfg *config.Config) error {
	if configLoadFailed {
		return errors.New(i18n.T("the config file could not be read at startup; fix or remove it before changing settings"))
	}
	return config.Save(cfg)
}

// initializeServices sets up all the required services and adapters.
func initializeServices() error {
	// Load configuration
	var err error
	var loadErr error
	app.config, loadErr = config.Load()
	configLoadFailed = loadErr != nil
	if loadErr != nil {
		// If config loading fails, use defaults. A config file exists, so this
		// is not a first run: no welcome screen, and nothing to save.
		app.config = config.DefaultConfig()
		app.config.FirstRun = false
	}

	// Resolve display language: FLOW_LANG env > config file > "en".
	lang := os.Getenv("FLOW_LANG")
	if lang == "" {
		lang = app.config.Language
	}
	i18n.SetLanguage(lang)

	// A config file that exists but cannot be parsed must not be ignored
	// silently: the user would otherwise wonder why their settings vanished.
	if errors.Is(loadErr, config.ErrInvalidConfig) {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.T("Warning: using default settings, config file could not be read: %v", loadErr))
	}

	// Initialize notifier
	app.notifier = notification.New(&app.config.Notifications)

	// Determine database path
	if dbPath == "" {
		dbPath = config.GetDBPath(app.config)
	}

	// Ensure directory exists
	dbDir := getDir(dbPath)
	if err := os.MkdirAll(dbDir, 0750); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to create database directory"), err)
	}

	// Initialize storage
	app.storage, err = storage.New(dbPath)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to initialize storage"), err)
	}

	// Initialize git detector
	app.git = git.NewDetector()

	// Initialize services
	app.tasks = services.NewTaskService(app.storage)
	app.pomodoro = services.NewPomodoroService(app.storage, app.git)
	app.state = services.NewStateService(app.storage, app.tasks, app.pomodoro)

	// Resolve effective methodology: --mode flag > config > default.
	// Must happen before applyBreakConfig, which depends on it.
	modeStr := app.config.Methodology
	if modeFlag != "" {
		modeStr = modeFlag
	}
	if modeStr == "" {
		modeStr = "pomodoro"
	}
	m, err := domain.ValidateMethodology(modeStr)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("invalid mode"), err)
	}
	setMethodology(m)

	// Strict focus mode: --strict flag overrides the config setting.
	app.pomodoro.SetStrictMode(app.config.Focus.Strict || strictFlag)

	return nil
}

// setMethodology switches the active methodology, rebuilds the configured
// Mode, and re-applies the methodology-specific break config and session
// defaults to the pomodoro service. Every place that changes app.methodology
// must go through here.
func setMethodology(m domain.Methodology) {
	app.methodology = m
	app.mode = methodology.ForMethodology(m, app.config)
	applyBreakConfig()
	applySessionDefaults()
}

// applySessionDefaults gives the pomodoro service the current methodology
// (recorded on breaks and on MCP sessions started without one) and each
// methodology's default work duration: the configured work duration for
// Pomodoro, the first preset for Deep Work and Make Time (what a bare start
// in the TUI uses).
func applySessionDefaults() {
	if app.pomodoro == nil || app.config == nil {
		return
	}
	workDur, _, _, _ := app.config.ToPomodoroDomainConfig()
	durations := map[domain.Methodology]time.Duration{
		domain.MethodologyPomodoro: workDur,
	}
	for _, m := range []domain.Methodology{domain.MethodologyDeepWork, domain.MethodologyMakeTime} {
		if presets := methodology.ForMethodology(m, app.config).Presets(); len(presets) > 0 {
			durations[m] = presets[0].Duration
		}
	}
	app.pomodoro.SetMethodologyDefaults(app.methodology, durations)
}

// effectiveSessionsBeforeLong returns the number of work sessions before a
// long break for methodology m. Pomodoro always uses 4 (the technique's rule);
// other methodologies use the configured value, falling back to the default
// when the configured value is not positive (it is used as a modulus).
func effectiveSessionsBeforeLong(cfg *config.Config, m domain.Methodology) int {
	const defaultSessionsBeforeLong = 4
	if m == domain.MethodologyPomodoro {
		return defaultSessionsBeforeLong
	}
	if n := cfg.Pomodoro.SessionsBeforeLong; n > 0 {
		return n
	}
	return defaultSessionsBeforeLong
}

// applyBreakConfig configures the pomodoro service with the break durations
// and long-break cadence of the current methodology.
func applyBreakConfig() {
	if app.pomodoro == nil || app.config == nil {
		return
	}
	workDur, _, _, _ := app.config.ToPomodoroDomainConfig()
	shortBreakDur, longBreakDur := app.config.GetBreakDurations(app.methodology)
	app.pomodoro.SetConfig(domain.PomodoroConfig{
		WorkDuration:       workDur,
		ShortBreakDuration: shortBreakDur,
		LongBreakDuration:  longBreakDur,
		SessionsBeforeLong: effectiveSessionsBeforeLong(app.config, app.methodology),
	})
}

// cleanupServices closes all resources.
func cleanupServices() error {
	if app.storage != nil {
		return app.storage.Close()
	}
	return nil
}

// signalOnce guards the single process-wide signal registration shared by
// every setupSignalHandler call; signalCtx is the context it cancels.
var (
	signalOnce sync.Once
	signalCtx  context.Context
)

// setupSignalHandler returns a context that is cancelled on the first
// SIGINT/SIGTERM. The handler is registered once per process: chained TUI
// launches share it instead of stacking a signal.Notify and a goroutine per
// launch. After the first signal the handler unregisters, so a second Ctrl+C
// gets the default behavior and terminates the process.
func setupSignalHandler() context.Context {
	signalOnce.Do(func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		signalCtx = cancelOnSignal(sigChan, func() { signal.Stop(sigChan) })
	})
	return signalCtx
}

// cancelOnSignal returns a context cancelled when sigChan delivers a signal;
// stop is called first to release the signal registration.
func cancelOnSignal(sigChan <-chan os.Signal, stop func()) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-sigChan
		stop()
		cancel()
	}()
	return ctx
}

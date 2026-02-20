package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/xvierd/flow-cli/internal/adapters/git"
	"github.com/xvierd/flow-cli/internal/adapters/notification"
	"github.com/xvierd/flow-cli/internal/adapters/storage"
	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
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

// initializeServices sets up all the required services and adapters.
func initializeServices() error {
	// Load configuration
	var err error
	app.config, err = config.Load()
	if err != nil {
		// If config loading fails, use defaults
		app.config = config.DefaultConfig()
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
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	// Initialize storage
	app.storage, err = storage.New(dbPath)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Initialize git detector
	app.git = git.NewDetector()

	// Initialize services
	app.tasks = services.NewTaskService(app.storage)
	app.pomodoro = services.NewPomodoroService(app.storage, app.git)
	app.state = services.NewStateService(app.storage)

	// Configure pomodoro service from config
	workDur, _, _, sessionsBeforeLong := app.config.ToPomodoroDomainConfig()
	shortBreakDur, longBreakDur := app.config.GetBreakDurations(app.methodology)
	// Pomodoro Technique rule: long break always after every 4 work sessions.
	if app.methodology == domain.MethodologyPomodoro {
		sessionsBeforeLong = 4
	}
	app.pomodoro.SetConfig(domain.PomodoroConfig{
		WorkDuration:       workDur,
		ShortBreakDuration: shortBreakDur,
		LongBreakDuration:  longBreakDur,
		SessionsBeforeLong: sessionsBeforeLong,
	})

	// Wire up services for state service
	app.state.SetTaskService(app.tasks)
	app.state.SetPomodoroService(app.pomodoro)

	// Resolve effective methodology: --mode flag > config > default
	modeStr := app.config.Methodology
	if modeFlag != "" {
		modeStr = modeFlag
	}
	if modeStr == "" {
		modeStr = "pomodoro"
	}
	m, err := domain.ValidateMethodology(modeStr)
	if err != nil {
		return fmt.Errorf("invalid mode: %w", err)
	}
	app.methodology = m
	app.mode = methodology.ForMethodology(app.methodology, app.config)

	return nil
}

// cleanupServices closes all resources.
func cleanupServices() error {
	if app.storage != nil {
		return app.storage.Close()
	}
	return nil
}

// setupSignalHandler sets up a context that cancels on interrupt signals.
func setupSignalHandler() context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		cancel()
	}()

	return ctx
}

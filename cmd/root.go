// Package cmd provides the CLI commands for the Flow application.
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/adapters/git"
	"github.com/xvierd/flow-cli/internal/adapters/notification"
	"github.com/xvierd/flow-cli/internal/adapters/storage"
	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
	"github.com/xvierd/flow-cli/internal/services"
)

var (
	// Global flags
	dbPath     string
	jsonOutput bool

	// Global dependencies
	storageAdapter ports.Storage
	taskService    *services.TaskService
	pomodoroSvc    *services.PomodoroService
	stateService   *services.StateService
	gitDetector    ports.GitDetector
	notifier       *notification.Notifier
	appConfig      *config.Config
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "flow",
	Short: "Flow - A Pomodoro CLI timer with task tracking",
	Long: `Flow is a command-line Pomodoro timer that helps you stay focused
and track your work sessions with optional git integration.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return initializeServices()
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		return cleanupServices()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "Path to the database file (default: ~/.flow/flow.db)")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output results in JSON format")

	// Add subcommands
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(breakCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(pauseCmd)
	rootCmd.AddCommand(resumeCmd)
	rootCmd.AddCommand(completeCmd)
}

// initializeServices sets up all the required services and adapters.
func initializeServices() error {
	// Load configuration
	var err error
	appConfig, err = config.Load()
	if err != nil {
		// If config loading fails, use defaults
		appConfig = config.DefaultConfig()
	}

	// Initialize notifier
	notifier = notification.New(&appConfig.Notifications)

	// Determine database path
	if dbPath == "" {
		dbPath = config.GetDBPath(appConfig)
	}

	// Ensure directory exists
	dbDir := getDir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	// Initialize storage
	storageAdapter, err = storage.New(dbPath)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Initialize git detector
	gitDetector = git.NewDetector()

	// Initialize services
	taskService = services.NewTaskService(storageAdapter)
	pomodoroSvc = services.NewPomodoroService(storageAdapter, gitDetector)
	stateService = services.NewStateService(storageAdapter)

	// Configure pomodoro service from config
	workDur, shortBreakDur, longBreakDur, sessionsBeforeLong := appConfig.ToPomodoroDomainConfig()
	pomodoroSvc.SetConfig(domain.PomodoroConfig{
		WorkDuration:       workDur,
		ShortBreakDuration: shortBreakDur,
		LongBreakDuration:  longBreakDur,
		SessionsBeforeLong: sessionsBeforeLong,
	})

	// Wire up services for state service
	stateService.SetTaskService(taskService)
	stateService.SetPomodoroService(pomodoroSvc)

	return nil
}

// cleanupServices closes all resources.
func cleanupServices() error {
	if storageAdapter != nil {
		return storageAdapter.Close()
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

// getDir returns the directory of a file path.
func getDir(path string) string {
	lastSep := 0
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			lastSep = i
			break
		}
	}
	if lastSep == 0 {
		return "."
	}
	return path[:lastSep]
}

// Package cmd provides the CLI commands for the Flow application.
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/dvidx/flow-cli/internal/adapters/git"
	"github.com/dvidx/flow-cli/internal/adapters/storage"
	"github.com/dvidx/flow-cli/internal/ports"
	"github.com/dvidx/flow-cli/internal/services"
)

var (
	// Global flags
	dbPath string

	// Global dependencies
	storageAdapter ports.Storage
	taskService    *services.TaskService
	pomodoroSvc    *services.PomodoroService
	stateService   *services.StateService
	gitDetector    ports.GitDetector
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
	// Determine database path
	if dbPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		flowDir := fmt.Sprintf("%s/.flow", homeDir)
		if err := os.MkdirAll(flowDir, 0755); err != nil {
			return fmt.Errorf("failed to create flow directory: %w", err)
		}
		dbPath = fmt.Sprintf("%s/flow.db", flowDir)
	}

	// Initialize storage
	var err error
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

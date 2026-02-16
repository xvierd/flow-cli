// Package cmd provides the CLI commands for the Flow application.
package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/adapters/git"
	"github.com/xvierd/flow-cli/internal/adapters/notification"
	"github.com/xvierd/flow-cli/internal/adapters/storage"
	"github.com/xvierd/flow-cli/internal/adapters/tui"
	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
	"github.com/xvierd/flow-cli/internal/services"
)

var (
	// Version info (set at build time via ldflags)
	Version   = "dev"
	BuildDate = "unknown"
	GitCommit = "unknown"

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
and track your work sessions with optional git integration.

Run "flow" with no arguments to start a quick session interactively.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return initializeServices()
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		return cleanupServices()
	},
	RunE: runWizard,
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

	// Set version - cobra handles --version automatically
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("Flow CLI\nVersion: {{.Version}}\n")

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

// runWizard implements the interactive wizard flow for bare "flow" command.
func runWizard(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	reader := bufio.NewReader(os.Stdin)
	workingDir, _ := os.Getwd()

	// Check for active session
	state, err := stateService.GetCurrentState(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current state: %w", err)
	}

	if state.ActiveSession != nil {
		active := state.ActiveSession
		remaining := active.RemainingTime()
		sessionType := domain.GetSessionTypeLabel(active.Type)
		sessionInfo := fmt.Sprintf("%s (%s remaining)", sessionType, formatWizardDuration(remaining))

		if state.ActiveTask != nil {
			sessionInfo = fmt.Sprintf("%s for \"%s\" (%s remaining)", sessionType, state.ActiveTask.Title, formatWizardDuration(remaining))
		}

		fmt.Printf("\n  Active session: %s\n", sessionInfo)
		fmt.Print("  Resume it? [Y/n] ")

		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))

		if answer == "" || answer == "y" || answer == "yes" {
			// Resume: just open the TUI for the existing session
			return launchTUI(ctx, state, workingDir)
		}

		// Stop old session and start fresh
		_, err := pomodoroSvc.StopSession(ctx)
		if err != nil {
			return fmt.Errorf("failed to stop current session: %w", err)
		}
	}

	// --- Wizard prompts ---
	fmt.Println()

	// 1. Ask for task name
	fmt.Print("  What are you working on? (Enter to skip): ")
	taskName, _ := reader.ReadString('\n')
	taskName = strings.TrimSpace(taskName)

	var taskID *string
	if taskName != "" {
		// Create the task inline
		task, err := taskService.AddTask(ctx, services.AddTaskRequest{
			Title: taskName,
		})
		if err != nil {
			return fmt.Errorf("failed to create task: %w", err)
		}
		taskID = &task.ID
	}

	// 2. Ask for duration with default
	defaultDuration := time.Duration(appConfig.Pomodoro.WorkDuration)
	defaultLabel := formatMinutes(defaultDuration)
	fmt.Printf("  Duration? [%s]: ", defaultLabel)
	durationInput, _ := reader.ReadString('\n')
	durationInput = strings.TrimSpace(durationInput)

	var customDuration time.Duration
	if durationInput != "" {
		parsed, err := time.ParseDuration(durationInput)
		if err != nil {
			return fmt.Errorf("invalid duration %q (use format like 25m, 1h, 45m): %w", durationInput, err)
		}
		customDuration = parsed
	}

	// Start the session
	req := services.StartPomodoroRequest{
		TaskID:     taskID,
		WorkingDir: workingDir,
		Duration:   customDuration,
	}

	session, err := pomodoroSvc.StartPomodoro(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to start pomodoro: %w", err)
	}

	fmt.Printf("\n  Starting %s session...\n\n", formatMinutes(session.Duration))

	// Refresh state and launch TUI
	state, err = stateService.GetCurrentState(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current state: %w", err)
	}

	return launchTUI(ctx, state, workingDir)
}

// launchTUI starts the Bubbletea timer interface.
func launchTUI(ctx context.Context, state *domain.CurrentState, workingDir string) error {
	ctx = setupSignalHandler()
	timer := tui.NewTimer()

	timer.SetFetchState(func() *domain.CurrentState {
		newState, err := stateService.GetCurrentState(ctx)
		if err != nil {
			// Log error but return nil to let TUI handle gracefully
			fmt.Fprintf(os.Stderr, "Warning: failed to fetch state: %v\n", err)
			return nil
		}
		return newState
	})

	timer.SetCommandCallback(func(cmd ports.TimerCommand) error {
		switch cmd {
		case ports.CmdStart:
			_, err := pomodoroSvc.StartPomodoro(ctx, services.StartPomodoroRequest{
				WorkingDir: workingDir,
			})
			return err
		case ports.CmdPause:
			_, err := pomodoroSvc.PauseSession(ctx)
			return err
		case ports.CmdResume:
			_, err := pomodoroSvc.ResumeSession(ctx)
			return err
		case ports.CmdStop:
			_, err := pomodoroSvc.StopSession(ctx)
			return err
		case ports.CmdCancel:
			return pomodoroSvc.CancelSession(ctx)
		case ports.CmdBreak:
			_, err := pomodoroSvc.StartBreak(ctx, workingDir)
			return err
		default:
			return fmt.Errorf("unknown command: %v", cmd)
		}
	})

	// Compute break info from config
	_, shortBreakDur, longBreakDur, sessionsBeforeLong := appConfig.ToPomodoroDomainConfig()
	workSessions := state.TodayStats.WorkSessions + 1 // +1 for the session about to complete
	nextSessionCount := workSessions
	sessionsUntilLong := sessionsBeforeLong - (nextSessionCount % sessionsBeforeLong)
	if sessionsUntilLong == sessionsBeforeLong {
		sessionsUntilLong = 0
	}

	nextBreakType := domain.SessionTypeShortBreak
	nextBreakDuration := shortBreakDur
	if sessionsUntilLong == 0 {
		nextBreakType = domain.SessionTypeLongBreak
		nextBreakDuration = longBreakDur
	}

	timer.SetCompletionInfo(&domain.CompletionInfo{
		NextBreakType:      nextBreakType,
		NextBreakDuration:  nextBreakDuration,
		SessionsUntilLong:  sessionsUntilLong,
		SessionsBeforeLong: sessionsBeforeLong,
	})

	// Desktop notifications on session completion
	timer.SetOnSessionComplete(func(sessionType domain.SessionType) {
		if notifier == nil || !notifier.IsEnabled() {
			return
		}
		
		var err error
		switch sessionType {
		case domain.SessionTypeWork:
			err = notifier.NotifyPomodoroComplete(formatMinutes(shortBreakDur))
		case domain.SessionTypeShortBreak:
			err = notifier.NotifyBreakComplete("Short")
		case domain.SessionTypeLongBreak:
			err = notifier.NotifyBreakComplete("Long")
		}
		
		if err != nil {
			// Log notification errors but don't fail
			fmt.Fprintf(os.Stderr, "Warning: notification failed: %v\n", err)
		}
	})

	if err := timer.Run(ctx, state); err != nil {
		return fmt.Errorf("timer error: %w", err)
	}

	return nil
}

// formatMinutes formats a duration as a human-friendly string like "25m" or "1h30m".
func formatMinutes(d time.Duration) string {
	if d.Minutes() == float64(int(d.Minutes())) && int(d.Minutes())%60 == 0 && d >= time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	if d >= time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

// formatWizardDuration formats a duration as MM:SS.
func formatWizardDuration(d time.Duration) string {
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", m, s)
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

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
	"github.com/xvierd/flow-cli/internal/methodology"
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
	inlineMode bool
	modeFlag   string

	// Global dependencies
	storageAdapter      ports.Storage
	taskService         *services.TaskService
	pomodoroSvc         *services.PomodoroService
	stateService        *services.StateService
	gitDetector         ports.GitDetector
	notifier            *notification.Notifier
	appConfig           *config.Config
	effectiveMethodology domain.Methodology
	activeMode          methodology.Mode
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
	rootCmd.PersistentFlags().BoolVarP(&inlineMode, "inline", "i", false, "Compact inline timer (no fullscreen)")
	rootCmd.PersistentFlags().StringVar(&modeFlag, "mode", "", "Productivity methodology: pomodoro, deepwork, maketime")

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

	// Resolve effective methodology: --mode flag > config > default
	modeStr := appConfig.Methodology
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
	effectiveMethodology = m
	activeMode = methodology.ForMethodology(effectiveMethodology)

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
	workingDir, _ := os.Getwd()

	// Check for active session
	state, err := stateService.GetCurrentState(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current state: %w", err)
	}

	// Inline mode: entire flow runs inside a single bubbletea program
	if inlineMode {
		return launchInlineTUI(cmd, ctx, state, workingDir)
	}

	// Fullscreen mode: wizard prompts then TUI
	reader := bufio.NewReader(os.Stdin)

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
			return launchTUI(ctx, state, workingDir)
		}

		_, err := pomodoroSvc.StopSession(ctx)
		if err != nil {
			return fmt.Errorf("failed to stop current session: %w", err)
		}
	}

	// --- Wizard prompts ---
	fmt.Println()

	// Main menu (skip if --mode was explicitly passed — user wants to start a session)
	if modeFlag == "" {
		menuItems := []tui.PickerItem{
			{Label: "Start session", Desc: "Begin a new focus session"},
			{Label: "View stats", Desc: "Show your productivity dashboard"},
			{Label: "Reflect", Desc: "Weekly reflection on your work"},
		}
		menuResult := tui.RunPicker("Flow:", menuItems, "", &appConfig.Theme)
		if menuResult.Aborted {
			return nil
		}
		switch menuResult.Index {
		case 1: // View stats
			return statsCmd.RunE(cmd, args)
		case 2: // Reflect
			return reflectCmd.RunE(cmd, args)
		}
		// Index 0: Start session — continue to mode picker
		fmt.Println()
	}

	// Mode picker (skip if --mode was explicitly passed)
	if modeFlag == "" {
		modeItems := []tui.PickerItem{
			{Label: "Pomodoro", Desc: "Classic 25/5 timer"},
			{Label: "Deep Work", Desc: "Longer sessions, distraction tracking"},
			{Label: "Make Time", Desc: "Daily Highlight, focus scoring"},
		}
		modeResult := tui.RunPicker("Mode:", modeItems, "", &appConfig.Theme)
		if modeResult.Aborted {
			return nil
		}
		methodologies := []domain.Methodology{domain.MethodologyPomodoro, domain.MethodologyDeepWork, domain.MethodologyMakeTime}
		effectiveMethodology = methodologies[modeResult.Index]
		activeMode = methodology.ForMethodology(effectiveMethodology)
		fmt.Println()
	}

	mode := activeMode

	// Session chaining loop: runs once normally, then repeats if user selects "new session"
	for {
		// Make Time: check for existing highlight or carry-over from yesterday
		if mode.HasHighlight() {
			highlight, _ := storageAdapter.Tasks().FindTodayHighlight(ctx, time.Now())
			if highlight != nil {
				fmt.Printf("  Today's Highlight: \"%s\"\n\n", highlight.Title)
			} else {
				// Check yesterday's unfinished highlight
				yesterdayHighlight, _ := storageAdapter.Tasks().FindYesterdayHighlight(ctx, time.Now())
				if yesterdayHighlight != nil {
					fmt.Printf("  Yesterday's Highlight \"%s\" wasn't completed.\n", yesterdayHighlight.Title)
					fmt.Print("  Carry forward as today's Highlight? [Y/n] ")
					answer, _ := reader.ReadString('\n')
					answer = strings.TrimSpace(strings.ToLower(answer))
					if answer == "" || answer == "y" || answer == "yes" {
						yesterdayHighlight.SetAsHighlight()
						_ = storageAdapter.Tasks().Update(ctx, yesterdayHighlight)
						fmt.Printf("  Today's Highlight: \"%s\"\n\n", yesterdayHighlight.Title)
					}
				}
			}
		}

		// 1. Show recent tasks for quick re-selection, then ask for task name
		var taskName string
		var sessionTags []string
		var taskID *string

		recentTasks, _ := storageAdapter.Tasks().FindRecentTasks(ctx, 3)
		if len(recentTasks) > 0 {
			fmt.Println("  Recent tasks:")
			for i, t := range recentTasks {
				fmt.Printf("    [%d] %s\n", i+1, t.Title)
			}
			fmt.Printf("  Pick [1-%d] or type a new name: ", len(recentTasks))

			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)

			picked := false
			if len(input) == 1 && input[0] >= '1' && input[0] <= '9' {
				idx := int(input[0]-'0') - 1
				if idx < len(recentTasks) {
					taskName = recentTasks[idx].Title
					taskID = &recentTasks[idx].ID
					picked = true
				}
			}
			if !picked {
				taskName = input
			}
		} else {
			fmt.Printf("  %s ", mode.TaskPrompt())
			taskName, _ = reader.ReadString('\n')
			taskName = strings.TrimSpace(taskName)
		}

		// Parse #tags from task name input
		if taskName != "" {
			taskName, sessionTags = domain.ParseTagsFromInput(taskName)
		}

		if taskName != "" && taskID == nil {
			task, err := taskService.AddTask(ctx, services.AddTaskRequest{
				Title: taskName,
			})
			if err != nil {
				return fmt.Errorf("failed to create task: %w", err)
			}
			taskID = &task.ID

			// Make Time: set as today's highlight
			if mode.HasHighlight() {
				task.SetAsHighlight()
				_ = storageAdapter.Tasks().Update(ctx, task)
			}
		}

		// Deep Work: ask for intended outcome
		var intendedOutcome string
		if mode.OutcomePrompt() != "" {
			fmt.Printf("  %s ", mode.OutcomePrompt())
			intendedOutcome, _ = reader.ReadString('\n')
			intendedOutcome = strings.TrimSpace(intendedOutcome)
		}

		// 2. Pick session type with arrow-key picker (mode-specific presets)
		presets := mode.Presets()
		shortBreak := time.Duration(appConfig.Pomodoro.ShortBreak)
		longBreak := time.Duration(appConfig.Pomodoro.LongBreak)

		var items []tui.PickerItem
		for _, p := range presets {
			items = append(items, tui.PickerItem{
				Label: p.Name,
				Desc:  formatMinutes(p.Duration),
			})
		}

		footer := fmt.Sprintf("Breaks: %s short / %s long (every %d) · \"flow config\" to customize",
			formatMinutes(shortBreak), formatMinutes(longBreak), appConfig.Pomodoro.SessionsBeforeLong)

		result := tui.RunPicker("Duration:", items, footer, &appConfig.Theme)
		if result.Aborted {
			return nil
		}

		customDuration := presets[result.Index].Duration

		// Start the session
		req := services.StartPomodoroRequest{
			TaskID:          taskID,
			WorkingDir:      workingDir,
			Duration:        customDuration,
			Methodology:     effectiveMethodology,
			IntendedOutcome: intendedOutcome,
			Tags:            sessionTags,
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

		if err := launchTUI(ctx, state, workingDir); err != nil {
			return err
		}

		// Check if user wants to chain another session (fullscreen only)
		if lastFullscreenTimer == nil || !lastFullscreenTimer.WantsNewSession {
			break
		}
		// Reset for next iteration
		lastFullscreenTimer.WantsNewSession = false
		fmt.Println("\n  Starting new session...")
		fmt.Println()
	}

	return nil
}

// lastInlineTimer holds a reference to the inline timer for post-exit action handling.
var lastInlineTimer *tui.Timer

// lastFullscreenTimer holds a reference to the fullscreen timer for session chaining.
var lastFullscreenTimer *tui.Timer

// launchInlineTUI launches the inline TUI and handles post-exit actions (stats/reflect).
func launchInlineTUI(cmd *cobra.Command, ctx context.Context, state *domain.CurrentState, workingDir string) error {
	if err := launchTUI(ctx, state, workingDir); err != nil {
		return err
	}

	// Handle post-TUI actions from the main menu
	if lastInlineTimer != nil {
		switch lastInlineTimer.PostAction {
		case tui.MainMenuViewStats:
			return statsCmd.RunE(cmd, nil)
		case tui.MainMenuReflect:
			return reflectCmd.RunE(cmd, nil)
		}
	}
	return nil
}

// launchTUI starts the Bubbletea timer interface.
func launchTUI(ctx context.Context, state *domain.CurrentState, workingDir string) error {
	ctx = setupSignalHandler()
	var timer ports.Timer
	if inlineMode {
		inlineTimer := tui.NewInlineTimer(&appConfig.Theme)
		lastInlineTimer = inlineTimer

		// Configure the setup phase with mode-specific presets
		presets := activeMode.Presets()
		shortBreak := time.Duration(appConfig.Pomodoro.ShortBreak)
		longBreak := time.Duration(appConfig.Pomodoro.LongBreak)
		breakInfo := fmt.Sprintf("Breaks: %s short / %s long (every %d) · \"flow config\" to customize",
			formatMinutes(shortBreak), formatMinutes(longBreak), appConfig.Pomodoro.SessionsBeforeLong)

		inlineTimer.SetMode(activeMode)
		inlineTimer.SetModeLocked(modeFlag != "")
		inlineTimer.SetOnModeSelected(func(m domain.Methodology) {
			effectiveMethodology = m
			activeMode = methodology.ForMethodology(m)
			presets = activeMode.Presets()
			inlineTimer.SetMode(activeMode)
		})

		inlineTimer.SetFetchRecentTasks(func(limit int) []*domain.Task {
			tasks, err := storageAdapter.Tasks().FindRecentTasks(ctx, limit)
			if err != nil {
				return nil
			}
			return tasks
		})

		inlineTimer.SetFetchYesterdayHighlight(func() *domain.Task {
			task, _ := storageAdapter.Tasks().FindYesterdayHighlight(ctx, time.Now())
			return task
		})

		inlineTimer.SetInlineSetup(presets, breakInfo, func(presetIndex int, taskName string) error {
			currentMode := activeMode
			currentPresets := currentMode.Presets()

			// Parse #tags from task name input
			var sessionTags []string
			if taskName != "" {
				taskName, sessionTags = domain.ParseTagsFromInput(taskName)
			}

			var taskID *string
			if taskName != "" {
				task, err := taskService.AddTask(ctx, services.AddTaskRequest{
					Title: taskName,
				})
				if err != nil {
					return err
				}
				taskID = &task.ID

				// Make Time: set as today's highlight
				if currentMode.HasHighlight() {
					task.SetAsHighlight()
					_ = storageAdapter.Tasks().Update(ctx, task)
				}
			}

			_, err := pomodoroSvc.StartPomodoro(ctx, services.StartPomodoroRequest{
				TaskID:          taskID,
				WorkingDir:      workingDir,
				Duration:        currentPresets[presetIndex].Duration,
				Methodology:     effectiveMethodology,
				Tags:            sessionTags,
			})
			return err
		})

		timer = inlineTimer
	} else {
		fsTimer := tui.NewFullscreenTimer(&appConfig.Theme)
		fsTimer.SetMode(activeMode)
		lastFullscreenTimer = fsTimer
		timer = fsTimer
	}

	timer.SetAutoBreak(appConfig.Pomodoro.AutoBreak)

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

	// Wire methodology callbacks
	timer.SetDistractionCallback(func(text string) error {
		activeState, err := pomodoroSvc.GetCurrentState(ctx)
		if err != nil || activeState.ActiveSession == nil {
			return nil
		}
		return pomodoroSvc.LogDistraction(ctx, activeState.ActiveSession.ID, text)
	})

	timer.SetAccomplishmentCallback(func(text string) error {
		// Find the most recently completed session
		recent, err := pomodoroSvc.GetRecentSessions(ctx, 1)
		if err != nil || len(recent) == 0 {
			return nil
		}
		return pomodoroSvc.SetAccomplishment(ctx, recent[0].ID, text)
	})

	timer.SetFocusScoreCallback(func(score int) error {
		recent, err := pomodoroSvc.GetRecentSessions(ctx, 1)
		if err != nil || len(recent) == 0 {
			return nil
		}
		return pomodoroSvc.SetFocusScore(ctx, recent[0].ID, score)
	})

	timer.SetEnergizeCallback(func(activity string) error {
		recent, err := pomodoroSvc.GetRecentSessions(ctx, 1)
		if err != nil || len(recent) == 0 {
			return nil
		}
		return pomodoroSvc.SetEnergizeActivity(ctx, recent[0].ID, activity)
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

	// Fetch Deep Work streak if in deepwork mode
	var deepWorkStreak int
	if effectiveMethodology == domain.MethodologyDeepWork {
		deepWorkStreak, _ = pomodoroSvc.GetDeepWorkStreak(ctx)
	}

	timer.SetCompletionInfo(&domain.CompletionInfo{
		NextBreakType:      nextBreakType,
		NextBreakDuration:  nextBreakDuration,
		SessionsUntilLong:  sessionsUntilLong,
		SessionsBeforeLong: sessionsBeforeLong,
		DeepWorkStreak:     deepWorkStreak,
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

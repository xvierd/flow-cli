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

	// app holds all initialized service dependencies.
	// Populated by initializeServices() and accessible to all commands.
	app appDeps
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
	rootCmd.AddCommand(resetCmd)
}

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
	if err := os.MkdirAll(dbDir, 0755); err != nil {
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

// runWizard implements the interactive wizard flow for bare "flow" command.
func runWizard(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	workingDir, _ := os.Getwd()

	// Show welcome screen on first run
	if app.config.FirstRun {
		printWelcome()
		app.config.FirstRun = false
		_ = config.Save(app.config)
	}

	// Check for active session
	state, err := app.state.GetCurrentState(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current state: %w", err)
	}

	// Inline mode: entire flow runs inside a single bubbletea program
	if inlineMode {
		return launchInlineTUI(cmd, ctx, state, workingDir)
	}

	// Fullscreen mode: wizard prompts then TUI
	if state.ActiveSession != nil {
		active := state.ActiveSession
		remaining := active.RemainingTime()
		sessionType := domain.GetSessionTypeLabel(active.Type)
		sessionInfo := fmt.Sprintf("%s (%s remaining)", sessionType, formatWizardDuration(remaining))

		if state.ActiveTask != nil {
			sessionInfo = fmt.Sprintf("%s for \"%s\" (%s remaining)", sessionType, state.ActiveTask.Title, formatWizardDuration(remaining))
		}

		resumeItems := []tui.PickerItem{
			{Label: "Resume", Desc: sessionInfo},
			{Label: "Stop", Desc: "End current session and start fresh"},
		}
		resumeResult := tui.RunPicker("Active session:", resumeItems, "", &app.config.Theme)
		if resumeResult.Aborted {
			return nil
		}

		if resumeResult.Index == 0 {
			return launchTUI(ctx, state, workingDir)
		}

		_, err := app.pomodoro.StopSession(ctx)
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
		menuResult := tui.RunPicker("Flow:", menuItems, "", &app.config.Theme)
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
		modeResult := tui.RunPicker("Mode:", modeItems, "", &app.config.Theme)
		if modeResult.Aborted {
			return nil
		}
		methodologies := []domain.Methodology{domain.MethodologyPomodoro, domain.MethodologyDeepWork, domain.MethodologyMakeTime}
		app.methodology = methodologies[modeResult.Index]
		app.mode = methodology.ForMethodology(app.methodology, app.config)
		fmt.Println()
	}

	mode := app.mode

	// Session chaining loop: runs once normally, then repeats if user selects "new session"
	for {
		// Make Time: check for existing highlight or carry-over from yesterday
		if mode.HasHighlight() {
			highlight, _ := app.storage.Tasks().FindTodayHighlight(ctx, time.Now())
			if highlight != nil {
				fmt.Printf("  Today's Highlight: \"%s\"\n\n", highlight.Title)
			} else {
				yesterdayHighlight, _ := app.storage.Tasks().FindYesterdayHighlight(ctx, time.Now())
				if yesterdayHighlight != nil {
					carryItems := []tui.PickerItem{
						{Label: "Yes", Desc: fmt.Sprintf("Continue with \"%s\"", yesterdayHighlight.Title)},
						{Label: "No", Desc: "Pick a new Highlight"},
					}
					carryResult := tui.RunPicker("Carry forward yesterday's Highlight?", carryItems, "", &app.config.Theme)
					if !carryResult.Aborted && carryResult.Index == 0 {
						yesterdayHighlight.SetAsHighlight()
						_ = app.storage.Tasks().Update(ctx, yesterdayHighlight)
					}
					fmt.Println()
				}
			}
		}

		// 1. Task selection via styled picker
		var taskName string
		var sessionTags []string
		var taskID *string

		recentTasks, _ := app.storage.Tasks().FindRecentTasks(ctx, 3)
		if len(recentTasks) > 0 {
			var taskItems []tui.PickerItem
			for _, t := range recentTasks {
				taskItems = append(taskItems, tui.PickerItem{
					Label: t.Title,
					Desc:  "",
				})
			}
			taskItems = append(taskItems, tui.PickerItem{
				Label: "New task...",
				Desc:  "Type a name",
			})

			taskResult := tui.RunPicker(mode.TaskPrompt(), taskItems, "", &app.config.Theme)
			if taskResult.Aborted {
				return nil
			}

			if taskResult.Index < len(recentTasks) {
				taskName = recentTasks[taskResult.Index].Title
				taskID = &recentTasks[taskResult.Index].ID
			} else {
				// "New task" selected — prompt for name
				textResult := tui.RunTextPrompt(mode.TaskPrompt(), "Enter to skip", &app.config.Theme)
				if textResult.Aborted {
					return nil
				}
				taskName = textResult.Value
			}
		} else {
			textResult := tui.RunTextPrompt(mode.TaskPrompt(), "Enter to skip", &app.config.Theme)
			if textResult.Aborted {
				return nil
			}
			taskName = textResult.Value
		}

		// Parse #tags from task name input
		if taskName != "" {
			taskName, sessionTags = domain.ParseTagsFromInput(taskName)
		}

		if taskName != "" && taskID == nil {
			task, err := app.tasks.AddTask(ctx, services.AddTaskRequest{
				Title: taskName,
			})
			if err != nil {
				return fmt.Errorf("failed to create task: %w", err)
			}
			taskID = &task.ID

			// Make Time: set as today's highlight
			if mode.HasHighlight() {
				task.SetAsHighlight()
				_ = app.storage.Tasks().Update(ctx, task)
			}
		}

		// Deep Work: ask for intended outcome
		var intendedOutcome string
		if mode.OutcomePrompt() != "" {
			outcomeResult := tui.RunTextPrompt(mode.OutcomePrompt(), "Enter to skip", &app.config.Theme)
			if outcomeResult.Aborted {
				return nil
			}
			intendedOutcome = outcomeResult.Value
		}

		// 2. Pick duration with arrow-key picker (mode-specific presets)
		presets := mode.Presets()
		shortBreak, longBreak := app.config.GetBreakDurations(app.methodology)

		var items []tui.PickerItem
		for _, p := range presets {
			items = append(items, tui.PickerItem{
				Label: p.Name,
				Desc:  formatMinutes(p.Duration),
			})
		}

		var footer string
		if app.methodology == domain.MethodologyPomodoro {
			footer = fmt.Sprintf("Breaks: %s short / %s long (every %d) · \"flow config\" to customize",
				formatMinutes(shortBreak), formatMinutes(longBreak), app.config.Pomodoro.SessionsBeforeLong)
		} else {
			footer = fmt.Sprintf("Break: %s · \"flow config\" to customize", formatMinutes(shortBreak))
		}

		result := tui.RunPicker("Duration:", items, footer, &app.config.Theme)
		if result.Aborted {
			return nil
		}

		customDuration := presets[result.Index].Duration

		// Start the session
		req := services.StartPomodoroRequest{
			TaskID:          taskID,
			WorkingDir:      workingDir,
			Duration:        customDuration,
			Methodology:     app.methodology,
			IntendedOutcome: intendedOutcome,
			Tags:            sessionTags,
		}

		_, err = app.pomodoro.StartPomodoro(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to start pomodoro: %w", err)
		}

		// Refresh state and launch TUI
		state, err = app.state.GetCurrentState(ctx)
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
	}

	// After quitting, in Make Time mode, prompt for tomorrow's highlight
	if mode.HasHighlight() {
		fmt.Print("\nWhat's your Highlight for tomorrow? (Enter to skip): ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			text := strings.TrimSpace(scanner.Text())
			if text != "" {
				task, err := domain.NewTask(text)
				if err == nil {
					_ = app.storage.Tasks().Save(ctx, task)
				}
			}
		}
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
		inlineTimer := tui.NewInlineTimer(&app.config.Theme)
		lastInlineTimer = inlineTimer

		// Configure the setup phase with mode-specific presets
		presets := app.mode.Presets()
		shortBreak, longBreak := app.config.GetBreakDurations(app.methodology)
		var breakInfo string
		if app.methodology == domain.MethodologyPomodoro {
			breakInfo = fmt.Sprintf("Breaks: %s short / %s long (every %d) · \"flow config\" to customize",
				formatMinutes(shortBreak), formatMinutes(longBreak), app.config.Pomodoro.SessionsBeforeLong)
		} else {
			breakInfo = fmt.Sprintf("Break: %s · \"flow config\" to customize", formatMinutes(shortBreak))
		}

		inlineTimer.SetMode(app.mode)
		inlineTimer.SetModeLocked(modeFlag != "")
		inlineTimer.SetOnModeSelected(func(m domain.Methodology) {
			app.methodology = m
			app.mode = methodology.ForMethodology(m, app.config)
			presets = app.mode.Presets()
			inlineTimer.SetMode(app.mode)
		})

		inlineTimer.SetFetchRecentTasks(func(limit int) []*domain.Task {
			tasks, err := app.storage.Tasks().FindRecentTasks(ctx, limit)
			if err != nil {
				return nil
			}
			return tasks
		})

		inlineTimer.SetFetchYesterdayHighlight(func() *domain.Task {
			task, _ := app.storage.Tasks().FindYesterdayHighlight(ctx, time.Now())
			return task
		})

		inlineTimer.SetInlineSetup(presets, breakInfo, func(presetIndex int, taskName string) error {
			currentMode := app.mode
			currentPresets := currentMode.Presets()

			// Parse #tags from task name input
			var sessionTags []string
			if taskName != "" {
				taskName, sessionTags = domain.ParseTagsFromInput(taskName)
			}

			var taskID *string
			if taskName != "" {
				task, err := app.tasks.AddTask(ctx, services.AddTaskRequest{
					Title: taskName,
				})
				if err != nil {
					return err
				}
				taskID = &task.ID

				// Make Time: set as today's highlight
				if currentMode.HasHighlight() {
					task.SetAsHighlight()
					_ = app.storage.Tasks().Update(ctx, task)
				}
			}

			_, err := app.pomodoro.StartPomodoro(ctx, services.StartPomodoroRequest{
				TaskID:      taskID,
				WorkingDir:  workingDir,
				Duration:    currentPresets[presetIndex].Duration,
				Methodology: app.methodology,
				Tags:        sessionTags,
			})
			return err
		})

		inlineTimer.SetFirstRun(app.config.FirstRun)
		if app.config.FirstRun {
			app.config.FirstRun = false
			_ = config.Save(app.config)
		}
		timer = inlineTimer
	} else {
		fsTimer := tui.NewFullscreenTimer(&app.config.Theme)
		fsTimer.SetMode(app.mode)
		lastFullscreenTimer = fsTimer
		timer = fsTimer
	}

	timer.SetAutoBreak(app.config.Pomodoro.AutoBreak)

	// Wire notification toggle: tab key in timer toggles on/off and persists to config
	if t, ok := timer.(*tui.Timer); ok {
		t.SetNotifications(app.config.Notifications.Enabled, func(enabled bool) {
			app.config.Notifications.Enabled = enabled
			if app.notifier != nil {
				app.notifier.SetEnabled(enabled)
			}
			_ = config.Save(app.config)
		})
	}

	timer.SetFetchState(func() *domain.CurrentState {
		newState, err := app.state.GetCurrentState(ctx)
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
			_, err := app.pomodoro.StartPomodoro(ctx, services.StartPomodoroRequest{
				WorkingDir: workingDir,
			})
			return err
		case ports.CmdPause:
			_, err := app.pomodoro.PauseSession(ctx)
			return err
		case ports.CmdResume:
			_, err := app.pomodoro.ResumeSession(ctx)
			return err
		case ports.CmdStop:
			_, err := app.pomodoro.StopSession(ctx)
			return err
		case ports.CmdCancel:
			return app.pomodoro.CancelSession(ctx)
		case ports.CmdBreak:
			_, err := app.pomodoro.StartBreak(ctx, workingDir)
			return err
		default:
			return fmt.Errorf("unknown command: %v", cmd)
		}
	})

	// Wire methodology callbacks
	timer.SetDistractionCallback(func(text string, category string) error {
		activeState, err := app.pomodoro.GetCurrentState(ctx)
		if err != nil || activeState.ActiveSession == nil {
			return nil
		}
		return app.pomodoro.LogDistraction(ctx, activeState.ActiveSession.ID, text, category)
	})

	timer.SetAccomplishmentCallback(func(text string) error {
		// Find the most recently completed session
		recent, err := app.pomodoro.GetRecentSessions(ctx, 1)
		if err != nil || len(recent) == 0 {
			return nil
		}
		return app.pomodoro.SetAccomplishment(ctx, recent[0].ID, text)
	})

	timer.SetShutdownRitualCallback(func(ritual domain.ShutdownRitual) error {
		recent, err := app.pomodoro.GetRecentSessions(ctx, 1)
		if err != nil || len(recent) == 0 {
			return nil
		}
		return app.pomodoro.SetShutdownRitual(ctx, recent[0].ID, ritual)
	})

	timer.SetFocusScoreCallback(func(score int) error {
		recent, err := app.pomodoro.GetRecentSessions(ctx, 1)
		if err != nil || len(recent) == 0 {
			return nil
		}
		return app.pomodoro.SetFocusScore(ctx, recent[0].ID, score)
	})

	timer.SetEnergizeCallback(func(activity string) error {
		recent, err := app.pomodoro.GetRecentSessions(ctx, 1)
		if err != nil || len(recent) == 0 {
			return nil
		}
		return app.pomodoro.SetEnergizeActivity(ctx, recent[0].ID, activity)
	})

	// Compute break info from config
	_, _, _, sessionsBeforeLong := app.config.ToPomodoroDomainConfig()
	shortBreakDur, longBreakDur := app.config.GetBreakDurations(app.methodology)
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
	if app.methodology == domain.MethodologyDeepWork {
		deepWorkStreak, _ = app.pomodoro.GetDeepWorkStreak(ctx, time.Duration(app.config.DeepWork.DeepWorkGoalHours*float64(time.Hour)))
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
		if app.notifier == nil || !app.notifier.IsEnabled() {
			return
		}

		var err error
		switch sessionType {
		case domain.SessionTypeWork:
			err = app.notifier.NotifyPomodoroComplete(formatMinutes(shortBreakDur))
		case domain.SessionTypeShortBreak:
			err = app.notifier.NotifyBreakComplete("Short")
		case domain.SessionTypeLongBreak:
			err = app.notifier.NotifyBreakComplete("Long")
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

// printWelcome shows the first-run welcome screen explaining the three methodologies.
func printWelcome() {
	fmt.Println()
	fmt.Println("  Welcome to Flow!")
	fmt.Println()
	fmt.Println("  Flow supports three productivity methodologies:")
	fmt.Println()
	fmt.Println("    Pomodoro    Classic 25-minute focus sprints with short breaks.")
	fmt.Println("                Great for staying fresh across many tasks.")
	fmt.Println()
	fmt.Println("    Deep Work   Long uninterrupted blocks (90m+) for cognitively")
	fmt.Println("                demanding work. Tracks distractions and ends with a")
	fmt.Println("                shutdown ritual (Cal Newport).")
	fmt.Println()
	fmt.Println("    Make Time   Choose a daily Highlight you'll laser-focus on. Rate")
	fmt.Println("                your focus after each session and log how you'll")
	fmt.Println("                recharge (Knapp & Zeratsky).")
	fmt.Println()
	fmt.Println("  You can change methodology anytime with \"flow config\".")
	fmt.Println()
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

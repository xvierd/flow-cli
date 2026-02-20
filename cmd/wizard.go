package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/adapters/tui"
	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/methodology"
	"github.com/xvierd/flow-cli/internal/services"
)

// lastInlineTimer holds a reference to the inline timer for post-exit action handling.
var lastInlineTimer *tui.Timer

// lastFullscreenTimer holds a reference to the fullscreen timer for session chaining.
var lastFullscreenTimer *tui.Timer

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

	// Deep Work: check if philosophy needs to be configured
	if mode.Name() == domain.MethodologyDeepWork {
		if app.config.DeepWork.Philosophy == "" {
			philosophyItems := []tui.PickerItem{
				{Label: "Rhythmic", Desc: "Daily habit, same time each day"},
				{Label: "Bimodal", Desc: "Alternate deep/shallow periods"},
				{Label: "Journalistic", Desc: "Grab depth whenever possible"},
				{Label: "Monastic", Desc: "Deep work is your primary work"},
			}
			result := tui.RunPicker("Deep Work philosophy:", philosophyItems, "", &app.config.Theme)
			if !result.Aborted {
				philosophies := []string{"rhythmic", "bimodal", "journalistic", "monastic"}
				app.config.DeepWork.Philosophy = philosophies[result.Index]
				_ = config.Save(app.config)
				// Refresh mode to pick up new philosophy
				app.mode = methodology.ForMethodology(app.methodology, app.config)
				mode = app.mode
			}
			fmt.Println()
		}
	}

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

		// 1. Pick duration with arrow-key picker (mode-specific presets)
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

		// Laser checklist (Make Time only)
		if mode.HasLaserChecklist() {
			fmt.Println()
			fmt.Println("  Laser Checklist:")
			checklistItems := []string{
				"Phone on Do Not Disturb?",
				"Notifications off?",
				"Distracting tabs/apps closed?",
			}
			for _, item := range checklistItems {
				checkResult := tui.RunPicker(item, []tui.PickerItem{
					{Label: "Yes", Desc: "Ready to focus"},
					{Label: "No", Desc: "Skip for now"},
				}, "", &app.config.Theme)
				if checkResult.Aborted {
					return nil
				}
				// User can skip with "No" and still proceed
			}
			fmt.Println()
		}

		// 2. Task selection via styled picker
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

		// 3. Deep Work: ask for intended outcome
		var intendedOutcome string
		if mode.OutcomePrompt() != "" {
			outcomeResult := tui.RunTextPrompt(mode.OutcomePrompt(), "Enter to skip", &app.config.Theme)
			if outcomeResult.Aborted {
				return nil
			}
			intendedOutcome = outcomeResult.Value
		}

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

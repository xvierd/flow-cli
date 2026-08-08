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
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/methodology"
	"github.com/xvierd/flow-cli/internal/ports"
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
		return fmt.Errorf("%s: %w", i18n.T("failed to get current state"), err)
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
		sessionInfo := i18n.T("%s (%s remaining)", sessionType, formatWizardDuration(remaining))

		if state.ActiveTask != nil {
			sessionInfo = i18n.T("%s for \"%s\" (%s remaining)", sessionType, state.ActiveTask.Title, formatWizardDuration(remaining))
		}

		resumeItems := []tui.PickerItem{
			{Label: i18n.T("Resume"), Desc: sessionInfo},
			{Label: i18n.T("Stop"), Desc: i18n.T("End current session and start fresh")},
		}
		resumeResult := tui.RunPicker(i18n.T("Active session:"), resumeItems, "", &app.config.Theme)
		if resumeResult.Aborted {
			return nil
		}

		if resumeResult.Index == 0 {
			return launchTUI(ctx, state, workingDir)
		}

		_, err := app.pomodoro.StopSession(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to stop current session"), err)
		}
	}

	// --- Wizard prompts ---
	fmt.Println()

	// Main menu (skip if --mode was explicitly passed — user wants to start a session)
	if modeFlag == "" {
		menuItems := []tui.PickerItem{
			{Label: i18n.T("Start session"), Desc: i18n.T("Begin a new focus session")},
			{Label: i18n.T("View stats"), Desc: i18n.T("Show your productivity dashboard")},
			{Label: i18n.T("Reflect"), Desc: i18n.T("Weekly reflection on your work")},
			{Label: i18n.T("Report"), Desc: i18n.T("Aggregated weekly or monthly report")},
		}
		menuResult := tui.RunPicker(i18n.T("Flow:"), menuItems, "", &app.config.Theme)
		if menuResult.Aborted {
			return nil
		}
		switch menuResult.Index {
		case 1: // View stats
			return statsCmd.RunE(cmd, args)
		case 2: // Reflect
			return reflectCmd.RunE(cmd, args)
		case 3: // Report
			return reportCmd.RunE(cmd, args)
		}
		// Index 0: Start session — continue to mode picker
		fmt.Println()
	}

	// Mode picker (skip if --mode was explicitly passed)
	if modeFlag == "" {
		// Methodology names stay untranslated (proper nouns).
		modeItems := []tui.PickerItem{
			{Label: "Pomodoro", Desc: i18n.T("Classic 25/5 timer")},
			{Label: "Deep Work", Desc: i18n.T("Longer sessions, distraction tracking")},
			{Label: "Make Time", Desc: i18n.T("Daily Highlight, focus scoring")},
		}
		modeResult := tui.RunPicker(i18n.T("Mode:"), modeItems, "", &app.config.Theme)
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
				{Label: i18n.T("Rhythmic"), Desc: i18n.T("Daily habit, same time each day")},
				{Label: i18n.T("Bimodal"), Desc: i18n.T("Alternate deep/shallow periods")},
				{Label: i18n.T("Journalistic"), Desc: i18n.T("Grab depth whenever possible")},
				{Label: i18n.T("Monastic"), Desc: i18n.T("Deep work is your primary work")},
			}
			result := tui.RunPicker(i18n.T("Deep Work philosophy:"), philosophyItems, "", &app.config.Theme)
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
				fmt.Printf("  %s\n\n", i18n.T("Today's Highlight: \"%s\"", highlight.Title))
			} else {
				yesterdayHighlight, _ := app.storage.Tasks().FindYesterdayHighlight(ctx, time.Now())
				if yesterdayHighlight != nil {
					carryItems := []tui.PickerItem{
						{Label: i18n.T("Yes"), Desc: i18n.T("Continue with \"%s\"", yesterdayHighlight.Title)},
						{Label: i18n.T("No"), Desc: i18n.T("Pick a new Highlight")},
					}
					carryResult := tui.RunPicker(i18n.T("Carry forward yesterday's Highlight?"), carryItems, "", &app.config.Theme)
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
			footer = i18n.T("Breaks: %s short / %s long (every %d) · \"flow config\" to customize",
				formatMinutes(shortBreak), formatMinutes(longBreak), app.config.Pomodoro.SessionsBeforeLong)
		} else {
			footer = i18n.T("Break: %s · \"flow config\" to customize", formatMinutes(shortBreak))
		}

		result := tui.RunPicker(i18n.T("Duration:"), items, footer, &app.config.Theme)
		if result.Aborted {
			return nil
		}

		customDuration := presets[result.Index].Duration

		// Laser checklist (Make Time only)
		if mode.HasLaserChecklist() {
			fmt.Println()
			fmt.Println("  " + i18n.T("Laser Checklist:"))
			checklistItems := []string{
				i18n.T("Phone on Do Not Disturb?"),
				i18n.T("Notifications off?"),
				i18n.T("Distracting tabs/apps closed?"),
			}
			for _, item := range checklistItems {
				checkResult := tui.RunPicker(item, []tui.PickerItem{
					{Label: i18n.T("Yes"), Desc: i18n.T("Ready to focus")},
					{Label: i18n.T("No"), Desc: i18n.T("Skip for now")},
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
				Label: i18n.T("New task..."),
				Desc:  i18n.T("Type a name"),
			})

			taskResult := tui.RunPicker(i18n.T(mode.TaskPrompt()), taskItems, "", &app.config.Theme)
			if taskResult.Aborted {
				return nil
			}

			if taskResult.Index < len(recentTasks) {
				taskName = recentTasks[taskResult.Index].Title
				taskID = &recentTasks[taskResult.Index].ID
			} else {
				// "New task" selected — prompt for name
				textResult := tui.RunTextPrompt(i18n.T(mode.TaskPrompt()), i18n.T("Enter to skip"), &app.config.Theme)
				if textResult.Aborted {
					return nil
				}
				taskName = textResult.Value
			}
		} else {
			textResult := tui.RunTextPrompt(i18n.T(mode.TaskPrompt()), i18n.T("Enter to skip"), &app.config.Theme)
			if textResult.Aborted {
				return nil
			}
			taskName = textResult.Value
		}

		// Parse #tags from task name input
		if taskName != "" {
			taskName, sessionTags = domain.ParseTagsFromInput(taskName)
		}

		// 3. Deep Work: ask for intended outcome
		var intendedOutcome string
		if mode.OutcomePrompt() != "" {
			outcomeResult := tui.RunTextPrompt(i18n.T(mode.OutcomePrompt()), i18n.T("Enter to skip"), &app.config.Theme)
			if outcomeResult.Aborted {
				return nil
			}
			intendedOutcome = outcomeResult.Value
		}

		// Start the session; task creation, highlight update, and session
		// start are one atomic unit.
		err = app.storage.WithTx(ctx, func(tx ports.Storage) error {
			if taskName != "" && taskID == nil {
				task, err := app.tasks.AddTaskWith(ctx, tx, services.AddTaskRequest{
					Title: taskName,
				})
				if err != nil {
					return fmt.Errorf("%s: %w", i18n.T("failed to create task"), err)
				}
				taskID = &task.ID

				// Make Time: set as today's highlight
				if mode.HasHighlight() {
					task.SetAsHighlight()
					if err := tx.Tasks().Update(ctx, task); err != nil {
						return err
					}
				}
			}

			req := services.StartPomodoroRequest{
				TaskID:          taskID,
				WorkingDir:      workingDir,
				Duration:        customDuration,
				Methodology:     app.methodology,
				IntendedOutcome: intendedOutcome,
				Tags:            sessionTags,
			}
			_, err := app.pomodoro.StartPomodoroWith(ctx, tx, req)
			return err
		})
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to start pomodoro"), err)
		}

		// Refresh state and launch TUI
		state, err = app.state.GetCurrentState(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to get current state"), err)
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
		fmt.Print("\n" + i18n.T("What's your Highlight for tomorrow? (Enter to skip): "))
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
// Methodology names are kept as-is (proper nouns); only descriptions translate.
func printWelcome() {
	fmt.Println()
	fmt.Println("  " + i18n.T("Welcome to Flow!"))
	fmt.Println()
	fmt.Println("  " + i18n.T("Flow supports three productivity methodologies:"))
	fmt.Println()
	fmt.Println("    Pomodoro    " + i18n.T("Classic 25-minute focus sprints with short breaks."))
	fmt.Println("                " + i18n.T("Great for staying fresh across many tasks."))
	fmt.Println()
	fmt.Println("    Deep Work   " + i18n.T("Long uninterrupted blocks (90m+) for cognitively"))
	fmt.Println("                " + i18n.T("demanding work. Tracks distractions and ends with a"))
	fmt.Println("                " + i18n.T("shutdown ritual (Cal Newport)."))
	fmt.Println()
	fmt.Println("    Make Time   " + i18n.T("Choose a daily Highlight you'll laser-focus on. Rate"))
	fmt.Println("                " + i18n.T("your focus after each session and log how you'll"))
	fmt.Println("                " + i18n.T("recharge (Knapp & Zeratsky)."))
	fmt.Println()
	fmt.Println("  " + i18n.T("You can change methodology anytime with \"flow config\"."))
	fmt.Println()
}

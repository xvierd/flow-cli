package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/adapters/tui"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/methodology"
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
		_ = saveConfig(app.config)
	}

	// Check for active session
	state, err := app.state.GetCurrentState(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to get current state"), err)
	}

	// Inline mode: entire flow runs inside a single bubbletea program
	if inlineMode {
		adoptSessionMethodology(state)
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
			if err := runSessions(state, workingDir); err != nil {
				return err
			}
			return promptTomorrowHighlight()
		}

		if _, err := app.pomodoro.StopSession(ctx); err != nil {
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
		setMethodology(methodologies[modeResult.Index])
		fmt.Println()
	}

	// Deep Work: check if philosophy needs to be configured
	if app.mode.Name() == domain.MethodologyDeepWork {
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
				_ = saveConfig(app.config)
				// Refresh mode to pick up new philosophy
				app.mode = methodology.ForMethodology(app.methodology, app.config)
			}
			fmt.Println()
		}
	}

	wizard := newSessionWizard(workingDir)
	state, err = wizard.startSession(ctx)
	if err != nil || state == nil {
		return err
	}
	if err := wizard.run(state); err != nil {
		return err
	}
	return promptTomorrowHighlight()
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

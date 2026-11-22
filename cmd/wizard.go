package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

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

	// Check for active session
	state, err := app.state.GetCurrentState(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to get current state"), err)
	}

	sessions, err := allSessions(ctx)
	if err != nil {
		return err
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

	// First run (empty database): one welcome screen that can start right away.
	if len(sessions) == 0 {
		switch runWelcome() {
		case welcomeAbort:
			return nil
		case welcomeStartNow:
			return runQuickStart(ctx, quickStart{}, false)
		}
		fmt.Println()
	} else if modeFlag == "" {
		// Main menu (skipped when --mode was passed: the user wants a session).
		plan, ok := lastSessionPlan(sessions)
		nowDesc := describePlan(ctx, plan, ok)
		if ok {
			nowDesc = i18n.T("Repeat last: %s", nowDesc)
		}
		menuItems := []tui.PickerItem{
			{Label: i18n.T("Start now"), Desc: nowDesc},
			{Label: i18n.T("New session"), Desc: i18n.T("Pick duration and task")},
			{Label: i18n.T("View stats"), Desc: i18n.T("Show your productivity dashboard")},
			{Label: i18n.T("Reflect"), Desc: i18n.T("Weekly reflection on your work")},
			{Label: i18n.T("Report"), Desc: i18n.T("Aggregated weekly or monthly report")},
		}
		menuResult := tui.RunPicker(i18n.T("Flow:"), menuItems, "", &app.config.Theme)
		if menuResult.Aborted {
			return nil
		}
		switch menuResult.Index {
		case 0: // Start now: what "flow go" does
			return goCmd.RunE(cmd, nil)
		case 2:
			return statsCmd.RunE(cmd, args)
		case 3:
			return reflectCmd.RunE(cmd, args)
		case 4:
			return reportCmd.RunE(cmd, args)
		}
		// Index 1: New session — continue to the duration picker
		fmt.Println()
	}

	// The mode step is only a step when no default methodology is
	// configured; otherwise [m] on the duration picker changes it.
	if modeFlag == "" && app.config.Methodology == "" {
		if !pickMethodology() {
			return nil
		}
	} else {
		ensureDeepWorkPhilosophy()
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

// pickMethodology runs the mode picker (Pomodoro first as the default, the
// other two as advanced modes) and the Deep Work philosophy setup. It reports
// false when the user cancelled the picker.
func pickMethodology() bool {
	// Methodology names stay untranslated (proper nouns).
	modeItems := []tui.PickerItem{
		{Label: "Pomodoro", Desc: i18n.T("Default · classic 25/5 timer")},
		{Label: "Deep Work", Desc: i18n.T("Advanced · longer sessions, distraction tracking")},
		{Label: "Make Time", Desc: i18n.T("Advanced · daily Highlight, focus scoring")},
	}
	methodologies := []domain.Methodology{domain.MethodologyPomodoro, domain.MethodologyDeepWork, domain.MethodologyMakeTime}
	cursor := 0
	for i, m := range methodologies {
		if m == app.methodology {
			cursor = i
		}
	}
	modeResult := tui.RunPickerWith(i18n.T("Mode:"), modeItems, "", &app.config.Theme, tui.PickerOptions{Cursor: cursor})
	if modeResult.Aborted {
		return false
	}
	setMethodology(methodologies[modeResult.Index])
	fmt.Println()
	ensureDeepWorkPhilosophy()
	return true
}

// ensureDeepWorkPhilosophy asks once, in Deep Work, for the philosophy.
func ensureDeepWorkPhilosophy() {
	if app.mode.Name() != domain.MethodologyDeepWork || app.config.DeepWork.Philosophy != "" {
		return
	}
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

// welcomeChoice is the outcome of the first-run welcome screen.
type welcomeChoice int

const (
	welcomeStartNow welcomeChoice = iota
	welcomeSetup
	welcomeAbort
)

// runWelcome shows the first-run screen: what Flow does in three lines, an
// immediate start, and three commands worth knowing.
func runWelcome() welcomeChoice {
	fmt.Print(welcomeText())
	items := []tui.PickerItem{
		{Label: i18n.T("Start now"), Desc: i18n.T("%s · %s · no questions asked", app.methodology.Label(), formatMinutes(defaultDurationFor(app.methodology)))},
		{Label: i18n.T("Set up"), Desc: i18n.T("Pick duration and task first")},
	}
	result := tui.RunPicker(i18n.T("Ready?"), items, "", &app.config.Theme)
	switch {
	case result.Aborted:
		return welcomeAbort
	case result.Index == 0:
		return welcomeStartNow
	default:
		return welcomeSetup
	}
}

// welcomeText is the body of the first-run welcome screen.
func welcomeText() string {
	var b strings.Builder
	b.WriteString("  " + i18n.T("Welcome to Flow!") + "\n\n")
	b.WriteString("  " + i18n.T("Flow times focused work sessions from your terminal, with breaks in between.") + "\n")
	b.WriteString("  " + i18n.T("Pomodoro (25 min) is the default; Deep Work and Make Time are advanced modes.") + "\n")
	b.WriteString("  " + i18n.T("Everything stays in a local database; your git branch is recorded with each session.") + "\n\n")
	b.WriteString("  " + i18n.T("Useful commands:") + "\n")
	b.WriteString("    flow go                    " + i18n.T("repeat your last session") + "\n")
	b.WriteString("    flow 25 \"fix auth #api\"    " + i18n.T("start 25 min on a task, tagged #api") + "\n")
	b.WriteString("    flow stats                 " + i18n.T("see how your week went") + "\n")
	return b.String()
}

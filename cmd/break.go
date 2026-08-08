package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
)

// breakCmd represents the break command
var breakCmd = &cobra.Command{
	Use:   "break",
	Short: "Start a break session",
	Long:  `Start a pomodoro break session (short or long depending on work completed).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		workingDir, _ := os.Getwd()

		// Start a break session
		session, err := app.pomodoro.StartBreak(ctx, workingDir)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to start break"), err)
		}

		fmt.Printf("☕ %s\n", i18n.T("Break started! Duration: %s (%s)",
			session.Duration,
			getBreakTypeLabel(session.Type)))

		// Get the current state for the TUI
		state, err := app.state.GetCurrentState(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to get current state"), err)
		}

		return launchTUI(ctx, state, workingDir)
	},
}

func getBreakTypeLabel(sessionType domain.SessionType) string {
	switch sessionType {
	case domain.SessionTypeShortBreak:
		return i18n.T("Short Break")
	case domain.SessionTypeLongBreak:
		return i18n.T("Long Break")
	default:
		return i18n.T("Break")
	}
}

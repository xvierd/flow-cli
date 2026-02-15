package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/domain"
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
		session, err := pomodoroSvc.StartBreak(ctx, workingDir)
		if err != nil {
			return fmt.Errorf("failed to start break: %w", err)
		}

		fmt.Printf("☕ Break started! Duration: %s (%s)\n",
			session.Duration,
			getBreakTypeLabel(session.Type))

		// Get the current state for the TUI
		state, err := stateService.GetCurrentState(ctx)
		if err != nil {
			return fmt.Errorf("failed to get current state: %w", err)
		}

		return launchTUI(ctx, state, workingDir)
	},
}

func getBreakTypeLabel(sessionType domain.SessionType) string {
	switch sessionType {
	case domain.SessionTypeShortBreak:
		return "Short Break"
	case domain.SessionTypeLongBreak:
		return "Long Break"
	default:
		return "Break"
	}
}

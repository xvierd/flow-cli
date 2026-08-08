package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/i18n"
)

// pauseCmd represents the pause command
var pauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Pause the current pomodoro session",
	Long:  `Pause the currently running pomodoro session.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		session, err := app.pomodoro.PauseSession(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to pause session"), err)
		}

		fmt.Printf("⏸️  %s\n", i18n.T("Session paused. Remaining: %s", session.RemainingTime()))
		return nil
	},
}

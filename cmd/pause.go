package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// pauseCmd represents the pause command
var pauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Pause the current pomodoro session",
	Long:  `Pause the currently running pomodoro session.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		session, err := pomodoroSvc.PauseSession(ctx)
		if err != nil {
			return fmt.Errorf("failed to pause session: %w", err)
		}

		fmt.Printf("⏸️  Session paused. Remaining: %s\n", session.RemainingTime())
		return nil
	},
}

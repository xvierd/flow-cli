package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// stopCmd represents the stop command
var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the current pomodoro session",
	Long:  `Complete the current active pomodoro session.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		session, err := pomodoroSvc.StopSession(ctx)
		if err != nil {
			return fmt.Errorf("failed to stop session: %w", err)
		}

		fmt.Printf("✅ Session completed! Duration: %s\n", session.Duration)
		if session.TaskID != nil {
			fmt.Printf("   Task ID: %s\n", *session.TaskID)
		}

		return nil
	},
}

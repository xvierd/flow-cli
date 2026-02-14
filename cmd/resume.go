package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// resumeCmd represents the resume command
var resumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume the paused pomodoro session",
	Long:  `Resume a previously paused pomodoro session.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		session, err := pomodoroSvc.ResumeSession(ctx)
		if err != nil {
			return fmt.Errorf("failed to resume session: %w", err)
		}

		fmt.Printf("▶️  Session resumed. Remaining: %s\n", session.RemainingTime())
		return nil
	},
}

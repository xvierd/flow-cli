package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/i18n"
)

// resumeCmd represents the resume command
var resumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume the paused session",
	Long:  `Resume the paused session; you are notified when it ends.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		session, err := app.pomodoro.ResumeSession(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to resume session"), err)
		}

		fmt.Printf("▶️  %s\n", i18n.T("Session resumed. Remaining: %s", formatCmdDuration(session.RemainingTime())))
		ensureWatcher()
		return nil
	},
}

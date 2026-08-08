package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/i18n"
)

// completeCmd represents the complete command
var completeCmd = &cobra.Command{
	Use:   "complete [task-id]",
	Short: "Complete a task",
	Long:  `Mark a task as completed.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		taskID := args[0]

		if err := app.tasks.CompleteTask(ctx, taskID); err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to complete task"), err)
		}

		fmt.Printf("✅ %s\n", i18n.T("Task completed (ID: %s)", taskID))
		return nil
	},
}

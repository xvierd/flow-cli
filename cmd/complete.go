package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
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

		if err := taskService.CompleteTask(ctx, taskID); err != nil {
			return fmt.Errorf("failed to complete task: %w", err)
		}

		fmt.Printf("✅ Task completed (ID: %s)\n", taskID)
		return nil
	},
}

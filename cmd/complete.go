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
	Long:  `Mark a task as completed. The short ID from "flow list" works.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		task, err := app.tasks.GetTask(ctx, args[0])
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to complete task"), err)
		}

		if err := app.tasks.CompleteTask(ctx, task.ID); err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to complete task"), err)
		}

		if jsonOutput {
			return jsonOut(map[string]interface{}{"completed": true, "task_id": task.ID, "title": task.Title})
		}
		fmt.Printf("✅ %s\n", i18n.T("Task completed: %s (ID: %s)", task.Title, task.ID[:8]))
		return nil
	},
}

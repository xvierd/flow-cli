package cmd

import (
	"context"
	"fmt"

	"github.com/dvidx/flow-cli/internal/domain"
	"github.com/spf13/cobra"
)

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:   "delete [task-id]",
	Short: "Delete a task",
	Long:  `Delete a task by its ID. Use with caution - this cannot be undone.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		taskID := args[0]

		// Get task info first for confirmation
		task, err := taskService.GetTask(ctx, taskID)
		if err != nil {
			if err == domain.ErrTaskNotFound {
				return fmt.Errorf("task not found: %s", taskID)
			}
			return fmt.Errorf("failed to get task: %w", err)
		}

		// Confirm deletion
		if !jsonOutput {
			fmt.Printf("Are you sure you want to delete task '%s' (%s)? [y/N]: ", task.Title, task.ID[:8])
			var confirm string
			fmt.Scanln(&confirm)
			if confirm != "y" && confirm != "Y" {
				fmt.Println("Deletion cancelled.")
				return nil
			}
		}

		// Delete the task
		err = taskService.DeleteTask(ctx, taskID)
		if err != nil {
			if err == domain.ErrTaskNotFound {
				return fmt.Errorf("task not found: %s", taskID)
			}
			return fmt.Errorf("failed to delete task: %w", err)
		}

		if jsonOutput {
			fmt.Printf(`{"deleted": true, "task_id": "%s"}\n`, taskID)
		} else {
			fmt.Printf("✅ Task '%s' deleted successfully.\n", task.Title)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}

package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
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
		task, err := app.tasks.GetTask(ctx, taskID)
		if err != nil {
			if errors.Is(err, domain.ErrTaskNotFound) {
				return fmt.Errorf("%s", i18n.T("task not found: %s", taskID))
			}
			return fmt.Errorf("%s: %w", i18n.T("failed to get task"), err)
		}

		// Confirm deletion
		if !jsonOutput {
			fmt.Printf("%s", i18n.T("Are you sure you want to delete task '%s' (%s)? [y/N]: ", task.Title, task.ID[:8]))
			reader := bufio.NewReader(os.Stdin)
			confirm, _ := reader.ReadString('\n')
			confirm = strings.TrimSpace(confirm)
			if confirm != "y" && confirm != "Y" {
				fmt.Println(i18n.T("Deletion cancelled."))
				return nil
			}
		}

		// Delete the task
		err = app.tasks.DeleteTask(ctx, taskID)
		if err != nil {
			if errors.Is(err, domain.ErrTaskNotFound) {
				return fmt.Errorf("%s", i18n.T("task not found: %s", taskID))
			}
			return fmt.Errorf("%s: %w", i18n.T("failed to delete task"), err)
		}

		if jsonOutput {
			return jsonOut(map[string]interface{}{
				"deleted": true,
				"task_id": taskID,
			})
		}
		fmt.Printf("✅ %s\n", i18n.T("Task '%s' deleted successfully.", task.Title))
		return nil
	},
}

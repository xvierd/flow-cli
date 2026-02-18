package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/domain"
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
			if err == domain.ErrTaskNotFound {
				return fmt.Errorf("task not found: %s", taskID)
			}
			return fmt.Errorf("failed to get task: %w", err)
		}

		// Confirm deletion
		if !jsonOutput {
			fmt.Printf("Are you sure you want to delete task '%s' (%s)? [y/N]: ", task.Title, task.ID[:8])
			reader := bufio.NewReader(os.Stdin)
			confirm, _ := reader.ReadString('\n')
			confirm = strings.TrimSpace(confirm)
			if confirm != "y" && confirm != "Y" {
				fmt.Println("Deletion cancelled.")
				return nil
			}
		}

		// Delete the task
		err = app.tasks.DeleteTask(ctx, taskID)
		if err != nil {
			if err == domain.ErrTaskNotFound {
				return fmt.Errorf("task not found: %s", taskID)
			}
			return fmt.Errorf("failed to delete task: %w", err)
		}

		if jsonOutput {
			data, _ := json.Marshal(map[string]interface{}{
				"deleted": true,
				"task_id": taskID,
			})
			fmt.Println(string(data))
		} else {
			fmt.Printf("✅ Task '%s' deleted successfully.\n", task.Title)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}

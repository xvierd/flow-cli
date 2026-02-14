package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/dvidx/flow-cli/internal/domain"
	"github.com/dvidx/flow-cli/internal/services"
)

var (
	listStatus string
	listAll    bool
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	Long:  `List all tasks, or filter by status.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		req := services.ListTasksRequest{
			OnlyPending: !listAll && listStatus == "",
		}

		if listStatus != "" {
			status := domain.TaskStatus(listStatus)
			req.Status = &status
		}

		tasks, err := taskService.ListTasks(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to list tasks: %w", err)
		}

		if len(tasks) == 0 {
			fmt.Println("No tasks found.")
			return nil
		}

		fmt.Printf("📋 Tasks (%d):\n\n", len(tasks))
		for _, task := range tasks {
			statusIcon := getStatusIcon(task.Status)
			fmt.Printf("%s %s (ID: %s)\n", statusIcon, task.Title, task.ID[:8])
			if len(task.Tags) > 0 {
				fmt.Printf("   Tags: %v\n", task.Tags)
			}
		}

		return nil
	},
}

func init() {
	listCmd.Flags().StringVarP(&listStatus, "status", "s", "", "Filter by status (pending, in_progress, completed, cancelled)")
	listCmd.Flags().BoolVarP(&listAll, "all", "a", false, "List all tasks (default: pending only)")
}

func getStatusIcon(status domain.TaskStatus) string {
	switch status {
	case domain.StatusPending:
		return "⏳"
	case domain.StatusInProgress:
		return "▶️"
	case domain.StatusCompleted:
		return "✅"
	case domain.StatusCancelled:
		return "❌"
	default:
		return "❓"
	}
}

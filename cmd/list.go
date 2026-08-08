package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/services"
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

		tasks, err := app.tasks.ListTasks(ctx, req)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to list tasks"), err)
		}

		if jsonOutput {
			var taskList []map[string]interface{}
			for _, task := range tasks {
				taskList = append(taskList, map[string]interface{}{
					"id":          task.ID,
					"title":       task.Title,
					"description": task.Description,
					"status":      string(task.Status),
					"tags":        task.Tags,
					"created_at":  task.CreatedAt.Format("2006-01-02T15:04:05"),
				})
			}
			data := map[string]interface{}{
				"tasks": taskList,
				"count": len(taskList),
			}
			jsonData, err := json.MarshalIndent(data, "", "  ")
			if err != nil {
				return fmt.Errorf("%s: %w", i18n.T("failed to marshal tasks"), err)
			}
			fmt.Println(string(jsonData))
			return nil
		}

		if len(tasks) == 0 {
			fmt.Println(i18n.T("No tasks found."))
			return nil
		}

		fmt.Printf("📋 %s\n\n", i18n.T("Tasks (%d):", len(tasks)))
		for _, task := range tasks {
			statusIcon := getStatusIcon(task.Status)
			fmt.Printf("%s %s (ID: %s)\n", statusIcon, task.Title, task.ID[:8])
			if len(task.Tags) > 0 {
				fmt.Printf("%s\n", i18n.T("   Tags: %v", task.Tags))
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

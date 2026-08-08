package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/services"
)

var addTags []string

// addCmd represents the add command
var addCmd = &cobra.Command{
	Use:   "add [title]",
	Short: "Add a new task",
	Long:  `Add a new task to the Flow task list.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Combine all arguments as the title
		title := ""
		for i, arg := range args {
			if i > 0 {
				title += " "
			}
			title += arg
		}

		req := services.AddTaskRequest{
			Title:       title,
			Description: "",
			Tags:        addTags,
		}

		task, err := app.tasks.AddTask(ctx, req)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to add task"), err)
		}

		if jsonOutput {
			data := map[string]interface{}{
				"id":          task.ID,
				"title":       task.Title,
				"description": task.Description,
				"status":      string(task.Status),
				"tags":        task.Tags,
				"created_at":  task.CreatedAt.Format("2006-01-02T15:04:05"),
			}
			jsonData, err := json.MarshalIndent(data, "", "  ")
			if err != nil {
				return fmt.Errorf("%s: %w", i18n.T("failed to marshal task"), err)
			}
			fmt.Println(string(jsonData))
			return nil
		}

		fmt.Printf("✅ %s\n", i18n.T("Task added: %s (ID: %s)", task.Title, task.ID))
		return nil
	},
}

func init() {
	addCmd.Flags().StringArrayVarP(&addTags, "tags", "t", []string{}, "Tags for the task")
}

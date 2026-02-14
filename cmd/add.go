package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/dvidx/flow-cli/internal/services"
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

		task, err := taskService.AddTask(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to add task: %w", err)
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
				return fmt.Errorf("failed to marshal task: %w", err)
			}
			fmt.Println(string(jsonData))
			return nil
		}

		fmt.Printf("✅ Task added: %s (ID: %s)\n", task.Title, task.ID)
		return nil
	},
}

func init() {
	addCmd.Flags().StringArrayVarP(&addTags, "tags", "t", []string{}, "Tags for the task")
}

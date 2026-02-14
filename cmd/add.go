package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/xavier/flow/internal/services"
)

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
		}

		task, err := taskService.AddTask(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to add task: %w", err)
		}

		fmt.Printf("✅ Task added: %s (ID: %s)\n", task.Title, task.ID)
		return nil
	},
}

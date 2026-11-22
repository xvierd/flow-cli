package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/services"
)

var (
	editTitle       string
	editDescription string
	editTags        []string
	editReopen      bool
)

// editCmd represents the edit command
var editCmd = &cobra.Command{
	Use:   "edit <task-id> [--title ...] [--description ...] [--tags a,b] [--reopen]",
	Short: "Edit a task's title, description or tags",
	Long: `Edit a task. The task ID may be the short ID shown by "flow list".
Only the flags you pass are changed. --tags replaces all tags (--tags "" clears
them); --reopen moves a completed task back to pending.`,
	Example: `  flow edit 4f894d3e --title "Write API docs v2"
  flow edit 4f894d3e --tags docs,backend
  flow edit 4f894d3e --reopen`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		flags := cmd.Flags()

		var req services.UpdateTaskRequest
		if flags.Changed("title") {
			req.Title = &editTitle
		}
		if flags.Changed("description") {
			req.Description = &editDescription
		}
		if flags.Changed("tags") {
			tags := splitTags(editTags)
			req.Tags = &tags
		}
		req.Reopen = editReopen

		if req.Title == nil && req.Description == nil && req.Tags == nil && !req.Reopen {
			return errors.New(i18n.T("nothing to change: use --title, --description, --tags or --reopen"))
		}

		task, err := app.tasks.UpdateTask(ctx, args[0], req)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to edit task"), err)
		}

		if jsonOutput {
			return jsonOut(map[string]interface{}{
				"id":          task.ID,
				"title":       task.Title,
				"description": task.Description,
				"status":      string(task.Status),
				"tags":        append([]string{}, task.Tags...),
				"updated_at":  task.UpdatedAt.Format(time.RFC3339),
			})
		}
		fmt.Printf("✅ %s\n", i18n.T("Task updated: %s (ID: %s)", task.Title, task.ID[:8]))
		if len(task.Tags) > 0 {
			fmt.Printf("%s\n", i18n.T("   Tags: %s", strings.Join(task.Tags, ", ")))
		}
		return nil
	},
}

// splitTags flattens repeated and comma-separated tag flags, dropping blanks.
func splitTags(values []string) []string {
	tags := []string{}
	for _, v := range values {
		for _, t := range strings.Split(v, ",") {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
	}
	return tags
}

func init() {
	editCmd.Flags().StringVar(&editTitle, "title", "", "New title")
	editCmd.Flags().StringVar(&editDescription, "description", "", "New description")
	editCmd.Flags().StringSliceVarP(&editTags, "tags", "t", nil, "Replace the tags (repeatable or comma-separated; \"\" clears them)")
	editCmd.Flags().BoolVar(&editReopen, "reopen", false, "Move a completed or cancelled task back to pending")
}

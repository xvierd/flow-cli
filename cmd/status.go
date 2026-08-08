package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/adapters/tui"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current status",
	Long:  `Display the current pomodoro session status and today's statistics.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		state, err := app.state.GetCurrentState(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to get current state"), err)
		}

		if jsonOutput {
			return outputStatusJSON(state)
		}

		// Use the TUI to display the status
		tui.ShowStatus(state, &app.config.Theme)

		// Show today's Highlight for Make Time mode
		if app.methodology == domain.MethodologyMakeTime {
			highlight, _ := app.state.GetTodayHighlight(ctx)
			if highlight != nil {
				fmt.Printf("\n%s\n", i18n.T("Highlight: %s", highlight.Title))
			} else if state.ActiveTask != nil {
				fmt.Printf("\n%s\n", i18n.T("Highlight: %s", state.ActiveTask.Title))
			}
		}

		return nil
	},
}

// outputStatusJSON outputs the status in JSON format
func outputStatusJSON(state *domain.CurrentState) error {
	ctx := context.Background()

	result := map[string]interface{}{
		"active_task":    nil,
		"active_session": nil,
		"highlight":      nil,
		"today_stats": map[string]interface{}{
			"work_sessions":   state.TodayStats.WorkSessions,
			"breaks_taken":    state.TodayStats.BreaksTaken,
			"total_work_time": state.TodayStats.TotalWorkTime.String(),
		},
	}

	if state.ActiveTask != nil {
		result["active_task"] = map[string]interface{}{
			"id":          state.ActiveTask.ID,
			"title":       state.ActiveTask.Title,
			"description": state.ActiveTask.Description,
			"status":      string(state.ActiveTask.Status),
			"tags":        state.ActiveTask.Tags,
		}
	}

	if state.ActiveSession != nil {
		session := state.ActiveSession
		sessionData := map[string]interface{}{
			"id":             session.ID,
			"type":           string(session.Type),
			"status":         string(session.Status),
			"duration":       session.Duration.String(),
			"remaining_time": session.RemainingTime().String(),
			"progress":       session.Progress(),
			"started_at":     session.StartedAt.Format("2006-01-02T15:04:05"),
			"git_branch":     session.GitBranch,
			"git_commit":     session.GitCommit,
			"notes":          session.Notes,
		}
		if session.TaskID != nil {
			sessionData["task_id"] = *session.TaskID
		}
		result["active_session"] = sessionData
	}

	// Include highlight for Make Time mode
	if app.methodology == domain.MethodologyMakeTime {
		highlight, _ := app.state.GetTodayHighlight(ctx)
		if highlight != nil {
			result["highlight"] = map[string]interface{}{
				"id":     highlight.ID,
				"title":  highlight.Title,
				"status": string(highlight.Status),
			}
		}
	}

	return jsonOut(result)
}

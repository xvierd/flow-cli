package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/adapters/tui"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current status",
	Long:  `Display the active session (time left, task, git branch) and today's statistics.`,
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
		printStatusGit(ctx, state)

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
			"started_at":     session.StartedAt.Format(time.RFC3339),
			"git_branch":     session.GitBranch,
			"git_commit":     session.GitCommit,
			"notes":          session.Notes,
		}
		if session.TaskID != nil {
			sessionData["task_id"] = *session.TaskID
		}
		if live := app.pomodoro.LiveGitSummary(ctx, session); live != nil {
			sessionData["git_summary"] = live
		}
		result["active_session"] = sessionData
	}

	result["last_session"] = nil
	if last := app.pomodoro.LastEndedWorkSession(ctx); last != nil {
		lastData := map[string]interface{}{
			"id":         last.ID,
			"status":     string(last.Status),
			"duration":   last.Duration.String(),
			"started_at": last.StartedAt.Format(time.RFC3339),
			"git_branch": last.GitBranch,
			"git_commit": last.GitCommit,
		}
		if last.GitSummary != nil {
			lastData["git_summary"] = last.GitSummary
			lastData["git_summary_text"] = domain.SessionGitLine(last)
		}
		result["last_session"] = lastData
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

// printStatusGit prints what changed in git: so far in the active work
// session, and in the last ended work session. Nothing is printed without git.
func printStatusGit(ctx context.Context, state *domain.CurrentState) {
	if active := state.ActiveSession; active != nil {
		if live := app.pomodoro.LiveGitSummary(ctx, active); !live.IsEmpty() {
			fmt.Printf("\n%s\n", i18n.T("Git so far: %s", live.String()))
		}
	}
	if last := app.pomodoro.LastEndedWorkSession(ctx); last != nil {
		if line := domain.SessionGitLine(last); line != "" {
			fmt.Printf("\n%s\n", i18n.T("Last session: %s", line))
		}
	}
}

package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/adapters/tui"
	"github.com/xvierd/flow-cli/internal/domain"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current status",
	Long:  `Display the current pomodoro session status and today's statistics.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		state, err := stateService.GetCurrentState(ctx)
		if err != nil {
			return fmt.Errorf("failed to get current state: %w", err)
		}

		if jsonOutput {
			return outputStatusJSON(state)
		}

		// Use the TUI to display the status
		tui.ShowStatus(state, &appConfig.Theme)

		return nil
	},
}

// outputStatusJSON outputs the status in JSON format
func outputStatusJSON(state *domain.CurrentState) error {
	result := map[string]interface{}{
		"active_task":    nil,
		"active_session": nil,
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

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal status: %w", err)
	}
	fmt.Println(string(jsonData))
	return nil
}

// printStatusText prints the status in plain text format
func printStatusText(state *domain.CurrentState) {
	if state.ActiveSession != nil {
		session := state.ActiveSession
		fmt.Println("🍅 Active Pomodoro Session")
		fmt.Printf("   Status: %s (%s)\n", domain.GetStatusLabel(session.Status), domain.GetSessionTypeLabel(session.Type))
		fmt.Printf("   Remaining: %s\n", session.RemainingTime())
		fmt.Printf("   Progress: %.0f%%\n", session.Progress()*100)
		if session.GitBranch != "" {
			fmt.Printf("   Git: %s (%s)\n", session.GitBranch, session.GitCommit[:7])
		}
	} else {
		fmt.Println("No active pomodoro session.")
	}

	if state.ActiveTask != nil {
		fmt.Printf("\n📋 Active Task: %s\n", state.ActiveTask.Title)
	}

	fmt.Printf("\n📊 Today's Stats:\n")
	fmt.Printf("   Work Sessions: %d\n", state.TodayStats.WorkSessions)
	fmt.Printf("   Breaks Taken: %d\n", state.TodayStats.BreaksTaken)
	fmt.Printf("   Total Work Time: %s\n", state.TodayStats.TotalWorkTime)
}

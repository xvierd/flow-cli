package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/xavier/flow/internal/adapters/tui"
	"github.com/xavier/flow/internal/domain"
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

		// Use the TUI to display the status
		tui.ShowStatus(state)

		return nil
	},
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

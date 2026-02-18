package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/services"
)

var startTaskID string
var startTags string

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start [task-id]",
	Short: "Start a pomodoro session",
	Long: `Start a new pomodoro work session. Optionally specify a task ID
to associate with the session. If no task ID is provided and there is
an active task, that task will be used.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Get current working directory for git context
		workingDir, _ := os.Getwd()

		// Determine task ID
		var taskID *string
		if startTaskID != "" {
			taskID = &startTaskID
		} else if len(args) > 0 {
			taskID = &args[0]
		}

		// Check for active session and prompt user
		state, err := app.state.GetCurrentState(ctx)
		if err != nil {
			return fmt.Errorf("failed to get current state: %w", err)
		}

		if state.ActiveSession != nil {
			active := state.ActiveSession
			remaining := active.RemainingTime()
			sessionType := domain.GetSessionTypeLabel(active.Type)
			sessionInfo := fmt.Sprintf("%s session (%s remaining)", sessionType, formatCmdDuration(remaining))

			if state.ActiveTask != nil {
				sessionInfo = fmt.Sprintf("%s for task \"%s\" (%s remaining)", sessionType, state.ActiveTask.Title, formatCmdDuration(remaining))
			}

			fmt.Printf("⚠️  A %s is already running: %s\n", strings.ToLower(sessionType), sessionInfo)
			fmt.Printf("   Session ID: %s\n", active.ID[:8])
			fmt.Print("Do you want to stop it and start a new one? [y/N] ")

			var answer string
			_, _ = fmt.Scanln(&answer)
			answer = strings.TrimSpace(strings.ToLower(answer))

			if answer != "y" && answer != "yes" {
				fmt.Println("Keeping current session.")
				return nil
			}

			_, err := app.pomodoro.StopSession(ctx)
			if err != nil {
				return fmt.Errorf("failed to stop current session: %w", err)
			}
			fmt.Println("⏹️  Previous session stopped.")
		}

		// Parse tags
		var tags []string
		if startTags != "" {
			for _, t := range strings.Split(startTags, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					tags = append(tags, t)
				}
			}
		}

		// Start the pomodoro session
		req := services.StartPomodoroRequest{
			TaskID:     taskID,
			WorkingDir: workingDir,
			Tags:       tags,
		}

		session, err := app.pomodoro.StartPomodoro(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to start pomodoro: %w", err)
		}

		fmt.Printf("🍅 Pomodoro started! Duration: %s\n", session.Duration)
		if taskID != nil {
			fmt.Printf("   Task ID: %s\n", *taskID)
		}

		// Refresh state and launch TUI
		state, err = app.state.GetCurrentState(ctx)
		if err != nil {
			return fmt.Errorf("failed to get current state: %w", err)
		}

		return launchTUI(ctx, state, workingDir)
	},
}

func init() {
	startCmd.Flags().StringVarP(&startTaskID, "task", "t", "", "Task ID to associate with this session")
	startCmd.Flags().StringVar(&startTags, "tags", "", "Comma-separated tags for this session (e.g. coding,backend)")
}

func formatCmdDuration(d time.Duration) string {
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}

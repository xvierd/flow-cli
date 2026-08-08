package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
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
			return fmt.Errorf("%s: %w", i18n.T("failed to get current state"), err)
		}

		if state.ActiveSession != nil {
			active := state.ActiveSession
			remaining := active.RemainingTime()
			sessionType := domain.GetSessionTypeLabel(active.Type)
			sessionInfo := i18n.T("%s session (%s remaining)", sessionType, formatCmdDuration(remaining))

			if state.ActiveTask != nil {
				sessionInfo = i18n.T("%s for task \"%s\" (%s remaining)", sessionType, state.ActiveTask.Title, formatCmdDuration(remaining))
			}

			fmt.Printf("%s\n", i18n.T("⚠️  A %s is already running: %s", strings.ToLower(sessionType), sessionInfo))
			fmt.Printf("%s\n", i18n.T("   Session ID: %s", active.ID[:8]))
			fmt.Print(i18n.T("Do you want to stop it and start a new one? [y/N] "))

			var answer string
			_, _ = fmt.Scanln(&answer)
			answer = strings.TrimSpace(strings.ToLower(answer))

			if answer != "y" && answer != "yes" {
				fmt.Println(i18n.T("Keeping current session."))
				return nil
			}

			_, err := app.pomodoro.StopSession(ctx)
			if err != nil {
				return fmt.Errorf("%s: %w", i18n.T("failed to stop current session"), err)
			}
			fmt.Println(i18n.T("⏹️  Previous session stopped."))
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
			return fmt.Errorf("%s: %w", i18n.T("failed to start pomodoro"), err)
		}

		fmt.Printf("🍅 %s\n", i18n.T("Pomodoro started! Duration: %s", session.Duration))
		if taskID != nil {
			fmt.Printf("%s\n", i18n.T("   Task ID: %s", *taskID))
		}

		// Refresh state and launch TUI
		state, err = app.state.GetCurrentState(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to get current state"), err)
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

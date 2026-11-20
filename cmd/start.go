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
var startTags []string

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start [task-id]",
	Short: "Start a pomodoro session",
	Long: `Start a new pomodoro work session. Optionally specify a task ID
to associate with the session (as an argument or with --task). The session
uses the current methodology: the configured work duration in Pomodoro, or
the first preset in Deep Work and Make Time.`,
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

			if _, err := app.pomodoro.StopSession(ctx); err != nil {
				return fmt.Errorf("%s: %w", i18n.T("failed to stop current session"), err)
			}
			fmt.Println(i18n.T("⏹️  Previous session stopped."))
		}

		// Parse tags
		var tags []string
		for _, t := range startTags {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}

		// Start the pomodoro session
		// Record the methodology on the session: storage would otherwise
		// default it to Pomodoro, and resuming adopts the session's one.
		req := services.StartPomodoroRequest{
			TaskID:      taskID,
			WorkingDir:  workingDir,
			Tags:        tags,
			Methodology: app.methodology,
			Duration:    defaultSessionDuration(),
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

		return runSessions(state, workingDir)
	},
}

func init() {
	startCmd.Flags().StringVarP(&startTaskID, "task", "t", "", "Task ID to associate with this session")
	startCmd.Flags().StringSliceVar(&startTags, "tags", nil, "Tags for this session (repeatable or comma-separated, e.g. coding,backend)")
}

func formatCmdDuration(d time.Duration) string {
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}

// defaultSessionDuration returns the work duration "flow start" uses for the
// current methodology: zero in Pomodoro (the service applies the configured
// work duration) and the first preset in Deep Work and Make Time, matching
// what the wizard preselects and what MCP sessions default to.
func defaultSessionDuration() time.Duration {
	if app.methodology == domain.MethodologyPomodoro || app.mode == nil {
		return 0
	}
	if presets := app.mode.Presets(); len(presets) > 0 {
		return presets[0].Duration
	}
	return 0
}

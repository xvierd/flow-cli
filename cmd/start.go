package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/adapters/tui"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
	"github.com/xvierd/flow-cli/internal/services"
)

var startTaskID string

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
		state, err := stateService.GetCurrentState(ctx)
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

			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))

			if answer != "y" && answer != "yes" {
				fmt.Println("Keeping current session.")
				return nil
			}

			_, err := pomodoroSvc.StopSession(ctx)
			if err != nil {
				return fmt.Errorf("failed to stop current session: %w", err)
			}
			fmt.Println("⏹️  Previous session stopped.")
		}

		// Start the pomodoro session
		req := services.StartPomodoroRequest{
			TaskID:     taskID,
			WorkingDir: workingDir,
		}

		session, err := pomodoroSvc.StartPomodoro(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to start pomodoro: %w", err)
		}

		fmt.Printf("🍅 Pomodoro started! Duration: %s\n", session.Duration)
		if taskID != nil {
			fmt.Printf("   Task ID: %s\n", *taskID)
		}

		// Refresh state for the TUI
		state, err = stateService.GetCurrentState(ctx)
		if err != nil {
			return fmt.Errorf("failed to get current state: %w", err)
		}

		// Run the TUI timer
		ctx = setupSignalHandler()
		timer := tui.NewTimer()
		
		// Set up update callback to refresh state
		timer.SetUpdateCallback(func() {
			newState, _ := stateService.GetCurrentState(ctx)
			if newState != nil {
				timer.UpdateState(newState)
			}
		})

		// Set up command callback to handle timer commands
		timer.SetCommandCallback(func(cmd ports.TimerCommand) {
			switch cmd {
			case ports.CmdPause:
				_, _ = pomodoroSvc.PauseSession(ctx)
			case ports.CmdResume:
				_, _ = pomodoroSvc.ResumeSession(ctx)
			case ports.CmdCancel:
				_ = pomodoroSvc.CancelSession(ctx)
			case ports.CmdBreak:
				_, _ = pomodoroSvc.StartBreak(ctx, workingDir)
			}
		})

		if err := timer.Run(ctx, state); err != nil {
			return fmt.Errorf("timer error: %w", err)
		}

		return nil
	},
}

func init() {
	startCmd.Flags().StringVarP(&startTaskID, "task", "t", "", "Task ID to associate with this session")
}

func formatCmdDuration(d time.Duration) string {
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}

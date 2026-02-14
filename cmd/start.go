package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/dvidx/flow-cli/internal/adapters/tui"
	"github.com/dvidx/flow-cli/internal/ports"
	"github.com/dvidx/flow-cli/internal/services"
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

		// Get the current state for the TUI
		state, err := stateService.GetCurrentState(ctx)
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

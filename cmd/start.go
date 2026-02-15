package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/dvidx/flow-cli/internal/adapters/tui"
	"github.com/dvidx/flow-cli/internal/ports"
	"github.com/dvidx/flow-cli/internal/services"
	"github.com/spf13/cobra"
)

var startTaskID string
var noTUI bool

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start [task-id]",
	Short: "Start a pomodoro session",
	Long: `Start a new pomodoro work session. Optionally specify a task ID
to associate with the session. If no task ID is provided and there is
an active task, that task will be used.

Use --no-tui to run without the interactive TUI (useful for scripts or when no terminal is available).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Check if we should run without TUI
		if noTUI || os.Getenv("FLOW_NO_TUI") != "" {
			return runHeadless(ctx, args)
		}

		// Try to run with TUI, fallback to headless if no TTY
		if isTTY() {
			return runWithTUI(ctx, args)
		}

		// No TTY available, run headless
		fmt.Println("Note: Running in headless mode (no TTY detected). Use 'flow status' to check progress.")
		return runHeadless(ctx, args)
	},
}

// isTTY checks if stdin is a terminal
func isTTY() bool {
	fileInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fileInfo.Mode()&os.ModeCharDevice != 0
}

// runHeadless runs the pomodoro without TUI
func runHeadless(ctx context.Context, args []string) error {
	workingDir, _ := os.Getwd()

	var taskID *string
	if startTaskID != "" {
		taskID = &startTaskID
	} else if len(args) > 0 {
		taskID = &args[0]
	}

	req := services.StartPomodoroRequest{
		TaskID:     taskID,
		WorkingDir: workingDir,
	}

	session, err := pomodoroSvc.StartPomodoro(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to start pomodoro: %w", err)
	}

	// Print success message
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF6B6B"))
	fmt.Println(titleStyle.Render("🍅 Pomodoro started in background mode!"))
	fmt.Printf("   Duration: %s\n", session.Duration)
	if taskID != nil {
		fmt.Printf("   Task ID: %s\n", *taskID)
	}
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  flow status     - Check session status")
	fmt.Println("  flow stop       - Complete the session")
	fmt.Println("  flow break      - Start a break")
	fmt.Println()
	fmt.Println("Note: The session is running in the background.")
	fmt.Println("      Use 'flow status --watch' to monitor (coming soon).")

	return nil
}

// runWithTUI runs the pomodoro with interactive TUI
func runWithTUI(ctx context.Context, args []string) error {
	workingDir, _ := os.Getwd()

	var taskID *string
	if startTaskID != "" {
		taskID = &startTaskID
	} else if len(args) > 0 {
		taskID = &args[0]
	}

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

	state, err := stateService.GetCurrentState(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current state: %w", err)
	}

	ctx = setupSignalHandler()
	timer := tui.NewTimer()

	timer.SetUpdateCallback(func() {
		newState, err := stateService.GetCurrentState(ctx)
		if err != nil {
			return
		}
		if newState != nil {
			timer.UpdateState(newState)
		}
	})

	timer.SetCommandCallback(func(cmd ports.TimerCommand) error {
		switch cmd {
		case ports.CmdPause:
			_, err := pomodoroSvc.PauseSession(ctx)
			if err != nil {
				return fmt.Errorf("failed to pause session: %w", err)
			}
		case ports.CmdResume:
			_, err := pomodoroSvc.ResumeSession(ctx)
			if err != nil {
				return fmt.Errorf("failed to resume session: %w", err)
			}
		case ports.CmdCancel:
			err := pomodoroSvc.CancelSession(ctx)
			if err != nil {
				return fmt.Errorf("failed to cancel session: %w", err)
			}
		case ports.CmdBreak:
			_, err := pomodoroSvc.StartBreak(ctx, workingDir)
			if err != nil {
				return fmt.Errorf("failed to start break: %w", err)
			}
		}
		return nil
	})

	if err := timer.Run(ctx, state); err != nil {
		return fmt.Errorf("timer error: %w", err)
	}

	return nil
}

func init() {
	startCmd.Flags().StringVarP(&startTaskID, "task", "t", "", "Task ID to associate with this session")
	startCmd.Flags().BoolVar(&noTUI, "no-tui", false, "Run without interactive TUI (headless mode)")
}

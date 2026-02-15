package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/adapters/tui"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

// breakCmd represents the break command
var breakCmd = &cobra.Command{
	Use:   "break",
	Short: "Start a break session",
	Long:  `Start a pomodoro break session (short or long depending on work completed).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		workingDir, _ := os.Getwd()

		// Start a break session
		session, err := pomodoroSvc.StartBreak(ctx, workingDir)
		if err != nil {
			return fmt.Errorf("failed to start break: %w", err)
		}

		fmt.Printf("☕ Break started! Duration: %s (%s)\n", 
			session.Duration, 
			getBreakTypeLabel(session.Type))

		// Get the current state for the TUI
		state, err := stateService.GetCurrentState(ctx)
		if err != nil {
			return fmt.Errorf("failed to get current state: %w", err)
		}

		// Run the TUI timer
		ctx = setupSignalHandler()
		timer := tui.NewTimer()
		
		timer.SetFetchState(func() *domain.CurrentState {
			newState, _ := stateService.GetCurrentState(ctx)
			return newState
		})

		timer.SetCommandCallback(func(cmd ports.TimerCommand) {
			switch cmd {
			case ports.CmdPause:
				_, _ = pomodoroSvc.PauseSession(ctx)
			case ports.CmdResume:
				_, _ = pomodoroSvc.ResumeSession(ctx)
			case ports.CmdStop:
				_, _ = pomodoroSvc.StopSession(ctx)
			case ports.CmdCancel:
				_ = pomodoroSvc.CancelSession(ctx)
			}
		})

		if err := timer.Run(ctx, state); err != nil {
			return fmt.Errorf("timer error: %w", err)
		}

		return nil
	},
}

func getBreakTypeLabel(sessionType domain.SessionType) string {
	switch sessionType {
	case domain.SessionTypeShortBreak:
		return "Short Break"
	case domain.SessionTypeLongBreak:
		return "Long Break"
	default:
		return "Break"
	}
}

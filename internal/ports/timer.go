package ports

import (
	"context"
	"time"

	"github.com/xavier/flow/internal/domain"
)

// TimerView defines the interface for timer display updates.
// This is a driving port (called by the application layer).
type TimerView interface {
	// UpdateTimer displays the current timer state.
	UpdateTimer(remaining time.Duration, progress float64)

	// UpdateSessionInfo displays session information.
	UpdateSessionInfo(session *domain.PomodoroSession)

	// UpdateTaskInfo displays task information.
	UpdateTaskInfo(task *domain.Task)

	// ShowNotification displays a notification to the user.
	ShowNotification(message string)

	// OnSessionComplete is called when a session finishes.
	OnSessionComplete(session *domain.PomodoroSession)
}

// TimerInput defines the interface for timer user input.
// This is a driving port (called by the application layer).
type TimerInput interface {
	// WaitForCommand blocks until the user issues a command.
	WaitForCommand() (TimerCommand, error)
}

// TimerCommand represents a user action during timer operation.
type TimerCommand string

const (
	// CmdStart starts or resumes the timer.
	CmdStart TimerCommand = "start"

	// CmdPause pauses the timer.
	CmdPause TimerCommand = "pause"

	// CmdResume resumes a paused timer.
	CmdResume TimerCommand = "resume"

	// CmdCancel cancels the current session.
	CmdCancel TimerCommand = "cancel"

	// CmdBreak starts a break session.
	CmdBreak TimerCommand = "break"

	// CmdQuit exits the application.
	CmdQuit TimerCommand = "quit"
)

// Timer is the combined interface for TUI timer operations.
// This is a driving port (called by the application layer).
type Timer interface {
	// Run starts the timer interface and blocks until completion.
	Run(ctx context.Context, initialState *domain.CurrentState) error

	// Stop gracefully stops the timer interface.
	Stop()

	// SetUpdateCallback sets a function to call on timer updates.
	SetUpdateCallback(callback func())

	// SetCommandCallback sets a function to call when commands are received.
	SetCommandCallback(callback func(cmd TimerCommand))
}

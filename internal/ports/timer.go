package ports

import (
	"context"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
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

	// CmdStop completes the current session.
	CmdStop TimerCommand = "stop"

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

	// SetFetchState sets a function that returns the current application state.
	// This is called asynchronously on each tick to refresh the TUI.
	SetFetchState(fetch func() *domain.CurrentState)

	// SetCommandCallback sets a function to call when commands are received.
	SetCommandCallback(callback func(cmd TimerCommand))

	// SetCommandCallbackWithError sets a function to call when commands are received that can return errors.
	SetCommandCallbackWithError(callback func(cmd TimerCommand) error)

	// SetOnSessionComplete sets a callback fired when a session naturally completes.
	SetOnSessionComplete(callback func(domain.SessionType))

	// SetCompletionInfo sets pre-computed break context for the completion screen.
	SetCompletionInfo(info *domain.CompletionInfo)

	// UpdateState updates the displayed state.
	UpdateState(state *domain.CurrentState)
}

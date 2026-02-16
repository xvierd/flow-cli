package tui

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

// Timer implements the ports.Timer interface using Bubbletea.
type Timer struct {
	program            *tea.Program
	fetchState         func() *domain.CurrentState
	commandCallback    func(ports.TimerCommand) error
	onSessionComplete  func(domain.SessionType)
	completionInfo     *domain.CompletionInfo
}

// NewTimer creates a new TUI timer adapter.
func NewTimer() ports.Timer {
	return &Timer{}
}

// Run starts the timer interface and blocks until completion.
func (t *Timer) Run(ctx context.Context, initialState *domain.CurrentState) error {
	model := NewModel(initialState, t.completionInfo)
	model.fetchState = t.fetchState
	model.commandCallback = t.commandCallback
	model.onSessionComplete = t.onSessionComplete

	t.program = tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	// Handle context cancellation
	go func() {
		<-ctx.Done()
		if t.program != nil {
			t.program.Quit()
		}
	}()

	_, err := t.program.Run()
	if err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}

	return nil
}

// Stop gracefully stops the timer interface.
func (t *Timer) Stop() {
	if t.program != nil {
		t.program.Quit()
	}
}

// SetFetchState sets a function that returns the current state.
func (t *Timer) SetFetchState(fetch func() *domain.CurrentState) {
	t.fetchState = fetch
}

// SetCommandCallback sets a function to call when commands are received.
func (t *Timer) SetCommandCallback(callback func(cmd ports.TimerCommand) error) {
	t.commandCallback = callback
}

// SetOnSessionComplete sets a callback fired when a session naturally completes.
func (t *Timer) SetOnSessionComplete(callback func(domain.SessionType)) {
	t.onSessionComplete = callback
}

// SetCompletionInfo sets pre-computed break context for the completion screen.
func (t *Timer) SetCompletionInfo(info *domain.CompletionInfo) {
	t.completionInfo = info
}

// SendCommand sends a command to the timer (for testing or automation).
func (t *Timer) SendCommand(cmd ports.TimerCommand) {
	if t.commandCallback != nil {
		_ = t.commandCallback(cmd)
	}
}

// UpdateState updates the displayed state.
func (t *Timer) UpdateState(state *domain.CurrentState) {
	if t.program != nil {
		t.program.Send(state)
	}
}

// Ensure Timer implements ports.Timer.
var _ ports.Timer = (*Timer)(nil)

// RunTimer is a convenience function to run the timer directly.
func RunTimer(ctx context.Context, state *domain.CurrentState) error {
	timer := NewTimer()
	return timer.Run(ctx, state)
}

// ShowStatus displays the current status without starting interactive mode.
func ShowStatus(state *domain.CurrentState) {
	model := NewModel(state, nil)
	fmt.Println(model.View())
}

// ShowError displays an error message.
func ShowError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
}

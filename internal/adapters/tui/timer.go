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
	program         *tea.Program
	updateCallback  func()
	commandCallback func(ports.TimerCommand)
}

// NewTimer creates a new TUI timer adapter.
func NewTimer() ports.Timer {
	return &Timer{}
}

// Run starts the timer interface and blocks until completion.
func (t *Timer) Run(ctx context.Context, initialState *domain.CurrentState) error {
	model := NewModel(initialState)
	model.updateCallback = t.updateCallback
	model.commandCallback = t.commandCallback

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

// SetUpdateCallback sets a function to call on timer updates.
func (t *Timer) SetUpdateCallback(callback func()) {
	t.updateCallback = callback
}

// SetCommandCallback sets a function to call when commands are received.
func (t *Timer) SetCommandCallback(callback func(cmd ports.TimerCommand)) {
	t.commandCallback = callback
}

// SendCommand sends a command to the timer (for testing or automation).
func (t *Timer) SendCommand(cmd ports.TimerCommand) {
	if t.commandCallback != nil {
		t.commandCallback(cmd)
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
	model := NewModel(state)
	fmt.Println(model.View())
}

// ShowError displays an error message.
func ShowError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
}

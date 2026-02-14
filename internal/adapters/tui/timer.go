package tui

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/xavier/flow/internal/domain"
	"github.com/xavier/flow/internal/ports"
)

// Timer implements the ports.Timer interface using Bubbletea.
type Timer struct {
	program *tea.Program
	model   Model
	cmdChan chan ports.TimerCommand
}

// NewTimer creates a new TUI timer adapter.
func NewTimer() ports.Timer {
	return &Timer{
		cmdChan: make(chan ports.TimerCommand, 10),
	}
}

// Run starts the timer interface and blocks until completion.
func (t *Timer) Run(ctx context.Context, initialState *domain.CurrentState) error {
	t.model = NewModel(initialState)
	t.model.SetUpdateCallback(func() {
		if t.model.updateCallback != nil {
			t.model.updateCallback()
		}
	})

	t.program = tea.NewProgram(
		t.model,
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
	t.model.SetUpdateCallback(callback)
}

// SetCommandCallback sets a function to call when commands are received.
func (t *Timer) SetCommandCallback(callback func(cmd ports.TimerCommand)) {
	go func() {
		for cmd := range t.cmdChan {
			callback(cmd)
		}
	}()
}

// SendCommand sends a command to the timer (for testing or automation).
func (t *Timer) SendCommand(cmd ports.TimerCommand) {
	t.cmdChan <- cmd
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

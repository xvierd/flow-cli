package tui

import (
	"context"
	"fmt"
	"os"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dvidx/flow-cli/internal/domain"
	"github.com/dvidx/flow-cli/internal/ports"
)

// Timer implements the ports.Timer interface using Bubbletea.
type Timer struct {
	program       *tea.Program
	model         Model
	cmdChan       chan ports.TimerCommand
	ctx           context.Context
	cancel        context.CancelFunc
	mu            sync.RWMutex
	wg            sync.WaitGroup
	errCallback   func(error)
	cmdCallback   func(cmd ports.TimerCommand) error
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
		t.mu.RLock()
		callback := t.model.updateCallback
		t.mu.RUnlock()
		if callback != nil {
			callback()
		}
	})

	t.mu.Lock()
	t.program = tea.NewProgram(
		t.model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	t.mu.Unlock()

	// Create a cancellable context for this timer instance
	t.ctx, t.cancel = context.WithCancel(ctx)
	defer t.cancel()

	// Start command callback goroutine if set
	t.mu.RLock()
	callback := t.cmdCallback
	t.mu.RUnlock()
	if callback != nil {
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			for {
				select {
				case <-t.ctx.Done():
					return
				case cmd, ok := <-t.cmdChan:
					if !ok {
						return
					}
					if err := callback(cmd); err != nil {
						// Store the error to be displayed
						t.mu.Lock()
						// Check if model and state are initialized before accessing
						if t.model.state != nil {
							t.model.lastError = err
						}
						t.mu.Unlock()
					}
				}
			}
		}()
	}

	// Handle context cancellation
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		select {
		case <-t.ctx.Done():
			t.mu.RLock()
			program := t.program
			t.mu.RUnlock()
			if program != nil {
				program.Quit()
			}
		}
	}()

	_, err := t.program.Run()
	if err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}

	// Signal cancellation and wait for goroutines
	t.cancel()
	t.wg.Wait()

	return nil
}

// Stop gracefully stops the timer interface.
func (t *Timer) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cancel != nil {
		t.cancel()
	}
	if t.program != nil {
		t.program.Quit()
	}
}

// SetUpdateCallback sets a function to call on timer updates.
func (t *Timer) SetUpdateCallback(callback func()) {
	t.model.SetUpdateCallback(callback)
}

// SetCommandCallback sets a function to call when commands are received.
// The callback should return an error if the command fails.
// Note: The goroutine is started in Run() after the context is initialized.
func (t *Timer) SetCommandCallback(callback func(cmd ports.TimerCommand) error) {
	t.mu.Lock()
	t.cmdCallback = callback
	t.mu.Unlock()
}

// SetErrorCallback sets a function to call when an error occurs.
func (t *Timer) SetErrorCallback(callback func(error)) {
	t.errCallback = callback
}

// SendCommand sends a command to the timer (for testing or automation).
func (t *Timer) SendCommand(cmd ports.TimerCommand) {
	select {
	case <-t.ctx.Done():
		return
	case t.cmdChan <- cmd:
	}
}

// UpdateState updates the displayed state.
func (t *Timer) UpdateState(state *domain.CurrentState) {
	t.mu.RLock()
	program := t.program
	t.mu.RUnlock()

	if program != nil {
		program.Send(state)
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
func ShowStatus(state *domain.CurrentState) error {
	if state.ActiveSession != nil {
		session := state.ActiveSession
		fmt.Println("🍅 Active Pomodoro Session")
		fmt.Printf("   Status: %s (%s)\n", domain.GetStatusLabel(session.Status), domain.GetSessionTypeLabel(session.Type))
		fmt.Printf("   Remaining: %s\n", session.RemainingTime())
		fmt.Printf("   Progress: %.0f%%\n", session.Progress()*100)
		if session.GitBranch != "" {
			fmt.Printf("   Git: %s (%s)\n", session.GitBranch, session.GitCommit[:7])
		}
		if session.Notes != "" {
			fmt.Printf("   Notes: %s\n", session.Notes)
		}
	} else {
		fmt.Println("No active pomodoro session.")
	}

	if state.ActiveTask != nil {
		fmt.Printf("\n📋 Active Task: %s\n", state.ActiveTask.Title)
		if state.ActiveTask.Description != "" {
			fmt.Printf("   Description: %s\n", state.ActiveTask.Description)
		}
		if len(state.ActiveTask.Tags) > 0 {
			fmt.Printf("   Tags: %v\n", state.ActiveTask.Tags)
		}
	}

	fmt.Printf("\n📊 Today's Stats:\n")
	fmt.Printf("   Work Sessions: %d\n", state.TodayStats.WorkSessions)
	fmt.Printf("   Breaks Taken: %d\n", state.TodayStats.BreaksTaken)
	fmt.Printf("   Total Work Time: %s\n", state.TodayStats.TotalWorkTime)

	return nil
}

// ShowError displays an error message.
func ShowError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
}

package tui

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/methodology"
	"github.com/xvierd/flow-cli/internal/ports"
)

// Timer implements the ports.Timer interface using Bubbletea.
type Timer struct {
	program                *tea.Program
	fetchState             func() *domain.CurrentState
	commandCallback        func(ports.TimerCommand) error
	onSessionComplete      func(domain.SessionType)
	distractionCallback    func(string) error
	accomplishmentCallback func(string) error
	focusScoreCallback     func(int) error
	energizeCallback       func(string) error
	completionInfo         *domain.CompletionInfo
	theme                  *config.ThemeConfig
	inline                 bool
	presets                []config.SessionPreset
	breakInfo              string
	onStartSession         func(presetIndex int, taskName string) error
	mode                   methodology.Mode
	modeLocked             bool
	onModeSelected         func(domain.Methodology)
	fetchRecentTasks       func(limit int) []*domain.Task
	fetchYesterdayHighlight func() *domain.Task
	autoBreak              bool
	// PostAction holds the action selected from the main menu (stats/reflect).
	PostAction             MainMenuAction
}

// NewTimer creates a new TUI timer adapter.
func NewTimer(theme *config.ThemeConfig) ports.Timer {
	return &Timer{theme: theme}
}

// NewInlineTimer creates a TUI timer that renders in-place without alt screen.
func NewInlineTimer(theme *config.ThemeConfig) *Timer {
	return &Timer{theme: theme, inline: true}
}

// SetMode sets the methodology mode for mode-aware UI behavior.
func (t *Timer) SetMode(mode methodology.Mode) {
	t.mode = mode
}

// SetModeLocked prevents the mode selection menu from appearing.
func (t *Timer) SetModeLocked(locked bool) {
	t.modeLocked = locked
}

// SetOnModeSelected sets a callback for when the user picks a methodology.
func (t *Timer) SetOnModeSelected(callback func(domain.Methodology)) {
	t.onModeSelected = callback
}

// Run starts the timer interface and blocks until completion.
func (t *Timer) Run(ctx context.Context, initialState *domain.CurrentState) error {
	if t.inline {
		return t.runInline(ctx, initialState)
	}

	model := NewModel(initialState, t.completionInfo, t.theme)
	model.fetchState = t.fetchState
	model.commandCallback = t.commandCallback
	model.onSessionComplete = t.onSessionComplete
	model.distractionCallback = t.distractionCallback
	model.accomplishmentCallback = t.accomplishmentCallback
	model.focusScoreCallback = t.focusScoreCallback
	model.energizeCallback = t.energizeCallback
	model.mode = t.mode
	model.autoBreak = t.autoBreak

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

func (t *Timer) runInline(ctx context.Context, initialState *domain.CurrentState) error {
	model := NewInlineModel(initialState, t.completionInfo, t.theme)
	model.fetchState = t.fetchState
	model.commandCallback = t.commandCallback
	model.onSessionComplete = t.onSessionComplete
	model.distractionCallback = t.distractionCallback
	model.accomplishmentCallback = t.accomplishmentCallback
	model.focusScoreCallback = t.focusScoreCallback
	model.energizeCallback = t.energizeCallback
	model.presets = t.presets
	model.breakInfo = t.breakInfo
	model.onStartSession = t.onStartSession
	model.mode = t.mode
	model.modeLocked = t.modeLocked
	model.onModeSelected = t.onModeSelected
	model.fetchRecentTasks = t.fetchRecentTasks
	model.fetchYesterdayHighlight = t.fetchYesterdayHighlight
	model.autoBreak = t.autoBreak

	// If no active session, start at main menu or mode picker
	if initialState.ActiveSession == nil {
		if !t.modeLocked {
			model.phase = phaseMainMenu
			// Pre-select current mode for when they reach mode picker
			for i, m := range domain.ValidMethodologies {
				if t.mode != nil && m == t.mode.Name() {
					model.modeCursor = i
					break
				}
			}
		} else {
			// --mode was passed, skip main menu and mode picker
			model.phase = phasePickDuration
		}
	}

	t.program = tea.NewProgram(model)

	go func() {
		<-ctx.Done()
		if t.program != nil {
			t.program.Quit()
		}
	}()

	result, err := t.program.Run()
	if err != nil {
		return fmt.Errorf("failed to run inline TUI: %w", err)
	}
	if final, ok := result.(InlineModel); ok {
		t.PostAction = final.SelectedAction
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

// SetDistractionCallback sets a callback for logging distractions (Deep Work mode).
func (t *Timer) SetDistractionCallback(callback func(text string) error) {
	t.distractionCallback = callback
}

// SetAccomplishmentCallback sets a callback for recording accomplishments (Deep Work shutdown ritual).
func (t *Timer) SetAccomplishmentCallback(callback func(text string) error) {
	t.accomplishmentCallback = callback
}

// SetFocusScoreCallback sets a callback for recording focus scores (Make Time).
func (t *Timer) SetFocusScoreCallback(callback func(score int) error) {
	t.focusScoreCallback = callback
}

// SetAutoBreak enables auto-starting breaks when a work session completes.
func (t *Timer) SetAutoBreak(enabled bool) {
	t.autoBreak = enabled
}

// SetEnergizeCallback sets a callback for recording energize activities (Make Time).
func (t *Timer) SetEnergizeCallback(callback func(activity string) error) {
	t.energizeCallback = callback
}

// SetFetchRecentTasks sets a callback to fetch recent tasks for the task select phase.
func (t *Timer) SetFetchRecentTasks(fetch func(limit int) []*domain.Task) {
	t.fetchRecentTasks = fetch
}

// SetFetchYesterdayHighlight sets a callback to fetch yesterday's unfinished highlight.
func (t *Timer) SetFetchYesterdayHighlight(fetch func() *domain.Task) {
	t.fetchYesterdayHighlight = fetch
}

// SetInlineSetup configures the inline setup phase (presets, break info, start callback).
func (t *Timer) SetInlineSetup(presets []config.SessionPreset, breakInfo string, onStart func(presetIndex int, taskName string) error) {
	t.presets = presets
	t.breakInfo = breakInfo
	t.onStartSession = onStart
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
	timer := NewTimer(nil)
	return timer.Run(ctx, state)
}

// ShowStatus displays the current status without starting interactive mode.
func ShowStatus(state *domain.CurrentState, theme *config.ThemeConfig) {
	model := NewModel(state, nil, theme)
	fmt.Println(model.View())
}

// ShowError displays an error message.
func ShowError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
}

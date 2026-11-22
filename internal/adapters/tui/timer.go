package tui

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/methodology"
	"github.com/xvierd/flow-cli/internal/ports"
)

// Timer implements the ports.Timer interface using Bubbletea.
type Timer struct {
	program                 *tea.Program
	fetchState              func() *domain.CurrentState
	commandCallback         func(ports.TimerCommand) error
	onSessionComplete       func(sessionType domain.SessionType, worked time.Duration)
	fetchCompletionInfo     func(*domain.CurrentState) *domain.CompletionInfo
	fetchGitSummary         func(sessionID string) string
	suggestedTaskTitle      string
	fetchBreakInfo          func() string
	onStartSessionForTask   func(presetIndex int, taskID, taskName, intendedOutcome string) error
	distractionCallback     func(string, string) error
	accomplishmentCallback  func(sessionID string, text string) error
	shutdownRitualCallback  func(sessionID string, ritual domain.ShutdownRitual) error
	focusScoreCallback      func(sessionID string, score int) error
	energizeCallback        func(sessionID string, activity string) error
	outcomeAchievedCallback func(sessionID string, achieved string) error
	completionInfo          *domain.CompletionInfo
	theme                   *config.ThemeConfig
	inline                  bool
	presets                 []config.SessionPreset
	breakInfo               string
	onStartSession          func(presetIndex int, taskName string, intendedOutcome string) error
	mode                    methodology.Mode
	modeLocked              bool
	onModeSelected          func(domain.Methodology) methodology.Mode
	fetchRecentTasks        func(limit int) []*domain.Task
	fetchYesterdayHighlight func() *domain.Task
	autoBreak               bool
	notificationsEnabled    bool
	notificationToggle      func(bool)
	firstRun                bool
	skipModePicker          bool
	lastDuration            func(domain.Methodology) time.Duration
	strict                  bool
	// PostAction holds the action selected from the main menu (stats/reflect).
	PostAction MainMenuAction
	// WantsNewSession is set when user wants to chain another session (fullscreen mode).
	WantsNewSession bool
}

// NewTimer creates a new TUI timer adapter.
func NewTimer(theme *config.ThemeConfig) ports.Timer {
	return &Timer{theme: theme}
}

// NewFullscreenTimer creates a TUI timer that renders in alt screen (fullscreen).
// Returns a concrete *Timer so callers can access WantsNewSession after Run().
func NewFullscreenTimer(theme *config.ThemeConfig) *Timer {
	return &Timer{theme: theme}
}

// NewInlineTimer creates a TUI timer that renders in-place without alt screen.
func NewInlineTimer(theme *config.ThemeConfig) *Timer {
	return &Timer{theme: theme, inline: true}
}

// TimerConfig holds all configuration for a Timer, replacing the individual Set* methods.
type TimerConfig struct {
	FetchState        func() *domain.CurrentState
	CommandCallback   func(ports.TimerCommand) error
	OnSessionComplete func(domain.SessionType)
	// OnSessionCompleted, when set, replaces OnSessionComplete and also
	// receives the actual (pause-aware) time worked in the ended session.
	OnSessionCompleted func(sessionType domain.SessionType, worked time.Duration)
	// FetchCompletionInfo recomputes the next-break / streak context when a
	// session completes, from the freshly fetched state (whose stats already
	// count the completed session). Falls back to the static CompletionInfo.
	FetchCompletionInfo func(state *domain.CurrentState) *domain.CompletionInfo
	// FetchBreakInfo recomputes the inline break-info line after a mode switch.
	FetchBreakInfo func() string
	// FetchGitSummary returns the git summary line ("3 commits · dir/ (+1 −2)")
	// of an ended session, or "" for none. It runs off the update loop.
	FetchGitSummary func(sessionID string) string
	// SuggestedTaskTitle pre-fills the inline setup's new-task name (the
	// title derived from the git branch; "" for none).
	SuggestedTaskTitle string
	// OnStartSessionForTask, when set, replaces OnStartSession and also receives
	// the ID of an existing task picked in the inline setup ("" for a new task).
	OnStartSessionForTask   func(presetIndex int, taskID, taskName, intendedOutcome string) error
	DistractionCallback     func(string, string) error
	AccomplishmentCallback  func(sessionID string, text string) error
	ShutdownRitualCallback  func(sessionID string, ritual domain.ShutdownRitual) error
	FocusScoreCallback      func(sessionID string, score int) error
	EnergizeCallback        func(sessionID string, activity string) error
	OutcomeAchievedCallback func(sessionID string, achieved string) error
	CompletionInfo          *domain.CompletionInfo
	AutoBreak               bool
	NotificationsEnabled    bool
	NotificationToggle      func(bool)
	Presets                 []config.SessionPreset
	BreakInfo               string
	OnStartSession          func(presetIndex int, taskName string, intendedOutcome string) error
	Mode                    methodology.Mode
	ModeLocked              bool
	OnModeSelected          func(domain.Methodology) methodology.Mode
	FetchRecentTasks        func(limit int) []*domain.Task
	FetchYesterdayHighlight func() *domain.Task
	FirstRun                bool
	Strict                  bool
	// SkipModePicker: "Start session" goes straight to the duration picker.
	SkipModePicker bool
	// LastDuration returns the last planned work duration in a methodology
	// (0 if none); the duration picker preselects the matching preset.
	LastDuration func(domain.Methodology) time.Duration
}

// Configure applies a TimerConfig to the timer, replacing individual Set* calls.
func (t *Timer) Configure(cfg TimerConfig) {
	t.fetchState = cfg.FetchState
	t.commandCallback = cfg.CommandCallback
	t.SetOnSessionComplete(cfg.OnSessionComplete)
	if cfg.OnSessionCompleted != nil {
		t.onSessionComplete = cfg.OnSessionCompleted
	}
	t.fetchCompletionInfo = cfg.FetchCompletionInfo
	t.fetchBreakInfo = cfg.FetchBreakInfo
	t.fetchGitSummary = cfg.FetchGitSummary
	t.suggestedTaskTitle = cfg.SuggestedTaskTitle
	t.onStartSessionForTask = cfg.OnStartSessionForTask
	t.distractionCallback = cfg.DistractionCallback
	t.accomplishmentCallback = cfg.AccomplishmentCallback
	t.shutdownRitualCallback = cfg.ShutdownRitualCallback
	t.focusScoreCallback = cfg.FocusScoreCallback
	t.energizeCallback = cfg.EnergizeCallback
	t.outcomeAchievedCallback = cfg.OutcomeAchievedCallback
	t.completionInfo = cfg.CompletionInfo
	t.autoBreak = cfg.AutoBreak
	t.notificationsEnabled = cfg.NotificationsEnabled
	t.notificationToggle = cfg.NotificationToggle
	t.presets = cfg.Presets
	t.breakInfo = cfg.BreakInfo
	t.onStartSession = cfg.OnStartSession
	t.mode = cfg.Mode
	t.modeLocked = cfg.ModeLocked
	t.onModeSelected = cfg.OnModeSelected
	t.fetchRecentTasks = cfg.FetchRecentTasks
	t.fetchYesterdayHighlight = cfg.FetchYesterdayHighlight
	t.firstRun = cfg.FirstRun
	t.strict = cfg.Strict
	t.skipModePicker = cfg.SkipModePicker
	t.lastDuration = cfg.LastDuration
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
// The callback returns the configured Mode for the selection (nil falls back
// to the built-in defaults).
func (t *Timer) SetOnModeSelected(callback func(domain.Methodology) methodology.Mode) {
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
	model.fetchCompletionInfo = t.fetchCompletionInfo
	model.fetchGitSummary = t.fetchGitSummary
	model.distractionCallback = t.distractionCallback
	model.accomplishmentCallback = t.accomplishmentCallback
	model.shutdownRitualCallback = t.shutdownRitualCallback
	model.focusScoreCallback = t.focusScoreCallback
	model.energizeCallback = t.energizeCallback
	model.outcomeAchievedCallback = t.outcomeAchievedCallback
	model.mode = t.mode
	model.autoBreak = t.autoBreak
	model.notificationsEnabled = t.notificationsEnabled
	model.notificationToggle = t.notificationToggle
	model.strict = t.strict

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

	result, err := t.program.Run()
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to run TUI"), err)
	}

	if final, ok := result.(Model); ok {
		t.WantsNewSession = final.WantsNewSession
	}

	return nil
}

func (t *Timer) runInline(ctx context.Context, initialState *domain.CurrentState) error {
	model := NewInlineModel(initialState, t.completionInfo, t.theme)
	model.fetchState = t.fetchState
	model.commandCallback = t.commandCallback
	model.onSessionComplete = t.onSessionComplete
	model.fetchCompletionInfo = t.fetchCompletionInfo
	model.fetchGitSummary = t.fetchGitSummary
	model.distractionCallback = t.distractionCallback
	model.accomplishmentCallback = t.accomplishmentCallback
	model.shutdownRitualCallback = t.shutdownRitualCallback
	model.focusScoreCallback = t.focusScoreCallback
	model.energizeCallback = t.energizeCallback
	model.outcomeAchievedCallback = t.outcomeAchievedCallback
	model.presets = t.presets
	model.breakInfo = t.breakInfo
	model.fetchBreakInfo = t.fetchBreakInfo
	model.onStartSession = t.onStartSession
	model.onStartSessionForTask = t.onStartSessionForTask
	model.mode = t.mode
	model.modeLocked = t.modeLocked
	model.onModeSelected = t.onModeSelected
	model.fetchRecentTasks = t.fetchRecentTasks
	model.fetchYesterdayHighlight = t.fetchYesterdayHighlight
	model.suggestedTaskTitle = t.suggestedTaskTitle
	model.autoBreak = t.autoBreak
	model.notificationsEnabled = t.notificationsEnabled
	model.notificationToggle = t.notificationToggle
	model.strict = t.strict
	model.skipModePicker = t.skipModePicker
	model.lastDuration = t.lastDuration

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
			model = model.enterPickDuration()
		}
	}

	if t.firstRun {
		model.phase = phaseWelcome
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
		return fmt.Errorf("%s: %w", i18n.T("failed to run inline TUI"), err)
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
	if callback == nil {
		t.onSessionComplete = nil
		return
	}
	t.onSessionComplete = func(sessionType domain.SessionType, _ time.Duration) {
		callback(sessionType)
	}
}

// SetDistractionCallback sets a callback for logging distractions (Deep Work mode).
func (t *Timer) SetDistractionCallback(callback func(text string, category string) error) {
	t.distractionCallback = callback
}

// SetAccomplishmentCallback sets a callback for recording accomplishments (Deep Work shutdown ritual).
func (t *Timer) SetAccomplishmentCallback(callback func(sessionID string, text string) error) {
	t.accomplishmentCallback = callback
}

// SetShutdownRitualCallback sets a callback for recording the 4-step shutdown ritual (Deep Work mode).
func (t *Timer) SetShutdownRitualCallback(callback func(sessionID string, ritual domain.ShutdownRitual) error) {
	t.shutdownRitualCallback = callback
}

// SetFocusScoreCallback sets a callback for recording focus scores (Make Time).
func (t *Timer) SetFocusScoreCallback(callback func(sessionID string, score int) error) {
	t.focusScoreCallback = callback
}

// SetAutoBreak enables auto-starting breaks when a work session completes.
func (t *Timer) SetAutoBreak(enabled bool) {
	t.autoBreak = enabled
}

// SetFirstRun enables the first-run welcome screen.
func (t *Timer) SetFirstRun(firstRun bool) {
	t.firstRun = firstRun
}

// SetEnergizeCallback sets a callback for recording energize activities (Make Time).
func (t *Timer) SetEnergizeCallback(callback func(sessionID string, activity string) error) {
	t.energizeCallback = callback
}

// SetNotifications configures the initial notification state and toggle callback.
func (t *Timer) SetNotifications(enabled bool, toggle func(bool)) {
	t.notificationsEnabled = enabled
	t.notificationToggle = toggle
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
func (t *Timer) SetInlineSetup(presets []config.SessionPreset, breakInfo string, onStart func(presetIndex int, taskName string, intendedOutcome string) error) {
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
	fmt.Println(renderStatus(state, theme, getTerminalWidth()))
}

// renderStatus renders the fullscreen view once at the given width. Without a
// size the model only renders "Loading...". Height 0 keeps the output compact
// (no vertical centering padding) for a one-shot print.
func renderStatus(state *domain.CurrentState, theme *config.ThemeConfig, width int) string {
	model := NewModel(state, nil, theme)
	// A one-shot print is not interactive: leave out the key hints.
	model.oneShot = true
	sized, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: 0})
	return sized.View()
}

// ShowError displays an error message.
func ShowError(err error) {
	fmt.Fprintf(os.Stderr, "%s\n", i18n.T("Error: %v", err))
}

// Package tui provides the terminal user interface implementation
// using the Bubbletea framework.
package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/methodology"
	"github.com/xvierd/flow-cli/internal/ports"
)

// resolveTheme fills any empty fields in the given ThemeConfig with defaults.
// If theme is nil, returns the full default theme.
func resolveTheme(theme *config.ThemeConfig) config.ThemeConfig {
	defaults := config.DefaultThemeConfig()
	if theme == nil {
		return defaults
	}
	resolved := *theme
	if resolved.ColorWork == "" {
		resolved.ColorWork = defaults.ColorWork
	}
	if resolved.ColorBreak == "" {
		resolved.ColorBreak = defaults.ColorBreak
	}
	if resolved.ColorPaused == "" {
		resolved.ColorPaused = defaults.ColorPaused
	}
	if resolved.ColorTitle == "" {
		resolved.ColorTitle = defaults.ColorTitle
	}
	if resolved.ColorTask == "" {
		resolved.ColorTask = defaults.ColorTask
	}
	if resolved.ColorHelp == "" {
		resolved.ColorHelp = defaults.ColorHelp
	}
	if resolved.WorkGradientStart == "" {
		resolved.WorkGradientStart = defaults.WorkGradientStart
	}
	if resolved.WorkGradientEnd == "" {
		resolved.WorkGradientEnd = defaults.WorkGradientEnd
	}
	if resolved.BreakGradientStart == "" {
		resolved.BreakGradientStart = defaults.BreakGradientStart
	}
	if resolved.BreakGradientEnd == "" {
		resolved.BreakGradientEnd = defaults.BreakGradientEnd
	}
	if resolved.PausedGradientStart == "" {
		resolved.PausedGradientStart = defaults.PausedGradientStart
	}
	if resolved.PausedGradientEnd == "" {
		resolved.PausedGradientEnd = defaults.PausedGradientEnd
	}
	if resolved.IconApp == "" {
		resolved.IconApp = defaults.IconApp
	}
	if resolved.IconTask == "" {
		resolved.IconTask = defaults.IconTask
	}
	if resolved.IconStats == "" {
		resolved.IconStats = defaults.IconStats
	}
	if resolved.IconGit == "" {
		resolved.IconGit = defaults.IconGit
	}
	if resolved.IconPaused == "" {
		resolved.IconPaused = defaults.IconPaused
	}
	return resolved
}

// tickMsg is sent on every timer tick.
type tickMsg time.Time

// stateMsg wraps an updated state fetched asynchronously.
type stateMsg struct {
	state *domain.CurrentState
}

// Model represents the TUI state.
type Model struct {
	state                  *domain.CurrentState
	progress               progress.Model
	width                  int
	height                 int
	completed              bool
	completedSessionType   domain.SessionType
	completedElapsed       time.Duration // actual time worked, captured at session end
	notified               bool
	confirmBreak           bool
	confirmFinish          bool
	fetchState             func() *domain.CurrentState
	commandCallback        func(ports.TimerCommand) error
	onSessionComplete      func(domain.SessionType)
	distractionCallback    func(string, string) error
	accomplishmentCallback func(string) error
	focusScoreCallback     func(int) error
	energizeCallback       func(string) error
	completionInfo         *domain.CompletionInfo
	theme                  config.ThemeConfig
	mode                   methodology.Mode

	// Deep Work: distraction log
	distractionMode         bool
	distractionInput        textinput.Model
	distractions            []string
	distractionCategoryMode bool
	distractionPendingText  string

	// Deep Work: accomplishment (shutdown ritual)
	accomplishmentMode  bool
	accomplishmentInput textinput.Model
	accomplishmentSaved bool

	// Deep Work: 3-step shutdown ritual
	shutdownRitualMode       bool
	shutdownStep             int // 0=pending tasks, 1=tomorrow plan, 2=closing phrase
	shutdownInputs           [3]textinput.Model
	shutdownComplete         bool
	shutdownRitualCallback   func(domain.ShutdownRitual) error

	// Make Time: focus score
	focusScore      *int
	focusScoreSaved bool

	// Make Time: energize reminder
	energizeShown bool
	energizeTicks int

	// Make Time: energize activity log
	energizeActivity string
	energizeSaved    bool

	// Auto-break
	autoBreak      bool
	autoBreakTicks int

	// Deep Work: distraction review (shown after accomplishment in shutdown ritual)
	distractionReviewMode bool
	distractionReviewDone bool

	// Notifications
	notificationsEnabled bool
	notificationToggle   func(bool)

	// Daily summary on quit
	showingSummary bool
	summaryTicks   int

	// Session chaining: signals that user wants to start a new session
	WantsNewSession bool
}

// NewModel creates a new TUI model.
func NewModel(initialState *domain.CurrentState, info *domain.CompletionInfo, theme *config.ThemeConfig) Model {
	di := textinput.New()
	di.Placeholder = "What distracted you?"
	di.CharLimit = 200
	di.Width = 40

	ai := textinput.New()
	ai.Placeholder = "What did you accomplish?"
	ai.CharLimit = 200
	ai.Width = 40

	var shutdownInputs [3]textinput.Model
	placeholders := [3]string{
		"Review pending tasks — anything urgent?",
		"Plan for tomorrow",
		"Closing phrase (e.g. 'Shutdown complete')",
	}
	for i := range shutdownInputs {
		si := textinput.New()
		si.Placeholder = placeholders[i]
		si.CharLimit = 200
		si.Width = 40
		shutdownInputs[i] = si
	}

	return Model{
		state:               initialState,
		progress:            progress.New(progress.WithDefaultGradient()),
		completionInfo:      info,
		theme:               resolveTheme(theme),
		distractionInput:    di,
		accomplishmentInput: ai,
		shutdownInputs:      shutdownInputs,
	}
}

// Init initializes the TUI.
func (m Model) Init() tea.Cmd {
	return tickCmd()
}

// fetchStateCmd returns a tea.Cmd that fetches state asynchronously.
func fetchStateCmd(fetch func() *domain.CurrentState) tea.Cmd {
	return func() tea.Msg {
		s := fetch()
		return stateMsg{state: s}
	}
}

// getThemeColor returns the color for the current session type.
func (m Model) getThemeColor() lipgloss.Color {
	if m.state.ActiveSession != nil && m.state.ActiveSession.IsBreakSession() {
		return lipgloss.Color(m.theme.ColorBreak)
	}
	return lipgloss.Color(m.theme.ColorWork)
}

// getTimerColor returns the color for the timer, accounting for pause state.
func (m Model) getTimerColor() lipgloss.Color {
	if m.state.ActiveSession != nil && m.state.ActiveSession.Status == domain.SessionStatusPaused {
		return lipgloss.Color(m.theme.ColorPaused)
	}
	return m.getThemeColor()
}

func (m Model) showDailySummaryOrQuit() (tea.Model, tea.Cmd) {
	if m.state.TodayStats.WorkSessions > 0 {
		m.showingSummary = true
		m.summaryTicks = 3
		return m, tickCmd()
	}
	return m, tea.Quit
}

// Update handles messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Daily summary dismiss
	if m.showingSummary {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			return m, tea.Quit
		case tickMsg:
			m.summaryTicks--
			if m.summaryTicks <= 0 {
				return m, tea.Quit
			}
			return m, tickCmd()
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
		}
		return m, nil
	}

	// If in text input mode, handle separately
	if m.distractionMode {
		return m.updateDistractionInput(msg)
	}
	if m.accomplishmentMode {
		return m.updateAccomplishmentInput(msg)
	}
	if m.shutdownRitualMode {
		return m.updateShutdownRitual(msg)
	}
	if m.distractionReviewMode {
		return m.updateDistractionReview(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Cancel auto-break countdown on any non-quit key
		if m.autoBreakTicks > 0 {
			m.autoBreakTicks = 0
		}

		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.notificationsEnabled = !m.notificationsEnabled
			if m.notificationToggle != nil {
				m.notificationToggle(m.notificationsEnabled)
			}
		case "q":
			if m.completed {
				return m.showDailySummaryOrQuit()
			}
		case "c":
			if !m.completed {
				return m.showDailySummaryOrQuit()
			}
		case "s":
			if m.completed && m.commandCallback != nil {
				_ = m.commandCallback(ports.CmdStart)
				m.completed = false
				m.notified = false
				m.confirmBreak = false
				m.resetCompletionState()
			} else if !m.completed && m.state.ActiveSession != nil && m.state.ActiveSession.IsBreakSession() && m.commandCallback != nil {
				_ = m.commandCallback(ports.CmdStop)
				_ = m.commandCallback(ports.CmdStart)
				m.completed = false
				m.notified = false
			} else if !m.completed && m.state.ActiveSession == nil && m.commandCallback != nil {
				_ = m.commandCallback(ports.CmdStart)
				m.completed = false
				m.notified = false
			}
		case "p":
			if m.commandCallback != nil && m.state.ActiveSession != nil {
				if m.state.ActiveSession.Status == domain.SessionStatusRunning {
					_ = m.commandCallback(ports.CmdPause)
				} else {
					_ = m.commandCallback(ports.CmdResume)
				}
			}
			m.confirmBreak = false
			m.confirmFinish = false
		case "d":
			// Deep Work: open distraction input during active work session
			if m.mode != nil && m.mode.HasDistractionLog() && !m.completed && m.state.ActiveSession != nil && m.state.ActiveSession.Type == domain.SessionTypeWork {
				m.distractionMode = true
				m.distractionInput.Reset()
				m.distractionInput.Focus()
				return m, m.distractionInput.Cursor.BlinkCmd()
			}
		case "r":
			// Deep Work: open distraction review on work completion
			if m.mode != nil && m.mode.HasShutdownRitual() && m.completed && m.completedSessionType == domain.SessionTypeWork && len(m.distractions) > 0 && !m.distractionReviewDone {
				m.distractionReviewMode = true
			}
		case "a":
			// Deep Work: open shutdown ritual on work completion
			if m.mode != nil && m.mode.HasShutdownRitual() && m.completed && m.completedSessionType == domain.SessionTypeWork && !m.shutdownComplete && !m.accomplishmentSaved {
				m.shutdownRitualMode = true
				m.shutdownStep = 0
				for i := range m.shutdownInputs {
					m.shutdownInputs[i].Reset()
				}
				m.shutdownInputs[0].Focus()
				return m, m.shutdownInputs[0].Cursor.BlinkCmd()
			}
		case "1", "2", "3", "4", "5":
			// Make Time: record focus score on work completion
			if m.mode != nil && m.mode.HasFocusScore() && m.completed && m.completedSessionType == domain.SessionTypeWork && !m.focusScoreSaved {
				score := int(msg.String()[0] - '0')
				m.focusScore = &score
				m.focusScoreSaved = true
				if m.focusScoreCallback != nil {
					_ = m.focusScoreCallback(score)
				}
			}
		case "w":
			if m.mode != nil && m.mode.HasEnergizeReminder() && m.completed && m.completedSessionType == domain.SessionTypeWork && m.focusScoreSaved && !m.energizeSaved {
				m.energizeActivity = "walk"
				m.energizeSaved = true
				if m.energizeCallback != nil {
					_ = m.energizeCallback("walk")
				}
			}
		case "t":
			if m.mode != nil && m.mode.HasEnergizeReminder() && m.completed && m.completedSessionType == domain.SessionTypeWork && m.focusScoreSaved && !m.energizeSaved {
				m.energizeActivity = "stretch"
				m.energizeSaved = true
				if m.energizeCallback != nil {
					_ = m.energizeCallback("stretch")
				}
			}
		case "e":
			if m.mode != nil && m.mode.HasEnergizeReminder() && m.completed && m.completedSessionType == domain.SessionTypeWork && m.focusScoreSaved && !m.energizeSaved {
				m.energizeActivity = "exercise"
				m.energizeSaved = true
				if m.energizeCallback != nil {
					_ = m.energizeCallback("exercise")
				}
			}
		case "n":
			// Make Time: energize "none" option
			if m.mode != nil && m.mode.HasEnergizeReminder() && m.completed && m.completedSessionType == domain.SessionTypeWork && m.focusScoreSaved && !m.energizeSaved {
				m.energizeActivity = "none"
				m.energizeSaved = true
				if m.energizeCallback != nil {
					_ = m.energizeCallback("none")
				}
				return m, nil
			}
			// Session chaining: signal new session and quit TUI
			if m.completed && m.completionPromptsComplete() {
				m.WantsNewSession = true
				return m, tea.Quit
			}
		case "b":
			if m.completed && m.completedSessionType == domain.SessionTypeWork {
				if m.commandCallback != nil {
					_ = m.commandCallback(ports.CmdBreak)
					m.completed = false
					m.notified = false
					m.resetCompletionState()
				}
			} else if !m.completed && m.state.ActiveSession != nil && m.state.ActiveSession.Type == domain.SessionTypeWork {
				if m.confirmBreak {
					if m.commandCallback != nil {
						_ = m.commandCallback(ports.CmdStop)
						_ = m.commandCallback(ports.CmdBreak)
						m.completed = false
						m.notified = false
						m.confirmBreak = false
					}
				} else {
					m.confirmBreak = true
					m.confirmFinish = false
				}
			}
		case "f":
			if m.completed || m.state.ActiveSession == nil {
				return m, nil
			}
			if m.confirmFinish {
				if m.commandCallback != nil {
					_ = m.commandCallback(ports.CmdStop)
				}
				m.confirmFinish = false
				m.confirmBreak = false
				// Don't quit - let completion screen show with "What next?" menu
				return m, nil
			}
			m.confirmFinish = true
			m.confirmBreak = false
		default:
			m.confirmBreak = false
			m.confirmFinish = false
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = msg.Width - 4

	case tickMsg:
		// Make Time: check for energize reminder at 50%
		if m.mode != nil && m.mode.HasEnergizeReminder() && !m.energizeShown && m.state.ActiveSession != nil && !m.state.ActiveSession.IsBreakSession() {
			prog := m.state.ActiveSession.Progress()
			if prog >= 0.5 {
				m.energizeTicks = 30
				m.energizeShown = true
			}
		}
		if m.energizeTicks > 0 {
			m.energizeTicks--
		}

		// Auto-break countdown
		if m.autoBreakTicks > 0 {
			m.autoBreakTicks--
			if m.autoBreakTicks == 0 && m.commandCallback != nil {
				_ = m.commandCallback(ports.CmdBreak)
				m.completed = false
				m.notified = false
				m.resetCompletionState()
			}
		}

		cmds := []tea.Cmd{tickCmd()}
		if m.fetchState != nil {
			cmds = append(cmds, fetchStateCmd(m.fetchState))
		}
		return m, tea.Batch(cmds...)

	case stateMsg:
		if msg.state != nil {
			// Detect session completion: had a session before, now it's gone
			if m.state.ActiveSession != nil && msg.state.ActiveSession == nil {
				m.completedSessionType = m.state.ActiveSession.Type
				m.completedElapsed = m.state.ActiveSession.Duration
				m.completed = true

				// Fire notification callback once
				if !m.notified && m.onSessionComplete != nil {
					m.onSessionComplete(m.completedSessionType)
					m.notified = true
				}

				// Start auto-break countdown for work sessions
				if m.autoBreak && m.completedSessionType == domain.SessionTypeWork {
					m.autoBreakTicks = 3
				}
			}

			// Detect new session started → reset completed state
			if m.completed && msg.state.ActiveSession != nil {
				m.completed = false
				m.notified = false
				m.energizeShown = false
				m.energizeTicks = 0
				m.resetCompletionState()
			}

			m.state = msg.state
		}

	case *domain.CurrentState:
		m.state = msg
	}

	var cmd tea.Cmd
	newProgress, cmd := m.progress.Update(msg)
	if p, ok := newProgress.(progress.Model); ok {
		m.progress = p
	}
	return m, cmd
}

// resetCompletionState resets mode-specific completion state.
func (m *Model) resetCompletionState() {
	m.completedElapsed = 0
	m.accomplishmentSaved = false
	m.focusScore = nil
	m.focusScoreSaved = false
	m.distractions = nil
	m.distractionReviewMode = false
	m.distractionReviewDone = false
	m.energizeActivity = ""
	m.energizeSaved = false
	m.shutdownRitualMode = false
	m.shutdownStep = 0
	m.shutdownComplete = false
}

// completionPromptsComplete returns true when all mode-specific completion prompts are done.
func (m Model) completionPromptsComplete() bool {
	if m.mode == nil {
		return true
	}
	// Deep Work: need shutdown ritual complete (or accomplishment saved for legacy) and distraction review
	if m.mode.HasShutdownRitual() && m.completedSessionType == domain.SessionTypeWork {
		if !m.shutdownComplete && !m.accomplishmentSaved {
			return false
		}
		if len(m.distractions) > 0 && !m.distractionReviewDone {
			return false
		}
		return true
	}
	// Make Time: need focus score and energize activity
	if m.mode.HasFocusScore() && m.completedSessionType == domain.SessionTypeWork {
		return m.focusScoreSaved && m.energizeSaved
	}
	// Pomodoro or break: always ready
	return true
}

// updateDistractionInput handles input while in distraction logging mode.
func (m Model) updateDistractionInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Category picker sub-mode
	if m.distractionCategoryMode {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "i":
				m.distractions = append(m.distractions, m.distractionPendingText)
				if m.distractionCallback != nil {
					_ = m.distractionCallback(m.distractionPendingText, "internal")
				}
				m.distractionCategoryMode = false
				m.distractionMode = false
				return m, nil
			case "e":
				m.distractions = append(m.distractions, m.distractionPendingText)
				if m.distractionCallback != nil {
					_ = m.distractionCallback(m.distractionPendingText, "external")
				}
				m.distractionCategoryMode = false
				m.distractionMode = false
				return m, nil
			case "enter", "esc":
				m.distractions = append(m.distractions, m.distractionPendingText)
				if m.distractionCallback != nil {
					_ = m.distractionCallback(m.distractionPendingText, "")
				}
				m.distractionCategoryMode = false
				m.distractionMode = false
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			}
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			text := m.distractionInput.Value()
			if text != "" {
				m.distractionInput.Blur()
				if m.mode != nil && m.mode.HasDistractionLog() {
					m.distractionPendingText = text
					m.distractionCategoryMode = true
					return m, nil
				}
				m.distractions = append(m.distractions, text)
				if m.distractionCallback != nil {
					_ = m.distractionCallback(text, "")
				}
			}
			m.distractionMode = false
			m.distractionInput.Blur()
			return m, nil
		case "esc":
			m.distractionMode = false
			m.distractionInput.Blur()
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.distractionInput, cmd = m.distractionInput.Update(msg)
	return m, cmd
}

// updateAccomplishmentInput handles input while in accomplishment mode.
func (m Model) updateAccomplishmentInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			text := m.accomplishmentInput.Value()
			m.accomplishmentSaved = true // Always mark saved (empty = "skipped")
			if text != "" {
				if m.accomplishmentCallback != nil {
					_ = m.accomplishmentCallback(text)
				}
			}
			m.accomplishmentMode = false
			m.accomplishmentInput.Blur()
			// Auto-enter distraction review if there are distractions
			if len(m.distractions) > 0 && !m.distractionReviewDone {
				m.distractionReviewMode = true
			}
			return m, nil
		case "esc":
			m.accomplishmentMode = false
			m.accomplishmentInput.Blur()
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.accomplishmentInput, cmd = m.accomplishmentInput.Update(msg)
	return m, cmd
}

// updateShutdownRitual handles the 3-step shutdown ritual input.
func (m Model) updateShutdownRitual(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			// Save current step value and advance
			m.shutdownInputs[m.shutdownStep].Blur()
			m.shutdownStep++
			if m.shutdownStep >= 3 {
				return m.finishShutdownRitual()
			}
			m.shutdownInputs[m.shutdownStep].Focus()
			return m, m.shutdownInputs[m.shutdownStep].Cursor.BlinkCmd()
		case "esc":
			// Abandon the entire ritual — return to the completion screen
			m.shutdownRitualMode = false
			for i := range m.shutdownInputs {
				m.shutdownInputs[i].Blur()
			}
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.shutdownInputs[m.shutdownStep], cmd = m.shutdownInputs[m.shutdownStep].Update(msg)
	return m, cmd
}

// finishShutdownRitual completes the 3-step shutdown ritual.
func (m Model) finishShutdownRitual() (tea.Model, tea.Cmd) {
	m.shutdownRitualMode = false
	m.shutdownComplete = true
	m.accomplishmentSaved = true // Marks completion prompts as done

	ritual := domain.ShutdownRitual{
		PendingTasksReview: m.shutdownInputs[0].Value(),
		TomorrowPlan:       m.shutdownInputs[1].Value(),
		ClosingPhrase:      m.shutdownInputs[2].Value(),
	}
	if m.shutdownRitualCallback != nil {
		_ = m.shutdownRitualCallback(ritual)
	}

	// Auto-enter distraction review if there are distractions
	if len(m.distractions) > 0 && !m.distractionReviewDone {
		m.distractionReviewMode = true
	}
	return m, nil
}

// updateDistractionReview handles the distraction review overlay.
func (m Model) updateDistractionReview(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "esc":
			m.distractionReviewMode = false
			m.distractionReviewDone = true
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

// View renders the TUI.
func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	if m.showingSummary {
		return m.viewFullscreenSummary()
	}

	var sections []string

	// Title — subdued, not competing with the timer
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.ColorTitle)).MarginBottom(1)
	title := "Flow"
	if m.mode != nil {
		title = m.mode.TUITitle()
	}
	sections = append(sections, titleStyle.Render(fmt.Sprintf("%s %s", m.theme.IconApp, title)))

	// Active task
	if m.state.ActiveTask != nil {
		taskText := fmt.Sprintf("%s Task: %s", m.theme.IconTask, m.state.ActiveTask.Title)
		taskStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorTask))
		sections = append(sections, taskStyle.Render(taskText))
	}

	// Intended outcome
	if m.state.ActiveSession != nil && m.state.ActiveSession.IntendedOutcome != "" {
		outcome := lipgloss.NewStyle().Italic(true).Faint(true).Render("Goal: " + m.state.ActiveSession.IntendedOutcome)
		sections = append(sections, outcome)
	}

	// Session tags
	if m.state.ActiveSession != nil && len(m.state.ActiveSession.Tags) > 0 {
		tagStr := ""
		for _, t := range m.state.ActiveSession.Tags {
			if tagStr != "" {
				tagStr += "  "
			}
			tagStr += "#" + t
		}
		tagStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))
		sections = append(sections, tagStyle.Render(tagStr))
	}

	if m.completed {
		if m.completedSessionType == domain.SessionTypeWork {
			sections = m.viewWorkComplete(sections)
		} else {
			sections = m.viewBreakComplete(sections)
		}
	} else if m.state.ActiveSession != nil {
		sections = m.viewActiveSession(sections)
	} else {
		idleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorPaused))
		sections = append(sections, idleStyle.Render("No active session"))
		sections = append(sections, "")
		helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))
		sections = append(sections, helpStyle.Render("[s]tart  [c]lose"))
	}

	content := lipgloss.JoinVertical(lipgloss.Center, sections...)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m Model) viewFullscreenSummary() string {
	var sections []string

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.ColorTitle)).MarginBottom(1)
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorWork))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	sections = append(sections, titleStyle.Render(fmt.Sprintf("%s Today's Summary", m.theme.IconApp)))

	stats := m.state.TodayStats
	sections = append(sections, statusStyle.Render(fmt.Sprintf("%s %d sessions, %s focused",
		m.theme.IconStats, stats.WorkSessions, formatDuration(stats.TotalWorkTime))))
	if stats.BreaksTaken > 0 {
		sections = append(sections, helpStyle.Render(fmt.Sprintf("%d breaks taken", stats.BreaksTaken)))
	}

	sections = append(sections, "")
	sections = append(sections, helpStyle.Render("Press any key to exit"))

	content := lipgloss.JoinVertical(lipgloss.Center, sections...)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m Model) viewWorkComplete(sections []string) []string {
	// Branch by methodology for mode-specific completion screens
	if m.mode != nil && m.mode.HasShutdownRitual() {
		return m.viewDeepWorkComplete(sections)
	}
	if m.mode != nil && m.mode.HasFocusScore() {
		return m.viewMakeTimeComplete(sections)
	}
	return m.viewDefaultWorkComplete(sections)
}

func (m Model) viewDefaultWorkComplete(sections []string) []string {
	statusStyle := lipgloss.NewStyle().Foreground(m.getThemeColor())
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	sections = append(sections, "")
	if m.completedElapsed > 0 {
		sections = append(sections, statusStyle.Render(fmt.Sprintf("Session complete — %s worked", formatDuration(m.completedElapsed))))
	} else {
		sections = append(sections, statusStyle.Render("Session complete! Great work."))
	}
	sections = append(sections, m.progress.ViewAs(1.0))

	// Show break info
	if m.completionInfo != nil {
		breakLabel := domain.GetSessionTypeLabel(m.completionInfo.NextBreakType)
		breakDur := formatDuration(m.completionInfo.NextBreakDuration)

		breakLine := fmt.Sprintf("[b] %s %s", breakDur, breakLabel)
		if m.completionInfo.NextBreakType == domain.SessionTypeLongBreak {
			breakLine += " - you earned it!"
		}
		sections = append(sections, "")
		sections = append(sections, statusStyle.Render(breakLine))

		if m.completionInfo.NextBreakType == domain.SessionTypeShortBreak {
			countLine := fmt.Sprintf("%d of %d sessions until long break",
				m.completionInfo.SessionsBeforeLong-m.completionInfo.SessionsUntilLong,
				m.completionInfo.SessionsBeforeLong)
			sections = append(sections, helpStyle.Render(countLine))
		}
	}

	// Daily stats
	stats := m.state.TodayStats
	statsText := fmt.Sprintf("%s Today: %d work sessions, %d breaks, %s worked",
		m.theme.IconStats, stats.WorkSessions, stats.BreaksTaken, formatDuration(stats.TotalWorkTime))
	sections = append(sections, "")
	sections = append(sections, helpStyle.Render(statsText))

	sections = append(sections, "")
	if m.autoBreakTicks > 0 {
		sections = append(sections, statusStyle.Render(fmt.Sprintf("Break starting in %ds... press any key to cancel", m.autoBreakTicks)))
	} else {
		sections = append(sections, helpStyle.Render("[n]ew session  [b]reak  [q]uit"))
	}
	sections = append(sections, "")
	sections = append(sections, helpStyle.Render("Customize in ~/.flow/config.toml"))
	return sections
}

func (m Model) viewDeepWorkComplete(sections []string) []string {
	statusStyle := lipgloss.NewStyle().Foreground(m.getThemeColor())
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	sections = append(sections, "")
	sections = append(sections, statusStyle.Render("Deep Work Session Complete."))
	sections = append(sections, m.progress.ViewAs(1.0))

	// Distraction count
	if len(m.distractions) > 0 {
		sections = append(sections, "")
		sections = append(sections, helpStyle.Render(fmt.Sprintf("Distractions logged: %d", len(m.distractions))))
	}

	// Deep Work Score: total work time today / goal target
	stats := m.state.TodayStats
	deepWorkHours := stats.TotalWorkTime.Hours()
	goalHours := 4.0
	if m.mode != nil && m.mode.DeepWorkGoalHours() > 0 {
		goalHours = m.mode.DeepWorkGoalHours()
	}
	sections = append(sections, "")
	sections = append(sections, statusStyle.Render(fmt.Sprintf("Deep Work Score: %s today", formatDuration(stats.TotalWorkTime))))
	sections = append(sections, helpStyle.Render(fmt.Sprintf("%.0f%% of %.0fh target", deepWorkHours/goalHours*100, goalHours)))

	// Deep Work streak
	if m.completionInfo != nil && m.completionInfo.DeepWorkStreak > 0 {
		sections = append(sections, "")
		sections = append(sections, statusStyle.Render(fmt.Sprintf("Deep Work streak: %d days", m.completionInfo.DeepWorkStreak)))
	}

	// Shutdown ritual / accomplishment prompt
	sections = append(sections, "")
	shutdownStepLabels := [3]string{
		"Review pending tasks — anything urgent?",
		"Plan for tomorrow:",
		"Closing phrase (e.g. 'Shutdown complete'):",
	}
	if m.shutdownRitualMode {
		sections = append(sections, statusStyle.Render(fmt.Sprintf("Shutdown Ritual (step %d/3):", m.shutdownStep+1)))
		sections = append(sections, helpStyle.Render(shutdownStepLabels[m.shutdownStep]))
		sections = append(sections, m.shutdownInputs[m.shutdownStep].View())
		sections = append(sections, helpStyle.Render("enter save/skip step · esc exit ritual"))
	} else if m.accomplishmentMode {
		sections = append(sections, helpStyle.Render("What did you accomplish? ")+m.accomplishmentInput.View())
		sections = append(sections, helpStyle.Render("enter save · esc cancel"))
	} else if m.distractionReviewMode {
		sections = append(sections, statusStyle.Render("Distraction Review:"))
		for i, d := range m.distractions {
			sections = append(sections, helpStyle.Render(fmt.Sprintf("  %d. %s", i+1, d)))
		}
		sections = append(sections, "")
		sections = append(sections, helpStyle.Render("Consider batching these for tomorrow."))
		sections = append(sections, helpStyle.Render("enter dismiss"))
	} else if m.shutdownComplete || m.accomplishmentSaved {
		if len(m.distractions) > 0 && !m.distractionReviewDone {
			sections = append(sections, statusStyle.Render("Shutdown ritual complete."))
			sections = append(sections, helpStyle.Render(fmt.Sprintf("[r]eview %d distractions", len(m.distractions))))
		} else {
			sections = append(sections, statusStyle.Render("Shutdown ritual complete."))
		}
	} else {
		sections = append(sections, helpStyle.Render("[a] Shutdown ritual"))
	}

	// Break info
	if m.completionInfo != nil {
		breakLabel := domain.GetSessionTypeLabel(m.completionInfo.NextBreakType)
		breakDur := formatDuration(m.completionInfo.NextBreakDuration)
		sections = append(sections, "")
		sections = append(sections, statusStyle.Render(fmt.Sprintf("[b] %s %s", breakDur, breakLabel)))
	}

	sections = append(sections, "")
	if m.completionPromptsComplete() {
		sections = append(sections, helpStyle.Render("[n]ew session  [b]reak  [q]uit"))
	} else if !m.shutdownComplete && !m.accomplishmentSaved {
		sections = append(sections, helpStyle.Render("→ [n]ew session locked: complete the shutdown ritual first"))
		sections = append(sections, helpStyle.Render("  Newport: a ritual trains your brain to fully disconnect — without it, work bleeds into rest."))
		sections = append(sections, helpStyle.Render("[a] shutdown ritual  [b]reak  [q]uit"))
	} else if len(m.distractions) > 0 && !m.distractionReviewDone {
		sections = append(sections, helpStyle.Render("→ [n]ew session locked: review your distractions first"))
		sections = append(sections, helpStyle.Render("  Newport: batch distractions and schedule them — don't let them follow you into the next block."))
		sections = append(sections, helpStyle.Render("[r]eview distractions  [b]reak  [q]uit"))
	} else {
		sections = append(sections, helpStyle.Render("[b]reak  [q]uit"))
	}
	return sections
}

func (m Model) viewMakeTimeComplete(sections []string) []string {
	statusStyle := lipgloss.NewStyle().Foreground(m.getThemeColor())
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	sections = append(sections, "")
	sections = append(sections, statusStyle.Render("Session complete!"))
	sections = append(sections, m.progress.ViewAs(1.0))

	// Highlight acknowledgment
	if m.state.ActiveTask != nil && m.state.ActiveTask.HighlightDate != nil {
		sections = append(sections, "")
		sections = append(sections, statusStyle.Render("You made time for your Highlight today."))
	}

	// Focus score
	sections = append(sections, "")
	if m.focusScoreSaved && m.focusScore != nil {
		sections = append(sections, statusStyle.Render(fmt.Sprintf("Focus score: %d/5", *m.focusScore)))
	} else {
		sections = append(sections, helpStyle.Render("How focused were you? [1] [2] [3] [4] [5]"))
	}

	// Energize activity (shown after focus score is saved)
	if m.focusScoreSaved {
		sections = append(sections, "")
		if m.energizeSaved {
			sections = append(sections, statusStyle.Render(fmt.Sprintf("Energize: %s", m.energizeActivity)))
		} else {
			sections = append(sections, helpStyle.Render("Energize? [w]alk [t]stretch [e]xercise [n]one"))
		}
	}

	// Daily stats
	stats := m.state.TodayStats
	statsText := fmt.Sprintf("%s Today: %d sessions, %s worked",
		m.theme.IconStats, stats.WorkSessions, formatDuration(stats.TotalWorkTime))
	sections = append(sections, "")
	sections = append(sections, helpStyle.Render(statsText))

	// Break info
	if m.completionInfo != nil {
		breakLabel := domain.GetSessionTypeLabel(m.completionInfo.NextBreakType)
		breakDur := formatDuration(m.completionInfo.NextBreakDuration)
		sections = append(sections, "")
		sections = append(sections, statusStyle.Render(fmt.Sprintf("[b] %s %s", breakDur, breakLabel)))
	}

	sections = append(sections, "")
	if m.completionPromptsComplete() {
		sections = append(sections, helpStyle.Render("[n]ew session  [b]reak  [q]uit"))
	} else if !m.focusScoreSaved {
		sections = append(sections, helpStyle.Render("→ [n]ew session locked: rate your focus first"))
		sections = append(sections, helpStyle.Render("  Make Time: tracking focus shows you when you're at your best — skip it and the data disappears."))
		sections = append(sections, helpStyle.Render("[1-5] focus score  [b]reak  [q]uit"))
	} else {
		sections = append(sections, helpStyle.Render("→ [n]ew session locked: log how you'll recharge first"))
		sections = append(sections, helpStyle.Render("  Make Time: energy fuels your next Highlight — Knapp says laser focus requires an energized body."))
		sections = append(sections, helpStyle.Render("[w]alk [t]stretch [e]xercise [n]one  [b]reak  [q]uit"))
	}
	return sections
}

func (m Model) viewBreakComplete(sections []string) []string {
	statusStyle := lipgloss.NewStyle().Foreground(m.getThemeColor())
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	sections = append(sections, "")
	sections = append(sections, statusStyle.Render("Break over!"))
	sections = append(sections, helpStyle.Render("Start your next session or call it a day."))
	sections = append(sections, m.progress.ViewAs(1.0))

	// Daily stats
	stats := m.state.TodayStats
	statsText := fmt.Sprintf("%s Today: %d work sessions, %d breaks, %s worked",
		m.theme.IconStats, stats.WorkSessions, stats.BreaksTaken, formatDuration(stats.TotalWorkTime))
	sections = append(sections, "")
	sections = append(sections, helpStyle.Render(statsText))

	sections = append(sections, "")
	sections = append(sections, helpStyle.Render("[n]ew session  [q]uit"))
	return sections
}

func (m Model) viewActiveSession(sections []string) []string {
	session := m.state.ActiveSession
	timerColor := m.getTimerColor()
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorPaused))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	// Session type and status
	statusText := fmt.Sprintf("Status: %s (%s)",
		domain.GetSessionTypeLabel(session.Type),
		domain.GetStatusLabel(session.Status))
	sections = append(sections, statusStyle.Render(statusText))

	// Big ASCII timer
	remaining := session.RemainingTime()
	timeStr := formatDuration(remaining)
	sections = append(sections, "")
	sections = append(sections, renderBigTime(timeStr, timerColor, m.width))

	// Pause badge
	if session.Status == domain.SessionStatusPaused {
		pauseBadge := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color(m.theme.ColorPaused)).
			Padding(0, 1).
			Render(fmt.Sprintf("%s PAUSED", m.theme.IconPaused))
		sections = append(sections, "")
		sections = append(sections, pauseBadge)
	}

	// Make Time: energize reminder
	if m.energizeTicks > 0 {
		reminderStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.ColorTask))
		sections = append(sections, "")
		sections = append(sections, reminderStyle.Render("Quick stretch? Take a moment to energize."))
		sections = append(sections, helpStyle.Render("[b]reak"))
	}

	// Distraction input overlay
	if m.distractionMode {
		sections = append(sections, "")
		if m.distractionCategoryMode {
			sections = append(sections, helpStyle.Render(fmt.Sprintf("Categorize: %s", m.distractionPendingText)))
			sections = append(sections, helpStyle.Render("[i]nternal  [e]xternal  [enter] skip category"))
		} else {
			sections = append(sections, helpStyle.Render("Log distraction: ")+m.distractionInput.View())
			sections = append(sections, helpStyle.Render("enter save · esc cancel"))
		}
	}

	// Dynamic progress bar
	sections = append(sections, "")
	prog := session.Progress()
	var pbar progress.Model
	if session.Status == domain.SessionStatusPaused {
		pbar = progress.New(progress.WithGradient(m.theme.PausedGradientStart, m.theme.PausedGradientEnd))
	} else if session.IsBreakSession() {
		pbar = progress.New(progress.WithGradient(m.theme.BreakGradientStart, m.theme.BreakGradientEnd))
	} else {
		pbar = progress.New(progress.WithGradient(m.theme.WorkGradientStart, m.theme.WorkGradientEnd))
	}
	pbar.Width = m.width - 4
	sections = append(sections, pbar.ViewAs(prog))

	// Git context
	if session.GitBranch != "" {
		commitShort := session.GitCommit
		if len(commitShort) > 7 {
			commitShort = commitShort[:7]
		}
		gitInfo := fmt.Sprintf("%s %s (%s)", m.theme.IconGit, session.GitBranch, commitShort)
		sections = append(sections, helpStyle.Render(gitInfo))
	}

	// Help
	notifLabel := "off"
	if m.notificationsEnabled {
		notifLabel = "on"
	}
	sections = append(sections, "")
	if m.confirmFinish {
		sections = append(sections, helpStyle.Render("Stop session? [f] confirm  [esc] cancel"))
	} else if m.confirmBreak {
		sections = append(sections, helpStyle.Render("Start break? [b] confirm  [esc] cancel"))
	} else if session.IsBreakSession() {
		sections = append(sections, helpStyle.Render(fmt.Sprintf("[s]kip  [p]ause  [f]inish  [c]lose  tab:notify %s", notifLabel)))
	} else {
		pauseAction := "[p]ause"
		if session.Status == domain.SessionStatusPaused {
			pauseAction = "[p]resume"
		}
		helpText := fmt.Sprintf("%s  [f]inish  [b]reak  [c]lose", pauseAction)
		if m.mode != nil && m.mode.HasDistractionLog() && session.Status == domain.SessionStatusRunning {
			if len(m.distractions) > 0 {
				helpText = fmt.Sprintf("%s  [d]istraction(%d)  [f]inish  [b]reak  [c]lose", pauseAction, len(m.distractions))
			} else {
				helpText = fmt.Sprintf("%s  [d]istraction  [f]inish  [b]reak  [c]lose", pauseAction)
			}
		}
		helpText += fmt.Sprintf("  tab:notify %s", notifLabel)
		sections = append(sections, helpStyle.Render(helpText))
	}
	return sections
}

// tickCmd creates a command that sends a tick message.
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// formatDuration formats a duration as MM:SS.
func formatDuration(d time.Duration) string {
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

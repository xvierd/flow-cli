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
	state                *domain.CurrentState
	progress             progress.Model
	width                int
	height               int
	completed            bool
	completedSessionType domain.SessionType
	notified             bool
	confirmBreak         bool
	confirmFinish        bool
	fetchState           func() *domain.CurrentState
	commandCallback      func(ports.TimerCommand) error
	onSessionComplete    func(domain.SessionType)
	distractionCallback    func(string) error
	accomplishmentCallback func(string) error
	focusScoreCallback     func(int) error
	completionInfo       *domain.CompletionInfo
	theme                config.ThemeConfig
	mode                 methodology.Mode

	// Deep Work: distraction log
	distractionMode  bool
	distractionInput textinput.Model
	distractions     []string

	// Deep Work: accomplishment (shutdown ritual)
	accomplishmentMode  bool
	accomplishmentInput textinput.Model
	accomplishmentSaved bool

	// Make Time: focus score
	focusScore      *int
	focusScoreSaved bool

	// Make Time: energize reminder
	energizeShown bool
	energizeTicks int
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

	return Model{
		state:               initialState,
		progress:            progress.New(progress.WithDefaultGradient()),
		completionInfo:      info,
		theme:               resolveTheme(theme),
		distractionInput:    di,
		accomplishmentInput: ai,
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

// Update handles messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// If in text input mode, handle separately
	if m.distractionMode {
		return m.updateDistractionInput(msg)
	}
	if m.accomplishmentMode {
		return m.updateAccomplishmentInput(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "c":
			return m, tea.Quit
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
		case "a":
			// Deep Work: open accomplishment input on work completion
			if m.mode != nil && m.mode.HasShutdownRitual() && m.completed && m.completedSessionType == domain.SessionTypeWork && !m.accomplishmentSaved {
				m.accomplishmentMode = true
				m.accomplishmentInput.Reset()
				m.accomplishmentInput.Focus()
				return m, m.accomplishmentInput.Cursor.BlinkCmd()
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
				return m, tea.Quit
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
				m.energizeTicks = 5
				m.energizeShown = true
			}
		}
		if m.energizeTicks > 0 {
			m.energizeTicks--
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
				m.completed = true

				// Fire notification callback once
				if !m.notified && m.onSessionComplete != nil {
					m.onSessionComplete(m.completedSessionType)
					m.notified = true
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
	m.accomplishmentSaved = false
	m.focusScore = nil
	m.focusScoreSaved = false
	m.distractions = nil
}

// updateDistractionInput handles input while in distraction logging mode.
func (m Model) updateDistractionInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			text := m.distractionInput.Value()
			if text != "" {
				m.distractions = append(m.distractions, text)
				if m.distractionCallback != nil {
					_ = m.distractionCallback(text)
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
			if text != "" {
				m.accomplishmentSaved = true
				if m.accomplishmentCallback != nil {
					_ = m.accomplishmentCallback(text)
				}
			}
			m.accomplishmentMode = false
			m.accomplishmentInput.Blur()
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

// View renders the TUI.
func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var sections []string

	// Title — subdued, not competing with the timer
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.ColorTitle)).MarginBottom(1)
	sections = append(sections, titleStyle.Render(fmt.Sprintf("%s Flow - Pomodoro Timer", m.theme.IconApp)))

	// Active task
	if m.state.ActiveTask != nil {
		taskText := fmt.Sprintf("%s Task: %s", m.theme.IconTask, m.state.ActiveTask.Title)
		taskStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorTask))
		sections = append(sections, taskStyle.Render(taskText))
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
	sections = append(sections, statusStyle.Render("Session complete! Great work."))
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
	sections = append(sections, helpStyle.Render("[b]reak  [s]kip  [c]lose"))
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

	// Deep Work Score: total work time today / 8h target
	stats := m.state.TodayStats
	deepWorkHours := stats.TotalWorkTime.Hours()
	sections = append(sections, "")
	sections = append(sections, statusStyle.Render(fmt.Sprintf("Deep Work Score: %s today", formatDuration(stats.TotalWorkTime))))
	sections = append(sections, helpStyle.Render(fmt.Sprintf("%.0f%% of 8h target", deepWorkHours/8.0*100)))

	// Accomplishment prompt / saved state
	sections = append(sections, "")
	if m.accomplishmentMode {
		sections = append(sections, helpStyle.Render("What did you accomplish? ")+m.accomplishmentInput.View())
		sections = append(sections, helpStyle.Render("enter save · esc cancel"))
	} else if m.accomplishmentSaved {
		sections = append(sections, statusStyle.Render("Shutdown ritual complete."))
	} else {
		sections = append(sections, helpStyle.Render("[a] Record what you accomplished (shutdown ritual)"))
	}

	// Break info
	if m.completionInfo != nil {
		breakLabel := domain.GetSessionTypeLabel(m.completionInfo.NextBreakType)
		breakDur := formatDuration(m.completionInfo.NextBreakDuration)
		sections = append(sections, "")
		sections = append(sections, statusStyle.Render(fmt.Sprintf("[b] %s %s", breakDur, breakLabel)))
	}

	sections = append(sections, "")
	helpText := "[b]reak  [s]kip  [c]lose"
	if !m.accomplishmentSaved {
		helpText = "[a]ccomplishment  [b]reak  [s]kip  [c]lose"
	}
	sections = append(sections, helpStyle.Render(helpText))
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
	sections = append(sections, helpStyle.Render("[b]reak  [s]kip  [c]lose"))
	return sections
}

func (m Model) viewBreakComplete(sections []string) []string {
	statusStyle := lipgloss.NewStyle().Foreground(m.getThemeColor())
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	sections = append(sections, "")
	sections = append(sections, statusStyle.Render("Break over! Ready to focus?"))
	sections = append(sections, m.progress.ViewAs(1.0))

	// Daily stats
	stats := m.state.TodayStats
	statsText := fmt.Sprintf("%s Today: %d work sessions, %d breaks, %s worked",
		m.theme.IconStats, stats.WorkSessions, stats.BreaksTaken, formatDuration(stats.TotalWorkTime))
	sections = append(sections, "")
	sections = append(sections, helpStyle.Render(statsText))

	sections = append(sections, "")
	sections = append(sections, helpStyle.Render("[s]tart new session  [c]lose"))
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
	}

	// Distraction input overlay
	if m.distractionMode {
		sections = append(sections, "")
		sections = append(sections, helpStyle.Render("Log distraction: ")+m.distractionInput.View())
		sections = append(sections, helpStyle.Render("enter save · esc cancel"))
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
	sections = append(sections, "")
	if m.confirmFinish {
		sections = append(sections, helpStyle.Render("Stop session? Press [f] again to confirm"))
	} else if m.confirmBreak {
		sections = append(sections, helpStyle.Render("End session and start break? Press [b] again to confirm"))
	} else if session.IsBreakSession() {
		sections = append(sections, helpStyle.Render("[s]kip  [p]ause  [f]inish  [c]lose"))
	} else {
		helpText := "[p]ause  [f]inish  [b]reak  [c]lose"
		if m.mode != nil && m.mode.HasDistractionLog() {
			if len(m.distractions) > 0 {
				helpText = fmt.Sprintf("[p]ause  [d]istraction(%d)  [f]inish  [b]reak  [c]lose", len(m.distractions))
			} else {
				helpText = "[p]ause  [d]istraction  [f]inish  [b]reak  [c]lose"
			}
		}
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

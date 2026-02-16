package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

// inlinePhase tracks which phase the inline UI is in.
type inlinePhase int

const (
	phasePickDuration inlinePhase = iota
	phaseTaskName
	phaseTimer
)

// sessionStartedMsg is sent after a session is started from the setup phase.
type sessionStartedMsg struct{}

// InlineModel is a compact timer that runs setup + timer in a single bubbletea program.
type InlineModel struct {
	// Phase
	phase inlinePhase

	// Setup: duration picker
	presets      []config.SessionPreset
	presetCursor int
	breakInfo    string

	// Setup: task name
	taskInput textinput.Model

	// Timer state
	state             *domain.CurrentState
	progress          progress.Model
	width             int
	completed         bool
	completedType     domain.SessionType
	notified          bool
	confirmBreak      bool
	confirmFinish     bool
	fetchState        func() *domain.CurrentState
	commandCallback   func(ports.TimerCommand) error
	onSessionComplete func(domain.SessionType)
	completionInfo    *domain.CompletionInfo
	theme             config.ThemeConfig

	// Callbacks for session creation (called during setup phase)
	onStartSession func(presetIndex int, taskName string) error
}

// getTerminalWidth returns the current terminal width, defaulting to 80.
func getTerminalWidth() int {
	w, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || w < 40 {
		return 80
	}
	return w
}

// NewInlineModel creates a new inline TUI model starting in the setup phase.
func NewInlineModel(state *domain.CurrentState, info *domain.CompletionInfo, theme *config.ThemeConfig) InlineModel {
	resolved := resolveTheme(theme)
	w := getTerminalWidth()
	pbar := progress.New(progress.WithGradient(resolved.WorkGradientStart, resolved.WorkGradientEnd))
	pbar.Width = w - 16

	ti := textinput.New()
	ti.Placeholder = "Enter to skip"
	ti.CharLimit = 120
	ti.Width = w - 10

	// If there's already an active session, skip setup
	startPhase := phasePickDuration
	if state.ActiveSession != nil {
		startPhase = phaseTimer
	}

	return InlineModel{
		phase:          startPhase,
		state:          state,
		progress:       pbar,
		width:          w,
		completionInfo: info,
		theme:          resolved,
		taskInput:      ti,
	}
}

func (m InlineModel) Init() tea.Cmd {
	if m.phase == phaseTimer {
		return tickCmd()
	}
	return nil
}

func (m InlineModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.phase {
	case phasePickDuration:
		return m.updatePickDuration(msg)
	case phaseTaskName:
		return m.updateTaskName(msg)
	case phaseTimer:
		return m.updateTimer(msg)
	}
	return m, nil
}

func (m InlineModel) updatePickDuration(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h":
			if m.presetCursor > 0 {
				m.presetCursor--
			}
		case "right", "l":
			if m.presetCursor < len(m.presets)-1 {
				m.presetCursor++
			}
		case "1":
			m.presetCursor = 0
			m.phase = phaseTaskName
			m.taskInput.Focus()
			return m, m.taskInput.Cursor.BlinkCmd()
		case "2":
			if len(m.presets) > 1 {
				m.presetCursor = 1
				m.phase = phaseTaskName
				m.taskInput.Focus()
				return m, m.taskInput.Cursor.BlinkCmd()
			}
		case "3":
			if len(m.presets) > 2 {
				m.presetCursor = 2
				m.phase = phaseTaskName
				m.taskInput.Focus()
				return m, m.taskInput.Cursor.BlinkCmd()
			}
		case "enter":
			m.phase = phaseTaskName
			m.taskInput.Focus()
			return m, m.taskInput.Cursor.BlinkCmd()
		case "c", "ctrl+c", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m InlineModel) updateTaskName(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			// Start the session
			if m.onStartSession != nil {
				taskName := strings.TrimSpace(m.taskInput.Value())
				if err := m.onStartSession(m.presetCursor, taskName); err != nil {
					// Transition to timer anyway, error will show as no session
					m.phase = phaseTimer
					return m, tickCmd()
				}
			}
			m.phase = phaseTimer
			return m, tickCmd()
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			// Go back to duration picker
			m.phase = phasePickDuration
			m.taskInput.Blur()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.taskInput, cmd = m.taskInput.Update(msg)
	return m, cmd
}

func (m InlineModel) updateTimer(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case "b":
			if m.completed && m.completedType == domain.SessionTypeWork {
				if m.commandCallback != nil {
					_ = m.commandCallback(ports.CmdBreak)
					m.completed = false
					m.notified = false
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

	case tickMsg:
		cmds := []tea.Cmd{tickCmd()}
		if m.fetchState != nil {
			cmds = append(cmds, fetchStateCmd(m.fetchState))
		}
		return m, tea.Batch(cmds...)

	case stateMsg:
		if msg.state != nil {
			if m.state.ActiveSession != nil && msg.state.ActiveSession == nil {
				m.completedType = m.state.ActiveSession.Type
				m.completed = true
				if !m.notified && m.onSessionComplete != nil {
					m.onSessionComplete(m.completedType)
					m.notified = true
				}
			}
			if m.completed && msg.state.ActiveSession != nil {
				m.completed = false
				m.notified = false
			}
			m.state = msg.state
		}

	case *domain.CurrentState:
		m.state = msg
	}

	return m, nil
}

// --- View ---

func (m InlineModel) View() string {
	switch m.phase {
	case phasePickDuration:
		return m.viewPickDuration()
	case phaseTaskName:
		return m.viewTaskName()
	case phaseTimer:
		return m.viewTimer()
	}
	return ""
}

func (m InlineModel) viewPickDuration() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.ColorTitle))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorWork)).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	b.WriteString(titleStyle.Render("  Duration:") + "  ")

	for i, p := range m.presets {
		label := fmt.Sprintf("%s %s", p.Name, formatMinutesCompact(p.Duration))
		if i == m.presetCursor {
			b.WriteString(activeStyle.Render(" ▸ " + label + " "))
		} else {
			b.WriteString(dimStyle.Render("   " + label + " "))
		}
	}
	b.WriteString("\n")

	if m.breakInfo != "" {
		b.WriteString(dimStyle.Render("  "+m.breakInfo) + "\n")
	}

	b.WriteString(dimStyle.Render("  ←/→ select · enter confirm · c close") + "\n")

	return b.String()
}

func (m InlineModel) viewTaskName() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.ColorTitle))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorWork)).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	// Show selected duration
	p := m.presets[m.presetCursor]
	b.WriteString(activeStyle.Render(fmt.Sprintf("  ▸ %s %s", p.Name, formatMinutesCompact(p.Duration))))
	b.WriteString("\n")

	// Task input
	b.WriteString(titleStyle.Render("  Task: "))
	b.WriteString(m.taskInput.View())
	b.WriteString("\n")

	b.WriteString(dimStyle.Render("  enter start · esc back · ctrl+c quit") + "\n")

	return b.String()
}

func (m InlineModel) viewTimer() string {
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorWork)).Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))
	pausedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorPaused)).Bold(true)

	if m.completed {
		return m.viewInlineComplete(accent, dim)
	}

	if m.state.ActiveSession == nil {
		return dim.Render("  No active session") + "\n" +
			dim.Render("  [s]tart  [c]lose") + "\n"
	}

	return m.viewInlineActive(accent, dim, pausedStyle)
}

func (m InlineModel) viewInlineActive(accent, dim, pausedStyle lipgloss.Style) string {
	session := m.state.ActiveSession
	remaining := session.RemainingTime()
	timeStr := formatDuration(remaining)
	prog := session.Progress()
	typeLabel := domain.GetSessionTypeLabel(session.Type)

	var b strings.Builder

	// Line 1: icon + type + time
	if session.Status == domain.SessionStatusPaused {
		b.WriteString(pausedStyle.Render(fmt.Sprintf("  %s %s  %s  %s PAUSED",
			m.theme.IconApp, typeLabel, timeStr, m.theme.IconPaused)))
	} else {
		b.WriteString(accent.Render(fmt.Sprintf("  %s %s  %s", m.theme.IconApp, typeLabel, timeStr)))
	}

	// Task on same line
	if m.state.ActiveTask != nil {
		b.WriteString(dim.Render(fmt.Sprintf("  %s %s", m.theme.IconTask, m.state.ActiveTask.Title)))
	}
	b.WriteString("\n")

	// Line 2: progress bar
	barWidth := m.width - 16
	if barWidth < 20 {
		barWidth = 20
	}
	var pbar progress.Model
	if session.Status == domain.SessionStatusPaused {
		pbar = progress.New(progress.WithGradient(m.theme.PausedGradientStart, m.theme.PausedGradientEnd))
	} else if session.IsBreakSession() {
		pbar = progress.New(progress.WithGradient(m.theme.BreakGradientStart, m.theme.BreakGradientEnd))
	} else {
		pbar = progress.New(progress.WithGradient(m.theme.WorkGradientStart, m.theme.WorkGradientEnd))
	}
	pbar.Width = barWidth
	b.WriteString("  " + pbar.ViewAs(prog))
	b.WriteString(dim.Render(fmt.Sprintf("  %d%%", int(prog*100))))
	b.WriteString("\n")

	// Line 3: help
	if m.confirmFinish {
		b.WriteString(dim.Render("  Stop session? Press [f] again to confirm"))
	} else if m.confirmBreak {
		b.WriteString(dim.Render("  Press [b] again to confirm break"))
	} else if session.IsBreakSession() {
		b.WriteString(dim.Render("  [s]kip [p]ause [f]inish [c]lose"))
	} else {
		b.WriteString(dim.Render("  [p]ause [f]inish [b]reak [c]lose"))
	}
	b.WriteString("\n")

	return b.String()
}

func (m InlineModel) viewInlineComplete(accent, dim lipgloss.Style) string {
	var b strings.Builder

	if m.completedType == domain.SessionTypeWork {
		b.WriteString(accent.Render(fmt.Sprintf("  %s Session complete! ", m.theme.IconApp)))

		if m.completionInfo != nil {
			breakLabel := domain.GetSessionTypeLabel(m.completionInfo.NextBreakType)
			breakDur := formatDuration(m.completionInfo.NextBreakDuration)
			b.WriteString(dim.Render(fmt.Sprintf("Next: %s %s", breakDur, breakLabel)))
		}
		b.WriteString("\n")

		stats := m.state.TodayStats
		b.WriteString(dim.Render(fmt.Sprintf("  %s %d sessions, %s worked today",
			m.theme.IconStats, stats.WorkSessions, formatMinutesCompact(stats.TotalWorkTime))))
		b.WriteString("\n")

		b.WriteString(dim.Render("  [b]reak [s]kip [c]lose"))
	} else {
		b.WriteString(accent.Render(fmt.Sprintf("  %s Break over! ", m.theme.IconApp)))
		b.WriteString("\n")
		b.WriteString(dim.Render("  [s]tart new session [c]lose"))
	}
	b.WriteString("\n")

	return b.String()
}

func formatMinutesCompact(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// Package tui provides the terminal user interface implementation
// using the Bubbletea framework.
package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

// Styles for the TUI.
var (
	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FF6B6B")).
		MarginBottom(1)

	statusStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#4ECDC4"))

	taskStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFE66D"))

	timeStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF"))

	helpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#95A5A6"))
)

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
	fetchState           func() *domain.CurrentState
	commandCallback      func(ports.TimerCommand) error
	onSessionComplete    func(domain.SessionType)
	completionInfo       *domain.CompletionInfo
}

// NewModel creates a new TUI model.
func NewModel(initialState *domain.CurrentState, info *domain.CompletionInfo) Model {
	return Model{
		state:          initialState,
		progress:       progress.New(progress.WithDefaultGradient()),
		completionInfo: info,
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

// Update handles messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "s":
			if m.completed && m.commandCallback != nil {
				m.commandCallback(ports.CmdStart)
				m.completed = false
				m.notified = false
				m.confirmBreak = false
			}
		case "p":
			if m.commandCallback != nil && m.state.ActiveSession != nil {
				if m.state.ActiveSession.Status == domain.SessionStatusRunning {
					m.commandCallback(ports.CmdPause)
				} else {
					m.commandCallback(ports.CmdResume)
				}
			}
			m.confirmBreak = false
		case "b":
			if m.completed && m.completedSessionType == domain.SessionTypeWork {
				if m.commandCallback != nil {
					m.commandCallback(ports.CmdBreak)
					m.completed = false
					m.notified = false
				}
			} else if !m.completed && m.state.ActiveSession != nil {
				if m.confirmBreak {
					if m.commandCallback != nil {
						m.commandCallback(ports.CmdBreak)
						m.completed = false
						m.notified = false
						m.confirmBreak = false
					}
				} else {
					m.confirmBreak = true
				}
			}
		case "x":
			if m.commandCallback != nil {
				m.commandCallback(ports.CmdStop)
			}
			m.confirmBreak = false
			return m, tea.Quit
		default:
			m.confirmBreak = false
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = msg.Width - 4

	case tickMsg:
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

// View renders the TUI.
func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var sections []string

	// Title
	sections = append(sections, titleStyle.Render("🍅 Flow - Pomodoro Timer"))

	// Active task
	if m.state.ActiveTask != nil {
		taskText := fmt.Sprintf("📋 Task: %s", m.state.ActiveTask.Title)
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
		sections = append(sections, statusStyle.Render("No active session"))
		sections = append(sections, "")
		sections = append(sections, helpStyle.Render("[q]uit"))
	}

	content := lipgloss.JoinVertical(lipgloss.Center, sections...)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m Model) viewWorkComplete(sections []string) []string {
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
	statsText := fmt.Sprintf("📊 Today: %d work sessions, %d breaks, %s worked",
		stats.WorkSessions, stats.BreaksTaken, formatDuration(stats.TotalWorkTime))
	sections = append(sections, "")
	sections = append(sections, helpStyle.Render(statsText))

	sections = append(sections, "")
	sections = append(sections, helpStyle.Render("[b]reak  [s]kip  [q]uit"))
	sections = append(sections, "")
	sections = append(sections, helpStyle.Render("Customize durations in ~/.flow/config.toml"))
	return sections
}

func (m Model) viewBreakComplete(sections []string) []string {
	sections = append(sections, "")
	sections = append(sections, statusStyle.Render("Break over! Ready to focus?"))
	sections = append(sections, m.progress.ViewAs(1.0))

	// Daily stats
	stats := m.state.TodayStats
	statsText := fmt.Sprintf("📊 Today: %d work sessions, %d breaks, %s worked",
		stats.WorkSessions, stats.BreaksTaken, formatDuration(stats.TotalWorkTime))
	sections = append(sections, "")
	sections = append(sections, helpStyle.Render(statsText))

	sections = append(sections, "")
	sections = append(sections, helpStyle.Render("[s]tart new session  [q]uit"))
	return sections
}

func (m Model) viewActiveSession(sections []string) []string {
	session := m.state.ActiveSession

	// Session type and status
	statusText := fmt.Sprintf("Status: %s (%s)",
		domain.GetSessionTypeLabel(session.Type),
		domain.GetStatusLabel(session.Status))
	sections = append(sections, statusStyle.Render(statusText))

	// Timer
	remaining := session.RemainingTime()
	timeText := formatDuration(remaining)
	sections = append(sections, timeStyle.Render(timeText))

	// Progress bar
	prog := session.Progress()
	sections = append(sections, m.progress.ViewAs(prog))

	// Git context
	if session.GitBranch != "" {
		commitShort := session.GitCommit
		if len(commitShort) > 7 {
			commitShort = commitShort[:7]
		}
		gitInfo := fmt.Sprintf("🌿 %s (%s)", session.GitBranch, commitShort)
		sections = append(sections, helpStyle.Render(gitInfo))
	}

	// Daily stats
	stats := m.state.TodayStats
	statsText := fmt.Sprintf("📊 Today: %d work sessions, %d breaks, %s worked",
		stats.WorkSessions, stats.BreaksTaken, formatDuration(stats.TotalWorkTime))
	sections = append(sections, "")
	sections = append(sections, helpStyle.Render(statsText))

	// Help
	sections = append(sections, "")
	if m.confirmBreak {
		sections = append(sections, helpStyle.Render("End session and start break? Press [b] again to confirm"))
	} else {
		sections = append(sections, helpStyle.Render("[p]ause  [x] stop  [b]reak  [q]uit"))
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

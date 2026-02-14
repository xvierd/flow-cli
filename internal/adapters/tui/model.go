// Package tui provides the terminal user interface implementation
// using the Bubbletea framework.
package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dvidx/flow-cli/internal/domain"
	"github.com/dvidx/flow-cli/internal/ports"
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

// Model represents the TUI state.
type Model struct {
	state         *domain.CurrentState
	progress      progress.Model
	width         int
	height        int
	commandChan   chan ports.TimerCommand
	updateCallback func()
}

// NewModel creates a new TUI model.
func NewModel(initialState *domain.CurrentState) Model {
	return Model{
		state:        initialState,
		progress:     progress.New(progress.WithDefaultGradient()),
		commandChan:  make(chan ports.TimerCommand, 10),
	}
}

// Init initializes the TUI.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		listenForCommands(m.commandChan),
	)
}

// Update handles messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "s":
			m.commandChan <- ports.CmdStart
		case "p":
			if m.state.ActiveSession != nil {
				if m.state.ActiveSession.Status == domain.SessionStatusRunning {
					m.commandChan <- ports.CmdPause
				} else {
					m.commandChan <- ports.CmdResume
				}
			}
		case "c":
			m.commandChan <- ports.CmdCancel
		case "b":
			m.commandChan <- ports.CmdBreak
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = msg.Width - 4

	case tickMsg:
		if m.updateCallback != nil {
			m.updateCallback()
		}
		return m, tickCmd()

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

	// Session status
	if m.state.ActiveSession != nil {
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
			gitInfo := fmt.Sprintf("🌿 %s (%s)", session.GitBranch, session.GitCommit[:7])
			sections = append(sections, helpStyle.Render(gitInfo))
		}
	} else {
		sections = append(sections, statusStyle.Render("No active session"))
	}

	// Daily stats
	stats := m.state.TodayStats
	statsText := fmt.Sprintf("📊 Today: %d work sessions, %d breaks, %s worked",
		stats.WorkSessions, stats.BreaksTaken, formatDuration(stats.TotalWorkTime))
	sections = append(sections, "")
	sections = append(sections, helpStyle.Render(statsText))

	// Help
	sections = append(sections, "")
	sections = append(sections, helpStyle.Render("[s]tart [p]ause [c]ancel [b]reak [q]uit"))

	return lipgloss.JoinVertical(lipgloss.Center, sections...)
}

// tickCmd creates a command that sends a tick message.
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// listenForCommands listens for commands from the command channel.
func listenForCommands(cmdChan chan ports.TimerCommand) tea.Cmd {
	return func() tea.Msg {
		// This will block until a command is sent
		// The command is processed in Update
		return nil
	}
}

// formatDuration formats a duration as MM:SS.
func formatDuration(d time.Duration) string {
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

// SetUpdateCallback sets the callback for state updates.
func (m *Model) SetUpdateCallback(callback func()) {
	m.updateCallback = callback
}

// SendStateUpdate sends a state update to the model.
func (m *Model) SendStateUpdate(state *domain.CurrentState) {
	// This is handled by the Bubbletea program
}

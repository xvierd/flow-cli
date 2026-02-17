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
	"github.com/xvierd/flow-cli/internal/methodology"
	"github.com/xvierd/flow-cli/internal/ports"
)

// inlinePhase tracks which phase the inline UI is in.
type inlinePhase int

const (
	phasePickMode inlinePhase = iota
	phasePickDuration
	phaseTaskName
	phaseTimer
)

// modeOption describes a methodology choice in the mode picker.
type modeOption struct {
	methodology domain.Methodology
	label       string
	desc        string
}

var modeOptions = []modeOption{
	{domain.MethodologyPomodoro, "Pomodoro", "Classic 25/5 timer"},
	{domain.MethodologyDeepWork, "Deep Work", "Longer sessions, distraction tracking"},
	{domain.MethodologyMakeTime, "Make Time", "Daily Highlight, focus scoring"},
}

// InlineModel is a compact timer that runs setup + timer in a single bubbletea program.
type InlineModel struct {
	// Phase
	phase inlinePhase

	// Mode picker
	modeCursor     int
	modeLocked     bool
	onModeSelected func(domain.Methodology)

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
	distractionCallback    func(string) error
	accomplishmentCallback func(string) error
	focusScoreCallback     func(int) error
	completionInfo    *domain.CompletionInfo
	theme             config.ThemeConfig

	// Callbacks for session creation (called during setup phase)
	onStartSession func(presetIndex int, taskName string) error

	// Methodology mode
	mode methodology.Mode

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

	di := textinput.New()
	di.Placeholder = "What distracted you?"
	di.CharLimit = 200
	di.Width = w - 10

	ai := textinput.New()
	ai.Placeholder = "What did you accomplish?"
	ai.CharLimit = 200
	ai.Width = w - 10

	// If there's already an active session, skip setup
	startPhase := phasePickDuration
	if state.ActiveSession != nil {
		startPhase = phaseTimer
	}

	return InlineModel{
		phase:               startPhase,
		state:               state,
		progress:            pbar,
		width:               w,
		completionInfo:      info,
		theme:               resolved,
		taskInput:           ti,
		distractionInput:    di,
		accomplishmentInput: ai,
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
	case phasePickMode:
		return m.updatePickMode(msg)
	case phasePickDuration:
		return m.updatePickDuration(msg)
	case phaseTaskName:
		return m.updateTaskName(msg)
	case phaseTimer:
		if m.distractionMode {
			return m.updateDistractionInput(msg)
		}
		if m.accomplishmentMode {
			return m.updateAccomplishmentInput(msg)
		}
		return m.updateTimer(msg)
	}
	return m, nil
}

func (m InlineModel) updatePickMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h":
			if m.modeCursor > 0 {
				m.modeCursor--
			}
		case "right", "l":
			if m.modeCursor < len(modeOptions)-1 {
				m.modeCursor++
			}
		case "1":
			m.modeCursor = 0
			return m.selectMode()
		case "2":
			if len(modeOptions) > 1 {
				m.modeCursor = 1
				return m.selectMode()
			}
		case "3":
			if len(modeOptions) > 2 {
				m.modeCursor = 2
				return m.selectMode()
			}
		case "enter":
			return m.selectMode()
		case "c", "ctrl+c", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m InlineModel) selectMode() (tea.Model, tea.Cmd) {
	selected := modeOptions[m.modeCursor]
	m.mode = methodology.ForMethodology(selected.methodology)
	m.presets = m.mode.Presets()

	if m.onModeSelected != nil {
		m.onModeSelected(selected.methodology)
	}

	m.phase = phasePickDuration
	m.presetCursor = 0
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
		case "esc":
			if !m.modeLocked {
				m.phase = phasePickMode
				return m, nil
			}
		case "c", "ctrl+c":
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
			if m.onStartSession != nil {
				taskName := strings.TrimSpace(m.taskInput.Value())
				if err := m.onStartSession(m.presetCursor, taskName); err != nil {
					m.phase = phaseTimer
					return m, tickCmd()
				}
			}
			m.phase = phaseTimer
			return m, tickCmd()
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.phase = phasePickDuration
			m.taskInput.Blur()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.taskInput, cmd = m.taskInput.Update(msg)
	return m, cmd
}

func (m InlineModel) updateDistractionInput(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			return m, tickCmd()
		case "esc":
			m.distractionMode = false
			m.distractionInput.Blur()
			return m, tickCmd()
		case "ctrl+c":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.distractionInput, cmd = m.distractionInput.Update(msg)
	return m, cmd
}

func (m InlineModel) updateAccomplishmentInput(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.mode != nil && m.mode.HasDistractionLog() && !m.completed && m.state.ActiveSession != nil && m.state.ActiveSession.Type == domain.SessionTypeWork {
				m.distractionMode = true
				m.distractionInput.Reset()
				m.distractionInput.Focus()
				return m, m.distractionInput.Cursor.BlinkCmd()
			}
		case "a":
			if m.mode != nil && m.mode.HasShutdownRitual() && m.completed && m.completedType == domain.SessionTypeWork && !m.accomplishmentSaved {
				m.accomplishmentMode = true
				m.accomplishmentInput.Reset()
				m.accomplishmentInput.Focus()
				return m, m.accomplishmentInput.Cursor.BlinkCmd()
			}
		case "1", "2", "3", "4", "5":
			if m.mode != nil && m.mode.HasFocusScore() && m.completed && m.completedType == domain.SessionTypeWork && !m.focusScoreSaved {
				score := int(msg.String()[0] - '0')
				m.focusScore = &score
				m.focusScoreSaved = true
				if m.focusScoreCallback != nil {
					_ = m.focusScoreCallback(score)
				}
			}
		case "b":
			if m.completed && m.completedType == domain.SessionTypeWork {
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

	case tickMsg:
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
				m.energizeShown = false
				m.energizeTicks = 0
				m.resetCompletionState()
			}
			m.state = msg.state
		}

	case *domain.CurrentState:
		m.state = msg
	}

	return m, nil
}

func (m *InlineModel) resetCompletionState() {
	m.accomplishmentSaved = false
	m.focusScore = nil
	m.focusScoreSaved = false
	m.distractions = nil
}

// --- View ---

func (m InlineModel) View() string {
	switch m.phase {
	case phasePickMode:
		return m.viewPickMode()
	case phasePickDuration:
		return m.viewPickDuration()
	case phaseTaskName:
		return m.viewTaskName()
	case phaseTimer:
		return m.viewTimer()
	}
	return ""
}

func (m InlineModel) viewPickMode() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.ColorTitle))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorWork)).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	b.WriteString(titleStyle.Render("  Mode:") + "  ")

	for i, opt := range modeOptions {
		label := fmt.Sprintf("%s", opt.label)
		if i == m.modeCursor {
			b.WriteString(activeStyle.Render(" ▸ "+label+" "))
		} else {
			b.WriteString(dimStyle.Render("   "+label+" "))
		}
	}
	b.WriteString("\n")

	// Show description of selected mode
	desc := modeOptions[m.modeCursor].desc
	b.WriteString(dimStyle.Render("  "+desc) + "\n")

	b.WriteString(dimStyle.Render("  ←/→ select · enter confirm · c close") + "\n")

	return b.String()
}

func (m InlineModel) viewPickDuration() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.ColorTitle))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorWork)).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	// Show selected mode
	if m.mode != nil {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  %s mode", m.mode.Name().Label())) + "\n")
	}

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

	escHint := "esc back · "
	if m.modeLocked {
		escHint = ""
	}
	b.WriteString(dimStyle.Render("  ←/→ select · enter confirm · "+escHint+"c close") + "\n")

	return b.String()
}

func (m InlineModel) viewTaskName() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.ColorTitle))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorWork)).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	p := m.presets[m.presetCursor]
	b.WriteString(activeStyle.Render(fmt.Sprintf("  ▸ %s %s", p.Name, formatMinutesCompact(p.Duration))))
	b.WriteString("\n")

	taskPrompt := "Task:"
	if m.mode != nil {
		taskPrompt = m.mode.TaskPrompt()
	}
	b.WriteString(titleStyle.Render("  "+taskPrompt+" "))
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

	if session.Status == domain.SessionStatusPaused {
		b.WriteString(pausedStyle.Render(fmt.Sprintf("  %s %s  %s  %s PAUSED",
			m.theme.IconApp, typeLabel, timeStr, m.theme.IconPaused)))
	} else {
		b.WriteString(accent.Render(fmt.Sprintf("  %s %s  %s", m.theme.IconApp, typeLabel, timeStr)))
	}

	if m.state.ActiveTask != nil {
		b.WriteString(dim.Render(fmt.Sprintf("  %s %s", m.theme.IconTask, m.state.ActiveTask.Title)))
	}
	b.WriteString("\n")

	// Make Time: energize reminder
	if m.energizeTicks > 0 {
		reminderStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.ColorTask))
		b.WriteString(reminderStyle.Render("  Quick stretch? Take a moment to energize."))
		b.WriteString("\n")
	}

	// Text input overlays
	if m.distractionMode {
		b.WriteString(dim.Render("  Distraction: ") + m.distractionInput.View())
		b.WriteString("\n")
		b.WriteString(dim.Render("  enter save · esc cancel"))
		b.WriteString("\n")
		return b.String()
	}
	if m.accomplishmentMode {
		b.WriteString(dim.Render("  Accomplishment: ") + m.accomplishmentInput.View())
		b.WriteString("\n")
		b.WriteString(dim.Render("  enter save · esc cancel"))
		b.WriteString("\n")
		return b.String()
	}

	// Progress bar
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

	// Help
	if m.confirmFinish {
		b.WriteString(dim.Render("  Stop session? Press [f] again to confirm"))
	} else if m.confirmBreak {
		b.WriteString(dim.Render("  Press [b] again to confirm break"))
	} else if session.IsBreakSession() {
		b.WriteString(dim.Render("  [s]kip [p]ause [f]inish [c]lose"))
	} else {
		helpText := "  [p]ause [f]inish [b]reak [c]lose"
		if m.mode != nil && m.mode.HasDistractionLog() {
			if len(m.distractions) > 0 {
				helpText = fmt.Sprintf("  [p]ause [d]istraction(%d) [f]inish [b]reak [c]lose", len(m.distractions))
			} else {
				helpText = "  [p]ause [d]istraction [f]inish [b]reak [c]lose"
			}
		}
		b.WriteString(dim.Render(helpText))
	}
	b.WriteString("\n")

	return b.String()
}

func (m InlineModel) viewInlineComplete(accent, dim lipgloss.Style) string {
	if m.completedType == domain.SessionTypeWork {
		// Branch by mode
		if m.mode != nil && m.mode.HasShutdownRitual() {
			return m.viewInlineDeepWorkComplete(accent, dim)
		}
		if m.mode != nil && m.mode.HasFocusScore() {
			return m.viewInlineMakeTimeComplete(accent, dim)
		}
		return m.viewInlineDefaultComplete(accent, dim)
	}

	// Break complete
	var b strings.Builder
	b.WriteString(accent.Render(fmt.Sprintf("  %s Break over! ", m.theme.IconApp)))
	b.WriteString("\n")
	b.WriteString(dim.Render("  [s]tart new session [c]lose"))
	b.WriteString("\n")
	return b.String()
}

func (m InlineModel) viewInlineDefaultComplete(accent, dim lipgloss.Style) string {
	var b strings.Builder
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
	b.WriteString("\n")
	return b.String()
}

func (m InlineModel) viewInlineDeepWorkComplete(accent, dim lipgloss.Style) string {
	var b strings.Builder
	b.WriteString(accent.Render(fmt.Sprintf("  %s Deep Work Session Complete.", m.theme.IconApp)))
	b.WriteString("\n")

	if len(m.distractions) > 0 {
		b.WriteString(dim.Render(fmt.Sprintf("  Distractions: %d", len(m.distractions))))
		b.WriteString("\n")
	}

	stats := m.state.TodayStats
	deepWorkHours := stats.TotalWorkTime.Hours()
	b.WriteString(dim.Render(fmt.Sprintf("  Deep Work: %s today (%.0f%% of 8h)",
		formatMinutesCompact(stats.TotalWorkTime), deepWorkHours/8.0*100)))
	b.WriteString("\n")

	if m.accomplishmentMode {
		b.WriteString(dim.Render("  Accomplishment: ") + m.accomplishmentInput.View())
		b.WriteString("\n")
		b.WriteString(dim.Render("  enter save · esc cancel"))
	} else if m.accomplishmentSaved {
		b.WriteString(accent.Render("  Shutdown ritual complete."))
	} else {
		b.WriteString(dim.Render("  [a]ccomplishment [b]reak [s]kip [c]lose"))
	}
	b.WriteString("\n")

	if !m.accomplishmentMode {
		helpText := "  [b]reak [s]kip [c]lose"
		if !m.accomplishmentSaved {
			helpText = "  [a]ccomplishment [b]reak [s]kip [c]lose"
		}
		b.WriteString(dim.Render(helpText))
		b.WriteString("\n")
	}
	return b.String()
}

func (m InlineModel) viewInlineMakeTimeComplete(accent, dim lipgloss.Style) string {
	var b strings.Builder
	b.WriteString(accent.Render(fmt.Sprintf("  %s Session complete!", m.theme.IconApp)))
	b.WriteString("\n")

	if m.state.ActiveTask != nil && m.state.ActiveTask.HighlightDate != nil {
		b.WriteString(accent.Render("  You made time for your Highlight today."))
		b.WriteString("\n")
	}

	if m.focusScoreSaved && m.focusScore != nil {
		b.WriteString(dim.Render(fmt.Sprintf("  Focus score: %d/5", *m.focusScore)))
	} else {
		b.WriteString(dim.Render("  How focused? [1] [2] [3] [4] [5]"))
	}
	b.WriteString("\n")

	stats := m.state.TodayStats
	b.WriteString(dim.Render(fmt.Sprintf("  %s %d sessions, %s worked today",
		m.theme.IconStats, stats.WorkSessions, formatMinutesCompact(stats.TotalWorkTime))))
	b.WriteString("\n")

	b.WriteString(dim.Render("  [b]reak [s]kip [c]lose"))
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

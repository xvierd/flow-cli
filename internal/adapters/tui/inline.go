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
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/methodology"
	"github.com/xvierd/flow-cli/internal/ports"
)

// InlineModel is a compact timer that runs setup + timer in a single bubbletea program.
type InlineModel struct {
	// Phase
	phase inlinePhase

	// Main menu
	menuCursor     int
	SelectedAction MainMenuAction

	// Mode picker
	modeCursor     int
	modeLocked     bool
	onModeSelected func(domain.Methodology)

	// Setup: duration picker
	presets      []config.SessionPreset
	presetCursor int
	breakInfo    string

	// Setup: task select (recent tasks)
	recentTasks      []*domain.Task
	taskSelectCursor int
	fetchRecentTasks func(limit int) []*domain.Task

	// Make Time: highlight carry-over
	yesterdayHighlight      *domain.Task
	fetchYesterdayHighlight func() *domain.Task

	// Setup: task name
	taskInput textinput.Model

	// Setup: laser checklist (Make Time only)
	laserChecklistCursor int
	laserChecklist       [3]bool // Phone DND, Notifications off, Tabs closed
	laserChecklistDone   bool

	// Setup: intended outcome (Deep Work only)
	outcomeInput    textinput.Model
	intendedOutcome string

	// Chaining: remembers the previous session's task for a frictionless repeat
	lastTaskTitle string

	// Timer state
	state                   *domain.CurrentState
	progress                progress.Model
	width                   int
	completedType           domain.SessionType
	completedElapsed        time.Duration // actual time worked, captured at session end
	notified                bool
	confirmMode             bool
	fetchState              func() *domain.CurrentState
	commandCallback         func(ports.TimerCommand) error
	onSessionComplete       func(domain.SessionType)
	distractionCallback     func(string, string) error
	accomplishmentCallback  func(sessionID string, text string) error
	focusScoreCallback      func(sessionID string, score int) error
	energizeCallback        func(sessionID string, activity string) error
	outcomeAchievedCallback func(sessionID string, achieved string) error
	completionInfo          *domain.CompletionInfo
	theme                   config.ThemeConfig

	// Callbacks for session creation (called during setup phase)
	onStartSession func(presetIndex int, taskName string, intendedOutcome string) error

	// Methodology mode
	mode           methodology.Mode
	onboardingMode bool

	// completionState holds all mode-specific fields shared with Model.
	completionState

	// Notifications
	notificationsEnabled bool
	notificationToggle   func(bool)

	// Daily summary on quit
	showingSummary bool
	summaryTicks   int
}

// getTerminalWidth returns the current terminal width, defaulting to 80.
func getTerminalWidth() int {
	w, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || w < 40 {
		return 80
	}
	return w
}

// setStrictLocked records a blocked-key notice and clears any confirm states,
// including the inline-only mode-switch confirm.
func (m *InlineModel) setStrictLocked() {
	m.completionState.setStrictLocked()
	m.confirmMode = false
}

// NewInlineModel creates a new inline TUI model starting in the setup phase.
func NewInlineModel(state *domain.CurrentState, info *domain.CompletionInfo, theme *config.ThemeConfig) InlineModel {
	resolved := resolveTheme(theme)
	w := getTerminalWidth()
	pbar := progress.New(progress.WithGradient(resolved.WorkGradientStart, resolved.WorkGradientEnd))
	pbar.Width = w - 16

	ti := textinput.New()
	ti.Placeholder = i18n.T("Enter to skip")
	ti.CharLimit = 120
	ti.Width = w - 10

	oi := textinput.New()
	oi.Placeholder = i18n.T("Enter to skip")
	oi.CharLimit = 200
	oi.Width = w - 10

	di, ai, shutdownInputs := newCompletionInputs(w - 10)

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
		outcomeInput:   oi,
		completionState: completionState{
			distractionInput:    di,
			accomplishmentInput: ai,
			shutdownInputs:      shutdownInputs,
		},
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
	case phaseWelcome:
		return m.updateWelcome(msg)
	case phaseMainMenu:
		return m.updateMainMenu(msg)
	case phasePickMode:
		return m.updatePickMode(msg)
	case phasePickDuration:
		return m.updatePickDuration(msg)
	case phaseLaserChecklist:
		return m.updateLaserChecklist(msg)
	case phaseTaskSelect:
		return m.updateTaskSelect(msg)
	case phaseTaskName:
		return m.updateTaskName(msg)
	case phaseOutcome:
		return m.updatePickOutcome(msg)
	case phaseTimer:
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
		if m.outcomeReviewMode {
			return m.updateOutcomeReview(msg)
		}
		return m.updateTimer(msg)
	}
	return m, nil
}

func (m InlineModel) updateDistractionInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	cb := &completionCallbacks{
		distractionCallback: m.distractionCallback,
		mode:                m.mode,
	}
	return m, handleDistractionInput(&m.completionState, cb, msg, tickCmd())
}

func (m InlineModel) updateAccomplishmentInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	cb := &completionCallbacks{
		accomplishmentCallback: m.accomplishmentCallback,
	}
	return m, handleAccomplishmentInput(&m.completionState, cb, msg)
}

func (m InlineModel) updateShutdownRitual(msg tea.Msg) (tea.Model, tea.Cmd) {
	cb := &completionCallbacks{
		shutdownRitualCallback: m.shutdownRitualCallback,
	}
	return m, handleShutdownRitual(&m.completionState, cb, msg)
}

func (m InlineModel) updateDistractionReview(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, handleDistractionReview(&m.completionState, msg)
}

func (m InlineModel) updateOutcomeReview(msg tea.Msg) (tea.Model, tea.Cmd) {
	cb := &completionCallbacks{
		outcomeAchievedCallback: m.outcomeAchievedCallback,
	}
	return m, handleOutcomeReview(&m.completionState, cb, msg)
}

func (m InlineModel) showDailySummaryOrQuit() (tea.Model, tea.Cmd) {
	if m.state.TodayStats.WorkSessions > 0 {
		m.showingSummary = true
		m.summaryTicks = 3
		return m, tickCmd()
	}
	return m, tea.Quit
}

func (m InlineModel) updateTimer(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Daily summary dismiss
	if m.showingSummary {
		switch msg.(type) {
		case tea.KeyMsg:
			return m, tea.Quit
		case tickMsg:
			m.summaryTicks--
			if m.summaryTicks <= 0 {
				return m, tea.Quit
			}
			return m, tickCmd()
		}
		return m, nil
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
			if m.completed {
				// Only allow starting a new session once completion prompts are satisfied
				if !m.completionPromptsComplete() {
					return m, nil
				}
				if m.commandCallback != nil {
					_ = m.commandCallback(ports.CmdStart)
					m.completed = false
					m.notified = false
					m.confirmBreak = false
					m.resetCompletionState()
				}
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
			if m.strictWorkBlocked(m.state) && m.state.ActiveSession.Status == domain.SessionStatusRunning {
				m.setStrictLocked()
				return m, nil
			}
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
		case "r":
			if m.mode != nil && m.mode.HasShutdownRitual() && m.completed && m.completedType == domain.SessionTypeWork && len(m.distractions) > 0 && !m.distractionReviewDone {
				m.distractionReviewMode = true
			}
		case "a":
			if m.mode != nil && m.mode.HasShutdownRitual() && m.completed && m.completedType == domain.SessionTypeWork && !m.shutdownComplete && !m.accomplishmentSaved {
				m.shutdownRitualMode = true
				m.shutdownStep = 0
				for i := range m.shutdownInputs {
					m.shutdownInputs[i].Reset()
				}
				m.shutdownInputs[0].Focus()
				return m, m.shutdownInputs[0].Cursor.BlinkCmd()
			}
		case "o":
			// Deep Work: outcome review
			if m.mode != nil && m.mode.HasShutdownRitual() && m.completed && m.completedType == domain.SessionTypeWork && m.completedIntendedOutcome != "" && !m.outcomeReviewDone {
				m.outcomeReviewMode = true
			}
		case "1", "2", "3", "4", "5":
			if m.mode != nil && m.mode.HasFocusScore() && m.completed && m.completedType == domain.SessionTypeWork && !m.focusScoreSaved {
				score := int(msg.String()[0] - '0')
				m.focusScore = &score
				m.focusScoreSaved = true
				if m.focusScoreCallback != nil {
					_ = m.focusScoreCallback(m.completedSessionID, score)
				}
			}
		case "w":
			if m.mode != nil && m.mode.HasEnergizeReminder() && m.completed && m.completedType == domain.SessionTypeWork && m.focusScoreSaved && !m.energizeSaved {
				m.energizeActivity = "walk"
				m.energizeSaved = true
				if m.energizeCallback != nil {
					_ = m.energizeCallback(m.completedSessionID, "walk")
				}
			}
		case "t":
			if m.mode != nil && m.mode.HasEnergizeReminder() && m.completed && m.completedType == domain.SessionTypeWork && m.focusScoreSaved && !m.energizeSaved {
				m.energizeActivity = "stretch"
				m.energizeSaved = true
				if m.energizeCallback != nil {
					_ = m.energizeCallback(m.completedSessionID, "stretch")
				}
			}
		case "e":
			if m.mode != nil && m.mode.HasEnergizeReminder() && m.completed && m.completedType == domain.SessionTypeWork && m.focusScoreSaved && !m.energizeSaved {
				m.energizeActivity = "exercise"
				m.energizeSaved = true
				if m.energizeCallback != nil {
					_ = m.energizeCallback(m.completedSessionID, "exercise")
				}
			}
		case "n":
			// Make Time: energize "none" option
			if m.mode != nil && m.mode.HasEnergizeReminder() && m.completed && m.completedType == domain.SessionTypeWork && m.focusScoreSaved && !m.energizeSaved {
				m.energizeActivity = "none"
				m.energizeSaved = true
				if m.energizeCallback != nil {
					_ = m.energizeCallback(m.completedSessionID, "none")
				}
				return m, nil
			}
			// Session chaining: start new session with the same methodology and last preset
			if m.completed && m.completionPromptsComplete() {
				m.phase = phasePickDuration
				m.completed = false
				m.notified = false
				m.resetCompletionState()
				return m, nil
			}
		case "b":
			if m.strictWorkBlocked(m.state) {
				m.setStrictLocked()
				return m, nil
			}
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
			if m.strictWorkBlocked(m.state) {
				m.setStrictLocked()
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
		case "v":
			if m.strictWorkBlocked(m.state) {
				m.setStrictLocked()
				return m, nil
			}
			// Void (interrupt) the current session — not counted in stats
			if !m.completed && m.state.ActiveSession != nil && m.state.ActiveSession.Type == domain.SessionTypeWork && m.commandCallback != nil {
				_ = m.commandCallback(ports.CmdVoid)
				m.completed = false
				m.notified = false
				m.confirmBreak = false
				m.confirmFinish = false
				m.resetCompletionState()
			}
		case "m":
			// Strict focus: cancel via mode switch is locked during a work session.
			if m.strictWorkBlocked(m.state) {
				m.setStrictLocked()
				return m, nil
			}
			// Switch mode. If a session is active, confirm before cancelling it.
			if !m.completed && m.state.ActiveSession != nil {
				if m.confirmMode {
					m.confirmMode = false
					m.confirmFinish = false
					m.confirmBreak = false
					if m.commandCallback != nil {
						_ = m.commandCallback(ports.CmdCancel)
					}
					m.completed = false
					m.notified = false
					m.resetCompletionState()
					m.phase = phasePickMode
					m.modeLocked = false
					return m, nil
				}
				m.confirmMode = true
				m.confirmFinish = false
				m.confirmBreak = false
				return m, nil
			}
			// No active session: switch mode immediately
			m.confirmMode = false
			m.confirmFinish = false
			m.confirmBreak = false
			m.completed = false
			m.notified = false
			m.resetCompletionState()
			m.phase = phasePickMode
			m.modeLocked = false
			return m, nil
		default:
			m.confirmBreak = false
			m.confirmFinish = false
			m.confirmMode = false
			m.strictLocked = false
		}

	case tickMsg:
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
			if m.state.ActiveSession != nil && msg.state.ActiveSession == nil {
				m.completedSessionID = m.state.ActiveSession.ID
				m.completedType = m.state.ActiveSession.Type
				m.completedElapsed = m.state.ActiveSession.Duration
				m.completedIntendedOutcome = m.state.ActiveSession.IntendedOutcome
				if m.state.ActiveTask != nil {
					m.lastTaskTitle = m.state.ActiveTask.Title
				}
				m.completed = true
				if !m.notified && m.onSessionComplete != nil {
					m.onSessionComplete(m.completedType)
					m.notified = true
				}

				// Start auto-break countdown for work sessions
				if m.autoBreak && m.completedType == domain.SessionTypeWork {
					m.autoBreakTicks = 3
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
	m.completedElapsed = 0
	m.reset()
}

// completionPromptsComplete returns true when all mode-specific completion prompts are done.
// Delegates to the shared completionState logic so Model and InlineModel stay in sync.
func (m InlineModel) completionPromptsComplete() bool {
	return m.promptsDone(m.mode, m.completedType)
}

// --- View ---

func (m InlineModel) View() string {
	switch m.phase {
	case phaseWelcome:
		return m.viewWelcome()
	case phaseMainMenu:
		return m.viewMainMenu()
	case phasePickMode:
		return m.viewPickMode()
	case phasePickDuration:
		return m.viewPickDuration()
	case phaseLaserChecklist:
		return m.viewLaserChecklist()
	case phaseTaskSelect:
		return m.viewTaskSelect()
	case phaseTaskName:
		return m.viewTaskName()
	case phaseOutcome:
		return m.viewPickOutcome()
	case phaseTimer:
		return m.viewTimer()
	}
	return ""
}

func (m InlineModel) viewTimer() string {
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorWork)).Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))
	pausedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorPaused)).Bold(true)

	if m.showingSummary {
		return m.viewDailySummary(accent, dim)
	}

	if m.completed {
		return m.viewInlineComplete(accent, dim)
	}

	if m.state.ActiveSession == nil {
		var b strings.Builder
		if m.mode != nil {
			b.WriteString(accent.Render(fmt.Sprintf("  %s", m.mode.TUITitle())))
			b.WriteString("\n")
			info := viewIdleMethodologyInfo(m.state, m.mode, m.completionInfo)
			if info != "" {
				b.WriteString(dim.Render(info))
				b.WriteString("\n")
			}
		} else {
			b.WriteString(dim.Render("  " + i18n.T("No active session")))
			b.WriteString("\n")
		}
		b.WriteString(dim.Render("  " + i18n.T("[s]tart  [m]ode  [c]lose")))
		b.WriteString("\n")
		return b.String()
	}

	return m.viewInlineActive(accent, dim, pausedStyle)
}

func (m InlineModel) viewDailySummary(accent, dim lipgloss.Style) string {
	var b strings.Builder
	stats := m.state.TodayStats

	b.WriteString(accent.Render(fmt.Sprintf("  %s %s", m.theme.IconApp, i18n.T("Today's Summary"))))
	b.WriteString("\n")
	b.WriteString(dim.Render(fmt.Sprintf("  %s %s",
		m.theme.IconStats, i18n.T("%d sessions, %s focused", stats.WorkSessions, formatMinutesCompact(stats.TotalWorkTime)))))
	if stats.BreaksTaken > 0 {
		b.WriteString(dim.Render(i18n.T(", %d breaks", stats.BreaksTaken)))
	}
	b.WriteString("\n")

	b.WriteString(dim.Render("  " + i18n.T("Press any key to exit")))
	b.WriteString("\n")

	return b.String()
}

func (m InlineModel) viewInlineActive(accent, dim, pausedStyle lipgloss.Style) string {
	session := m.state.ActiveSession
	remaining := session.RemainingTime()
	timeStr := formatDuration(remaining)
	prog := session.Progress()
	typeLabel := domain.GetSessionTypeLabel(session.Type)

	// Show methodology name for work sessions, session type for breaks
	displayLabel := typeLabel
	if m.mode != nil && session.IsWorkSession() {
		displayLabel = m.mode.TUITitle()
	}

	var b strings.Builder

	if session.Status == domain.SessionStatusPaused {
		b.WriteString(pausedStyle.Render(fmt.Sprintf("  %s %s  %s  %s %s",
			m.theme.IconApp, displayLabel, timeStr, m.theme.IconPaused, i18n.T("PAUSED"))))
	} else {
		b.WriteString(accent.Render(fmt.Sprintf("  %s %s  %s", m.theme.IconApp, displayLabel, timeStr)))
	}
	if m.strict && session.IsWorkSession() {
		b.WriteString(accent.Render("  " + i18n.T("🔒 STRICT")))
	}

	if m.state.ActiveTask != nil {
		b.WriteString(dim.Render(fmt.Sprintf("  %s %s", m.theme.IconTask, m.state.ActiveTask.Title)))
	}
	if session.IntendedOutcome != "" {
		b.WriteString(dim.Italic(true).Render("  " + i18n.T("Goal: %s", session.IntendedOutcome)))
	}
	if len(session.Tags) > 0 {
		tagStr := ""
		for _, t := range session.Tags {
			tagStr += " #" + t
		}
		b.WriteString(dim.Render(tagStr))
	}
	b.WriteString("\n")

	// Make Time: energize reminder
	if m.energizeTicks > 0 {
		reminderStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.ColorTask))
		b.WriteString(reminderStyle.Render("  " + i18n.T("Quick stretch? Take a moment to energize.")))
		b.WriteString("  ")
		b.WriteString(dim.Render(i18n.T("[b]reak")))
		b.WriteString("\n")
	}

	// Text input overlays
	if m.distractionMode {
		if m.distractionCategoryMode {
			b.WriteString(dim.Render("  " + i18n.T("Categorize: %s", m.distractionPendingText)))
			b.WriteString("\n")
			b.WriteString(dim.Render("  " + i18n.T("[i]nternal  [e]xternal  [enter] no category  [esc] cancel")))
			b.WriteString("\n")
		} else {
			b.WriteString(dim.Render("  "+i18n.T("Distraction: ")) + m.distractionInput.View())
			b.WriteString("\n")
			b.WriteString(dim.Render("  " + i18n.T("enter save · esc cancel")))
			b.WriteString("\n")
		}
		return b.String()
	}
	if m.accomplishmentMode {
		b.WriteString(dim.Render("  "+i18n.T("Accomplishment: ")) + m.accomplishmentInput.View())
		b.WriteString("\n")
		b.WriteString(dim.Render("  " + i18n.T("enter save · esc cancel")))
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

	// Notification indicator
	notifLabel := i18n.T("off")
	if m.notificationsEnabled {
		notifLabel = i18n.T("on")
	}

	// Help
	if m.strictLocked {
		b.WriteString(dim.Render("  " + strictLockMessage(true)))
	} else if m.confirmMode {
		b.WriteString(dim.Render("  " + i18n.T("Switch mode? This cancels the current session  [m] confirm  [esc] cancel")))
	} else if m.confirmFinish {
		b.WriteString(dim.Render("  " + i18n.T("Stop session? [f] confirm  [esc] cancel  [m]ode")))
	} else if m.confirmBreak {
		b.WriteString(dim.Render("  " + i18n.T("Start break? [b] confirm  [esc] cancel  [m]ode")))
	} else if session.IsBreakSession() {
		b.WriteString(dim.Render("  " + i18n.T("[s]kip [p]ause [f]inish [m]ode [c]lose  tab:notify %s", notifLabel)))
	} else {
		pauseAction := i18n.T("[p]ause")
		if session.Status == domain.SessionStatusPaused {
			pauseAction = i18n.T("[p]resume")
		}
		helpText := "  " + i18n.T("%s [f]inish [v]oid [b]reak [m]ode [c]lose", pauseAction)
		if m.mode != nil && m.mode.HasDistractionLog() && session.Status == domain.SessionStatusRunning {
			if len(m.distractions) > 0 {
				helpText = "  " + i18n.T("%s [d]istraction(%d) [f]inish [v]oid [b]reak [m]ode [c]lose", pauseAction, len(m.distractions))
			} else {
				helpText = "  " + i18n.T("%s [d]istraction [f]inish [v]oid [b]reak [m]ode [c]lose", pauseAction)
			}
		}
		helpText += i18n.T("  tab:notify %s", notifLabel)
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
	b.WriteString(accent.Render(fmt.Sprintf("  %s %s", m.theme.IconApp, i18n.T("Break over!"))))
	b.WriteString("\n")
	b.WriteString(dim.Render("  " + i18n.T("Start your next session or call it a day.")))
	b.WriteString("\n")
	b.WriteString(dim.Render("  " + i18n.T("[n]ew session  [m]ode  [q]uit")))
	b.WriteString("\n")
	return b.String()
}

func (m InlineModel) viewInlineDefaultComplete(accent, dim lipgloss.Style) string {
	vd := buildCompletionViewData(&m.completionState, m.mode, m.state, m.completionInfo, m.completedElapsed)
	var b strings.Builder

	if vd.elapsed > 0 {
		b.WriteString(accent.Render(fmt.Sprintf("  %s %s", m.theme.IconApp, i18n.T("Session complete — %s worked", formatMinutesCompact(vd.elapsed)))))
	} else {
		b.WriteString(accent.Render(fmt.Sprintf("  %s %s", m.theme.IconApp, i18n.T("Session complete!"))))
	}
	b.WriteString("\n")

	b.WriteString(dim.Render(fmt.Sprintf("  %s %s",
		m.theme.IconStats, i18n.T("%d sessions, %s today", vd.statsWorkSessions, formatMinutesCompact(vd.statsTotalWorkTime)))))
	b.WriteString("\n")

	if m.mode != nil && m.mode.Name() == domain.MethodologyPomodoro {
		b.WriteString(dim.Render(fmt.Sprintf("  \U0001F345 %s", i18n.T("%d sessions today", vd.statsWorkSessions))))
		b.WriteString("\n")
	}

	if m.autoBreakTicks > 0 {
		b.WriteString(accent.Render("  " + i18n.T("Break starting in %ds... press any key to cancel", m.autoBreakTicks)))
	} else if vd.hasBreakInfo {
		b.WriteString(dim.Render("  " + i18n.T("[n]ew session  [b]reak %s %s  [m]ode  [q]uit", vd.breakDur, vd.breakLabel)))
	} else {
		b.WriteString(dim.Render("  " + i18n.T("[n]ew session  [b]reak  [m]ode  [q]uit")))
	}
	b.WriteString("\n")
	return b.String()
}

func (m InlineModel) viewInlineDeepWorkComplete(accent, dim lipgloss.Style) string {
	vd := buildCompletionViewData(&m.completionState, m.mode, m.state, m.completionInfo, m.completedElapsed)
	var b strings.Builder
	b.WriteString(accent.Render(fmt.Sprintf("  %s %s", m.theme.IconApp, i18n.T("Deep Work Session Complete."))))
	b.WriteString("\n")

	if vd.intendedOutcome != "" {
		b.WriteString(dim.Render("  " + i18n.T("Goal: %s", vd.intendedOutcome)))
		b.WriteString("\n")
	}

	if vd.distractionCount > 0 {
		b.WriteString(dim.Render("  " + i18n.T("Distractions: %d", vd.distractionCount)))
		b.WriteString("\n")
	}

	b.WriteString(dim.Render("  " + i18n.T("Deep Work: %s today (%.0f%% of %.0fh)",
		formatMinutesCompact(vd.statsTotalWorkTime), vd.deepWorkPct, vd.deepWorkGoalHours)))
	b.WriteString("\n")

	if m.shutdownRitualMode {
		b.WriteString(accent.Render("  " + i18n.T("Shutdown Ritual (step %d/4):", m.shutdownStep+1)))
		b.WriteString("\n")
		b.WriteString(dim.Render("  " + i18n.T(shutdownStepLabels[m.shutdownStep])))
		b.WriteString("\n")
		b.WriteString("  " + m.shutdownInputs[m.shutdownStep].View())
		b.WriteString("\n")
		b.WriteString(dim.Render("  " + i18n.T("enter save/skip step · esc exit ritual")))
	} else if m.accomplishmentMode {
		b.WriteString(dim.Render("  "+i18n.T("Accomplishment: ")) + m.accomplishmentInput.View())
		b.WriteString("\n")
		b.WriteString(dim.Render("  " + i18n.T("enter save · esc cancel")))
	} else if m.outcomeReviewMode {
		b.WriteString(accent.Render("  " + i18n.T("Did you achieve your intended outcome?")))
		b.WriteString("\n")
		b.WriteString(dim.Render("  " + i18n.T("Goal: %s", vd.intendedOutcome)))
		b.WriteString("\n")
		b.WriteString(dim.Render("  " + i18n.T("[y]es [p]artially [n]o [enter] skip")))
	} else if m.distractionReviewMode {
		b.WriteString(accent.Render("  " + i18n.T("Distraction Review:")))
		b.WriteString("\n")
		for i, d := range m.distractions {
			b.WriteString(dim.Render(fmt.Sprintf("    %d. %s", i+1, d)))
			b.WriteString("\n")
		}
		b.WriteString(dim.Render("  " + i18n.T("Consider batching these for tomorrow.")))
		b.WriteString("\n")
		b.WriteString(dim.Render("  " + i18n.T("enter dismiss")))
	} else if m.shutdownComplete || m.accomplishmentSaved {
		if !m.outcomeReviewDone && vd.intendedOutcome != "" {
			b.WriteString(accent.Render("  " + i18n.T("Shutdown ritual complete.")))
			b.WriteString("\n")
			b.WriteString(dim.Render("  " + i18n.T("[o]utcome review")))
		} else if vd.distractionCount > 0 && !m.distractionReviewDone {
			b.WriteString(accent.Render("  " + i18n.T("Shutdown ritual complete.")))
			b.WriteString("\n")
			b.WriteString(dim.Render("  " + i18n.T("[r]eview %d distractions", vd.distractionCount)))
		} else {
			b.WriteString(accent.Render("  " + i18n.T("Shutdown ritual complete.")))
		}
	}
	b.WriteString("\n")

	if !m.accomplishmentMode && !m.distractionReviewMode && !m.shutdownRitualMode && !m.outcomeReviewMode {
		if m.completionPromptsComplete() {
			b.WriteString(dim.Render("  " + i18n.T("[n]ew session [b]reak [m]ode [q]uit")))
		} else if !m.shutdownComplete && !m.accomplishmentSaved {
			b.WriteString(dim.Render("  " + i18n.T("→ [n]ew session locked: complete the shutdown ritual first")))
			b.WriteString("\n")
			b.WriteString(dim.Render("    " + i18n.T("Newport: a ritual trains your brain to fully disconnect — without it, work bleeds into rest.")))
			b.WriteString("\n")
			b.WriteString(dim.Render("  " + i18n.T("[a] shutdown ritual [b]reak [m]ode [q]uit")))
		} else if vd.intendedOutcome != "" && !m.outcomeReviewDone {
			b.WriteString(dim.Render("  " + i18n.T("→ [n]ew session locked: review your outcome first")))
			b.WriteString("\n")
			b.WriteString(dim.Render("    " + i18n.T("Did you achieve what you set out to do?")))
			b.WriteString("\n")
			b.WriteString(dim.Render("  " + i18n.T("[o]utcome review [b]reak [m]ode [q]uit")))
		} else if vd.distractionCount > 0 && !m.distractionReviewDone {
			b.WriteString(dim.Render("  " + i18n.T("→ [n]ew session locked: review your distractions first")))
			b.WriteString("\n")
			b.WriteString(dim.Render("    " + i18n.T("Newport: batch distractions and schedule them — don't let them follow you into the next block.")))
			b.WriteString("\n")
			b.WriteString(dim.Render("  " + i18n.T("[r]eview distractions [b]reak [m]ode [q]uit")))
		} else {
			b.WriteString(dim.Render("  " + i18n.T("[b]reak [m]ode [q]uit")))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m InlineModel) viewInlineMakeTimeComplete(accent, dim lipgloss.Style) string {
	vd := buildCompletionViewData(&m.completionState, m.mode, m.state, m.completionInfo, m.completedElapsed)
	var b strings.Builder
	b.WriteString(accent.Render(fmt.Sprintf("  %s %s", m.theme.IconApp, i18n.T("Session complete!"))))
	b.WriteString("\n")

	if vd.hasHighlightTask {
		b.WriteString(accent.Render("  " + i18n.T("You made time for your Highlight today.")))
		b.WriteString("\n")
	}

	if m.focusScoreSaved && m.focusScore != nil {
		b.WriteString(dim.Render("  " + i18n.T("Focus score: %d/5", *m.focusScore)))
	} else {
		b.WriteString(dim.Render("  " + i18n.T("How focused? [1] [2] [3] [4] [5]")))
	}
	b.WriteString("\n")

	if m.focusScoreSaved {
		if m.energizeSaved {
			b.WriteString(dim.Render("  " + i18n.T("Energize: %s", m.energizeActivity)))
		} else {
			b.WriteString(dim.Render("  " + i18n.T("Energize? [w]alk [t]stretch [e]xercise [n]one")))
		}
		b.WriteString("\n")
	}

	b.WriteString(dim.Render(fmt.Sprintf("  %s %s",
		m.theme.IconStats, i18n.T("%d sessions, %s worked today", vd.statsWorkSessions, formatMinutesCompact(vd.statsTotalWorkTime)))))
	b.WriteString("\n")

	if m.completionPromptsComplete() {
		b.WriteString(dim.Render("  " + i18n.T("[n]ew session [b]reak [m]ode [q]uit")))
	} else if !m.focusScoreSaved {
		b.WriteString(dim.Render("  " + i18n.T("→ [n]ew session locked: rate your focus first")))
		b.WriteString("\n")
		b.WriteString(dim.Render("    " + i18n.T("Make Time: tracking focus shows you when you're at your best — skip it and the data disappears.")))
		b.WriteString("\n")
		b.WriteString(dim.Render("  " + i18n.T("[1-5] focus score [b]reak [m]ode [q]uit")))
	} else {
		b.WriteString(dim.Render("  " + i18n.T("→ [n]ew session locked: log how you'll recharge first")))
		b.WriteString("\n")
		b.WriteString(dim.Render("    " + i18n.T("Make Time: energy fuels your next Highlight — Knapp says laser focus requires an energized body.")))
		b.WriteString("\n")
		b.WriteString(dim.Render("  " + i18n.T("[w]alk [t]stretch [e]xercise [n]one [b]reak [m]ode [q]uit")))
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

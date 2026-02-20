package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/methodology"
)

// inlinePhase tracks which phase the inline UI is in.
type inlinePhase int

const (
	phaseMainMenu inlinePhase = iota
	phasePickMode
	phasePickDuration
	phaseLaserChecklist
	phaseTaskSelect
	phaseTaskName
	phaseOutcome // Deep Work: intended outcome (shown after task, before timer)
	phaseTimer
	phaseWelcome
)

// MainMenuAction represents what the user selected from the main menu.
type MainMenuAction int

const (
	// MainMenuStartSession means continue to mode picker / session start.
	MainMenuStartSession MainMenuAction = iota
	// MainMenuViewStats means quit TUI and show stats dashboard.
	MainMenuViewStats
	// MainMenuReflect means quit TUI and show weekly reflection.
	MainMenuReflect
)

type mainMenuOption struct {
	action MainMenuAction
	label  string
	desc   string
}

var mainMenuOptions = []mainMenuOption{
	{MainMenuStartSession, "Start session", "Begin a new focus session"},
	{MainMenuViewStats, "View stats", "Show your productivity dashboard"},
	{MainMenuReflect, "Reflect", "Weekly reflection on your work"},
}

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

func (m InlineModel) updateWelcome(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", " ":
			if m.modeLocked {
				m.phase = phasePickDuration
			} else {
				m.phase = phaseMainMenu
			}
			return m, nil
		case "c", "ctrl+c", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m InlineModel) viewWelcome() string {
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorWork)).Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	var b strings.Builder
	b.WriteString(accent.Render("  Welcome to Flow!"))
	b.WriteString("\n")
	b.WriteString(dim.Render("  Three methodologies to choose from:"))
	b.WriteString("\n")
	b.WriteString(accent.Render("  Pomodoro   ") + dim.Render("25m sprints, short breaks — frictionless and fast"))
	b.WriteString("\n")
	b.WriteString(accent.Render("  Deep Work  ") + dim.Render("long blocks, distraction tracking, shutdown ritual (Newport)"))
	b.WriteString("\n")
	b.WriteString(accent.Render("  Make Time  ") + dim.Render("daily Highlight, focus score, energize (Knapp)"))
	b.WriteString("\n")
	b.WriteString(dim.Render("  Change anytime with \"flow config\""))
	b.WriteString("\n")
	b.WriteString(dim.Render("  enter continue · c close"))
	b.WriteString("\n")
	return b.String()
}

func (m InlineModel) updateMainMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k", "left", "h":
			if m.menuCursor > 0 {
				m.menuCursor--
			}
		case "down", "j", "right", "l":
			if m.menuCursor < len(mainMenuOptions)-1 {
				m.menuCursor++
			}
		case "1":
			m.menuCursor = 0
			return m.selectMainMenu()
		case "2":
			if len(mainMenuOptions) > 1 {
				m.menuCursor = 1
				return m.selectMainMenu()
			}
		case "3":
			if len(mainMenuOptions) > 2 {
				m.menuCursor = 2
				return m.selectMainMenu()
			}
		case "enter":
			return m.selectMainMenu()
		case "c", "ctrl+c", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m InlineModel) selectMainMenu() (tea.Model, tea.Cmd) {
	selected := mainMenuOptions[m.menuCursor]
	m.SelectedAction = selected.action
	switch selected.action {
	case MainMenuViewStats, MainMenuReflect:
		return m, tea.Quit
	default:
		// Start session → advance to mode picker
		if m.modeLocked {
			m.phase = phasePickDuration
		} else {
			m.phase = phasePickMode
		}
		return m, nil
	}
}

func (m InlineModel) viewMainMenu() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.ColorTitle))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorWork)).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	b.WriteString(titleStyle.Render("  Flow:") + "\n")

	for i, opt := range mainMenuOptions {
		if i == m.menuCursor {
			b.WriteString(activeStyle.Render("  ▸ "+opt.label) + "\n")
		} else {
			b.WriteString(dimStyle.Render("    "+opt.label) + "\n")
		}
	}

	desc := mainMenuOptions[m.menuCursor].desc
	b.WriteString(dimStyle.Render("  "+desc) + "\n")

	b.WriteString(dimStyle.Render("  ↑/↓ select · enter confirm · c close") + "\n")

	return b.String()
}

func (m InlineModel) updatePickMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Onboarding overlay: dismiss on Enter
	if m.onboardingMode {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				m.onboardingMode = false
				m.phase = phasePickDuration
				m.presetCursor = 0
				return m, nil
			case "esc":
				// Go back to mode picker
				m.onboardingMode = false
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
		case "esc":
			// Go back to main menu if we came from there
			if !m.modeLocked {
				m.phase = phaseMainMenu
				return m, nil
			}
			return m, tea.Quit
		case "c", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m InlineModel) selectMode() (tea.Model, tea.Cmd) {
	selected := modeOptions[m.modeCursor]
	m.mode = methodology.ForMethodology(selected.methodology, nil)
	m.presets = m.mode.Presets()

	if m.onModeSelected != nil {
		m.onModeSelected(selected.methodology)
	}

	// Show onboarding overlay with mode description
	if m.mode.Description() != "" {
		m.onboardingMode = true
		return m, nil
	}

	m.phase = phasePickDuration
	m.presetCursor = 0
	return m, nil
}

func (m InlineModel) viewPickMode() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.ColorTitle))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorWork)).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	// Onboarding overlay
	if m.onboardingMode && m.mode != nil {
		b.WriteString(activeStyle.Render("  "+m.mode.TUITitle()) + "\n")
		b.WriteString(dimStyle.Render("  "+m.mode.Description()) + "\n")
		b.WriteString(dimStyle.Render("  enter continue · esc back to mode picker") + "\n")
		return b.String()
	}

	b.WriteString(titleStyle.Render("  Mode:") + "  ")

	for i, opt := range modeOptions {
		label := opt.label
		if i == m.modeCursor {
			b.WriteString(activeStyle.Render(" ▸ " + label + " "))
		} else {
			b.WriteString(dimStyle.Render("   " + label + " "))
		}
	}
	b.WriteString("\n")

	// Show description of selected mode
	desc := modeOptions[m.modeCursor].desc
	b.WriteString(dimStyle.Render("  "+desc) + "\n")

	b.WriteString(dimStyle.Render("  ←/→ select · enter confirm · esc back · c close") + "\n")

	return b.String()
}

func (m InlineModel) advanceToTaskPhase() (tea.Model, tea.Cmd) {
	// Check for laser checklist first (Make Time mode)
	if m.mode != nil && m.mode.HasLaserChecklist() {
		m.laserChecklistCursor = 0
		m.laserChecklist = [3]bool{false, false, false}
		m.laserChecklistDone = false
		m.phase = phaseLaserChecklist
		return m, nil
	}

	// Fetch recent tasks if callback is available
	if m.fetchRecentTasks != nil {
		m.recentTasks = m.fetchRecentTasks(3)
	}
	// Check for yesterday's highlight carry-over (Make Time mode)
	if m.mode != nil && m.mode.HasHighlight() && m.fetchYesterdayHighlight != nil {
		m.yesterdayHighlight = m.fetchYesterdayHighlight()
	}
	if len(m.recentTasks) > 0 || m.yesterdayHighlight != nil {
		m.phase = phaseTaskSelect
		m.taskSelectCursor = 0
		return m, nil
	}
	// No recent tasks, skip to task name input
	m.phase = phaseTaskName
	m.taskInput.Focus()
	return m, m.taskInput.Cursor.BlinkCmd()
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
			return m.advanceToTaskPhase()
		case "2":
			if len(m.presets) > 1 {
				m.presetCursor = 1
				return m.advanceToTaskPhase()
			}
		case "3":
			if len(m.presets) > 2 {
				m.presetCursor = 2
				return m.advanceToTaskPhase()
			}
		case "enter":
			return m.advanceToTaskPhase()
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

// taskSelectOptionCount returns the total number of options in the task select list.
func (m InlineModel) taskSelectOptionCount() int {
	count := len(m.recentTasks) + 1 // recent tasks + "New task"
	if m.yesterdayHighlight != nil {
		count++ // carry-over option
	}
	return count
}

// taskSelectCarryOverIdx returns the index of the carry-over option, or -1 if none.
func (m InlineModel) taskSelectCarryOverIdx() int {
	if m.yesterdayHighlight != nil {
		return 0
	}
	return -1
}

// taskSelectRecentBaseIdx returns the starting index for recent tasks in the list.
func (m InlineModel) taskSelectRecentBaseIdx() int {
	if m.yesterdayHighlight != nil {
		return 1
	}
	return 0
}

func (m InlineModel) updateTaskSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	optionCount := m.taskSelectOptionCount()
	recentBase := m.taskSelectRecentBaseIdx()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.taskSelectCursor > 0 {
				m.taskSelectCursor--
			}
		case "down", "j":
			if m.taskSelectCursor < optionCount-1 {
				m.taskSelectCursor++
			}
		case "1":
			if len(m.recentTasks) >= 1 {
				m.taskSelectCursor = recentBase
				return m.selectRecentTask()
			}
		case "2":
			if len(m.recentTasks) >= 2 {
				m.taskSelectCursor = recentBase + 1
				return m.selectRecentTask()
			}
		case "3":
			if len(m.recentTasks) >= 3 {
				m.taskSelectCursor = recentBase + 2
				return m.selectRecentTask()
			}
		case "enter":
			// Carry-over option
			if m.taskSelectCursor == m.taskSelectCarryOverIdx() {
				return m.selectCarryOverTask()
			}
			// Recent task
			if m.taskSelectCursor >= recentBase && m.taskSelectCursor < recentBase+len(m.recentTasks) {
				return m.selectRecentTask()
			}
			// "New task" selected
			m.phase = phaseTaskName
			m.taskInput.Focus()
			return m, m.taskInput.Cursor.BlinkCmd()
		case "esc":
			m.phase = phasePickDuration
			return m, nil
		case "c", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m InlineModel) selectCarryOverTask() (tea.Model, tea.Cmd) {
	task := m.yesterdayHighlight
	task.SetAsHighlight() // updates to today
	m.taskInput.SetValue(task.Title)
	return m.advanceFromTask(task.Title)
}

func (m InlineModel) selectRecentTask() (tea.Model, tea.Cmd) {
	recentIdx := m.taskSelectCursor - m.taskSelectRecentBaseIdx()
	if recentIdx < 0 || recentIdx >= len(m.recentTasks) {
		return m, nil
	}
	task := m.recentTasks[recentIdx]
	m.taskInput.SetValue(task.Title)
	return m.advanceFromTask(task.Title)
}

// advanceFromTask decides whether to show the outcome phase (Deep Work)
// or start the session immediately. taskName may be empty.
func (m InlineModel) advanceFromTask(taskName string) (tea.Model, tea.Cmd) {
	if m.mode != nil && m.mode.OutcomePrompt() != "" {
		// Deep Work: ask for intended outcome before starting
		m.taskInput.Blur()
		m.outcomeInput.SetValue("")
		m.outcomeInput.Focus()
		m.phase = phaseOutcome
		return m, m.outcomeInput.Cursor.BlinkCmd()
	}
	return m.startSession(taskName, "")
}

// startSession calls onStartSession and transitions to the timer phase.
func (m InlineModel) startSession(taskName, intendedOutcome string) (tea.Model, tea.Cmd) {
	if m.onStartSession != nil {
		if err := m.onStartSession(m.presetCursor, taskName, intendedOutcome); err != nil {
			m.phase = phaseTimer
			return m, tickCmd()
		}
	}
	m.phase = phaseTimer
	return m, tickCmd()
}

func (m InlineModel) viewTaskSelect() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.ColorTitle))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorWork)).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	p := m.presets[m.presetCursor]
	b.WriteString(activeStyle.Render(fmt.Sprintf("  ▸ %s %s", p.Name, formatMinutesCompact(p.Duration))))
	b.WriteString("\n")

	b.WriteString(titleStyle.Render("  Task:") + "\n")

	idx := 0

	// Carry-over option
	if m.yesterdayHighlight != nil {
		label := fmt.Sprintf("Carry forward: %s", m.yesterdayHighlight.Title)
		if idx == m.taskSelectCursor {
			b.WriteString(activeStyle.Render("  ▸ "+label) + "\n")
		} else {
			b.WriteString(dimStyle.Render("    "+label) + "\n")
		}
		idx++
	}

	// Recent tasks
	for i, task := range m.recentTasks {
		label := fmt.Sprintf("[%d] %s", i+1, task.Title)
		if idx == m.taskSelectCursor {
			b.WriteString(activeStyle.Render("  ▸ "+label) + "\n")
		} else {
			b.WriteString(dimStyle.Render("    "+label) + "\n")
		}
		idx++
	}

	// "New task" option
	newLabel := "New task..."
	if idx == m.taskSelectCursor {
		b.WriteString(activeStyle.Render("  ▸ "+newLabel) + "\n")
	} else {
		b.WriteString(dimStyle.Render("    "+newLabel) + "\n")
	}

	b.WriteString(dimStyle.Render("  ↑/↓ select · enter confirm · esc back") + "\n")

	return b.String()
}

func (m InlineModel) updateTaskName(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			taskName := strings.TrimSpace(m.taskInput.Value())
			return m.advanceFromTask(taskName)
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.taskInput.Blur()
			if len(m.recentTasks) > 0 {
				m.phase = phaseTaskSelect
			} else {
				m.phase = phasePickDuration
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.taskInput, cmd = m.taskInput.Update(msg)
	return m, cmd
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
	b.WriteString(titleStyle.Render("  " + taskPrompt + " "))
	b.WriteString(m.taskInput.View())
	b.WriteString("\n")

	b.WriteString(dimStyle.Render("  enter start · esc back · ctrl+c quit") + "\n")

	return b.String()
}

// updatePickOutcome handles the intended outcome input phase (Deep Work only).
func (m InlineModel) updatePickOutcome(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			outcome := strings.TrimSpace(m.outcomeInput.Value())
			m.intendedOutcome = outcome
			m.outcomeInput.Blur()
			taskName := strings.TrimSpace(m.taskInput.Value())
			return m.startSession(taskName, outcome)
		case "esc":
			// Go back to task name input
			m.outcomeInput.Blur()
			m.phase = phaseTaskName
			m.taskInput.Focus()
			return m, m.taskInput.Cursor.BlinkCmd()
		case "ctrl+c":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.outcomeInput, cmd = m.outcomeInput.Update(msg)
	return m, cmd
}

// viewPickOutcome renders the intended outcome prompt (Deep Work only).
func (m InlineModel) viewPickOutcome() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.ColorTitle))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorWork)).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	p := m.presets[m.presetCursor]
	b.WriteString(activeStyle.Render(fmt.Sprintf("  ▸ %s %s", p.Name, formatMinutesCompact(p.Duration))))
	b.WriteString("\n")

	if m.taskInput.Value() != "" {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  Task: %s", strings.TrimSpace(m.taskInput.Value()))))
		b.WriteString("\n")
	}

	outcomePrompt := "Intended outcome:"
	if m.mode != nil && m.mode.OutcomePrompt() != "" {
		outcomePrompt = m.mode.OutcomePrompt()
	}
	b.WriteString(titleStyle.Render("  " + outcomePrompt + " "))
	b.WriteString(m.outcomeInput.View())
	b.WriteString("\n")

	b.WriteString(dimStyle.Render("  enter start · esc back · ctrl+c quit") + "\n")

	return b.String()
}

// laserChecklistItems defines the items in the laser checklist.
var laserChecklistItems = []string{
	"Phone on Do Not Disturb?",
	"Notifications off?",
	"Distracting tabs/apps closed?",
}

// updateLaserChecklist handles the laser checklist phase (Make Time only).
func (m InlineModel) updateLaserChecklist(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.laserChecklistCursor > 0 {
				m.laserChecklistCursor--
			}
		case "down", "j":
			if m.laserChecklistCursor < len(laserChecklistItems) {
				m.laserChecklistCursor++
			}
		case "y":
			if m.laserChecklistCursor < len(laserChecklistItems) {
				m.laserChecklist[m.laserChecklistCursor] = true
				if m.laserChecklistCursor < len(laserChecklistItems)-1 {
					m.laserChecklistCursor++
				}
			}
		case "n":
			if m.laserChecklistCursor < len(laserChecklistItems) {
				m.laserChecklist[m.laserChecklistCursor] = false
				if m.laserChecklistCursor < len(laserChecklistItems)-1 {
					m.laserChecklistCursor++
				}
			}
		case "enter":
			// Skip remaining items and proceed
			return m.advanceFromLaserChecklist()
		case "esc":
			m.phase = phasePickDuration
			return m, nil
		case "c", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

// advanceFromLaserChecklist proceeds to task selection after the checklist.
func (m InlineModel) advanceFromLaserChecklist() (tea.Model, tea.Cmd) {
	m.laserChecklistDone = true
	// Fetch recent tasks if callback is available
	if m.fetchRecentTasks != nil {
		m.recentTasks = m.fetchRecentTasks(3)
	}
	// Check for yesterday's highlight carry-over (Make Time mode)
	if m.mode != nil && m.mode.HasHighlight() && m.fetchYesterdayHighlight != nil {
		m.yesterdayHighlight = m.fetchYesterdayHighlight()
	}
	if len(m.recentTasks) > 0 || m.yesterdayHighlight != nil {
		m.phase = phaseTaskSelect
		m.taskSelectCursor = 0
		return m, nil
	}
	// No recent tasks, skip to task name input
	m.phase = phaseTaskName
	m.taskInput.Focus()
	return m, m.taskInput.Cursor.BlinkCmd()
}

// viewLaserChecklist renders the laser checklist (Make Time only).
func (m InlineModel) viewLaserChecklist() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.ColorTitle))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorWork)).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))
	checkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")) // Green check
	crossStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")) // Red cross

	p := m.presets[m.presetCursor]
	b.WriteString(activeStyle.Render(fmt.Sprintf("  ▸ %s %s", p.Name, formatMinutesCompact(p.Duration))))
	b.WriteString("\n")

	b.WriteString(titleStyle.Render("  Laser Checklist:"))
	b.WriteString("\n")

	for i, item := range laserChecklistItems {
		var status string
		if m.laserChecklist[i] {
			status = checkStyle.Render("✓")
		} else if i < m.laserChecklistCursor || (m.laserChecklistCursor == i && m.laserChecklist[i]) {
			status = crossStyle.Render("✗")
		} else {
			status = " "
		}

		if i == m.laserChecklistCursor {
			b.WriteString(activeStyle.Render(fmt.Sprintf("  ▸ [%s] %s", status, item)))
		} else {
			b.WriteString(dimStyle.Render(fmt.Sprintf("    [%s] %s", status, item)))
		}
		b.WriteString("\n")
	}

	b.WriteString(dimStyle.Render("  [y]es [n]o [enter] skip all · esc back"))
	b.WriteString("\n")

	return b.String()
}

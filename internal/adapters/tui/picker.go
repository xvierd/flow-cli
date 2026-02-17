package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/xvierd/flow-cli/internal/config"
)

// PickerItem represents one option in the picker.
type PickerItem struct {
	Label string
	Desc  string
}

// PickerResult holds the outcome of a picker interaction.
type PickerResult struct {
	Index   int
	Aborted bool
}

type pickerModel struct {
	title   string
	items   []PickerItem
	footer  string
	cursor  int
	chosen  bool
	aborted bool
	theme   config.ThemeConfig
}

func (m pickerModel) Init() tea.Cmd { return nil }

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter":
			m.chosen = true
			return m, tea.Quit
		case "ctrl+c", "esc":
			m.aborted = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m pickerModel) View() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.ColorTitle))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorWork)).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))
	footerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	b.WriteString("\n")
	b.WriteString(titleStyle.Render("  "+m.title) + "\n\n")

	arrowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorWork)).Bold(true)

	for i, item := range m.items {
		if i == m.cursor {
			arrow := arrowStyle.Render("▸")
			line := activeStyle.Render(fmt.Sprintf(" %-10s %s", item.Label, item.Desc))
			b.WriteString(fmt.Sprintf("  %s%s\n", arrow, line))
		} else {
			b.WriteString(dimStyle.Render(fmt.Sprintf("    %-10s %s", item.Label, item.Desc)) + "\n")
		}
	}

	if m.footer != "" {
		b.WriteString("\n")
		b.WriteString(footerStyle.Render("  "+m.footer) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  ↑/↓ navigate · enter select · esc back") + "\n")

	return b.String()
}

// --- Horizontal picker (for inline / narrow terminals) ---

type hPickerModel struct {
	title   string
	items   []PickerItem
	footer  string
	cursor  int
	chosen  bool
	aborted bool
	theme   config.ThemeConfig
}

func (m hPickerModel) Init() tea.Cmd { return nil }

func (m hPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h":
			if m.cursor > 0 {
				m.cursor--
			}
		case "right", "l":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "1":
			m.cursor = 0
			m.chosen = true
			return m, tea.Quit
		case "2":
			if len(m.items) > 1 {
				m.cursor = 1
				m.chosen = true
				return m, tea.Quit
			}
		case "3":
			if len(m.items) > 2 {
				m.cursor = 2
				m.chosen = true
				return m, tea.Quit
			}
		case "enter":
			m.chosen = true
			return m, tea.Quit
		case "ctrl+c", "esc":
			m.aborted = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m hPickerModel) View() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.ColorTitle))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorWork)).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	b.WriteString(titleStyle.Render("  "+m.title) + "  ")

	for i, item := range m.items {
		label := fmt.Sprintf("%s %s", item.Label, item.Desc)
		if i == m.cursor {
			b.WriteString(activeStyle.Render(" ▸ " + label + " "))
		} else {
			b.WriteString(dimStyle.Render("   " + label + " "))
		}
	}
	b.WriteString("\n")

	if m.footer != "" {
		b.WriteString(dimStyle.Render("  "+m.footer) + "\n")
	}

	b.WriteString(dimStyle.Render("  ←/→ navigate · enter select · esc back") + "\n")

	return b.String()
}

// RunHorizontalPicker launches a compact horizontal arrow-key picker.
func RunHorizontalPicker(title string, items []PickerItem, footer string, theme *config.ThemeConfig) PickerResult {
	resolved := resolveTheme(theme)
	m := hPickerModel{
		title:  title,
		items:  items,
		footer: footer,
		theme:  resolved,
	}

	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return PickerResult{Aborted: true}
	}

	final := result.(hPickerModel)
	if final.aborted {
		return PickerResult{Aborted: true}
	}
	return PickerResult{Index: final.cursor}
}

// RunPicker launches an interactive arrow-key picker and returns the selected index.
func RunPicker(title string, items []PickerItem, footer string, theme *config.ThemeConfig) PickerResult {
	resolved := resolveTheme(theme)
	m := pickerModel{
		title:  title,
		items:  items,
		footer: footer,
		theme:  resolved,
	}

	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return PickerResult{Aborted: true}
	}

	final := result.(pickerModel)
	if final.aborted {
		return PickerResult{Aborted: true}
	}
	return PickerResult{Index: final.cursor}
}

// --- Styled text prompt ---

// TextPromptResult holds the outcome of a text prompt.
type TextPromptResult struct {
	Value   string
	Aborted bool
}

type textPromptModel struct {
	title       string
	placeholder string
	input       textinput.Model
	aborted     bool
	theme       config.ThemeConfig
}

func (m textPromptModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m textPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			return m, tea.Quit
		case "ctrl+c", "esc":
			m.aborted = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m textPromptModel) View() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.ColorTitle))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.ColorHelp))

	b.WriteString("\n")
	b.WriteString(titleStyle.Render("  "+m.title) + " ")
	b.WriteString(m.input.View())
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("  enter confirm · esc back") + "\n")

	return b.String()
}

// RunTextPrompt launches a styled text input prompt.
func RunTextPrompt(title string, placeholder string, theme *config.ThemeConfig) TextPromptResult {
	resolved := resolveTheme(theme)

	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 120
	ti.Width = 50
	ti.Focus()

	m := textPromptModel{
		title:       title,
		placeholder: placeholder,
		input:       ti,
		theme:       resolved,
	}

	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return TextPromptResult{Aborted: true}
	}

	final := result.(textPromptModel)
	if final.aborted {
		return TextPromptResult{Aborted: true}
	}
	return TextPromptResult{Value: strings.TrimSpace(final.input.Value())}
}

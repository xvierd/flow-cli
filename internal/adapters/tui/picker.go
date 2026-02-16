package tui

import (
	"fmt"
	"strings"

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
	Index    int
	Aborted  bool
}

type pickerModel struct {
	title    string
	items    []PickerItem
	footer   string
	cursor   int
	chosen   bool
	aborted  bool
	theme    config.ThemeConfig
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
		case "q", "ctrl+c", "esc":
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
	b.WriteString(dimStyle.Render("  ↑/↓ navigate · enter select · q quit") + "\n")

	return b.String()
}

// RunPicker launches an interactive arrow-key picker and returns the selected index.
func RunPicker(title string, items []PickerItem, footer string, theme *config.ThemeConfig) PickerResult {
	resolved := resolveTheme(theme)
	m := pickerModel{
		title: title,
		items: items,
		footer: footer,
		theme: resolved,
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

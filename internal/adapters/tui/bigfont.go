package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// digitMap maps each digit character (0-9) and colon to a 5-line ASCII representation.
// Each digit is 4 chars wide, colon is 1 char wide.
var digitMap = map[rune][5]string{
	'0': {
		"████",
		"█  █",
		"█  █",
		"█  █",
		"████",
	},
	'1': {
		" █ ",
		"██ ",
		" █ ",
		" █ ",
		"███",
	},
	'2': {
		"████",
		"   █",
		"████",
		"█   ",
		"████",
	},
	'3': {
		"████",
		"   █",
		"████",
		"   █",
		"████",
	},
	'4': {
		"█  █",
		"█  █",
		"████",
		"   █",
		"   █",
	},
	'5': {
		"████",
		"█   ",
		"████",
		"   █",
		"████",
	},
	'6': {
		"████",
		"█   ",
		"████",
		"█  █",
		"████",
	},
	'7': {
		"████",
		"   █",
		"  █ ",
		" █  ",
		" █  ",
	},
	'8': {
		"████",
		"█  █",
		"████",
		"█  █",
		"████",
	},
	'9': {
		"████",
		"█  █",
		"████",
		"   █",
		"████",
	},
	':': {
		" ",
		"█",
		" ",
		"█",
		" ",
	},
}

// renderBigTime takes a time string like "14:32" and returns a multi-line
// styled ASCII art representation. Falls back to a single styled line
// if the terminal width is less than 40.
func renderBigTime(timeStr string, color lipgloss.Color, width int) string {
	if width < 40 {
		style := lipgloss.NewStyle().Bold(true).Foreground(color)
		return style.Render(timeStr)
	}

	lines := [5]string{}
	for _, ch := range timeStr {
		glyph, ok := digitMap[ch]
		if !ok {
			continue
		}
		spacing := " "
		if ch == ':' {
			spacing = " "
		}
		for i := 0; i < 5; i++ {
			if lines[i] != "" {
				lines[i] += spacing
			}
			lines[i] += glyph[i]
		}
	}

	style := lipgloss.NewStyle().Bold(true).Foreground(color)
	styled := make([]string, 5)
	for i, line := range lines {
		styled[i] = style.Render(line)
	}

	return strings.Join(styled, "\n")
}

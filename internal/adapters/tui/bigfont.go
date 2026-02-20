package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// digitMap maps each digit character (0-9) and colon to a 3-line half-block representation.
// Each digit is 3 chars wide, colon is 1 char wide.
var digitMap = map[rune][3]string{
	'0': {
		"█▀█",
		"█ █",
		"▀▀▀",
	},
	'1': {
		"▀█ ",
		" █ ",
		"▀▀▀",
	},
	'2': {
		"▀▀█",
		"█▀▀",
		"▀▀▀",
	},
	'3': {
		"▀▀█",
		"▀▀█",
		"▀▀▀",
	},
	'4': {
		"█ █",
		"▀▀█",
		"  ▀",
	},
	'5': {
		"█▀▀",
		"▀▀█",
		"▀▀▀",
	},
	'6': {
		"█▀▀",
		"█▀█",
		"▀▀▀",
	},
	'7': {
		"▀▀█",
		"  █",
		"  ▀",
	},
	'8': {
		"█▀█",
		"█▀█",
		"▀▀▀",
	},
	'9': {
		"█▀█",
		"▀▀█",
		"▀▀▀",
	},
	':': {
		"▀",
		" ",
		"▀",
	},
}

// renderBigTime takes a time string like "14:32" and returns a multi-line
// styled half-block representation. Falls back to a single styled line
// if the terminal width is less than 40.
func renderBigTime(timeStr string, color lipgloss.Color, width int) string {
	if width < 40 {
		style := lipgloss.NewStyle().Bold(true).Foreground(color)
		return style.Render(timeStr)
	}

	lines := [3]string{}
	for _, ch := range timeStr {
		glyph, ok := digitMap[ch]
		if !ok {
			continue
		}
		spacing := " "
		if ch == ':' {
			spacing = " "
		}
		for i := 0; i < 3; i++ {
			if lines[i] != "" {
				lines[i] += spacing
			}
			lines[i] += glyph[i]
		}
	}

	style := lipgloss.NewStyle().Bold(true).Foreground(color)
	styled := make([]string, 0, len(lines))
	for _, line := range lines {
		styled = append(styled, style.Render(line))
	}

	return strings.Join(styled, "\n")
}

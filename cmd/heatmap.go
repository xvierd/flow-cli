package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/services"
)

// heatColors holds the 5 heatmap levels (0 = no focus), tuned per background.
// Every value is an exact xterm-256 palette entry so the ramp stays monotonic
// on 256-color terminals (e.g. macOS Terminal.app), not just truecolor ones.
var heatColors = [domain.HeatmapLevels]lipgloss.AdaptiveColor{
	{Light: "#e4e4e4", Dark: "#3a3a3a"}, // 254 / 237
	{Light: "#d7d7ff", Dark: "#5f5f87"}, // 189 / 60
	{Light: "#afafff", Dark: "#5f5fd7"}, // 147 / 62
	{Light: "#8787d7", Dark: "#8787ff"}, // 104 / 105
	{Light: "#5f5faf", Dark: "#d7d7ff"}, // 61 / 189
}

const heatCell = "■"

// heatGlyphs distinguish levels by shape when the output has no colors
// (piped, NO_COLOR, dumb terminals).
var heatGlyphs = [domain.HeatmapLevels]string{"·", "░", "▒", "▓", "█"}

// heatGlyph renders one heatmap level, by color when available.
func heatGlyph(level int) string {
	if lipgloss.ColorProfile() == termenv.Ascii {
		return heatGlyphs[level]
	}
	return lipgloss.NewStyle().Foreground(heatColors[level]).Render(heatCell)
}

func runYearHeatmap(ctx context.Context, now time.Time) error {
	hm, err := services.NewWrappedService(app.storage).GetYearHeatmap(ctx, now)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to get stats"), err)
	}
	if jsonOutput {
		days := make([]map[string]interface{}, 0, services.HeatmapWeeks*7)
		for _, week := range hm.Weeks {
			for _, c := range week {
				if c.Future {
					continue
				}
				days = append(days, map[string]interface{}{
					"date": c.Date.Format("2006-01-02"), "minutes": c.Minutes, "level": c.Level,
				})
			}
		}
		return jsonOut(map[string]interface{}{
			"start":         hm.Start.Format("2006-01-02"),
			"end":           hm.End.Format("2006-01-02"),
			"total_minutes": hm.TotalMinutes,
			"active_days":   hm.ActiveDays,
			"max_minutes":   hm.MaxMinutes,
			"days":          days,
		})
	}
	var buf bytes.Buffer
	renderYearHeatmap(&buf, hm)
	fmt.Println()
	_, err = os.Stdout.Write(buf.Bytes())
	return err
}

// renderYearHeatmap draws the 53x7 grid with month labels on top, weekday
// labels on the left and a Less→More legend.
func renderYearHeatmap(w *bytes.Buffer, hm *domain.YearHeatmap) {
	title := lipgloss.NewStyle().Bold(true).Foreground(wrapAccent)
	dim := lipgloss.NewStyle().Foreground(wrapMuted)
	value := lipgloss.NewStyle().Bold(true).Foreground(wrapAccent)
	const gutter = 5 // weekday label column

	fmt.Fprintf(w, "  %s\n", title.Render(i18n.T("Focus over the last 12 months")))
	fmt.Fprintf(w, "  %s\n\n", i18n.T("%s of focus across %s",
		value.Render(formatHours(float64(hm.TotalMinutes)/60)),
		value.Render(i18n.T("%d days", hm.ActiveDays))))

	// Month labels: placed at the first column whose Monday falls in a new
	// month, skipped when they would overlap the previous label.
	months := []rune(strings.Repeat(" ", len(hm.Weeks)+4))
	nextFree := 0
	for col, week := range hm.Weeks {
		m := week[0].Date.Month()
		if col > 0 && hm.Weeks[col-1][0].Date.Month() == m {
			continue
		}
		name := []rune(i18n.MonthName(m))
		if col < nextFree || col+len(name) > len(months) {
			continue
		}
		copy(months[col:], name)
		nextFree = col + len(name) + 1
	}
	fmt.Fprintf(w, "  %s%s\n", strings.Repeat(" ", gutter), dim.Render(strings.TrimRight(string(months), " ")))

	for row := 0; row < 7; row++ {
		label := ""
		if row%2 == 0 && row < 6 { // Mon, Wed, Fri
			label = i18n.DayName(time.Weekday((row + 1) % 7))
		}
		var line strings.Builder
		for _, week := range hm.Weeks {
			c := week[row]
			if c.Future {
				line.WriteString(" ")
				continue
			}
			line.WriteString(heatGlyph(c.Level))
		}
		fmt.Fprintf(w, "  %s%s\n", dim.Render(fmt.Sprintf("%-*s", gutter, label)), line.String())
	}

	var legend strings.Builder
	for lvl := range heatColors {
		legend.WriteString(heatGlyph(lvl) + " ")
	}
	legendLine := dim.Render(i18n.T("Less")) + " " + legend.String() + dim.Render(i18n.T("More"))
	pad := gutter + len(hm.Weeks) - lipgloss.Width(legendLine)
	fmt.Fprintf(w, "\n  %s%s\n", strings.Repeat(" ", max(pad, 0)), legendLine)

	if best := busiestDay(hm); best.Minutes > 0 {
		fmt.Fprintf(w, "\n  %s  %s %s\n", dim.Render(i18n.T("Busiest day:")),
			value.Render(fmt.Sprintf("%s %s %d, %d", i18n.DayName(best.Date.Weekday()), i18n.MonthName(best.Date.Month()), best.Date.Day(), best.Date.Year())),
			dim.Render("("+formatHours(float64(best.Minutes)/60)+")"))
	}
	fmt.Fprintln(w)
}

// busiestDay returns the earliest cell with the most minutes.
func busiestDay(hm *domain.YearHeatmap) domain.HeatmapCell {
	var best domain.HeatmapCell
	for _, week := range hm.Weeks {
		for _, c := range week {
			if c.Minutes > best.Minutes {
				best = c
			}
		}
	}
	return best
}

package cmd

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/domain"
	"sort"
)

var statsPeriod string

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show a dashboard of session statistics",
	Long:  `Display a terminal dashboard with session counts, deep work hours, focus scores, and distraction trends.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		now := time.Now()

		var start, end time.Time
		var label string

		switch statsPeriod {
		case "month":
			start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
			end = start.AddDate(0, 1, 0)
			label = now.Format("January 2006")
		default:
			// Default to current week (Monday start)
			weekday := int(now.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			start = time.Date(now.Year(), now.Month(), now.Day()-(weekday-1), 0, 0, 0, 0, now.Location())
			end = start.AddDate(0, 0, 7)
			label = fmt.Sprintf("Week of %s", start.Format("Jan 2"))
		}

		stats, err := app.storage.Sessions().GetPeriodStats(ctx, start, end)
		if err != nil {
			return fmt.Errorf("failed to get stats: %w", err)
		}
		stats.Label = label

		// Fetch hourly productivity (last 30 days)
		hourly, err := app.storage.Sessions().GetHourlyProductivity(ctx, 30)
		if err != nil {
			hourly = nil // non-fatal
		}

		// Fetch energize stats (Make Time)
		energize, err := app.storage.Sessions().GetEnergizeStats(ctx, start, end)
		if err != nil {
			energize = nil // non-fatal
		}

		fmt.Println()
		renderDashboard(stats, hourly, energize)
		return nil
	},
}

func init() {
	statsCmd.Flags().StringVarP(&statsPeriod, "period", "p", "week", "Time period: week or month")
	rootCmd.AddCommand(statsCmd)
}

func renderDashboard(stats *domain.PeriodStats, hourly map[int]time.Duration, energize []domain.EnergizeStat) {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C6FE0"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	valueStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA"))
	barColor := lipgloss.NewStyle().Foreground(lipgloss.Color("#7C6FE0"))

	// Header
	fmt.Printf("  %s\n", titleStyle.Render(stats.Label))
	fmt.Printf("  %s\n\n", dimStyle.Render(strings.Repeat("─", 40)))

	// Summary line
	hours := stats.TotalWorkTime.Hours()
	fmt.Printf("  Total: %s sessions, %s deep work\n\n",
		valueStyle.Render(fmt.Sprintf("%d", stats.TotalSessions)),
		valueStyle.Render(formatHours(hours)),
	)

	if stats.TotalSessions == 0 {
		fmt.Printf("  %s\n\n", dimStyle.Render("No completed sessions in this period."))
		return
	}

	// Bar chart: sessions per methodology
	fmt.Printf("  %s\n", dimStyle.Render("Sessions by mode"))
	maxCount := 0
	for _, m := range stats.ByMethodology {
		if m.SessionCount > maxCount {
			maxCount = m.SessionCount
		}
	}

	maxBarWidth := 30
	for _, m := range stats.ByMethodology {
		barWidth := 0
		if maxCount > 0 {
			barWidth = int(math.Round(float64(m.SessionCount) / float64(maxCount) * float64(maxBarWidth)))
		}
		if barWidth < 1 && m.SessionCount > 0 {
			barWidth = 1
		}
		bar := buildBar(barWidth)
		methodLabel := fmt.Sprintf("%-10s", m.Methodology.Label())
		fmt.Printf("  %s %s %d (%s)\n",
			dimStyle.Render(methodLabel),
			barColor.Render(bar),
			m.SessionCount,
			formatHours(m.TotalTime.Hours()),
		)
	}
	fmt.Println()

	// Focus score (Make Time)
	if stats.FocusScoreCount > 0 {
		fmt.Printf("  %s  %s  %s\n",
			dimStyle.Render("Avg focus score:"),
			valueStyle.Render(fmt.Sprintf("%.1f/5", stats.AvgFocusScore)),
			dimStyle.Render(fmt.Sprintf("(%d sessions)", stats.FocusScoreCount)),
		)
	}

	// Distraction count (Deep Work)
	if stats.DistractionCount > 0 {
		fmt.Printf("  %s  %s\n",
			dimStyle.Render("Distractions:"),
			valueStyle.Render(fmt.Sprintf("%d", stats.DistractionCount)),
		)
	}

	if stats.FocusScoreCount > 0 || stats.DistractionCount > 0 {
		fmt.Println()
	}

	// Hourly productivity heatmap
	renderHourlyProductivity(hourly, dimStyle, valueStyle, barColor)

	// Energize Insights (Make Time)
	renderEnergizeInsights(energize, dimStyle, valueStyle, titleStyle)
}

// renderEnergizeInsights displays a table of energize activities and their avg focus scores.
func renderEnergizeInsights(energize []domain.EnergizeStat, dimStyle, valueStyle, titleStyle lipgloss.Style) {
	if len(energize) == 0 {
		return
	}

	fmt.Printf("  %s\n", dimStyle.Render("Energize Insights"))
	for _, e := range energize {
		activityLabel := fmt.Sprintf("%-10s", e.Activity)
		fmt.Printf("  %s  %s session%s  avg focus %s\n",
			dimStyle.Render(activityLabel),
			valueStyle.Render(fmt.Sprintf("%d", e.SessionCount)),
			func() string {
				if e.SessionCount == 1 {
					return ""
				}
				return "s"
			}(),
			valueStyle.Render(fmt.Sprintf("%.1f", e.AvgFocusScore)),
		)
	}
	fmt.Println()
}

// hourEntry pairs an hour with its total duration for sorting.
type hourEntry struct {
	Hour     int
	Duration time.Duration
}

func renderHourlyProductivity(hourly map[int]time.Duration, dimStyle, valueStyle, barColor lipgloss.Style) {
	if len(hourly) == 0 {
		return
	}

	// Sort hours by total duration descending to find top 3
	entries := make([]hourEntry, 0, len(hourly))
	for h, d := range hourly {
		entries = append(entries, hourEntry{Hour: h, Duration: d})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Duration > entries[j].Duration
	})

	fmt.Printf("  %s\n", dimStyle.Render("Your most productive hours (last 30 days)"))
	top := 3
	if len(entries) < top {
		top = len(entries)
	}
	for _, e := range entries[:top] {
		hourLabel := fmt.Sprintf("%2d:00-%d:00", e.Hour, e.Hour+1)
		fmt.Printf("  %s  %s\n",
			dimStyle.Render(hourLabel),
			valueStyle.Render(formatHours(e.Duration.Hours())),
		)
	}
	fmt.Println()
}

// buildBar creates a horizontal bar using block characters.
func buildBar(width int) string {
	if width <= 0 {
		return ""
	}
	return strings.Repeat("█", width)
}

// formatHours formats a float hours value as "Xh Ym".
func formatHours(h float64) string {
	if h < 0.01 {
		return "0m"
	}
	hours := int(h)
	minutes := int(math.Round((h - float64(hours)) * 60))
	if hours > 0 && minutes > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dm", minutes)
}

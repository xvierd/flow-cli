package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var reflectCmd = &cobra.Command{
	Use:   "reflect",
	Short: "Show a weekly reflection dashboard",
	Long:  `Display a reflection of your week: daily sessions, highlights, focus scores, and distraction trends.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		now := time.Now()

		// Compute week start (Monday)
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		weekStart := time.Date(now.Year(), now.Month(), now.Day()-(weekday-1), 0, 0, 0, 0, now.Location())
		weekEnd := weekStart.AddDate(0, 0, 7)

		titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C6FE0"))
		dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
		valueStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA"))
		accentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399"))

		fmt.Println()
		fmt.Printf("  %s\n", titleStyle.Render(fmt.Sprintf("Weekly Reflection — %s", weekStart.Format("Jan 2"))))
		fmt.Printf("  %s\n\n", dimStyle.Render(strings.Repeat("─", 45)))

		// Day-by-day breakdown
		dayNames := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
		fmt.Printf("  %s\n", dimStyle.Render("Day       Sessions   Work Time"))
		fmt.Printf("  %s\n", dimStyle.Render(strings.Repeat("─", 35)))

		totalSessions := 0
		var totalWork time.Duration

		for i := 0; i < 7; i++ {
			day := weekStart.AddDate(0, 0, i)
			if day.After(now) {
				break
			}

			stats, err := storageAdapter.Sessions().GetDailyStats(ctx, day)
			if err != nil {
				continue
			}

			sessionsStr := fmt.Sprintf("%d", stats.WorkSessions)
			workStr := formatMinutes(stats.TotalWorkTime)
			if stats.WorkSessions == 0 {
				sessionsStr = "-"
				workStr = "-"
			}

			isToday := day.Day() == now.Day() && day.Month() == now.Month()
			dayLabel := dayNames[i]
			if isToday {
				dayLabel = dayLabel + "*"
			}

			fmt.Printf("  %-10s %s    %s\n",
				dimStyle.Render(fmt.Sprintf("%-6s", dayLabel)),
				valueStyle.Render(fmt.Sprintf("%-8s", sessionsStr)),
				valueStyle.Render(workStr),
			)

			totalSessions += stats.WorkSessions
			totalWork += stats.TotalWorkTime
		}

		fmt.Printf("  %s\n", dimStyle.Render(strings.Repeat("─", 35)))
		fmt.Printf("  %-10s %s    %s\n\n",
			dimStyle.Render("Total"),
			valueStyle.Render(fmt.Sprintf("%-8d", totalSessions)),
			valueStyle.Render(formatMinutes(totalWork)),
		)

		// Period stats for focus score and distractions
		periodStats, err := storageAdapter.Sessions().GetPeriodStats(ctx, weekStart, weekEnd)
		if err == nil {
			if periodStats.FocusScoreCount > 0 {
				fmt.Printf("  %s  %s  %s\n",
					dimStyle.Render("Avg focus score:"),
					valueStyle.Render(fmt.Sprintf("%.1f/5", periodStats.AvgFocusScore)),
					dimStyle.Render(fmt.Sprintf("(%d sessions)", periodStats.FocusScoreCount)),
				)
			}

			if periodStats.DistractionCount > 0 {
				fmt.Printf("  %s  %s\n",
					dimStyle.Render("Distractions:"),
					valueStyle.Render(fmt.Sprintf("%d", periodStats.DistractionCount)),
				)
			}

			if periodStats.FocusScoreCount > 0 || periodStats.DistractionCount > 0 {
				fmt.Println()
			}
		}

		// Highlights for the week
		fmt.Printf("  %s\n", dimStyle.Render("Highlights this week"))
		foundHighlight := false
		for i := 0; i < 7; i++ {
			day := weekStart.AddDate(0, 0, i)
			if day.After(now) {
				break
			}
			highlight, err := storageAdapter.Tasks().FindTodayHighlight(ctx, day)
			if err != nil || highlight == nil {
				continue
			}
			foundHighlight = true
			status := dimStyle.Render("  ")
			if highlight.Status == "completed" {
				status = accentStyle.Render("  ")
			}
			fmt.Printf("  %s %s %s\n",
				dimStyle.Render(day.Format("Mon")),
				status,
				valueStyle.Render(highlight.Title),
			)
		}
		if !foundHighlight {
			fmt.Printf("  %s\n", dimStyle.Render("No highlights set this week."))
		}
		fmt.Println()

		// Energize correlation
		energizeStats, err := storageAdapter.Sessions().GetEnergizeStats(ctx, weekStart, weekEnd)
		if err == nil && len(energizeStats) > 0 {
			fmt.Printf("  %s\n", dimStyle.Render("Energize vs Focus"))
			fmt.Printf("  %s\n", dimStyle.Render(strings.Repeat("─", 35)))
			for _, es := range energizeStats {
				fmt.Printf("  %-12s %s  %s\n",
					dimStyle.Render(es.Activity),
					valueStyle.Render(fmt.Sprintf("%.1f/5", es.AvgFocusScore)),
					dimStyle.Render(fmt.Sprintf("(%d sessions)", es.SessionCount)),
				)
			}
			fmt.Println()
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(reflectCmd)
}

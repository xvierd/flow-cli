package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var reflectTodayFlag bool

var reflectCmd = &cobra.Command{
	Use:   "reflect",
	Short: "Show a weekly reflection dashboard",
	Long:  `Display a reflection of your week: daily sessions, highlights, focus scores, and distraction trends.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		now := time.Now()

		if reflectTodayFlag {
			return runReflectToday(ctx, now)
		}

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

			stats, err := app.storage.Sessions().GetDailyStats(ctx, day)
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
		periodStats, err := app.storage.Sessions().GetPeriodStats(ctx, weekStart, weekEnd)
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
			highlight, err := app.storage.Tasks().FindTodayHighlight(ctx, day)
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
		energizeStats, err := app.storage.Sessions().GetEnergizeStats(ctx, weekStart, weekEnd)
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
	reflectCmd.Flags().BoolVar(&reflectTodayFlag, "today", false, "Show today's reflection summary")
}

// runReflectToday displays a summary of today's sessions, highlight, and focus scores.
func runReflectToday(ctx context.Context, now time.Time) error {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C6FE0"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	valueStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA"))

	fmt.Println()
	fmt.Printf("  %s\n", titleStyle.Render(fmt.Sprintf("Today's Reflection — %s", now.Format("Mon Jan 2"))))
	fmt.Printf("  %s\n\n", dimStyle.Render(strings.Repeat("─", 45)))

	// Today's stats
	stats, err := app.storage.Sessions().GetDailyStats(ctx, now)
	if err != nil {
		return fmt.Errorf("failed to get today's stats: %w", err)
	}

	fmt.Printf("  %s  %s\n", dimStyle.Render("Sessions:"), valueStyle.Render(fmt.Sprintf("%d", stats.WorkSessions)))
	fmt.Printf("  %s  %s\n", dimStyle.Render("Work time:"), valueStyle.Render(formatMinutes(stats.TotalWorkTime)))
	fmt.Println()

	// Today's highlight
	highlight, _ := app.storage.Tasks().FindTodayHighlight(ctx, now)
	if highlight != nil {
		status := dimStyle.Render("in progress")
		if highlight.Status == "completed" {
			status = valueStyle.Render("completed")
		}
		fmt.Printf("  %s  %s (%s)\n", dimStyle.Render("Highlight:"), valueStyle.Render(highlight.Title), status)
		fmt.Println()
	}

	// Today's focus scores from sessions
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	todayEnd := todayStart.AddDate(0, 0, 1)
	sessions, _ := app.storage.Sessions().FindRecent(ctx, todayStart)
	var focusScores []int
	var energizeActivities []string
	for _, s := range sessions {
		if s.StartedAt.After(todayEnd) {
			continue
		}
		if s.FocusScore != nil {
			focusScores = append(focusScores, *s.FocusScore)
		}
		if s.EnergizeActivity != "" {
			energizeActivities = append(energizeActivities, s.EnergizeActivity)
		}
	}

	if len(focusScores) > 0 {
		sum := 0
		for _, sc := range focusScores {
			sum += sc
		}
		avg := float64(sum) / float64(len(focusScores))
		fmt.Printf("  %s  %s  %s\n",
			dimStyle.Render("Avg focus:"),
			valueStyle.Render(fmt.Sprintf("%.1f/5", avg)),
			dimStyle.Render(fmt.Sprintf("(%d sessions)", len(focusScores))),
		)
	}

	if len(energizeActivities) > 0 {
		fmt.Printf("  %s  %s\n", dimStyle.Render("Energize:"), valueStyle.Render(strings.Join(energizeActivities, ", ")))
	}

	fmt.Println()
	return nil
}

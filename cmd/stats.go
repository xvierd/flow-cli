package cmd

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/services"
)

var statsPeriod string

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show a dashboard of session statistics",
	Long:  `Display a terminal dashboard with session counts, deep work hours, focus scores, and distraction trends.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		now := time.Now()

		period := services.ReportPeriodWeek
		if statsPeriod == "month" {
			period = services.ReportPeriodMonth
		} else {
			statsPeriod = "week"
		}

		start, end, err := services.PeriodRange(period, now)
		if err != nil {
			return err
		}
		label := i18n.T("Week of %s", fmt.Sprintf("%s %d", i18n.MonthName(start.Month()), start.Day()))
		if period == services.ReportPeriodMonth {
			label = fmt.Sprintf("%s %d", i18n.FullMonthName(now.Month()), now.Year())
		}

		stats, err := app.storage.Sessions().GetPeriodStats(ctx, start, end)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to get stats"), err)
		}
		stats.Label = label

		// Fetch hourly productivity (last 30 days)
		hourly, err := app.storage.Sessions().GetHourlyProductivity(ctx, 30)
		if err != nil {
			hourly = nil // non-fatal
		}

		// Fetch energize stats — only relevant for Make Time methodology
		var energize []domain.EnergizeStat
		if app.methodology == domain.MethodologyMakeTime {
			energize, err = app.storage.Sessions().GetEnergizeStats(ctx, start, end)
			if err != nil {
				energize = nil // non-fatal
			}
		}

		// Fetch Deep Work philosophy-specific stats
		var philosophy string
		var streak int
		var prevWeekHours time.Duration
		var monthHours time.Duration
		if app.methodology == domain.MethodologyDeepWork {
			philosophy = app.config.DeepWork.Philosophy
			if philosophy == "" {
				philosophy = "rhythmic"
			}

			switch philosophy {
			case "rhythmic":
				// Get streak (consecutive days with deep work)
				streak, _ = app.storage.Sessions().GetDeepWorkStreak(ctx, 1*time.Minute)
			case "bimodal", "journalistic":
				// Get previous week hours for comparison
				prevWeekStart := start.AddDate(0, 0, -7)
				prevWeekHours, _ = app.storage.Sessions().GetDeepWorkHours(ctx, prevWeekStart, start)
			case "monastic":
				// Get current month hours
				monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
				monthEnd := monthStart.AddDate(0, 1, 0)
				monthHours, _ = app.storage.Sessions().GetDeepWorkHours(ctx, monthStart, monthEnd)
			}
		}

		if jsonOutput {
			methodologies := make([]map[string]interface{}, 0, len(stats.ByMethodology))
			for _, m := range stats.ByMethodology {
				methodologies = append(methodologies, map[string]interface{}{
					"methodology":   string(m.Methodology),
					"session_count": m.SessionCount,
					"total_time":    m.TotalTime.String(),
				})
			}
			hourlyOut := make(map[string]string)
			for hour, dur := range hourly {
				hourlyOut[fmt.Sprintf("%02d:00", hour)] = dur.String()
			}
			return jsonOut(map[string]interface{}{
				"period":            string(period),
				"label":             stats.Label,
				"start":             start.Format(time.RFC3339),
				"end":               end.Format(time.RFC3339),
				"total_sessions":    stats.TotalSessions,
				"total_work_time":   stats.TotalWorkTime.String(),
				"avg_focus_score":   stats.AvgFocusScore,
				"focus_score_count": stats.FocusScoreCount,
				"distraction_count": stats.DistractionCount,
				"deep_work_streak":  streak,
				"total_prev_week":   prevWeekHours.String(),
				"total_month":       monthHours.String(),
				"by_methodology":    methodologies,
				"hourly":            hourlyOut,
			})
		}

		fmt.Println()
		renderDashboard(stats, hourly, energize, philosophy, streak, prevWeekHours, monthHours)
		return nil
	},
}

func init() {
	statsCmd.Flags().StringVarP(&statsPeriod, "period", "p", "week", "Time period: week or month")
}

func renderDashboard(stats *domain.PeriodStats, hourly map[int]time.Duration, energize []domain.EnergizeStat, philosophy string, streak int, prevWeekHours, monthHours time.Duration) {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C6FE0"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	valueStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA"))
	barColor := lipgloss.NewStyle().Foreground(lipgloss.Color("#7C6FE0"))

	// Header
	fmt.Printf("  %s\n", titleStyle.Render(stats.Label))
	fmt.Printf("  %s\n\n", dimStyle.Render(strings.Repeat("─", 40)))

	// Summary line
	hours := stats.TotalWorkTime.Hours()
	fmt.Printf("  %s\n\n", i18n.T("Total: %s sessions, %s deep work",
		valueStyle.Render(fmt.Sprintf("%d", stats.TotalSessions)),
		valueStyle.Render(formatHours(hours)),
	))

	if stats.TotalSessions == 0 {
		fmt.Printf("  %s\n\n", dimStyle.Render(i18n.T("No completed sessions in this period.")))
		return
	}

	// Philosophy-specific context for Deep Work
	if philosophy != "" {
		renderPhilosophyContext(philosophy, stats, streak, prevWeekHours, monthHours, dimStyle, valueStyle)
	}

	// Bar chart: sessions per methodology
	fmt.Printf("  %s\n", dimStyle.Render(i18n.T("Sessions by mode")))
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
			dimStyle.Render(i18n.T("Avg focus score:")),
			valueStyle.Render(fmt.Sprintf("%.1f/5", stats.AvgFocusScore)),
			dimStyle.Render(i18n.T("(%d sessions)", stats.FocusScoreCount)),
		)
	}

	// Distraction count (Deep Work)
	if stats.DistractionCount > 0 {
		fmt.Printf("  %s  %s\n",
			dimStyle.Render(i18n.T("Distractions:")),
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

// renderEnergizeInsights displays a table of energize activities vs avg focus score (Make Time).
// Results are sorted by avg focus score descending.
func renderEnergizeInsights(energize []domain.EnergizeStat, dimStyle, valueStyle, titleStyle lipgloss.Style) {
	if len(energize) == 0 {
		return
	}

	fmt.Printf("  %s\n", titleStyle.Render(i18n.T("Energize → Focus correlation")))
	for _, e := range energize {
		fmt.Printf("  %-12s  %-15s  %s %s\n",
			dimStyle.Render(e.Activity),
			dimStyle.Render(sessionCountLabel(e.SessionCount)),
			dimStyle.Render(i18n.T("avg focus")),
			valueStyle.Render(fmt.Sprintf("%.1f/5", e.AvgFocusScore)),
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

	fmt.Printf("  %s\n", dimStyle.Render(i18n.T("Your most productive hours (last 30 days)")))
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

// renderPhilosophyContext displays Deep Work philosophy-specific stats.
func renderPhilosophyContext(philosophy string, stats *domain.PeriodStats, streak int, prevWeekHours, monthHours time.Duration, dimStyle, valueStyle lipgloss.Style) {
	// Find Deep Work hours in this period
	var dwHours float64
	for _, m := range stats.ByMethodology {
		if m.Methodology == domain.MethodologyDeepWork {
			dwHours = m.TotalTime.Hours()
			break
		}
	}

	switch philosophy {
	case "rhythmic":
		// Show streak and daily goal progress.
		// goalHours is a daily target (e.g. 4h/day). Expected hours = goalHours × workdays elapsed.
		goalHours := app.config.DeepWork.DeepWorkGoalHours
		if goalHours == 0 {
			goalHours = 4.0
		}
		now := time.Now()
		weekday := int(now.Weekday()) // 0=Sun, 1=Mon, ..., 6=Sat
		// Workdays elapsed this week (Mon–Fri). Cap at 5 on weekends.
		workdaysPassed := weekday
		if workdaysPassed == 0 {
			workdaysPassed = 5 // Sunday — full week elapsed
		} else if workdaysPassed > 5 {
			workdaysPassed = 5 // Saturday — full week elapsed
		}
		expectedHours := goalHours * float64(workdaysPassed)
		progress := 0.0
		if expectedHours > 0 {
			progress = (dwHours / expectedHours) * 100
		}

		fmt.Printf("  %s  %s", dimStyle.Render(i18n.T("Streak:")), valueStyle.Render(i18n.T("%d days", streak)))
		if progress > 0 {
			fmt.Printf("  %s  %s", dimStyle.Render(i18n.T("Weekly progress:")), valueStyle.Render(fmt.Sprintf("%.0f%%", progress)))
		}
		fmt.Println()
		fmt.Println()

	case "journalistic":
		// Show this week vs last week — no daily pressure, just totals.
		fmt.Printf("  %s  %s", dimStyle.Render(i18n.T("This week:")), valueStyle.Render(formatHours(dwHours)))
		if prevWeekHours > 0 {
			change := ""
			if dwHours > prevWeekHours.Hours() {
				change = fmt.Sprintf("↑ %.0f%%", ((dwHours-prevWeekHours.Hours())/prevWeekHours.Hours())*100)
			} else if dwHours < prevWeekHours.Hours() {
				change = fmt.Sprintf("↓ %.0f%%", ((prevWeekHours.Hours()-dwHours)/prevWeekHours.Hours())*100)
			} else {
				change = "—"
			}
			fmt.Printf("  %s  %s  %s",
				dimStyle.Render(i18n.T("Last week:")),
				valueStyle.Render(formatHours(prevWeekHours.Hours())),
				valueStyle.Render(change),
			)
		}
		fmt.Printf("  %s\n\n", dimStyle.Render(i18n.T("(grab depth when you can)")))

	case "bimodal":
		// Show this week vs last week
		fmt.Printf("  %s  %s", dimStyle.Render(i18n.T("This week:")), valueStyle.Render(formatHours(dwHours)))
		if prevWeekHours > 0 {
			change := ""
			if dwHours > prevWeekHours.Hours() {
				change = fmt.Sprintf("↑ %.0f%%", ((dwHours-prevWeekHours.Hours())/prevWeekHours.Hours())*100)
			} else if dwHours < prevWeekHours.Hours() {
				change = fmt.Sprintf("↓ %.0f%%", ((prevWeekHours.Hours()-dwHours)/prevWeekHours.Hours())*100)
			} else {
				change = "—"
			}
			fmt.Printf("  %s  %s  %s",
				dimStyle.Render(i18n.T("Last week:")),
				valueStyle.Render(formatHours(prevWeekHours.Hours())),
				valueStyle.Render(change),
			)
		}
		fmt.Println()
		fmt.Println()

	case "monastic":
		// Show monthly hours
		fmt.Printf("  %s  %s  %s\n\n",
			dimStyle.Render(i18n.T("Monthly Deep Work:")),
			valueStyle.Render(formatHours(monthHours.Hours())),
			dimStyle.Render(i18n.T("(monastic view)")),
		)
	}
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

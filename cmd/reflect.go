package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/services"
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
			if jsonOutput {
				return outputReflectTodayJSON(ctx, now)
			}
			return runReflectToday(ctx, now)
		}

		// Compute week start (Monday)
		weekStart, weekEnd, err := services.PeriodRange(services.ReportPeriodWeek, now)
		if err != nil {
			return err
		}

		if jsonOutput {
			return outputReflectJSON(ctx, weekStart, weekEnd, now)
		}

		titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C6FE0"))
		dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
		valueStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA"))
		accentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399"))

		fmt.Println()
		fmt.Printf("  %s\n", titleStyle.Render(i18n.T("Weekly Reflection — %s", fmt.Sprintf("%s %d", i18n.MonthName(weekStart.Month()), weekStart.Day()))))
		fmt.Printf("  %s\n\n", dimStyle.Render(strings.Repeat("─", 45)))

		// Day-by-day breakdown
		fmt.Printf("  %s\n", dimStyle.Render(i18n.T("Day       Sessions   Work Time")))
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
			dayLabel := i18n.DayName(day.Weekday())
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
			dimStyle.Render(i18n.T("Total")),
			valueStyle.Render(fmt.Sprintf("%-8d", totalSessions)),
			valueStyle.Render(formatMinutes(totalWork)),
		)

		// Period stats for focus score and distractions
		periodStats, err := app.storage.Sessions().GetPeriodStats(ctx, weekStart, weekEnd)
		if err == nil {
			if periodStats.FocusScoreCount > 0 {
				fmt.Printf("  %s  %s  %s\n",
					dimStyle.Render(i18n.T("Avg focus score:")),
					valueStyle.Render(fmt.Sprintf("%.1f/5", periodStats.AvgFocusScore)),
					dimStyle.Render(i18n.T("(%d sessions)", periodStats.FocusScoreCount)),
				)
			}

			if periodStats.DistractionCount > 0 {
				fmt.Printf("  %s  %s\n",
					dimStyle.Render(i18n.T("Distractions:")),
					valueStyle.Render(fmt.Sprintf("%d", periodStats.DistractionCount)),
				)
			}

			if periodStats.FocusScoreCount > 0 || periodStats.DistractionCount > 0 {
				fmt.Println()
			}
		}

		// Highlights for the week
		fmt.Printf("  %s\n", dimStyle.Render(i18n.T("Highlights this week")))
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
				dimStyle.Render(i18n.DayName(day.Weekday())),
				status,
				valueStyle.Render(highlight.Title),
			)
		}
		if !foundHighlight {
			fmt.Printf("  %s\n", dimStyle.Render(i18n.T("No highlights set this week.")))
		}
		fmt.Println()

		// Energize correlation
		energizeStats, err := app.storage.Sessions().GetEnergizeStats(ctx, weekStart, weekEnd)
		if err == nil && len(energizeStats) > 0 {
			fmt.Printf("  %s\n", dimStyle.Render(i18n.T("Energize vs Focus")))
			fmt.Printf("  %s\n", dimStyle.Render(strings.Repeat("─", 35)))
			for _, es := range energizeStats {
				fmt.Printf("  %-12s %s  %s\n",
					dimStyle.Render(es.Activity),
					valueStyle.Render(fmt.Sprintf("%.1f/5", es.AvgFocusScore)),
					dimStyle.Render(i18n.T("(%d sessions)", es.SessionCount)),
				)
			}
			fmt.Println()
		}

		return nil
	},
}

func init() {
	reflectCmd.Flags().BoolVar(&reflectTodayFlag, "today", false, "Show today's reflection summary")
}

// outputReflectJSON emits the weekly reflection data as JSON.
func outputReflectJSON(ctx context.Context, weekStart, weekEnd, now time.Time) error {
	days := make([]map[string]interface{}, 0, 7)
	highlights := make([]map[string]interface{}, 0, 7)
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
		days = append(days, map[string]interface{}{
			"date":      day.Format("2006-01-02"),
			"sessions":  stats.WorkSessions,
			"work_time": formatMinutes(stats.TotalWorkTime),
		})
		totalSessions += stats.WorkSessions
		totalWork += stats.TotalWorkTime

		if hl, err := app.storage.Tasks().FindTodayHighlight(ctx, day); err == nil && hl != nil {
			highlights = append(highlights, map[string]interface{}{
				"date":   day.Format("2006-01-02"),
				"title":  hl.Title,
				"status": string(hl.Status),
			})
		}
	}

	periodStats, err := app.storage.Sessions().GetPeriodStats(ctx, weekStart, weekEnd)
	if err != nil {
		periodStats = &domain.PeriodStats{}
	}

	energize, _ := app.storage.Sessions().GetEnergizeStats(ctx, weekStart, weekEnd)
	energizeOut := make([]map[string]interface{}, 0, len(energize))
	for _, e := range energize {
		energizeOut = append(energizeOut, map[string]interface{}{
			"activity":      e.Activity,
			"avg_focus":     e.AvgFocusScore,
			"session_count": e.SessionCount,
		})
	}

	return jsonOut(map[string]interface{}{
		"week_start":        weekStart.Format(time.RFC3339),
		"week_end":          weekEnd.Format(time.RFC3339),
		"days":              days,
		"total_sessions":    totalSessions,
		"total_work_time":   totalWork.String(),
		"avg_focus_score":   periodStats.AvgFocusScore,
		"focus_score_count": periodStats.FocusScoreCount,
		"distraction_count": periodStats.DistractionCount,
		"highlights":        highlights,
		"energize":          energizeOut,
	})
}

// outputReflectTodayJSON renders today's daily stats as JSON.
func outputReflectTodayJSON(ctx context.Context, now time.Time) error {
	stats, err := app.state.GetDailySummary(ctx, now)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to get today's stats"), err)
	}
	return jsonOut(map[string]interface{}{
		"date":            now.Format("2006-01-02"),
		"work_sessions":   stats.WorkSessions,
		"breaks_taken":    stats.BreaksTaken,
		"total_work_time": stats.TotalWorkTime.String(),
		"tasks_completed": stats.TasksCompleted,
	})
}

// runReflectToday displays a methodology-aware summary of today's sessions with interactive prompts.
func runReflectToday(ctx context.Context, now time.Time) error {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C6FE0"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	valueStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA"))
	accentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399"))

	fmt.Println()
	fmt.Printf("  %s\n", titleStyle.Render(i18n.T("Today's Reflection — %s", fmt.Sprintf("%s %s %d", i18n.DayName(now.Weekday()), i18n.MonthName(now.Month()), now.Day()))))
	fmt.Printf("  %s\n\n", dimStyle.Render(strings.Repeat("─", 45)))

	// Today's stats (common to all methodologies)
	stats, err := app.storage.Sessions().GetDailyStats(ctx, now)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to get today's stats"), err)
	}

	fmt.Printf("  %s  %s\n", dimStyle.Render(i18n.T("Sessions:")), valueStyle.Render(fmt.Sprintf("%d", stats.WorkSessions)))
	fmt.Printf("  %s  %s\n", dimStyle.Render(i18n.T("Work time:")), valueStyle.Render(formatMinutes(stats.TotalWorkTime)))
	fmt.Println()

	// Fetch today's sessions for methodology-specific stats and persistence
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	todayEnd := todayStart.AddDate(0, 0, 1)
	sessions, _ := app.storage.Sessions().FindRecent(ctx, todayStart)

	// Filter to today only
	var todaySessions []*domain.PomodoroSession
	for _, s := range sessions {
		if !s.StartedAt.After(todayEnd) {
			todaySessions = append(todaySessions, s)
		}
	}

	// Find today's last work session (for persistence)
	var lastWorkSession *domain.PomodoroSession
	for i := len(todaySessions) - 1; i >= 0; i-- {
		if todaySessions[i].IsWorkSession() {
			lastWorkSession = todaySessions[i]
			break
		}
	}

	// Branch by methodology
	switch app.methodology {
	case domain.MethodologyMakeTime:
		runReflectMakeTime(ctx, now, stats, todaySessions, lastWorkSession, dimStyle, valueStyle, accentStyle)
	case domain.MethodologyDeepWork:
		runReflectDeepWork(ctx, now, stats, todaySessions, lastWorkSession, dimStyle, valueStyle)
	default:
		runReflectPomodoro(ctx, todaySessions, lastWorkSession, dimStyle, valueStyle)
	}

	return nil
}

// runReflectPomodoro shows pomodoro-specific reflection.
func runReflectPomodoro(
	ctx context.Context,
	todaySessions []*domain.PomodoroSession,
	lastWorkSession *domain.PomodoroSession,
	dimStyle, valueStyle lipgloss.Style,
) {
	// Count completed and interrupted work sessions
	var completed, interrupted int
	for _, s := range todaySessions {
		if !s.IsWorkSession() {
			continue
		}
		switch s.Status {
		case domain.SessionStatusCompleted:
			completed++
		case domain.SessionStatusInterrupted:
			interrupted++
		}
	}

	fmt.Printf("  %s\n", dimStyle.Render(i18n.T("— Pomodoro Review —")))
	fmt.Println()
	interruptedStr := ""
	if interrupted > 0 {
		interruptedStr = i18n.T(" · %d interrupted", interrupted)
	}
	fmt.Printf("  %s\n", i18n.T("%s sessions completed today%s",
		valueStyle.Render(fmt.Sprintf("%d", completed)),
		dimStyle.Render(interruptedStr),
	))
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("  %s ", dimStyle.Render(i18n.T("What did you learn today? (Enter to skip):")))
	if scanner.Scan() {
		answer := strings.TrimSpace(scanner.Text())
		if answer != "" && lastWorkSession != nil {
			_ = app.pomodoro.SetAccomplishment(ctx, lastWorkSession.ID, answer)
		}
	}
	fmt.Println()
}

// runReflectMakeTime shows Make Time specific reflection with highlight review.
func runReflectMakeTime(
	ctx context.Context,
	now time.Time,
	stats *domain.DailyStats,
	todaySessions []*domain.PomodoroSession,
	lastWorkSession *domain.PomodoroSession,
	dimStyle, valueStyle, accentStyle lipgloss.Style,
) {
	fmt.Printf("  %s\n", dimStyle.Render(i18n.T("— Make Time · Reflect —")))
	fmt.Println()

	// Show today's highlight
	highlight, _ := app.storage.Tasks().FindTodayHighlight(ctx, now)
	if highlight != nil {
		statusLabel := dimStyle.Render(i18n.T("in progress"))
		if highlight.Status == "completed" {
			statusLabel = accentStyle.Render(i18n.T("completed"))
		}
		fmt.Printf("  %s  %s (%s)\n",
			dimStyle.Render(i18n.T("Today's Highlight:")),
			valueStyle.Render(fmt.Sprintf("%q", highlight.Title)),
			statusLabel,
		)
	} else {
		fmt.Printf("  %s  %s\n",
			dimStyle.Render(i18n.T("Today's Highlight:")),
			dimStyle.Render(i18n.T("No highlight set")),
		)
	}

	// Show highlight target progress if configured
	if app.config.MakeTime.HighlightTargetMinutes > 0 {
		target := time.Duration(app.config.MakeTime.HighlightTargetMinutes) * time.Minute
		pct := 0
		if target > 0 {
			pct = int(stats.TotalWorkTime * 100 / target)
		}
		if pct > 100 {
			pct = 100
		}
		fmt.Printf("  %s  %s %s\n",
			dimStyle.Render(i18n.T("Highlight target:")),
			valueStyle.Render(formatMinutes(target)),
			dimStyle.Render(i18n.T("(%d%% complete)", pct)),
		)
	}

	// Show focus scores
	var focusScores []int
	for _, s := range todaySessions {
		if s.FocusScore != nil {
			focusScores = append(focusScores, *s.FocusScore)
		}
	}
	if len(focusScores) > 0 {
		sum := 0
		for _, sc := range focusScores {
			sum += sc
		}
		avg := float64(sum) / float64(len(focusScores))
		fmt.Printf("  %s  %s  %s\n",
			dimStyle.Render(i18n.T("Avg focus:")),
			valueStyle.Render(fmt.Sprintf("%.1f/5", avg)),
			dimStyle.Render(i18n.T("(%d sessions)", len(focusScores))),
		)
	}

	fmt.Println()

	// Interactive reflection prompts
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Printf("  %s ", dimStyle.Render(i18n.T("Did you make time for your Highlight? (y/n, Enter to skip):")))
	if scanner.Scan() {
		// Display-only, no persistence needed
		_ = strings.TrimSpace(scanner.Text())
	}

	fmt.Printf("  %s ", dimStyle.Render(i18n.T("What worked well today? (Enter to skip):")))
	if scanner.Scan() {
		answer := strings.TrimSpace(scanner.Text())
		if answer != "" && lastWorkSession != nil {
			_ = app.pomodoro.SetAccomplishment(ctx, lastWorkSession.ID, answer)
		}
	}

	fmt.Printf("  %s ", dimStyle.Render(i18n.T("What would you change tomorrow? (Enter to skip):")))
	if scanner.Scan() {
		// Display-only prompt; no domain field for this yet
		_ = strings.TrimSpace(scanner.Text())
	}

	fmt.Println()
	fmt.Printf("  %s\n", accentStyle.Render(i18n.T("Reflection complete. Tomorrow, pick a new Highlight.")))
	fmt.Println()
}

// runReflectDeepWork shows Deep Work specific reflection with depth tracking.
func runReflectDeepWork(
	ctx context.Context,
	now time.Time,
	stats *domain.DailyStats,
	todaySessions []*domain.PomodoroSession,
	lastWorkSession *domain.PomodoroSession,
	dimStyle, valueStyle lipgloss.Style,
) {
	fmt.Printf("  %s\n", dimStyle.Render(i18n.T("— Deep Work · Review —")))
	fmt.Println()

	// Show depth vs goal
	goalHours := app.config.DeepWork.DeepWorkGoalHours
	if goalHours > 0 {
		depthHours := stats.TotalWorkTime.Hours()
		fmt.Printf("  %s  %s\n",
			dimStyle.Render(i18n.T("Depth today:")),
			valueStyle.Render(i18n.T("%.1fh / %.1fh (goal)", depthHours, goalHours)),
		)
	}

	// Show streak
	threshold := time.Duration(goalHours * float64(time.Hour))
	streak, err := app.pomodoro.GetDeepWorkStreak(ctx, threshold)
	if err == nil && streak > 0 {
		fmt.Printf("  %s  %s\n",
			dimStyle.Render(i18n.T("Streak:")),
			valueStyle.Render(i18n.T("%d days", streak)),
		)
	}

	// Show focus scores
	var focusScores []int
	for _, s := range todaySessions {
		if s.FocusScore != nil {
			focusScores = append(focusScores, *s.FocusScore)
		}
	}
	if len(focusScores) > 0 {
		sum := 0
		for _, sc := range focusScores {
			sum += sc
		}
		avg := float64(sum) / float64(len(focusScores))
		fmt.Printf("  %s  %s  %s\n",
			dimStyle.Render(i18n.T("Avg focus:")),
			valueStyle.Render(fmt.Sprintf("%.1f/5", avg)),
			dimStyle.Render(i18n.T("(%d sessions)", len(focusScores))),
		)
	}

	fmt.Println()

	// Interactive shutdown ritual question
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("  %s ", dimStyle.Render(i18n.T("Did you complete the shutdown ritual? (y/n, Enter to skip):")))
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if (answer == "y" || answer == "s") && lastWorkSession != nil {
			// Record that shutdown ritual was completed
			_ = app.pomodoro.SetShutdownRitual(ctx, lastWorkSession.ID, domain.ShutdownRitual{
				ClosingPhrase: "Shutdown complete",
			})
		}
	}
	fmt.Println()
}

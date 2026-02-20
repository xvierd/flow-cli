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

// runReflectToday displays a methodology-aware summary of today's sessions with interactive prompts.
func runReflectToday(ctx context.Context, now time.Time) error {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C6FE0"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	valueStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA"))
	accentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399"))

	fmt.Println()
	fmt.Printf("  %s\n", titleStyle.Render(fmt.Sprintf("Today's Reflection — %s", now.Format("Mon Jan 2"))))
	fmt.Printf("  %s\n\n", dimStyle.Render(strings.Repeat("─", 45)))

	// Today's stats (common to all methodologies)
	stats, err := app.storage.Sessions().GetDailyStats(ctx, now)
	if err != nil {
		return fmt.Errorf("failed to get today's stats: %w", err)
	}

	fmt.Printf("  %s  %s\n", dimStyle.Render("Sessions:"), valueStyle.Render(fmt.Sprintf("%d", stats.WorkSessions)))
	fmt.Printf("  %s  %s\n", dimStyle.Render("Work time:"), valueStyle.Render(formatMinutes(stats.TotalWorkTime)))
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

	fmt.Printf("  %s\n", dimStyle.Render("— Pomodoro Review —"))
	fmt.Println()
	interruptedStr := ""
	if interrupted > 0 {
		interruptedStr = fmt.Sprintf(" · %d interrupted", interrupted)
	}
	fmt.Printf("  %s %s completed today%s\n",
		valueStyle.Render(fmt.Sprintf("%d", completed)),
		dimStyle.Render("sessions"),
		dimStyle.Render(interruptedStr),
	)
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("  %s ", dimStyle.Render("Que aprendiste hoy? (Enter to skip):"))
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
	fmt.Printf("  %s\n", dimStyle.Render("— Make Time · Reflect —"))
	fmt.Println()

	// Show today's highlight
	highlight, _ := app.storage.Tasks().FindTodayHighlight(ctx, now)
	if highlight != nil {
		statusLabel := dimStyle.Render("in progress")
		if highlight.Status == "completed" {
			statusLabel = accentStyle.Render("completed")
		}
		fmt.Printf("  %s  %s (%s)\n",
			dimStyle.Render("Highlight de hoy:"),
			valueStyle.Render(fmt.Sprintf("%q", highlight.Title)),
			statusLabel,
		)
	} else {
		fmt.Printf("  %s  %s\n",
			dimStyle.Render("Highlight de hoy:"),
			dimStyle.Render("No highlight set"),
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
			dimStyle.Render("Highlight target:"),
			valueStyle.Render(formatMinutes(target)),
			dimStyle.Render(fmt.Sprintf("(%d%% complete)", pct)),
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
			dimStyle.Render("Avg focus:"),
			valueStyle.Render(fmt.Sprintf("%.1f/5", avg)),
			dimStyle.Render(fmt.Sprintf("(%d sessions)", len(focusScores))),
		)
	}

	fmt.Println()

	// Interactive reflection prompts
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Printf("  %s ", dimStyle.Render("Hiciste tiempo para tu Highlight? (s/n, Enter to skip):"))
	if scanner.Scan() {
		// Display-only, no persistence needed
		_ = strings.TrimSpace(scanner.Text())
	}

	fmt.Printf("  %s ", dimStyle.Render("Que funciono bien hoy? (Enter to skip):"))
	if scanner.Scan() {
		answer := strings.TrimSpace(scanner.Text())
		if answer != "" && lastWorkSession != nil {
			_ = app.pomodoro.SetAccomplishment(ctx, lastWorkSession.ID, answer)
		}
	}

	fmt.Printf("  %s ", dimStyle.Render("Que cambiarias manana? (Enter to skip):"))
	if scanner.Scan() {
		// Display-only prompt; no domain field for this yet
		_ = strings.TrimSpace(scanner.Text())
	}

	fmt.Println()
	fmt.Printf("  %s\n", accentStyle.Render("Reflect completado. Manana elige un nuevo Highlight."))
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
	fmt.Printf("  %s\n", dimStyle.Render("— Deep Work · Review —"))
	fmt.Println()

	// Show depth vs goal
	goalHours := app.config.DeepWork.DeepWorkGoalHours
	if goalHours > 0 {
		depthHours := stats.TotalWorkTime.Hours()
		fmt.Printf("  %s  %s\n",
			dimStyle.Render("Profundidad hoy:"),
			valueStyle.Render(fmt.Sprintf("%.1fh / %.1fh (goal)", depthHours, goalHours)),
		)
	}

	// Show streak
	threshold := time.Duration(goalHours * float64(time.Hour))
	streak, err := app.pomodoro.GetDeepWorkStreak(ctx, threshold)
	if err == nil && streak > 0 {
		fmt.Printf("  %s  %s\n",
			dimStyle.Render("Racha:"),
			valueStyle.Render(fmt.Sprintf("%d dias", streak)),
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
			dimStyle.Render("Avg focus:"),
			valueStyle.Render(fmt.Sprintf("%.1f/5", avg)),
			dimStyle.Render(fmt.Sprintf("(%d sessions)", len(focusScores))),
		)
	}

	fmt.Println()

	// Interactive shutdown ritual question
	scanner := bufio.NewScanner(os.Stdin)
	shutdownQ := "Completaste el shutdown ritual? (s/n, Enter to skip):" //nolint:misspell
	fmt.Printf("  %s ", dimStyle.Render(shutdownQ))
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer == "s" && lastWorkSession != nil {
			// Record that shutdown ritual was completed
			_ = app.pomodoro.SetShutdownRitual(ctx, lastWorkSession.ID, domain.ShutdownRitual{
				ClosingPhrase: "Shutdown complete",
			})
		}
	}
	fmt.Println()
}

package cmd

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/services"
)

var (
	exportFormat string
	exportPeriod string
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export session history",
	Long:  "Export your session history in markdown or CSV format.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runExport(cmd.Context())
	},
}

func init() {
	exportCmd.Flags().StringVar(&exportFormat, "format", "md", "Output format: md or csv")
	exportCmd.Flags().StringVar(&exportPeriod, "period", "week", "Time period: week, month, or all")
}

func runExport(ctx context.Context) error {
	now := time.Now()
	var since time.Time
	switch exportPeriod {
	case "week":
		start, _, err := services.PeriodRange(services.ReportPeriodWeek, now)
		if err != nil {
			return err
		}
		since = start
	case "month":
		start, _, err := services.PeriodRange(services.ReportPeriodMonth, now)
		if err != nil {
			return err
		}
		since = start
	default: // "all"
		since = time.Time{}
	}

	sessions, err := app.storage.Sessions().FindRecent(ctx, since)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to fetch sessions"), err)
	}

	switch exportFormat {
	case "csv":
		return exportCSV(sessions)
	default:
		return exportMarkdown(sessions)
	}
}

func exportMarkdown(sessions []*domain.PomodoroSession) error {
	fmt.Printf("# %s\n\n", i18n.T("Flow Session Export"))
	fmt.Printf("%s\n\n", i18n.T("Generated: %s", time.Now().Format("2006-01-02 15:04")))

	for _, s := range sessions {
		if !s.IsWorkSession() {
			continue
		}
		fmt.Printf("## %s — %s\n", s.StartedAt.Format("2006-01-02"), s.Methodology)
		fmt.Printf("- %s\n", i18n.T("Duration: %s", s.Duration.String()))
		if s.IntendedOutcome != "" {
			fmt.Printf("- %s\n", i18n.T("Goal: %s", s.IntendedOutcome))
		}
		if s.Accomplishment != "" {
			fmt.Printf("- %s\n", i18n.T("Accomplished: %s", s.Accomplishment))
		}
		if s.FocusScore != nil {
			fmt.Printf("- %s\n", i18n.T("Focus: %d/5", *s.FocusScore))
		}
		if s.EnergizeActivity != "" {
			fmt.Printf("- %s\n", i18n.T("Energize: %s", s.EnergizeActivity))
		}
		if len(s.Distractions) > 0 {
			fmt.Printf("- %s\n", i18n.T("Distractions (%d):", len(s.Distractions)))
			for _, d := range s.Distractions {
				if d.Category != "" {
					fmt.Printf("  - [%s] %s\n", d.Category, d.Text)
				} else {
					fmt.Printf("  - %s\n", d.Text)
				}
			}
		}
		if s.ShutdownRitual != nil {
			if s.ShutdownRitual.PendingTasksReview != "" {
				fmt.Printf("- %s\n", i18n.T("Pending review: %s", s.ShutdownRitual.PendingTasksReview))
			}
			if s.ShutdownRitual.CalendarReview != "" {
				fmt.Printf("- %s\n", i18n.T("Calendar review: %s", s.ShutdownRitual.CalendarReview))
			}
			if s.ShutdownRitual.TomorrowPlan != "" {
				fmt.Printf("- %s\n", i18n.T("Tomorrow: %s", s.ShutdownRitual.TomorrowPlan))
			}
			if s.ShutdownRitual.ClosingPhrase != "" {
				fmt.Printf("- %s\n", i18n.T("Closing: %s", s.ShutdownRitual.ClosingPhrase))
			}
		}
		fmt.Println()
	}
	return nil
}

func exportCSV(sessions []*domain.PomodoroSession) error {
	w := csv.NewWriter(os.Stdout)

	if err := w.Write([]string{
		"date", "methodology", "duration_min", "goal", "accomplished",
		"focus_score", "tags", "energize_activity", "distraction_count",
		"distractions", "pending_tasks_review", "calendar_review", "tomorrow_plan",
	}); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to write CSV header"), err)
	}

	for _, s := range sessions {
		if !s.IsWorkSession() {
			continue
		}
		focusScore := ""
		if s.FocusScore != nil {
			focusScore = fmt.Sprintf("%d", *s.FocusScore)
		}
		distractionCount := fmt.Sprintf("%d", len(s.Distractions))
		var distractionTexts []string
		for _, d := range s.Distractions {
			distractionTexts = append(distractionTexts, d.Text)
		}
		pendingTasksReview, calendarReview, tomorrowPlan := "", "", ""
		if s.ShutdownRitual != nil {
			pendingTasksReview = s.ShutdownRitual.PendingTasksReview
			calendarReview = s.ShutdownRitual.CalendarReview
			tomorrowPlan = s.ShutdownRitual.TomorrowPlan
		}
		if err := w.Write([]string{
			s.StartedAt.Format("2006-01-02"),
			string(s.Methodology),
			fmt.Sprintf("%.0f", s.Duration.Minutes()),
			s.IntendedOutcome,
			s.Accomplishment,
			focusScore,
			strings.Join(s.Tags, ";"),
			s.EnergizeActivity,
			distractionCount,
			strings.Join(distractionTexts, "; "),
			pendingTasksReview,
			calendarReview,
			tomorrowPlan,
		}); err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to write CSV row"), err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to flush CSV output"), err)
	}
	return nil
}

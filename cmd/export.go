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
	rootCmd.AddCommand(exportCmd)
	exportCmd.Flags().StringVar(&exportFormat, "format", "md", "Output format: md or csv")
	exportCmd.Flags().StringVar(&exportPeriod, "period", "week", "Time period: week, month, or all")
}

func runExport(ctx context.Context) error {
	var since time.Time
	switch exportPeriod {
	case "week":
		since = time.Now().AddDate(0, 0, -7)
	case "month":
		since = time.Now().AddDate(0, -1, 0)
	default: // "all"
		since = time.Time{}
	}

	sessions, err := app.storage.Sessions().FindRecent(ctx, since)
	if err != nil {
		return fmt.Errorf("failed to fetch sessions: %w", err)
	}

	switch exportFormat {
	case "csv":
		return exportCSV(sessions)
	default:
		return exportMarkdown(sessions)
	}
}

func exportMarkdown(sessions []*domain.PomodoroSession) error {
	fmt.Printf("# Flow Session Export\n\n")
	fmt.Printf("Generated: %s\n\n", time.Now().Format("2006-01-02 15:04"))

	for _, s := range sessions {
		if !s.IsWorkSession() {
			continue
		}
		fmt.Printf("## %s — %s\n", s.StartedAt.Format("2006-01-02"), s.Methodology)
		fmt.Printf("- Duration: %s\n", s.Duration.String())
		if s.IntendedOutcome != "" {
			fmt.Printf("- Goal: %s\n", s.IntendedOutcome)
		}
		if s.Accomplishment != "" {
			fmt.Printf("- Accomplished: %s\n", s.Accomplishment)
		}
		if s.FocusScore != nil {
			fmt.Printf("- Focus: %d/5\n", *s.FocusScore)
		}
		if s.EnergizeActivity != "" {
			fmt.Printf("- Energize: %s\n", s.EnergizeActivity)
		}
		if len(s.Distractions) > 0 {
			fmt.Printf("- Distractions (%d):\n", len(s.Distractions))
			for _, d := range s.Distractions {
				if d.Category != "" {
					fmt.Printf("  - [%s] %s\n", d.Category, d.Text)
				} else {
					fmt.Printf("  - %s\n", d.Text)
				}
			}
		}
		if s.ShutdownRitual != nil {
			if s.ShutdownRitual.TomorrowPlan != "" {
				fmt.Printf("- Tomorrow: %s\n", s.ShutdownRitual.TomorrowPlan)
			}
			if s.ShutdownRitual.PendingTasksReview != "" {
				fmt.Printf("- Pending review: %s\n", s.ShutdownRitual.PendingTasksReview)
			}
			if s.ShutdownRitual.ClosingPhrase != "" {
				fmt.Printf("- Closing: %s\n", s.ShutdownRitual.ClosingPhrase)
			}
		}
		fmt.Println()
	}
	return nil
}

func exportCSV(sessions []*domain.PomodoroSession) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()

	_ = w.Write([]string{
		"date", "methodology", "duration_min", "goal", "accomplished",
		"focus_score", "tags", "energize_activity", "distraction_count",
		"distractions", "tomorrow_plan",
	})

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
		tomorrowPlan := ""
		if s.ShutdownRitual != nil {
			tomorrowPlan = s.ShutdownRitual.TomorrowPlan
		}
		_ = w.Write([]string{
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
			tomorrowPlan,
		})
	}
	return nil
}

package cmd

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/services"
)

var (
	reportMonth  bool
	reportWeek   bool
	reportFormat string
	reportOut    string
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Show an aggregated report for the period",
	Long:  `Build a full report over the current week or month: summary, daily breakdown, 24h heatmap, top tags, methodology stats, and streaks.`,
	RunE:  runReport,
}

func init() {
	reportCmd.Flags().BoolVar(&reportMonth, "month", false, "Report over the current month instead of the week")
	reportCmd.Flags().BoolVar(&reportWeek, "week", false, "Report over the current week (default)")
	reportCmd.Flags().StringVarP(&reportFormat, "format", "f", "terminal", "Output format: terminal, md, or csv")
	reportCmd.Flags().StringVarP(&reportOut, "out", "o", "", "Write output to a file instead of stdout")
}

func runReport(cmd *cobra.Command, args []string) error {
	period := services.ReportPeriodWeek
	if reportMonth {
		period = services.ReportPeriodMonth
	}

	ctx := context.Background()
	svc := services.NewReportService(app.storage)
	report, err := svc.GetReport(ctx, period)
	if err != nil {
		return fmt.Errorf("failed to build report: %w", err)
	}

	var buf bytes.Buffer
	switch {
	case jsonOutput:
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return fmt.Errorf("failed to encode JSON: %w", err)
		}
	case reportFormat == "terminal":
		renderTerminalReport(&buf, report)
	case reportFormat == "md":
		renderMarkdownReport(&buf, report)
	case reportFormat == "csv":
		if err := renderCSVReport(&buf, report); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown format %q: use terminal, md, or csv", reportFormat)
	}

	out := os.Stdout
	if reportOut != "" {
		f, err := os.Create(reportOut)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer func() { _ = f.Close() }()
		out = f
	}
	_, err = out.Write(buf.Bytes())
	return err
}

// -- Markdown rendering -----------------------------------------------------

func renderMarkdownReport(w *bytes.Buffer, r *domain.Report) {
	fmt.Fprintf(w, "# %s\n\n", r.Label)
	fmt.Fprintf(w, "- Period: %s\n", r.Period)
	fmt.Fprintf(w, "- Range: %s → %s\n",
		r.Start.Format("2006-01-02"),
		r.End.Add(-time.Second).Format("2006-01-02"),
	)
	fmt.Fprintf(w, "- Total sessions: %d\n", r.Summary.TotalSessions)
	fmt.Fprintf(w, "- Total work: %.1fh\n", r.Summary.TotalWorkTime.Hours())
	if r.Summary.FocusScoreCount > 0 {
		fmt.Fprintf(w, "- Avg focus: %.1f/5 (%d sessions)\n", r.Summary.AvgFocusScore, r.Summary.FocusScoreCount)
	}
	if r.Summary.DistractionCount > 0 {
		fmt.Fprintf(w, "- Distractions: %d\n", r.Summary.DistractionCount)
	}
	if r.Summary.HighlightDays > 0 {
		fmt.Fprintf(w, "- Highlights set: %d days\n", r.Summary.HighlightDays)
	}
	fmt.Fprintf(w, "- Streak: %d days (longest %d)\n\n", r.Streaks.CurrentDays, r.Streaks.LongestDays)

	fmt.Fprintf(w, "## Daily\n\n")
	fmt.Fprintf(w, "| Day | Sessions | Breaks | Work time | Avg focus | Distractions | Highlight |\n")
	fmt.Fprintf(w, "|---|---|---|---|---|---|---|\n")
	for _, d := range r.Daily {
		focus := ""
		if d.FocusCount > 0 {
			focus = fmt.Sprintf("%.1f", d.AvgFocusScore)
		}
		highlight := ""
		if d.HasHighlight {
			highlight = "yes"
		}
		fmt.Fprintf(w, "| %s | %d | %d | %s | %s | %d | %s |\n",
			d.Day, d.WorkSessions, d.Breaks, formatMinutes(d.WorkTime), focus, d.Distractions, highlight)
	}
	fmt.Fprintln(w)

	if len(r.Heatmap) > 0 {
		fmt.Fprintf(w, "## Deep work heatmap\n\n")
		fmt.Fprintf(w, "| Hour | Work time |\n")
		fmt.Fprintf(w, "|---|---|\n")
		for _, h := range r.Heatmap {
			fmt.Fprintf(w, "| %s | %s |\n", h.Hour, formatMinutes(h.Minutes))
		}
		fmt.Fprintln(w)
	}

	if len(r.Tags) > 0 {
		fmt.Fprintf(w, "## Top tags\n\n")
		fmt.Fprintf(w, "| Tag | Time | Sessions |\n")
		fmt.Fprintf(w, "|---|---|---|\n")
		for _, t := range r.Tags {
			fmt.Fprintf(w, "| %s | %s | %d |\n", t.Tag, formatMinutes(t.TotalTime), t.SessionCount)
		}
		fmt.Fprintln(w)
	}

	if len(r.Methodologies) > 0 {
		fmt.Fprintf(w, "## By methodology\n\n")
		fmt.Fprintf(w, "| Methodology | Sessions | Work time | Avg focus |\n")
		fmt.Fprintf(w, "|---|---|---|---|\n")
		for _, m := range r.Methodologies {
			focus := ""
			if m.FocusScoreCount > 0 {
				focus = fmt.Sprintf("%.1f", m.AvgFocusScore)
			}
			fmt.Fprintf(w, "| %s | %d | %s | %s |\n", m.Label, m.SessionCount, formatMinutes(m.TotalTime), focus)
		}
		fmt.Fprintln(w)
	}
}

func renderTerminalReport(w *bytes.Buffer, r *domain.Report) {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C6FE0"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	valueStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA"))
	accentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399"))

	fmt.Fprintf(w, "  %s\n", titleStyle.Render(r.Label))
	fmt.Fprintf(w, "  %s\n\n", dimStyle.Render(strings.Repeat("─", 40)))

	// Summary
	hours := r.Summary.TotalWorkTime.Hours()
	hitRate := ""
	if r.Summary.HighlightHitRate > 0 {
		hitRate = fmt.Sprintf(", highlight hit rate %s", valueStyle.Render(fmt.Sprintf("%.0f%%", r.Summary.HighlightHitRate*100)))
	}
	fmt.Fprintf(w, "  %s  %s\n",
		dimStyle.Render("Total:"),
		valueStyle.Render(fmt.Sprintf("%d sessions, %s deep work", r.Summary.TotalSessions, formatHours(hours))),
	)
	if r.Summary.FocusScoreCount > 0 {
		fmt.Fprintf(w, "  %s  %s  %s\n",
			dimStyle.Render("Avg focus:"),
			valueStyle.Render(fmt.Sprintf("%.1f/5", r.Summary.AvgFocusScore)),
			dimStyle.Render(fmt.Sprintf("(%d scored sessions)", r.Summary.FocusScoreCount)),
		)
	}
	if r.Summary.DistractionCount > 0 {
		fmt.Fprintf(w, "  %s  %s\n",
			dimStyle.Render("Distractions:"),
			valueStyle.Render(fmt.Sprintf("%d", r.Summary.DistractionCount)),
		)
	}
	if r.Summary.HighlightDays > 0 {
		fmt.Fprintf(w, "  %s  %s%s\n",
			dimStyle.Render("Highlights set:"),
			valueStyle.Render(fmt.Sprintf("%d days", r.Summary.HighlightDays)),
			dimStyle.Render(hitRate),
		)
	}
	if r.Summary.TotalSessions == 0 {
		fmt.Fprintf(w, "\n  %s\n", dimStyle.Render("No completed sessions in this period."))
		return
	}
	fmt.Fprintln(w)

	// Daily breakdown
	fmt.Fprintf(w, "  %s\n", dimStyle.Render("Daily"))
	for _, d := range r.Daily {
		marker := " "
		if d.HasHighlight {
			marker = accentStyle.Render("◆")
		}
		fmt.Fprintf(w, "  %s %-30s %s %s\n",
			marker,
			dimStyle.Render(d.Day),
			valueStyle.Render(fmt.Sprintf("%-3d", d.WorkSessions)+" sessions"),
			valueStyle.Render(formatMinutes(d.WorkTime)),
		)
	}
	fmt.Fprintln(w)

	// Heatmap (24h)
	if len(r.Heatmap) > 0 {
		fmt.Fprintf(w, "  %s\n", dimStyle.Render("Deep work heatmap (hour of day)"))
		var max time.Duration
		for _, h := range r.Heatmap {
			if h.Minutes > max {
				max = h.Minutes
			}
		}
		for _, h := range r.Heatmap {
			width := 0
			if max > 0 {
				width = int(math.Round(float64(h.Minutes) / float64(max) * 24))
			}
			if width < 1 && h.Minutes > 0 {
				width = 1
			}
			fmt.Fprintf(w, "  %-7s%s %s\n",
				dimStyle.Render(h.Hour),
				buildBar(width),
				dimStyle.Render(formatMinutes(h.Minutes)),
			)
		}
		fmt.Fprintln(w)
	}

	// Tags
	if len(r.Tags) > 0 {
		fmt.Fprintf(w, "  %s\n", dimStyle.Render("Top tags"))
		for _, t := range r.Tags {
			fmt.Fprintf(w, "  %-16s %s %s\n",
				dimStyle.Render(t.Tag),
				valueStyle.Render(formatMinutes(t.TotalTime)),
				dimStyle.Render(fmt.Sprintf("(%d sessions)", t.SessionCount)),
			)
		}
		fmt.Fprintln(w)
	}

	// Methodologies
	if len(r.Methodologies) > 0 {
		fmt.Fprintf(w, "  %s\n", dimStyle.Render("By methodology"))
		for _, m := range r.Methodologies {
			focus := ""
			if m.FocusScoreCount > 0 {
				focus = fmt.Sprintf("avg focus %s", valueStyle.Render(fmt.Sprintf("%.1f/5", m.AvgFocusScore)))
			}
			fmt.Fprintf(w, "  %-12s %s %s\n",
				dimStyle.Render(m.Label),
				valueStyle.Render(fmt.Sprintf("%-3d sessions", m.SessionCount)),
				dimStyle.Render(fmt.Sprintf("(%s %s)", formatMinutes(m.TotalTime), focus)),
			)
		}
		fmt.Fprintln(w)
	}

	// Streaks
	fmt.Fprintf(w, "  %s  %s\n",
		dimStyle.Render("Streak:"),
		valueStyle.Render(fmt.Sprintf("%d days", r.Streaks.CurrentDays)),
	)
	fmt.Fprintf(w, "  %s  %s\n\n",
		dimStyle.Render("Longest:"),
		valueStyle.Render(fmt.Sprintf("%d days", r.Streaks.LongestDays)),
	)
}

func renderCSVReport(w *bytes.Buffer, r *domain.Report) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	if err := cw.Write([]string{"report", r.Period}); err != nil {
		return err
	}
	if err := cw.Write([]string{"label", r.Label}); err != nil {
		return err
	}
	if err := cw.Write([]string{"total_sessions", fmt.Sprintf("%d", r.Summary.TotalSessions)}); err != nil {
		return err
	}
	if err := cw.Write([]string{"total_work_min", fmt.Sprintf("%.0f", r.Summary.TotalWorkTime.Minutes())}); err != nil {
		return err
	}
	if r.Summary.FocusScoreCount > 0 {
		if err := cw.Write([]string{"avg_focus_score", fmt.Sprintf("%.1f", r.Summary.AvgFocusScore)}); err != nil {
			return err
		}
	}
	if err := cw.Write([]string{"distractions", fmt.Sprintf("%d", r.Summary.DistractionCount)}); err != nil {
		return err
	}
	if err := cw.Write([]string{"highlight_days", fmt.Sprintf("%d", r.Summary.HighlightDays)}); err != nil {
		return err
	}
	if err := cw.Write([]string{"streak_current", fmt.Sprintf("%d", r.Streaks.CurrentDays)}); err != nil {
		return err
	}
	if err := cw.Write([]string{"streak_longest", fmt.Sprintf("%d", r.Streaks.LongestDays)}); err != nil {
		return err
	}
	if err := cw.Write([]string{}); err != nil {
		return err
	}

	if err := cw.Write([]string{"day", "sessions", "breaks", "work_min", "avg_focus", "distractions", "highlight"}); err != nil {
		return err
	}
	for _, d := range r.Daily {
		highlight := ""
		if d.HasHighlight {
			highlight = "yes"
		}
		focus := ""
		if d.FocusCount > 0 {
			focus = fmt.Sprintf("%.1f", d.AvgFocusScore)
		}
		if err := cw.Write([]string{
			d.Date, fmt.Sprintf("%d", d.WorkSessions), fmt.Sprintf("%d", d.Breaks),
			fmt.Sprintf("%.0f", d.WorkTime.Minutes()), focus, fmt.Sprintf("%d", d.Distractions), highlight,
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

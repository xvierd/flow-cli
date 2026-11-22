package cmd

import (
	"bytes"
	"context"
	"encoding/json"
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

var (
	insightsMonth bool
	insightsWeek  bool
)

var insightsCmd = &cobra.Command{
	Use:   "insights",
	Short: "Show actionable findings from your focus history",
	Long: `Analyze your session history and show 3 to 5 findings with evidence:
best time slot and days, when you void sessions, which tags take more
sessions per finished task, your optimal session length, streak and trend
versus the previous period, and dominant Deep Work distractions.

The window is rolling: the last 7 days (--week, default) or 30 days (--month).
Detectors without enough data say how many sessions they still need.`,
	RunE: runInsights,
}

func init() {
	insightsCmd.Flags().BoolVar(&insightsMonth, "month", false, "Analyze the last 30 days instead of 7")
	insightsCmd.Flags().BoolVar(&insightsWeek, "week", false, "Analyze the last 7 days (default)")
	rootCmd.AddCommand(insightsCmd)
}

func runInsights(cmd *cobra.Command, args []string) error {
	if insightsWeek && insightsMonth {
		return fmt.Errorf("%s", i18n.T("--week and --month are mutually exclusive"))
	}
	period := services.ReportPeriodWeek
	if insightsMonth {
		period = services.ReportPeriodMonth
	}

	report, err := services.NewInsightsService(app.storage).GetInsights(context.Background(), period)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to compute insights"), err)
	}

	var buf bytes.Buffer
	if jsonOutput {
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to encode JSON"), err)
		}
	} else {
		renderInsights(&buf, report)
	}
	_, err = os.Stdout.Write(buf.Bytes())
	return err
}

// renderInsights writes the human-readable insights view.
func renderInsights(w *bytes.Buffer, r *domain.InsightsReport) {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C6FE0"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	valueStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA"))

	title := i18n.T("Insights · last 7 days")
	if r.Period == string(services.ReportPeriodMonth) {
		title = i18n.T("Insights · last 30 days")
	}
	fmt.Fprintf(w, "  %s\n", titleStyle.Render(title))
	fmt.Fprintf(w, "  %s\n", dimStyle.Render(i18n.T("%s → %s · %d sessions analyzed",
		r.Start.Format("2006-01-02"), r.End.Add(-time.Second).Format("2006-01-02"), r.SessionsAnalyzed)))
	fmt.Fprintf(w, "  %s\n\n", dimStyle.Render(strings.Repeat("─", 40)))

	if len(r.Insights) == 0 {
		fmt.Fprintf(w, "  %s\n\n", i18n.T("Not enough history for insights yet. Keep logging sessions."))
	}
	for i, in := range r.Insights {
		fmt.Fprintf(w, "  %s %s\n", valueStyle.Render(fmt.Sprintf("%d.", i+1)), in.Message)
		if in.Evidence != "" {
			fmt.Fprintf(w, "     %s\n", dimStyle.Render(in.Evidence))
		}
		fmt.Fprintln(w)
	}

	if len(r.Gaps) > 0 {
		fmt.Fprintf(w, "  %s\n", dimStyle.Render(i18n.T("Waiting for more data:")))
		for _, g := range r.Gaps {
			fmt.Fprintf(w, "  %s\n", dimStyle.Render("· "+g.Message))
		}
		fmt.Fprintln(w)
	}
}

// topInsightLine renders the main insight as one subtle line, or "".
func topInsightLine(message string) string {
	if message == "" {
		return ""
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render("💡 " + message)
}

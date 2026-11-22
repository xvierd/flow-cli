package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
)

func TestInsightsCmd_Structure(t *testing.T) {
	if insightsCmd.Use != "insights" || insightsCmd.Short == "" {
		t.Errorf("insightsCmd = %q / %q", insightsCmd.Use, insightsCmd.Short)
	}
	for _, name := range []string{"week", "month"} {
		if insightsCmd.Flags().Lookup(name) == nil {
			t.Errorf("insightsCmd should have --%s", name)
		}
	}
	found := false
	for _, c := range rootCmd.Commands() {
		found = found || c == insightsCmd
	}
	if !found {
		t.Error("insights not registered on the root command")
	}
}

func TestInsightsCmd_WeekMonthMutuallyExclusive(t *testing.T) {
	insightsWeek, insightsMonth = true, true
	defer func() { insightsWeek, insightsMonth = false, false }()
	if err := runInsights(insightsCmd, nil); err == nil {
		t.Error("expected error when both --week and --month are set")
	}
}

func TestRenderInsights(t *testing.T) {
	start := time.Date(2026, 11, 12, 0, 0, 0, 0, time.Local)
	r := &domain.InsightsReport{
		Period: "month", Start: start, End: start.AddDate(0, 0, 30), SessionsAnalyzed: 21,
		Insights: []domain.Insight{
			{Message: "After 16:00 you void 3.0x more sessions.", Evidence: "6 of 9 voided"},
			{Message: "Protect 09–12h.", Evidence: "60% of minutes"},
		},
		Gaps: []domain.InsightGap{{Message: "Tag effort: finish 2 more tagged tasks"}},
	}
	var buf bytes.Buffer
	renderInsights(&buf, r)
	out := buf.String()
	for _, want := range []string{"last 30 days", "21 sessions analyzed", "1.", "After 16:00", "6 of 9 voided", "2.", "Waiting for more data", "finish 2 more"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "After 16:00") > strings.Index(out, "Protect") {
		t.Error("insights should render in rank order")
	}

	buf.Reset()
	renderInsights(&buf, &domain.InsightsReport{Period: "week", Start: start, End: start.AddDate(0, 0, 7)})
	if !strings.Contains(buf.String(), "Not enough history") || !strings.Contains(buf.String(), "last 7 days") {
		t.Errorf("empty output:\n%s", buf.String())
	}
}

func TestTopInsightLine(t *testing.T) {
	if topInsightLine("") != "" {
		t.Error("no insight should render nothing")
	}
	if got := topInsightLine("Protect 09–12h."); !strings.Contains(got, "💡") || !strings.Contains(got, "Protect 09–12h.") {
		t.Errorf("line = %q", got)
	}
}

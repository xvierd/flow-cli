package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
)

func TestReportCmd_Structure(t *testing.T) {
	if reportCmd.Use != "report" {
		t.Errorf("reportCmd.Use = %q, want %q", reportCmd.Use, "report")
	}
	if reportCmd.Short == "" {
		t.Error("reportCmd.Short should be set")
	}
}

func TestReportCmd_Flags(t *testing.T) {
	for _, name := range []string{"month", "week", "format", "out"} {
		flag := reportCmd.Flags().Lookup(name)
		if flag == nil {
			t.Errorf("reportCmd should have --%s flag", name)
		}
	}
}

func TestReportCmd_ValidateFormat(t *testing.T) {
	cases := []struct {
		format  string
		wantErr bool
	}{
		{"terminal", false},
		{"md", false},
		{"csv", false},
		{"bogus", true},
	}

	for _, tc := range cases {
		_, err := normalizeReportFormat(tc.format)
		if tc.wantErr && err == nil {
			t.Errorf("format %q: expected error", tc.format)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("format %q: unexpected error %v", tc.format, err)
		}
	}
}

// normalizeReportFormat returns the canonical name for a format, or an error if unsupported.
func normalizeReportFormat(format string) (string, error) {
	switch format {
	case "terminal", "md", "markdown", "csv":
		return format, nil
	default:
		return "", errUnsupportedReportFormat(format)
	}
}

func errUnsupportedReportFormat(format string) error {
	return &reportFormatError{format: format}
}

type reportFormatError struct {
	format string
}

func (e *reportFormatError) Error() string {
	return "unknown format " + e.format + ": use terminal, md, or csv"
}

func TestRenderTerminalReport(t *testing.T) {
	r := &domain.Report{
		Period: "week",
		Label:  "Week of Jul 6",
		Summary: domain.ReportSummary{
			TotalSessions: 2,
			TotalWorkTime: 75 * time.Minute,
			HighlightDays: 1,
		},
		Daily: []domain.DayRow{
			{Date: "2026-07-06", Day: "Mon Jul 6", WorkSessions: 1, WorkTime: 25 * time.Minute},
		},
		Methodologies: []domain.MethodologyStat{
			{Methodology: domain.MethodologyPomodoro, Label: "Pomodoro", SessionCount: 1, TotalTime: 25 * time.Minute},
		},
		Tags: []domain.TagStat{
			{Tag: "deep", SessionCount: 2, TotalTime: 75 * time.Minute},
		},
		Heatmap: []domain.HourBucket{
			{Hour: "09:00", Minutes: 25 * time.Minute},
		},
		Streaks: domain.StreakInfo{CurrentDays: 1, LongestDays: 3},
	}

	var buf bytes.Buffer
	renderTerminalReport(&buf, r)
	out := buf.String()

	for _, want := range []string{"Week of Jul 6", "2 sessions", "deep work", "deep", "Pomodoro", "Streak"} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Errorf("terminal report missing %q\n%s", want, out)
		}
	}
}

func TestRenderMarkdownReport(t *testing.T) {
	r := &domain.Report{
		Label: "August 2026",
		Daily: []domain.DayRow{
			{Date: "2026-08-03", Day: "Mon Aug 3", WorkSessions: 1, WorkTime: 25 * time.Minute, HasHighlight: true},
		},
		Tags: []domain.TagStat{{Tag: "deep", SessionCount: 1, TotalTime: 25 * time.Minute}},
	}

	var buf bytes.Buffer
	renderMarkdownReport(&buf, r)

	out := buf.String()
	for _, want := range []string{"# August 2026", "## Daily", "| Day |", "deep", "yes"} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Errorf("markdown report missing %q\n%s", want, out)
		}
	}
}

func TestRenderCSVReport(t *testing.T) {
	r := &domain.Report{
		Period: "week",
		Label:  "Week of Jul 6",
		Summary: domain.ReportSummary{
			TotalSessions: 1,
			TotalWorkTime: 25 * time.Minute,
		},
		Daily: []domain.DayRow{
			{Date: "2026-07-06", WorkSessions: 1, WorkTime: 25 * time.Minute},
		},
	}

	var buf bytes.Buffer
	if err := renderCSVReport(&buf, r); err != nil {
		t.Fatalf("renderCSVReport() error = %v", err)
	}

	out := buf.String()
	for _, want := range []string{"total_sessions,1", "total_work_min,25", "day,sessions,breaks,work_min"} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Errorf("csv report missing %q\n%s", want, out)
		}
	}
}

func TestReportJSON(t *testing.T) {
	r := &domain.Report{Period: "week", Label: "Week"}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal error = %v", err)
	}
	if !bytes.Contains(data, []byte(`"period"`)) || !bytes.Contains(data, []byte(`"summary"`)) {
		t.Errorf("json report missing expected keys: %s", data)
	}
}

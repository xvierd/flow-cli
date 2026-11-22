package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/services"
)

func TestShareCommandsRegistered(t *testing.T) {
	for _, name := range []string{"wrapped", "badge"} {
		found := false
		for _, c := range rootCmd.Commands() {
			if c.Name() == name {
				found = true
			}
		}
		if !found {
			t.Errorf("command %q not registered", name)
		}
	}
	for _, f := range []string{"week", "month", "year", "png", "theme"} {
		if wrappedCmd.Flags().Lookup(f) == nil {
			t.Errorf("wrapped is missing --%s", f)
		}
	}
	for _, f := range []string{"week", "month", "year", "out", "label"} {
		if badgeCmd.Flags().Lookup(f) == nil {
			t.Errorf("badge is missing --%s", f)
		}
	}
	if statsCmd.Flags().Lookup("year") == nil {
		t.Error("stats is missing --year")
	}
}

func TestPeriodFromFlags(t *testing.T) {
	if periodFromFlags(false, false) != services.ReportPeriodWeek ||
		periodFromFlags(true, false) != services.ReportPeriodMonth ||
		periodFromFlags(false, true) != services.ReportPeriodYear {
		t.Error("periodFromFlags mapping is wrong")
	}
}

func TestBuildWrappedCard(t *testing.T) {
	w := &domain.Wrapped{
		Period: "week", Start: time.Date(2025, 6, 9, 0, 0, 0, 0, time.UTC),
		TotalFocus: 12*time.Hour + 30*time.Minute, Sessions: 18,
		CurrentStreak: 5, LongestStreak: 9,
		BestDay: time.Date(2025, 6, 11, 0, 0, 0, 0, time.UTC), BestDayFocus: 3 * time.Hour,
		BestHour: 10, BestHourFocus: 4 * time.Hour,
		TopTag: "docs", TopTask: "Write docs",
		FunFact: domain.FunFact{Kind: domain.FunFactNightOwl, Value: 34},
	}
	c := buildWrappedCard(w)
	if c.HeroValue != "12h 30m" || c.Subtitle != "Week of Jun 9" {
		t.Errorf("hero/subtitle = %q / %q", c.HeroValue, c.Subtitle)
	}
	want := map[string]string{"Sessions": "18", "Streak · best 9": "5 days", "Best day · 3h": "Wed Jun 11",
		"Best hour": "10:00", "Top tag": "#docs", "Top task": "Write docs"}
	for _, s := range c.Stats {
		if want[s.Label] != s.Value {
			t.Errorf("stat %q = %q, want %q", s.Label, s.Value, want[s.Label])
		}
	}
	if !strings.Contains(c.FunFact, "34%") {
		t.Errorf("fun fact = %q", c.FunFact)
	}

	empty := buildWrappedCard(&domain.Wrapped{Period: "year", Start: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), BestHour: -1})
	if empty.Subtitle != "2024" || empty.Stats[3].Value != "—" || !strings.Contains(empty.FunFact, "flow start") {
		t.Errorf("empty card = %+v", empty)
	}
	out := renderWrappedCard(c)
	if !strings.Contains(out, "flow wrapped") || !strings.Contains(out, "#docs") {
		t.Errorf("terminal card missing content:\n%s", out)
	}
}

func TestBigText(t *testing.T) {
	rows := strings.Split(bigText("12h 30m"), "\n")
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	for _, r := range rows[1:] {
		if lipgloss.Width(r) != lipgloss.Width(rows[0]) {
			t.Errorf("ragged big text: %q vs %q", r, rows[0])
		}
	}
	if bigText("n/a") != "n/a" {
		t.Error("unsupported runes must fall back to plain text")
	}
}

func TestRenderYearHeatmap(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(prev)

	ref := time.Date(2024, 3, 6, 12, 0, 0, 0, time.UTC)
	hm := services.BuildYearHeatmap([]*domain.PomodoroSession{{
		ID: "a", Type: domain.SessionTypeWork, Status: domain.SessionStatusCompleted,
		Duration: 2 * time.Hour, StartedAt: time.Date(2024, 2, 29, 9, 0, 0, 0, time.UTC),
	}}, ref)
	var buf bytes.Buffer
	renderYearHeatmap(&buf, hm)
	out := buf.String()

	gridRows := 0
	for _, line := range strings.Split(out, "\n") {
		if n := strings.Count(line, "·") + strings.Count(line, "█"); n >= services.HeatmapWeeks-1 {
			gridRows++
			if w := lipgloss.Width(strings.TrimSpace(line)); w > 5+services.HeatmapWeeks {
				t.Errorf("grid row too wide (%d): %q", w, line)
			}
		}
	}
	if gridRows != 7 {
		t.Errorf("grid rows = %d, want 7\n%s", gridRows, out)
	}
	if strings.Count(out, "█") != 2 { // Feb 29 cell + legend
		t.Errorf("want exactly one max-level cell plus legend:\n%s", out)
	}
	if !strings.Contains(out, "Thu Feb 29, 2024 (2h)") {
		t.Errorf("busiest day missing:\n%s", out)
	}
}

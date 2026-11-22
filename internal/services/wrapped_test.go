package services

import (
	"context"
	"testing"
	"time"
	_ "time/tzdata" // deterministic zones regardless of the host's zoneinfo

	"github.com/xvierd/flow-cli/internal/domain"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return loc
}

func work(id string, start time.Time, d time.Duration, tags ...string) *domain.PomodoroSession {
	return &domain.PomodoroSession{
		ID: id, Type: domain.SessionTypeWork, Status: domain.SessionStatusCompleted,
		Duration: d, StartedAt: start, Tags: tags,
	}
}

func strPtr(s string) *string { return &s }

func TestBuildWrapped_Aggregations(t *testing.T) {
	loc := mustLoc(t, "Europe/Madrid")
	ref := time.Date(2025, 6, 13, 18, 0, 0, 0, loc) // Friday
	start, end, _ := PeriodRange(ReportPeriodWeek, ref)
	mon := start

	s1 := work("s1", mon.Add(10*time.Hour), 50*time.Minute, "docs")
	s1.TaskID = strPtr("t1")
	s2 := work("s2", mon.Add(11*time.Hour), 25*time.Minute, "docs", "api")
	s2.TaskID = strPtr("t1")
	s3 := work("s3", mon.AddDate(0, 0, 2).Add(10*time.Hour+15*time.Minute), 90*time.Minute, "api")
	s3.TaskID = strPtr("t2")
	brk := &domain.PomodoroSession{ID: "b", Type: domain.SessionTypeShortBreak, Status: domain.SessionStatusCompleted, Duration: 5 * time.Minute, StartedAt: mon.Add(12 * time.Hour)}
	cancelled := work("c", mon.Add(13*time.Hour), 4*time.Hour)
	cancelled.Status = domain.SessionStatusCancelled
	outside := work("o", mon.AddDate(0, 0, -3).Add(10*time.Hour), 3*time.Hour, "old")

	w := BuildWrapped([]*domain.PomodoroSession{s1, s2, s3, brk, cancelled, outside},
		map[string]string{"t1": "Write docs", "t2": "Ship API"}, ReportPeriodWeek, start, end, ref)

	if w.Sessions != 3 {
		t.Errorf("Sessions = %d, want 3", w.Sessions)
	}
	if w.TotalFocus != 165*time.Minute {
		t.Errorf("TotalFocus = %v, want 2h45m", w.TotalFocus)
	}
	if w.ActiveDays != 2 {
		t.Errorf("ActiveDays = %d, want 2", w.ActiveDays)
	}
	if w.PeriodDays != 5 { // Mon..Fri (ref day), not the future weekend
		t.Errorf("PeriodDays = %d, want 5", w.PeriodDays)
	}
	if want := mon.AddDate(0, 0, 2); !w.BestDay.Equal(want) || w.BestDayFocus != 90*time.Minute {
		t.Errorf("BestDay = %v (%v), want %v (1h30m)", w.BestDay, w.BestDayFocus, want)
	}
	if w.BestHour != 10 || w.BestHourFocus != 140*time.Minute {
		t.Errorf("BestHour = %d (%v), want 10 (2h20m)", w.BestHour, w.BestHourFocus)
	}
	if w.TopTag != "api" || w.TopTagFocus != 115*time.Minute {
		t.Errorf("TopTag = %q (%v), want api (1h55m)", w.TopTag, w.TopTagFocus)
	}
	if w.TopTask != "Ship API" || w.TopTaskFocus != 90*time.Minute {
		t.Errorf("TopTask = %q (%v), want Ship API (1h30m)", w.TopTask, w.TopTaskFocus)
	}
	if w.LongestSession != 90*time.Minute {
		t.Errorf("LongestSession = %v", w.LongestSession)
	}
	if w.LongestStreak != 1 {
		t.Errorf("LongestStreak = %d, want 1", w.LongestStreak)
	}
	if w.FunFact.Kind != domain.FunFactMarathon || w.FunFact.Value != 90 {
		t.Errorf("FunFact = %+v, want marathon 90", w.FunFact)
	}
}

func TestBuildWrapped_Empty(t *testing.T) {
	ref := time.Date(2025, 6, 13, 18, 0, 0, 0, time.UTC)
	start, end, _ := PeriodRange(ReportPeriodMonth, ref)
	w := BuildWrapped(nil, nil, ReportPeriodMonth, start, end, ref)
	if w.Sessions != 0 || w.BestHour != -1 || !w.BestDay.IsZero() || w.FunFact.Kind != domain.FunFactNone {
		t.Errorf("unexpected empty wrapped: %+v", w)
	}
	if w.PeriodDays != 13 {
		t.Errorf("PeriodDays = %d, want 13", w.PeriodDays)
	}
}

// A session stored in UTC must be attributed to the local calendar day and
// hour of the reference zone, not the UTC ones.
func TestBuildWrapped_LocalDayAndHour(t *testing.T) {
	loc := mustLoc(t, "America/Mexico_City") // UTC-6, no DST since 2022
	ref := time.Date(2025, 3, 15, 12, 0, 0, 0, loc)
	start, end, _ := PeriodRange(ReportPeriodMonth, ref)

	// 2025-03-10 02:30 UTC == 2025-03-09 20:30 local.
	s := work("s", time.Date(2025, 3, 10, 2, 30, 0, 0, time.UTC), 2*time.Hour)
	w := BuildWrapped([]*domain.PomodoroSession{s}, nil, ReportPeriodMonth, start, end, ref)

	if want := time.Date(2025, 3, 9, 0, 0, 0, 0, loc); !w.BestDay.Equal(want) {
		t.Errorf("BestDay = %v, want %v", w.BestDay, want)
	}
	if w.BestHour != 20 {
		t.Errorf("BestHour = %d, want 20", w.BestHour)
	}
	if w.FunFact.Kind != domain.FunFactNightOwl || w.FunFact.Value != 100 {
		t.Errorf("FunFact = %+v, want night_owl 100", w.FunFact)
	}

	// The same instant at the very start of the month in UTC belongs to the
	// previous month locally and must be excluded.
	edge := work("e", time.Date(2025, 3, 1, 3, 0, 0, 0, time.UTC), time.Hour) // Feb 28 21:00 local
	w = BuildWrapped([]*domain.PomodoroSession{edge}, nil, ReportPeriodMonth, start, end, ref)
	if w.Sessions != 0 {
		t.Errorf("Sessions = %d, want 0 (session is in February locally)", w.Sessions)
	}
}

func TestBuildWrapped_Streaks(t *testing.T) {
	loc := mustLoc(t, "Europe/Madrid")
	ref := time.Date(2025, 6, 4, 9, 0, 0, 0, loc) // Wednesday, nothing today yet
	start, end, _ := PeriodRange(ReportPeriodWeek, ref)

	var sessions []*domain.PomodoroSession
	// Every day from May 27 through June 3 (8 days), spanning the week start.
	for i := 0; i < 8; i++ {
		d := time.Date(2025, 5, 27+i, 10, 0, 0, 0, loc)
		sessions = append(sessions, work(d.Format("0102"), d, 30*time.Minute))
	}
	w := BuildWrapped(sessions, nil, ReportPeriodWeek, start, end, ref)
	if w.CurrentStreak != 8 {
		t.Errorf("CurrentStreak = %d, want 8 (a quiet today must not break it)", w.CurrentStreak)
	}
	if w.LongestStreak != 2 { // Mon + Tue inside this week
		t.Errorf("LongestStreak = %d, want 2", w.LongestStreak)
	}
}

func TestPeriodRange_Year(t *testing.T) {
	loc := mustLoc(t, "Europe/Madrid")
	start, end, err := PeriodRange(ReportPeriodYear, time.Date(2024, 7, 1, 12, 0, 0, 0, loc))
	if err != nil {
		t.Fatal(err)
	}
	if !start.Equal(time.Date(2024, 1, 1, 0, 0, 0, 0, loc)) || !end.Equal(time.Date(2025, 1, 1, 0, 0, 0, 0, loc)) {
		t.Errorf("year range = %v..%v", start, end)
	}
	w := BuildWrapped(nil, nil, ReportPeriodYear, start, end, time.Date(2025, 2, 1, 0, 0, 0, 0, loc))
	if w.PeriodDays != 366 {
		t.Errorf("PeriodDays for leap year 2024 = %d, want 366", w.PeriodDays)
	}
}

func TestHeatmapLevel(t *testing.T) {
	cases := []struct{ minutes, max, want int }{
		{0, 100, 0}, {1, 100, 1}, {25, 100, 1}, {26, 100, 2}, {50, 100, 2},
		{51, 100, 3}, {75, 100, 3}, {76, 100, 4}, {100, 100, 4}, {5, 0, 0},
	}
	for _, c := range cases {
		if got := HeatmapLevel(c.minutes, c.max); got != c.want {
			t.Errorf("HeatmapLevel(%d, %d) = %d, want %d", c.minutes, c.max, got, c.want)
		}
	}
}

func assertGridShape(t *testing.T, hm *domain.YearHeatmap, ref time.Time) {
	t.Helper()
	if len(hm.Weeks) != HeatmapWeeks {
		t.Fatalf("weeks = %d, want %d", len(hm.Weeks), HeatmapWeeks)
	}
	loc := ref.Location()
	prev := time.Time{}
	foundRef := false
	for w, week := range hm.Weeks {
		if week[0].Date.Weekday() != time.Monday {
			t.Fatalf("week %d starts on %v, want Monday", w, week[0].Date.Weekday())
		}
		for d, c := range week {
			if h, m, s := c.Date.Clock(); h != 0 || m != 0 || s != 0 || c.Date.Location() != loc {
				t.Fatalf("cell %d/%d = %v, want local midnight", w, d, c.Date)
			}
			if !prev.IsZero() {
				py, pm, pd := prev.AddDate(0, 0, 1).Date()
				cy, cm, cd := c.Date.Date()
				if py != cy || pm != cm || pd != cd {
					t.Fatalf("cell %d/%d = %v does not follow %v", w, d, c.Date, prev)
				}
			}
			prev = c.Date
			isRef := c.Date.Equal(dayOf(ref, loc))
			foundRef = foundRef || isRef
			if c.Future != c.Date.After(dayOf(ref, loc)) {
				t.Fatalf("cell %v future = %v", c.Date, c.Future)
			}
			if isRef && w != HeatmapWeeks-1 {
				t.Fatalf("reference day in week %d, want last column", w)
			}
		}
	}
	if !foundRef {
		t.Fatal("reference day not in grid")
	}
}

func TestBuildYearHeatmap_LeapYear(t *testing.T) {
	loc := mustLoc(t, "America/New_York") // DST transitions inside the window
	ref := time.Date(2024, 3, 6, 15, 0, 0, 0, loc)

	sessions := []*domain.PomodoroSession{
		work("leap", time.Date(2024, 2, 29, 9, 0, 0, 0, loc), 100*time.Minute),
		work("a", time.Date(2024, 2, 29, 14, 0, 0, 0, loc), 20*time.Minute),
		// 2023-11-05 is the fall-back DST day; 23:30 local is 04:30 UTC next day.
		work("dst", time.Date(2023, 11, 6, 4, 30, 0, 0, time.UTC), 30*time.Minute),
		work("future", time.Date(2024, 3, 8, 9, 0, 0, 0, loc), time.Hour),
		work("old", time.Date(2022, 1, 1, 9, 0, 0, 0, loc), time.Hour),
	}
	hm := BuildYearHeatmap(sessions, ref)
	assertGridShape(t, hm, ref)

	cells := map[string]domain.HeatmapCell{}
	for _, week := range hm.Weeks {
		for _, c := range week {
			cells[c.Date.Format("2006-01-02")] = c
		}
	}
	if c := cells["2024-02-29"]; c.Minutes != 120 || c.Level != 4 {
		t.Errorf("Feb 29 = %+v, want 120 min level 4", c)
	}
	if c := cells["2023-11-05"]; c.Minutes != 30 || c.Level != 1 {
		t.Errorf("DST day = %+v, want 30 min level 1", c)
	}
	if c := cells["2024-03-08"]; !c.Future || c.Minutes != 0 {
		t.Errorf("future day = %+v, want blank", c)
	}
	if hm.TotalMinutes != 150 || hm.ActiveDays != 2 || hm.MaxMinutes != 120 {
		t.Errorf("totals = %d min / %d days / max %d", hm.TotalMinutes, hm.ActiveDays, hm.MaxMinutes)
	}
	if want := time.Date(2023, 3, 6, 0, 0, 0, 0, loc); !hm.Start.Equal(want) {
		t.Errorf("Start = %v, want %v", hm.Start, want)
	}
}

func TestBuildYearHeatmap_NonLeapYear(t *testing.T) {
	loc := mustLoc(t, "Europe/Madrid")
	ref := time.Date(2025, 6, 1, 23, 59, 0, 0, loc) // Sunday: last column is full
	hm := BuildYearHeatmap(nil, ref)
	assertGridShape(t, hm, ref)
	last := hm.Weeks[HeatmapWeeks-1][6]
	if !last.Date.Equal(time.Date(2025, 6, 1, 0, 0, 0, 0, loc)) || last.Future {
		t.Errorf("last cell = %+v, want 2025-06-01 (not future)", last)
	}
	for _, week := range hm.Weeks {
		for _, c := range week {
			if c.Date.Month() == time.February && c.Date.Day() == 29 {
				t.Fatalf("unexpected Feb 29 in non-leap window: %v", c.Date)
			}
		}
	}
}

func TestWrappedService_UsesStorage(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now()

	task, err := domain.NewTask("Polish wrapped")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Tasks().Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	s := work("w1", now.Add(-time.Minute), 45*time.Minute, "share")
	s.TaskID = &task.ID
	completed := now
	s.CompletedAt = &completed
	if err := store.Sessions().Save(ctx, s); err != nil {
		t.Fatal(err)
	}

	svc := NewWrappedService(store)
	w, err := svc.GetWrapped(ctx, ReportPeriodWeek, now)
	if err != nil {
		t.Fatal(err)
	}
	if w.Sessions != 1 || w.TopTask != "Polish wrapped" || w.TopTag != "share" {
		t.Errorf("wrapped = %+v", w)
	}
	total, err := svc.FocusTotal(ctx, ReportPeriodWeek, now)
	if err != nil || total != 45*time.Minute {
		t.Errorf("FocusTotal = %v, %v", total, err)
	}
	hm, err := svc.GetYearHeatmap(ctx, now)
	if err != nil || hm.TotalMinutes != 45 {
		t.Errorf("heatmap total = %v, %v", hm, err)
	}
}

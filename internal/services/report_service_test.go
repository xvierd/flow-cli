package services

import (
	"context"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
)

func TestReportService_GetReport_Week(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Determine this week's Monday start and seed sessions relative to it.
	weekStart, _, _ := PeriodRange(ReportPeriodWeek, now)

	sessions := []*domain.PomodoroSession{
		{
			ID:          "s1",
			Type:        domain.SessionTypeWork,
			Status:      domain.SessionStatusCompleted,
			Duration:    25 * time.Minute,
			StartedAt:   weekStart.Add(24 * time.Hour),
			Methodology: domain.MethodologyPomodoro,
			Tags:        []string{"deep", "focus"},
		},
		{
			ID:          "s2",
			Type:        domain.SessionTypeWork,
			Status:      domain.SessionStatusCompleted,
			Duration:    50 * time.Minute,
			StartedAt:   weekStart.Add(48 * time.Hour),
			Methodology: domain.MethodologyDeepWork,
			Tags:        []string{"deep"},
		},
		{
			ID:          "s3",
			Type:        domain.SessionTypeWork,
			Status:      domain.SessionStatusCompleted,
			Duration:    90 * time.Minute,
			StartedAt:   now.AddDate(0, 0, -60), // outside week
			Methodology: domain.MethodologyPomodoro,
		},
		{
			ID:          "s4",
			Type:        domain.SessionTypeShortBreak,
			Status:      domain.SessionStatusCompleted,
			Duration:    5 * time.Minute,
			StartedAt:   weekStart.Add(24*time.Hour + 30*time.Minute),
			Methodology: domain.MethodologyPomodoro,
		},
	}
	for _, s := range sessions {
		ts := s.StartedAt
		s.CompletedAt = &ts
		if err := store.Sessions().Save(ctx, s); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}

	score := 4
	s := sessions[0]
	s.FocusScore = &score
	if err := store.Sessions().Update(ctx, s); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	svc := NewReportService(store)
	report, err := svc.GetReport(ctx, ReportPeriodWeek)
	if err != nil {
		t.Fatalf("GetReport() error = %v", err)
	}

	if report.Period != "week" {
		t.Errorf("Period = %q, want %q", report.Period, "week")
	}
	if report.Summary.TotalSessions != 2 {
		t.Errorf("TotalSessions = %d, want 2", report.Summary.TotalSessions)
	}
	if report.Summary.TotalWorkTime != 75*time.Minute {
		t.Errorf("TotalWorkTime = %s, want 1h15m", report.Summary.TotalWorkTime)
	}
	if report.Summary.FocusScoreCount != 1 {
		t.Errorf("FocusScoreCount = %d, want 1", report.Summary.FocusScoreCount)
	}
	if report.Summary.DistractionCount != 0 {
		t.Errorf("DistractionCount = %d, want 0", report.Summary.DistractionCount)
	}

	if len(report.Daily) != 2 {
		t.Errorf("Daily rows = %d, want 2", len(report.Daily))
	}
	if len(report.Methodologies) != 2 {
		t.Errorf("Methodologies = %d, want 2", len(report.Methodologies))
	}

	// Streaks: only the two days with work sessions should be consecutive.
	if report.Streaks.LongestDays < 2 {
		t.Errorf("LongestDays = %d, want >= 2", report.Streaks.LongestDays)
	}

	// Tags: aggregated from both work sessions.
	var deepTag *domain.TagStat
	for i := range report.Tags {
		if report.Tags[i].Tag == "deep" {
			deepTag = &report.Tags[i]
			break
		}
	}
	if deepTag == nil {
		t.Fatal("expected 'deep' tag in report")
	}
	if deepTag.SessionCount != 2 || deepTag.TotalTime != 75*time.Minute {
		t.Errorf("deep tag = %+v, want 2 sessions / 75m", deepTag)
		for _, tag := range report.Tags {
			t.Logf("tag: %+v", tag)
		}
	}
}

func TestReportService_GetReport_Month(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	monthStart, _, _ := PeriodRange(ReportPeriodMonth, now)

	session := &domain.PomodoroSession{
		ID:          "m1",
		Type:        domain.SessionTypeWork,
		Status:      domain.SessionStatusCompleted,
		Duration:    120 * time.Minute,
		StartedAt:   monthStart.Add(48 * time.Hour),
		Methodology: domain.MethodologyDeepWork,
	}
	completed := session.StartedAt
	session.CompletedAt = &completed
	if err := store.Sessions().Save(ctx, session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	svc := NewReportService(store)
	report, err := svc.GetReport(ctx, ReportPeriodMonth)
	if err != nil {
		t.Fatalf("GetReport() error = %v", err)
	}

	if report.Period != "month" {
		t.Errorf("Period = %q, want %q", report.Period, "month")
	}
	if report.Summary.TotalWorkTime != 120*time.Minute {
		t.Errorf("TotalWorkTime = %s, want 2h", report.Summary.TotalWorkTime)
	}
	if len(report.Methodologies) != 1 {
		t.Errorf("Methodologies = %d, want 1", len(report.Methodologies))
	}
}

func TestReportService_EmptyPeriod(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	svc := NewReportService(store)
	report, err := svc.GetReport(ctx, ReportPeriodWeek)
	if err != nil {
		t.Fatalf("GetReport() error = %v", err)
	}

	if report.Summary.TotalSessions != 0 {
		t.Errorf("TotalSessions = %d, want 0", report.Summary.TotalSessions)
	}
	if report.Streaks.LongestDays != 0 {
		t.Errorf("LongestDays = %d, want 0", report.Streaks.LongestDays)
	}
}

func TestReportService_TotalWork(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	session := &domain.PomodoroSession{
		ID:          "s1",
		Type:        domain.SessionTypeWork,
		Status:      domain.SessionStatusCompleted,
		Duration:    45 * time.Minute,
		StartedAt:   now.Add(-3 * time.Hour),
		Methodology: domain.MethodologyPomodoro,
	}
	completed := session.StartedAt
	session.CompletedAt = &completed
	if err := store.Sessions().Save(ctx, session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	svc := NewReportService(store)
	report, err := svc.GetReport(ctx, ReportPeriodWeek)
	if err != nil {
		t.Fatalf("GetReport() error = %v", err)
	}

	if report.Summary.TotalWorkTime != 45*time.Minute {
		t.Errorf("TotalWorkTime = %s, want 45m", report.Summary.TotalWorkTime)
	}
	if len(report.Heatmap) == 0 {
		t.Error("Heatmap should have entries for a completed work session")
	}
}

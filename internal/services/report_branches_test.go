package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
)

func TestReportService_FocusByBranch(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now()
	weekStart, _, _ := PeriodRange(ReportPeriodWeek, now)

	mk := func(id, branch string, start time.Time, d time.Duration, status domain.SessionStatus, commits int) {
		s := &domain.PomodoroSession{ID: id, Type: domain.SessionTypeWork, Status: status, Duration: d, StartedAt: start, GitBranch: branch}
		end := start.Add(d)
		s.CompletedAt = &end
		if err := store.Sessions().Save(ctx, s); err != nil {
			t.Fatalf("Save(%s) error = %v", id, err)
		}
		if commits > 0 {
			if err := store.Sessions().UpdateGitSummary(ctx, id, &domain.GitSummary{Commits: commits}); err != nil {
				t.Fatalf("UpdateGitSummary(%s) error = %v", id, err)
			}
		}
	}
	day := weekStart.Add(2 * time.Hour)
	mk("a1", "feat/auth-refresh", day, 50*time.Minute, domain.SessionStatusCompleted, 2)
	mk("a2", "feat/auth-refresh", day.Add(time.Hour), 40*time.Minute, domain.SessionStatusCompleted, 1)
	mk("a3", "feat/auth-refresh", day.Add(2*time.Hour), 40*time.Minute, domain.SessionStatusCompleted, 0)
	mk("a4", "feat/auth-refresh", day.Add(3*time.Hour), 40*time.Minute, domain.SessionStatusCompleted, 0)
	mk("b1", "fix/typo", day.Add(4*time.Hour), 25*time.Minute, domain.SessionStatusCompleted, 0)
	mk("v1", "fix/typo", day.Add(5*time.Hour), 25*time.Minute, domain.SessionStatusInterrupted, 0)
	mk("old", "feat/old", now.AddDate(0, 0, -60), time.Hour, domain.SessionStatusCompleted, 0)

	report, err := NewReportService(store).GetReport(ctx, ReportPeriodWeek)
	if err != nil {
		t.Fatalf("GetReport() error = %v", err)
	}
	if len(report.Branches) != 2 {
		t.Fatalf("Branches = %+v, want 2 (old and voided sessions excluded)", report.Branches)
	}
	auth := report.Branches[0]
	if auth.Branch != "feat/auth-refresh" || auth.SessionCount != 4 || auth.TotalTime != 170*time.Minute || auth.Commits != 3 {
		t.Errorf("auth branch = %+v, want 4 sessions, 2h50m, 3 commits", auth)
	}
	if report.Branches[1].Branch != "fix/typo" || report.Branches[1].SessionCount != 1 {
		t.Errorf("typo branch = %+v", report.Branches[1])
	}
}

func TestTaskService_FindOrAddTaskWith(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	tasks := NewTaskService(store)

	first, err := tasks.FindOrAddTaskWith(ctx, store, "auth refresh")
	if err != nil {
		t.Fatalf("FindOrAddTaskWith() error = %v", err)
	}
	again, err := tasks.FindOrAddTaskWith(ctx, store, "Auth Refresh")
	if err != nil {
		t.Fatalf("FindOrAddTaskWith() again error = %v", err)
	}
	if again.ID != first.ID {
		t.Errorf("second call created %s, want reuse of %s", again.ID, first.ID)
	}

	// A completed task of that name is not reused.
	if err := tasks.CompleteTask(ctx, first.ID); err != nil {
		t.Fatalf("CompleteTask() error = %v", err)
	}
	fresh, err := tasks.FindOrAddTaskWith(ctx, store, "auth refresh")
	if err != nil {
		t.Fatalf("FindOrAddTaskWith() after complete error = %v", err)
	}
	if fresh.ID == first.ID || !strings.EqualFold(fresh.Title, "auth refresh") {
		t.Errorf("after completion got %+v, want a new task", fresh)
	}
}

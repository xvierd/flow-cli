package storage

import (
	"context"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
)

func TestSessionRepository_GetInsightSessions(t *testing.T) {
	store, _ := NewMemory()
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	task, _ := domain.NewTask("Ship API")
	task.Tags = []string{"backend", "api"}
	task.Complete()
	if err := store.Tasks().Save(ctx, task); err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, 11, 16, 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 0, 1)
	score := 4
	zero := 0
	save := func(id string, typ domain.SessionType, status domain.SessionStatus, at time.Time, mut func(*domain.PomodoroSession)) {
		taskID := task.ID
		s := &domain.PomodoroSession{
			ID: id, TaskID: &taskID, Type: typ, Status: status, Duration: 25 * time.Minute,
			StartedAt: at, Methodology: domain.MethodologyDeepWork,
		}
		if mut != nil {
			mut(s)
		}
		if err := store.Sessions().Save(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	// Stored as a UTC instant: the projection must hand it back in local time.
	save("done", domain.SessionTypeWork, domain.SessionStatusCompleted, start.Add(9*time.Hour).UTC(), func(s *domain.PomodoroSession) {
		s.Tags = []string{"api", "urgent"}
		s.FocusScore = &score
		s.Distractions = []domain.Distraction{{Text: "slack", Category: "external"}}
	})
	save("void", domain.SessionTypeWork, domain.SessionStatusInterrupted, start.Add(17*time.Hour), func(s *domain.PomodoroSession) { s.FocusScore = &zero })
	save("cancel", domain.SessionTypeWork, domain.SessionStatusCancelled, start.Add(18*time.Hour), nil)
	save("break", domain.SessionTypeShortBreak, domain.SessionStatusCompleted, start.Add(10*time.Hour), nil)
	save("outside", domain.SessionTypeWork, domain.SessionStatusCompleted, end.Add(time.Hour), nil)

	got, err := store.Sessions().GetInsightSessions(ctx, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d sessions, want 3 (work only, finished, in range): %+v", len(got), got)
	}
	first := got[0]
	if first.ID != "done" {
		t.Fatalf("want oldest first, got %s", first.ID)
	}
	if first.StartedAt.Location() != time.Local || first.StartedAt.Hour() != 9 {
		t.Errorf("StartedAt = %v, want 09:00 local", first.StartedAt)
	}
	if !first.TaskDone || first.TaskID != task.ID {
		t.Errorf("task = %q done=%v", first.TaskID, first.TaskDone)
	}
	tags := map[string]bool{}
	for _, tag := range first.Tags {
		tags[tag] = true
	}
	if len(first.Tags) != 3 || !tags["api"] || !tags["urgent"] || !tags["backend"] {
		t.Errorf("tags = %v, want union of session and task tags", first.Tags)
	}
	if first.FocusScore == nil || *first.FocusScore != 4 || len(first.Distractions) != 1 || first.Methodology != domain.MethodologyDeepWork {
		t.Errorf("projection = %+v", first)
	}
	if got[1].FocusScore != nil {
		t.Errorf("focus score 0 means unrated, got %v", *got[1].FocusScore)
	}
	if !got[1].Abandoned() || !got[2].Abandoned() {
		t.Error("voided and cancelled sessions should be Abandoned()")
	}
}

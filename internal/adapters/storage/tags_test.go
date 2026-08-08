package storage

import (
	"context"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
)

func TestNormalizeTags(t *testing.T) {
	got := normalizeTags([]string{" Deep ", "FOCUS", "deep", "", "  ", "focus", "solo"})
	want := []string{"deep", "focus", "solo"}
	if !equalStrings(got, want) {
		t.Errorf("normalizeTags() = %v, want %v", got, want)
	}
	if got := normalizeTags(nil); len(got) != 0 {
		t.Errorf("normalizeTags(nil) = %v, want empty", got)
	}
}

func TestTaskRepository_TagsRoundTrip(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	repo := storage.Tasks()

	task, _ := domain.NewTask("tagged task")
	task.Tags = []string{" One ", "TWO", "one"}
	if err := repo.Save(ctx, task); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	found, err := repo.FindByID(ctx, task.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if !equalStrings(found.Tags, []string{"one", "two"}) {
		t.Errorf("Tags = %v, want [one two]", found.Tags)
	}

	// Update replaces the tag set; stale rows are removed.
	found.Tags = []string{"three"}
	if err := repo.Update(ctx, found); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	reloaded, err := repo.FindByID(ctx, task.ID)
	if err != nil {
		t.Fatalf("FindByID() after update error = %v", err)
	}
	if !equalStrings(reloaded.Tags, []string{"three"}) {
		t.Errorf("Tags after update = %v, want [three]", reloaded.Tags)
	}
	s := storage.(*sqliteStorage)
	if got := tagsFor(t, s.db, "SELECT tag FROM task_tags WHERE task_id = ?", task.ID); !equalStrings(got, []string{"three"}) {
		t.Errorf("task_tags rows after update = %v, want [three]", got)
	}

	// Delete cascades into task_tags (foreign_keys = ON).
	if err := repo.Delete(ctx, task.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if got := tagsFor(t, s.db, "SELECT tag FROM task_tags WHERE task_id = ?", task.ID); len(got) != 0 {
		t.Errorf("task_tags rows after delete = %v, want empty (cascade)", got)
	}
}

func TestSessionRepository_TagsRoundTrip(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	repo := storage.Sessions()
	config := domain.DefaultPomodoroConfig()

	session := domain.NewPomodoroSession(config, nil)
	session.Tags = []string{" Deep ", "WORK", "deep"}
	if err := repo.Save(ctx, session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	found, err := repo.FindByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if !equalStrings(found.Tags, []string{"deep", "work"}) {
		t.Errorf("Tags = %v, want [deep work]", found.Tags)
	}

	// Update replaces the tag set; stale rows are removed.
	found.Tags = []string{"other"}
	if err := repo.Update(ctx, found); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	reloaded, err := repo.FindByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("FindByID() after update error = %v", err)
	}
	if !equalStrings(reloaded.Tags, []string{"other"}) {
		t.Errorf("Tags after update = %v, want [other]", reloaded.Tags)
	}
	s := storage.(*sqliteStorage)
	if got := tagsFor(t, s.db, "SELECT tag FROM session_tags WHERE session_id = ?", session.ID); !equalStrings(got, []string{"other"}) {
		t.Errorf("session_tags rows after update = %v, want [other]", got)
	}
}

func TestSessionRepository_GetTagStats_Parity(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	repo := storage.Sessions()

	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.Add(24 * time.Hour)

	score := func(v int) *int { return &v }
	seed := func(id string, dur time.Duration, tags []string, focus *int, status domain.SessionStatus) {
		s := &domain.PomodoroSession{
			ID:          id,
			Type:        domain.SessionTypeWork,
			Status:      status,
			Duration:    dur,
			StartedAt:   start.Add(10 * time.Hour),
			Methodology: domain.MethodologyPomodoro,
			Tags:        tags,
			FocusScore:  focus,
		}
		if status == domain.SessionStatusCompleted {
			c := s.StartedAt
			s.CompletedAt = &c
		}
		if err := repo.Save(ctx, s); err != nil {
			t.Fatalf("Save() %s error = %v", id, err)
		}
	}

	seed("p1", 25*time.Minute, []string{"deep", "focus"}, score(4), domain.SessionStatusCompleted)
	seed("p2", 50*time.Minute, []string{"deep"}, score(8), domain.SessionStatusCompleted)
	// No focus score: counts sessions/time but not the average.
	seed("p3", 25*time.Minute, []string{"focus"}, nil, domain.SessionStatusCompleted)
	// Running sessions are excluded even though tagged.
	seed("p4", 25*time.Minute, []string{"deep"}, score(10), domain.SessionStatusRunning)

	stats, err := repo.GetTagStats(ctx, start, end)
	if err != nil {
		t.Fatalf("GetTagStats() error = %v", err)
	}

	byTag := map[string]domain.TagStat{}
	for _, s := range stats {
		byTag[s.Tag] = s
	}
	if len(byTag) != 2 {
		t.Fatalf("got %d tags (%v), want 2 (deep, focus)", len(byTag), stats)
	}

	deep := byTag["deep"]
	if deep.SessionCount != 2 {
		t.Errorf("deep SessionCount = %d, want 2", deep.SessionCount)
	}
	if deep.TotalTime != 75*time.Minute {
		t.Errorf("deep TotalTime = %s, want 1h15m", deep.TotalTime)
	}
	if deep.FocusScoreCount != 2 || deep.AvgFocusScore != 6 {
		t.Errorf("deep focus = %d/%.1f, want 2/6", deep.FocusScoreCount, deep.AvgFocusScore)
	}

	focus := byTag["focus"]
	if focus.SessionCount != 2 {
		t.Errorf("focus SessionCount = %d, want 2", focus.SessionCount)
	}
	if focus.TotalTime != 50*time.Minute {
		t.Errorf("focus TotalTime = %s, want 50m", focus.TotalTime)
	}
	if focus.FocusScoreCount != 1 || focus.AvgFocusScore != 4 {
		t.Errorf("focus focus = %d/%.1f, want 1/4", focus.FocusScoreCount, focus.AvgFocusScore)
	}
}

package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

func TestNewMemory(t *testing.T) {
	storage, err := NewMemory()
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	if storage == nil {
		t.Error("NewMemory() returned nil storage")
	}
}

func TestTaskRepository_SaveAndFind(t *testing.T) {
	storage, err := NewMemory()
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	repo := storage.Tasks()

	t.Run("save new task", func(t *testing.T) {
		task, _ := domain.NewTask("Test Task")
		err := repo.Save(ctx, task)
		if err != nil {
			t.Errorf("Save() error = %v", err)
		}
	})

	t.Run("find by id", func(t *testing.T) {
		task, _ := domain.NewTask("Find Me")
		err := repo.Save(ctx, task)
		if err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		found, err := repo.FindByID(ctx, task.ID)
		if err != nil {
			t.Errorf("FindByID() error = %v", err)
		}
		if found == nil {
			t.Fatal("FindByID() returned nil")
		}
		if found.Title != task.Title {
			t.Errorf("Found task title = %v, want %v", found.Title, task.Title)
		}
	})

	t.Run("find non-existent", func(t *testing.T) {
		_, err := repo.FindByID(ctx, "non-existent-id")
		if err != domain.ErrTaskNotFound {
			t.Errorf("FindByID() error = %v, want ErrTaskNotFound", err)
		}
	})
}

func TestTaskRepository_FindAll(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	repo := storage.Tasks()

	// Create test tasks
	task1, _ := domain.NewTask("Task 1")
	task2, _ := domain.NewTask("Task 2")
	task3, _ := domain.NewTask("Task 3")
	task3.Complete()

	_ = repo.Save(ctx, task1)
	_ = repo.Save(ctx, task2)
	_ = repo.Save(ctx, task3)

	t.Run("find all without filter", func(t *testing.T) {
		tasks, err := repo.FindAll(ctx, nil)
		if err != nil {
			t.Errorf("FindAll() error = %v", err)
		}
		if len(tasks) != 3 {
			t.Errorf("FindAll() returned %d tasks, want 3", len(tasks))
		}
	})

	t.Run("find with status filter", func(t *testing.T) {
		status := domain.StatusCompleted
		tasks, err := repo.FindAll(ctx, &status)
		if err != nil {
			t.Errorf("FindAll() error = %v", err)
		}
		if len(tasks) != 1 {
			t.Errorf("FindAll() returned %d tasks, want 1", len(tasks))
		}
	})
}

func TestTaskRepository_FindPending(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	repo := storage.Tasks()

	task1, _ := domain.NewTask("Pending Task")
	task2, _ := domain.NewTask("In Progress Task")
	task2.Start()
	task3, _ := domain.NewTask("Completed Task")
	task3.Complete()

	_ = repo.Save(ctx, task1)
	_ = repo.Save(ctx, task2)
	_ = repo.Save(ctx, task3)

	tasks, err := repo.FindPending(ctx)
	if err != nil {
		t.Errorf("FindPending() error = %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("FindPending() returned %d tasks, want 2", len(tasks))
	}
}

func TestTaskRepository_FindActive(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	repo := storage.Tasks()

	t.Run("no active task", func(t *testing.T) {
		active, err := repo.FindActive(ctx)
		if err != nil {
			t.Errorf("FindActive() error = %v", err)
		}
		if active != nil {
			t.Error("FindActive() should return nil when no active task")
		}
	})

	t.Run("with active task", func(t *testing.T) {
		task, _ := domain.NewTask("Active Task")
		task.Start()
		if err := repo.Save(ctx, task); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		active, err := repo.FindActive(ctx)
		if err != nil {
			t.Errorf("FindActive() error = %v", err)
		}
		if active == nil {
			t.Fatal("FindActive() returned nil")
		}
		if active.ID != task.ID {
			t.Errorf("FindActive() returned wrong task")
		}
	})
}

func TestTaskRepository_Update(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	repo := storage.Tasks()

	task, _ := domain.NewTask("Original Title")
	if err := repo.Save(ctx, task); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	task.Title = "Updated Title"
	task.Start()

	err := repo.Update(ctx, task)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	found, _ := repo.FindByID(ctx, task.ID)
	if found.Title != "Updated Title" {
		t.Errorf("Update() title = %v, want 'Updated Title'", found.Title)
	}
	if found.Status != domain.StatusInProgress {
		t.Errorf("Update() status = %v, want in_progress", found.Status)
	}
}

func TestTaskRepository_Delete(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	repo := storage.Tasks()

	task, _ := domain.NewTask("To Delete")
	if err := repo.Save(ctx, task); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	err := repo.Delete(ctx, task.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = repo.FindByID(ctx, task.ID)
	if err != domain.ErrTaskNotFound {
		t.Errorf("FindByID() after delete error = %v, want ErrTaskNotFound", err)
	}
}

func TestSessionRepository_SaveAndFind(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	repo := storage.Sessions()
	config := domain.DefaultPomodoroConfig()

	t.Run("save work session", func(t *testing.T) {
		session := domain.NewPomodoroSession(config, nil)
		session.Complete() // completed: the unique index allows only one active session
		err := repo.Save(ctx, session)
		if err != nil {
			t.Errorf("Save() error = %v", err)
		}
	})

	t.Run("save break session", func(t *testing.T) {
		session := domain.NewBreakSession(config, 1)
		session.Complete() // completed: the unique index allows only one active session
		err := repo.Save(ctx, session)
		if err != nil {
			t.Errorf("Save() error = %v", err)
		}
	})

	t.Run("find by id", func(t *testing.T) {
		session := domain.NewPomodoroSession(config, nil)
		if err := repo.Save(ctx, session); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		found, err := repo.FindByID(ctx, session.ID)
		if err != nil {
			t.Errorf("FindByID() error = %v", err)
		}
		if found == nil {
			t.Fatal("FindByID() returned nil")
		}
		if found.Type != session.Type {
			t.Errorf("Found session type = %v, want %v", found.Type, session.Type)
		}
	})
}

func TestSessionRepository_FindActive(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	repo := storage.Sessions()
	config := domain.DefaultPomodoroConfig()

	t.Run("find running session", func(t *testing.T) {
		session := domain.NewPomodoroSession(config, nil)
		if err := repo.Save(ctx, session); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		active, err := repo.FindActive(ctx)
		if err != nil {
			t.Errorf("FindActive() error = %v", err)
		}
		if active == nil {
			t.Fatal("FindActive() returned nil")
		}
		if active.ID != session.ID {
			t.Error("FindActive() returned wrong session")
		}
	})

	t.Run("find paused session", func(t *testing.T) {
		session := domain.NewPomodoroSession(config, nil)
		session.Pause()
		if err := repo.Save(ctx, session); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		active, err := repo.FindActive(ctx)
		if err != nil {
			t.Errorf("FindActive() error = %v", err)
		}
		if active == nil {
			t.Fatal("FindActive() returned nil for paused session")
		}
	})
}

func TestSessionRepository_Update(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	repo := storage.Sessions()
	config := domain.DefaultPomodoroConfig()

	session := domain.NewPomodoroSession(config, nil)
	if err := repo.Save(ctx, session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	session.Complete()
	err := repo.Update(ctx, session)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	found, _ := repo.FindByID(ctx, session.ID)
	if found.Status != domain.SessionStatusCompleted {
		t.Errorf("Update() status = %v, want completed", found.Status)
	}
	if found.CompletedAt == nil {
		t.Error("Update() completed_at should not be nil")
	}
}

func TestSessionRepository_FindRecent(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	repo := storage.Sessions()
	config := domain.DefaultPomodoroConfig()

	// Create sessions
	session1 := domain.NewPomodoroSession(config, nil)
	session1.Complete()
	if err := repo.Save(ctx, session1); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	sessions, err := repo.FindRecent(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Errorf("FindRecent() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("FindRecent() returned %d sessions, want 1", len(sessions))
	}
}

func TestSessionRepository_GetDailyStats(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	repo := storage.Sessions()
	config := domain.DefaultPomodoroConfig()

	// Create completed work session
	session := domain.NewPomodoroSession(config, nil)
	session.Complete()
	if err := repo.Save(ctx, session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Create completed break session
	breakSession := domain.NewBreakSession(config, 1)
	breakSession.Complete()
	if err := repo.Save(ctx, breakSession); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	stats, err := repo.GetDailyStats(ctx, time.Now())
	if err != nil {
		t.Errorf("GetDailyStats() error = %v", err)
	}
	if stats.WorkSessions != 1 {
		t.Errorf("WorkSessions = %d, want 1", stats.WorkSessions)
	}
	if stats.BreaksTaken != 1 {
		t.Errorf("BreaksTaken = %d, want 1", stats.BreaksTaken)
	}
	if stats.TotalWorkTime == 0 {
		t.Error("TotalWorkTime should not be zero")
	}
}

func TestTaskRepository_CountCompleted(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	taskRepo := storage.Tasks()

	t.Run("counts tasks completed on the day", func(t *testing.T) {
		complete := func(title string) string {
			task, err := domain.NewTask(title)
			if err != nil {
				t.Fatalf("NewTask() error = %v", err)
			}
			task.Complete()
			if err := taskRepo.Save(ctx, task); err != nil {
				t.Fatalf("Save() task error = %v", err)
			}
			return task.ID
		}
		complete("done one")
		complete("done two")

		// A task completed on a different day must be excluded.
		pending, err := domain.NewTask("not yet")
		if err != nil {
			t.Fatalf("NewTask() error = %v", err)
		}
		old := time.Now().AddDate(0, 0, -1)
		pending.CompletedAt = &old
		pending.Status = domain.StatusCompleted
		if err := taskRepo.Save(ctx, pending); err != nil {
			t.Fatalf("Save() old task error = %v", err)
		}

		count, err := taskRepo.CountCompleted(ctx, time.Now())
		if err != nil {
			t.Fatalf("CountCompleted() error = %v", err)
		}
		if count != 2 {
			t.Errorf("CountCompleted() = %d, want 2", count)
		}
	})
}

func TestSessionRepository_GetPeriodStats(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	sessionRepo := storage.Sessions()
	config := domain.DefaultPomodoroConfig()

	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.Add(24 * time.Hour)

	t.Run("empty period", func(t *testing.T) {
		stats, err := sessionRepo.GetPeriodStats(ctx, start, end)
		if err != nil {
			t.Fatalf("GetPeriodStats() error = %v", err)
		}
		if stats.TotalSessions != 0 {
			t.Errorf("TotalSessions = %d, want 0", stats.TotalSessions)
		}
		if stats.TotalWorkTime != 0 {
			t.Errorf("TotalWorkTime = %v, want 0", stats.TotalWorkTime)
		}
	})

	t.Run("with completed sessions", func(t *testing.T) {
		// Create a completed pomodoro session
		session := domain.NewPomodoroSession(config, nil)
		session.Methodology = domain.MethodologyPomodoro
		session.Complete()
		if err := sessionRepo.Save(ctx, session); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		// Create a deep work session
		dwSession := domain.NewPomodoroSession(config, nil)
		dwSession.Methodology = domain.MethodologyDeepWork
		dwSession.Distractions = []domain.Distraction{{Text: "email"}, {Text: "slack"}}
		dwSession.Complete()
		if err := sessionRepo.Save(ctx, dwSession); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		stats, err := sessionRepo.GetPeriodStats(ctx, start, end)
		if err != nil {
			t.Fatalf("GetPeriodStats() error = %v", err)
		}
		if stats.TotalSessions != 2 {
			t.Errorf("TotalSessions = %d, want 2", stats.TotalSessions)
		}
		if stats.TotalWorkTime == 0 {
			t.Error("TotalWorkTime should not be zero")
		}
		if len(stats.ByMethodology) < 2 {
			t.Errorf("ByMethodology has %d entries, want at least 2", len(stats.ByMethodology))
		}
		if stats.DistractionCount == 0 {
			t.Error("DistractionCount should not be zero")
		}
	})

	t.Run("with focus scores", func(t *testing.T) {
		mtSession := domain.NewPomodoroSession(config, nil)
		mtSession.Methodology = domain.MethodologyMakeTime
		score := 8
		mtSession.FocusScore = &score
		mtSession.Complete()
		if err := sessionRepo.Save(ctx, mtSession); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		stats, err := sessionRepo.GetPeriodStats(ctx, start, end)
		if err != nil {
			t.Fatalf("GetPeriodStats() error = %v", err)
		}
		if stats.FocusScoreCount == 0 {
			t.Error("FocusScoreCount should not be zero")
		}
		if stats.AvgFocusScore == 0 {
			t.Error("AvgFocusScore should not be zero")
		}
	})
}

func TestSessionRepository_GetDeepWorkStreak(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	sessionRepo := storage.Sessions()

	t.Run("no sessions returns zero", func(t *testing.T) {
		streak, err := sessionRepo.GetDeepWorkStreak(ctx, 1*time.Hour)
		if err != nil {
			t.Fatalf("GetDeepWorkStreak() error = %v", err)
		}
		if streak != 0 {
			t.Errorf("streak = %d, want 0", streak)
		}
	})

	t.Run("with deep work sessions", func(t *testing.T) {
		// Create a deep work session for today
		session := &domain.PomodoroSession{
			ID:          "dw-today",
			Type:        domain.SessionTypeWork,
			Status:      domain.SessionStatusCompleted,
			Duration:    90 * time.Minute,
			StartedAt:   time.Now().Add(-1 * time.Hour),
			Methodology: domain.MethodologyDeepWork,
		}
		now := time.Now()
		session.CompletedAt = &now
		if err := sessionRepo.Save(ctx, session); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		streak, err := sessionRepo.GetDeepWorkStreak(ctx, 1*time.Hour)
		if err != nil {
			t.Fatalf("GetDeepWorkStreak() error = %v", err)
		}
		if streak < 1 {
			t.Errorf("streak = %d, want >= 1", streak)
		}
	})
}

func TestSessionRepository_GetHourlyProductivity(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	sessionRepo := storage.Sessions()
	config := domain.DefaultPomodoroConfig()

	t.Run("no sessions returns empty map", func(t *testing.T) {
		result, err := sessionRepo.GetHourlyProductivity(ctx, 7)
		if err != nil {
			t.Fatalf("GetHourlyProductivity() error = %v", err)
		}
		if len(result) != 0 {
			t.Errorf("len(result) = %d, want 0", len(result))
		}
	})

	t.Run("with sessions", func(t *testing.T) {
		session := &domain.PomodoroSession{
			ID:        "hourly-test-1",
			Type:      domain.SessionTypeWork,
			Status:    domain.SessionStatusCompleted,
			Duration:  config.WorkDuration,
			StartedAt: time.Now(),
		}
		completed := time.Now()
		session.CompletedAt = &completed
		if err := sessionRepo.Save(ctx, session); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		result, err := sessionRepo.GetHourlyProductivity(ctx, 7)
		if err != nil {
			t.Fatalf("GetHourlyProductivity() error = %v", err)
		}
		wantHour := session.StartedAt.Local().Hour()
		got, ok := result[wantHour]
		if !ok {
			t.Fatalf("expected hour %d in result, got %v", wantHour, result)
		}
		if got != session.Duration {
			t.Errorf("hour %d duration = %s, want %s", wantHour, got, session.Duration)
		}
	})
}

func TestSessionRepository_GetEnergizeStats(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	sessionRepo := storage.Sessions()

	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.Add(24 * time.Hour)

	t.Run("no sessions returns empty", func(t *testing.T) {
		stats, err := sessionRepo.GetEnergizeStats(ctx, start, end)
		if err != nil {
			t.Fatalf("GetEnergizeStats() error = %v", err)
		}
		if len(stats) != 0 {
			t.Errorf("len(stats) = %d, want 0", len(stats))
		}
	})

	t.Run("with energize sessions", func(t *testing.T) {
		// Use fixed times relative to start (noon and 10 AM) to avoid midnight edge cases.
		// Using time.Now().Add(-60min) can fall in the previous day if the test runs between 00:00–01:00.
		session1Start := start.Add(12 * time.Hour) // noon today
		session1End := session1Start.Add(25 * time.Minute)
		session2Start := start.Add(10 * time.Hour) // 10 AM today
		session2End := session2Start.Add(25 * time.Minute)

		score := 7
		session := &domain.PomodoroSession{
			ID:               "energize-1",
			Type:             domain.SessionTypeWork,
			Status:           domain.SessionStatusCompleted,
			Duration:         25 * time.Minute,
			StartedAt:        session1Start,
			Methodology:      domain.MethodologyMakeTime,
			FocusScore:       &score,
			EnergizeActivity: "morning walk",
		}
		session.CompletedAt = &session1End
		if err := sessionRepo.Save(ctx, session); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		score2 := 9
		session2 := &domain.PomodoroSession{
			ID:               "energize-2",
			Type:             domain.SessionTypeWork,
			Status:           domain.SessionStatusCompleted,
			Duration:         25 * time.Minute,
			StartedAt:        session2Start,
			Methodology:      domain.MethodologyMakeTime,
			FocusScore:       &score2,
			EnergizeActivity: "morning walk",
		}
		session2.CompletedAt = &session2End
		if err := sessionRepo.Save(ctx, session2); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		stats, err := sessionRepo.GetEnergizeStats(ctx, start, end)
		if err != nil {
			t.Fatalf("GetEnergizeStats() error = %v", err)
		}
		if len(stats) != 1 {
			t.Fatalf("len(stats) = %d, want 1 (morning walk)", len(stats))
		}
		if stats[0].Activity != "morning walk" {
			t.Errorf("Activity = %v, want 'morning walk'", stats[0].Activity)
		}
		if stats[0].SessionCount != 2 {
			t.Errorf("SessionCount = %d, want 2", stats[0].SessionCount)
		}
		if stats[0].AvgFocusScore != 8 {
			t.Errorf("AvgFocusScore = %v, want 8", stats[0].AvgFocusScore)
		}
	})
}

func TestSessionRepository_GetTagStats(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	sessionRepo := storage.Sessions()

	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.Add(24 * time.Hour)

	scoreA := 4
	sessionA := &domain.PomodoroSession{
		ID:          "tags-1",
		Type:        domain.SessionTypeWork,
		Status:      domain.SessionStatusCompleted,
		Duration:    25 * time.Minute,
		StartedAt:   start.Add(10 * time.Hour),
		Methodology: domain.MethodologyPomodoro,
		Tags:        []string{"deep", "focus"},
		FocusScore:  &scoreA,
	}
	sessionA.CompletedAt = &sessionA.StartedAt
	if err := sessionRepo.Save(ctx, sessionA); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	scoreB := 8
	sessionB := &domain.PomodoroSession{
		ID:          "tags-2",
		Type:        domain.SessionTypeWork,
		Status:      domain.SessionStatusCompleted,
		Duration:    50 * time.Minute,
		StartedAt:   start.Add(12 * time.Hour),
		Methodology: domain.MethodologyDeepWork,
		Tags:        []string{"deep"},
		FocusScore:  &scoreB,
	}
	sessionB.CompletedAt = &sessionB.StartedAt
	if err := sessionRepo.Save(ctx, sessionB); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// A session outside the range with a distinct tag should be excluded.
	outside := &domain.PomodoroSession{
		ID:          "tags-3",
		Type:        domain.SessionTypeWork,
		Status:      domain.SessionStatusCompleted,
		Duration:    25 * time.Minute,
		StartedAt:   start.AddDate(0, 0, -5),
		Methodology: domain.MethodologyPomodoro,
		Tags:        []string{"other"},
	}
	outside.CompletedAt = &outside.StartedAt
	if err := sessionRepo.Save(ctx, outside); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	stats, err := sessionRepo.GetTagStats(ctx, start, end)
	if err != nil {
		t.Fatalf("GetTagStats() error = %v", err)
	}

	found := map[string]domain.TagStat{}
	for _, s := range stats {
		found[s.Tag] = s
	}

	deep, ok := found["deep"]
	if !ok {
		t.Fatal("expected 'deep' tag stats")
	}
	if deep.SessionCount != 2 {
		t.Errorf("deep SessionCount = %d, want 2", deep.SessionCount)
	}
	if deep.TotalTime != 75*time.Minute {
		t.Errorf("deep TotalTime = %s, want 1h15m", deep.TotalTime)
	}
	if deep.AvgFocusScore != 6 {
		t.Errorf("deep AvgFocusScore = %v, want 6", deep.AvgFocusScore)
	}

	focus, ok := found["focus"]
	if !ok {
		t.Fatal("expected 'focus' tag stats")
	}
	if focus.SessionCount != 1 {
		t.Errorf("focus SessionCount = %d, want 1", focus.SessionCount)
	}

	if _, exists := found["other"]; exists {
		t.Error("out-of-range tag 'other' should not be included")
	}
}

func TestTaskRepository_FindRecentTasks(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	taskRepo := storage.Tasks()
	sessionRepo := storage.Sessions()
	config := domain.DefaultPomodoroConfig()

	t.Run("no tasks with sessions", func(t *testing.T) {
		tasks, err := taskRepo.FindRecentTasks(ctx, 5)
		if err != nil {
			t.Fatalf("FindRecentTasks() error = %v", err)
		}
		if len(tasks) != 0 {
			t.Errorf("len(tasks) = %d, want 0", len(tasks))
		}
	})

	t.Run("returns tasks ordered by recent session", func(t *testing.T) {
		task1, _ := domain.NewTask("Task A")
		task2, _ := domain.NewTask("Task B")
		_ = taskRepo.Save(ctx, task1)
		_ = taskRepo.Save(ctx, task2)

		// Session for task1 (older)
		s1 := domain.NewPomodoroSession(config, &task1.ID)
		s1.Complete()
		_ = sessionRepo.Save(ctx, s1)

		// Session for task2 (newer)
		s2 := domain.NewPomodoroSession(config, &task2.ID)
		s2.Complete()
		_ = sessionRepo.Save(ctx, s2)

		tasks, err := taskRepo.FindRecentTasks(ctx, 5)
		if err != nil {
			t.Fatalf("FindRecentTasks() error = %v", err)
		}
		if len(tasks) != 2 {
			t.Fatalf("len(tasks) = %d, want 2", len(tasks))
		}
		// Most recent session (task2) should be first
		if tasks[0].ID != task2.ID {
			t.Errorf("first task = %v, want %v", tasks[0].ID, task2.ID)
		}
	})

	t.Run("respects limit", func(t *testing.T) {
		tasks, err := taskRepo.FindRecentTasks(ctx, 1)
		if err != nil {
			t.Fatalf("FindRecentTasks() error = %v", err)
		}
		if len(tasks) != 1 {
			t.Errorf("len(tasks) = %d, want 1", len(tasks))
		}
	})
}

func TestTaskRepository_FindTodayHighlight(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	repo := storage.Tasks()

	t.Run("no highlight returns nil", func(t *testing.T) {
		task, err := repo.FindTodayHighlight(ctx, time.Now())
		if err != nil {
			t.Fatalf("FindTodayHighlight() error = %v", err)
		}
		if task != nil {
			t.Error("FindTodayHighlight() should return nil when no highlight set")
		}
	})

	t.Run("returns today highlight", func(t *testing.T) {
		task, _ := domain.NewTask("Today Highlight")
		task.SetAsHighlight()
		_ = repo.Save(ctx, task)

		found, err := repo.FindTodayHighlight(ctx, time.Now())
		if err != nil {
			t.Fatalf("FindTodayHighlight() error = %v", err)
		}
		if found == nil {
			t.Fatal("FindTodayHighlight() returned nil")
		}
		if found.ID != task.ID {
			t.Errorf("FindTodayHighlight() returned wrong task")
		}
	})
}

func TestTaskRepository_FindYesterdayHighlight(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	repo := storage.Tasks()

	t.Run("no yesterday highlight returns nil", func(t *testing.T) {
		task, err := repo.FindYesterdayHighlight(ctx, time.Now())
		if err != nil {
			t.Fatalf("FindYesterdayHighlight() error = %v", err)
		}
		if task != nil {
			t.Error("FindYesterdayHighlight() should return nil when no highlight")
		}
	})

	t.Run("returns incomplete yesterday highlight", func(t *testing.T) {
		yesterday := time.Now().AddDate(0, 0, -1)
		yesterdayStart := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, yesterday.Location())

		task, _ := domain.NewTask("Yesterday Highlight")
		task.HighlightDate = &yesterdayStart
		_ = repo.Save(ctx, task)

		found, err := repo.FindYesterdayHighlight(ctx, time.Now())
		if err != nil {
			t.Fatalf("FindYesterdayHighlight() error = %v", err)
		}
		if found == nil {
			t.Fatal("FindYesterdayHighlight() returned nil")
		}
		if found.ID != task.ID {
			t.Errorf("FindYesterdayHighlight() returned wrong task")
		}
	})

	t.Run("excludes completed yesterday highlight", func(t *testing.T) {
		yesterday := time.Now().AddDate(0, 0, -1)
		yesterdayStart := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, yesterday.Location())

		task, _ := domain.NewTask("Completed Yesterday")
		task.HighlightDate = &yesterdayStart
		task.Complete()
		_ = repo.Save(ctx, task)

		found, err := repo.FindYesterdayHighlight(ctx, time.Now())
		if err != nil {
			t.Fatalf("FindYesterdayHighlight() error = %v", err)
		}
		// Should still return the first incomplete one, or nil if only completed tasks
		// The first (incomplete) task from previous subtest might still be there
		if found != nil && found.Status == domain.StatusCompleted {
			t.Error("FindYesterdayHighlight() should not return completed tasks")
		}
	})
}

func TestStorage_DistractionBackwardCompat(t *testing.T) {
	t.Run("new JSON format", func(t *testing.T) {
		data := `[{"Text":"email","Category":"external"},{"Text":"phone","Category":""}]`
		result := unmarshalDistractions(data)
		if len(result) != 2 {
			t.Fatalf("got %d distractions, want 2", len(result))
		}
		if result[0].Text != "email" || result[0].Category != "external" {
			t.Errorf("first distraction = %+v, want {Text:email Category:external}", result[0])
		}
		if result[1].Text != "phone" {
			t.Errorf("second distraction text = %q, want phone", result[1].Text)
		}
	})

	t.Run("old newline-separated format", func(t *testing.T) {
		data := "checked email\nlooked at phone"
		result := unmarshalDistractions(data)
		if len(result) != 2 {
			t.Fatalf("got %d distractions, want 2", len(result))
		}
		if result[0].Text != "checked email" {
			t.Errorf("first distraction text = %q, want 'checked email'", result[0].Text)
		}
		if result[0].Category != "" {
			t.Errorf("first distraction category = %q, want empty", result[0].Category)
		}
		if result[1].Text != "looked at phone" {
			t.Errorf("second distraction text = %q, want 'looked at phone'", result[1].Text)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		result := unmarshalDistractions("")
		if result != nil {
			t.Errorf("got %v, want nil", result)
		}
	})
}

func TestStorage_ShutdownRitualPersistence(t *testing.T) {
	store, _ := NewMemory()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	repo := store.Sessions()
	config := domain.DefaultPomodoroConfig()

	session := domain.NewPomodoroSession(config, nil)
	session.Methodology = domain.MethodologyDeepWork
	session.ShutdownRitual = &domain.ShutdownRitual{
		PendingTasksReview: "reviewed inbox",
		TomorrowPlan:       "finish API endpoint",
		ClosingPhrase:      "shutdown complete",
	}
	session.Complete()

	if err := repo.Save(ctx, session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	found, err := repo.FindByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if found.ShutdownRitual == nil {
		t.Fatal("ShutdownRitual should not be nil after reload")
	}
	if found.ShutdownRitual.PendingTasksReview != "reviewed inbox" {
		t.Errorf("PendingTasksReview = %q, want 'reviewed inbox'", found.ShutdownRitual.PendingTasksReview)
	}
	if found.ShutdownRitual.TomorrowPlan != "finish API endpoint" {
		t.Errorf("TomorrowPlan = %q, want 'finish API endpoint'", found.ShutdownRitual.TomorrowPlan)
	}
	if found.ShutdownRitual.ClosingPhrase != "shutdown complete" {
		t.Errorf("ClosingPhrase = %q, want 'shutdown complete'", found.ShutdownRitual.ClosingPhrase)
	}
}

func TestStorage_DistractionPersistence(t *testing.T) {
	store, _ := NewMemory()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	repo := store.Sessions()
	config := domain.DefaultPomodoroConfig()

	session := domain.NewPomodoroSession(config, nil)
	session.Distractions = []domain.Distraction{
		{Text: "email notification", Category: "external"},
		{Text: "random thought", Category: "internal"},
	}

	if err := repo.Save(ctx, session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	found, err := repo.FindByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if len(found.Distractions) != 2 {
		t.Fatalf("got %d distractions, want 2", len(found.Distractions))
	}
	if found.Distractions[0].Text != "email notification" || found.Distractions[0].Category != "external" {
		t.Errorf("first distraction = %+v, want {Text:email notification Category:external}", found.Distractions[0])
	}
	if found.Distractions[1].Text != "random thought" || found.Distractions[1].Category != "internal" {
		t.Errorf("second distraction = %+v, want {Text:random thought Category:internal}", found.Distractions[1])
	}
}

func TestSessionRepository_GetDeepWorkHours(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	repo := storage.Sessions()
	config := domain.DefaultPomodoroConfig()
	now := time.Now()

	seed := func(methodology domain.Methodology, status domain.SessionStatus, dur time.Duration, startedAt time.Time) {
		s := domain.NewPomodoroSession(config, nil)
		s.Methodology = methodology
		s.Duration = dur
		s.StartedAt = startedAt
		if status == domain.SessionStatusCompleted {
			s.Complete()
		}
		if err := repo.Save(ctx, s); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}

	start := now.Add(-7 * 24 * time.Hour)
	end := now

	seed(domain.MethodologyDeepWork, domain.SessionStatusCompleted, 90*time.Minute, now.Add(-24*time.Hour))
	seed(domain.MethodologyDeepWork, domain.SessionStatusCompleted, 30*time.Minute, now.Add(-48*time.Hour))
	// Not completed → excluded.
	seed(domain.MethodologyDeepWork, domain.SessionStatusRunning, 60*time.Minute, now)
	// Wrong methodology → excluded.
	seed(domain.MethodologyPomodoro, domain.SessionStatusCompleted, 25*time.Minute, now.Add(-2*time.Hour))
	// Out of range → excluded.
	seed(domain.MethodologyDeepWork, domain.SessionStatusCompleted, 45*time.Minute, now.AddDate(0, 0, 30))

	total, err := repo.GetDeepWorkHours(ctx, start, end)
	if err != nil {
		t.Fatalf("GetDeepWorkHours() error = %v", err)
	}
	if total != 2*time.Hour {
		t.Errorf("GetDeepWorkHours() = %v, want 2h", total)
	}
}

func TestSessionRepository_FindByTask(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	repo := storage.Sessions()
	taskRepo := storage.Tasks()
	config := domain.DefaultPomodoroConfig()

	task, _ := domain.NewTask("linked task")
	if err := taskRepo.Save(ctx, task); err != nil {
		t.Fatalf("Save() task error = %v", err)
	}
	other, _ := domain.NewTask("other task")
	if err := taskRepo.Save(ctx, other); err != nil {
		t.Fatalf("Save() task error = %v", err)
	}

	s1 := domain.NewPomodoroSession(config, &task.ID)
	s1.Complete() // completed: the unique index allows only one active session
	_ = repo.Save(ctx, s1)
	s2 := domain.NewPomodoroSession(config, &task.ID)
	s2.Complete()
	_ = repo.Save(ctx, s2)
	s3 := domain.NewPomodoroSession(config, &other.ID)
	s3.Complete()
	_ = repo.Save(ctx, s3)

	sessions, err := repo.FindByTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("FindByTask() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("FindByTask() returned %d sessions, want 2", len(sessions))
	}
	ids := map[string]bool{}
	for _, s := range sessions {
		ids[s.ID] = true
	}
	if !ids[s1.ID] || !ids[s2.ID] {
		t.Errorf("FindByTask() sessions = %v, want to include %s and %s", ids, s1.ID, s2.ID)
	}
}

func TestStorage_MigrateIdempotent(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	// NewMemory already ran Migrate; running it again must be a no-op.
	if err := storage.Migrate(); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}
	if err := storage.Migrate(); err != nil {
		t.Fatalf("third Migrate() error = %v", err)
	}

	ctx := context.Background()
	if err := storage.Sessions().Save(ctx, domain.NewPomodoroSession(domain.DefaultPomodoroConfig(), nil)); err != nil {
		t.Errorf("Save() after repeat migrates error = %v", err)
	}
}

func TestStorage_WithTx_Commit(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	config := domain.DefaultPomodoroConfig()

	task, _ := domain.NewTask("tx task")
	session := domain.NewPomodoroSession(config, &task.ID)

	err := storage.WithTx(ctx, func(tx ports.Storage) error {
		if err := tx.Tasks().Save(ctx, task); err != nil {
			return err
		}
		return tx.Sessions().Save(ctx, session)
	})
	if err != nil {
		t.Fatalf("WithTx() error = %v", err)
	}

	// Both writes are visible through the root storage after commit.
	if found, err := storage.Tasks().FindByID(ctx, task.ID); err != nil || found == nil {
		t.Errorf("task not visible after commit: found=%v err=%v", found, err)
	}
	if found, err := storage.Sessions().FindByID(ctx, session.ID); err != nil || found == nil {
		t.Errorf("session not visible after commit: found=%v err=%v", found, err)
	}
}

func TestStorage_WithTx_Rollback(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	config := domain.DefaultPomodoroConfig()

	task, _ := domain.NewTask("tx rollback task")
	session := domain.NewPomodoroSession(config, &task.ID)

	boom := errors.New("boom")
	err := storage.WithTx(ctx, func(tx ports.Storage) error {
		if err := tx.Tasks().Save(ctx, task); err != nil {
			return err
		}
		if err := tx.Sessions().Save(ctx, session); err != nil {
			return err
		}
		return boom
	})
	if err != boom {
		t.Fatalf("WithTx() error = %v, want %v", err, boom)
	}

	// Neither write persisted.
	if _, err := storage.Tasks().FindByID(ctx, task.ID); err != domain.ErrTaskNotFound {
		t.Errorf("FindByID() after rollback error = %v, want ErrTaskNotFound", err)
	}
	if found, err := storage.Sessions().FindByID(ctx, session.ID); err != nil || found != nil {
		t.Errorf("session persisted despite rollback: found=%v err=%v", found, err)
	}
}

func TestStorage_WithTx_NestingRejected(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	err := storage.WithTx(ctx, func(tx ports.Storage) error {
		return tx.WithTx(ctx, func(inner ports.Storage) error {
			return nil
		})
	})
	if err != errNestedTx {
		t.Errorf("nested WithTx() error = %v, want %v", err, errNestedTx)
	}
}

func TestStorage_WithTx_InterleavedSingleConnection(t *testing.T) {
	// With the pool pinned to one connection (SetMaxOpenConns(1)), a
	// transaction holds that connection for its whole lifetime. This would
	// deadlock if any code path inside WithTx queried through the root
	// storage, and the in-memory database would diverge if transactions and
	// plain calls used different pooled connections.
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	config := domain.DefaultPomodoroConfig()

	for i := 0; i < 3; i++ {
		task, _ := domain.NewTask(fmt.Sprintf("interleave task %d", i))
		if err := storage.Tasks().Save(ctx, task); err != nil {
			t.Fatalf("Save() task error = %v", err)
		}

		session := domain.NewPomodoroSession(config, &task.ID)
		session.Complete()
		err := storage.WithTx(ctx, func(tx ports.Storage) error {
			if _, err := tx.Tasks().FindByID(ctx, task.ID); err != nil {
				return err
			}
			task.Start()
			if err := tx.Tasks().Update(ctx, task); err != nil {
				return err
			}
			return tx.Sessions().Save(ctx, session)
		})
		if err != nil {
			t.Fatalf("WithTx() iteration %d error = %v", i, err)
		}

		if _, err := storage.Sessions().FindRecent(ctx, time.Now().Add(-time.Hour)); err != nil {
			t.Fatalf("FindRecent() iteration %d error = %v", i, err)
		}
	}

	tasks, err := storage.Tasks().FindAll(ctx, nil)
	if err != nil {
		t.Fatalf("FindAll() error = %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("FindAll() returned %d tasks, want 3", len(tasks))
	}
}

func TestSessionRepository_OneActiveSessionIndex(t *testing.T) {
	storage, _ := NewMemory()
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	repo := storage.Sessions()
	config := domain.DefaultPomodoroConfig()

	first := domain.NewPomodoroSession(config, nil)
	if err := repo.Save(ctx, first); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// A second active session violates the partial unique index.
	second := domain.NewPomodoroSession(config, nil)
	err := repo.Save(ctx, second)
	if err == nil {
		t.Fatal("Save() of second running session should violate idx_sessions_one_active")
	}
	if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
		t.Errorf("Save() error = %v, want UNIQUE constraint violation", err)
	}

	// Once the first session is completed, a new active session is allowed.
	first.Complete()
	if err := repo.Update(ctx, first); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if err := repo.Save(ctx, second); err != nil {
		t.Errorf("Save() after completing active session error = %v", err)
	}
}

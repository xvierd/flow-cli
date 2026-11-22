package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/services"
)

func TestParseQuickDuration(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"25", 25 * time.Minute, true},
		{"5", 5 * time.Minute, true},
		{"90m", 90 * time.Minute, true},
		{"90M", 90 * time.Minute, true},
		{"1h", time.Hour, true},
		{"1h30m", 90 * time.Minute, true},
		{"480", 8 * time.Hour, true},
		{"0", 0, false},
		{"0m", 0, false},
		{"481", 0, false},  // over 8h
		{"9h", 0, false},   // over 8h
		{"1000", 0, false}, // four digits
		{"12345678", 0, false},
		{"4f894d3e", 0, false}, // short task ID
		{"25s", 0, false},
		{"1.5h", 0, false},
		{"-5", 0, false},
		{"go", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseQuickDuration(tt.in)
		if ok != tt.ok || got != tt.want {
			t.Errorf("parseQuickDuration(%q) = %v, %v; want %v, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestRootArgs(t *testing.T) {
	valid := [][]string{
		nil,
		{"25"},
		{"25", "fix auth #backend"},
		{"90m", "write", "the", "RFC"},
	}
	for _, args := range valid {
		if err := rootArgs(rootCmd, args); err != nil {
			t.Errorf("rootArgs(%q) = %v, want nil", args, err)
		}
	}

	for _, args := range [][]string{{"statz"}, {"12345678"}, {"4f894d3e", "x"}, {"fix auth"}} {
		err := rootArgs(rootCmd, args)
		if err == nil {
			t.Errorf("rootArgs(%q) = nil, want an unknown command error", args)
			continue
		}
		if !strings.Contains(err.Error(), "unknown command") {
			t.Errorf("rootArgs(%q) error = %q, want it to say unknown command", args, err)
		}
	}

	if err := rootArgs(rootCmd, []string{"statz"}); err == nil || !strings.Contains(err.Error(), "stats") {
		t.Errorf("a typo should suggest the subcommand, got %v", err)
	}
}

// TestRootCmd_SubcommandsWin ensures subcommands are still dispatched before
// the duration shortcut.
func TestRootCmd_SubcommandsWin(t *testing.T) {
	for _, name := range []string{"go", "start", "stats", "list"} {
		cmd, _, err := rootCmd.Find([]string{name})
		if err != nil || cmd.Name() != name {
			t.Errorf("Find(%q) = %v, %v; want the %s subcommand", name, cmd.Name(), err, name)
		}
	}
	cmd, args, err := rootCmd.Find([]string{"25", "task"})
	if err != nil || cmd != rootCmd || len(args) != 2 {
		t.Errorf("Find(25 task) = %v %v %v, want root with both args", cmd.Name(), args, err)
	}
}

func workSession(m domain.Methodology, status domain.SessionStatus, d time.Duration, taskID string, tags ...string) *domain.PomodoroSession {
	s := domain.NewPomodoroSession(domain.DefaultPomodoroConfig(), nil)
	s.Methodology = m
	s.Status = status
	s.Duration = d
	if taskID != "" {
		s.TaskID = &taskID
	}
	s.Tags = tags
	return s
}

func TestLastSessionPlan(t *testing.T) {
	if _, ok := lastSessionPlan(nil); ok {
		t.Fatal("no sessions: want no plan")
	}

	brk := domain.NewBreakSession(domain.DefaultPomodoroConfig(), 1)
	brk.Status = domain.SessionStatusCompleted

	// Newest first: a break, a cancelled session, a session stopped early
	// (duration shrunk to the time worked), then a full 50m session.
	sessions := []*domain.PomodoroSession{
		brk,
		workSession(domain.MethodologyPomodoro, domain.SessionStatusCancelled, 25*time.Minute, "cancelled-task"),
		workSession(domain.MethodologyPomodoro, domain.SessionStatusCompleted, 3*time.Minute+12345*time.Millisecond, "task-1", "backend"),
		workSession(domain.MethodologyPomodoro, domain.SessionStatusCompleted, 50*time.Minute, "task-0"),
	}
	plan, ok := lastSessionPlan(sessions)
	if !ok {
		t.Fatal("want a plan")
	}
	if plan.TaskID == nil || *plan.TaskID != "task-1" {
		t.Errorf("task = %v, want task-1 (latest non-cancelled work session)", plan.TaskID)
	}
	if len(plan.Tags) != 1 || plan.Tags[0] != "backend" {
		t.Errorf("tags = %v, want [backend]", plan.Tags)
	}
	if plan.Duration != 50*time.Minute {
		t.Errorf("duration = %v, want the 50m planned before the early stop", plan.Duration)
	}
	if plan.Methodology != domain.MethodologyPomodoro {
		t.Errorf("methodology = %q, want pomodoro", plan.Methodology)
	}
}

func TestLastSessionPlan_DurationFollowsMethodology(t *testing.T) {
	sessions := []*domain.PomodoroSession{
		workSession(domain.MethodologyDeepWork, domain.SessionStatusInterrupted, 90*time.Minute, ""),
		workSession(domain.MethodologyPomodoro, domain.SessionStatusCompleted, 25*time.Minute, ""),
	}
	plan, _ := lastSessionPlan(sessions)
	if plan.Methodology != domain.MethodologyDeepWork || plan.Duration != 90*time.Minute {
		t.Errorf("plan = %+v, want deepwork 90m", plan)
	}

	// Legacy rows without a methodology count as Pomodoro; with no whole
	// minute duration to repeat the default (zero) is used.
	legacy := workSession("", domain.SessionStatusCompleted, 7*time.Minute+time.Second, "")
	plan, _ = lastSessionPlan([]*domain.PomodoroSession{legacy})
	if plan.Methodology != domain.MethodologyPomodoro || plan.Duration != 0 {
		t.Errorf("legacy plan = %+v, want pomodoro with the default duration", plan)
	}
}

func TestPresetIndexFor(t *testing.T) {
	presets := []config.SessionPreset{{Name: "Focus", Duration: 25 * time.Minute}, {Name: "Short", Duration: 15 * time.Minute}, {Name: "Deep", Duration: 50 * time.Minute}}
	for d, want := range map[time.Duration]int{0: 0, 25 * time.Minute: 0, 15 * time.Minute: 1, 50 * time.Minute: 2, 40 * time.Minute: 0} {
		if got := presetIndexFor(presets, d); got != want {
			t.Errorf("presetIndexFor(%v) = %d, want %d", d, got, want)
		}
	}
}

// TestRunQuickStart_DurationTaskAndTags covers "flow 90m fix auth #backend":
// the session gets that duration, task and tags, and a second start with the
// same title reuses the task instead of creating a duplicate.
func TestRunQuickStart_DurationTaskAndTags(t *testing.T) {
	setupBackgroundApp(t)
	ctx := context.Background()

	if err := runRoot(rootCmd, []string{"90m", "fix", "auth", "#backend"}); err != nil {
		t.Fatalf("runRoot() error = %v", err)
	}
	state, err := app.state.GetCurrentState(ctx)
	if err != nil || state.ActiveSession == nil {
		t.Fatalf("no active session after quick start (err %v)", err)
	}
	s := state.ActiveSession
	if s.Duration != 90*time.Minute {
		t.Errorf("duration = %v, want 90m", s.Duration)
	}
	if len(s.Tags) != 1 || s.Tags[0] != "backend" {
		t.Errorf("tags = %v, want [backend]", s.Tags)
	}
	if state.ActiveTask == nil || state.ActiveTask.Title != "fix auth" {
		t.Fatalf("task = %v, want \"fix auth\"", state.ActiveTask)
	}
	firstTask := state.ActiveTask.ID

	if _, err := app.pomodoro.StopSession(ctx); err != nil {
		t.Fatalf("StopSession() error = %v", err)
	}
	if err := runRoot(rootCmd, []string{"25", "Fix Auth"}); err != nil {
		t.Fatalf("second runRoot() error = %v", err)
	}
	state, _ = app.state.GetCurrentState(ctx)
	if state.ActiveTask == nil || state.ActiveTask.ID != firstTask {
		t.Errorf("second start task = %v, want the existing task %s reused", state.ActiveTask, firstTask)
	}
	if state.ActiveSession.Duration != 25*time.Minute {
		t.Errorf("second duration = %v, want 25m", state.ActiveSession.Duration)
	}
}

// TestGoCmd_RepeatsLastSession: after a completed 50m session, a session on
// another task that was stopped early, "flow go" starts 50m on that task.
func TestGoCmd_RepeatsLastSession(t *testing.T) {
	setupBackgroundApp(t)
	ctx := context.Background()

	// Empty database: the default 25-minute Pomodoro.
	if err := goCmd.RunE(goCmd, nil); err != nil {
		t.Fatalf("go on empty db: %v", err)
	}
	state, _ := app.state.GetCurrentState(ctx)
	if state.ActiveSession == nil || state.ActiveSession.Duration != 25*time.Minute || state.ActiveSession.TaskID != nil {
		t.Fatalf("empty-db go = %+v, want a 25m session without task", state.ActiveSession)
	}
	if err := app.pomodoro.CancelSession(ctx); err != nil {
		t.Fatal(err)
	}

	saveEndedSession(t, 50*time.Minute, nil, nil)
	task, err := app.tasks.AddTask(ctx, services.AddTaskRequest{Title: "write RFC"})
	if err != nil {
		t.Fatal(err)
	}
	// Stopped early: Duration shrunk to the time worked.
	saveEndedSession(t, 4*time.Minute+321*time.Millisecond, &task.ID, []string{"docs"})

	if err := goCmd.RunE(goCmd, nil); err != nil {
		t.Fatalf("go: %v", err)
	}
	state, _ = app.state.GetCurrentState(ctx)
	s := state.ActiveSession
	if s == nil {
		t.Fatal("no active session after flow go")
	}
	if s.Duration != 50*time.Minute {
		t.Errorf("duration = %v, want 50m (the last planned length)", s.Duration)
	}
	if s.TaskID == nil || *s.TaskID != task.ID {
		t.Errorf("task = %v, want %s", s.TaskID, task.ID)
	}
	if len(s.Tags) != 1 || s.Tags[0] != "docs" {
		t.Errorf("tags = %v, want [docs]", s.Tags)
	}
}

// saveEndedSession stores a completed Pomodoro work session that started
// a little later than any saved before it.
func saveEndedSession(t *testing.T, d time.Duration, taskID *string, tags []string) *domain.PomodoroSession {
	t.Helper()
	s := domain.NewPomodoroSession(domain.DefaultPomodoroConfig(), taskID)
	s.Methodology = domain.MethodologyPomodoro
	s.Duration = d
	s.Tags = tags
	sessionClock = sessionClock.Add(time.Hour)
	s.StartedAt = sessionClock
	s.Complete()
	if err := app.storage.Sessions().Save(context.Background(), s); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	return s
}

var sessionClock = time.Now().Add(-48 * time.Hour)

package cmd

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/methodology"
)

// TestLastWorkSession_StartRequest guards the bare [s] start: it must repeat
// the last work session of the current methodology instead of a generic
// Pomodoro, and fall back to the mode's first preset otherwise.
func TestLastWorkSession_StartRequest(t *testing.T) {
	deep := methodology.ForMethodology(domain.MethodologyDeepWork, nil)
	firstDeep := deep.Presets()[0].Duration

	var empty lastWorkSession
	req := empty.startRequest("/w", domain.MethodologyDeepWork, deep)
	if req.Duration != firstDeep || req.Methodology != domain.MethodologyDeepWork || req.TaskID != nil {
		t.Errorf("nothing observed: got %+v, want first Deep Work preset", req)
	}

	var l lastWorkSession
	l.observe(nil)
	l.observe(&domain.PomodoroSession{Type: domain.SessionTypeShortBreak, Duration: 5 * time.Minute})
	if l.set {
		t.Fatal("nil and break sessions must be ignored")
	}

	taskID := "task-1"
	l.observe(&domain.PomodoroSession{
		Type: domain.SessionTypeWork, Duration: 90 * time.Minute, TaskID: &taskID,
		Methodology: domain.MethodologyDeepWork, Tags: []string{"api"},
	})
	req = l.startRequest("/w", domain.MethodologyDeepWork, deep)
	if req.Duration != 90*time.Minute || req.TaskID == nil || *req.TaskID != taskID || len(req.Tags) != 1 || req.WorkingDir != "/w" {
		t.Errorf("same methodology should repeat the last session, got %+v", req)
	}

	pomo := methodology.ForMethodology(domain.MethodologyPomodoro, nil)
	req = l.startRequest("/w", domain.MethodologyPomodoro, pomo)
	if req.Duration != pomo.Presets()[0].Duration || req.TaskID != nil || req.Methodology != domain.MethodologyPomodoro {
		t.Errorf("after a mode switch the first preset of the new mode is used, got %+v", req)
	}
}

// TestCompletionInfoFor_LongBreakRule pins the next-break context to
// domain.NewBreakSession's rule, given the number of work sessions done.
func TestCompletionInfoFor_LongBreakRule(t *testing.T) {
	isolateApp(t)
	dbPath = filepath.Join(t.TempDir(), "flow.db")
	modeFlag = "pomodoro"
	if err := initializeServices(); err != nil {
		t.Fatalf("initializeServices() error = %v", err)
	}
	ctx := t.Context()
	short, long := app.config.GetBreakDurations(domain.MethodologyPomodoro)

	cases := []struct {
		done      int
		wantType  domain.SessionType
		wantDur   time.Duration
		wantUntil int
	}{
		{0, domain.SessionTypeShortBreak, short, 4},
		{1, domain.SessionTypeShortBreak, short, 3},
		{3, domain.SessionTypeShortBreak, short, 1},
		{4, domain.SessionTypeLongBreak, long, 0},
		{5, domain.SessionTypeShortBreak, short, 3},
	}
	for _, c := range cases {
		info := completionInfoFor(ctx, c.done)
		if info.NextBreakType != c.wantType || info.NextBreakDuration != c.wantDur || info.SessionsUntilLong != c.wantUntil {
			t.Errorf("done=%d: got %+v, want type=%s dur=%v until=%d", c.done, info, c.wantType, c.wantDur, c.wantUntil)
		}
		// Same rule as the break actually started.
		brk := domain.NewBreakSession(domain.PomodoroConfig{ShortBreakDuration: short, LongBreakDuration: long, SessionsBeforeLong: 4}, c.done)
		if brk.Type != info.NextBreakType {
			t.Errorf("done=%d: completion screen says %s but [b] starts %s", c.done, info.NextBreakType, brk.Type)
		}
	}
}

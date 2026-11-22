package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/i18n"
)

func guidanceState(m Methodology, typ SessionType, status SessionStatus, remaining time.Duration, now time.Time, task string) *CurrentState {
	s := &PomodoroSession{
		ID:          "s1",
		Type:        typ,
		Status:      status,
		Methodology: m,
		Duration:    90 * time.Minute,
		StartedAt:   now.Add(-(90*time.Minute - remaining)),
	}
	if status == SessionStatusPaused {
		p := now
		s.PausedAt = &p
	}
	state := &CurrentState{ActiveSession: s}
	if task != "" {
		state.ActiveTask = &Task{ID: "t1", Title: task}
	}
	return state
}

func TestFocusGuidance(t *testing.T) {
	i18n.SetLanguage("en")
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		state    *CurrentState
		mode     FocusMode
		contains []string
		absent   []string
	}{
		{"no session", &CurrentState{}, FocusModeIdle, []string{"No focus session is active"}, nil},
		{"nil state", nil, FocusModeIdle, []string{"No focus session is active"}, nil},
		{"completed session is idle", guidanceState(MethodologyDeepWork, SessionTypeWork, SessionStatusCompleted, 0, now, "x"), FocusModeIdle, []string{"No focus session"}, nil},
		{
			"deep work",
			guidanceState(MethodologyDeepWork, SessionTypeWork, SessionStatusRunning, 40*time.Minute, now, "Refactor auth"),
			FocusModeDeepWork,
			[]string{"deep focus", "40 min left", "'Refactor auth'", "concise", "log_distraction", "create_task"},
			[]string{"about to end"},
		},
		{
			"deep work rounds remaining up",
			guidanceState(MethodologyDeepWork, SessionTypeWork, SessionStatusRunning, 39*time.Minute+time.Second, now, ""),
			FocusModeDeepWork,
			[]string{"40 min left"},
			[]string{"task '"},
		},
		{
			"wrap up near the end",
			guidanceState(MethodologyPomodoro, SessionTypeWork, SessionStatusRunning, 3*time.Minute, now, "Docs"),
			FocusModePomodoro,
			[]string{"Pomodoro", "3 min left", "about to end"},
			nil,
		},
		{
			"make time",
			guidanceState(MethodologyMakeTime, SessionTypeWork, SessionStatusRunning, 30*time.Minute, now, "Ship v2"),
			FocusModeMakeTime,
			[]string{"Highlight 'Ship v2'", "30 min left"},
			nil,
		},
		{
			"break",
			guidanceState(MethodologyDeepWork, SessionTypeShortBreak, SessionStatusRunning, 4*time.Minute, now, ""),
			FocusModeBreak,
			[]string{"break", "4 min left", "rest"},
			[]string{"about to end"},
		},
		{
			"paused",
			guidanceState(MethodologyDeepWork, SessionTypeWork, SessionStatusPaused, 20*time.Minute, now, "Refactor"),
			FocusModePaused,
			[]string{"paused", "'Refactor'", "Do not push"},
			[]string{"about to end"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CurrentFocusMode(tt.state); got != tt.mode {
				t.Errorf("CurrentFocusMode() = %q, want %q", got, tt.mode)
			}
			g := FocusGuidance(tt.state, now)
			for _, want := range tt.contains {
				if !strings.Contains(g, want) {
					t.Errorf("guidance %q does not contain %q", g, want)
				}
			}
			for _, bad := range tt.absent {
				if strings.Contains(g, bad) {
					t.Errorf("guidance %q unexpectedly contains %q", g, bad)
				}
			}
		})
	}
}

func TestFocusGuidance_IntendedOutcomeAndSpanish(t *testing.T) {
	now := time.Now()
	state := guidanceState(MethodologyDeepWork, SessionTypeWork, SessionStatusRunning, 40*time.Minute, now, "Auth")
	state.ActiveSession.IntendedOutcome = "login works."

	i18n.SetLanguage("en")
	if g := FocusGuidance(state, now); !strings.Contains(g, "Intended outcome for this session: login works.") {
		t.Errorf("guidance %q lacks the intended outcome", g)
	}

	i18n.SetLanguage("es")
	defer i18n.SetLanguage("en")
	if g := FocusGuidance(state, now); !strings.Contains(g, "foco profundo") || !strings.Contains(g, "log_distraction") {
		t.Errorf("Spanish guidance = %q", g)
	}
}

func TestRemainingAt(t *testing.T) {
	now := time.Now()
	s := &PomodoroSession{Status: SessionStatusRunning, Duration: 10 * time.Minute, StartedAt: now.Add(-4 * time.Minute)}
	if got := s.RemainingAt(now); got != 6*time.Minute {
		t.Errorf("running RemainingAt = %v", got)
	}
	if got := s.RemainingAt(now.Add(time.Hour)); got != 0 {
		t.Errorf("overdue RemainingAt = %v, want 0", got)
	}
	p := now.Add(-time.Minute)
	s.Status, s.PausedAt = SessionStatusPaused, &p
	if got := s.RemainingAt(now.Add(time.Hour)); got != 7*time.Minute {
		t.Errorf("paused RemainingAt = %v, want 7m", got)
	}
}

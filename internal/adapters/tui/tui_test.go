package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{25 * time.Minute, "25:00"},
		{5 * time.Minute, "05:00"},
		{1*time.Minute + 30*time.Second, "01:30"},
		{0, "00:00"},
		{90 * time.Second, "01:30"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDuration(tt.duration)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %v, want %v", tt.duration, got, tt.want)
			}
		})
	}
}

func TestNewModel(t *testing.T) {
	state := &domain.CurrentState{}
	model := NewModel(state, nil, nil)

	if model.state != state {
		t.Error("NewModel() should store the initial state")
	}

	if model.state == nil {
		t.Error("NewModel() should store state")
	}
}

func TestModel_View(t *testing.T) {
	config := domain.DefaultPomodoroConfig()
	session := domain.NewPomodoroSession(config, nil)
	task, _ := domain.NewTask("Test Task")

	state := &domain.CurrentState{
		ActiveTask:    task,
		ActiveSession: session,
		TodayStats: domain.DailyStats{
			WorkSessions:  2,
			BreaksTaken:   1,
			TotalWorkTime: 50 * time.Minute,
		},
	}

	model := NewModel(state, nil, nil)
	model.width = 80
	model.height = 24

	view := model.View()

	if view == "" {
		t.Error("View() should not return empty string")
	}

	if view == "Loading..." {
		t.Error("View() should not show loading when width is set")
	}
}

func TestModel_View_NoActiveSession(t *testing.T) {
	state := &domain.CurrentState{
		ActiveSession: nil,
		TodayStats:    domain.DailyStats{},
	}

	model := NewModel(state, nil, nil)
	model.width = 80
	model.height = 24

	view := model.View()

	if view == "" {
		t.Error("View() should not return empty string")
	}
}

func TestModel_View_WorkComplete(t *testing.T) {
	state := &domain.CurrentState{
		TodayStats: domain.DailyStats{
			WorkSessions:  3,
			BreaksTaken:   2,
			TotalWorkTime: 75 * time.Minute,
		},
	}

	info := &domain.CompletionInfo{
		NextBreakType:      domain.SessionTypeShortBreak,
		NextBreakDuration:  5 * time.Minute,
		SessionsUntilLong:  1,
		SessionsBeforeLong: 4,
	}

	model := NewModel(state, info, nil)
	model.width = 80
	model.height = 24
	model.completed = true
	model.completedSessionType = domain.SessionTypeWork

	view := model.View()

	if !strings.Contains(view, "Session complete") {
		t.Error("Work-complete view should contain 'Session complete'")
	}
	if !strings.Contains(view, "05:00") {
		t.Error("Work-complete view should show break duration '05:00'")
	}
	if !strings.Contains(view, "Short Break") {
		t.Error("Work-complete view should show 'Short Break'")
	}
	if !strings.Contains(view, "3 of 4 sessions until long break") {
		t.Error("Work-complete view should show session count info")
	}
	if !strings.Contains(view, "[b]reak") {
		t.Error("Work-complete view should show [b]reak option")
	}
	if !strings.Contains(view, "[s]kip") {
		t.Error("Work-complete view should show [s]kip option")
	}
}

func TestModel_View_WorkComplete_LongBreak(t *testing.T) {
	state := &domain.CurrentState{
		TodayStats: domain.DailyStats{
			WorkSessions:  4,
			BreaksTaken:   3,
			TotalWorkTime: 100 * time.Minute,
		},
	}

	info := &domain.CompletionInfo{
		NextBreakType:      domain.SessionTypeLongBreak,
		NextBreakDuration:  15 * time.Minute,
		SessionsUntilLong:  0,
		SessionsBeforeLong: 4,
	}

	model := NewModel(state, info, nil)
	model.width = 80
	model.height = 24
	model.completed = true
	model.completedSessionType = domain.SessionTypeWork

	view := model.View()

	if !strings.Contains(view, "you earned it") {
		t.Error("Long break view should contain 'you earned it'")
	}
	if !strings.Contains(view, "15:00") {
		t.Error("Long break view should show '15:00' duration")
	}
}

func TestModel_View_BreakComplete(t *testing.T) {
	state := &domain.CurrentState{
		TodayStats: domain.DailyStats{
			WorkSessions:  3,
			BreaksTaken:   3,
			TotalWorkTime: 75 * time.Minute,
		},
	}

	model := NewModel(state, nil, nil)
	model.width = 80
	model.height = 24
	model.completed = true
	model.completedSessionType = domain.SessionTypeShortBreak

	view := model.View()

	if !strings.Contains(view, "Break over") {
		t.Error("Break-complete view should contain 'Break over'")
	}
	if !strings.Contains(view, "[s]tart") {
		t.Error("Break-complete view should show [s]tart option")
	}
}

package tui

import (
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
	model := NewModel(state)

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
			WorkSessions: 2,
			BreaksTaken:  1,
			TotalWorkTime: 50 * time.Minute,
		},
	}

	model := NewModel(state)
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

	model := NewModel(state)
	model.width = 80
	model.height = 24

	view := model.View()

	if view == "" {
		t.Error("View() should not return empty string")
	}
}

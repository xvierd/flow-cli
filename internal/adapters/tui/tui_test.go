package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/methodology"
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
	if !strings.Contains(view, "[n]ew session") {
		t.Error("Work-complete view should show [n]ew session option")
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
	if !strings.Contains(view, "[n]ew session") {
		t.Error("Break-complete view should show [n]ew session option")
	}
}

// --- Session chaining tests ---

func TestInlineModel_CompletionPromptsComplete_NilMode(t *testing.T) {
	m := InlineModel{mode: nil}
	if !m.completionPromptsComplete() {
		t.Error("completionPromptsComplete() should return true when mode is nil")
	}
}

func TestInlineModel_CompletionPromptsComplete_Pomodoro(t *testing.T) {
	m := InlineModel{
		mode:          methodology.ForMethodology(domain.MethodologyPomodoro, nil),
		completedType: domain.SessionTypeWork,
	}
	if !m.completionPromptsComplete() {
		t.Error("completionPromptsComplete() should return true for Pomodoro (no extra prompts)")
	}
}

func TestInlineModel_CompletionPromptsComplete_DeepWork(t *testing.T) {
	mode := methodology.ForMethodology(domain.MethodologyDeepWork, nil)

	// Not complete: accomplishment not saved
	m := InlineModel{
		mode:          mode,
		completedType: domain.SessionTypeWork,
	}
	if m.completionPromptsComplete() {
		t.Error("Deep Work should not be complete without accomplishment saved")
	}

	// Not complete: accomplishment saved but distractions not reviewed
	m.accomplishmentSaved = true
	m.distractions = []string{"email"}
	m.distractionReviewDone = false
	if m.completionPromptsComplete() {
		t.Error("Deep Work should not be complete with unreviewed distractions")
	}

	// Complete: accomplishment saved, distractions reviewed
	m.distractionReviewDone = true
	if !m.completionPromptsComplete() {
		t.Error("Deep Work should be complete after accomplishment + distraction review")
	}

	// Complete: accomplishment saved, no distractions
	m2 := InlineModel{
		mode:          mode,
		completedType: domain.SessionTypeWork,
		completionState: completionState{
			accomplishmentSaved: true,
		},
	}
	if !m2.completionPromptsComplete() {
		t.Error("Deep Work should be complete with accomplishment saved and no distractions")
	}
}

func TestInlineModel_CompletionPromptsComplete_MakeTime(t *testing.T) {
	mode := methodology.ForMethodology(domain.MethodologyMakeTime, nil)

	// Not complete: neither saved
	m := InlineModel{
		mode:          mode,
		completedType: domain.SessionTypeWork,
	}
	if m.completionPromptsComplete() {
		t.Error("Make Time should not be complete without focus score and energize")
	}

	// Not complete: only focus score
	m.focusScoreSaved = true
	if m.completionPromptsComplete() {
		t.Error("Make Time should not be complete without energize activity")
	}

	// Complete: both saved
	m.energizeSaved = true
	if !m.completionPromptsComplete() {
		t.Error("Make Time should be complete after focus score + energize")
	}
}

func TestInlineModel_CompletionPromptsComplete_BreakSession(t *testing.T) {
	// Break sessions should always be "complete" regardless of mode
	for _, meth := range []domain.Methodology{domain.MethodologyPomodoro, domain.MethodologyDeepWork, domain.MethodologyMakeTime} {
		m := InlineModel{
			mode:          methodology.ForMethodology(meth, nil),
			completedType: domain.SessionTypeShortBreak,
		}
		if !m.completionPromptsComplete() {
			t.Errorf("completionPromptsComplete() should return true for break session in %v mode", meth)
		}
	}
}

func TestModel_CompletionPromptsComplete_NilMode(t *testing.T) {
	m := Model{mode: nil}
	if !m.completionPromptsComplete() {
		t.Error("Model.completionPromptsComplete() should return true when mode is nil")
	}
}

func TestModel_CompletionPromptsComplete_Pomodoro(t *testing.T) {
	m := Model{
		mode:                 methodology.ForMethodology(domain.MethodologyPomodoro, nil),
		completedSessionType: domain.SessionTypeWork,
	}
	if !m.completionPromptsComplete() {
		t.Error("Model.completionPromptsComplete() should return true for Pomodoro")
	}
}

func TestModel_CompletionPromptsComplete_DeepWork(t *testing.T) {
	mode := methodology.ForMethodology(domain.MethodologyDeepWork, nil)

	m := Model{
		mode:                 mode,
		completedSessionType: domain.SessionTypeWork,
	}
	if m.completionPromptsComplete() {
		t.Error("Model Deep Work should not be complete without accomplishment")
	}

	m.accomplishmentSaved = true
	m.distractions = []string{"slack"}
	m.distractionReviewDone = false
	if m.completionPromptsComplete() {
		t.Error("Model Deep Work should not be complete with unreviewed distractions")
	}

	m.distractionReviewDone = true
	if !m.completionPromptsComplete() {
		t.Error("Model Deep Work should be complete after ritual done")
	}
}

func TestModel_CompletionPromptsComplete_MakeTime(t *testing.T) {
	mode := methodology.ForMethodology(domain.MethodologyMakeTime, nil)

	m := Model{
		mode:                 mode,
		completedSessionType: domain.SessionTypeWork,
	}
	if m.completionPromptsComplete() {
		t.Error("Model Make Time should not be complete without focus score and energize")
	}

	m.focusScoreSaved = true
	m.energizeSaved = true
	if !m.completionPromptsComplete() {
		t.Error("Model Make Time should be complete after focus score + energize")
	}
}

func TestModel_WantsNewSession_Default(t *testing.T) {
	state := &domain.CurrentState{}
	model := NewModel(state, nil, nil)
	if model.WantsNewSession {
		t.Error("WantsNewSession should be false by default")
	}
}

func TestModel_View_WorkComplete_ShowsNewSession(t *testing.T) {
	state := &domain.CurrentState{
		TodayStats: domain.DailyStats{WorkSessions: 1},
	}
	model := NewModel(state, nil, nil)
	model.width = 80
	model.height = 24
	model.completed = true
	model.completedSessionType = domain.SessionTypeWork

	view := model.View()
	if !strings.Contains(view, "[n]ew session") {
		t.Error("Work-complete view should show [n]ew session option for session chaining")
	}
}

func TestModel_EmptyAccomplishment_UnblocksNewSession(t *testing.T) {
	mode := methodology.ForMethodology(domain.MethodologyDeepWork, nil)
	m := Model{
		state: &domain.CurrentState{
			TodayStats: domain.DailyStats{},
		},
		mode:                 mode,
		completed:            true,
		completedSessionType: domain.SessionTypeWork,
		completionState: completionState{
			accomplishmentMode: true,
		},
		width:  80,
		height: 24,
	}
	// Send Enter key with empty input
	result, _ := m.updateAccomplishmentInput(tea.KeyMsg{Type: tea.KeyEnter})
	updated := result.(Model)
	if !updated.accomplishmentSaved {
		t.Error("Empty Enter should mark accomplishmentSaved = true")
	}
	if updated.accomplishmentMode {
		t.Error("Empty Enter should exit accomplishmentMode")
	}
}

func TestInlineModel_EmptyAccomplishment_UnblocksNewSession(t *testing.T) {
	mode := methodology.ForMethodology(domain.MethodologyDeepWork, nil)
	m := InlineModel{
		state: &domain.CurrentState{
			TodayStats: domain.DailyStats{},
		},
		mode:          mode,
		completed:     true,
		completedType: domain.SessionTypeWork,
		completionState: completionState{
			accomplishmentMode: true,
		},
		width: 80,
	}
	result, _ := m.updateAccomplishmentInput(tea.KeyMsg{Type: tea.KeyEnter})
	updated := result.(InlineModel)
	if !updated.accomplishmentSaved {
		t.Error("Empty Enter should mark accomplishmentSaved = true")
	}
	if updated.accomplishmentMode {
		t.Error("Empty Enter should exit accomplishmentMode")
	}
}

func TestModel_View_ShowsIntendedOutcome(t *testing.T) {
	config := domain.DefaultPomodoroConfig()
	session := domain.NewPomodoroSession(config, nil)
	session.IntendedOutcome = "Build the API"

	state := &domain.CurrentState{
		ActiveSession: session,
		TodayStats:    domain.DailyStats{},
	}
	model := NewModel(state, nil, nil)
	model.width = 80
	model.height = 24

	view := model.View()
	if !strings.Contains(view, "Goal: Build the API") {
		t.Error("View() should show 'Goal: Build the API' when IntendedOutcome is set")
	}
}

func TestModel_View_HidesEmptyOutcome(t *testing.T) {
	config := domain.DefaultPomodoroConfig()
	session := domain.NewPomodoroSession(config, nil)
	session.IntendedOutcome = ""

	state := &domain.CurrentState{
		ActiveSession: session,
		TodayStats:    domain.DailyStats{},
	}
	model := NewModel(state, nil, nil)
	model.width = 80
	model.height = 24

	view := model.View()
	if strings.Contains(view, "Goal:") {
		t.Error("View() should not show 'Goal:' when IntendedOutcome is empty")
	}
}

func TestModel_View_DynamicTitle(t *testing.T) {
	state := &domain.CurrentState{
		TodayStats: domain.DailyStats{},
	}

	// Pomodoro mode
	model := NewModel(state, nil, nil)
	model.width = 80
	model.height = 24
	model.mode = methodology.ForMethodology(domain.MethodologyPomodoro, nil)
	view := model.View()
	if !strings.Contains(view, "Pomodoro") {
		t.Error("Pomodoro mode should show 'Pomodoro' in title")
	}

	// Nil mode fallback
	model2 := NewModel(state, nil, nil)
	model2.width = 80
	model2.height = 24
	model2.mode = nil
	view2 := model2.View()
	if !strings.Contains(view2, "Flow") {
		t.Error("Nil mode should show 'Flow' in title")
	}
}

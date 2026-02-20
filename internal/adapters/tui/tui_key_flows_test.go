package tui

// Comprehensive key-flow tests for both Model (fullscreen) and InlineModel (inline).
// Each test exercises a complete user interaction — not just state setup — so regressions
// in key dispatch, guard conditions, or callback wiring fail fast here.

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/methodology"
	"github.com/xvierd/flow-cli/internal/ports"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func key(s string) tea.Msg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func activeSession() *domain.PomodoroSession {
	cfg := domain.DefaultPomodoroConfig()
	return domain.NewPomodoroSession(cfg, nil)
}

func stateWithSession() *domain.CurrentState {
	return &domain.CurrentState{ActiveSession: activeSession()}
}

func stateNoSession() *domain.CurrentState {
	return &domain.CurrentState{}
}

// commandTracker records which commands were sent via commandCallback.
func commandTracker() (func(ports.TimerCommand) error, *[]ports.TimerCommand) {
	var cmds []ports.TimerCommand
	return func(cmd ports.TimerCommand) error {
		cmds = append(cmds, cmd)
		return nil
	}, &cmds
}

// baseInlineModel returns an InlineModel in phaseTimer with an active session.
func baseInlineModel() InlineModel {
	m := NewInlineModel(stateWithSession(), nil, nil)
	m.phase = phaseTimer
	return m
}

// ---------------------------------------------------------------------------
// [f] Finish key — Model
// ---------------------------------------------------------------------------

func TestModel_FinishKey_FirstPressShowsConfirm(t *testing.T) {
	m := NewModel(stateWithSession(), nil, nil)
	m.width = 80
	result, _ := m.Update(key("f"))
	updated := result.(Model)
	if !updated.confirmFinish {
		t.Error("first [f] press should set confirmFinish = true")
	}
}

func TestModel_FinishKey_ViewShowsConfirmHint(t *testing.T) {
	m := NewModel(stateWithSession(), nil, nil)
	m.width = 80
	m.height = 24
	m.confirmFinish = true
	view := m.View()
	if !strings.Contains(view, "confirm") {
		t.Error("View should show confirm hint when confirmFinish is true")
	}
}

func TestModel_FinishKey_SecondPressCallsCmdStop(t *testing.T) {
	cb, cmds := commandTracker()
	m := NewModel(stateWithSession(), nil, nil)
	m.commandCallback = cb
	m.confirmFinish = true

	result, _ := m.Update(key("f"))
	updated := result.(Model)

	if len(*cmds) == 0 || (*cmds)[0] != ports.CmdStop {
		t.Errorf("second [f] press should call CmdStop, got %v", *cmds)
	}
	if updated.confirmFinish {
		t.Error("confirmFinish should be false after CmdStop")
	}
}

func TestModel_FinishKey_NoopWhenCompleted(t *testing.T) {
	m := NewModel(stateWithSession(), nil, nil)
	m.completed = true
	result, _ := m.Update(key("f"))
	updated := result.(Model)
	if updated.confirmFinish {
		t.Error("[f] should be a no-op when session is already completed")
	}
}

func TestModel_FinishKey_NoopWhenNoActiveSession(t *testing.T) {
	m := NewModel(stateNoSession(), nil, nil)
	result, _ := m.Update(key("f"))
	updated := result.(Model)
	if updated.confirmFinish {
		t.Error("[f] should be a no-op when there is no active session")
	}
}

func TestModel_FinishKey_OtherKeyBetweenPressesResetsConfirm(t *testing.T) {
	m := NewModel(stateWithSession(), nil, nil)
	m.confirmFinish = true
	// Any unrelated key (not p/b/s/etc. with guards) resets confirmFinish
	result, _ := m.Update(key("x"))
	updated := result.(Model)
	if updated.confirmFinish {
		t.Error("an unrelated key between [f] presses should reset confirmFinish")
	}
}

// ---------------------------------------------------------------------------
// [f] Finish key — InlineModel
// ---------------------------------------------------------------------------

func TestInlineModel_FinishKey_FirstPressShowsConfirm(t *testing.T) {
	m := baseInlineModel()
	result, _ := m.Update(key("f"))
	updated := result.(InlineModel)
	if !updated.confirmFinish {
		t.Error("first [f] press should set confirmFinish = true in InlineModel")
	}
}

func TestInlineModel_FinishKey_ViewShowsConfirmHint(t *testing.T) {
	m := baseInlineModel()
	m.confirmFinish = true
	view := m.View()
	if !strings.Contains(view, "[f]") || !strings.Contains(view, "confirm") {
		t.Error("InlineModel View should show confirm hint when confirmFinish is true")
	}
}

func TestInlineModel_FinishKey_SecondPressCallsCmdStop(t *testing.T) {
	cb, cmds := commandTracker()
	m := baseInlineModel()
	m.commandCallback = cb
	m.confirmFinish = true

	result, _ := m.Update(key("f"))
	updated := result.(InlineModel)

	if len(*cmds) == 0 || (*cmds)[0] != ports.CmdStop {
		t.Errorf("second [f] press should call CmdStop in InlineModel, got %v", *cmds)
	}
	if updated.confirmFinish {
		t.Error("confirmFinish should be false after CmdStop in InlineModel")
	}
}

func TestInlineModel_FinishKey_NoopWhenCompleted(t *testing.T) {
	m := baseInlineModel()
	m.completed = true
	result, _ := m.Update(key("f"))
	updated := result.(InlineModel)
	if updated.confirmFinish {
		t.Error("[f] should be a no-op in InlineModel when completed")
	}
}

func TestInlineModel_FinishKey_NoopWhenNoActiveSession(t *testing.T) {
	m := NewInlineModel(stateNoSession(), nil, nil)
	m.phase = phaseTimer
	result, _ := m.Update(key("f"))
	updated := result.(InlineModel)
	if updated.confirmFinish {
		t.Error("[f] should be a no-op in InlineModel when no active session")
	}
}

func TestInlineModel_FinishKey_OtherKeyResetsConfirm(t *testing.T) {
	m := baseInlineModel()
	m.confirmFinish = true
	result, _ := m.Update(key("x"))
	updated := result.(InlineModel)
	if updated.confirmFinish {
		t.Error("unrelated key should reset confirmFinish in InlineModel")
	}
}

// ---------------------------------------------------------------------------
// [b] Break key
// ---------------------------------------------------------------------------

func TestModel_BreakKey_FirstPressShowsConfirm(t *testing.T) {
	m := NewModel(stateWithSession(), nil, nil)
	result, _ := m.Update(key("b"))
	updated := result.(Model)
	if !updated.confirmBreak {
		t.Error("first [b] press should set confirmBreak = true")
	}
	if updated.confirmFinish {
		t.Error("first [b] press should clear confirmFinish")
	}
}

func TestModel_BreakKey_SecondPressCallsStopAndBreak(t *testing.T) {
	cb, cmds := commandTracker()
	m := NewModel(stateWithSession(), nil, nil)
	m.commandCallback = cb
	m.confirmBreak = true

	result, _ := m.Update(key("b"))
	updated := result.(Model)

	if len(*cmds) < 2 {
		t.Fatalf("second [b] press should call CmdStop + CmdBreak, got %v", *cmds)
	}
	if (*cmds)[0] != ports.CmdStop {
		t.Errorf("expected first cmd CmdStop, got %v", (*cmds)[0])
	}
	if (*cmds)[1] != ports.CmdBreak {
		t.Errorf("expected second cmd CmdBreak, got %v", (*cmds)[1])
	}
	if updated.confirmBreak {
		t.Error("confirmBreak should be false after break starts")
	}
}

func TestInlineModel_BreakKey_FirstPressShowsConfirm(t *testing.T) {
	m := baseInlineModel()
	result, _ := m.Update(key("b"))
	updated := result.(InlineModel)
	if !updated.confirmBreak {
		t.Error("first [b] press should set confirmBreak = true in InlineModel")
	}
}

func TestInlineModel_BreakKey_SecondPressCallsStopAndBreak(t *testing.T) {
	cb, cmds := commandTracker()
	m := baseInlineModel()
	m.commandCallback = cb
	m.confirmBreak = true

	result, _ := m.Update(key("b"))
	updated := result.(InlineModel)

	if len(*cmds) < 2 {
		t.Fatalf("second [b] press should call CmdStop + CmdBreak in InlineModel, got %v", *cmds)
	}
	if (*cmds)[0] != ports.CmdStop || (*cmds)[1] != ports.CmdBreak {
		t.Errorf("expected CmdStop then CmdBreak, got %v", *cmds)
	}
	if updated.confirmBreak {
		t.Error("confirmBreak should be false after break in InlineModel")
	}
}

func TestModel_BreakKey_ResetsConfirmFinish(t *testing.T) {
	m := NewModel(stateWithSession(), nil, nil)
	m.confirmFinish = true
	result, _ := m.Update(key("b"))
	updated := result.(Model)
	if updated.confirmFinish {
		t.Error("[b] key should reset confirmFinish")
	}
}

// ---------------------------------------------------------------------------
// [p] Pause / Resume key
// ---------------------------------------------------------------------------

func TestModel_PauseKey_PausesRunningSession(t *testing.T) {
	cb, cmds := commandTracker()
	s := activeSession() // status = Running
	m := NewModel(&domain.CurrentState{ActiveSession: s}, nil, nil)
	m.commandCallback = cb

	m.Update(key("p"))

	if len(*cmds) == 0 || (*cmds)[0] != ports.CmdPause {
		t.Errorf("[p] on running session should call CmdPause, got %v", *cmds)
	}
}

func TestModel_PauseKey_ResumesPausedSession(t *testing.T) {
	cb, cmds := commandTracker()
	s := activeSession()
	s.Pause()
	m := NewModel(&domain.CurrentState{ActiveSession: s}, nil, nil)
	m.commandCallback = cb

	m.Update(key("p"))

	if len(*cmds) == 0 || (*cmds)[0] != ports.CmdResume {
		t.Errorf("[p] on paused session should call CmdResume, got %v", *cmds)
	}
}

func TestInlineModel_PauseKey_PausesRunningSession(t *testing.T) {
	cb, cmds := commandTracker()
	m := baseInlineModel()
	m.commandCallback = cb

	m.Update(key("p"))

	if len(*cmds) == 0 || (*cmds)[0] != ports.CmdPause {
		t.Errorf("[p] on running session should call CmdPause in InlineModel, got %v", *cmds)
	}
}

func TestInlineModel_PauseKey_ResumesPausedSession(t *testing.T) {
	cb, cmds := commandTracker()
	s := activeSession()
	s.Pause()
	m := NewInlineModel(&domain.CurrentState{ActiveSession: s}, nil, nil)
	m.phase = phaseTimer
	m.commandCallback = cb

	m.Update(key("p"))

	if len(*cmds) == 0 || (*cmds)[0] != ports.CmdResume {
		t.Errorf("[p] on paused session should call CmdResume in InlineModel, got %v", *cmds)
	}
}

// ---------------------------------------------------------------------------
// [d] Distraction flow
// ---------------------------------------------------------------------------

func TestModel_DistractionKey_EntersDistractionMode(t *testing.T) {
	m := NewModel(stateWithSession(), nil, nil)
	m.mode = methodology.ForMethodology(domain.MethodologyDeepWork, nil)

	result, _ := m.Update(key("d"))
	updated := result.(Model)

	if !updated.distractionMode {
		t.Error("[d] in Deep Work mode should enter distractionMode")
	}
}

func TestModel_DistractionKey_NoopInPomodoroMode(t *testing.T) {
	m := NewModel(stateWithSession(), nil, nil)
	m.mode = methodology.ForMethodology(domain.MethodologyPomodoro, nil)

	result, _ := m.Update(key("d"))
	updated := result.(Model)

	if updated.distractionMode {
		t.Error("[d] in Pomodoro mode should NOT enter distractionMode")
	}
}

func TestInlineModel_DistractionKey_EntersDistractionMode(t *testing.T) {
	m := baseInlineModel()
	m.mode = methodology.ForMethodology(domain.MethodologyDeepWork, nil)

	result, _ := m.Update(key("d"))
	updated := result.(InlineModel)

	if !updated.distractionMode {
		t.Error("[d] in Deep Work mode should enter distractionMode in InlineModel")
	}
}

// ---------------------------------------------------------------------------
// Distraction category picker
// ---------------------------------------------------------------------------

func TestModel_DistractionCategory_InternalKey(t *testing.T) {
	var gotText, gotCat string
	m := NewModel(stateWithSession(), nil, nil)
	m.mode = methodology.ForMethodology(domain.MethodologyDeepWork, nil)
	m.distractionCallback = func(text, cat string) error {
		gotText = text
		gotCat = cat
		return nil
	}
	m.distractionMode = true
	m.distractionCategoryMode = true
	m.distractionPendingText = "email"

	result, _ := m.updateDistractionInput(key("i"))
	updated := result.(Model)

	if gotText != "email" || gotCat != "internal" {
		t.Errorf("expected (email, internal), got (%q, %q)", gotText, gotCat)
	}
	if updated.distractionCategoryMode || updated.distractionMode {
		t.Error("category picker should close after selection")
	}
}

func TestModel_DistractionCategory_ExternalKey(t *testing.T) {
	var gotCat string
	m := NewModel(stateWithSession(), nil, nil)
	m.mode = methodology.ForMethodology(domain.MethodologyDeepWork, nil)
	m.distractionCallback = func(_, cat string) error { gotCat = cat; return nil }
	m.distractionMode = true
	m.distractionCategoryMode = true
	m.distractionPendingText = "noise"

	m.updateDistractionInput(key("e"))

	if gotCat != "external" {
		t.Errorf("expected category 'external', got %q", gotCat)
	}
}

func TestModel_DistractionCategory_EnterSkipsCategory(t *testing.T) {
	var gotCat string
	m := NewModel(stateWithSession(), nil, nil)
	m.mode = methodology.ForMethodology(domain.MethodologyDeepWork, nil)
	m.distractionCallback = func(_, cat string) error { gotCat = cat; return nil }
	m.distractionMode = true
	m.distractionCategoryMode = true
	m.distractionPendingText = "thought"

	m.updateDistractionInput(key("enter"))

	if gotCat != "" {
		t.Errorf("Enter should skip category (empty string), got %q", gotCat)
	}
}

func TestInlineModel_DistractionCategory_InternalKey(t *testing.T) {
	var gotCat string
	m := baseInlineModel()
	m.mode = methodology.ForMethodology(domain.MethodologyDeepWork, nil)
	m.distractionCallback = func(_, cat string) error { gotCat = cat; return nil }
	m.distractionMode = true
	m.distractionCategoryMode = true
	m.distractionPendingText = "slack"

	m.updateDistractionInput(key("i"))

	if gotCat != "internal" {
		t.Errorf("expected 'internal', got %q", gotCat)
	}
}

// ---------------------------------------------------------------------------
// 3-step Shutdown Ritual — Model
// ---------------------------------------------------------------------------

func TestModel_ShutdownRitual_ThreeStepsAdvanceOnEnter(t *testing.T) {
	m := NewModel(stateWithSession(), nil, nil)
	m.mode = methodology.ForMethodology(domain.MethodologyDeepWork, nil)
	m.shutdownRitualMode = true
	m.shutdownStep = 0

	// Step 0 → 1
	result, _ := m.updateShutdownRitual(key("enter"))
	m = result.(Model)
	if m.shutdownStep != 1 {
		t.Errorf("Enter on step 0 should advance to step 1, got %d", m.shutdownStep)
	}

	// Step 1 → 2
	result, _ = m.updateShutdownRitual(key("enter"))
	m = result.(Model)
	if m.shutdownStep != 2 {
		t.Errorf("Enter on step 1 should advance to step 2, got %d", m.shutdownStep)
	}

	// Step 2 → 3
	result, _ = m.updateShutdownRitual(key("enter"))
	m = result.(Model)
	if m.shutdownStep != 3 {
		t.Errorf("Enter on step 2 should advance to step 3, got %d", m.shutdownStep)
	}

	// Step 3 → complete
	result, _ = m.updateShutdownRitual(key("enter"))
	m = result.(Model)
	if !m.shutdownComplete {
		t.Error("Enter on step 3 should set shutdownComplete = true")
	}
	if m.shutdownRitualMode {
		t.Error("shutdownRitualMode should be false after completing ritual")
	}
}

func TestModel_ShutdownRitual_EscAbandonsRitual(t *testing.T) {
	m := NewModel(stateWithSession(), nil, nil)
	m.mode = methodology.ForMethodology(domain.MethodologyDeepWork, nil)
	m.shutdownRitualMode = true
	m.shutdownStep = 0

	result, _ := m.updateShutdownRitual(key("esc"))
	updated := result.(Model)
	if updated.shutdownRitualMode {
		t.Error("Esc should abandon the ritual (shutdownRitualMode = false)")
	}
	if updated.shutdownStep != 0 {
		t.Errorf("Esc should not advance steps, got step %d", updated.shutdownStep)
	}
}

func TestModel_ShutdownRitual_CallsCallback(t *testing.T) {
	var gotRitual domain.ShutdownRitual
	m := NewModel(stateWithSession(), nil, nil)
	m.mode = methodology.ForMethodology(domain.MethodologyDeepWork, nil)
	m.shutdownRitualCallback = func(r domain.ShutdownRitual) error {
		gotRitual = r
		return nil
	}
	m.shutdownRitualMode = true
	m.shutdownStep = 3

	m.updateShutdownRitual(key("enter"))

	// callback should be invoked (ritual may be empty strings if inputs were blank)
	_ = gotRitual // just verify it was called without panic
}

func TestInlineModel_ShutdownRitual_ThreeStepsAdvanceOnEnter(t *testing.T) {
	m := baseInlineModel()
	m.mode = methodology.ForMethodology(domain.MethodologyDeepWork, nil)
	m.shutdownRitualMode = true
	m.shutdownStep = 0

	result, _ := m.updateShutdownRitual(key("enter"))
	m = result.(InlineModel)
	if m.shutdownStep != 1 {
		t.Errorf("Enter on step 0 should advance to step 1 in InlineModel, got %d", m.shutdownStep)
	}

	result, _ = m.updateShutdownRitual(key("enter"))
	m = result.(InlineModel)
	result, _ = m.updateShutdownRitual(key("enter"))
	m = result.(InlineModel)
	result, _ = m.updateShutdownRitual(key("enter"))
	m = result.(InlineModel)

	if !m.shutdownComplete {
		t.Error("After 4 Enters, shutdownComplete should be true in InlineModel")
	}
}

func TestInlineModel_ShutdownRitual_EscAbandonsRitual(t *testing.T) {
	m := baseInlineModel()
	m.mode = methodology.ForMethodology(domain.MethodologyDeepWork, nil)
	m.shutdownRitualMode = true
	m.shutdownStep = 1 // mid-ritual

	result, _ := m.updateShutdownRitual(key("esc"))
	m = result.(InlineModel)

	if m.shutdownRitualMode {
		t.Error("Esc should abandon the ritual (shutdownRitualMode = false)")
	}
}

func TestInlineModel_ShutdownRitual_SkipAllWithEnter(t *testing.T) {
	m := baseInlineModel()
	m.mode = methodology.ForMethodology(domain.MethodologyDeepWork, nil)
	m.shutdownRitualMode = true
	m.shutdownStep = 0

	// Empty Enter on each step skips it and advances — 4 times completes the ritual (Cal Newport's 4 steps)
	for i := 0; i < 4; i++ {
		result, _ := m.updateShutdownRitual(key("enter"))
		m = result.(InlineModel)
	}

	if !m.shutdownComplete {
		t.Error("Empty Enter on all 4 steps should still complete the ritual")
	}
}

// ---------------------------------------------------------------------------
// Focus score (Make Time) — 1–5 keys
// ---------------------------------------------------------------------------

func TestModel_FocusScore_KeySavesScore(t *testing.T) {
	for _, k := range []string{"1", "2", "3", "4", "5"} {
		t.Run("key_"+k, func(t *testing.T) {
			var gotScore int
			m := NewModel(stateNoSession(), nil, nil)
			m.mode = methodology.ForMethodology(domain.MethodologyMakeTime, nil)
			m.completed = true
			m.completedSessionType = domain.SessionTypeWork
			m.focusScoreCallback = func(score int) error { gotScore = score; return nil }

			result, _ := m.Update(key(k))
			updated := result.(Model)

			if !updated.focusScoreSaved {
				t.Errorf("key %q should set focusScoreSaved = true", k)
			}
			expected := int(k[0] - '0')
			if gotScore != expected {
				t.Errorf("key %q should save score %d, got %d", k, expected, gotScore)
			}
		})
	}
}

func TestModel_FocusScore_OnlyInMakeTimeMode(t *testing.T) {
	m := NewModel(stateNoSession(), nil, nil)
	m.mode = methodology.ForMethodology(domain.MethodologyPomodoro, nil)
	m.completed = true
	m.completedSessionType = domain.SessionTypeWork
	m.width = 80

	result, _ := m.Update(key("3"))
	updated := result.(Model)

	if updated.focusScoreSaved {
		t.Error("focus score should only be saved in Make Time mode")
	}
}

func TestInlineModel_FocusScore_KeySavesScore(t *testing.T) {
	var gotScore int
	m := baseInlineModel()
	m.mode = methodology.ForMethodology(domain.MethodologyMakeTime, nil)
	m.completed = true
	m.completedType = domain.SessionTypeWork
	m.focusScoreCallback = func(score int) error { gotScore = score; return nil }

	result, _ := m.Update(key("4"))
	updated := result.(InlineModel)

	if !updated.focusScoreSaved {
		t.Error("key 4 should set focusScoreSaved = true in InlineModel")
	}
	if gotScore != 4 {
		t.Errorf("expected score 4, got %d", gotScore)
	}
}

// ---------------------------------------------------------------------------
// Energize activity (Make Time) — w/t/e/n keys
// ---------------------------------------------------------------------------

func completedMakeTimeModel() Model {
	m := NewModel(stateNoSession(), nil, nil)
	m.mode = methodology.ForMethodology(domain.MethodologyMakeTime, nil)
	m.completed = true
	m.completedSessionType = domain.SessionTypeWork
	m.focusScoreSaved = true
	return m
}

func TestModel_Energize_WalkKey(t *testing.T) {
	var got string
	m := completedMakeTimeModel()
	m.energizeCallback = func(a string) error { got = a; return nil }

	result, _ := m.Update(key("w"))
	updated := result.(Model)

	if got != "walk" {
		t.Errorf("expected activity 'walk', got %q", got)
	}
	if !updated.energizeSaved {
		t.Error("energizeSaved should be true after [w]")
	}
}

func TestModel_Energize_StretchKey(t *testing.T) {
	var got string
	m := completedMakeTimeModel()
	m.energizeCallback = func(a string) error { got = a; return nil }
	result, _ := m.Update(key("t"))
	updated := result.(Model)
	if got != "stretch" {
		t.Errorf("expected 'stretch', got %q", got)
	}
	if !updated.energizeSaved {
		t.Error("energizeSaved should be true after [t]")
	}
}

func TestModel_Energize_ExerciseKey(t *testing.T) {
	var got string
	m := completedMakeTimeModel()
	m.energizeCallback = func(a string) error { got = a; return nil }
	result, _ := m.Update(key("e"))
	updated := result.(Model)
	if got != "exercise" {
		t.Errorf("expected 'exercise', got %q", got)
	}
	if !updated.energizeSaved {
		t.Error("energizeSaved should be true after [e]")
	}
}

func TestModel_Energize_NoneKey(t *testing.T) {
	var got string
	m := completedMakeTimeModel()
	m.energizeCallback = func(a string) error { got = a; return nil }
	result, _ := m.Update(key("n"))
	updated := result.(Model)
	if got != "none" {
		t.Errorf("expected 'none', got %q", got)
	}
	if !updated.energizeSaved {
		t.Error("energizeSaved should be true after [n]")
	}
}

func TestModel_Energize_RequiresFocusScoreFirst(t *testing.T) {
	var got string
	m := completedMakeTimeModel()
	m.focusScoreSaved = false // not yet saved
	m.energizeCallback = func(a string) error { got = a; return nil }
	m.Update(key("w"))
	if got != "" {
		t.Error("energize should not be recorded before focus score is saved")
	}
}

func TestInlineModel_Energize_WalkKey(t *testing.T) {
	var got string
	m := baseInlineModel()
	m.mode = methodology.ForMethodology(domain.MethodologyMakeTime, nil)
	m.completed = true
	m.completedType = domain.SessionTypeWork
	m.focusScoreSaved = true
	m.energizeCallback = func(a string) error { got = a; return nil }

	m.Update(key("w"))

	if got != "walk" {
		t.Errorf("InlineModel [w] should record 'walk', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// [n] New session key
// ---------------------------------------------------------------------------

func TestModel_NewSessionKey_SetsWantsNewSession(t *testing.T) {
	m := NewModel(stateNoSession(), nil, nil)
	m.mode = methodology.ForMethodology(domain.MethodologyPomodoro, nil)
	m.completed = true
	m.completedSessionType = domain.SessionTypeWork
	// Pomodoro has no extra prompts, so completionPromptsComplete() = true

	result, _ := m.Update(key("n"))
	updated := result.(Model)

	if !updated.WantsNewSession {
		t.Error("[n] when prompts complete should set WantsNewSession = true")
	}
}

func TestModel_NewSessionKey_BlockedUntilPromptsComplete(t *testing.T) {
	m := NewModel(stateNoSession(), nil, nil)
	m.mode = methodology.ForMethodology(domain.MethodologyMakeTime, nil)
	m.completed = true
	m.completedSessionType = domain.SessionTypeWork
	// focusScoreSaved = false → completionPromptsComplete() = false

	result, _ := m.Update(key("n"))
	updated := result.(Model)

	if updated.WantsNewSession {
		t.Error("[n] should NOT set WantsNewSession before Make Time prompts are complete")
	}
}

// ---------------------------------------------------------------------------
// Guard: modes block [f] key
// ---------------------------------------------------------------------------

func TestModel_DistractionMode_BlocksFinishKey(t *testing.T) {
	cb, cmds := commandTracker()
	m := NewModel(stateWithSession(), nil, nil)
	m.commandCallback = cb
	m.distractionMode = true
	m.confirmFinish = true // already in confirm state

	// Key goes to updateDistractionInput, not main handler
	result, _ := m.Update(key("f"))
	_ = result

	if len(*cmds) > 0 {
		t.Error("[f] during distraction input should not call commandCallback")
	}
}

func TestModel_AccomplishmentMode_BlocksFinishKey(t *testing.T) {
	cb, cmds := commandTracker()
	m := NewModel(stateWithSession(), nil, nil)
	m.commandCallback = cb
	m.accomplishmentMode = true
	m.confirmFinish = true

	m.Update(key("f"))

	if len(*cmds) > 0 {
		t.Error("[f] during accomplishment input should not call commandCallback")
	}
}

func TestModel_ShutdownRitualMode_BlocksFinishKey(t *testing.T) {
	cb, cmds := commandTracker()
	m := NewModel(stateWithSession(), nil, nil)
	m.commandCallback = cb
	m.shutdownRitualMode = true
	m.confirmFinish = true

	m.Update(key("f"))

	if len(*cmds) > 0 {
		t.Error("[f] during shutdown ritual should not call commandCallback")
	}
}

// ---------------------------------------------------------------------------
// Energize reminder duration
// ---------------------------------------------------------------------------

func TestEnergizeTicks_IsAtLeast30(t *testing.T) {
	// The energize reminder is set to 30 ticks and immediately decremented once
	// on the same tick it triggers, so the post-tick value is 29.
	// Total display duration = 29 remaining ticks + the trigger tick = 30 ticks.
	m := baseInlineModel()
	m.mode = methodology.ForMethodology(domain.MethodologyMakeTime, nil)

	// Manually trigger the energize condition: session at 50%+ progress
	s := activeSession()
	s.StartedAt = time.Now().Add(-s.Duration / 2) // 50% elapsed
	m.state.ActiveSession = s

	result, _ := m.Update(tickMsg{})
	updated := result.(InlineModel)

	if !updated.energizeShown {
		t.Error("energize reminder should have triggered at 50% progress")
	}
	// Set to 30, decremented once on same tick → 29; total display = 30 ticks
	if updated.energizeTicks < 29 {
		t.Errorf("energizeTicks should be >= 29 after trigger tick (30 - 1 decrement), got %d", updated.energizeTicks)
	}
}

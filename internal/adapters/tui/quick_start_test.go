package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/methodology"
)

// --- Picker: initial cursor and hotkeys -------------------------------------

func pickerItems() []PickerItem {
	return []PickerItem{{Label: "Focus"}, {Label: "Short"}, {Label: "Deep"}}
}

func pressPicker(m pickerModel, keys ...string) pickerModel {
	for _, k := range keys {
		next, _ := m.Update(key(k))
		m = next.(pickerModel)
	}
	return m
}

func TestPicker_InitialCursorThenEnter(t *testing.T) {
	m := newPickerModel("Duration:", pickerItems(), "", config.DefaultThemeConfig(), PickerOptions{Cursor: 2})
	got := pressPicker(m, "enter").result()
	if got.Aborted || got.Index != 2 || got.Hotkey != "" {
		t.Errorf("Enter on preselected item = %+v, want index 2", got)
	}
}

func TestPicker_OutOfRangeCursorSelectsFirst(t *testing.T) {
	for _, c := range []int{-1, 3, 99} {
		m := newPickerModel("t", pickerItems(), "", config.DefaultThemeConfig(), PickerOptions{Cursor: c})
		if m.cursor != 0 {
			t.Errorf("Cursor %d → %d, want 0", c, m.cursor)
		}
	}
}

func TestPicker_HotkeyClosesWithoutChoosing(t *testing.T) {
	opts := PickerOptions{Hotkeys: []PickerHotkey{{Key: "m", Help: "m change mode"}}}
	m := newPickerModel("t", pickerItems(), "", config.DefaultThemeConfig(), opts)
	if !strings.Contains(m.View(), "m change mode") {
		t.Error("hotkey help should be shown in the key help line")
	}
	next, cmd := m.Update(key("m"))
	got := next.(pickerModel).result()
	if got.Hotkey != "m" || got.Aborted {
		t.Errorf("result = %+v, want Hotkey m", got)
	}
	if cmd == nil {
		t.Error("hotkey should quit the picker")
	}
}

func TestPicker_UnregisteredHotkeyIgnored(t *testing.T) {
	m := newPickerModel("t", pickerItems(), "", config.DefaultThemeConfig(), PickerOptions{})
	m = pressPicker(m, "m", "down", "enter")
	if got := m.result(); got.Hotkey != "" || got.Index != 1 {
		t.Errorf("result = %+v, want index 1 and no hotkey", got)
	}
}

// --- Inline wizard: skip the mode step, preselect the last duration --------

// setupInlineModel returns an idle InlineModel at the main menu in Pomodoro
// with the configured presets (Focus 25, Short 15, Deep 50).
func setupInlineModel(skipMode bool, last time.Duration) InlineModel {
	cfg := config.DefaultConfig()
	m := NewInlineModel(stateNoSession(), nil, nil)
	m.phase = phaseMainMenu
	m.mode = methodology.ForMethodology(domain.MethodologyPomodoro, cfg)
	m.presets = m.mode.Presets()
	m.skipModePicker = skipMode
	m.lastDuration = func(domain.Methodology) time.Duration { return last }
	m.onModeSelected = func(d domain.Methodology) methodology.Mode {
		return methodology.ForMethodology(d, cfg)
	}
	return m
}

func pressInline(m InlineModel, keys ...string) InlineModel {
	for _, k := range keys {
		next, _ := m.Update(key(k))
		m = next.(InlineModel)
	}
	return m
}

func presetIndex(m InlineModel, d time.Duration) int {
	for i, p := range m.presets {
		if p.Duration == d {
			return i
		}
	}
	return -1
}

func TestInlineWizard_DefaultModeSkipsModeStep(t *testing.T) {
	m := setupInlineModel(true, 0)
	m = pressInline(m, "enter") // Start session
	if m.phase != phasePickDuration {
		t.Fatalf("Start session with a default mode → phase %d, want the duration picker", m.phase)
	}
	if m.presetCursor != 0 {
		t.Errorf("no previous session: cursor %d, want the first preset", m.presetCursor)
	}
}

func TestInlineWizard_NoDefaultModeKeepsModeStep(t *testing.T) {
	m := setupInlineModel(false, 0)
	m = pressInline(m, "enter")
	if m.phase != phasePickMode {
		t.Errorf("without a default mode → phase %d, want the mode picker", m.phase)
	}
}

func TestInlineWizard_PreselectsLastDuration(t *testing.T) {
	m := setupInlineModel(true, 50*time.Minute)
	want := presetIndex(m, 50*time.Minute)
	if want <= 0 {
		t.Fatalf("test needs a non-first 50m preset, presets = %v", m.presets)
	}
	m = pressInline(m, "enter")
	if m.presetCursor != want {
		t.Errorf("cursor = %d, want %d (the last 50m session)", m.presetCursor, want)
	}

	// Enter, Enter: the preselected duration goes straight on to the task.
	m = pressInline(m, "enter")
	if m.phase != phaseTaskName && m.phase != phaseTaskSelect {
		t.Errorf("second Enter → phase %d, want the task step", m.phase)
	}
	if p := m.presets[m.presetCursor]; p.Duration != 50*time.Minute {
		t.Errorf("session would use %v, want 50m", p.Duration)
	}
}

func TestInlineWizard_UnknownLastDurationSelectsFirst(t *testing.T) {
	m := setupInlineModel(true, 40*time.Minute)
	m = pressInline(m, "enter")
	if m.presetCursor != 0 {
		t.Errorf("cursor = %d, want 0 for a duration matching no preset", m.presetCursor)
	}
}

func TestInlineWizard_ModeKeyOnDurationOpensModePicker(t *testing.T) {
	m := setupInlineModel(true, 0)
	m = pressInline(m, "enter", "m")
	if m.phase != phasePickMode {
		t.Fatalf("[m] on the duration picker → phase %d, want the mode picker", m.phase)
	}
	// Pick Deep Work (2) and dismiss its onboarding note.
	m = pressInline(m, "2", "enter")
	if m.phase != phasePickDuration || m.mode.Name() != domain.MethodologyDeepWork {
		t.Errorf("after picking Deep Work: phase %d mode %v, want the Deep Work duration picker", m.phase, m.mode.Name())
	}
}

func TestInlineWizard_ModeKeyIgnoredWhenLocked(t *testing.T) {
	m := setupInlineModel(true, 0)
	m.modeLocked = true
	m.phase = phasePickDuration
	m = pressInline(m, "m")
	if m.phase != phasePickDuration {
		t.Errorf("[m] with --mode → phase %d, want to stay on the duration picker", m.phase)
	}
}

func TestInlineWizard_EscOnDurationGoesBack(t *testing.T) {
	m := pressInline(setupInlineModel(true, 0), "enter", "esc")
	if m.phase != phaseMainMenu {
		t.Errorf("esc with the mode step skipped → phase %d, want the main menu", m.phase)
	}
	m = setupInlineModel(false, 0)
	m.phase = phasePickDuration
	m = pressInline(m, "esc")
	if m.phase != phasePickMode {
		t.Errorf("esc with a mode step → phase %d, want the mode picker", m.phase)
	}
}

// --- Inline welcome: start now ----------------------------------------------

func TestInlineWelcome_EnterStartsDefaultSessionNow(t *testing.T) {
	m := setupInlineModel(true, 0)
	m.phase = phaseWelcome
	var gotPreset = -1
	var gotTask string
	m.onStartSessionForTask = func(presetIndex int, existingTaskID, taskName, outcome string) error {
		gotPreset, gotTask = presetIndex, taskName
		return nil
	}
	if !strings.Contains(m.View(), "flow go") {
		t.Error("welcome should list useful commands, including flow go")
	}
	m = pressInline(m, "enter")
	if gotPreset != 0 || gotTask != "" {
		t.Errorf("start callback got preset %d task %q, want preset 0 with no task", gotPreset, gotTask)
	}
	if m.phase != phaseTimer {
		t.Errorf("phase = %d, want the timer", m.phase)
	}
}

func TestInlineWelcome_SetUpOpensMenu(t *testing.T) {
	m := setupInlineModel(true, 0)
	m.phase = phaseWelcome
	started := false
	m.onStartSessionForTask = func(int, string, string, string) error { started = true; return nil }
	m = pressInline(m, "s")
	if m.phase != phaseMainMenu || started {
		t.Errorf("[s] → phase %d started %v, want the main menu without starting", m.phase, started)
	}
}

var _ tea.Model = pickerModel{}

package config

import (
	"testing"
	"time"
)

func TestDefaultConfig_DeepWorkPresets(t *testing.T) {
	cfg := DefaultConfig()
	presets := cfg.DeepWork.GetPresets()
	if len(presets) != 3 {
		t.Fatalf("expected 3 presets, got %d", len(presets))
	}
	if presets[0].Name != "Deep" {
		t.Errorf("expected preset1 name 'Deep', got %q", presets[0].Name)
	}
	if presets[0].Duration != 90*time.Minute {
		t.Errorf("expected preset1 duration 90m, got %v", presets[0].Duration)
	}
}

func TestDefaultConfig_MakeTimePresets(t *testing.T) {
	cfg := DefaultConfig()
	presets := cfg.MakeTime.GetPresets()
	if len(presets) != 3 {
		t.Fatalf("expected 3 presets, got %d", len(presets))
	}
	if presets[0].Name != "Highlight" {
		t.Errorf("expected preset1 name 'Highlight', got %q", presets[0].Name)
	}
	if presets[0].Duration != 60*time.Minute {
		t.Errorf("expected preset1 duration 60m, got %v", presets[0].Duration)
	}
}

func TestDeepWorkGoalHours_Default4(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.DeepWork.DeepWorkGoalHours != 4.0 {
		t.Errorf("expected default DeepWorkGoalHours=4.0, got %f", cfg.DeepWork.DeepWorkGoalHours)
	}
}

func TestDefaultConfig_FocusStrictFalse(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Focus.Strict {
		t.Error("expected focus.strict to default to false")
	}
}

func TestConfig_LoadSaveRoundTrip(t *testing.T) {
	// Isolate the config file under a fresh HOME.
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := DefaultConfig()
	cfg.Methodology = "deepwork"
	cfg.Pomodoro.WorkDuration = Duration(45 * time.Minute)
	cfg.Pomodoro.SessionsBeforeLong = 6
	cfg.Focus.Strict = true
	cfg.Notifications.Enabled = false

	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.Methodology != "deepwork" {
		t.Errorf("methodology = %q, want deepwork", loaded.Methodology)
	}
	if time.Duration(loaded.Pomodoro.WorkDuration) != 45*time.Minute {
		t.Errorf("work_duration = %v, want 45m", loaded.Pomodoro.WorkDuration)
	}
	if loaded.Pomodoro.SessionsBeforeLong != 6 {
		t.Errorf("sessions_before_long = %d, want 6", loaded.Pomodoro.SessionsBeforeLong)
	}
	if !loaded.Focus.Strict {
		t.Error("focus.strict should round-trip as true")
	}
	if loaded.Notifications.Enabled {
		t.Error("notifications.enabled should round-trip as false")
	}
}

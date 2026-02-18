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

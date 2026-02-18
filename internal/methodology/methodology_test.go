package methodology

import (
	"testing"

	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
)

func TestPresetsFromConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	mode := ForMethodology(domain.MethodologyDeepWork, cfg)
	presets := mode.Presets()
	if len(presets) != 3 {
		t.Fatalf("expected 3 presets, got %d", len(presets))
	}
}

func TestPresetsNilConfig(t *testing.T) {
	mode := ForMethodology(domain.MethodologyDeepWork, nil)
	presets := mode.Presets()
	if len(presets) != 3 {
		t.Fatalf("expected 3 presets with nil config, got %d", len(presets))
	}
}

func TestDeepWorkGoalHours(t *testing.T) {
	cfg := config.DefaultConfig()
	mode := ForMethodology(domain.MethodologyDeepWork, cfg)
	if mode.DeepWorkGoalHours() != 4.0 {
		t.Errorf("expected 4.0, got %f", mode.DeepWorkGoalHours())
	}

	pomMode := ForMethodology(domain.MethodologyPomodoro, cfg)
	if pomMode.DeepWorkGoalHours() != 0 {
		t.Errorf("expected 0 for pomodoro mode, got %f", pomMode.DeepWorkGoalHours())
	}
}

func TestTUITitle(t *testing.T) {
	cfg := config.DefaultConfig()
	tests := []struct {
		methodology domain.Methodology
		expected    string
	}{
		{domain.MethodologyPomodoro, "Flow - Pomodoro Timer"},
		{domain.MethodologyDeepWork, "Deep Work"},
		{domain.MethodologyMakeTime, "Make Time"},
	}
	for _, tt := range tests {
		mode := ForMethodology(tt.methodology, cfg)
		if mode.TUITitle() != tt.expected {
			t.Errorf("TUITitle() for %s: expected %q, got %q", tt.methodology, tt.expected, mode.TUITitle())
		}
	}
}

func TestDescription(t *testing.T) {
	cfg := config.DefaultConfig()
	tests := []domain.Methodology{
		domain.MethodologyPomodoro,
		domain.MethodologyDeepWork,
		domain.MethodologyMakeTime,
	}
	for _, m := range tests {
		mode := ForMethodology(m, cfg)
		if mode.Description() == "" {
			t.Errorf("Description() for %s should not be empty", m)
		}
	}
}

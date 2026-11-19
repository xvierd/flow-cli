package methodology

import (
	"fmt"
	"reflect"
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

func TestModeMatrix(t *testing.T) {
	tests := []struct {
		methodology       domain.Methodology
		hasOutcomePrompt  bool
		hasDistractionLog bool
		hasEnergize       bool
		hasFocusScore     bool
		hasShutdown       bool
		hasHighlight      bool
		hasLaser          bool
		deepWork          bool
	}{
		{methodology: domain.MethodologyPomodoro},
		{methodology: domain.MethodologyDeepWork, hasOutcomePrompt: true, hasDistractionLog: true, hasShutdown: true, deepWork: true},
		{methodology: domain.MethodologyMakeTime, hasEnergize: true, hasFocusScore: true, hasHighlight: true, hasLaser: true},
	}
	for _, cfg := range []*config.Config{nil, config.DefaultConfig()} {
		for _, tt := range tests {
			mode := ForMethodology(tt.methodology, cfg)
			name := fmt.Sprintf("%s (nil config: %v)", tt.methodology, cfg == nil)

			if mode.Name() != tt.methodology {
				t.Errorf("%s: Name() = %q, want %q", name, mode.Name(), tt.methodology)
			}
			checks := []struct {
				flag string
				got  bool
				want bool
			}{
				{"HasDistractionLog", mode.HasDistractionLog(), tt.hasDistractionLog},
				{"HasEnergizeReminder", mode.HasEnergizeReminder(), tt.hasEnergize},
				{"HasFocusScore", mode.HasFocusScore(), tt.hasFocusScore},
				{"HasShutdownRitual", mode.HasShutdownRitual(), tt.hasShutdown},
				{"HasHighlight", mode.HasHighlight(), tt.hasHighlight},
				{"HasLaserChecklist", mode.HasLaserChecklist(), tt.hasLaser},
			}
			for _, c := range checks {
				if c.got != c.want {
					t.Errorf("%s: %s() = %v, want %v", name, c.flag, c.got, c.want)
				}
			}

			if mode.TaskPrompt() == "" {
				t.Errorf("%s: TaskPrompt() should not be empty", name)
			}
			if mode.CompletionTitle() == "" {
				t.Errorf("%s: CompletionTitle() should not be empty", name)
			}
			if mode.TUITitle() == "" {
				t.Errorf("%s: TUITitle() should not be empty", name)
			}
			if mode.Description() == "" {
				t.Errorf("%s: Description() should not be empty", name)
			}
			if got := mode.OutcomePrompt() != ""; got != tt.hasOutcomePrompt {
				t.Errorf("%s: OutcomePrompt() = %q, want non-empty=%v", name, mode.OutcomePrompt(), tt.hasOutcomePrompt)
			}
			if !tt.deepWork {
				if mode.DeepWorkGoalHours() != 0 {
					t.Errorf("%s: DeepWorkGoalHours() = %v, want 0", name, mode.DeepWorkGoalHours())
				}
				if mode.DeepWorkPhilosophy() != "" {
					t.Errorf("%s: DeepWorkPhilosophy() = %q, want empty", name, mode.DeepWorkPhilosophy())
				}
			} else if mode.DeepWorkGoalHours() <= 0 {
				t.Errorf("%s: DeepWorkGoalHours() = %v, want > 0", name, mode.DeepWorkGoalHours())
			}
		}
	}
}

func TestPresetsNilConfigMatchDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	for _, m := range []domain.Methodology{
		domain.MethodologyPomodoro,
		domain.MethodologyDeepWork,
		domain.MethodologyMakeTime,
	} {
		got := ForMethodology(m, nil).Presets()
		want := ForMethodology(m, cfg).Presets()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: nil-config presets %v differ from DefaultConfig presets %v", m, got, want)
		}
	}
}

func TestForMethodology_UnknownFallsBackToPomodoro(t *testing.T) {
	cfg := config.DefaultConfig()
	for _, m := range []domain.Methodology{"", "unknown"} {
		for _, c := range []*config.Config{nil, cfg} {
			mode := ForMethodology(m, c)
			if mode.Name() != domain.MethodologyPomodoro {
				t.Errorf("ForMethodology(%q, nil config: %v).Name() = %q, want pomodoro", m, c == nil, mode.Name())
			}
		}
	}
	got := ForMethodology("unknown", cfg).Presets()
	want := ForMethodology(domain.MethodologyPomodoro, cfg).Presets()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("unknown methodology presets = %v, want pomodoro presets %v", got, want)
	}
}

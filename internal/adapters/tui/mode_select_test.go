package tui

import (
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/methodology"
)

// TestInlineModel_SelectMode_UsesConfiguredMode guards the inline mode switch:
// the picker must show the configured presets (the ones the session will
// actually start with), not the hard-coded defaults of a nil-config Mode.
func TestInlineModel_SelectMode_UsesConfiguredMode(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DeepWork.Preset1Name = "Marathon"
	cfg.DeepWork.Preset1Duration = config.Duration(3 * time.Hour)

	var selected domain.Methodology
	m := NewInlineModel(stateNoSession(), nil, nil)
	m.phase = phasePickMode
	m.modeCursor = 1 // Deep Work
	m.onModeSelected = func(d domain.Methodology) methodology.Mode {
		selected = d
		return methodology.ForMethodology(d, cfg)
	}

	result, _ := m.selectMode()
	updated := result.(InlineModel)

	if selected != domain.MethodologyDeepWork {
		t.Fatalf("callback got %q, want deepwork", selected)
	}
	if updated.mode == nil || updated.mode.Name() != domain.MethodologyDeepWork {
		t.Fatalf("mode = %v, want deepwork", updated.mode)
	}
	if len(updated.presets) == 0 {
		t.Fatal("no presets after mode selection")
	}
	if p := updated.presets[0]; p.Name != "Marathon" || p.Duration != 3*time.Hour {
		t.Errorf("first preset = %s %v, want the configured Marathon 3h", p.Name, p.Duration)
	}
}

func TestInlineModel_SelectMode_NilCallbackFallsBackToDefaults(t *testing.T) {
	m := NewInlineModel(stateNoSession(), nil, nil)
	m.phase = phasePickMode
	m.modeCursor = 2 // Make Time
	m.onModeSelected = nil

	result, _ := m.selectMode()
	updated := result.(InlineModel)

	if updated.mode == nil || updated.mode.Name() != domain.MethodologyMakeTime {
		t.Fatalf("mode = %v, want maketime", updated.mode)
	}
	if len(updated.presets) == 0 {
		t.Error("expected default presets when no callback is set")
	}
}

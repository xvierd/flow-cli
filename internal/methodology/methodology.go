// Package methodology encapsulates mode-specific behavior for each productivity methodology.
// The TUI and wizard query the Mode interface for presets, prompts, and feature flags
// instead of scattering methodology checks everywhere.
package methodology

import (
	"time"

	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
)

// Mode defines the interface for methodology-specific behavior.
type Mode interface {
	// Name returns the methodology identifier.
	Name() domain.Methodology

	// Presets returns the session duration presets for this mode.
	Presets() []config.SessionPreset

	// TaskPrompt returns the prompt text shown when asking for a task name.
	TaskPrompt() string

	// OutcomePrompt returns the prompt for intended outcome (Deep Work only; empty for others).
	OutcomePrompt() string

	// HasDistractionLog returns true if this mode tracks distractions during a session.
	HasDistractionLog() bool

	// HasEnergizeReminder returns true if this mode shows energize reminders mid-session.
	HasEnergizeReminder() bool

	// HasFocusScore returns true if this mode asks for a focus score on completion.
	HasFocusScore() bool

	// HasShutdownRitual returns true if this mode asks for accomplishment on completion.
	HasShutdownRitual() bool

	// HasHighlight returns true if this mode uses a daily highlight concept.
	HasHighlight() bool

	// CompletionTitle returns the title shown on session completion.
	CompletionTitle() string

	// DeepWorkGoalHours returns the daily deep work goal in hours (0 for non-deep-work modes).
	DeepWorkGoalHours() float64

	// Description returns a short onboarding description for this methodology.
	Description() string

	// TUITitle returns the title shown in the fullscreen TUI.
	TUITitle() string
}

// ForMethodology returns the Mode implementation for the given methodology.
func ForMethodology(m domain.Methodology, cfg *config.Config) Mode {
	switch m {
	case domain.MethodologyDeepWork:
		if cfg != nil {
			return &deepWorkMode{cfg: &cfg.DeepWork}
		}
		return &deepWorkMode{}
	case domain.MethodologyMakeTime:
		if cfg != nil {
			return &makeTimeMode{cfg: &cfg.MakeTime}
		}
		return &makeTimeMode{}
	default:
		if cfg != nil {
			return &pomodoroMode{cfg: &cfg.Pomodoro}
		}
		return &pomodoroMode{}
	}
}

// --- Pomodoro Mode ---

type pomodoroMode struct {
	cfg *config.PomodoroConfig
}

func (p *pomodoroMode) Name() domain.Methodology  { return domain.MethodologyPomodoro }
func (p *pomodoroMode) TaskPrompt() string        { return "What are you working on? (Enter to skip):" }
func (p *pomodoroMode) OutcomePrompt() string     { return "" }
func (p *pomodoroMode) HasDistractionLog() bool   { return false }
func (p *pomodoroMode) HasEnergizeReminder() bool { return false }
func (p *pomodoroMode) HasFocusScore() bool       { return false }
func (p *pomodoroMode) HasShutdownRitual() bool   { return false }
func (p *pomodoroMode) HasHighlight() bool        { return false }
func (p *pomodoroMode) CompletionTitle() string   { return "Session complete! Great work." }
func (p *pomodoroMode) DeepWorkGoalHours() float64 { return 0 }
func (p *pomodoroMode) Description() string {
	return "The Pomodoro Technique: 25-minute focused sprints with short breaks to maintain sustainable productivity."
}
func (p *pomodoroMode) TUITitle() string { return "Flow - Pomodoro Timer" }

func (p *pomodoroMode) Presets() []config.SessionPreset {
	if p.cfg != nil {
		return p.cfg.GetPresets()
	}
	return []config.SessionPreset{
		{Name: "Focus", Duration: 25 * time.Minute},
		{Name: "Short", Duration: 15 * time.Minute},
		{Name: "Deep", Duration: 50 * time.Minute},
	}
}

// --- Deep Work Mode ---

type deepWorkMode struct {
	cfg *config.DeepWorkConfig
}

func (d *deepWorkMode) Name() domain.Methodology  { return domain.MethodologyDeepWork }
func (d *deepWorkMode) TaskPrompt() string        { return "What will you focus on deeply?" }
func (d *deepWorkMode) OutcomePrompt() string     { return "Intended outcome for this session:" }
func (d *deepWorkMode) HasDistractionLog() bool   { return true }
func (d *deepWorkMode) HasEnergizeReminder() bool { return false }
func (d *deepWorkMode) HasFocusScore() bool       { return false }
func (d *deepWorkMode) HasShutdownRitual() bool   { return true }
func (d *deepWorkMode) HasHighlight() bool        { return false }
func (d *deepWorkMode) CompletionTitle() string   { return "Deep Work Session Complete." }
func (d *deepWorkMode) DeepWorkGoalHours() float64 {
	if d.cfg != nil {
		return d.cfg.DeepWorkGoalHours
	}
	return 4.0
}
func (d *deepWorkMode) Description() string {
	return "Cal Newport's Deep Work: distraction-free blocks of cognitively demanding work that push your abilities to their limit."
}
func (d *deepWorkMode) TUITitle() string { return "Deep Work" }

func (d *deepWorkMode) Presets() []config.SessionPreset {
	if d.cfg != nil {
		return d.cfg.GetPresets()
	}
	return []config.SessionPreset{
		{Name: "Deep", Duration: 90 * time.Minute},
		{Name: "Focus", Duration: 50 * time.Minute},
		{Name: "Shallow", Duration: 25 * time.Minute},
	}
}

// --- Make Time Mode ---

type makeTimeMode struct {
	cfg *config.MakeTimeConfig
}

func (mt *makeTimeMode) Name() domain.Methodology  { return domain.MethodologyMakeTime }
func (mt *makeTimeMode) TaskPrompt() string        { return "What's your Highlight for today?" }
func (mt *makeTimeMode) OutcomePrompt() string     { return "" }
func (mt *makeTimeMode) HasDistractionLog() bool   { return false }
func (mt *makeTimeMode) HasEnergizeReminder() bool { return true }
func (mt *makeTimeMode) HasFocusScore() bool       { return true }
func (mt *makeTimeMode) HasShutdownRitual() bool   { return false }
func (mt *makeTimeMode) HasHighlight() bool        { return true }
func (mt *makeTimeMode) CompletionTitle() string   { return "Session Complete!" }
func (mt *makeTimeMode) DeepWorkGoalHours() float64 { return 0 }
func (mt *makeTimeMode) Description() string {
	return "Jake Knapp's Make Time: choose a daily Highlight and laser focus on it. Energize your body to fuel your mind."
}
func (mt *makeTimeMode) TUITitle() string { return "Make Time" }

func (mt *makeTimeMode) Presets() []config.SessionPreset {
	if mt.cfg != nil {
		return mt.cfg.GetPresets()
	}
	return []config.SessionPreset{
		{Name: "Highlight", Duration: 60 * time.Minute},
		{Name: "Sprint", Duration: 25 * time.Minute},
		{Name: "Quick", Duration: 15 * time.Minute},
	}
}

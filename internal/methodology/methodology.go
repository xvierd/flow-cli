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
}

// ForMethodology returns the Mode implementation for the given methodology.
func ForMethodology(m domain.Methodology) Mode {
	switch m {
	case domain.MethodologyDeepWork:
		return &deepWorkMode{}
	case domain.MethodologyMakeTime:
		return &makeTimeMode{}
	default:
		return &pomodoroMode{}
	}
}

// --- Pomodoro Mode ---

type pomodoroMode struct{}

func (p *pomodoroMode) Name() domain.Methodology  { return domain.MethodologyPomodoro }
func (p *pomodoroMode) TaskPrompt() string        { return "What are you working on? (Enter to skip):" }
func (p *pomodoroMode) OutcomePrompt() string     { return "" }
func (p *pomodoroMode) HasDistractionLog() bool   { return false }
func (p *pomodoroMode) HasEnergizeReminder() bool { return false }
func (p *pomodoroMode) HasFocusScore() bool       { return false }
func (p *pomodoroMode) HasShutdownRitual() bool   { return false }
func (p *pomodoroMode) HasHighlight() bool        { return false }
func (p *pomodoroMode) CompletionTitle() string   { return "Session complete! Great work." }

func (p *pomodoroMode) Presets() []config.SessionPreset {
	return []config.SessionPreset{
		{Name: "Focus", Duration: 25 * time.Minute},
		{Name: "Short", Duration: 15 * time.Minute},
		{Name: "Deep", Duration: 50 * time.Minute},
	}
}

// --- Deep Work Mode ---

type deepWorkMode struct{}

func (d *deepWorkMode) Name() domain.Methodology  { return domain.MethodologyDeepWork }
func (d *deepWorkMode) TaskPrompt() string        { return "What will you focus on deeply?" }
func (d *deepWorkMode) OutcomePrompt() string     { return "Intended outcome for this session:" }
func (d *deepWorkMode) HasDistractionLog() bool   { return true }
func (d *deepWorkMode) HasEnergizeReminder() bool { return false }
func (d *deepWorkMode) HasFocusScore() bool       { return false }
func (d *deepWorkMode) HasShutdownRitual() bool   { return true }
func (d *deepWorkMode) HasHighlight() bool        { return false }
func (d *deepWorkMode) CompletionTitle() string   { return "Deep Work Session Complete." }

func (d *deepWorkMode) Presets() []config.SessionPreset {
	return []config.SessionPreset{
		{Name: "Deep", Duration: 90 * time.Minute},
		{Name: "Focus", Duration: 50 * time.Minute},
		{Name: "Shallow", Duration: 25 * time.Minute},
	}
}

// --- Make Time Mode ---

type makeTimeMode struct{}

func (mt *makeTimeMode) Name() domain.Methodology  { return domain.MethodologyMakeTime }
func (mt *makeTimeMode) TaskPrompt() string        { return "What's your Highlight for today?" }
func (mt *makeTimeMode) OutcomePrompt() string     { return "" }
func (mt *makeTimeMode) HasDistractionLog() bool   { return false }
func (mt *makeTimeMode) HasEnergizeReminder() bool { return true }
func (mt *makeTimeMode) HasFocusScore() bool       { return true }
func (mt *makeTimeMode) HasShutdownRitual() bool   { return false }
func (mt *makeTimeMode) HasHighlight() bool        { return true }
func (mt *makeTimeMode) CompletionTitle() string   { return "Session Complete!" }

func (mt *makeTimeMode) Presets() []config.SessionPreset {
	return []config.SessionPreset{
		{Name: "Highlight", Duration: 60 * time.Minute},
		{Name: "Sprint", Duration: 25 * time.Minute},
		{Name: "Quick", Duration: 15 * time.Minute},
	}
}

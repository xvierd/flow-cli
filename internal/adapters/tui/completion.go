package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/methodology"
)

// completionState holds all mode-specific state that is captured and managed
// after a session completes. It is embedded in both Model and InlineModel to
// eliminate field-level duplication between the two TUI implementations.
type completionState struct {
	// Deep Work: distraction log
	distractionMode         bool
	distractionInput        textinput.Model
	distractions            []string
	distractionCategoryMode bool
	distractionPendingText  string

	// Deep Work: accomplishment (shutdown ritual)
	accomplishmentMode  bool
	accomplishmentInput textinput.Model
	accomplishmentSaved bool

	// Deep Work: 4-step shutdown ritual (Cal Newport)
	shutdownRitualMode     bool
	shutdownStep           int // 0=pending tasks, 1=calendar review, 2=tomorrow plan, 3=closing phrase
	shutdownInputs         [4]textinput.Model
	shutdownComplete       bool
	shutdownRitualCallback func(sessionID string, ritual domain.ShutdownRitual) error

	// Deep Work: distraction review (shown after accomplishment in shutdown ritual)
	distractionReviewMode bool
	distractionReviewDone bool

	// Deep Work: outcome review (did you achieve the intended outcome?)
	outcomeReviewMode bool
	outcomeReviewDone bool
	outcomeAchieved   string // y/p/n

	// Make Time: focus score
	focusScore      *int
	focusScoreSaved bool

	// Make Time: energize reminder
	energizeShown bool
	energizeTicks int

	// Make Time: energize activity log
	energizeActivity string
	energizeSaved    bool

	// Auto-break
	autoBreak      bool
	autoBreakTicks int

	// Shared: captured at session completion (Deep Work)
	completedIntendedOutcome string
	// completedSessionID is the ID of the session that just completed, captured
	// when completion is detected so post-completion callbacks can target the
	// correct session even if chaining or auto-break has already started a new one.
	completedSessionID string
}

// reset clears all mode-specific completion state, ready for the next session.
func (c *completionState) reset() {
	c.accomplishmentSaved = false
	c.focusScore = nil
	c.focusScoreSaved = false
	c.distractions = nil
	c.distractionReviewMode = false
	c.distractionReviewDone = false
	c.outcomeReviewMode = false
	c.outcomeReviewDone = false
	c.outcomeAchieved = ""
	c.energizeActivity = ""
	c.energizeSaved = false
	c.shutdownRitualMode = false
	c.shutdownStep = 0
	c.shutdownComplete = false
	c.completedIntendedOutcome = ""
	c.completedSessionID = ""
}

// promptsDone returns true when all mode-specific completion prompts are satisfied,
// given the completed session type and current methodology mode.
func (c *completionState) promptsDone(mode methodology.Mode, completedType domain.SessionType) bool {
	if mode == nil {
		return true
	}
	// Deep Work: need shutdown ritual complete (or accomplishment saved), outcome review if applicable, and distraction review done
	if mode.HasShutdownRitual() && completedType == domain.SessionTypeWork {
		if !c.shutdownComplete && !c.accomplishmentSaved {
			return false
		}
		// Outcome review: if there was an intended outcome, ask if achieved
		if c.completedIntendedOutcome != "" && !c.outcomeReviewDone {
			return false
		}
		if len(c.distractions) > 0 && !c.distractionReviewDone {
			return false
		}
		return true
	}
	// Make Time: need focus score and energize activity logged
	if mode.HasFocusScore() && completedType == domain.SessionTypeWork {
		return c.focusScoreSaved && c.energizeSaved
	}
	// Pomodoro or break: always ready
	return true
}

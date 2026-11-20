package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
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
	// autoBreakOffered records that the countdown was already armed for the
	// current completion, so a countdown cancelled by a keypress is not re-armed.
	autoBreakOffered bool

	// Session completion flag (set when the active session ends)
	completed bool

	// Strict focus mode
	strict       bool
	strictLocked bool // transient notice shown when a strict-locked key is pressed

	// Confirm states for destructive keys
	confirmBreak  bool
	confirmFinish bool

	// Shared: captured at session completion (Deep Work)
	completedIntendedOutcome string
	// completedSessionID is the ID of the session that just completed, captured
	// when completion is detected so post-completion callbacks can target the
	// correct session even if chaining or auto-break has already started a new one.
	completedSessionID string
	// completedTaskID is the task of the session that just completed (empty if
	// none), used to tell whether the user worked on today's Highlight.
	completedTaskID string

	// discardedSessionID is the ID of a session the user ended without
	// completing it (void, or inline mode-switch cancel). Its disappearance from
	// the state is a return to idle, not a completion. It is deliberately not
	// cleared by reset().
	discardedSessionID string

	// tickGen is the generation of the one live tick chain. Ticks carrying any
	// other generation are stale and dropped, so there is never more than one
	// chain re-arming itself.
	tickGen int
}

// restartTick starts a fresh tick chain and retires any previous one.
func (c *completionState) restartTick() tea.Cmd {
	c.tickGen++
	return tickCmd(c.tickGen)
}

// sessionTransition classifies a state update by comparing the previously
// observed active session with the new one.
type sessionTransition int

const (
	transitionNone       sessionTransition = iota
	transitionCompleted                    // a session ended on its own or via stop
	transitionDiscarded                    // a session the user voided/cancelled ended
	transitionNewSession                   // a different session became active
)

// classifyTransition reports what changed between prev and next. A session
// replaced directly by another one (stop+break, skip break) is a new session,
// not a completion.
func (c *completionState) classifyTransition(prev, next *domain.PomodoroSession) sessionTransition {
	if prev != nil && next == nil {
		if prev.ID == c.discardedSessionID {
			return transitionDiscarded
		}
		return transitionCompleted
	}
	if next != nil && (prev == nil || prev.ID != next.ID) {
		return transitionNewSession
	}
	return transitionNone
}

// captureCompletion records the context of the session that just ended and
// returns the actual (pause-aware) time worked, capped at the planned duration.
func (c *completionState) captureCompletion(prev *domain.PomodoroSession) time.Duration {
	c.completedSessionID = prev.ID
	c.completedIntendedOutcome = prev.IntendedOutcome
	c.completedTaskID = ""
	if prev.TaskID != nil {
		c.completedTaskID = *prev.TaskID
	}
	c.completed = true
	// The distraction input belongs to the active session; it is not rendered
	// on the completion screen, so close it rather than capture keys invisibly.
	c.distractionMode = false
	c.distractionCategoryMode = false
	c.distractionPendingText = ""
	c.distractionInput.Blur()

	elapsed := prev.ElapsedTime()
	if elapsed > prev.Duration {
		elapsed = prev.Duration
	}
	if elapsed < 0 {
		elapsed = 0
	}
	return elapsed
}

// armAutoBreakIfReady starts the auto-break countdown once per completion, and
// only when the completed work session has no pending completion prompts, so
// the break never wipes an unanswered focus score or shutdown ritual.
func (c *completionState) armAutoBreakIfReady(mode methodology.Mode, completedType domain.SessionType) {
	if !c.autoBreak || !c.completed || c.autoBreakOffered || completedType != domain.SessionTypeWork {
		return
	}
	if !c.promptsDone(mode, completedType) {
		return
	}
	c.autoBreakTicks = 3
	c.autoBreakOffered = true
}

// energizeBreakHint reports whether the "[b]reak" hint may be shown next to
// the energize reminder: in strict mode a running work session cannot break.
func (c *completionState) energizeBreakHint(state *domain.CurrentState) bool {
	return !c.strictWorkBlocked(state)
}

// reset clears all per-session state, ready for the next session or break.
// It is the single reset point used whenever a new session starts (by key or
// when a different active session ID is observed). Config fields (strict,
// autoBreak, callbacks) and the text input models themselves are kept.
func (c *completionState) reset() {
	c.distractionMode = false
	c.distractionCategoryMode = false
	c.distractionPendingText = ""
	c.distractionInput.Blur()
	c.accomplishmentMode = false
	c.accomplishmentInput.Blur()
	for i := range c.shutdownInputs {
		c.shutdownInputs[i].Blur()
	}
	c.energizeShown = false
	c.energizeTicks = 0
	c.autoBreakTicks = 0
	c.autoBreakOffered = false
	c.completedTaskID = ""
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

// strictWorkBlocked reports whether strict focus mode locks early disengagement:
// an active (not yet completed) work session cannot be paused, finished, voided,
// or skipped into a break.
func (c *completionState) strictWorkBlocked(state *domain.CurrentState) bool {
	return c.strict && !c.completed && state != nil && state.ActiveSession != nil && state.ActiveSession.IsWorkSession()
}

// setStrictLocked records a blocked-key notice and clears any confirm states.
func (c *completionState) setStrictLocked() {
	c.strictLocked = true
	c.confirmBreak = false
	c.confirmFinish = false
}

// strictLockMessage returns the strict-mode lock notice. The inline variant also
// mentions mode switching, which only exists in the inline UI.
func strictLockMessage(inline bool) string {
	if inline {
		return i18n.T("🔒 STRICT: pause, finish, void, break and mode are locked — let the session run to completion")
	}
	return i18n.T("🔒 STRICT: pause, finish, void and break are locked — let the session run to completion")
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

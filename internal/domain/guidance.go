package domain

import (
	"strings"
	"time"

	"github.com/xvierd/flow-cli/internal/i18n"
)

// FocusMode is a coarse description of what the user is doing right now,
// meant for AI agents that adapt their behavior to the user's focus.
type FocusMode string

const (
	// FocusModeIdle means no session is active.
	FocusModeIdle FocusMode = "idle"
	// FocusModeDeepWork is a running Deep Work session.
	FocusModeDeepWork FocusMode = "deep_work"
	// FocusModePomodoro is a running Pomodoro work session.
	FocusModePomodoro FocusMode = "pomodoro"
	// FocusModeMakeTime is a running Make Time session.
	FocusModeMakeTime FocusMode = "make_time"
	// FocusModeBreak is a running or paused break.
	FocusModeBreak FocusMode = "break"
	// FocusModePaused is a paused work session.
	FocusModePaused FocusMode = "paused"
)

// wrapUpThreshold is how close to the end of a work session the guidance
// switches to helping the user wrap up.
const wrapUpThreshold = 5 * time.Minute

// RemainingAt returns how much time is left in s at now. It mirrors
// RemainingTime with an injectable clock.
func (s *PomodoroSession) RemainingAt(now time.Time) time.Duration {
	var elapsed time.Duration
	switch s.Status {
	case SessionStatusPaused:
		if s.PausedAt == nil {
			return s.Duration
		}
		elapsed = s.PausedAt.Sub(s.StartedAt)
	case SessionStatusRunning:
		elapsed = now.Sub(s.StartedAt)
	default:
		return 0
	}
	if remaining := s.Duration - elapsed; remaining > 0 {
		return remaining
	}
	return 0
}

// CurrentFocusMode classifies the active session of state.
func CurrentFocusMode(state *CurrentState) FocusMode {
	if state == nil || !state.IsSessionActive() {
		return FocusModeIdle
	}
	s := state.ActiveSession
	if s.IsBreakSession() {
		return FocusModeBreak
	}
	if s.Status == SessionStatusPaused {
		return FocusModePaused
	}
	switch s.Methodology {
	case MethodologyDeepWork:
		return FocusModeDeepWork
	case MethodologyMakeTime:
		return FocusModeMakeTime
	default:
		return FocusModePomodoro
	}
}

// minutesCeil rounds d up to whole minutes, so "40 min left" never
// understates the time the user committed to.
func minutesCeil(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	return int((d + time.Minute - 1) / time.Minute)
}

// FocusGuidance tells an AI agent how to behave given the user's current
// focus state at now. It is plain text, written to be injected verbatim into
// an agent's context.
func FocusGuidance(state *CurrentState, now time.Time) string {
	mode := CurrentFocusMode(state)
	if mode == FocusModeIdle {
		return i18n.T("No focus session is active. Behave as usual.")
	}

	s := state.ActiveSession
	mins := minutesCeil(s.RemainingAt(now))
	task := ""
	if state.ActiveTask != nil {
		task = strings.TrimSpace(state.ActiveTask.Title)
	}

	var parts []string
	switch mode {
	case FocusModeBreak:
		return i18n.T("The user is on a break (%d min left). Encourage them to rest away from the screen; keep answers short and defer non-urgent work until the break is over.", mins)
	case FocusModePaused:
		if task != "" {
			return i18n.T("The user paused their focus session on the task '%s'. Do not push them to resume; answer normally but keep the task in mind.", task)
		}
		return i18n.T("The user paused their focus session. Do not push them to resume; answer normally.")
	case FocusModeDeepWork:
		if task != "" {
			parts = append(parts, i18n.T("The user is in deep focus (Deep Work, %d min left) on the task '%s'.", mins, task))
		} else {
			parts = append(parts, i18n.T("The user is in deep focus (Deep Work, %d min left).", mins))
		}
		parts = append(parts, i18n.T("Keep answers concise, do not propose work outside the current task, and capture side ideas with log_distraction or create_task instead of digressing."))
	case FocusModeMakeTime:
		if task != "" {
			parts = append(parts, i18n.T("The user is working on today's Highlight '%s' (Make Time, %d min left).", task, mins))
		} else {
			parts = append(parts, i18n.T("The user is in a Make Time focus session (%d min left).", mins))
		}
		parts = append(parts, i18n.T("Help them stay on the Highlight: keep answers focused and park unrelated ideas with create_task."))
	default:
		if task != "" {
			parts = append(parts, i18n.T("The user is in a Pomodoro work session (%d min left) on the task '%s'.", mins, task))
		} else {
			parts = append(parts, i18n.T("The user is in a Pomodoro work session (%d min left).", mins))
		}
		parts = append(parts, i18n.T("Stay on task and keep answers focused; park side ideas with create_task instead of digressing."))
	}

	if outcome := strings.TrimSpace(s.IntendedOutcome); outcome != "" {
		parts = append(parts, i18n.T("Intended outcome for this session: %s.", strings.TrimRight(outcome, ".")))
	}
	if s.Status == SessionStatusRunning && s.RemainingAt(now) <= wrapUpThreshold {
		parts = append(parts, i18n.T("The session is about to end: help them reach a clean stopping point and avoid starting large changes."))
	}
	return strings.Join(parts, " ")
}

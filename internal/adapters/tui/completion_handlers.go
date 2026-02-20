package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/methodology"
)

// completionCallbacks holds the external callbacks used by completion input handlers.
type completionCallbacks struct {
	distractionCallback    func(string, string) error
	accomplishmentCallback func(string) error
	shutdownRitualCallback func(domain.ShutdownRitual) error
	mode                   methodology.Mode
}

// handleDistractionInput processes messages while in distraction logging mode.
// doneCmd is returned when the user finishes or cancels input (nil for fullscreen,
// tickCmd() for inline — which needs to keep the timer ticking).
func handleDistractionInput(cs *completionState, cb *completionCallbacks, msg tea.Msg, doneCmd tea.Cmd) tea.Cmd {
	// Category picker sub-mode
	if cs.distractionCategoryMode {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "i":
				cs.distractions = append(cs.distractions, cs.distractionPendingText)
				if cb.distractionCallback != nil {
					_ = cb.distractionCallback(cs.distractionPendingText, "internal")
				}
				cs.distractionCategoryMode = false
				cs.distractionMode = false
				return doneCmd
			case "e":
				cs.distractions = append(cs.distractions, cs.distractionPendingText)
				if cb.distractionCallback != nil {
					_ = cb.distractionCallback(cs.distractionPendingText, "external")
				}
				cs.distractionCategoryMode = false
				cs.distractionMode = false
				return doneCmd
			case "enter", "esc":
				cs.distractions = append(cs.distractions, cs.distractionPendingText)
				if cb.distractionCallback != nil {
					_ = cb.distractionCallback(cs.distractionPendingText, "")
				}
				cs.distractionCategoryMode = false
				cs.distractionMode = false
				return doneCmd
			case "ctrl+c":
				return tea.Quit
			}
		}
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			text := cs.distractionInput.Value()
			if text != "" {
				cs.distractionInput.Blur()
				if cb.mode != nil && cb.mode.HasDistractionLog() {
					cs.distractionPendingText = text
					cs.distractionCategoryMode = true
					return nil
				}
				cs.distractions = append(cs.distractions, text)
				if cb.distractionCallback != nil {
					_ = cb.distractionCallback(text, "")
				}
			}
			cs.distractionMode = false
			cs.distractionInput.Blur()
			return doneCmd
		case "esc":
			cs.distractionMode = false
			cs.distractionInput.Blur()
			return doneCmd
		case "ctrl+c":
			return tea.Quit
		}
	}

	var cmd tea.Cmd
	cs.distractionInput, cmd = cs.distractionInput.Update(msg)
	return cmd
}

// handleAccomplishmentInput processes messages while in accomplishment input mode.
func handleAccomplishmentInput(cs *completionState, cb *completionCallbacks, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			text := cs.accomplishmentInput.Value()
			cs.accomplishmentSaved = true // Always mark saved (empty = "skipped")
			if text != "" && cb.accomplishmentCallback != nil {
				_ = cb.accomplishmentCallback(text)
			}
			cs.accomplishmentMode = false
			cs.accomplishmentInput.Blur()
			// Auto-enter distraction review if there are distractions
			if len(cs.distractions) > 0 && !cs.distractionReviewDone {
				cs.distractionReviewMode = true
			}
			return nil
		case "esc":
			cs.accomplishmentMode = false
			cs.accomplishmentInput.Blur()
			return nil
		case "ctrl+c":
			return tea.Quit
		}
	}

	var cmd tea.Cmd
	cs.accomplishmentInput, cmd = cs.accomplishmentInput.Update(msg)
	return cmd
}

// handleShutdownRitual processes messages during the 3-step shutdown ritual.
func handleShutdownRitual(cs *completionState, cb *completionCallbacks, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			cs.shutdownInputs[cs.shutdownStep].Blur()
			cs.shutdownStep++
			if cs.shutdownStep >= 4 {
				return finishShutdownRitual(cs, cb)
			}
			cs.shutdownInputs[cs.shutdownStep].Focus()
			return cs.shutdownInputs[cs.shutdownStep].Cursor.BlinkCmd()
		case "esc":
			// Abandon the entire ritual — return to the completion screen
			cs.shutdownRitualMode = false
			for i := range cs.shutdownInputs {
				cs.shutdownInputs[i].Blur()
			}
			return nil
		case "ctrl+c":
			return tea.Quit
		}
	}

	var cmd tea.Cmd
	cs.shutdownInputs[cs.shutdownStep], cmd = cs.shutdownInputs[cs.shutdownStep].Update(msg)
	return cmd
}

// finishShutdownRitual completes the 3-step shutdown ritual and fires the callback.
func finishShutdownRitual(cs *completionState, cb *completionCallbacks) tea.Cmd {
	cs.shutdownRitualMode = false
	cs.shutdownComplete = true
	cs.accomplishmentSaved = true // Marks completion prompts as done

	ritual := domain.ShutdownRitual{
		PendingTasksReview: cs.shutdownInputs[0].Value(),
		CalendarReview:     cs.shutdownInputs[1].Value(),
		TomorrowPlan:       cs.shutdownInputs[2].Value(),
		ClosingPhrase:      cs.shutdownInputs[3].Value(),
	}
	if cb.shutdownRitualCallback != nil {
		_ = cb.shutdownRitualCallback(ritual)
	}

	// Auto-enter distraction review if there are distractions
	if len(cs.distractions) > 0 && !cs.distractionReviewDone {
		cs.distractionReviewMode = true
	}
	return nil
}

// handleDistractionReview processes messages during the distraction review overlay.
func handleDistractionReview(cs *completionState, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "esc":
			cs.distractionReviewMode = false
			cs.distractionReviewDone = true
			return nil
		case "ctrl+c":
			return tea.Quit
		}
	}
	return nil
}

// newCompletionInputs returns initialized text inputs for completion state.
// Exported for reuse in NewModel and NewInlineModel.
func newCompletionInputs(width int) (distraction, accomplishment textinput.Model, shutdown [4]textinput.Model) {
	distraction = textinput.New()
	distraction.Placeholder = "What distracted you?"
	distraction.CharLimit = 200
	distraction.Width = width

	accomplishment = textinput.New()
	accomplishment.Placeholder = "What did you accomplish?"
	accomplishment.CharLimit = 200
	accomplishment.Width = width

	// Cal Newport's 4-step shutdown ritual — explicit init avoids false-positive G602 warnings.
	newShutdownInput := func(placeholder string) textinput.Model {
		si := textinput.New()
		si.Placeholder = placeholder
		si.CharLimit = 200
		si.Width = width
		return si
	}
	shutdown[0] = newShutdownInput("Review pending tasks — anything urgent?")
	shutdown[1] = newShutdownInput("Review tomorrow's calendar — any conflicts?")
	shutdown[2] = newShutdownInput("Plan for tomorrow")
	shutdown[3] = newShutdownInput("Closing phrase (e.g. 'Shutdown complete')")
	return
}

package domain

import "fmt"

// Methodology represents the productivity methodology for a session.
type Methodology string

const (
	MethodologyPomodoro Methodology = "pomodoro"
	MethodologyDeepWork Methodology = "deepwork"
	MethodologyMakeTime Methodology = "maketime"
)

// ValidMethodologies lists all supported methodology values.
var ValidMethodologies = []Methodology{
	MethodologyPomodoro,
	MethodologyDeepWork,
	MethodologyMakeTime,
}

// ValidateMethodology checks if a string is a valid methodology.
func ValidateMethodology(s string) (Methodology, error) {
	m := Methodology(s)
	for _, valid := range ValidMethodologies {
		if m == valid {
			return m, nil
		}
	}
	return "", fmt.Errorf("invalid methodology %q: must be one of pomodoro, deepwork, maketime", s)
}

// MethodologyLabel returns a human-readable label.
func (m Methodology) Label() string {
	switch m {
	case MethodologyPomodoro:
		return "Pomodoro"
	case MethodologyDeepWork:
		return "Deep Work"
	case MethodologyMakeTime:
		return "Make Time"
	default:
		return "Unknown"
	}
}

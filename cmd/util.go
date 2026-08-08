package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/xvierd/flow-cli/internal/i18n"
)

// jsonOut prints a value as indented JSON. Returns the marshal error so callers
// surface it instead of silently dropping invalid output.
func jsonOut(v interface{}) error {
	jsonData, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to marshal JSON output"), err)
	}
	fmt.Println(string(jsonData))
	return nil
}

// formatMinutes formats a duration as a human-friendly string like "25m" or "1h30m".
func formatMinutes(d time.Duration) string {
	if d.Minutes() == float64(int(d.Minutes())) && int(d.Minutes())%60 == 0 && d >= time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	if d >= time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

// formatWizardDuration formats a duration as MM:SS.
func formatWizardDuration(d time.Duration) string {
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}

// sessionCountLabel returns a localized "1 session" / "N sessions" label.
func sessionCountLabel(n int) string {
	if n == 1 {
		return i18n.T("1 session")
	}
	return i18n.T("%d sessions", n)
}

// getDir returns the directory of a file path.
func getDir(path string) string {
	lastSep := 0
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			lastSep = i
			break
		}
	}
	if lastSep == 0 {
		return "."
	}
	return path[:lastSep]
}

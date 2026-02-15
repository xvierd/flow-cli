// Package notification provides desktop notification utilities.
package notification

import (
	"fmt"

	"github.com/gen2brain/beeep"
	"github.com/xvierd/flow-cli/internal/config"
)

// Notifier handles desktop notifications.
type Notifier struct {
	cfg *config.NotificationConfig
}

// New creates a new notifier with the given configuration.
func New(cfg *config.NotificationConfig) *Notifier {
	return &Notifier{cfg: cfg}
}

// Notify displays a desktop notification if enabled.
func (n *Notifier) Notify(title, message string) error {
	if n.cfg == nil || !n.cfg.Enabled {
		return nil
	}

	return beeep.Notify(title, message, "")
}

// NotifyPomodoroComplete displays a notification when a pomodoro session completes.
func (n *Notifier) NotifyPomodoroComplete(duration string) error {
	title := "🍅 Pomodoro Complete!"
	message := fmt.Sprintf("Great job! You completed a %s work session.", duration)
	return n.Notify(title, message)
}

// NotifyBreakComplete displays a notification when a break session completes.
func (n *Notifier) NotifyBreakComplete(breakType string) error {
	title := "☕ Break Over!"
	message := fmt.Sprintf("Your %s break is complete. Ready to focus?", breakType)
	return n.Notify(title, message)
}

// IsEnabled returns true if notifications are enabled.
func (n *Notifier) IsEnabled() bool {
	return n.cfg != nil && n.cfg.Enabled
}

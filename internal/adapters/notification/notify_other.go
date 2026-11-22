//go:build !darwin

package notification

import "github.com/gen2brain/beeep"

// send posts a desktop notification with the Flow icon.
func send(title, message string) error {
	return beeep.Notify(title, message, iconPNG)
}

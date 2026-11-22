package config

import "github.com/spf13/viper"

// IntegrationsConfig holds the built-in integrations toggled with
// `flow integrate enable|disable`. Secrets (the Slack token) are never stored
// here: they are read from the environment or the OS keychain.
type IntegrationsConfig struct {
	// DND turns on Do Not Disturb / Focus while a work session runs.
	DND bool `mapstructure:"dnd"`
	// DNDOnShortcut and DNDOffShortcut name the macOS Shortcuts that turn
	// Focus on and off.
	DNDOnShortcut  string `mapstructure:"dnd_on_shortcut"`
	DNDOffShortcut string `mapstructure:"dnd_off_shortcut"`
	// Slack sets a "Focusing until HH:MM" status during work sessions.
	Slack      bool   `mapstructure:"slack"`
	SlackEmoji string `mapstructure:"slack_emoji"`
	// Music pauses Spotify / Music when a work session starts and resumes
	// what it paused when the session ends (macOS).
	Music bool `mapstructure:"music"`
	// Tmux, Sketchybar, Waybar and Xbar record that the status-bar snippet
	// or plugin was installed; they have no start/end action.
	Tmux       bool `mapstructure:"tmux"`
	Sketchybar bool `mapstructure:"sketchybar"`
	Waybar     bool `mapstructure:"waybar"`
	Xbar       bool `mapstructure:"xbar"`
}

// Default names of the macOS Shortcuts used by the dnd integration.
const (
	DefaultDNDOnShortcut  = "Flow Focus On"
	DefaultDNDOffShortcut = "Flow Focus Off"
	DefaultSlackEmoji     = ":tomato:"
)

// DefaultIntegrationsConfig returns every integration disabled.
func DefaultIntegrationsConfig() IntegrationsConfig {
	return IntegrationsConfig{
		DNDOnShortcut:  DefaultDNDOnShortcut,
		DNDOffShortcut: DefaultDNDOffShortcut,
		SlackEmoji:     DefaultSlackEmoji,
	}
}

func setIntegrationsDefaults() {
	d := DefaultIntegrationsConfig()
	viper.SetDefault("integrations.dnd", false)
	viper.SetDefault("integrations.dnd_on_shortcut", d.DNDOnShortcut)
	viper.SetDefault("integrations.dnd_off_shortcut", d.DNDOffShortcut)
	viper.SetDefault("integrations.slack", false)
	viper.SetDefault("integrations.slack_emoji", d.SlackEmoji)
	viper.SetDefault("integrations.music", false)
	viper.SetDefault("integrations.tmux", false)
	viper.SetDefault("integrations.sketchybar", false)
	viper.SetDefault("integrations.waybar", false)
	viper.SetDefault("integrations.xbar", false)
}

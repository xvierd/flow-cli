package config

import (
	"os"
	"strings"
	"testing"
)

func TestIntegrations_DefaultsDisabled(t *testing.T) {
	cfg := DefaultConfig()
	in := cfg.Integrations
	if in.DND || in.Slack || in.Music || in.Tmux || in.Sketchybar || in.Waybar || in.Xbar {
		t.Errorf("integrations should default to disabled, got %+v", in)
	}
	if in.DNDOnShortcut != DefaultDNDOnShortcut || in.DNDOffShortcut != DefaultDNDOffShortcut {
		t.Errorf("unexpected default shortcut names: %+v", in)
	}
}

func TestIntegrations_LoadSaveRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := DefaultConfig()
	cfg.Integrations.DND = true
	cfg.Integrations.Slack = true
	cfg.Integrations.DNDOnShortcut = "My Focus"
	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !loaded.Integrations.DND || !loaded.Integrations.Slack || loaded.Integrations.Music {
		t.Errorf("integrations flags did not round-trip: %+v", loaded.Integrations)
	}
	if loaded.Integrations.DNDOnShortcut != "My Focus" {
		t.Errorf("dnd_on_shortcut = %q, want My Focus", loaded.Integrations.DNDOnShortcut)
	}

	path, _ := GetConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "[integrations]") {
		t.Errorf("config file has no [integrations] table:\n%s", data)
	}
}

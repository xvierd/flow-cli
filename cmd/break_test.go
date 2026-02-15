package cmd

import (
	"testing"

	"github.com/xvierd/flow-cli/internal/domain"
)

func TestBreakCmd(t *testing.T) {
	t.Run("break command structure", func(t *testing.T) {
		if breakCmd.Use != "break" {
			t.Errorf("breakCmd.Use = %q, want %q", breakCmd.Use, "break")
		}
		
		if breakCmd.Short != "Start a break session" {
			t.Errorf("breakCmd.Short = %q, want %q", breakCmd.Short, "Start a break session")
		}
	})
}

// TestGetBreakTypeLabel tests the break type label helper
func TestGetBreakTypeLabel(t *testing.T) {
	tests := []struct {
		sessionType domain.SessionType
		expected    string
	}{
		{domain.SessionTypeShortBreak, "Short Break"},
		{domain.SessionTypeLongBreak, "Long Break"},
		{domain.SessionTypeWork, "Break"},
		{domain.SessionType("unknown"), "Break"},
	}
	
	for _, tt := range tests {
		t.Run(string(tt.sessionType), func(t *testing.T) {
			got := getBreakTypeLabel(tt.sessionType)
			if got != tt.expected {
				t.Errorf("getBreakTypeLabel(%q) = %q, want %q", tt.sessionType, got, tt.expected)
			}
		})
	}
}

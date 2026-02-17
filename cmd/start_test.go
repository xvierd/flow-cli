package cmd

import (
	"testing"
	"time"
)

func TestStartCmd(t *testing.T) {
	t.Run("start command structure", func(t *testing.T) {
		if startCmd.Use != "start [task-id]" {
			t.Errorf("startCmd.Use = %q, want %q", startCmd.Use, "start [task-id]")
		}

		if startCmd.Short != "Start a pomodoro session" {
			t.Errorf("startCmd.Short = %q, want %q", startCmd.Short, "Start a pomodoro session")
		}
	})

	t.Run("start command has task flag", func(t *testing.T) {
		flag := startCmd.Flags().Lookup("task")
		if flag == nil {
			t.Fatal("startCmd should have --task flag")
		}
		if flag.Shorthand != "t" {
			t.Errorf("task flag shorthand = %q, want %q", flag.Shorthand, "t")
		}
	})
}

// TestFormatCmdDuration tests the formatCmdDuration helper
func TestFormatCmdDuration(t *testing.T) {
	tests := []struct {
		name     string
		minutes  int
		seconds  int
		expected string
	}{
		{"25 minutes", 25, 0, "25:00"},
		{"5 minutes 30 seconds", 5, 30, "05:30"},
		{"0 seconds", 0, 0, "00:00"},
		{"59 seconds", 0, 59, "00:59"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := time.Duration(tt.minutes)*time.Minute + time.Duration(tt.seconds)*time.Second
			got := formatCmdDuration(d)
			if got != tt.expected {
				t.Errorf("formatCmdDuration() = %q, want %q", got, tt.expected)
			}
		})
	}
}

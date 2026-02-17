package cmd

import (
	"testing"

	"github.com/xvierd/flow-cli/internal/domain"
)

func TestListCmd(t *testing.T) {
	t.Run("list command structure", func(t *testing.T) {
		if listCmd.Use != "list" {
			t.Errorf("listCmd.Use = %q, want %q", listCmd.Use, "list")
		}
	})

	t.Run("list command has status flag", func(t *testing.T) {
		flag := listCmd.Flags().Lookup("status")
		if flag == nil {
			t.Fatal("listCmd should have --status flag")
		}
		if flag.Shorthand != "s" {
			t.Errorf("status flag shorthand = %q, want %q", flag.Shorthand, "s")
		}
	})

	t.Run("list command has all flag", func(t *testing.T) {
		flag := listCmd.Flags().Lookup("all")
		if flag == nil {
			t.Fatal("listCmd should have --all flag")
		}
		if flag.Shorthand != "a" {
			t.Errorf("all flag shorthand = %q, want %q", flag.Shorthand, "a")
		}
	})
}

// TestGetStatusIcon tests the status icon helper
func TestGetStatusIcon(t *testing.T) {
	tests := []struct {
		status   domain.TaskStatus
		expected string
	}{
		{domain.StatusPending, "⏳"},
		{domain.StatusInProgress, "▶️"},
		{domain.StatusCompleted, "✅"},
		{domain.StatusCancelled, "❌"},
		{domain.TaskStatus("unknown"), "❓"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := getStatusIcon(tt.status)
			if got != tt.expected {
				t.Errorf("getStatusIcon(%q) = %q, want %q", tt.status, got, tt.expected)
			}
		})
	}
}

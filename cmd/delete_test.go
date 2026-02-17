package cmd

import (
	"testing"
)

func TestDeleteCmd(t *testing.T) {
	t.Run("delete command structure", func(t *testing.T) {
		if deleteCmd.Use != "delete [task-id]" {
			t.Errorf("deleteCmd.Use = %q, want %q", deleteCmd.Use, "delete [task-id]")
		}

		if deleteCmd.Short != "Delete a task" {
			t.Errorf("deleteCmd.Short = %q, want %q", deleteCmd.Short, "Delete a task")
		}
	})

	t.Run("delete command requires exact 1 arg", func(t *testing.T) {
		if deleteCmd.Args == nil {
			t.Fatal("deleteCmd.Args should be set")
		}

		// Test with correct number of args
		err := deleteCmd.Args(deleteCmd, []string{"task-id-123"})
		if err != nil {
			t.Errorf("delete with 1 arg should not error: %v", err)
		}
	})
}

// TestDeleteCmd_ValidateArgs tests argument validation
func TestDeleteCmd_ValidateArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"no args", []string{}, true},
		{"one arg", []string{"task-123"}, false},
		{"two args", []string{"task-123", "task-456"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := deleteCmd.Args(deleteCmd, tt.args)
			if tt.wantErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

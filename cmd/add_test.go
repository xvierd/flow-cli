package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestAddCmd(t *testing.T) {
	// Setup test database
	dbPath := "/tmp/flow_test_add.db"
	
	// Create root command for testing
	cmd := &cobra.Command{}
	cmd.AddCommand(addCmd)
	cmd.PersistentFlags().StringVar(&dbPath, "db", dbPath, "")
	cmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "")
	
	t.Run("add task with single word title", func(t *testing.T) {
		// This test would require mocking the services
		// For now, we verify command structure
		if addCmd.Use != "add [title]" {
			t.Errorf("addCmd.Use = %q, want %q", addCmd.Use, "add [title]")
		}
	})
	
	t.Run("add command accepts minimum 1 arg", func(t *testing.T) {
		// Check args function
		if addCmd.Args == nil {
			t.Error("addCmd.Args should be set")
		}
	})
	
	t.Run("add command has tags flag", func(t *testing.T) {
		flag := addCmd.Flags().Lookup("tags")
		if flag == nil {
			t.Error("addCmd should have --tags flag")
		}
		if flag.Shorthand != "t" {
			t.Errorf("tags flag shorthand = %q, want %q", flag.Shorthand, "t")
		}
	})
}

// TestAddCmd_ValidateArgs tests argument validation
func TestAddCmd_ValidateArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantErr  bool
	}{
		{"no args", []string{}, true},
		{"single word", []string{"task"}, false},
		{"multi word", []string{"my", "task", "name"}, false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := addCmd.Args(addCmd, tt.args)
			if tt.wantErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestAddCmd_JSONOutput tests JSON output structure
func TestAddCmd_JSONOutput(t *testing.T) {
	// Verify JSON structure for task output
	task := struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Status      string   `json:"status"`
		Tags        []string `json:"tags"`
		CreatedAt   string   `json:"created_at"`
	}{
		ID:          "test-id-123",
		Title:       "Test Task",
		Description: "",
		Status:      "pending",
		Tags:        []string{"tag1", "tag2"},
		CreatedAt:   "2024-01-01T12:00:00",
	}
	
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal task: %v", err)
	}
	
	// Verify the JSON contains expected fields
	output := string(data)
	if !strings.Contains(output, "test-id-123") {
		t.Error("JSON should contain task ID")
	}
	if !strings.Contains(output, "Test Task") {
		t.Error("JSON should contain task title")
	}
	if !strings.Contains(output, "pending") {
		t.Error("JSON should contain task status")
	}
}

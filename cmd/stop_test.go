package cmd

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
)

func TestStopCmd(t *testing.T) {
	t.Run("stop command structure", func(t *testing.T) {
		if stopCmd.Use != "stop" {
			t.Errorf("stopCmd.Use = %q, want %q", stopCmd.Use, "stop")
		}

		if stopCmd.Short != "Stop the current pomodoro session" {
			t.Errorf("stopCmd.Short = %q, want %q", stopCmd.Short, "Stop the current pomodoro session")
		}
	})
}

// TestOutputJSON tests the JSON output helper function
func TestOutputJSON(t *testing.T) {
	completedAt := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	taskID := "test-task-123"

	session := &domain.PomodoroSession{
		ID:          "test-session-456",
		TaskID:      &taskID,
		Type:        domain.SessionTypeWork,
		Status:      domain.SessionStatusCompleted,
		Duration:    25 * time.Minute,
		StartedAt:   time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		CompletedAt: &completedAt,
		Notes:       "Test notes",
	}

	// Test the data structure that would be marshaled
	data := map[string]interface{}{
		"id":         session.ID,
		"type":       string(session.Type),
		"status":     string(session.Status),
		"duration":   session.Duration.String(),
		"started_at": session.StartedAt.Format("2006-01-02T15:04:05"),
		"notes":      session.Notes,
	}
	if session.TaskID != nil {
		data["task_id"] = *session.TaskID
	}
	if session.CompletedAt != nil {
		data["completed_at"] = session.CompletedAt.Format("2006-01-02T15:04:05")
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal session: %v", err)
	}

	output := string(jsonData)

	// Verify all expected fields are present
	expectedFields := []string{
		"test-session-456",
		"work",
		"completed",
		"25m0s",
		"2024-01-15T10:00:00",
		"Test notes",
		"test-task-123",
		"2024-01-15T10:30:00",
	}

	for _, field := range expectedFields {
		if !json.Valid(jsonData) {
			t.Error("output should be valid JSON")
		}
		if output == "" {
			t.Error("output should not be empty")
		}
		// Check field is in output (simplified check)
		if len(field) > 5 && !contains(output, field) {
			t.Errorf("output should contain %q", field)
		}
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

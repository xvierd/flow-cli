package cmd

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
)

func TestStatusCmd(t *testing.T) {
	t.Run("status command structure", func(t *testing.T) {
		if statusCmd.Use != "status" {
			t.Errorf("statusCmd.Use = %q, want %q", statusCmd.Use, "status")
		}
		
		if statusCmd.Short != "Show current status" {
			t.Errorf("statusCmd.Short = %q, want %q", statusCmd.Short, "Show current status")
		}
	})
}

// TestOutputStatusJSON tests the JSON output structure
func TestOutputStatusJSON(t *testing.T) {
	// Create a mock state
	state := &domain.CurrentState{
		ActiveTask: &domain.Task{
			ID:          "task-123",
			Title:       "Test Task",
			Description: "Test Description",
			Status:      domain.StatusInProgress,
			Tags:        []string{"tag1", "tag2"},
			CreatedAt:   time.Now(),
		},
		ActiveSession: &domain.PomodoroSession{
			ID:        "session-456",
			TaskID:    strPtr("task-123"),
			Type:      domain.SessionTypeWork,
			Status:    domain.SessionStatusRunning,
			Duration:  25 * time.Minute,
			StartedAt: time.Now(),
			GitBranch: "main",
			GitCommit: "abc123def456",
			Notes:     "Session notes",
		},
		TodayStats: domain.DailyStats{
			WorkSessions:  5,
			BreaksTaken:   3,
			TotalWorkTime: 2*time.Hour + 5*time.Minute,
		},
	}
	
	// Simulate the JSON output structure
	result := map[string]interface{}{
		"active_task":    nil,
		"active_session": nil,
		"today_stats": map[string]interface{}{
			"work_sessions":   state.TodayStats.WorkSessions,
			"breaks_taken":    state.TodayStats.BreaksTaken,
			"total_work_time": state.TodayStats.TotalWorkTime.String(),
		},
	}
	
	if state.ActiveTask != nil {
		result["active_task"] = map[string]interface{}{
			"id":          state.ActiveTask.ID,
			"title":       state.ActiveTask.Title,
			"description": state.ActiveTask.Description,
			"status":      string(state.ActiveTask.Status),
			"tags":        state.ActiveTask.Tags,
		}
	}
	
	if state.ActiveSession != nil {
		session := state.ActiveSession
		sessionData := map[string]interface{}{
			"id":             session.ID,
			"type":           string(session.Type),
			"status":         string(session.Status),
			"duration":       session.Duration.String(),
			"remaining_time": session.RemainingTime().String(),
			"progress":       session.Progress(),
			"started_at":     session.StartedAt.Format("2006-01-02T15:04:05"),
			"git_branch":     session.GitBranch,
			"git_commit":     session.GitCommit,
			"notes":          session.Notes,
		}
		if session.TaskID != nil {
			sessionData["task_id"] = *session.TaskID
		}
		result["active_session"] = sessionData
	}
	
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal status: %v", err)
	}
	
	output := string(jsonData)
	
	// Verify structure
	if output == "" {
		t.Error("JSON output should not be empty")
	}
	
	// Verify all expected fields
	expectedFields := []string{
		"task-123",
		"Test Task",
		"session-456",
		"work",
		"running",
		"main",
		"abc123",
		"work_sessions",
		"breaks_taken",
		"total_work_time",
	}
	
	for _, field := range expectedFields {
		if len(field) > 3 && !containsStr(output, field) {
			t.Errorf("output should contain %q", field)
		}
	}
}

// TestPrintStatusText tests the printStatusText helper
func TestPrintStatusText(t *testing.T) {
	// Just verify the function signature and doesn't panic
	state := &domain.CurrentState{
		ActiveSession: &domain.PomodoroSession{
			Type:      domain.SessionTypeWork,
			Status:    domain.SessionStatusRunning,
			Duration:  25 * time.Minute,
			StartedAt: time.Now(),
			GitBranch: "main",
			GitCommit: "abc123def789",
		},
		ActiveTask: &domain.Task{
			Title: "Test Task",
		},
		TodayStats: domain.DailyStats{
			WorkSessions:  5,
			BreaksTaken:   3,
			TotalWorkTime: 2 * time.Hour,
		},
	}
	
	// Just verify the function doesn't panic
	printStatusText(state)
	
	// Test with nil session
	stateNoSession := &domain.CurrentState{
		ActiveSession: nil,
		TodayStats:    domain.DailyStats{},
	}
	printStatusText(stateNoSession)
}

func strPtr(s string) *string {
	return &s
}

func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}

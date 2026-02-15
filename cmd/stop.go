package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/dvidx/flow-cli/internal/domain"
	"github.com/spf13/cobra"
)

// stopCmd represents the stop command
var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the current pomodoro session",
	Long:  `Complete the current active pomodoro session.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		session, err := pomodoroSvc.StopSession(ctx)
		if err != nil {
			return fmt.Errorf("failed to stop session: %w", err)
		}

		// Prompt for notes (optional)
		fmt.Print("Add session notes (optional, press Enter to skip): ")
		reader := bufio.NewReader(os.Stdin)
		notes, _ := reader.ReadString('\n')
		notes = notes[:len(notes)-1] // Remove newline
		if notes != "" {
			session, _ = pomodoroSvc.AddSessionNotes(ctx, session.ID, notes)
		}

		// Send notification if enabled
		if notifier != nil && notifier.IsEnabled() {
			notifier.NotifyPomodoroComplete(session.Duration.String())
		}

		if jsonOutput {
			return outputJSON(session)
		}

		fmt.Printf("✅ Session completed! Duration: %s\n", session.Duration)
		if session.TaskID != nil {
			fmt.Printf("   Task ID: %s\n", *session.TaskID)
		}
		if session.Notes != "" {
			fmt.Printf("   Notes: %s\n", session.Notes)
		}

		return nil
	},
}

// outputJSON outputs a pomodoro session as JSON
func outputJSON(session *domain.PomodoroSession) error {
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
		return fmt.Errorf("failed to marshal session: %w", err)
	}
	fmt.Println(string(jsonData))
	return nil
}

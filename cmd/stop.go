package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/domain"
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

		// Methodology-aware prompts (only in interactive mode)
		if !jsonOutput {
			switch session.Methodology {
			case domain.MethodologyDeepWork:
				scanner := bufio.NewScanner(os.Stdin)
				fmt.Println()
				fmt.Println("  — Shutdown Ritual —")

				fmt.Print("  What did you accomplish? (Enter to skip): ")
				var accomplishment string
				if scanner.Scan() {
					accomplishment = strings.TrimSpace(scanner.Text())
					if accomplishment != "" {
						_ = pomodoroSvc.SetAccomplishment(ctx, session.ID, accomplishment)
					}
				}

				fmt.Print("  Pending tasks to review (Enter to skip): ")
				var pendingReview string
				if scanner.Scan() {
					pendingReview = strings.TrimSpace(scanner.Text())
				}

				fmt.Print("  Plan for tomorrow (Enter to skip): ")
				var tomorrowPlan string
				if scanner.Scan() {
					tomorrowPlan = strings.TrimSpace(scanner.Text())
				}

				fmt.Print("  Closing phrase (e.g. 'Shutdown complete', Enter to skip): ")
				var closingPhrase string
				if scanner.Scan() {
					closingPhrase = strings.TrimSpace(scanner.Text())
				}

				if pendingReview != "" || tomorrowPlan != "" || closingPhrase != "" {
					ritual := domain.ShutdownRitual{
						PendingTasksReview: pendingReview,
						TomorrowPlan:       tomorrowPlan,
						ClosingPhrase:      closingPhrase,
					}
					_ = pomodoroSvc.SetShutdownRitual(ctx, session.ID, ritual)
				}
				fmt.Println()
			case domain.MethodologyMakeTime:
				fmt.Print("Focus score (1-5, Enter to skip): ")
				scanner := bufio.NewScanner(os.Stdin)
				if scanner.Scan() {
					text := strings.TrimSpace(scanner.Text())
					if text != "" {
						score, err := strconv.Atoi(text)
						if err == nil && score >= 1 && score <= 5 {
							_ = pomodoroSvc.SetFocusScore(ctx, session.ID, score)
						}
					}
				}
			default:
				// Pomodoro: prompt for notes
				fmt.Print("Add session notes (optional, press Enter to skip): ")
				scanner := bufio.NewScanner(os.Stdin)
				if scanner.Scan() {
					notes := strings.TrimSpace(scanner.Text())
					if notes != "" {
						session, _ = pomodoroSvc.AddSessionNotes(ctx, session.ID, notes)
					}
				}
			}
		}

		// Send notification if enabled
		if notifier != nil && notifier.IsEnabled() {
			switch session.Type {
			case domain.SessionTypeWork:
				_ = notifier.NotifyPomodoroComplete(session.Duration.String())
			case domain.SessionTypeShortBreak:
				_ = notifier.NotifyBreakComplete("Short")
			case domain.SessionTypeLongBreak:
				_ = notifier.NotifyBreakComplete("Long")
			}
		}

		if jsonOutput {
			return outputJSON(session)
		}

		fmt.Printf("Session completed! Duration: %s\n", session.Duration)
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

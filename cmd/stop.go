package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
)

// stopCmd represents the stop command
var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the current session",
	Long:  `Complete the active session now, recording the time actually worked, then run the methodology's closing prompts.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		session, err := app.pomodoro.StopSession(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to stop session"), err)
		}

		// Methodology-aware prompts (only in interactive mode)
		if !jsonOutput {
			switch stopPromptMethodology(session) {
			case domain.MethodologyDeepWork:
				scanner := bufio.NewScanner(os.Stdin)
				fmt.Println()
				fmt.Println("  " + i18n.T("— Shutdown Ritual —"))

				fmt.Print("  " + i18n.T("What did you accomplish? (Enter to skip): "))
				var accomplishment string
				if scanner.Scan() {
					accomplishment = strings.TrimSpace(scanner.Text())
					if accomplishment != "" {
						_ = app.pomodoro.SetAccomplishment(ctx, session.ID, accomplishment)
					}
				}

				fmt.Print("  " + i18n.T("Pending tasks to review (Enter to skip): "))
				var pendingReview string
				if scanner.Scan() {
					pendingReview = strings.TrimSpace(scanner.Text())
				}

				fmt.Print("  " + i18n.T("Review tomorrow's calendar — any conflicts? (Enter to skip): "))
				var calendarReview string
				if scanner.Scan() {
					calendarReview = strings.TrimSpace(scanner.Text())
				}

				fmt.Print("  " + i18n.T("Plan for tomorrow (Enter to skip): "))
				var tomorrowPlan string
				if scanner.Scan() {
					tomorrowPlan = strings.TrimSpace(scanner.Text())
				}

				fmt.Print("  " + i18n.T("Closing phrase (e.g. 'Shutdown complete', Enter to skip): "))
				var closingPhrase string
				if scanner.Scan() {
					closingPhrase = strings.TrimSpace(scanner.Text())
				}

				if pendingReview != "" || calendarReview != "" || tomorrowPlan != "" || closingPhrase != "" {
					ritual := domain.ShutdownRitual{
						PendingTasksReview: pendingReview,
						CalendarReview:     calendarReview,
						TomorrowPlan:       tomorrowPlan,
						ClosingPhrase:      closingPhrase,
					}
					_ = app.pomodoro.SetShutdownRitual(ctx, session.ID, ritual)
				}

				// Outcome review: ask if intended outcome was achieved
				if session.IntendedOutcome != "" {
					fmt.Println()
					fmt.Printf("  %s\n", i18n.T("Did you achieve your intended outcome? (y/p/n): %s", session.IntendedOutcome))
					fmt.Print("  " + i18n.T("Answer [y]es/[p]artially/[n]o: "))
					if scanner.Scan() {
						answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
						if answer == "y" || answer == "p" || answer == "n" {
							_ = app.pomodoro.SetOutcomeAchieved(ctx, session.ID, answer)
						}
					}
				}
				fmt.Println()
			case domain.MethodologyMakeTime:
				scanner := bufio.NewScanner(os.Stdin)
				fmt.Println()
				fmt.Println("  " + i18n.T("— Make Time Reflection —"))

				fmt.Print("  " + i18n.T("Focus score (1-5, Enter to skip): "))
				if scanner.Scan() {
					text := strings.TrimSpace(scanner.Text())
					if text != "" {
						score, err := strconv.Atoi(text)
						if err == nil && score >= 1 && score <= 5 {
							_ = app.pomodoro.SetFocusScore(ctx, session.ID, score)
						}
					}
				}

				fmt.Print("  " + i18n.T("How did you energize? (walk, nap, exercise… Enter to skip): "))
				if scanner.Scan() {
					activity := strings.TrimSpace(scanner.Text())
					if activity != "" {
						_ = app.pomodoro.SetEnergizeActivity(ctx, session.ID, activity)
					}
				}
				fmt.Println()
			default:
				// Pomodoro: prompt for notes
				fmt.Print(i18n.T("Add session notes (optional, press Enter to skip): "))
				scanner := bufio.NewScanner(os.Stdin)
				if scanner.Scan() {
					notes := strings.TrimSpace(scanner.Text())
					if notes != "" {
						session, _ = app.pomodoro.AddSessionNotes(ctx, session.ID, notes)
					}
				}
			}
		}

		// Notification and on_end hook, as for any completed session.
		announceSessionEnd(ctx, session, session.Duration)

		if jsonOutput {
			return outputJSON(session)
		}

		fmt.Printf("%s\n", i18n.T("Session completed! Duration: %s", formatCmdDuration(session.Duration)))
		if session.TaskID != nil {
			fmt.Printf("%s\n", i18n.T("   Task ID: %s", *session.TaskID))
		}
		if session.Notes != "" {
			fmt.Printf("%s\n", i18n.T("   Notes: %s", session.Notes))
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
		"started_at": session.StartedAt.Format(time.RFC3339),
		"notes":      session.Notes,
	}
	if session.TaskID != nil {
		data["task_id"] = *session.TaskID
	}
	if session.CompletedAt != nil {
		data["completed_at"] = session.CompletedAt.Format(time.RFC3339)
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to marshal session"), err)
	}
	fmt.Println(string(jsonData))
	return nil
}

// stopPromptMethodology returns the methodology whose stop prompts apply to
// session. Breaks record their methodology too, but the shutdown ritual and
// the Make Time reflection belong to work sessions: a break keeps the
// Pomodoro notes prompt.
func stopPromptMethodology(session *domain.PomodoroSession) domain.Methodology {
	if !session.IsWorkSession() {
		return domain.MethodologyPomodoro
	}
	return session.Methodology
}

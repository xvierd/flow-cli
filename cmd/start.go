package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/ports"
	"github.com/xvierd/flow-cli/internal/services"
)

var startTaskID string
var startTags []string
var startDetach bool
var startNoBranchTask bool

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start [task-id]",
	Short: "Start a work session",
	Long: `Start a work session in the current methodology and open the timer.
Optionally pass a task ID (the short ID from "flow list" works) as an argument
or with --task. Without one, inside a git repository the session is named
after the branch ("feat/auth-refresh" → task "auth refresh", reusing a pending
task of that name); --no-branch-task turns this off. Nothing is suggested on
main, master or a detached HEAD. The length is the configured work duration
in Pomodoro, or the first preset in Deep Work and Make Time.

With --detach, or when there is no terminal (scripts, editors), the timer is
not opened: the session runs in the background and you are notified when it
ends.`,
	Example: `  flow start
  flow start 4f894d3e --tags backend
  flow --mode deepwork start --detach`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Get current working directory for git context
		workingDir, _ := os.Getwd()

		// Determine task ID
		var taskID *string
		if startTaskID != "" {
			taskID = &startTaskID
		} else if len(args) > 0 {
			taskID = &args[0]
		}

		// Check for active session and prompt user
		state, err := app.state.GetCurrentState(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to get current state"), err)
		}

		if state.ActiveSession != nil {
			proceed, err := confirmReplaceActive(ctx, state)
			if err != nil || !proceed {
				return err
			}
		}

		// Parse tags
		var tags []string
		for _, t := range startTags {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}

		// Start the pomodoro session
		// Record the methodology on the session: storage would otherwise
		// default it to Pomodoro, and resuming adopts the session's one.
		// Git runs before the transaction, which holds the write lock.
		req := services.StartPomodoroRequest{
			TaskID:      taskID,
			WorkingDir:  workingDir,
			Tags:        tags,
			Methodology: app.methodology,
			Duration:    defaultSessionDuration(),
			Git:         app.pomodoro.DetectGit(ctx, workingDir),
		}

		// Without a task, the git branch names one ("feat/auth-refresh" →
		// "auth refresh"); an existing pending task of that name is reused.
		// Finding or creating it and starting the session are one unit.
		branchTitle := ""
		if taskID == nil && !startNoBranchTask {
			branchTitle = branchTaskTitle(workingDir)
		}
		var session *domain.PomodoroSession
		err = app.storage.WithTx(ctx, func(tx ports.Storage) error {
			if branchTitle != "" {
				task, err := app.tasks.FindOrAddTaskWith(ctx, tx, branchTitle)
				if err != nil {
					return fmt.Errorf("%s: %w", i18n.T("failed to create task"), err)
				}
				req.TaskID = &task.ID
			}
			var err error
			session, err = app.pomodoro.StartPomodoroWith(ctx, tx, req)
			return err
		})
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to start pomodoro"), err)
		}

		fmt.Printf("▶️  %s\n", i18n.T("%s session started! Duration: %s", app.methodology.Label(), session.Duration))
		if session.TaskID != nil {
			if task, err := app.tasks.GetTask(ctx, *session.TaskID); err == nil {
				fmt.Printf("%s\n", i18n.T("   Task: %s (ID: %s)", task.Title, task.ID[:8]))
			}
		}

		background := startDetach || !interactiveTerminal()
		sessionStarted(ctx, session, background)
		if background {
			printBackgroundHint()
			return nil
		}

		// Refresh state and launch TUI
		state, err = app.state.GetCurrentState(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to get current state"), err)
		}

		return runSessions(state, workingDir)
	},
}

func init() {
	startCmd.Flags().StringVarP(&startTaskID, "task", "t", "", "Task ID to associate with this session")
	startCmd.Flags().BoolVarP(&startDetach, "detach", "d", false, "Start the session without opening the timer (it keeps running in the background)")
	startCmd.Flags().BoolVar(&startNoBranchTask, "no-branch-task", false, "Without a task, do not name one after the git branch")
	startCmd.Flags().StringSliceVar(&startTags, "tags", nil, "Tags for this session (repeatable or comma-separated, e.g. coding,backend)")
}

// confirmReplaceActive asks whether to stop the running session so a new one
// can start, and stops it on yes. Without a terminal the answer reads as no
// and the running session is kept.
func confirmReplaceActive(ctx context.Context, state *domain.CurrentState) (bool, error) {
	active := state.ActiveSession
	remaining := active.RemainingTime()
	sessionType := domain.GetSessionTypeLabel(active.Type)
	sessionInfo := i18n.T("%s session (%s remaining)", sessionType, formatCmdDuration(remaining))

	if state.ActiveTask != nil {
		sessionInfo = i18n.T("%s for task \"%s\" (%s remaining)", sessionType, state.ActiveTask.Title, formatCmdDuration(remaining))
	}

	fmt.Printf("%s\n", i18n.T("⚠️  A %s is already running: %s", strings.ToLower(sessionType), sessionInfo))
	fmt.Printf("%s\n", i18n.T("   Session ID: %s", active.ID[:8]))
	fmt.Print(i18n.T("Do you want to stop it and start a new one? [y/N] "))

	var answer string
	_, _ = fmt.Scanln(&answer)
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer != "y" && answer != "yes" {
		fmt.Println(i18n.T("Keeping current session."))
		return false, nil
	}

	if _, err := app.pomodoro.StopSession(ctx); err != nil {
		return false, fmt.Errorf("%s: %w", i18n.T("failed to stop current session"), err)
	}
	fmt.Println(i18n.T("⏹️  Previous session stopped."))
	return true, nil
}

func formatCmdDuration(d time.Duration) string {
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}

// defaultSessionDuration returns the work duration "flow start" uses for the
// current methodology: zero in Pomodoro (the service applies the configured
// work duration) and the first preset in Deep Work and Make Time, matching
// what the wizard preselects and what MCP sessions default to.
func defaultSessionDuration() time.Duration {
	if app.methodology == domain.MethodologyPomodoro || app.mode == nil {
		return 0
	}
	if presets := app.mode.Presets(); len(presets) > 0 {
		return presets[0].Duration
	}
	return 0
}

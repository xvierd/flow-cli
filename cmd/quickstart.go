package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/methodology"
	"github.com/xvierd/flow-cli/internal/ports"
	"github.com/xvierd/flow-cli/internal/services"
)

// quickDetach is the --detach flag of the frictionless starts ("flow go" and
// "flow 25 task").
var quickDetach bool

// Bounds of a duration given on the command line ("flow 25", "flow 90m").
const (
	minQuickDuration = time.Minute
	maxQuickDuration = 8 * time.Hour
)

// quickDurationPattern accepts a bare number of minutes ("25") or a number
// with an m/h unit, optionally combined ("90m", "1h", "1h30m"). At most three
// digits per number, so an all-digit 8-character short ID never parses.
var quickDurationPattern = regexp.MustCompile(`^(\d{1,3}|\d{1,2}h(\d{1,3}m)?|\d{1,3}m)$`)

// parseQuickDuration parses the leading duration argument of "flow 25 task".
// A bare number is minutes. It reports false for anything else, including
// out-of-range values, so callers can treat the argument as a typo instead.
func parseQuickDuration(s string) (time.Duration, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if !quickDurationPattern.MatchString(s) {
		return 0, false
	}
	var d time.Duration
	if n, err := strconv.Atoi(s); err == nil {
		d = time.Duration(n) * time.Minute
	} else {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return 0, false
		}
		d = parsed
	}
	if d < minQuickDuration || d > maxQuickDuration {
		return 0, false
	}
	return d, true
}

// rootArgs validates the bare "flow" arguments: none (the wizard) or a
// duration followed by an optional task ("flow 25 fix auth #backend").
// Anything else is reported the way cobra reports an unknown command, with
// suggestions, so a mistyped subcommand never starts a session.
func rootArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	if _, ok := parseQuickDuration(args[0]); ok {
		return nil
	}
	msg := fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath())
	if cmd.SuggestionsMinimumDistance <= 0 {
		cmd.SuggestionsMinimumDistance = 2 // cobra's default for its own suggestions
	}
	if suggestions := cmd.SuggestionsFor(args[0]); len(suggestions) > 0 {
		msg += "\n\nDid you mean this?\n\t" + strings.Join(suggestions, "\n\t")
	}
	msg += fmt.Sprintf("\n\nRun '%s --help' for usage; '%s 25 \"task\"' starts a 25-minute session.", cmd.CommandPath(), cmd.CommandPath())
	return errors.New(msg)
}

// runRoot dispatches the bare "flow" command: the wizard without arguments,
// a direct start for "flow <duration> [task...]".
func runRoot(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return runWizard(cmd, args)
	}
	d, _ := parseQuickDuration(args[0]) // validated by rootArgs
	title, tags := domain.ParseTagsFromInput(strings.Join(args[1:], " "))
	return runQuickStart(context.Background(), quickStart{
		Duration:  d,
		TaskTitle: title,
		Tags:      tags,
	}, quickDetach)
}

// goCmd repeats the last work session.
var goCmd = &cobra.Command{
	Use:   "go",
	Short: "Start now: repeat your last session (or a 25-minute Pomodoro)",
	Long: `Start a session with no questions asked. "flow go" repeats your last work
session: same methodology, duration, task and tags. With no previous session
it starts the default one (a 25-minute Pomodoro unless configured otherwise).

To pick the length directly, pass it to flow itself: "flow 25 \"fix auth\""
or "flow 90m deep dive #research" (a bare number is minutes; #words are tags).`,
	Example: `  flow go
  flow 25 "fix auth #backend"
  flow 90m "write the RFC"`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		sessions, err := allSessions(ctx)
		if err != nil {
			return err
		}
		qs := quickStart{}
		if plan, ok := lastSessionPlan(sessions); ok {
			qs = quickStart{
				Methodology: plan.Methodology,
				Duration:    plan.Duration,
				TaskID:      plan.TaskID,
				Tags:        plan.Tags,
			}
			// An explicit --mode wins; the remembered duration belongs to
			// the remembered methodology only.
			if modeFlag != "" && plan.Methodology != app.methodology {
				qs.Methodology = ""
				qs.Duration = 0
			}
		}
		return runQuickStart(ctx, qs, quickDetach)
	},
}

func init() {
	goCmd.Flags().BoolVarP(&quickDetach, "detach", "d", false, "Start the session without opening the timer (it keeps running in the background)")
}

// allSessions returns every recorded session, newest first.
func allSessions(ctx context.Context) ([]*domain.PomodoroSession, error) {
	sessions, err := app.storage.Sessions().FindRecent(ctx, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("failed to fetch sessions"), err)
	}
	return sessions, nil
}

// sessionPlan is what "flow go" repeats.
type sessionPlan struct {
	Methodology domain.Methodology
	// Duration is the planned length, or zero for the methodology default.
	Duration time.Duration
	TaskID   *string
	Tags     []string
}

// lastSessionPlan derives the session to repeat from sessions (newest
// first): the methodology, task and tags of the latest work session that was
// not cancelled, and the last planned duration in that methodology (see
// lastPlannedDuration).
func lastSessionPlan(sessions []*domain.PomodoroSession) (sessionPlan, bool) {
	for _, s := range sessions {
		if !s.IsWorkSession() || s.Status == domain.SessionStatusCancelled {
			continue
		}
		plan := sessionPlan{
			Methodology: sessionMethodologyOrDefault(s),
			TaskID:      s.TaskID,
			Tags:        s.Tags,
		}
		plan.Duration = lastPlannedDuration(sessions, plan.Methodology)
		return plan, true
	}
	return sessionPlan{}, false
}

// lastPlannedDuration returns the planned length of the latest ended work
// session in methodology m, or zero when there is none. Stopping or voiding
// a session early overwrites its Duration with the time worked (to the
// millisecond), while planned lengths (presets, "flow 25") are whole
// minutes, so sessions whose Duration is not a whole number of minutes are
// skipped as cut short.
func lastPlannedDuration(sessions []*domain.PomodoroSession, m domain.Methodology) time.Duration {
	for _, s := range sessions {
		if !s.IsWorkSession() || sessionMethodologyOrDefault(s) != m {
			continue
		}
		if s.Status != domain.SessionStatusCompleted && s.Status != domain.SessionStatusInterrupted {
			continue
		}
		if s.Duration > 0 && s.Duration%time.Minute == 0 {
			return s.Duration
		}
	}
	return 0
}

// sessionMethodologyOrDefault returns s's methodology, Pomodoro when it is
// missing or unknown (rows written before the column existed).
func sessionMethodologyOrDefault(s *domain.PomodoroSession) domain.Methodology {
	if m, err := domain.ValidateMethodology(string(s.Methodology)); err == nil {
		return m
	}
	return domain.MethodologyPomodoro
}

// quickStart describes a session started without the wizard.
type quickStart struct {
	// Methodology switches the current methodology; "" keeps it.
	Methodology domain.Methodology
	// Duration is the work length; zero uses the methodology default.
	Duration time.Duration
	// TaskID reuses a task; TaskTitle finds or creates one by title.
	TaskID    *string
	TaskTitle string
	Tags      []string
}

// runQuickStart starts qs and opens the timer, or leaves it running in the
// background with detach or without a terminal. A running session is only
// replaced after the user confirms, as with "flow start".
func runQuickStart(ctx context.Context, qs quickStart, detach bool) error {
	workingDir, _ := os.Getwd()

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

	if qs.Methodology != "" && qs.Methodology != app.methodology {
		setMethodology(qs.Methodology)
	}
	duration := qs.Duration
	if duration == 0 {
		duration = defaultSessionDuration()
	}

	session, err := startQuickSession(ctx, qs, duration, workingDir)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to start pomodoro"), err)
	}

	fmt.Printf("▶️  %s\n", i18n.T("%s session started! Duration: %s", app.methodology.Label(), formatMinutes(session.Duration)))
	if session.TaskID != nil {
		if task, err := app.tasks.GetTask(ctx, *session.TaskID); err == nil {
			fmt.Printf("%s\n", i18n.T("   Task: %s (ID: %s)", task.Title, task.ID[:8]))
		}
	}
	if len(session.Tags) > 0 {
		fmt.Printf("%s\n", i18n.T("   Tags: %s", "#"+strings.Join(session.Tags, " #")))
	}

	background := detach || !interactiveTerminal()
	sessionStarted(ctx, session, background)
	if background {
		printBackgroundHint()
		return nil
	}

	state, err = app.state.GetCurrentState(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to get current state"), err)
	}
	return runSessions(state, workingDir)
}

// startQuickSession resolves the task and starts the session in one
// transaction. A remembered task that was deleted or finished since is
// dropped rather than failing the start.
func startQuickSession(ctx context.Context, qs quickStart, duration time.Duration, workingDir string) (*domain.PomodoroSession, error) {
	gitInfo := app.pomodoro.DetectGit(ctx, workingDir)
	mode := app.mode
	var started *domain.PomodoroSession
	err := app.storage.WithTx(ctx, func(tx ports.Storage) error {
		taskID := qs.TaskID
		if taskID != nil {
			task, err := tx.Tasks().FindByID(ctx, *taskID)
			if err != nil || task.Status == domain.StatusCompleted || task.Status == domain.StatusCancelled {
				taskID = nil
			}
		}
		if taskID == nil && qs.TaskTitle != "" {
			id, created, err := findOrCreateTask(ctx, tx, qs.TaskTitle)
			if err != nil {
				return fmt.Errorf("%s: %w", i18n.T("failed to create task"), err)
			}
			taskID = &id
			if created && mode.HasHighlight() {
				if _, err := app.tasks.SetHighlightWith(ctx, tx, id); err != nil {
					return err
				}
			}
		}
		var err error
		started, err = app.pomodoro.StartPomodoroWith(ctx, tx, services.StartPomodoroRequest{
			TaskID:      taskID,
			WorkingDir:  workingDir,
			Duration:    duration,
			Methodology: app.methodology,
			Tags:        qs.Tags,
			Git:         gitInfo,
		})
		return err
	})
	return started, err
}

// findOrCreateTask returns the open task titled title (case-insensitive) so
// "flow 25 fix auth" run every day keeps one task, creating it when missing.
func findOrCreateTask(ctx context.Context, tx ports.Storage, title string) (id string, created bool, err error) {
	pending, err := tx.Tasks().FindPending(ctx)
	if err != nil {
		return "", false, err
	}
	for _, t := range pending {
		if strings.EqualFold(strings.TrimSpace(t.Title), title) {
			return t.ID, false, nil
		}
	}
	task, err := app.tasks.AddTaskWith(ctx, tx, services.AddTaskRequest{Title: title})
	if err != nil {
		return "", false, err
	}
	return task.ID, true, nil
}

// describePlan summarizes what a quick start would run, e.g.
// "Pomodoro · 25m · fix auth", for the wizard's "Start now" row.
func describePlan(ctx context.Context, plan sessionPlan, ok bool) string {
	m := app.methodology
	if ok && modeFlag == "" {
		m = plan.Methodology
	}
	d := plan.Duration
	if !ok || d == 0 || m != plan.Methodology {
		d = defaultDurationFor(m)
	}
	parts := []string{m.Label(), formatMinutes(d)}
	if ok && plan.TaskID != nil {
		if task, err := app.storage.Tasks().FindByID(ctx, *plan.TaskID); err == nil &&
			task.Status != domain.StatusCompleted && task.Status != domain.StatusCancelled {
			parts = append(parts, task.Title)
		}
	}
	return strings.Join(parts, " · ")
}

// defaultDurationFor returns the work duration a bare start uses in m: the
// configured work duration for Pomodoro, the first preset otherwise.
func defaultDurationFor(m domain.Methodology) time.Duration {
	if m == domain.MethodologyPomodoro {
		workDur, _, _, _ := app.config.ToPomodoroDomainConfig()
		return workDur
	}
	if presets := methodology.ForMethodology(m, app.config).Presets(); len(presets) > 0 {
		return presets[0].Duration
	}
	return 0
}

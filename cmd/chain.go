package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/xvierd/flow-cli/internal/adapters/tui"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/ports"
	"github.com/xvierd/flow-cli/internal/services"
)

// sessionWizard runs the fullscreen "new session" prompts (duration, task,
// outcome) and chains sessions. It carries the state that must survive across
// the sessions chained in one run.
type sessionWizard struct {
	workingDir string
	// carryAsked is set once the "carry forward yesterday's Highlight?"
	// question has been asked, so chained sessions do not ask it again.
	carryAsked bool
}

func newSessionWizard(workingDir string) *sessionWizard {
	return &sessionWizard{workingDir: workingDir}
}

// runSessions launches the TUI for state and keeps chaining new sessions while
// the user asks for one from the completion screen. Every command that launches
// the TUI for a session goes through here.
func runSessions(state *domain.CurrentState, workingDir string) error {
	return newSessionWizard(workingDir).run(state)
}

// run is runSessions with this wizard's run state.
func (w *sessionWizard) run(state *domain.CurrentState) error {
	return chainSessions(setupSignalHandler(), state,
		func(ctx context.Context, state *domain.CurrentState) error {
			return launchTUI(ctx, state, w.workingDir)
		},
		consumeNewSessionRequest,
		w.startSession,
	)
}

// chainSessions launches state, then, while wantsNew reports that the user
// pressed 'n' on the completion screen, prompts for and starts the next
// session via next and launches it. next returns a nil state when the user
// aborted the prompts. A cancelled ctx (SIGINT/SIGTERM) ends the chain.
func chainSessions(
	ctx context.Context,
	state *domain.CurrentState,
	launch func(context.Context, *domain.CurrentState) error,
	wantsNew func() bool,
	next func(context.Context) (*domain.CurrentState, error),
) error {
	for state != nil {
		adoptSessionMethodology(state)
		if err := launch(ctx, state); err != nil {
			return err
		}
		if ctx.Err() != nil || !wantsNew() {
			return nil
		}
		var err error
		state, err = next(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

// consumeNewSessionRequest reports whether the last fullscreen TUI ended with
// a request for a new session, and clears the request.
func consumeNewSessionRequest() bool {
	if inlineMode || lastFullscreenTimer == nil || !lastFullscreenTimer.WantsNewSession {
		return false
	}
	lastFullscreenTimer.WantsNewSession = false
	return true
}

// adoptSessionMethodology switches to the active session's methodology so the
// TUI's prompts, completion screen and break config match the session being
// resumed rather than the configured default.
func adoptSessionMethodology(state *domain.CurrentState) {
	if m, ok := sessionMethodology(state); ok && m != app.methodology {
		setMethodology(m)
	}
}

// sessionMethodology returns the methodology recorded on the active session.
// Breaks record the methodology they were started in, but breaks written
// before that read back as Pomodoro, so a break's Pomodoro is not trusted:
// only a break's non-Pomodoro methodology is adopted.
func sessionMethodology(state *domain.CurrentState) (domain.Methodology, bool) {
	if state == nil || state.ActiveSession == nil {
		return "", false
	}
	m, err := domain.ValidateMethodology(string(state.ActiveSession.Methodology))
	if err != nil {
		return "", false
	}
	if state.ActiveSession.Type != domain.SessionTypeWork && m == domain.MethodologyPomodoro {
		return "", false
	}
	return m, true
}

// carryForwardHighlight shows today's Highlight or, once per run, offers to
// carry forward yesterday's unfinished one (ask returns the user's answer).
// On Yes it makes that task today's Highlight and returns it so the session
// uses it without asking for a task again.
func (w *sessionWizard) carryForwardHighlight(ctx context.Context, ask func(*domain.Task) bool) *domain.Task {
	highlight, _ := app.storage.Tasks().FindTodayHighlight(ctx, time.Now())
	if highlight != nil {
		fmt.Printf("  %s\n\n", i18n.T("Today's Highlight: \"%s\"", highlight.Title))
		return nil
	}
	if w.carryAsked {
		return nil
	}
	yesterdayHighlight, _ := app.storage.Tasks().FindYesterdayHighlight(ctx, time.Now())
	if yesterdayHighlight == nil {
		return nil
	}
	w.carryAsked = true
	if !ask(yesterdayHighlight) {
		return nil
	}
	_, _ = app.tasks.SetHighlight(ctx, yesterdayHighlight.ID)
	return yesterdayHighlight
}

// askCarryForward asks whether to carry forward yesterday's Highlight.
func askCarryForward(task *domain.Task) bool {
	carryItems := []tui.PickerItem{
		{Label: i18n.T("Yes"), Desc: i18n.T("Continue with \"%s\"", task.Title)},
		{Label: i18n.T("No"), Desc: i18n.T("Pick a new Highlight")},
	}
	carryResult := tui.RunPicker(i18n.T("Carry forward yesterday's Highlight?"), carryItems, "", &app.config.Theme)
	fmt.Println()
	return !carryResult.Aborted && carryResult.Index == 0
}

// startSession prompts for the next session (Make Time highlight, duration,
// laser checklist, task, intended outcome), starts it, and returns the fresh
// state. It returns a nil state when the user aborts a prompt.
func (w *sessionWizard) startSession(ctx context.Context) (*domain.CurrentState, error) {
	mode := app.mode

	// Make Time: check for existing highlight or carry-over from yesterday
	var carried *domain.Task
	if mode.HasHighlight() {
		carried = w.carryForwardHighlight(ctx, askCarryForward)
	}

	// 1. Pick duration with arrow-key picker (mode-specific presets)
	presets := mode.Presets()
	shortBreak, longBreak := app.config.GetBreakDurations(app.methodology)

	var items []tui.PickerItem
	for _, p := range presets {
		items = append(items, tui.PickerItem{
			Label: p.Name,
			Desc:  formatMinutes(p.Duration),
		})
	}

	var footer string
	if app.methodology == domain.MethodologyPomodoro {
		footer = i18n.T("Breaks: %s short / %s long (every %d) · \"flow config\" to customize",
			formatMinutes(shortBreak), formatMinutes(longBreak), effectiveSessionsBeforeLong(app.config, app.methodology))
	} else {
		footer = i18n.T("Break: %s · \"flow config\" to customize", formatMinutes(shortBreak))
	}

	result := tui.RunPicker(i18n.T("Duration:"), items, footer, &app.config.Theme)
	if result.Aborted {
		return nil, nil
	}

	customDuration := presets[result.Index].Duration

	// Laser checklist (Make Time only)
	if mode.HasLaserChecklist() {
		fmt.Println()
		fmt.Println("  " + i18n.T("Laser Checklist:"))
		checklistItems := []string{
			i18n.T("Phone on Do Not Disturb?"),
			i18n.T("Notifications off?"),
			i18n.T("Distracting tabs/apps closed?"),
		}
		for _, item := range checklistItems {
			checkResult := tui.RunPicker(item, []tui.PickerItem{
				{Label: i18n.T("Yes"), Desc: i18n.T("Ready to focus")},
				{Label: i18n.T("No"), Desc: i18n.T("Skip for now")},
			}, "", &app.config.Theme)
			if checkResult.Aborted {
				return nil, nil
			}
			// User can skip with "No" and still proceed
		}
		fmt.Println()
	}

	// 2. Task selection via styled picker; a carried-forward Highlight is
	// already the chosen task.
	var taskName string
	var sessionTags []string
	var taskID *string

	recentTasks, _ := app.storage.Tasks().FindRecentTasks(ctx, 3)
	// The git branch suggests a new task's name, as an editable default.
	suggested := suggestionUnlessRecent(branchTaskTitle(w.workingDir), recentTasks)
	newTaskDesc := i18n.T("Type a name")
	if suggested != "" {
		newTaskDesc = i18n.T("Type a name (from branch: %s)", suggested)
	}
	switch {
	case carried != nil:
		taskName = carried.Title
		taskID = &carried.ID
	case len(recentTasks) > 0:
		var taskItems []tui.PickerItem
		for _, t := range recentTasks {
			taskItems = append(taskItems, tui.PickerItem{
				Label: t.Title,
				Desc:  "",
			})
		}
		taskItems = append(taskItems, tui.PickerItem{
			Label: i18n.T("New task..."),
			Desc:  newTaskDesc,
		})

		taskResult := tui.RunPicker(i18n.T(mode.TaskPrompt()), taskItems, "", &app.config.Theme)
		if taskResult.Aborted {
			return nil, nil
		}

		if taskResult.Index < len(recentTasks) {
			taskName = recentTasks[taskResult.Index].Title
			taskID = &recentTasks[taskResult.Index].ID
		} else {
			// "New task" selected — prompt for name
			textResult := tui.RunTextPromptWithValue(i18n.T(mode.TaskPrompt()), i18n.T("Enter to skip"), suggested, &app.config.Theme)
			if textResult.Aborted {
				return nil, nil
			}
			taskName = textResult.Value
		}
	default:
		textResult := tui.RunTextPromptWithValue(i18n.T(mode.TaskPrompt()), i18n.T("Enter to skip"), suggested, &app.config.Theme)
		if textResult.Aborted {
			return nil, nil
		}
		taskName = textResult.Value
	}

	// Parse #tags from task name input
	if taskName != "" {
		taskName, sessionTags = domain.ParseTagsFromInput(taskName)
	}

	// 3. Deep Work: ask for intended outcome
	var intendedOutcome string
	if mode.OutcomePrompt() != "" {
		outcomeResult := tui.RunTextPrompt(i18n.T(mode.OutcomePrompt()), i18n.T("Enter to skip"), &app.config.Theme)
		if outcomeResult.Aborted {
			return nil, nil
		}
		intendedOutcome = outcomeResult.Value
	}

	// Start the session; task creation, highlight update, and session
	// start are one atomic unit. Git runs before the transaction, which
	// holds the write lock.
	gitInfo := app.pomodoro.DetectGit(ctx, w.workingDir)
	var started *domain.PomodoroSession
	err := app.storage.WithTx(ctx, func(tx ports.Storage) error {
		if taskName != "" && taskID == nil {
			task, err := app.tasks.AddTaskWith(ctx, tx, services.AddTaskRequest{
				Title: taskName,
			})
			if err != nil {
				return fmt.Errorf("%s: %w", i18n.T("failed to create task"), err)
			}
			taskID = &task.ID

			// Make Time: set as today's highlight
			if mode.HasHighlight() {
				if _, err := app.tasks.SetHighlightWith(ctx, tx, task.ID); err != nil {
					return err
				}
			}
		}

		req := services.StartPomodoroRequest{
			TaskID:          taskID,
			WorkingDir:      w.workingDir,
			Duration:        customDuration,
			Methodology:     app.methodology,
			IntendedOutcome: intendedOutcome,
			Tags:            sessionTags,
			Git:             gitInfo,
		}
		var err error
		started, err = app.pomodoro.StartPomodoroWith(ctx, tx, req)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("failed to start pomodoro"), err)
	}
	// The TUI opening next treats this session as already announced.
	sessionStarted(ctx, started, false)

	state, err := app.state.GetCurrentState(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("failed to get current state"), err)
	}
	return state, nil
}

// promptTomorrowHighlight asks, in Make Time, for tomorrow's Highlight and
// saves it dated tomorrow so that tomorrow it is found as that day's
// Highlight. Ctrl+C (or SIGTERM) at the prompt skips it.
func promptTomorrowHighlight() error {
	if !app.mode.HasHighlight() {
		return nil
	}
	ctx := setupSignalHandler()
	fmt.Print("\n" + i18n.T("What's your Highlight for tomorrow? (Enter to skip): "))
	text, err := readLine(ctx, os.Stdin)
	if err != nil {
		fmt.Println()
		return nil
	}
	if text == "" {
		return nil
	}
	// The session already finished: a rejected title or a failed save is a
	// warning, not a failing exit status.
	task, err := newTomorrowHighlight(text, time.Now())
	if err == nil {
		err = app.storage.Tasks().Save(ctx, task)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", i18n.T("failed to create task"), err)
	}
	return nil
}

// newTomorrowHighlight returns a new task marked as the Highlight of the day
// after now (local midnight, calendar day so DST is handled).
func newTomorrowHighlight(title string, now time.Time) (*domain.Task, error) {
	task, err := domain.NewTask(title)
	if err != nil {
		return nil, err
	}
	tomorrow := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)
	task.HighlightDate = &tomorrow
	return task, nil
}

// readLine reads one trimmed line from r, returning ctx.Err() as soon as ctx
// is cancelled. Once the shared signal handler owns SIGINT, Ctrl+C no longer
// interrupts a blocking read, so plain-terminal prompts must use this. On
// cancellation the reading goroutine stays blocked on r; callers only use it
// for a final prompt before the process exits.
func readLine(ctx context.Context, r io.Reader) (string, error) {
	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := bufio.NewReader(r).ReadString('\n')
		if errors.Is(err, io.EOF) && line != "" {
			err = nil
		}
		ch <- result{strings.TrimSpace(line), err}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-ch:
		return res.line, res.err
	}
}

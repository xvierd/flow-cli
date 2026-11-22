package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/xvierd/flow-cli/internal/adapters/tui"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/methodology"
	"github.com/xvierd/flow-cli/internal/ports"
	"github.com/xvierd/flow-cli/internal/services"
)

// currentBreakInfo describes the break durations of the current methodology.
// It is recomputed on demand so it follows an in-TUI mode switch.
func currentBreakInfo() string {
	shortBreakDur, longBreakDur := app.config.GetBreakDurations(app.methodology)
	if app.methodology == domain.MethodologyPomodoro {
		return i18n.T("Breaks: %s short / %s long (every %d) · \"flow config\" to customize",
			formatMinutes(shortBreakDur), formatMinutes(longBreakDur), effectiveSessionsBeforeLong(app.config, app.methodology))
	}
	return i18n.T("Break: %s · \"flow config\" to customize", formatMinutes(shortBreakDur))
}

// completionInfoFor computes the next-break context for the current
// methodology once workSessions work sessions are done today. It mirrors
// domain.NewBreakSession's long-break rule, which StartBreak applies.
func completionInfoFor(ctx context.Context, workSessions int) *domain.CompletionInfo {
	shortBreakDur, longBreakDur := app.config.GetBreakDurations(app.methodology)
	// Always > 0 (Pomodoro forces 4; a non-positive config falls back to 4),
	// so the modulo below is safe.
	sessionsBeforeLong := effectiveSessionsBeforeLong(app.config, app.methodology)
	info := &domain.CompletionInfo{
		NextBreakType:      domain.SessionTypeShortBreak,
		NextBreakDuration:  shortBreakDur,
		SessionsUntilLong:  sessionsBeforeLong - workSessions%sessionsBeforeLong,
		SessionsBeforeLong: sessionsBeforeLong,
	}
	if workSessions > 0 && workSessions%sessionsBeforeLong == 0 {
		info.NextBreakType = domain.SessionTypeLongBreak
		info.NextBreakDuration = longBreakDur
		info.SessionsUntilLong = 0
	}
	if app.methodology == domain.MethodologyDeepWork {
		info.DeepWorkStreak, _ = app.pomodoro.GetDeepWorkStreak(ctx, time.Duration(app.config.DeepWork.DeepWorkGoalHours*float64(time.Hour)))
	}
	return info
}

// lastWorkSession remembers the planned parameters of the most recent work
// session so a bare "start" (the TUI's [s] key) repeats it instead of
// starting a generic Pomodoro. It is written from FetchState (which runs off
// the Bubbletea update loop) and read from CommandCallback, hence the mutex.
type lastWorkSession struct {
	mu          sync.Mutex
	set         bool
	duration    time.Duration
	taskID      *string
	methodology domain.Methodology
	tags        []string
}

// observe records s if it is an active work session. Only active sessions are
// used: StopSession and VoidSession shrink Duration on the ended session.
func (l *lastWorkSession) observe(s *domain.PomodoroSession) {
	if s == nil || !s.IsWorkSession() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.set = true
	l.duration = s.Duration
	l.taskID = s.TaskID
	l.methodology = s.Methodology
	l.tags = s.Tags
}

// startRequest builds the request for a bare start in the current mode. The
// last session's duration, task and tags are reused only when it belongs to
// the current methodology; otherwise the mode's first preset is used.
func (l *lastWorkSession) startRequest(workingDir string, current domain.Methodology, mode methodology.Mode) services.StartPomodoroRequest {
	req := services.StartPomodoroRequest{WorkingDir: workingDir, Methodology: current}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.set && (l.methodology == "" || l.methodology == current) {
		req.Duration = l.duration
		req.TaskID = l.taskID
		req.Tags = l.tags
		return req
	}
	if mode != nil {
		if presets := mode.Presets(); len(presets) > 0 {
			req.Duration = presets[0].Duration
		}
	}
	return req
}

// launchTUI starts the Bubbletea timer interface.
func launchTUI(_ context.Context, state *domain.CurrentState, workingDir string) error {
	ctx := setupSignalHandler()

	var timer *tui.Timer
	if inlineMode {
		timer = tui.NewInlineTimer(&app.config.Theme)
		lastInlineTimer = timer
	} else {
		timer = tui.NewFullscreenTimer(&app.config.Theme)
		lastFullscreenTimer = timer
	}

	// Presets (used by inline setup phase; harmless for fullscreen).
	presets := app.mode.Presets()

	var lastWork lastWorkSession
	lastWork.observe(state.ActiveSession)

	// The session the TUI opens with already ran its on_start hook.
	var seen seenSessions
	seen.observe(state.ActiveSession)

	// While the TUI runs it owns end-of-session announcements.
	tuiHeartbeat := heartbeatPath("tui")
	touchHeartbeat(tuiHeartbeat)

	// FetchState runs every second; report a failure once instead of
	// printing over the TUI on every tick.
	var fetchWarnOnce sync.Once

	firstRun := app.config.FirstRun
	if firstRun {
		app.config.FirstRun = false
		_ = saveConfig(app.config)
	}

	timer.Configure(tui.TimerConfig{
		Mode:       app.mode,
		ModeLocked: modeFlag != "",
		OnModeSelected: func(m domain.Methodology) methodology.Mode {
			setMethodology(m)
			presets = app.mode.Presets()
			timer.SetMode(app.mode)
			return app.mode
		},
		AutoBreak:            app.config.Pomodoro.AutoBreak,
		NotificationsEnabled: app.config.Notifications.Enabled,
		NotificationToggle: func(enabled bool) {
			app.config.Notifications.Enabled = enabled
			if app.notifier != nil {
				app.notifier.SetEnabled(enabled)
			}
			_ = saveConfig(app.config)
		},
		Strict: app.config.Focus.Strict || strictFlag,
		FetchState: func() *domain.CurrentState {
			touchHeartbeat(tuiHeartbeat)
			newState, err := app.state.GetCurrentState(ctx)
			if err != nil {
				fetchWarnOnce.Do(func() {
					fmt.Fprintf(os.Stderr, "%s\n", i18n.T("Warning: failed to fetch state: %v", err))
				})
				return nil
			}
			lastWork.observe(newState.ActiveSession)
			if started := seen.observe(newState.ActiveSession); started != nil {
				go func() { _ = runHook(ctx, "start", started) }()
			}
			return newState
		},
		CommandCallback: func(cmd ports.TimerCommand) error {
			switch cmd {
			case ports.CmdStart:
				req := lastWork.startRequest(workingDir, app.methodology, app.mode)
				_, err := app.pomodoro.StartPomodoro(ctx, req)
				if err != nil && req.TaskID != nil && !errors.Is(err, domain.ErrSessionAlreadyActive) {
					// The remembered task may have been deleted: start without it.
					req.TaskID = nil
					_, err = app.pomodoro.StartPomodoro(ctx, req)
				}
				return err
			case ports.CmdPause:
				_, err := app.pomodoro.PauseSession(ctx)
				return err
			case ports.CmdResume:
				_, err := app.pomodoro.ResumeSession(ctx)
				return err
			case ports.CmdStop:
				_, err := app.pomodoro.StopSession(ctx)
				return err
			case ports.CmdCancel:
				return app.pomodoro.CancelSession(ctx)
			case ports.CmdVoid:
				_, err := app.pomodoro.VoidSession(ctx)
				return err
			case ports.CmdBreak:
				_, err := app.pomodoro.StartBreak(ctx, workingDir)
				return err
			default:
				return fmt.Errorf("%s", i18n.T("unknown command: %v", cmd))
			}
		},
		DistractionCallback: func(text string, category string) error {
			activeState, err := app.state.GetCurrentState(ctx)
			if err != nil || activeState.ActiveSession == nil {
				return nil
			}
			return app.pomodoro.LogDistraction(ctx, activeState.ActiveSession.ID, text, category)
		},
		AccomplishmentCallback: func(sessionID string, text string) error {
			if sessionID == "" {
				return nil
			}
			return app.pomodoro.SetAccomplishment(ctx, sessionID, text)
		},
		ShutdownRitualCallback: func(sessionID string, ritual domain.ShutdownRitual) error {
			if sessionID == "" {
				return nil
			}
			return app.pomodoro.SetShutdownRitual(ctx, sessionID, ritual)
		},
		FocusScoreCallback: func(sessionID string, score int) error {
			if sessionID == "" {
				return nil
			}
			return app.pomodoro.SetFocusScore(ctx, sessionID, score)
		},
		EnergizeCallback: func(sessionID string, activity string) error {
			if sessionID == "" {
				return nil
			}
			return app.pomodoro.SetEnergizeActivity(ctx, sessionID, activity)
		},
		OutcomeAchievedCallback: func(sessionID string, achieved string) error {
			if sessionID == "" {
				return nil
			}
			return app.pomodoro.SetOutcomeAchieved(ctx, sessionID, achieved)
		},
		OnSessionCompleted: func(sessionType domain.SessionType, worked time.Duration) {
			notifySessionEnd(sessionType, app.methodology, worked)
			if ended := seen.last(); ended != nil {
				go func() { _ = runHook(ctx, "end", ended) }()
			}
		},
		// Initial context for the upcoming completion (this session included);
		// refreshed from the fresh state whenever a session completes.
		CompletionInfo: completionInfoFor(ctx, state.TodayStats.WorkSessions+1),
		FetchCompletionInfo: func(fresh *domain.CurrentState) *domain.CompletionInfo {
			return completionInfoFor(ctx, fresh.TodayStats.WorkSessions)
		},
		FetchGitSummary: func(sessionID string) string {
			return app.pomodoro.GitSummaryFor(ctx, sessionID).String()
		},
		SuggestedTaskTitle: branchTaskTitle(workingDir),
		// Inline-specific fields (zero/nil values are ignored by fullscreen mode).
		Presets:        presets,
		BreakInfo:      currentBreakInfo(),
		FetchBreakInfo: currentBreakInfo,
		FetchRecentTasks: func(limit int) []*domain.Task {
			tasks, err := app.storage.Tasks().FindRecentTasks(ctx, limit)
			if err != nil {
				return nil
			}
			return tasks
		},
		FetchYesterdayHighlight: func() *domain.Task {
			task, _ := app.storage.Tasks().FindYesterdayHighlight(ctx, time.Now())
			return task
		},
		OnStartSessionForTask: func(presetIndex int, existingTaskID, taskName, intendedOutcome string) error {
			currentMode := app.mode
			currentPresets := currentMode.Presets()

			var sessionTags []string
			if taskName != "" && existingTaskID == "" {
				taskName, sessionTags = domain.ParseTagsFromInput(taskName)
			}

			// Git runs before the transaction, which holds the write lock.
			gitInfo := app.pomodoro.DetectGit(ctx, workingDir)

			// Task creation, highlight update, and session start are one
			// atomic unit.
			return app.storage.WithTx(ctx, func(tx ports.Storage) error {
				var taskID *string
				switch {
				case existingTaskID != "":
					// A recent task or the carried-over Highlight: reuse it.
					id := existingTaskID
					taskID = &id
				case taskName != "":
					task, err := app.tasks.AddTaskWith(ctx, tx, services.AddTaskRequest{
						Title: taskName,
					})
					if err != nil {
						return err
					}
					taskID = &task.ID
				}
				if taskID != nil && currentMode.HasHighlight() {
					if _, err := app.tasks.SetHighlightWith(ctx, tx, *taskID); err != nil {
						return err
					}
				}

				_, err := app.pomodoro.StartPomodoroWith(ctx, tx, services.StartPomodoroRequest{
					TaskID:          taskID,
					WorkingDir:      workingDir,
					Duration:        currentPresets[presetIndex].Duration,
					Methodology:     app.methodology,
					Tags:            sessionTags,
					IntendedOutcome: intendedOutcome,
					Git:             gitInfo,
				})
				return err
			})
		},
		FirstRun: firstRun,
	})

	runErr := timer.Run(ctx, state)
	if tuiHeartbeat != "" {
		_ = os.Remove(tuiHeartbeat)
	}

	// A session still active when the timer closes (c, ctrl+c, or a TUI that
	// failed to start) is announced by the background watcher instead.
	if current, err := app.state.GetCurrentState(context.Background()); err == nil && current.ActiveSession != nil {
		ensureWatcher()
	}

	if runErr != nil {
		return fmt.Errorf("%s: %w", i18n.T("timer error"), runErr)
	}
	return nil
}

// seenSessions tracks the active sessions the TUI has shown, to run the
// on_start hook for sessions it starts and to know which session just ended.
type seenSessions struct {
	mu       sync.Mutex
	activeID string
	lastSeen *domain.PomodoroSession
}

// observe records the active session s and returns it when it is a session
// not seen before (one that just started), nil otherwise.
func (t *seenSessions) observe(s *domain.PomodoroSession) *domain.PomodoroSession {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s == nil {
		t.activeID = ""
		return nil
	}
	t.lastSeen = s
	if s.ID == t.activeID {
		return nil
	}
	t.activeID = s.ID
	return s
}

// last returns the most recent active session observed.
func (t *seenSessions) last() *domain.PomodoroSession {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastSeen
}

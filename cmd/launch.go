package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/xvierd/flow-cli/internal/adapters/tui"
	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/methodology"
	"github.com/xvierd/flow-cli/internal/ports"
	"github.com/xvierd/flow-cli/internal/services"
)

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

	// Presets and break info (used by inline setup phase; harmless for fullscreen).
	presets := app.mode.Presets()
	shortBreakDur, longBreakDur := app.config.GetBreakDurations(app.methodology)
	var breakInfo string
	if app.methodology == domain.MethodologyPomodoro {
		breakInfo = fmt.Sprintf("Breaks: %s short / %s long (every %d) · \"flow config\" to customize",
			formatMinutes(shortBreakDur), formatMinutes(longBreakDur), app.config.Pomodoro.SessionsBeforeLong)
	} else {
		breakInfo = fmt.Sprintf("Break: %s · \"flow config\" to customize", formatMinutes(shortBreakDur))
	}

	// Completion info: next break type and duration.
	_, _, _, sessionsBeforeLong := app.config.ToPomodoroDomainConfig()
	workSessions := state.TodayStats.WorkSessions + 1
	sessionsUntilLong := sessionsBeforeLong - (workSessions % sessionsBeforeLong)
	if sessionsUntilLong == sessionsBeforeLong {
		sessionsUntilLong = 0
	}
	nextBreakType := domain.SessionTypeShortBreak
	nextBreakDuration := shortBreakDur
	if sessionsUntilLong == 0 {
		nextBreakType = domain.SessionTypeLongBreak
		nextBreakDuration = longBreakDur
	}

	var deepWorkStreak int
	if app.methodology == domain.MethodologyDeepWork {
		deepWorkStreak, _ = app.pomodoro.GetDeepWorkStreak(ctx, time.Duration(app.config.DeepWork.DeepWorkGoalHours*float64(time.Hour)))
	}

	firstRun := app.config.FirstRun
	if firstRun {
		app.config.FirstRun = false
		_ = config.Save(app.config)
	}

	timer.Configure(tui.TimerConfig{
		Mode:       app.mode,
		ModeLocked: modeFlag != "",
		OnModeSelected: func(m domain.Methodology) {
			app.methodology = m
			app.mode = methodology.ForMethodology(m, app.config)
			presets = app.mode.Presets()
			timer.SetMode(app.mode)
		},
		AutoBreak:            app.config.Pomodoro.AutoBreak,
		NotificationsEnabled: app.config.Notifications.Enabled,
		NotificationToggle: func(enabled bool) {
			app.config.Notifications.Enabled = enabled
			if app.notifier != nil {
				app.notifier.SetEnabled(enabled)
			}
			_ = config.Save(app.config)
		},
		FetchState: func() *domain.CurrentState {
			newState, err := app.state.GetCurrentState(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to fetch state: %v\n", err)
				return nil
			}
			return newState
		},
		CommandCallback: func(cmd ports.TimerCommand) error {
			switch cmd {
			case ports.CmdStart:
				_, err := app.pomodoro.StartPomodoro(ctx, services.StartPomodoroRequest{
					WorkingDir: workingDir,
				})
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
				return fmt.Errorf("unknown command: %v", cmd)
			}
		},
		DistractionCallback: func(text string, category string) error {
			activeState, err := app.pomodoro.GetCurrentState(ctx)
			if err != nil || activeState.ActiveSession == nil {
				return nil
			}
			return app.pomodoro.LogDistraction(ctx, activeState.ActiveSession.ID, text, category)
		},
		AccomplishmentCallback: func(text string) error {
			recent, err := app.pomodoro.GetRecentSessions(ctx, 1)
			if err != nil || len(recent) == 0 {
				return nil
			}
			return app.pomodoro.SetAccomplishment(ctx, recent[0].ID, text)
		},
		ShutdownRitualCallback: func(ritual domain.ShutdownRitual) error {
			recent, err := app.pomodoro.GetRecentSessions(ctx, 1)
			if err != nil || len(recent) == 0 {
				return nil
			}
			return app.pomodoro.SetShutdownRitual(ctx, recent[0].ID, ritual)
		},
		FocusScoreCallback: func(score int) error {
			recent, err := app.pomodoro.GetRecentSessions(ctx, 1)
			if err != nil || len(recent) == 0 {
				return nil
			}
			return app.pomodoro.SetFocusScore(ctx, recent[0].ID, score)
		},
		EnergizeCallback: func(activity string) error {
			recent, err := app.pomodoro.GetRecentSessions(ctx, 1)
			if err != nil || len(recent) == 0 {
				return nil
			}
			return app.pomodoro.SetEnergizeActivity(ctx, recent[0].ID, activity)
		},
		OnSessionComplete: func(sessionType domain.SessionType) {
			if app.notifier == nil || !app.notifier.IsEnabled() {
				return
			}
			var err error
			switch sessionType {
			case domain.SessionTypeWork:
				err = app.notifier.NotifyPomodoroComplete(formatMinutes(shortBreakDur))
			case domain.SessionTypeShortBreak:
				err = app.notifier.NotifyBreakComplete("Short")
			case domain.SessionTypeLongBreak:
				err = app.notifier.NotifyBreakComplete("Long")
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: notification failed: %v\n", err)
			}
		},
		CompletionInfo: &domain.CompletionInfo{
			NextBreakType:      nextBreakType,
			NextBreakDuration:  nextBreakDuration,
			SessionsUntilLong:  sessionsUntilLong,
			SessionsBeforeLong: sessionsBeforeLong,
			DeepWorkStreak:     deepWorkStreak,
		},
		// Inline-specific fields (zero/nil values are ignored by fullscreen mode).
		Presets:   presets,
		BreakInfo: breakInfo,
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
		OnStartSession: func(presetIndex int, taskName string, intendedOutcome string) error {
			currentMode := app.mode
			currentPresets := currentMode.Presets()

			var sessionTags []string
			if taskName != "" {
				taskName, sessionTags = domain.ParseTagsFromInput(taskName)
			}

			var taskID *string
			if taskName != "" {
				task, err := app.tasks.AddTask(ctx, services.AddTaskRequest{
					Title: taskName,
				})
				if err != nil {
					return err
				}
				taskID = &task.ID
				if currentMode.HasHighlight() {
					task.SetAsHighlight()
					_ = app.storage.Tasks().Update(ctx, task)
				}
			}

			_, err := app.pomodoro.StartPomodoro(ctx, services.StartPomodoroRequest{
				TaskID:          taskID,
				WorkingDir:      workingDir,
				Duration:        currentPresets[presetIndex].Duration,
				Methodology:     app.methodology,
				Tags:            sessionTags,
				IntendedOutcome: intendedOutcome,
			})
			return err
		},
		FirstRun: firstRun,
	})

	if err := timer.Run(ctx, state); err != nil {
		return fmt.Errorf("timer error: %w", err)
	}

	return nil
}

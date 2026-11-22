package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
)

// End-of-session announcements (desktop notification and the on_end hook)
// have exactly one owner at a time:
//
//   - an open timer TUI announces the sessions it shows; it refreshes a
//     heartbeat file next to the database every second while it runs;
//   - otherwise a detached background watcher (`flow __watch`) waits for the
//     active session to run out, completes it and announces it.
//
// The watcher is started whenever a session may outlive the TUI: `flow start`
// or `flow break` without a terminal (or with --detach), the TUI closing with
// a session still active, and sessions started or resumed over MCP. Only the
// process whose transaction completes an expired session announces it
// (PomodoroService.CompleteIfExpired), so two watchers never double-notify.

const (
	// tuiHeartbeatMaxAge is how old the TUI heartbeat may be before the TUI
	// counts as gone (it is refreshed every second).
	tuiHeartbeatMaxAge = 5 * time.Second
	// watchPollInterval bounds how long the watcher sleeps between checks,
	// so it notices pauses, stops and new sessions.
	watchPollInterval = 15 * time.Second
	// watcherHeartbeatMaxAge is how old the watcher heartbeat may be before
	// a new watcher is started.
	watcherHeartbeatMaxAge = 2*watchPollInterval + 5*time.Second
	// watchMaxLifetime caps how long a watcher stays alive.
	watchMaxLifetime = 48 * time.Hour
	// hookTimeout caps how long a hook command may run.
	hookTimeout = 10 * time.Second
	// noWatcherEnv disables starting the background watcher (tests, CI).
	noWatcherEnv = "FLOW_NO_WATCHER"
)

// heartbeatPath returns the heartbeat file of kind ("tui" or "watcher") for
// the current database, so separate databases never share an owner. It is
// empty when no database path is set.
func heartbeatPath(kind string) string {
	if dbPath == "" {
		return ""
	}
	return dbPath + "." + kind
}

// touchHeartbeat writes the current PID to path, refreshing its mtime.
func touchHeartbeat(path string) {
	if path == "" {
		return
	}
	_ = os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0600)
}

// heartbeatFresh reports whether path was refreshed within maxAge.
func heartbeatFresh(path string, maxAge time.Duration) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && time.Since(info.ModTime()) < maxAge
}

// tuiAttached reports whether a timer TUI is showing this database.
func tuiAttached() bool {
	return heartbeatFresh(heartbeatPath("tui"), tuiHeartbeatMaxAge)
}

// spawnWatcher starts the detached background watcher. It is a variable so
// tests can replace it.
var spawnWatcher = func() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// #nosec G204 -- re-executes this binary with fixed arguments.
	c := exec.Command(exe, "--db", dbPath, "__watch")
	c.Stdin, c.Stdout, c.Stderr = nil, nil, nil
	detach(c)
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}

// ensureWatcher starts the background watcher unless one is already alive or
// watchers are disabled. Failures are reported on stderr and are not fatal:
// the session itself is already saved.
func ensureWatcher() {
	if os.Getenv(noWatcherEnv) != "" || dbPath == "" {
		return
	}
	if heartbeatFresh(heartbeatPath("watcher"), watcherHeartbeatMaxAge) {
		return
	}
	if err := spawnWatcher(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.T("Warning: could not start the background timer, you will not be notified when the session ends: %v", err))
	}
}

// watchCmd is the hidden background watcher started by ensureWatcher.
var watchCmd = &cobra.Command{
	Use:    "__watch",
	Short:  "Background watcher that announces the end of the active session",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := setupSignalHandler()
		hb := heartbeatPath("watcher")
		touchHeartbeat(hb)
		defer func() { _ = os.Remove(hb) }()
		watchActiveSessions(ctx, hb, time.Sleep)
		return nil
	},
}

// watchActiveSessions loops until no session is active (or ctx ends),
// completing and announcing each session whose timer runs out. sleep is
// injectable for tests.
//
// Any state read (flow status, the Claude Code status line, MCP
// get_current_state) also completes an expired session, possibly before the
// watcher wakes up. So the watcher remembers the session it is waiting on and
// when that session is due; if it disappears completed at or after that time
// it ran out (a manual stop ends it earlier and announces itself), and the
// watcher announces it all the same.
func watchActiveSessions(ctx context.Context, hb string, sleep func(time.Duration)) {
	var w sessionWait
	deadline := time.Now().Add(watchMaxLifetime)
	for ctx.Err() == nil && time.Now().Before(deadline) {
		touchHeartbeat(hb)

		session, err := app.storage.Sessions().FindActive(ctx)
		if err != nil {
			// Transient (e.g. the database is busy): try again later.
			sleep(watchPollInterval)
			continue
		}
		if w.id != "" && (session == nil || session.ID != w.id) {
			w.announceIfRanOut(ctx)
		}
		if session == nil {
			return
		}
		w.track(session)
		if session.Status != domain.SessionStatusRunning {
			sleep(watchPollInterval)
			continue
		}
		if remaining := session.RemainingTime(); remaining > 0 {
			sleep(min(remaining, watchPollInterval))
			continue
		}

		done, err := app.pomodoro.CompleteIfExpired(ctx)
		if err != nil {
			sleep(watchPollInterval)
			continue
		}
		if done != nil {
			w = sessionWait{}
			announceSessionEnd(ctx, done, done.Duration)
		}
	}
}

// sessionWait is the session the watcher is waiting on and when it is due.
type sessionWait struct {
	id  string
	due time.Time // zero while the session is paused: it cannot run out
}

// track records s as the session being waited on.
func (w *sessionWait) track(s *domain.PomodoroSession) {
	w.id = s.ID
	w.due = time.Time{}
	if s.Status == domain.SessionStatusRunning {
		w.due = time.Now().Add(s.RemainingTime())
	}
}

// dueTolerance absorbs rounding between successive computations of the due
// time of the same session.
const dueTolerance = 500 * time.Millisecond

// announceIfRanOut announces the waited-on session if another process
// completed it because its timer ran out, then forgets it.
func (w *sessionWait) announceIfRanOut(ctx context.Context) {
	id, due := w.id, w.due
	*w = sessionWait{}
	if due.IsZero() {
		return
	}
	s, err := app.storage.Sessions().FindByID(ctx, id)
	if err != nil || s.Status != domain.SessionStatusCompleted || s.CompletedAt == nil {
		return
	}
	if s.CompletedAt.Before(due.Add(-dueTolerance)) {
		return
	}
	announceSessionEnd(ctx, s, s.Duration)
}

// announceSessionEnd sends the end-of-session desktop notification and runs
// the on_end hook, unless an open TUI is showing the session: it announces
// the end itself. worked is the time actually worked.
func announceSessionEnd(ctx context.Context, s *domain.PomodoroSession, worked time.Duration) {
	if tuiAttached() {
		return
	}
	notifySessionEnd(s.Type, s.Methodology, worked)
	_ = runHook(ctx, "end", s)
}

// notifySessionEnd sends the desktop notification for a session of the given
// type that ended after worked.
func notifySessionEnd(t domain.SessionType, m domain.Methodology, worked time.Duration) {
	if app.notifier == nil || !app.notifier.IsEnabled() {
		return
	}
	if m == "" {
		m = app.methodology
	}
	var err error
	switch t {
	case domain.SessionTypeWork:
		err = app.notifier.NotifyWorkComplete(m.Label(), formatMinutes(worked))
	case domain.SessionTypeShortBreak:
		err = app.notifier.NotifyBreakComplete("Short")
	case domain.SessionTypeLongBreak:
		err = app.notifier.NotifyBreakComplete("Long")
	}
	if err != nil && !tuiAttached() {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.T("Warning: notification failed: %v", err))
	}
}

// runHook runs the configured hook command for event ("start" or "end")
// with the session described in FLOW_* environment variables. Output is
// discarded (it would corrupt the TUI or the MCP stream). An unset hook is a
// no-op.
func runHook(ctx context.Context, event string, s *domain.PomodoroSession) error {
	if app.config == nil || s == nil {
		return nil
	}
	command := app.config.Hooks.OnStart
	if event == "end" {
		command = app.config.Hooks.OnEnd
	}
	if command == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	shell, flag := "sh", "-c"
	if runtime.GOOS == "windows" {
		shell, flag = "cmd", "/C"
	}
	// #nosec G204 -- the command comes from the user's own config file.
	c := exec.CommandContext(ctx, shell, flag, command)
	c.Env = append(os.Environ(), hookEnv(ctx, event, s)...)
	return c.Run()
}

// hookEnv describes the session for a hook command.
func hookEnv(ctx context.Context, event string, s *domain.PomodoroSession) []string {
	m := s.Methodology
	if m == "" {
		m = domain.MethodologyPomodoro
	}
	env := []string{
		"FLOW_EVENT=" + event,
		"FLOW_SESSION_ID=" + s.ID,
		"FLOW_SESSION_TYPE=" + string(s.Type),
		"FLOW_METHODOLOGY=" + string(m),
		"FLOW_DURATION=" + s.Duration.String(),
		"FLOW_TASK=",
	}
	if s.TaskID != nil && app.tasks != nil {
		if task, err := app.tasks.GetTask(ctx, *s.TaskID); err == nil {
			env[len(env)-1] = "FLOW_TASK=" + task.Title
		}
	}
	return env
}

// sessionStarted runs after a session or break starts outside the TUI's own
// start path: it runs the on_start hook and, when no TUI will show the
// session, makes sure the background watcher announces its end.
func sessionStarted(ctx context.Context, s *domain.PomodoroSession, background bool) {
	_ = runHook(ctx, "start", s)
	if background {
		ensureWatcher()
	}
}

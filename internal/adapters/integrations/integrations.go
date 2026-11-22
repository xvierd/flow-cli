// Package integrations implements Flow's built-in recipes that react to
// session start and end (Do Not Disturb, Slack status, music) or show the
// session in a status bar (tmux, SketchyBar, Waybar, xbar).
//
// They run next to the user's own [hooks], never instead of them, and follow
// the same rules: short timeouts, errors swallowed, never blocking a session
// from starting or ending. Every external effect goes through Env (command
// runner, HTTP client, file system root) so tests never run osascript or call
// Slack.
package integrations

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/xvierd/flow-cli/internal/config"
)

// Runner runs an external command and returns its combined output.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner runs commands with os/exec.
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	// #nosec G204 -- integrations run fixed programs (shortcuts, osascript,
	// gsettings, ...) with arguments built from the user's own config.
	c := exec.CommandContext(ctx, name, args...)
	c.Stdin = nil
	return c.CombinedOutput()
}

// HTTPDoer sends HTTP requests (an *http.Client in production).
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Env is everything an integration may touch. Tests fill it with fakes.
type Env struct {
	Runner   Runner
	LookPath func(file string) (string, error)
	Getenv   func(key string) string
	GOOS     string
	HTTP     HTTPDoer
	// SlackAPI is the Slack Web API base URL (no trailing slash).
	SlackAPI string
	// Home is the user's home directory; config files and plugins are
	// written under it.
	Home string
	// StateDir holds integrations.json, which remembers what a start did so
	// the matching end (often in another process) undoes exactly that.
	StateDir string
	// FlowBin is the absolute path of the flow binary, embedded in scripts
	// that status bars run with a minimal PATH.
	FlowBin string
	// DBPath, when set, is passed to flow in generated scripts.
	DBPath string
	Now    func() time.Time
	Config config.IntegrationsConfig
	// Timeout, when positive, replaces each integration's own timeout.
	Timeout time.Duration
}

func (e Env) timeoutFor(h SessionHandler) time.Duration {
	if e.Timeout > 0 {
		return e.Timeout
	}
	return h.Timeout()
}

// DefaultEnv returns an Env backed by the real system.
func DefaultEnv(cfg config.IntegrationsConfig, stateDir, dbPath string) Env {
	home, _ := os.UserHomeDir()
	bin, err := os.Executable()
	if err != nil {
		bin = "flow"
	} else if resolved, err := filepath.EvalSymlinks(bin); err == nil {
		bin = resolved
	}
	return Env{
		Runner:   ExecRunner{},
		LookPath: exec.LookPath,
		Getenv:   os.Getenv,
		GOOS:     runtime.GOOS,
		HTTP:     &http.Client{Timeout: SlackTimeout},
		SlackAPI: "https://slack.com/api",
		Home:     home,
		StateDir: stateDir,
		FlowBin:  bin,
		DBPath:   dbPath,
		Now:      time.Now,
		Config:   cfg,
	}
}

func (e Env) has(cmd string) bool {
	if e.LookPath == nil {
		return false
	}
	_, err := e.LookPath(cmd)
	return err == nil
}

func (e Env) now() time.Time {
	if e.Now == nil {
		return time.Now()
	}
	return e.Now()
}

// Event describes a session start or end.
type Event struct {
	// Kind is "start" or "end".
	Kind string
	// SessionType is "work", "short_break" or "long_break".
	SessionType string
	StartedAt   time.Time
	Duration    time.Duration
	Task        string
}

// Ends returns when the session is due to end.
func (ev Event) Ends() time.Time { return ev.StartedAt.Add(ev.Duration) }

// Status is the readiness of an integration on this machine.
type Status struct {
	// Supported is false when the integration cannot work on this OS or
	// with the tools installed.
	Supported bool
	// Ready is true when nothing else needs setting up.
	Ready bool
	// Detail explains what is missing (English source text, translated by
	// the caller). Args are its printf arguments.
	Detail string
	Args   []any
}

// Integration is one built-in recipe.
type Integration interface {
	Name() string
	// Description is English source text for i18n.T.
	Description() string
	Enabled(cfg config.IntegrationsConfig) bool
	SetEnabled(cfg *config.IntegrationsConfig, on bool)
	Check(ctx context.Context, env Env) Status
	// Plan returns the files to change and notes to show for enable (on)
	// or disable (!on).
	Plan(ctx context.Context, env Env, on bool) (Plan, error)
}

// SessionHandler is implemented by integrations that act on start/end.
type SessionHandler interface {
	Start(ctx context.Context, env Env, st *State, ev Event) error
	End(ctx context.Context, env Env, st *State, ev Event) error
	// Timeout bounds one Start or End call.
	Timeout() time.Duration
}

// All returns every integration, in display order.
func All() []Integration {
	return []Integration{dnd{}, slack{}, music{}, tmux{}, sketchybar(), waybar(), xbar()}
}

// Find returns the integration called name.
func Find(name string) (Integration, bool) {
	for _, in := range All() {
		if in.Name() == name {
			return in, true
		}
	}
	return nil, false
}

// Names returns every integration name, sorted.
func Names() []string {
	var names []string
	for _, in := range All() {
		names = append(names, in.Name())
	}
	sort.Strings(names)
	return names
}

// Dispatch runs Start or End (per ev.Kind) of every enabled integration in
// parallel, each bounded by its own timeout, and waits for all of them.
// Errors are collected per integration and never stop the others; callers
// normally ignore them.
//
// Start only acts on work sessions. End undoes whatever the recorded state
// says a start did, whatever the session type, so it is safe to call after a
// void or cancel too.
func Dispatch(ctx context.Context, env Env, ev Event) map[string]error {
	if ev.Kind == "start" && ev.SessionType != "work" {
		return nil
	}
	var handlers []Integration
	for _, in := range All() {
		if _, ok := in.(SessionHandler); ok && in.Enabled(env.Config) {
			handlers = append(handlers, in)
		}
	}
	if len(handlers) == 0 {
		return nil
	}

	st := LoadState(env.StateDir)
	errs := make(map[string]error)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, in := range handlers {
		h := in.(SessionHandler)
		wg.Add(1)
		go func(name string, h SessionHandler) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, env.timeoutFor(h))
			defer cancel()
			done := make(chan error, 1)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						done <- errPanic
					}
				}()
				if ev.Kind == "start" {
					done <- h.Start(cctx, env, st, ev)
				} else {
					done <- h.End(cctx, env, st, ev)
				}
			}()
			var err error
			select {
			case err = <-done:
			case <-cctx.Done():
				// A handler ignoring ctx must not hold the session up.
				err = cctx.Err()
			}
			if err != nil {
				mu.Lock()
				errs[name] = err
				mu.Unlock()
			}
		}(in.Name(), h)
	}
	wg.Wait()
	_ = st.Save()
	return errs
}

// Undo runs the End of one integration now, whatever its enabled flag, so
// disabling it mid-session turns DND off, clears the Slack status or resumes
// the music it paused. Integrations without session actions are a no-op.
func Undo(ctx context.Context, env Env, in Integration) error {
	h, ok := in.(SessionHandler)
	if !ok {
		return nil
	}
	st := LoadState(env.StateDir)
	cctx, cancel := context.WithTimeout(ctx, env.timeoutFor(h))
	defer cancel()
	err := h.End(cctx, env, st, Event{Kind: "end"})
	if saveErr := st.Save(); err == nil {
		err = saveErr
	}
	return err
}

package integrations

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/xvierd/flow-cli/internal/config"
)

// dnd turns on Do Not Disturb while a work session runs.
//
//   - macOS: there is no supported command-line switch for Focus; the
//     reliable way is two Shortcuts that use the built-in "Set Focus" action,
//     run with `shortcuts run`.
//   - Linux: `dunstctl set-paused` (dunst) or the GNOME
//     `org.gnome.desktop.notifications show-banners` key.
type dnd struct{}

func (dnd) Name() string { return "dnd" }
func (dnd) Description() string {
	return "Do Not Disturb / Focus during work sessions"
}
func (dnd) Enabled(c config.IntegrationsConfig) bool { return c.DND }
func (dnd) SetEnabled(c *config.IntegrationsConfig, on bool) {
	c.DND = on
	if c.DNDOnShortcut == "" {
		c.DNDOnShortcut = config.DefaultDNDOnShortcut
	}
	if c.DNDOffShortcut == "" {
		c.DNDOffShortcut = config.DefaultDNDOffShortcut
	}
}
func (dnd) Timeout() time.Duration { return 5 * time.Second }

// dndBackend is how DND is toggled on this machine.
type dndBackend int

const (
	dndNone dndBackend = iota
	dndShortcuts
	dndDunst
	dndGnome
)

func dndBackendFor(env Env) dndBackend {
	switch env.GOOS {
	case "darwin":
		if env.has("shortcuts") {
			return dndShortcuts
		}
	case "linux", "freebsd", "openbsd", "netbsd":
		if env.has("dunstctl") {
			return dndDunst
		}
		if env.has("gsettings") {
			return dndGnome
		}
	}
	return dndNone
}

func shortcutNames(env Env) (on, off string) {
	on, off = env.Config.DNDOnShortcut, env.Config.DNDOffShortcut
	if on == "" {
		on = config.DefaultDNDOnShortcut
	}
	if off == "" {
		off = config.DefaultDNDOffShortcut
	}
	return on, off
}

// missingShortcuts returns the configured shortcut names `shortcuts list`
// does not show.
func missingShortcuts(ctx context.Context, env Env) ([]string, error) {
	out, err := env.Runner.Run(ctx, "shortcuts", "list")
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		have[strings.TrimSpace(line)] = true
	}
	on, off := shortcutNames(env)
	var missing []string
	for _, name := range []string{on, off} {
		if !have[name] {
			missing = append(missing, name)
		}
	}
	return missing, nil
}

func (d dnd) Check(ctx context.Context, env Env) Status {
	switch dndBackendFor(env) {
	case dndShortcuts:
		missing, err := missingShortcuts(ctx, env)
		if err != nil {
			return Status{Supported: true, Detail: "could not list Shortcuts: %v", Args: []any{err}}
		}
		if len(missing) > 0 {
			return Status{Supported: true, Detail: "missing Shortcuts: %s", Args: []any{strings.Join(missing, ", ")}}
		}
		return Status{Supported: true, Ready: true, Detail: "via Shortcuts"}
	case dndDunst:
		return Status{Supported: true, Ready: true, Detail: "via dunstctl"}
	case dndGnome:
		return Status{Supported: true, Ready: true, Detail: "via gsettings (GNOME)"}
	}
	if env.GOOS == "darwin" {
		return Status{Detail: "the shortcuts command is not available (macOS 12 or later is required)"}
	}
	return Status{Detail: "no supported notification daemon found (needs dunstctl or gsettings)"}
}

func (d dnd) Plan(ctx context.Context, env Env, on bool) (Plan, error) {
	var p Plan
	if !on {
		return p, nil
	}
	if dndBackendFor(env) != dndShortcuts {
		return p, nil
	}
	missing, err := missingShortcuts(ctx, env)
	if err != nil || len(missing) == 0 {
		return p, nil
	}
	onName, offName := shortcutNames(env)
	p.note("macOS has no command to toggle Focus, so flow runs two Shortcuts. Create them once in the Shortcuts app:")
	p.note("  1. New shortcut named \"%s\" → add the action \"Set Focus\" → \"Turn Do Not Disturb On\" (until turned off).", onName)
	p.note("  2. New shortcut named \"%s\" → add \"Set Focus\" → \"Turn Do Not Disturb Off\".", offName)
	p.note("  Open the Shortcuts app with: open -a Shortcuts. Check with: flow integrate list")
	p.note("Other shortcut names can be set in config.toml: [integrations] dnd_on_shortcut / dnd_off_shortcut.")
	return p, nil
}

func (d dnd) set(ctx context.Context, env Env, on bool) error {
	switch dndBackendFor(env) {
	case dndShortcuts:
		onName, offName := shortcutNames(env)
		name := offName
		if on {
			name = onName
		}
		_, err := env.Runner.Run(ctx, "shortcuts", "run", name)
		return err
	case dndDunst:
		_, err := env.Runner.Run(ctx, "dunstctl", "set-paused", boolWord(on))
		return err
	case dndGnome:
		// show-banners is the inverse of Do Not Disturb.
		_, err := env.Runner.Run(ctx, "gsettings", "set", "org.gnome.desktop.notifications", "show-banners", boolWord(!on))
		return err
	}
	return errors.New("do not disturb is not supported on this system")
}

func boolWord(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func (d dnd) Start(ctx context.Context, env Env, st *State, _ Event) error {
	// Always run "on" (it is idempotent): a recorded DND=true may be stale
	// after an MCP void or a crash while the user turned Focus off by hand.
	if err := d.set(ctx, env, true); err != nil {
		return err
	}
	st.update(func(s *stateData) { s.DND = true })
	return nil
}

func (d dnd) End(ctx context.Context, env Env, st *State, _ Event) error {
	if !st.get().DND {
		return nil // flow did not turn it on: leave the user's setting alone
	}
	err := d.set(ctx, env, false)
	st.update(func(s *stateData) { s.DND = false })
	return err
}

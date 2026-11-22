package integrations

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/xvierd/flow-cli/internal/config"
)

// musicPlayers are the macOS players music pauses, by AppleScript name.
var musicPlayers = []string{"Spotify", "Music"}

// music pauses Spotify / Music when a work session starts and resumes only
// the players it paused when the session ends. A player that is not running
// is never launched.
type music struct{}

func (music) Name() string { return "music" }
func (music) Description() string {
	return "Pause Spotify / Music during work sessions (macOS)"
}
func (music) Enabled(c config.IntegrationsConfig) bool         { return c.Music }
func (music) SetEnabled(c *config.IntegrationsConfig, on bool) { c.Music = on }
func (music) Timeout() time.Duration                           { return 4 * time.Second }

func (music) Check(_ context.Context, env Env) Status {
	if env.GOOS != "darwin" {
		return Status{Detail: "only available on macOS"}
	}
	if !env.has("osascript") {
		return Status{Detail: "osascript not found"}
	}
	return Status{Supported: true, Ready: true, Detail: "Spotify, Music (macOS may ask once to allow automation)"}
}

func (music) Plan(_ context.Context, env Env, on bool) (Plan, error) {
	var p Plan
	if on && env.GOOS == "darwin" {
		p.note("The first time a player is paused, macOS asks to let your terminal (or flow) control it: allow it in that prompt, or later in System Settings → Privacy & Security → Automation.")
	}
	return p, nil
}

// pauseScript pauses app if it is running and playing, printing "paused".
// The `is running` guard keeps osascript from launching the app.
func pauseScript(app string) string {
	return fmt.Sprintf(`if application %q is running then
	tell application %q
		if player state is playing then
			pause
			return "paused"
		end if
	end tell
end if
return ""`, app, app)
}

func playScript(app string) string {
	return fmt.Sprintf(`if application %q is running then
	tell application %q to play
end if`, app, app)
}

func (m music) Start(ctx context.Context, env Env, st *State, _ Event) error {
	if env.GOOS != "darwin" {
		return nil
	}
	var firstErr error
	for _, app := range musicPlayers {
		out, err := env.Runner.Run(ctx, "osascript", "-e", pauseScript(app))
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if strings.TrimSpace(string(out)) == "paused" {
			st.update(func(d *stateData) {
				if !slices.Contains(d.Music, app) {
					d.Music = append(d.Music, app)
				}
			})
		}
	}
	return firstErr
}

func (m music) End(ctx context.Context, env Env, st *State, _ Event) error {
	paused := st.get().Music
	if len(paused) == 0 {
		return nil
	}
	var firstErr error
	for _, app := range paused {
		if !slices.Contains(musicPlayers, app) {
			continue
		}
		if _, err := env.Runner.Run(ctx, "osascript", "-e", playScript(app)); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	st.update(func(d *stateData) { d.Music = nil })
	return firstErr
}

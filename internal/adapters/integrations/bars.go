package integrations

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/xvierd/flow-cli/internal/config"
)

// bar is a status-bar plugin: a script that reads `flow status --json`
// (with jq when available, falling back to `flow tmux-status`) and prints it
// in the bar's format.
type bar struct {
	name, desc string
	// path returns where the script is written.
	path func(env Env) string
	// body renders the script.
	body func(env Env) string
	// notes are extra setup steps shown after writing the script.
	notes   func(env Env, p *Plan, path string)
	enabled func(c config.IntegrationsConfig) bool
	set     func(c *config.IntegrationsConfig, on bool)
	// tool is the bar's own command, used to report whether it is installed
	// ("" when it cannot be detected from PATH).
	tool string
	// goos limits the bar to one OS ("" for any).
	goos []string
}

func (b bar) Name() string                                     { return b.name }
func (b bar) Description() string                              { return b.desc }
func (b bar) Enabled(c config.IntegrationsConfig) bool         { return b.enabled(c) }
func (b bar) SetEnabled(c *config.IntegrationsConfig, on bool) { b.set(c, on) }

func (b bar) Check(_ context.Context, env Env) Status {
	if len(b.goos) > 0 && !containsStr(b.goos, env.GOOS) {
		return Status{Detail: "not available on %s", Args: []any{env.GOOS}}
	}
	path := b.path(env)
	if exists, owned := ownedByFlow(path); !exists || !owned {
		return Status{Supported: true, Detail: "plugin not installed (%s)", Args: []any{path}}
	}
	if b.tool != "" && !env.has(b.tool) {
		return Status{Supported: true, Ready: true, Detail: "plugin installed, but %s is not in PATH", Args: []any{b.tool}}
	}
	return Status{Supported: true, Ready: true, Detail: "%s", Args: []any{path}}
}

func (b bar) Plan(_ context.Context, env Env, on bool) (Plan, error) {
	var p Plan
	path := b.path(env)
	if !on {
		if c, ok := removeOwned(path); ok {
			p.Changes = append(p.Changes, c)
		}
		return p, nil
	}
	p.Changes = append(p.Changes, writeOwned(path, b.body(env), 0755))
	if b.notes != nil {
		b.notes(env, &p, path)
	}
	return p, nil
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// scriptHeader starts every plugin script: a known PATH (bars start scripts
// with a minimal one) and the status read into $text / $type / $task.
func scriptHeader(env Env) string {
	return `#!/bin/sh
# Flow timer plugin — ` + Marker + `.
# Re-create with "flow integrate enable <bar>", remove with "flow integrate disable <bar>".
PATH="/usr/bin:/bin:/usr/local/bin:/opt/homebrew/bin:$PATH"
FLOW() { ` + flowCommand(env, `"$@"`) + ` 2>/dev/null; }

text="" type="idle" task=""
if command -v jq >/dev/null 2>&1 && json=$(FLOW status --json); then
  type=$(printf '%s' "$json" | jq -r '.active_session.type // "idle"')
  status=$(printf '%s' "$json" | jq -r '.active_session.status // ""')
  task=$(printf '%s' "$json" | jq -r '.active_task.title // ""')
  # remaining_time is a Go duration ("1h2m3.5s", "12m0s", "800ms").
  secs=$(printf '%s' "$json" | jq -r '
    (.active_session.remaining_time // "0s") as $d
    | if ($d | test("^[0-9.]+(ms|us|µs|ns)$")) then 0
      else ($d | capture("^((?<h>[0-9]+)h)?((?<m>[0-9]+)m)?((?<s>[0-9.]+)s)?$")
            | ((.h // "0" | tonumber) * 3600 + (.m // "0" | tonumber) * 60 + (.s // "0" | tonumber)) | floor)
      end' 2>/dev/null)
  secs=${secs:-0}
  if [ "$type" != "idle" ]; then
    icon="🍅"
    [ "$type" = "work" ] || icon="☕"
    [ "$status" = "paused" ] && icon="⏸"
    h=$((secs / 3600)); m=$(((secs / 60) % 60)); s=$((secs % 60))
    if [ "$h" -gt 0 ]; then clock=$(printf '%d:%02d:%02d' "$h" "$m" "$s"); else clock=$(printf '%02d:%02d' "$m" "$s"); fi
    text="$icon $clock"
  fi
else
  text=$(FLOW tmux-status)
  [ -n "$text" ] && type="work"
fi
`
}

func sketchybar() bar {
	return bar{
		name: "sketchybar",
		desc: "SketchyBar plugin reading flow status --json",
		tool: "sketchybar",
		goos: []string{"darwin"},
		path: func(env Env) string { return filepath.Join(env.Home, ".config", "sketchybar", "plugins", "flow.sh") },
		body: func(env Env) string {
			return scriptHeader(env) + `
if [ -n "$text" ]; then
  sketchybar --set "${NAME:-flow}" drawing=on label="$text"
else
  sketchybar --set "${NAME:-flow}" drawing=off
fi
`
		},
		notes: func(_ Env, p *Plan, path string) {
			p.note("Add the item to your sketchybarrc, then run: sketchybar --reload")
			p.note("  sketchybar --add item flow right --set flow update_freq=5 script=\"%s\"", path)
		},
		enabled: func(c config.IntegrationsConfig) bool { return c.Sketchybar },
		set:     func(c *config.IntegrationsConfig, on bool) { c.Sketchybar = on },
	}
}

func waybar() bar {
	return bar{
		name: "waybar",
		desc: "Waybar custom module reading flow status --json",
		tool: "waybar",
		path: func(env Env) string { return filepath.Join(env.Home, ".config", "waybar", "scripts", "flow.sh") },
		body: func(env Env) string {
			return scriptHeader(env) + `
esc() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }
tooltip="$task"
[ -n "$tooltip" ] || tooltip="flow"
printf '{"text":"%s","tooltip":"%s","class":"%s","alt":"%s"}\n' "$(esc "$text")" "$(esc "$tooltip")" "$type" "$type"
`
		},
		notes: func(_ Env, p *Plan, path string) {
			p.note("Add the module to your Waybar config (and \"custom/flow\" to modules-left/right), then reload Waybar:")
			p.note("  \"custom/flow\": { \"exec\": \"%s\", \"return-type\": \"json\", \"interval\": 5 }", path)
		},
		enabled: func(c config.IntegrationsConfig) bool { return c.Waybar },
		set:     func(c *config.IntegrationsConfig, on bool) { c.Waybar = on },
	}
}

func xbar() bar {
	return bar{
		name: "xbar",
		desc: "xbar / SwiftBar menu-bar plugin reading flow status --json",
		goos: []string{"darwin"},
		path: func(env Env) string {
			return filepath.Join(env.Home, "Library", "Application Support", "xbar", "plugins", "flow.5s.sh")
		},
		body: func(env Env) string {
			return scriptHeader(env) + `
if [ -n "$text" ]; then echo "$text"; else echo "🍅"; fi
echo "---"
[ -n "$task" ] && echo "$task"
echo "Open flow | shell=` + strings.ReplaceAll(env.FlowBin, " ", `\ `) + ` terminal=true"
`
		},
		notes: func(_ Env, p *Plan, path string) {
			p.note("xbar picks the plugin up from its plugins folder (refresh from the xbar menu).")
			p.note("Using SwiftBar instead? Copy or link %s into your SwiftBar plugin folder.", path)
		},
		enabled: func(c config.IntegrationsConfig) bool { return c.Xbar },
		set:     func(c *config.IntegrationsConfig, on bool) { c.Xbar = on },
	}
}

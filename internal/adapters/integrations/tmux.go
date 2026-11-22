package integrations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
)

// StatusLine is the short text `flow tmux-status` prints: "🍅 12:34" for a
// work session, "☕ 04:10" for a break, "⏸ 12:34" when paused, and "" when
// no session is active (so an idle status bar shows nothing).
func StatusLine(s *domain.PomodoroSession, workIcon string) string {
	if s == nil {
		return ""
	}
	if workIcon == "" {
		workIcon = "🍅"
	}
	icon := workIcon
	if !s.IsWorkSession() {
		icon = "☕"
	}
	if s.Status == domain.SessionStatusPaused {
		icon = "⏸"
	}
	return icon + " " + clock(s.RemainingTime())
}

// clock formats d as MM:SS, or H:MM:SS from an hour up.
func clock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Round(time.Second) / time.Second)
	h, m, sec := total/3600, (total/60)%60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%02d:%02d", m, sec)
}

// flowCommand is the shell command that runs flow from generated config,
// with --db when a non-default database is in use.
func flowCommand(env Env, args string) string {
	bin := env.FlowBin
	if bin == "" {
		bin = "flow"
	}
	cmd := shellQuote(bin)
	if env.DBPath != "" {
		cmd += " --db " + shellQuote(env.DBPath)
	}
	return cmd + " " + args
}

// shellQuote quotes s for sh when it holds anything but safe characters.
func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && !strings.ContainsRune("/._-+:@", r)
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// tmux appends `flow tmux-status` to the tmux status-right, in a marked
// block of ~/.tmux.conf that disable removes.
type tmux struct{}

func (tmux) Name() string                                     { return "tmux" }
func (tmux) Description() string                              { return "Timer in the tmux status bar (flow tmux-status)" }
func (tmux) Enabled(c config.IntegrationsConfig) bool         { return c.Tmux }
func (tmux) SetEnabled(c *config.IntegrationsConfig, on bool) { c.Tmux = on }

// tmuxConfPath is ~/.tmux.conf, or the XDG config when that is the one in
// use (creating ~/.tmux.conf next to it could shadow it).
func tmuxConfPath(env Env) string {
	classic := filepath.Join(env.Home, ".tmux.conf")
	if _, err := os.Stat(classic); err == nil {
		return classic
	}
	xdgHome := ""
	if env.Getenv != nil {
		xdgHome = env.Getenv("XDG_CONFIG_HOME")
	}
	if xdgHome == "" {
		xdgHome = filepath.Join(env.Home, ".config")
	}
	if xdg := filepath.Join(xdgHome, "tmux", "tmux.conf"); fileExists(xdg) {
		return xdg
	}
	return classic
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (tmux) Check(_ context.Context, env Env) Status {
	if !env.has("tmux") {
		return Status{Supported: true, Detail: "tmux not found in PATH"}
	}
	return Status{Supported: true, Ready: true, Detail: "%s", Args: []any{tmuxConfPath(env)}}
}

// tmuxBlock is the snippet added to ~/.tmux.conf. status-interval makes the
// countdown tick; tmux shows only the first line of #() output.
func tmuxBlock(env Env) string {
	// Inside a tmux string #() runs through sh; single quotes in the
	// command survive inside tmux's double-quoted string.
	return "set -g status-interval 5\n" +
		"set -ga status-right \" #(" + strings.ReplaceAll(flowCommand(env, "tmux-status"), `"`, `\"`) + ")\""
}

func (t tmux) Plan(_ context.Context, env Env, on bool) (Plan, error) {
	var p Plan
	path := tmuxConfPath(env)
	if on {
		p.Changes = append(p.Changes, Change{Kind: UpsertBlock, Path: path, Block: "tmux", Content: tmuxBlock(env), Confirm: true})
		p.note("Reload tmux to see it: tmux source-file %s", path)
		p.note("Prefer to edit it yourself? Add this to %s:", path)
		for _, line := range strings.Split(tmuxBlock(env), "\n") {
			p.note("  %s", line)
		}
		return p, nil
	}
	p.Changes = append(p.Changes, Change{Kind: RemoveBlock, Path: path, Block: "tmux", Confirm: true})
	p.note("Restart the tmux server (tmux kill-server) or reset status-right for the change to take effect.")
	return p, nil
}

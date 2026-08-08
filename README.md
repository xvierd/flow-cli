# Flow

> The AI-native focus CLI — Pomodoro, Deep Work & Make Time from your terminal, with an MCP server your AI agents can drive.

![CI](https://img.shields.io/github/actions/workflow/status/xvierd/flow-cli/ci.yml?branch=main)
![Go](https://img.shields.io/github/go-version/xvierd/flow-cli)
![Release](https://img.shields.io/github/v/release/xvierd/flow-cli?sort=semver)
![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/xvierd/flow-cli)](https://goreportcard.com/report/github.com/xvierd/flow-cli)
![MCP Server](https://img.shields.io/badge/MCP%20Server-compatible-8A2BE2)

Flow is a productivity CLI that gets out of your way. Built in Go with an interactive TUI, git awareness, and a full MCP server that lets Claude Code, Cursor, and other agents read and write your focus state. Supports three focus methodologies: **Pomodoro**, **Deep Work**, and **Make Time**.

```
$ flow

  Flow:
  > Start session
    View stats
    Reflect
    Report

  Mode:
  > Pomodoro    Classic 25/5 timer
    Deep Work   Longer sessions, distraction tracking
    Make Time   Daily Highlight, focus scoring

  What are you working on? (Enter to skip): Write API docs #coding

  Starting 25m session...
```

```
       🍅 Flow - Pomodoro Timer

       📋 Task: Write API docs
       Status: Work Session (Running)
              21:34
       ██░░░░░░░░░░░░░░░░░░░░░░░░░░
       🌿 main (a5e7d58)

       📊 Today: 3 work sessions, 1 break, 1h15m worked

       [s]tart [p]ause [x] stop [c]ancel [b]reak [q]uit
```

## Why Flow

Most productivity timers are either too heavy (Electron apps with accounts, cloud sync, and onboarding flows) or too trivial (a `sleep` script in your shell). Flow sits in between:

- **Flow stays in the terminal.** No new window, no login, no telemetry. Just a TUI that tracks your tasks, knows which git branch you're on, and persists everything to a local SQLite database.
- **Flow structures your focus, not just your time.** Three methodologies with real ceremonies — Deep Work shutdown ritual, Make Time highlight + focus score, Pomodoro break cycles.
- **Flow is AI-native.** Agents can *see* and *change* your state through the MCP server: your Claude Code status line shows the live timer, and any MCP-capable agent can start a session, log a distraction, or read your weekly report without leaving the conversation.

## Install

```bash
# Go
go install github.com/xvierd/flow-cli/cmd/flow@latest

# Script (installs to ~/.local/bin)
curl -sSL https://raw.githubusercontent.com/xvierd/flow-cli/main/install.sh | sh

# From source
git clone https://github.com/xvierd/flow-cli.git
cd flow-cli && go build -o flow ./cmd/flow
```

Uninstall with `curl -sSL https://raw.githubusercontent.com/xvierd/flow-cli/main/uninstall.sh | sh`.

## Quick Start

```bash
flow                        # interactive wizard (main menu, task, session)
flow add "Fix auth bug"     # create a task
flow start                 # start a session for the active task
flow status                 # current session + today's stats
flow stats                  # productivity dashboard
flow reflect                # weekly reflection
flow report --week          # aggregated weekly report (terminal / md / csv / json)
flow report --week -fmd -o report.md
flow break                   # take a break
flow complete <id>           # mark a task done
flow mcp                     # start the MCP server
```

Add `#tags` inline when naming a task to tag the session:

```
What are you working on? Fix login bug #backend #urgent
```

## Methodology Modes

Flow ships three rigorously separated methodologies. Pick one from the main menu or set a default with `--mode` / `methodology` in config.

| Mode | Description | Session Presets |
|------|-------------|-----------------|
| **Pomodoro** | Classic 25/5 timer with a long break every 4 sessions | Focus (25m), Short (15m), Deep (50m) |
| **Deep Work** | Longer sessions, distraction log, intended outcome, 4-step shutdown ritual | Deep (90m), Focus (50m), Shallow (25m) |
| **Make Time** | Daily Highlight, laser checklist, focus scoring, energize reminders | Highlight (60m), Sprint (25m), Quick (15m) |

```toml
methodology = "deepwork"   # pomodoro, deepwork, or maketime
```

## Commands

| Command | What it does |
|---------|-------------|
| `flow` | Interactive wizard: main menu, mode picker, task, duration, start |
| `flow add "title" [-t tag1,tag2]` | Create a new task |
| `flow list [--all] [--status pending]` | List tasks |
| `flow start [task-id]` | Start a session (`--task`, `--tags` flags also work) |
| `flow stop` | Complete the active session (Deep Work shutdown ritual) |
| `flow pause` / `flow resume` | Pause / resume the active session |
| `flow break` | Start a short or long break |
| `flow void` | Void a session due to interruption |
| `flow status [--json]` | Current session + today's stats |
| `flow stats [--period week|month]` | Productivity dashboard: sessions by mode, focus scores, heatmap |
| `flow reflect [--today]` | Weekly reflection: day-by-day breakdown, highlights, focus |
| `flow report [--week\|--month] [--format terminal\|md\|csv] [--out file] [--json]` | Aggregated report: daily breakdown, 24h heatmap, top tags, methodology focus |
| `flow export [--format md|csv] [--period week|month|all]` | Raw session history dump |
| `flow complete <id>` | Mark a task as completed |
| `flow delete <id>` | Delete a task |
| `flow config` | Configure presets, breaks, methodology, notifications |
| `flow reset [--force]` | Wipe the database |
| `flow mcp` | Start the MCP server |

### Global Flags

| Flag | Description |
|------|-------------|
| `--mode <pomodoro\|deepwork\|maketime>` | Methodology for this session (or default) |
| `--strict` | Enforce strict focus mode (overrides `[focus] strict` in config) |
| `--inline`, `-i` | Compact inline timer (narrow terminals / non-TTY) |
| `--json` | Machine-readable output |
| `--db <path>` | Custom database path |

## Session Chaining

When a session completes, Flow shows a "What next?" menu instead of exiting. Chain sessions without leaving the terminal:

```
  Session complete!
  Today: 4 work sessions, 2 breaks, 2h10m worked

  [n]ew session  [b]reak  [q]uit
```

- **`n`** — start a new session (last methodology is pre-selected; your last task is the first option)
- **`b`** — start a break (only after work sessions)
- **`q`** — quit the timer

`[n]` is unlocked only after the mode's ceremony is done: immediately in Pomodoro, after the 4-step shutdown ritual + distraction/outcome review in Deep Work, and after the focus score + energize log in Make Time.

## Deep Work Shutdown Ritual

Deep Work sessions end with a 4-step closing ritual — the same sequence the book prescribes:

1. **Review pending tasks** — anything urgent left over?
2. **Review tomorrow's calendar** — any conflicts?
3. **Plan for tomorrow** — write tomorrow's plan
4. **Closing phrase** — e.g. *"Shutdown complete"*

Plus outcome review (`did you achieve your intended outcome?`) and a distraction review when applicable.

## Focus Mode (Strict)

Flow is distraction-free by default; **strict focus mode** makes it enforced, at the service layer — the CLI, TUI, and MCP all respect the same rules:

```toml
[focus]
strict = true          # default false
```

Or enable it per command with the global `--strict` flag, e.g. `flow --strict start` (this overrides the config).

In strict mode, while a work session is active:

- **Pausing, finishing (stopping), voiding, and cancelling are locked** — the session must run to completion. CLI commands (`flow stop`, `flow pause`, `flow void`) and MCP tools (`stop_pomodoro`, `pause_pomodoro`, `void_session`, `cancel_session`) fail with a descriptive "strict focus mode" error.
- **Break sessions are unaffected** — you can still pause, finish, or skip a break.
- The TUI shows a `🔒 STRICT` badge and locked keys (`p`/`f`/`v`/`b`, plus `m` inline) explain themselves instead of acting.

To leave strict mode, set `[focus] strict = false` in the config (or drop the `--strict` flag).

## TUI Key Bindings

| Key | Action | Context |
|-----|--------|---------|
| `s` | Start session / skip break | Idle / break |
| `p` | Pause / resume | Work or break |
| `f` | Finish (stop) session | Work / break |
| `x` | Finish session | Work (alias) |
| `v` | Void session | Work |
| `c` | Cancel / close session | Work / break |
| `b`, `m` | Take a break | Work / inline |
| `q` | Quit | Idle / completion |
| `d` | Log a distraction (`,n` next) | Deep Work |
| `r` | Review distractions | Deep Work completion |
| `a` | Shutdown ritual | Deep Work completion |
| `o` | Outcome review | Deep Work completion |
| `1`–`5` | Rate focus score | Make Time |
| `w`/`t`/`e`/`n` | Energize: walk / stretch / exercise / none | Make Time |
| `tab` | Toggle notifications on/off | Any session |
| `n` | New session | Completion screen |
| `esc` | Back / cancel prompt | Any menu |
| `ctrl+c` | Quit | Any |

## Claude Code Integration

### Status Line

See your timer in Claude Code's status bar:

```
[Opus 4.6] 12% ctx | 🍅 18:52 ███░░ Write API docs
```

```bash
cp scripts/claude-statusline.sh ~/.claude/flow-statusline.sh
```

Then add to `~/.claude/settings.json`:

```json
{
  "statusLine": {
    "type": "command",
    "command": "~/.claude/flow-statusline.sh"
  }
}
```

### MCP Server

Let AI assistants read and act on your focus state. Add to your editor's MCP config:

```json
{
  "mcpServers": {
    "flow": {
      "command": "flow",
      "args": ["mcp"]
    }
  }
}
```

Works with Claude Code, Cursor, and any MCP-compatible client.

**Sessions** — `start_session` (methodology, task, duration, tags, intended outcome), `start_break`, `stop_pomodoro`, `pause_pomodoro`, `resume_pomodoro`, `cancel_session`, `void_session`.

**Reviews & insights** — `get_current_state`, `get_recent_sessions`, `get_daily_summary`, `get_period_stats`, `get_focus_report`, `get_today_highlight`.

**Tasks** — `get_task`, `list_tasks`, `get_task_history`, `create_task`, `complete_task`, `delete_task`, `start_task`.

**Methodology ceremonies** — `set_highlight`, `log_distraction`, `set_focus_score`, `set_accomplishment`, `set_shutdown_ritual`, `set_energize_activity`, `set_outcome_achieved`, `add_session_notes`.

Example: an agent asks you to log a distraction:

```
> tell the agent to log a distraction
log_distraction(session_id="8f2a…", text="Phone rings", category="external")
→ {"session_id": "8f2a…", "logged": true}
```

## Configuration

Flow stores config at `~/.flow/config.toml` and data at `~/.flow/flow.db`.

```toml
methodology = "pomodoro"  # default mode: pomodoro, deepwork, maketime
first_run = true

[focus]
strict = false           # strict focus mode (see above)

[pomodoro]
work_duration = "25m"
short_break = "5m"
long_break = "15m"
sessions_before_long = 4
auto_break = false
preset1_name = "Focus"
preset1_duration = "25m0s"
preset2_name = "Short"
preset2_duration = "15m0s"
preset3_name = "Deep"
preset3_duration = "50m0s"

[deepwork]
deep_work_goal_hours = 4.0
break_duration = "20m0s"
philosophy = "journalistic"           # journalistic, bimodal, monastic
preset1_name = "Deep"
preset1_duration = "1h30m0s"
preset2_name = "Focus"
preset2_duration = "50m0s"
preset3_name = "Shallow"
preset3_duration = "25m0s"

[maketime]
break_duration = "15m0s"
highlight_target_minutes = 60
preset1_name = "Highlight"
preset1_duration = "1h0m0s"
preset2_name = "Sprint"
preset2_duration = "25m0s"
preset3_name = "Quick"
preset3_duration = "15m0s"

[notifications]
enabled = true
sound = true

[theme]
color_work = "#7C6FE0"
color_break = "#4ECDC4"
icon_app = "🍅"
```

## Architecture

Hexagonal architecture with a clean separation between business logic and external concerns.

```
internal/
├── domain/       # Entities: Task, Session, State, Report
├── ports/        # Interfaces: Storage, Timer, GitDetector, MCP
├── services/     # Use cases: TaskService, PomodoroService, StateService, ReportService
└── adapters/     # Implementations
    ├── storage/  # SQLite
    ├── tui/      # Bubbletea
    ├── mcp/      # MCP server
    ├── git/      # Git context detection
    └── notification/
```

## Development

```bash
go test -race ./...   # tests with race detector
go vet ./...          # static analysis
golangci-lint run     # linters (v2 config)
go build -o flow ./cmd/flow
```

Install the pre-commit hook (runs `gofmt` + `go vet`):

```bash
git config core.hooksPath .githooks
```

> 🎬 Suggested demo (manual): record `flow`, `flow report --week`, and an MCP session with [asciinema](https://asciinema.org) or [vhs](https://github.com/charmbracelet/vhs).

## GitHub Topics

`productivity` `pomodoro` `cli` `tui` `golang` `mcp` `mcp-server` `deep-work` `make-time` `ai-agents` `terminal` `focus`

## License

MIT — see [LICENSE](LICENSE) for the full text.
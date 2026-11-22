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

  Duration:
  > Focus       25m
    Short       15m
    Deep        50m

  What are you working on? (Enter to skip): Write API docs #coding
```

```
       🍅 Flow - Pomodoro Timer

       📋 Task: Write API docs
       Status: Work Session (Running)
              21:34
       ██░░░░░░░░░░░░░░░░░░░░░░░░░░
       🌿 main (a5e7d58)

       📊 Today: 3 work sessions, 1 break, 1h15m worked

       [p]ause  [f]inish  [v]oid  [b]reak  [c]lose  tab:notify on
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

# Script (Linux / macOS, installs to ~/.local/bin)
curl -sSL https://raw.githubusercontent.com/xvierd/flow-cli/main/install.sh | sh

# Windows: download flow-windows-amd64.zip (or arm64) from the latest release
# https://github.com/xvierd/flow-cli/releases/latest and put flow.exe on your PATH

# From source
git clone https://github.com/xvierd/flow-cli.git
cd flow-cli && go build -o flow ./cmd/flow
```

Uninstall with `curl -sSL https://raw.githubusercontent.com/xvierd/flow-cli/main/uninstall.sh | sh`.

## Quick Start

Start focusing with one command; there's nothing to set up first:

```bash
flow go                     # start now: repeats your last session, or a 25-minute Pomodoro
flow 25 "fix auth #backend" # 25 minutes on a task, tagged #backend (a bare number is minutes)
flow 90m "write the RFC"    # 90m, 1h, 1h30m work too (1 minute to 8 hours)
flow                        # wizard: Enter on "Start now" repeats your last session
```

`flow go` reuses the last work session's methodology, duration, task and tags; a session you stopped early is repeated at its planned length, not the minutes you worked. `flow 25 "title"` reuses an open task with that title instead of creating a duplicate. Both open the timer, or with `--detach` / `-d` (or without a terminal) run in the background like `flow start`. On a fresh install `flow` opens a one-screen welcome that can start a 25-minute session straight away.

More commands:

```bash
flow add "Fix auth bug"     # create a task (prints its short ID, e.g. 4f894d3e)
flow start 4f894d3e         # start a work session on that task and open the timer
flow start --detach         # start without the timer: runs in the background, notifies at the end
flow status                 # one-shot view of the current session + today's stats
flow stats                  # productivity dashboard
flow reflect                # weekly reflection
flow report --week          # aggregated weekly report (terminal / md / csv / json)
flow report --week -fmd -o report.md
flow break                   # take a break
flow complete 4f894d3e       # mark a task done
flow edit 4f894d3e --tags docs,backend
flow mcp                     # start the MCP server
```

Task and session IDs can be shortened: the 8 characters `flow list` shows (or any unique prefix of at least 4) work everywhere a full ID does, in the CLI and the MCP tools.

Add `#tags` inline when naming a task to tag the session:

```
What are you working on? Fix login bug #backend #urgent
```

## Methodology Modes

Flow ships three rigorously separated methodologies. **Pomodoro is the default**; Deep Work and Make Time are advanced modes. The wizard goes straight to the duration picker in your configured mode, with the length you used last preselected; press `m` there to switch mode. Set the default with `--mode` / `methodology` in config.

| Mode | Description | Session Presets |
|------|-------------|-----------------|
| **Pomodoro** | Classic 25/5 timer with a long break every 4 sessions | Focus (25m), Short (15m), Deep (50m) |
| **Deep Work** | Longer sessions, distraction log, intended outcome, 4-step shutdown ritual | Deep (90m), Focus (50m), Shallow (25m) |
| **Make Time** | Daily Highlight, laser checklist, focus scoring, energize reminders | Highlight (60m), Sprint (25m), Quick (15m) |

```toml
methodology = "deepwork"   # pomodoro, deepwork, or maketime
```

Breaks follow the active mode (`methodology` in config, or `--mode`). In Pomodoro a break taken after every 4th work session of the day is long, and any other break (including one taken before any work that day) is short; Deep Work and Make Time use their own `break_duration`.

## Commands

| Command | What it does |
|---------|-------------|
| `flow` | Interactive wizard: main menu (**Start now** repeats your last session), then duration (last one preselected, `m` changes mode) and task (offers Resume / Stop if a session is active; a one-screen welcome on an empty database) |
| `flow go [-d]` | Start now: repeat the last work session (methodology, duration, task, tags), or the default 25-minute Pomodoro |
| `flow <duration> [task…] [-d]` | Start directly: `flow 25 "fix auth #backend"`, `flow 90m deep dive`. A bare number is minutes; `#words` become tags; an open task with the same title is reused |
| `flow add "title" [-t tag1,tag2]` | Create a new task (`-t` is repeatable or comma-separated: `-t a,b -t c`) |
| `flow list [--all] [--status pending]` | List tasks |
| `flow start [task-id]` | Start a work session and open the timer (`--task <id>`, `--tags a,b` also work; `--detach` / `-d` skips the timer). Length: `work_duration` in Pomodoro, the first preset in Deep Work / Make Time |
| `flow stop` | Complete the active session, recording the time actually worked; then runs the mode's prompts (notes / shutdown ritual / focus score) |
| `flow pause` / `flow resume` | Pause / resume the active session |
| `flow break [--detach]` | Start a short or long break and open the timer |
| `flow void` | Void a session due to interruption (not counted in stats) |
| `flow status [--json]` | One-shot view of the current session + today's stats (`active_task` is `null` when no session is active) |
| `flow stats [--period week|month]` | Productivity dashboard: sessions by mode, focus scores, heatmap |
| `flow reflect [--today]` | Weekly reflection: day-by-day breakdown, highlights, focus |
| `flow report [--week\|--month] [--format terminal\|md\|csv] [--out file] [--json]` | Aggregated report: daily breakdown, 24h heatmap, top tags, methodology focus. In `--json`, each duration comes as a Go duration string (`"total_work_time": "1h15m0s"`) plus a number of minutes (`"total_work_minutes": 75`); heatmap `minutes` is a number |
| `flow export [--format md|csv] [--period week|month|all]` | Raw session history dump |
| `flow complete <id>` | Mark a task as completed |
| `flow edit <id> [--title t] [--description d] [--tags a,b] [--reopen]` | Edit a task; `--tags` replaces all tags (`--tags ""` clears them), `--reopen` moves a completed task back to pending |
| `flow delete <id>` | Delete a task |
| `flow config` | Configure presets, breaks, methodology, notifications |
| `flow reset [--force]` | Wipe the database |
| `flow mcp` | Start the MCP server |

Without a terminal (scripts, editor tasks, `flow start | …`) `flow start` and `flow break` behave as with `--detach`: the session is saved, the command exits 0, and the background timer notifies you when it ends.

Only one session (running or paused) can be active at a time; `flow start` offers to stop the current one first. A running session whose timer has already run out is completed automatically and never blocks a new start. Stopping records the time actually worked (paused time is not counted), and when a session ends (stopped, voided, cancelled or run out, from the CLI, TUI or MCP) its in-progress task returns to pending.

### Global Flags

| Flag | Description |
|------|-------------|
| `--mode <pomodoro\|deepwork\|maketime>` | Methodology for this session (or default) |
| `--strict` | Enforce strict focus mode (overrides `[focus] strict` in config) |
| `--inline`, `-i` | Compact inline timer (narrow terminals / non-TTY) |
| `--json` | Machine-readable output |
| `--db <path>` | Custom database path |

## Notifications & Background Timer

You get a desktop notification when a session ends, whether or not the timer is on screen:

- While the timer TUI is open, it announces the end of the session itself.
- When nothing is showing the session — you closed the timer with `c`, started it with `--detach` or from a script, or an AI agent started it over MCP — a small detached `flow` process waits for it to run out, completes it and notifies you. It exits as soon as no session is active (or after 48 h), never shows a window, and only one runs per database.

Turn notifications off with `[notifications] enabled = false` (or `tab` in the timer). Set `FLOW_NO_WATCHER=1` to never start the background process.

## Hooks

Run your own commands when sessions start and end — pause music, set Slack to "focusing", log to a time tracker:

```toml
[hooks]
on_start = "osascript -e 'tell application \"Music\" to pause'"
on_end   = "echo \"$FLOW_EVENT $FLOW_SESSION_TYPE $FLOW_DURATION $FLOW_TASK\" >> ~/flow-hooks.log"
```

Commands run through `sh -c` (`cmd /C` on Windows) with a 10 s timeout and their output discarded. They receive `FLOW_EVENT` (`start` / `end`), `FLOW_SESSION_ID`, `FLOW_SESSION_TYPE` (`work`, `short_break`, `long_break`), `FLOW_METHODOLOGY`, `FLOW_DURATION` and `FLOW_TASK`. `on_start` fires for sessions started from the CLI, the timer or MCP; `on_end` fires when a session completes (timer ran out or stopped), not when it is voided or cancelled.

## Session Chaining

When a session completes, Flow shows a "What next?" menu instead of exiting. Chain sessions without leaving the terminal:

```
  Session complete!
  Today: 4 work sessions, 2 breaks, 2h10m worked

  [n]ew session  [b]reak  [q]uit
```

- **`n`** — set up a new session in the same methodology (duration, then task; recent tasks are offered first). Works whether the timer was opened by `flow`, `flow start`, `flow break` or Resume
- **`s`** — start right away, repeating the last session (same duration, task and tags) when it belongs to the current methodology; otherwise the mode's first preset
- **`b`** — start a break (only after work sessions)
- **`q`** — quit the timer

`[n]` and `[s]` are unlocked only after the mode's ceremony is done: immediately in Pomodoro, after the 4-step shutdown ritual (or accomplishment) + outcome/distraction review in Deep Work, and after the focus score + energize log in Make Time. With `auto_break = true`, the automatic break (a 3-second countdown, cancelled by any key) also waits for those prompts.

Resuming an active session adopts that session's methodology, whatever the configured default. Voiding (`v`) returns the timer to idle: the session is not counted and no completion notification is sent.

In Make Time, quitting the fullscreen timer started from `flow` asks for your **Highlight for tomorrow**, which is saved as the next day's Highlight. If yesterday's Highlight is still open, the fullscreen wizard offers to carry it forward once per run (inline mode lists it as a "Carry forward" option).

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

- **Pausing, finishing (stopping), voiding, and cancelling are locked** until the timer reaches zero — the session must run to completion. CLI commands (`flow stop`, `flow pause`, `flow void`) and MCP tools (`stop_pomodoro`, `pause_pomodoro`, `void_session`, `cancel_session`) fail with a descriptive "strict focus mode" error. A session that is already paused can still be resumed.
- **Break sessions are unaffected** — you can still pause, finish, or skip a break.
- The TUI shows a `🔒 STRICT` badge and locked keys (`p`/`f`/`v`/`b`, plus `m` inline) explain themselves instead of acting. `c` (close the view) is never locked; the session keeps running.

To leave strict mode, set `[focus] strict = false` in the config (or drop the `--strict` flag).

## TUI Key Bindings

| Key | Action | Context |
|-----|--------|---------|
| `s` | Start session (repeats the last one) / skip break and start working | Idle, completion / break |
| `p` | Pause / resume | Work or break |
| `f` | Finish (stop) session — press twice to confirm | Work / break |
| `v` | Void session and return to idle (not counted, no notification) | Work |
| `c` | Close the timer view (the session keeps running) | Idle / work / break |
| `b` | Take a break (mid-session: press twice; stops the work session first) | Work / completion |
| `m` | Switch mode (cancels an active session after a second `m`) | Inline only |
| `q` | Quit | Completion screen |
| `d` | Log a distraction: type it, `enter`, then `i` internal / `e` external (`enter` skips) | Deep Work |
| `r` | Review distractions | Deep Work completion |
| `a` | Shutdown ritual | Deep Work completion |
| `o` | Outcome review | Deep Work completion |
| `1`–`5` | Rate focus score | Make Time completion |
| `w`/`t`/`e`/`n` | Energize: walk / stretch / exercise / none | Make Time completion |
| `tab` | Toggle notifications on/off | Any session |
| `n` | New session | Completion screen |
| `esc` | Back / cancel prompt or confirmation | Any menu |
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

- The server speaks JSON-RPC over stdio: stdout carries only protocol messages, and the startup banner and diagnostics go to stderr.
- `duration_minutes` must be a whole number from 1 to 1440. Omit it for the methodology's default: `work_duration` in Pomodoro, the first preset in Deep Work / Make Time. `start_session` without `methodology` uses the configured one.
- Invalid arguments and storage failures come back as tool errors (`isError: true`); a panic inside a tool is reported the same way instead of killing the server.
- Tool calls are serialized, and the CLI, TUI and MCP server can all use the same database at once.
- Sessions an agent starts or resumes are covered by the background timer, so you are notified when they end even with no Flow window open; `on_start` / `on_end` hooks run for them too.
- `task_id` and `session_id` arguments accept the short IDs shown by `flow list`.

**Sessions** — `start_session` (`methodology`, `task_id` or `task_title`, `duration_minutes`, `tags`, `intended_outcome`), `start_break`, `stop_pomodoro`, `pause_pomodoro`, `resume_pomodoro`, `cancel_session`, `void_session`, and `start_pomodoro` (deprecated alias of `start_session`).

**Reviews & insights** — `get_current_state`, `get_recent_sessions`, `get_daily_summary`, `get_period_stats`, `get_focus_report`, `get_today_highlight`.

**Tasks** — `get_task`, `list_tasks`, `get_task_history`, `create_task`, `complete_task`, `delete_task`, `start_task`.

**Methodology ceremonies** — `set_highlight`, `log_distraction`, `set_focus_score`, `set_accomplishment`, `set_shutdown_ritual`, `set_energize_activity`, `set_outcome_achieved`, `add_session_notes`.

Example: an agent asks you to log a distraction:

```
> tell the agent to log a distraction
log_distraction(session_id="8f2a…", text="Phone rings", category="external")
→ {"session_id": "8f2a…", "logged": "Phone rings", "category": "external"}
```

## Configuration

Flow stores config at `~/.flow/config.toml` (created with defaults on first run) and data at `<storage.data_dir>/flow.db` (`~/.flow/flow.db` by default; `--db` overrides it).

If the config file cannot be parsed, Flow prints a warning on stderr and runs with the defaults without touching the file. Until you fix or remove it, `flow config` refuses to save, and settings changed elsewhere (such as the TUI notification toggle) are not persisted.

The database schema migrates automatically on startup (current version: 3). A database written by a newer flow is rejected with a message asking you to upgrade. The database uses WAL and a busy timeout, so the TUI, the MCP server and CLI commands can share it at the same time.

```toml
methodology = "pomodoro"  # default mode: pomodoro, deepwork, maketime
language = "en"           # UI language: en, es (FLOW_LANG env var overrides this)
first_run = true

[focus]
strict = false           # strict focus mode (see above)

[hooks]
on_start = ""            # shell command run when a session starts (see Hooks)
on_end = ""              # shell command run when a session completes

[pomodoro]
work_duration = "25m0s"   # default length of Pomodoro sessions (`flow start`, MCP)
short_break = "5m0s"
long_break = "15m0s"
sessions_before_long = 4  # Pomodoro mode always uses 4
auto_break = false        # start the break automatically after a work session
preset1_name = "Focus"
preset1_duration = "25m0s"
preset2_name = "Short"
preset2_duration = "15m0s"
preset3_name = "Deep"
preset3_duration = "50m0s"

[deepwork]
deep_work_goal_hours = 4.0
break_duration = "20m0s"
philosophy = "journalistic"           # rhythmic, bimodal, journalistic, monastic (empty: asked on first Deep Work run)
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

[storage]
data_dir = "~/.flow"     # "~" is expanded to your home directory

[theme]
color_work = "#7C6FE0"
color_break = "#4ECDC4"
icon_app = "🍅"
```

## Internationalization

The CLI and TUI are available in English and Spanish. Flow picks the language from the `FLOW_LANG` environment variable first, then the `language` key in `config.toml`, defaulting to English:

```bash
FLOW_LANG=es flow          # one-off
flow config                # or set it permanently ([l] Cambiar idioma)
```

Command help text (`--help`, cobra metadata) stays in English.

## Architecture

Hexagonal architecture with a clean separation between business logic and external concerns.

```
internal/
├── domain/       # Entities: Task, Session, State, Report
├── ports/        # Interfaces: Storage, Timer, GitDetector, MCP
├── services/     # Use cases: TaskService, PomodoroService, StateService, ReportService
├── methodology/  # Pomodoro / Deep Work / Make Time mode rules and presets
├── config/       # ~/.flow/config.toml loading and saving
├── i18n/         # en/es message catalogs
└── adapters/     # Implementations
    ├── storage/  # SQLite (versioned migrations, transactions, normalized tags)
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

The README demo is recorded with [vhs](https://github.com/charmbracelet/vhs) from [`docs/demo.tape`](docs/demo.tape) on a throwaway database: `go build -o flow ./cmd/flow && vhs docs/demo.tape`.

## License

MIT — see [LICENSE](LICENSE) for the full text.
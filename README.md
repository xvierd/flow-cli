# Flow

A productivity CLI that gets out of your way. Built in Go with an interactive TUI, git awareness, and AI assistant integration. Supports three focus methodologies: Pomodoro, Deep Work, and Make Time.

```
$ flow

  Flow:
  > Start session
    View stats
    Reflect

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

       📊 Today: 3 work sessions, 1 breaks, 1h15m worked

       [s]tart [p]ause [x] stop [c]ancel [b]reak [q]uit
```

## Why

Most pomodoro apps are either too heavy (Electron apps with accounts and syncing) or too simple (a bash timer). Flow sits in between: it tracks your tasks, knows what git branch you're on, and integrates with AI coding assistants - all from the terminal.

## Install

```bash
# Homebrew (macOS)
brew tap xvierd/tap
brew install flow

# Go
go install github.com/xvierd/flow-cli/cmd/flow@latest

# Script
curl -sSL https://raw.githubusercontent.com/xvierd/flow-cli/main/install.sh | sh

# From source
git clone https://github.com/xvierd/flow-cli.git
cd flow-cli && go build -o flow ./cmd/flow
```

## Uninstall

```bash
curl -sSL https://raw.githubusercontent.com/xvierd/flow-cli/main/uninstall.sh | sh

# Or if installed via Homebrew
brew uninstall flow
```

## Methodology Modes

Flow supports three productivity methodologies. Pick one from the main menu or set a default with `--mode`.

| Mode | Description | Session Presets |
|------|-------------|-----------------|
| **Pomodoro** | Classic 25/5 timer with long break every 4 sessions | Focus (25m), Short (15m), Deep (50m) |
| **Deep Work** | Longer sessions with distraction logging and shutdown ritual | Deep (90m), Focus (50m), Shallow (25m) |
| **Make Time** | Daily Highlight, focus scoring, and energize reminders | Highlight (60m), Sprint (25m), Quick (15m) |

Set a default mode in config:

```toml
methodology = "deepwork"   # pomodoro, deepwork, or maketime
```

Or pass it per-session:

```bash
flow --mode deepwork
```

## Quick Start

```bash
# Just type flow - the interactive wizard handles the rest
flow

# Or use commands directly
flow add "Fix auth bug"        # create a task
flow start abc123              # start a pomodoro for that task
flow status                    # check current state
flow stats                     # view productivity dashboard
flow reflect                   # weekly reflection
flow break                     # take a break
flow complete abc123           # mark task done
```

## Commands

| Command | What it does |
|---------|-------------|
| `flow` | Interactive wizard - main menu, mode picker, task, duration, start |
| `flow add "title"` | Create a new task |
| `flow list` | List tasks (`--all`, `--status pending`) |
| `flow start [task-id]` | Start a pomodoro (`--task` flag also works) |
| `flow status` | Show current session and daily stats |
| `flow stats` | Productivity dashboard: sessions by mode, focus scores, hourly heatmap |
| `flow reflect` | Weekly reflection: day-by-day breakdown, highlights, energize vs focus |
| `flow break` | Start a short or long break |
| `flow pause` | Pause the active session |
| `flow resume` | Resume a paused session |
| `flow stop` | Complete the current session |
| `flow complete <id>` | Mark a task as completed |
| `flow mcp` | Start the MCP server |

### Global Flags

| Flag | Description |
|------|-------------|
| `--mode <mode>` | Set methodology for this session: `pomodoro`, `deepwork`, `maketime` |
| `--inline`, `-i` | Compact inline timer (no fullscreen TUI) |
| `--json` | Output results in JSON format |
| `--db <path>` | Custom database path |

## Session Chaining

When a session completes, Flow shows a "What next?" menu instead of exiting. Chain sessions without leaving the terminal:

```
  Session complete!
  Today: 4 work sessions, 2 breaks, 2h10m worked

  [n]ew session  [b]reak  [q]uit
```

- **`n`** -- start a new session (re-runs the full wizard: mode, task, duration)
- **`b`** -- start a break (only after work sessions)
- **`q`** -- quit the timer

The `[n]` option appears after all mode-specific prompts are done: immediately in Pomodoro, after the shutdown ritual in Deep Work, and after focus score + energize log in Make Time.

## Session Tagging

Add `#tags` inline when entering a task name. Tags are stored with the session for filtering and stats.

```
What are you working on? Fix login bug #backend #urgent
```

## TUI Key Bindings

| Key | Action | Modes |
|-----|--------|-------|
| `s` | Start session | All |
| `p` | Pause / Resume | All |
| `b` | Start break | All |
| `c` | Cancel session | All |
| `x` | Stop session | All |
| `q` | Quit | All |
| `n` | New session (on completion screen) | All |
| `d` | Log a distraction | Deep Work |
| `a` | Record accomplishment (shutdown ritual) | Deep Work |
| `r` | Review distractions (after accomplishment) | Deep Work |
| `1`-`5` | Rate focus score | Make Time |
| `w/t/e/n` | Log energize activity (walk/stretch/exercise/none) | Make Time |

## Claude Code Integration

### Status Line

See your pomodoro timer in Claude Code's status bar:

```
[Opus 4.6] 12% ctx | 🍅 18:32 ███░░ Write API docs
```

Setup:

```bash
cp scripts/claude-statusline.sh ~/.claude/flow-statusline.sh
```

Add to `~/.claude/settings.json`:

```json
{
  "statusLine": {
    "type": "command",
    "command": "~/.claude/flow-statusline.sh"
  }
}
```

### MCP Server

Let AI assistants read your flow state. Add to your editor's MCP config:

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

Available tools: `get_current_state`, `list_tasks`, `get_task_history`, `start_pomodoro`, `stop_pomodoro`, `pause_pomodoro`, `resume_pomodoro`, `create_task`, `complete_task`, `add_session_notes`.

## Configuration

Flow stores config at `~/.flow/config.toml` and data at `~/.flow/flow.db`.

```toml
methodology = "pomodoro"  # default mode: pomodoro, deepwork, maketime

[pomodoro]
work_duration = "25m"
short_break = "5m"
long_break = "15m"
sessions_before_long = 4
auto_break = false        # automatically start break after work session ends

[notifications]
enabled = true
sound = true
```

## Architecture

Hexagonal architecture with clean separation between business logic and external concerns.

```
internal/
├── domain/       # Entities: Task, PomodoroSession, State
├── ports/        # Interfaces: Storage, Timer, GitDetector, MCP
├── services/     # Use cases: TaskService, PomodoroService, StateService
└── adapters/     # Implementations
    ├── storage/  # SQLite
    ├── tui/      # Bubbletea
    ├── mcp/      # MCP server
    ├── git/      # Git context detection
    └── notification/
```

## Development

```bash
go test ./...        # run tests
go vet ./...         # lint
go build -o flow .   # build
```

### Git hooks

Install the pre-commit hook (runs `gofmt` + `go vet` before every commit):

```bash
git config core.hooksPath .githooks
```

## License

MIT

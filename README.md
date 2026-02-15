# Flow

A Pomodoro CLI that gets out of your way. Built in Go with an interactive TUI, git awareness, and AI assistant integration.

```
$ flow

  What are you working on? (Enter to skip): Write API docs
  Duration? [25m]:

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

## Quick Start

```bash
# Just type flow - the wizard handles the rest
flow

# Or use commands directly
flow add "Fix auth bug"        # create a task
flow start abc123              # start a pomodoro for that task
flow status                    # check current state
flow break                     # take a break
flow complete abc123           # mark task done
```

## Commands

| Command | What it does |
|---------|-------------|
| `flow` | Interactive wizard - ask task name, duration, start |
| `flow add "title"` | Create a new task |
| `flow list` | List tasks (`--all`, `--status pending`) |
| `flow start [task-id]` | Start a pomodoro (`--task` flag also works) |
| `flow status` | Show current session and daily stats |
| `flow break` | Start a short or long break |
| `flow pause` | Pause the active session |
| `flow resume` | Resume a paused session |
| `flow stop` | Complete the current session |
| `flow complete <id>` | Mark a task as completed |
| `flow mcp` | Start the MCP server |

## TUI Shortcuts

| Key | Action |
|-----|--------|
| `s` | Start session |
| `p` | Pause / Resume |
| `b` | Start break |
| `c` | Cancel session |
| `x` | Stop session |
| `q` | Quit TUI (session keeps running) |

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
[pomodoro]
work_duration = "25m"
short_break = "5m"
long_break = "15m"
sessions_before_long = 4

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

## License

MIT

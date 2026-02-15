# Flow

Flow is a Pomodoro CLI timer with task tracking, interactive TUI, and MCP (Model Context Protocol) server support, built in Go.

## Features

- **Task Tracking**: Manage tasks with statuses and metadata
- **Pomodoro Timer**: Work sessions with integrated timer
- **Interactive TUI**: Visual interface using Bubbletea with progress bar animation
- **MCP Server**: Model Context Protocol integration for AI assistants
- **Git Integration**: Automatic detection of git context (branch, commit, modified files)
- **SQLite Storage**: Lightweight local persistence

## Architecture

The project follows hexagonal architecture (clean architecture) principles:

```
flow/
├── cmd/                    # CLI commands (cobra)
│   ├── add.go             # Add task command
│   ├── list.go            # List tasks command
│   ├── start.go           # Start pomodoro command
│   ├── status.go          # Show status command
│   ├── break.go           # Start break command
│   ├── mcp.go             # MCP server command
│   └── ...
├── internal/
│   ├── domain/            # Business entities (Task, PomodoroSession)
│   ├── ports/             # Interfaces (Storage, Timer, MCP, Git)
│   ├── services/          # Use cases (TaskService, PomodoroService, StateService)
│   └── adapters/          # Implementations
│       ├── storage/       # SQLite repository implementation
│       ├── tui/           # Bubbletea TUI implementation
│       ├── mcp/           # MCP server implementation
│       └── git/           # Git context detector
└── main.go                # Application entry point
```

## Installation

### Quick Install

```bash
curl -sSL https://raw.githubusercontent.com/xvierd/flow-cli/main/install.sh | sh
```

This downloads the latest release binary and installs it to `/usr/local/bin/flow`.

### Using go install

```bash
go install github.com/xvierd/flow-cli/cmd/flow@latest
```

> **Note:** Make sure `$(go env GOPATH)/bin` is in your `PATH`.

### From Source

```bash
git clone https://github.com/xvierd/flow-cli.git
cd flow-cli
go build -o flow ./cmd/flow
./flow --help
```

## Usage

### Quick Start

```bash
# Add a task
flow add "Implement feature X"

# List tasks
flow list

# Start a pomodoro session (without task)
flow start

# Start a pomodoro session with a task
flow start --task <task-id>

# Check current status
flow status

# Start a break
flow break
```

### Commands

#### `flow add "task title"`
Add a new task to the list.

```bash
flow add "Fix bug in authentication"
```

#### `flow list`
List all tasks. Use flags to filter:

```bash
# List all tasks (including completed)
flow list --all

# Filter by status
flow list --status pending
flow list --status in_progress
flow list --status completed
```

#### `flow start [task-id]`
Start a pomodoro work session. Optionally specify a task ID to associate with the session.

```bash
# Start without task
flow start

# Start with specific task
flow start abc123

# Or use the --task flag
flow start --task abc123
```

#### `flow status`
Display the current pomodoro session status and today's statistics.

#### `flow break`
Start a pomodoro break session (short or long depending on work completed).

#### `flow pause`
Pause the currently running pomodoro session.

#### `flow resume`
Resume a paused pomodoro session.

#### `flow stop`
Complete the current pomodoro session.

#### `flow complete [task-id]`
Mark a task as completed.

```bash
flow complete abc123
```

#### `flow mcp`
Start the Model Context Protocol (MCP) server for integration with AI assistants.

```bash
flow mcp
```

### TUI Controls

During a pomodoro or break session, the TUI provides:

- **Visual progress bar**: Animated progress showing session completion
- **Timer display**: Countdown in MM:SS format
- **Git context**: Shows current branch and commit when available
- **Daily stats**: Today's work sessions, breaks, and total work time

**Keyboard shortcuts:**
- `s` - Start (or pause/resume if running)
- `p` - Pause/Resume
- `c` - Cancel current session
- `b` - Start break
- `q` or `Ctrl+C` - Quit

### MCP Server Tools

When running `flow mcp`, the following tools are available:

#### `get_current_state`
Returns the current Flow state including active task, session, and daily stats.

#### `list_tasks`
Lists all tasks, optionally filtered by status.

Parameters:
- `status` (optional): Filter by status (pending, in_progress, completed, cancelled)

#### `get_task_history`
Returns pomodoro session history for a specific task.

Parameters:
- `task_id` (required): The ID of the task

### MCP Configuration

To use Flow with AI assistants, add the MCP server to your configuration:

#### Claude Code

Add to your Claude Code configuration (`~/.claude/config.json`):

**If installed from source:**
```json
{
  "mcpServers": {
    "flow": {
      "command": "/path/to/flow",
      "args": ["mcp"]
    }
  }
}
```

**If installed via `go install`:**
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

Or with full path:
```json
{
  "mcpServers": {
    "flow": {
      "command": "~/go/bin/flow",
      "args": ["mcp"]
    }
  }
}
```

#### Cursor

Add to your Cursor settings (Settings → MCP):

**If installed from source:**
```json
{
  "mcpServers": {
    "flow": {
      "command": "/path/to/flow",
      "args": ["mcp"]
    }
  }
}
```

**If installed via `go install`:**
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

Once configured, you can ask:
- "What task am I working on?"
- "How many pomodoros have I completed today?"
- "List my pending tasks"

### Claude Code Status Line

Show your pomodoro timer directly in Claude Code's status bar:

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

The status line shows: `[Opus 4.6] 42% ctx | 🍅 18:32 ███░░ Implement feature X`

## Configuration

Flow stores its database in `~/.flow/flow.db` by default. You can specify a custom path:

```bash
flow --db /path/to/custom.db add "My task"
```

## Development

### Running Tests

```bash
go test ./...
```

### Project Structure

The codebase follows clean architecture principles:

1. **Domain Layer** (`internal/domain/`): Pure business logic, no external dependencies
2. **Ports Layer** (`internal/ports/`): Interface definitions for adapters
3. **Services Layer** (`internal/services/`): Application use cases, orchestrate domain and ports
4. **Adapters Layer** (`internal/adapters/`): Concrete implementations of external concerns

## License

MIT

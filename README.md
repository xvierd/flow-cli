# Flow

> The AI-native focus CLI — Pomodoro, Deep Work & Make Time from your terminal, with an MCP server your AI agents can drive.

![CI](https://img.shields.io/github/actions/workflow/status/xvierd/flow-cli/ci.yml?branch=main)
![Go](https://img.shields.io/github/go-version/xvierd/flow-cli)
![Release](https://img.shields.io/github/v/release/xvierd/flow-cli?sort=semver)
![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/xvierd/flow-cli)](https://goreportcard.com/report/github.com/xvierd/flow-cli)
![MCP Server](https://img.shields.io/badge/MCP%20Server-compatible-8A2BE2)

![flow demo: wrapped card, yearly heatmap and badge](docs/demo.gif)

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
# Homebrew (macOS / Linux). Use the tap-qualified name: homebrew-core's
# "flow" is a different tool (Facebook's type checker) with the same binary name.
brew install xvierd/tap/flow

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

```bash
flow                        # interactive wizard (main menu, task, session)
flow add "Fix auth bug"     # create a task (prints its short ID, e.g. 4f894d3e)
flow start 4f894d3e         # start a work session on that task and open the timer
flow start --detach         # start without the timer: runs in the background, notifies at the end
flow status                 # one-shot view of the current session + today's stats
flow stats                  # productivity dashboard
flow reflect                # weekly reflection
flow report --week          # aggregated weekly report (terminal / md / csv / json)
flow report --week -fmd -o report.md
flow insights --month       # 3-5 actionable findings from your history, with evidence
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

Flow ships three rigorously separated methodologies. Pick one from the main menu or set a default with `--mode` / `methodology` in config.

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
| `flow` | Interactive wizard: main menu, mode picker, duration, task, start (offers Resume / Stop if a session is active) |
| `flow add "title" [-t tag1,tag2]` | Create a new task (`-t` is repeatable or comma-separated: `-t a,b -t c`) |
| `flow list [--all] [--status pending]` | List tasks |
| `flow start [task-id]` | Start a work session and open the timer (`--task <id>`, `--tags a,b` also work; `--detach` / `-d` skips the timer; without a task it is named after the git branch, see [Git Awareness](#git-awareness)). Length: `work_duration` in Pomodoro, the first preset in Deep Work / Make Time |
| `flow stop` | Complete the active session, recording the time actually worked; then runs the mode's prompts (notes / shutdown ritual / focus score) |
| `flow pause` / `flow resume` | Pause / resume the active session |
| `flow break [--detach]` | Start a short or long break and open the timer |
| `flow void` | Void a session due to interruption (not counted in stats) |
| `flow status [--json]` | One-shot view of the current session + today's stats, and the git summary of the last session (`active_task` is `null` when no session is active) |
| `flow stats [--period week|month]` | Productivity dashboard: sessions by mode, focus scores, heatmap |
| `flow stats --year` | GitHub-style heatmap of daily focus minutes over the last 12 months (53 weeks × 7 days, 5 levels) |
| `flow wrapped [--week\|--month\|--year] [--png file] [--theme dark\|light]` | Shareable summary card: focus hours, sessions, streak, best day and hour, top tag and task, a fun fact; `--png` also saves it as an image |
| `flow badge [--week\|--month\|--year] [-o file] [--label text]` | Shields-style SVG badge such as "focus this week \| 12h" to commit to your README |
| `flow reflect [--today]` | Weekly reflection: day-by-day breakdown, highlights, focus |
| `flow report [--week\|--month] [--format terminal\|md\|csv] [--out file] [--json]` | Aggregated report: daily breakdown, 24h heatmap, top tags, focus by git branch, methodology focus. In `--json`, each duration comes as a Go duration string (`"total_work_time": "1h15m0s"`) plus a number of minutes (`"total_work_minutes": 75`); heatmap `minutes` is a number |
| `flow insights [--week\|--month] [--json]` | Actionable findings from the last 7 (default) or 30 days, ranked by relevance, each with its evidence. See [Insights](#insights) |
| `flow export [--format md|csv] [--period week|month|all]` | Raw session history dump |
| `flow complete <id>` | Mark a task as completed |
| `flow edit <id> [--title t] [--description d] [--tags a,b] [--reopen]` | Edit a task; `--tags` replaces all tags (`--tags ""` clears them), `--reopen` moves a completed task back to pending |
| `flow delete <id>` | Delete a task |
| `flow config` | Configure presets, breaks, methodology, notifications |
| `flow reset [--force]` | Wipe the database |
| `flow mcp` | Start the MCP server |
| `flow claude install [--dry-run] [--statusline]` / `flow claude uninstall [--dry-run]` | Add / remove Flow's focus-aware hooks (and status line) in Claude Code's `settings.json` (see [Claude Code Integration](#claude-code-integration)) |

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

## Share Your Focus

`flow wrapped` turns the current week (default), month (`--month`) or year (`--year`) into a card you can screenshot, and `--png` renders the same card as a 1200×630 image, ready for social previews. The terminal card adapts to light and dark terminals; pick the image palette with `--theme dark|light`:

```bash
flow wrapped                       # this week, in the terminal
flow wrapped --year --png year.png # this year, also saved as an image
```

`flow stats --year` draws a contribution-style heatmap: one column per week (Monday first), one cell per local calendar day, five intensity levels relative to your busiest day. When the output has no colors (piped, `NO_COLOR`) the levels are drawn as `· ░ ▒ ▓ █`.

`flow badge` writes an SVG badge with your focus time. Regenerate it whenever you like and commit it:

```bash
flow badge -o docs/focus.svg                  # "focus this week | 12h"
flow badge --month --label "deep work" -o docs/focus.svg
```

```markdown
![focus this week](docs/focus.svg)
```

All three count only completed work sessions, like `flow report`. `flow wrapped` and `flow stats --year` support `--json`.

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

## Git Awareness

Inside a git repository, Flow ties your focus time to the code you touched. Everything is best-effort: outside a repository, without the `git` binary, or for sessions recorded by older versions, Flow behaves exactly as before.

**Task from the branch.** Start a session without a task and Flow names it after the branch: `feat/auth-refresh` → *auth refresh*, `fix/ABC-123-login-bug` → *login bug* (branch-type prefixes such as `feat/`, `fix/`, `chore/` and ticket IDs such as `ABC-123-` are dropped). In the `flow` wizard it is the editable default of the new task's name (not offered when a recent task already has that name); `flow start` uses it directly, reusing a pending task of the same name instead of creating duplicates (`--no-branch-task` turns this off). Nothing is suggested on `main`, `master`, `develop`, `trunk` or a detached HEAD.

**End-of-session summary.** Each work session records the commit it started on. When it ends (stopped, voided or run out), Flow counts the commits made since then and the lines changed, plus changes still uncommitted when the session has just ended, and stores the summary with the session:

```
  Session complete — 38m worked
  3 commits · internal/auth/ (+120 −40)
```

It appears on the end-of-session screen, in `flow status` (`Last session: 38m · 3 commits · internal/auth/ (+120 −40)`, plus `Git so far` for the running session; `last_session.git_summary` and `active_session.git_summary` in `--json`), in `flow export`, and in the MCP `get_recent_sessions` tool as `git_summary` (`commits`, `files_changed`, `insertions`, `deletions`, `dirs_changed`, `top_dirs`, `uncommitted`) and `git_summary_text`. A session that ended while nothing was watching gets its summary the next time it is listed, counting only commits made before it ended. git runs outside database transactions with a 5 s limit per command, so a slow repository never blocks other Flow processes.

**Focus by branch.** `flow report` and `flow export` add up completed work sessions per branch (`feat/auth-refresh: 4 sessions, 2h50m · 3 commits`); in `flow report --json` this is the `branches` array, and in CSV a `branch,sessions,work_min,commits,insertions,deletions` table. `flow export --format csv` adds `git_branch`, `git_commits`, `git_insertions`, `git_deletions` and `git_top_dir` columns.

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

## Insights

`flow insights` reads your history and answers "what should I change?" with sentences, not totals:

```
  Insights · last 30 days
  2026-10-24 → 2026-11-22 · 64 sessions analyzed
  ────────────────────────────────────────

  1. After 16:00 you void 3.1x more sessions: save that time for lighter work.
     7 of 15 voided or cancelled after 16:00 (47%) vs 7 of 49 before (14%)

  2. Protect 09–12h for your hardest work: it's your strongest focus slot.
     52% of your focus minutes (1100 of 2115) happen there · avg focus 4.3 vs 3.6 overall

  3. #backend tasks take 2.5x the sessions of #docs: split them smaller or budget more blocks.
     #backend: 5.0 sessions per finished task (4 tasks) · #docs: 2.0 (3 tasks)
```

Detectors, each with a minimum amount of data before it speaks:

| Finding | Needs |
|---|---|
| Best time slot (3-hour slots, by focus minutes and focus score) and best weekday | 8 completed sessions, in at least 2 slots / 3 weekdays |
| When you void or cancel sessions, by hour of day and by weekday | 10 finished sessions (completed, voided or cancelled) |
| Tags that take more sessions per finished task | 2 tags with 2 finished tasks each |
| Optimal session length (the length you finish most often) | 10 finished sessions across 2 length bands |
| Trend of focus time vs the previous 7 / 30 days | 3 completed sessions in each period |
| Streak (consecutive days with completed work) | a 3-day streak |
| Dominant Deep Work distractions (internal vs external) | 5 categorized distractions |

When a detector lacks data, the output says how many sessions it still needs instead of guessing. The window is rolling (today included), so a Monday is compared with the previous 7 days, not an empty week; hours and weekdays are in local time. Voided sessions only keep the time they lasted, so for the length detector they count against the length you most often finish beyond that point.

The main insight also shows as one dim line at the end of `flow report` and on the end-of-session screen (from the last 30 days).

## Claude Code Integration

### Focus-aware Claude (hooks)

```bash
flow claude install --dry-run   # preview the merged settings.json
flow claude install             # add the hooks
flow claude install --statusline  # …and the status line below
flow claude uninstall           # remove only what Flow added
```

`flow claude install` adds two hooks to Claude Code's user settings (`~/.claude/settings.json`, or `$CLAUDE_CONFIG_DIR/settings.json`):

- **`SessionStart`**: when a Claude Code conversation starts (or is resumed or cleared) and no Flow session is active, and `[claude] auto_start = true`, Flow starts a background work session in your configured methodology, on a task named after the repository and branch (`flow-cli: feat/login`; just `flow-cli` on `main`/`master`). An open task with that name is reused. You are notified when it ends, like any `flow start --detach`.
- **`UserPromptSubmit`** (and `SessionStart`): while a session or break is active and `[claude] inject_guidance = true`, the current focus guidance is added to Claude's context, for example *"The user is in deep focus (Deep Work, 40 min left) on the task 'Refactor auth'. Keep answers concise, do not propose work outside the current task, and capture side ideas with log_distraction or create_task instead of digressing."* On a break Claude is asked to encourage rest; near the end of a session, to help reach a clean stopping point. With no active session the hook prints nothing, so idle prompts cost no context.

The settings file is merged, not replaced: other keys and hooks are kept, a timestamped backup (`settings.json.flow-backup-…`) is written before any change, running install twice changes nothing, and invalid JSON aborts without writing. An existing status line that is not Flow's is left alone. The hooks call the `flow` binary that ran `install` (and pass `--db` if you gave one).

The hook handler (`flow claude hook <event>`, hidden) reads Claude Code's JSON on stdin, never fails or blocks a prompt (errors go to stderr, exit code is always 0), reads git information straight from `.git` (worktrees included), and only starts the background timer when it actually starts a session; it typically runs in well under 100 ms.

### Status Line

See your timer in Claude Code's status bar:

```
[Opus 4.6] 12% ctx | 🍅 18:52 ███░░ Write API docs
```

`flow claude install --statusline` installs it for you. To do it by hand:

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

**Reviews & insights** — `get_current_state`, `get_recent_sessions` (with each ended session's `git_summary`), `get_daily_summary`, `get_period_stats`, `get_focus_report`, `get_today_highlight`, `get_insights` (`period`: `week` or `month`; same findings and gaps as `flow insights --json`).

**Prompts** — `plan_my_day` (optional `available_hours`) hands the assistant your insights, pending tasks and today's highlight and asks for a schedule of focus blocks; `weekly_review` walks you through a reflection grounded in the last 7 days of data.

**Tasks** — `get_task`, `list_tasks`, `get_task_history`, `create_task`, `complete_task`, `delete_task`, `start_task`.

**Methodology ceremonies** — `set_highlight`, `log_distraction`, `set_focus_score`, `set_accomplishment`, `set_shutdown_ritual`, `set_energize_activity`, `set_outcome_achieved`, `add_session_notes`.

**Resources** — `flow://focus`: the current focus state as JSON (`mode`: `idle`, `pomodoro`, `deep_work`, `make_time`, `break` or `paused`; the active `task` and `session` with `remaining_seconds` and `ends_at`) plus a `guidance` text telling the agent how to behave, the same text the Claude Code hooks inject. `flow://today`: today's completed sessions, focused minutes, breaks, distractions, average focus score, highlight, open tasks and the current guidance. The server does not push change notifications (resource subscriptions are not supported by the MCP library Flow uses), so agents re-read the resource when they need fresh state.

Example: an agent asks you to log a distraction:

```
> tell the agent to log a distraction
log_distraction(session_id="8f2a…", text="Phone rings", category="external")
→ {"session_id": "8f2a…", "logged": "Phone rings", "category": "external"}
```

## Configuration

Flow stores config at `~/.flow/config.toml` (created with defaults on first run) and data at `<storage.data_dir>/flow.db` (`~/.flow/flow.db` by default; `--db` overrides it).

If the config file cannot be parsed, Flow prints a warning on stderr and runs with the defaults without touching the file. Until you fix or remove it, `flow config` refuses to save, and settings changed elsewhere (such as the TUI notification toggle) are not persisted.

The database schema migrates automatically on startup (current version: 4). A database written by a newer flow is rejected with a message asking you to upgrade. The database uses WAL and a busy timeout, so the TUI, the MCP server and CLI commands can share it at the same time.

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

[claude]                 # Claude Code hooks (only active after `flow claude install`)
auto_start = false       # start a background session when Claude Code starts and none is active
inject_guidance = true   # add the focus guidance to Claude's context while a session is active

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
    ├── git/      # Git context detection and end-of-session summaries
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

The README demo is recorded with [vhs](https://github.com/charmbracelet/vhs) from [`docs/demo.tape`](docs/demo.tape) on a throwaway HOME and a fresh database seeded with a year of fake history by [`scripts/demoseed`](scripts/demoseed/main.go) (it refuses to overwrite an existing file): `go build -o flow ./cmd/flow && vhs docs/demo.tape`.

### Releasing

Pushing a `v*` tag runs [GoReleaser](https://goreleaser.com) ([`.goreleaser.yaml`](.goreleaser.yaml)): it builds the `flow-<os>-<arch>` archives that `install.sh` downloads, writes `checksums.txt` (sha256), creates the GitHub release and publishes the Homebrew formula. One-time setup for `brew install xvierd/tap/flow`:

1. Create the public repository `xvierd/homebrew-tap` (an empty repo with a `main` branch is enough; GoReleaser writes `Formula/flow.rb`).
2. Create a fine-grained personal access token with **Contents: Read and write** on `xvierd/homebrew-tap` only.
3. Save it as the `HOMEBREW_TAP_GITHUB_TOKEN` Actions secret of `xvierd/flow-cli`.

Without the secret the release still publishes; only the formula upload is skipped. Check the config locally with `goreleaser check` or `goreleaser release --snapshot --clean`.

## License

MIT — see [LICENSE](LICENSE) for the full text.
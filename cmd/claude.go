package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/adapters/claudecode"
	"github.com/xvierd/flow-cli/internal/adapters/git"
	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/ports"
	"github.com/xvierd/flow-cli/internal/services"
	"github.com/xvierd/flow-cli/scripts"
)

// StatusLineScript is the Claude Code status line script installed by
// `flow claude install --statusline`; tests replace it.
var StatusLineScript = scripts.ClaudeStatusLine

var (
	claudeDryRun     bool
	claudeStatusLine bool
)

// claudeCmd groups the Claude Code integration commands. It owns its
// PersistentPreRunE, so the root's (which opens the database and fails the
// command on any error) does not run: install/uninstall never need the
// database, and the hook must never fail.
var claudeCmd = &cobra.Command{
	Use:   "claude",
	Short: "Integrate Flow with Claude Code (hooks and status line)",
	Long: `Integrate Flow with Claude Code.

"flow claude install" adds hooks to Claude Code's user settings
(~/.claude/settings.json, or $CLAUDE_CONFIG_DIR/settings.json) so that Claude
knows about your focus: while a Flow session is active, the current guidance
(Deep Work: concise answers, stay on the task; break: encourage rest) is added
to Claude's context at session start and on every prompt. With
[claude] auto_start = true, starting Claude Code also starts a background Flow
session named after the repository and branch when none is active.

The settings file is merged, never replaced: other keys and hooks are kept, a
timestamped backup is written before any change, and running install twice
changes nothing.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		loadLanguageOnly()
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		return cleanupServices()
	},
}

var claudeInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Add Flow's hooks (and optionally its status line) to Claude Code",
	Example: `  flow claude install --dry-run
  flow claude install --statusline`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := claudecode.ConfigDir()
		if err != nil {
			return err
		}
		flowCmd, err := hookFlowCommand(cmd)
		if err != nil {
			return err
		}
		opts := claudecode.Options{FlowCommand: flowCmd}
		scriptPath := filepath.Join(dir, claudecode.StatusLineScript)
		if claudeStatusLine {
			opts.StatusLineCommand = claudecode.ShellQuote(scriptPath)
		}
		return editClaudeSettings(cmd.OutOrStdout(), dir, func(data []byte) (claudecode.Result, error) {
			return claudecode.Install(data, opts)
		}, func(res claudecode.Result) error {
			// Only write the script when the status line is Flow's (not when
			// the user's own one was kept).
			if !claudeStatusLine || claudeDryRun || len(StatusLineScript) == 0 || !claudecode.HasFlowStatusLine(res.Data) {
				return nil
			}
			return claudecode.WriteAtomic(scriptPath, StatusLineScript, 0o755)
		})
	},
}

var claudeUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove Flow's hooks and status line from Claude Code",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := claudecode.ConfigDir()
		if err != nil {
			return err
		}
		return editClaudeSettings(cmd.OutOrStdout(), dir, claudecode.Uninstall, func(claudecode.Result) error {
			if claudeDryRun {
				return nil
			}
			err := os.Remove(filepath.Join(dir, claudecode.StatusLineScript))
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return nil
		})
	},
}

var claudeHookCmd = &cobra.Command{
	Use:    "hook <session-start|user-prompt-submit>",
	Short:  "Claude Code hook handler (reads the hook JSON from stdin)",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	// The hook never fails: an error would surface in Claude Code (and exit
	// code 2 would block the user's prompt). Diagnostics go to stderr.
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		// A Go panic exits with status 2, which would block the prompt.
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "flow: internal error in claude hook: %v\n", r)
				err = nil
			}
		}()
		if err := initializeServices(); err != nil {
			fmt.Fprintf(os.Stderr, "flow: %v\n", err)
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := runClaudeHook(ctx, args[0], cmd.InOrStdin(), cmd.OutOrStdout()); err != nil {
			fmt.Fprintf(os.Stderr, "flow: %v\n", err)
		}
		return nil
	},
}

func init() {
	claudeInstallCmd.Flags().BoolVar(&claudeDryRun, "dry-run", false, "Show the resulting settings.json without writing it")
	claudeInstallCmd.Flags().BoolVar(&claudeStatusLine, "statusline", false, "Also install the Flow status line (kept if you already have another one)")
	claudeUninstallCmd.Flags().BoolVar(&claudeDryRun, "dry-run", false, "Show the resulting settings.json without writing it")
	claudeCmd.AddCommand(claudeInstallCmd, claudeUninstallCmd, claudeHookCmd)
	rootCmd.AddCommand(claudeCmd)
}

// loadLanguageOnly resolves the display language without opening the
// database.
func loadLanguageOnly() {
	lang := os.Getenv("FLOW_LANG")
	if lang == "" {
		if cfg, err := config.Load(); err == nil {
			lang = cfg.Language
		}
	}
	i18n.SetLanguage(lang)
}

// hookFlowCommand returns the shell invocation the hooks use to run this
// flow binary: "flow" from PATH when that is this binary (survives upgrades
// that move the real file), else the absolute executable path, plus --db when
// it was given explicitly.
func hookFlowCommand(cmd *cobra.Command) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	bin := exe
	if onPath, err := exec.LookPath("flow"); err == nil && sameFile(onPath, exe) {
		if abs, err := filepath.Abs(onPath); err == nil {
			bin = abs
		}
	}
	parts := []string{claudecode.ShellQuote(bin)}
	if f := cmd.Flag("db"); f != nil && f.Changed && dbPath != "" {
		abs, err := filepath.Abs(dbPath)
		if err != nil {
			return "", err
		}
		parts = append(parts, "--db", claudecode.ShellQuote(abs))
	}
	return strings.Join(parts, " "), nil
}

// sameFile reports whether a and b resolve to the same file.
func sameFile(a, b string) bool {
	ai, err1 := os.Stat(a)
	bi, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(ai, bi)
}

// editClaudeSettings applies edit to settings.json in dir, honoring
// --dry-run, and reports the outcome. after runs once the settings are
// written (or would be, on a dry run).
func editClaudeSettings(out io.Writer, dir string, edit func([]byte) (claudecode.Result, error), after func(claudecode.Result) error) error {
	path := filepath.Join(dir, "settings.json")
	res, backup, err := claudecode.EditFile(path, claudeDryRun, edit)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for _, n := range res.Notes {
		_, _ = fmt.Fprintf(out, "%s\n", i18n.T("Note: %s", n))
	}
	switch {
	case claudeDryRun && res.Changed:
		_, _ = fmt.Fprintf(out, "%s\n", i18n.T("Dry run: %s would become:", path))
		_, _ = out.Write(res.Data)
	case claudeDryRun:
		_, _ = fmt.Fprintf(out, "%s\n", i18n.T("Dry run: %s is already up to date.", path))
	case res.Changed:
		_, _ = fmt.Fprintf(out, "%s\n", i18n.T("✅ Updated %s", path))
		if backup != "" {
			_, _ = fmt.Fprintf(out, "%s\n", i18n.T("   Backup: %s", backup))
		}
	default:
		_, _ = fmt.Fprintf(out, "%s\n", i18n.T("%s is already up to date.", path))
	}
	return after(res)
}

// claudeHookInput is the part of Claude Code's hook JSON Flow reads.
type claudeHookInput struct {
	HookEventName string `json:"hook_event_name"`
	Cwd           string `json:"cwd"`
	Source        string `json:"source"`
}

// claudeHookOutput is the JSON a SessionStart/UserPromptSubmit hook prints:
// additionalContext goes to Claude, systemMessage to the user.
type claudeHookOutput struct {
	SystemMessage      string                 `json:"systemMessage,omitempty"`
	HookSpecificOutput claudeHookSpecificData `json:"hookSpecificOutput"`
}

type claudeHookSpecificData struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// autoStartSources are the SessionStart sources that may auto-start a
// session: a fresh, resumed or cleared conversation, not a compaction.
var autoStartSources = map[string]bool{"": true, "startup": true, "resume": true, "clear": true}

// runClaudeHook handles one Claude Code hook event. It writes nothing to out
// unless there is something to tell Claude, so idle prompts cost no context.
func runClaudeHook(ctx context.Context, event string, in io.Reader, out io.Writer) error {
	var hookEvent string
	switch event {
	case claudecode.ArgSessionStart:
		hookEvent = claudecode.EventSessionStart
	case claudecode.ArgUserPromptSubmit:
		hookEvent = claudecode.EventUserPromptSubmit
	default:
		return fmt.Errorf("unknown hook event %q", event)
	}

	var input claudeHookInput
	if data, err := io.ReadAll(io.LimitReader(in, 1<<20)); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &input); err != nil {
			return fmt.Errorf("invalid hook input: %w", err)
		}
	}
	if input.Cwd == "" {
		input.Cwd, _ = os.Getwd()
	}

	cfg := app.config.Claude
	state, err := app.state.GetCurrentState(ctx)
	if err != nil {
		return err
	}

	var started *domain.PomodoroSession
	var startedTitle string
	if hookEvent == claudecode.EventSessionStart && cfg.AutoStart && autoStartSources[input.Source] && !state.IsSessionActive() {
		started, startedTitle, err = autoStartSession(ctx, input.Cwd)
		if err != nil {
			return err
		}
		if started != nil {
			if state, err = app.state.GetCurrentState(ctx); err != nil {
				return err
			}
		}
	}

	output := claudeHookOutput{HookSpecificOutput: claudeHookSpecificData{HookEventName: hookEvent}}
	if started != nil {
		output.SystemMessage = i18n.T("Flow: started a %s session (%s) for \"%s\"", started.Methodology.Label(), formatMinutes(started.Duration), startedTitle)
	}
	if cfg.InjectGuidance && state.IsSessionActive() {
		output.HookSpecificOutput.AdditionalContext = "Flow focus: " + domain.FocusGuidance(state, time.Now())
	}

	if output.SystemMessage != "" || output.HookSpecificOutput.AdditionalContext != "" {
		enc := json.NewEncoder(out)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(output); err != nil {
			return err
		}
	}

	if started != nil {
		startOnStartHookDetached(ctx, started)
		ensureWatcher()
	}
	return nil
}

// claudeTaskTitle names the auto-started task after the repository and,
// unless it is a mainline branch, the branch; outside git, after the
// directory.
func claudeTaskTitle(cwd string, gi *ports.GitInfo) string {
	if gi == nil {
		return filepath.Base(filepath.Clean(cwd))
	}
	repo := strings.TrimSuffix(gi.Repository, ".git")
	if i := strings.LastIndexAny(repo, "/:"); i >= 0 {
		repo = repo[i+1:]
	}
	if repo == "" {
		repo = filepath.Base(filepath.Clean(cwd))
	}
	switch gi.Branch {
	case "", "main", "master", "trunk", "develop", "HEAD detached":
		return repo
	}
	return repo + ": " + gi.Branch
}

// autoStartSession starts a background work session for cwd, reusing an
// open task with the derived title. It returns a nil session, without
// error, when another process started one first.
func autoStartSession(ctx context.Context, cwd string) (*domain.PomodoroSession, string, error) {
	gi := git.QuickInfo(cwd)
	title := claudeTaskTitle(cwd, gi)
	if gi == nil {
		// An empty context stops StartPomodoro from running the (slow)
		// full git detection.
		gi = &ports.GitInfo{}
	}

	var taskID *string
	// Exact match over all tasks: the fuzzy FindByTitle can miss long exact
	// titles.
	if matches, err := app.storage.Tasks().FindAll(ctx, nil); err == nil {
		for _, m := range matches {
			if m.Status != domain.StatusCompleted && m.Status != domain.StatusCancelled && strings.EqualFold(m.Title, title) {
				id := m.ID
				taskID = &id
				break
			}
		}
	}
	if taskID == nil {
		task, err := app.tasks.AddTask(ctx, services.AddTaskRequest{Title: title})
		if err != nil {
			return nil, "", err
		}
		taskID = &task.ID
	}

	session, err := app.pomodoro.StartPomodoro(ctx, services.StartPomodoroRequest{
		TaskID:      taskID,
		Methodology: app.methodology,
		Duration:    defaultSessionDuration(),
		Tags:        []string{"claude"},
		Git:         gi,
	})
	if errors.Is(err, domain.ErrSessionAlreadyActive) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	return session, title, nil
}

// startOnStartHookDetached runs the on_start hook without waiting for it:
// Claude Code waits for the hook process, and a slow user hook must not
// delay the prompt.
func startOnStartHookDetached(ctx context.Context, s *domain.PomodoroSession) {
	if app.config == nil || app.config.Hooks.OnStart == "" {
		return
	}
	c := hookCommand(context.Background(), app.config.Hooks.OnStart, hookEnv(ctx, "start", s))
	c.Stdin, c.Stdout, c.Stderr = nil, nil, nil
	detach(c)
	if err := c.Start(); err == nil {
		_ = c.Process.Release()
	}
}

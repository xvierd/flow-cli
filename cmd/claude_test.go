package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
	"github.com/xvierd/flow-cli/internal/services"
	"github.com/xvierd/flow-cli/scripts"
)

// setupClaudeHookApp initializes the services on a temp database and HOME
// with the given [claude] settings.
func setupClaudeHookApp(t *testing.T, autoStart, inject bool) {
	t.Helper()
	isolateApp(t)
	dbPath = filepath.Join(t.TempDir(), "flow.db")
	if err := initializeServices(); err != nil {
		t.Fatalf("initializeServices() error = %v", err)
	}
	app.config.Notifications.Enabled = false
	app.config.Claude.AutoStart = autoStart
	app.config.Claude.InjectGuidance = inject
}

// fakeRepo creates a git checkout named name on branch and returns its path.
func fakeRepo(t *testing.T, name, branch string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/"+branch+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func hookInput(t *testing.T, v map[string]string) *strings.Reader {
	t.Helper()
	b, _ := json.Marshal(v)
	return strings.NewReader(string(b))
}

func decodeHookOutput(t *testing.T, out string) claudeHookOutput {
	t.Helper()
	var o claudeHookOutput
	if err := json.Unmarshal([]byte(out), &o); err != nil {
		t.Fatalf("hook output is not JSON: %v (%q)", err, out)
	}
	return o
}

func TestClaudeHook_IdleIsSilent(t *testing.T) {
	setupClaudeHookApp(t, false, true)
	var out bytes.Buffer
	for _, ev := range []string{"session-start", "user-prompt-submit"} {
		if err := runClaudeHook(context.Background(), ev, hookInput(t, map[string]string{"cwd": t.TempDir(), "source": "startup"}), &out); err != nil {
			t.Fatalf("%s: %v", ev, err)
		}
	}
	if out.Len() != 0 {
		t.Errorf("idle hook printed %q", out.String())
	}
	if s, _ := app.storage.Sessions().FindActive(context.Background()); s != nil {
		t.Error("auto_start=false must not start a session")
	}
}

func TestClaudeHook_AutoStartOnSessionStart(t *testing.T) {
	setupClaudeHookApp(t, true, true)
	ctx := context.Background()
	repo := fakeRepo(t, "myapp", "feature/login")

	var out bytes.Buffer
	if err := runClaudeHook(ctx, "session-start", hookInput(t, map[string]string{"cwd": repo, "source": "startup", "hook_event_name": "SessionStart"}), &out); err != nil {
		t.Fatal(err)
	}
	o := decodeHookOutput(t, out.String())
	if o.HookSpecificOutput.HookEventName != "SessionStart" {
		t.Errorf("hookEventName = %q", o.HookSpecificOutput.HookEventName)
	}
	if !strings.Contains(o.SystemMessage, "myapp: feature/login") {
		t.Errorf("systemMessage = %q", o.SystemMessage)
	}
	if !strings.Contains(o.HookSpecificOutput.AdditionalContext, "Pomodoro work session") {
		t.Errorf("additionalContext = %q", o.HookSpecificOutput.AdditionalContext)
	}

	active, err := app.storage.Sessions().FindActive(ctx)
	if err != nil || active == nil {
		t.Fatalf("no active session: %v", err)
	}
	if active.GitBranch != "feature/login" || active.TaskID == nil || len(active.Tags) != 1 || active.Tags[0] != "claude" {
		t.Errorf("session = %+v", active)
	}
	task, _ := app.tasks.GetTask(ctx, *active.TaskID)
	if task.Title != "myapp: feature/login" {
		t.Errorf("task title = %q", task.Title)
	}

	// A second Claude session while one is active neither restarts nor
	// announces anything; it only injects guidance.
	out.Reset()
	if err := runClaudeHook(ctx, "session-start", hookInput(t, map[string]string{"cwd": repo, "source": "startup"}), &out); err != nil {
		t.Fatal(err)
	}
	if o := decodeHookOutput(t, out.String()); o.SystemMessage != "" || o.HookSpecificOutput.AdditionalContext == "" {
		t.Errorf("second session-start output = %+v", o)
	}

	// After the session ends, a new one reuses the open task.
	if _, err := app.pomodoro.StopSession(ctx); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runClaudeHook(ctx, "session-start", hookInput(t, map[string]string{"cwd": repo, "source": "resume"}), &out); err != nil {
		t.Fatal(err)
	}
	again, _ := app.storage.Sessions().FindActive(ctx)
	if again == nil || again.TaskID == nil || *again.TaskID != task.ID {
		t.Errorf("restart did not reuse task %s: %+v", task.ID, again)
	}
}

func TestClaudeHook_NoAutoStartOnPromptOrCompact(t *testing.T) {
	setupClaudeHookApp(t, true, true)
	ctx := context.Background()
	repo := fakeRepo(t, "app", "main")
	var out bytes.Buffer
	_ = runClaudeHook(ctx, "user-prompt-submit", hookInput(t, map[string]string{"cwd": repo, "prompt": "hi"}), &out)
	_ = runClaudeHook(ctx, "session-start", hookInput(t, map[string]string{"cwd": repo, "source": "compact"}), &out)
	if s, _ := app.storage.Sessions().FindActive(ctx); s != nil {
		t.Errorf("session started from %+v", s)
	}
	if out.Len() != 0 {
		t.Errorf("output = %q", out.String())
	}
}

func TestClaudeHook_GuidanceDuringDeepWorkAndBreak(t *testing.T) {
	setupClaudeHookApp(t, false, true)
	ctx := context.Background()
	task, _ := app.tasks.AddTask(ctx, services.AddTaskRequest{Title: "Refactor auth"})
	if _, err := app.state.StartSession(ctx, ports.StartSessionRequest{TaskID: &task.ID, Methodology: domain.MethodologyDeepWork}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runClaudeHook(ctx, "user-prompt-submit", strings.NewReader(`{"prompt":"x"}`), &out); err != nil {
		t.Fatal(err)
	}
	o := decodeHookOutput(t, out.String())
	ctxText := o.HookSpecificOutput.AdditionalContext
	if o.HookSpecificOutput.HookEventName != "UserPromptSubmit" || !strings.Contains(ctxText, "deep focus") || !strings.Contains(ctxText, "'Refactor auth'") {
		t.Errorf("deep work output = %+v", o)
	}

	if _, err := app.pomodoro.StopSession(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := app.state.StartBreak(ctx); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	_ = runClaudeHook(ctx, "user-prompt-submit", strings.NewReader(`{}`), &out)
	if o := decodeHookOutput(t, out.String()); !strings.Contains(o.HookSpecificOutput.AdditionalContext, "break") {
		t.Errorf("break output = %+v", o)
	}

	// inject_guidance = false keeps the hook silent.
	app.config.Claude.InjectGuidance = false
	out.Reset()
	_ = runClaudeHook(ctx, "user-prompt-submit", strings.NewReader(`{}`), &out)
	if out.Len() != 0 {
		t.Errorf("inject_guidance=false printed %q", out.String())
	}
}

func TestClaudeHook_BadInput(t *testing.T) {
	setupClaudeHookApp(t, true, true)
	var out bytes.Buffer
	if err := runClaudeHook(context.Background(), "session-start", strings.NewReader("not json"), &out); err == nil {
		t.Error("invalid JSON should be reported")
	}
	if err := runClaudeHook(context.Background(), "bogus", strings.NewReader("{}"), &out); err == nil {
		t.Error("unknown event should be reported")
	}
	if out.Len() != 0 {
		t.Errorf("output = %q", out.String())
	}
}

// TestClaudeHook_WatcherAndOnStartOnlyWhenStarted checks the background
// watcher is spawned (and on_start runs, detached) only when the hook starts
// a session.
func TestClaudeHook_WatcherAndOnStartOnlyWhenStarted(t *testing.T) {
	setupClaudeHookApp(t, true, true)
	t.Setenv(noWatcherEnv, "")
	spawns := 0
	oldSpawn := spawnWatcher
	spawnWatcher = func() error { spawns++; return nil }
	t.Cleanup(func() { spawnWatcher = oldSpawn })
	marker := filepath.Join(t.TempDir(), "started")
	app.config.Hooks.OnStart = `printf '%s' "$FLOW_TASK" > "` + marker + `"`

	ctx := context.Background()
	repo := fakeRepo(t, "svc", "main")
	var out bytes.Buffer
	_ = runClaudeHook(ctx, "user-prompt-submit", hookInput(t, map[string]string{"cwd": repo}), &out)
	if spawns != 0 {
		t.Fatalf("idle prompt spawned the watcher")
	}
	if err := runClaudeHook(ctx, "session-start", hookInput(t, map[string]string{"cwd": repo, "source": "startup"}), &out); err != nil {
		t.Fatal(err)
	}
	if spawns != 1 {
		t.Fatalf("auto-start spawns = %d, want 1", spawns)
	}
	_ = runClaudeHook(ctx, "user-prompt-submit", hookInput(t, map[string]string{"cwd": repo}), &out)
	_ = runClaudeHook(ctx, "session-start", hookInput(t, map[string]string{"cwd": repo, "source": "startup"}), &out)
	if spawns != 1 {
		t.Errorf("guidance-only hooks spawned the watcher (spawns = %d)", spawns)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		data, err := os.ReadFile(marker)
		if err == nil && string(data) == "svc" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("on_start hook did not run: %q, %v", data, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestClaudeHookCmd_NeverFails runs the hidden command end to end: even with
// garbage input it exits successfully with nothing on stdout.
func TestClaudeHookCmd_NeverFails(t *testing.T) {
	isolateApp(t)
	db := filepath.Join(t.TempDir(), "flow.db")
	rootCmd.SetIn(strings.NewReader("garbage"))
	t.Cleanup(func() { rootCmd.SetIn(nil) })
	stdout, _, err := executeCmd(rootCmd, "--db", db, "claude", "hook", "user-prompt-submit")
	if err != nil || stdout != "" {
		t.Errorf("hook cmd = %q, %v", stdout, err)
	}
}

func TestClaudeTaskTitle(t *testing.T) {
	for _, tt := range []struct {
		gi   *ports.GitInfo
		want string
	}{
		{nil, "dir"},
		{&ports.GitInfo{Repository: "me/app", Branch: "main"}, "app"},
		{&ports.GitInfo{Repository: "app", Branch: "fix/bug"}, "app: fix/bug"},
		{&ports.GitInfo{Branch: "HEAD detached"}, "dir"},
		{&ports.GitInfo{Repository: "ssh://host/srv/repo.git", Branch: "main"}, "repo"},
	} {
		if got := claudeTaskTitle("/x/dir", tt.gi); got != tt.want {
			t.Errorf("claudeTaskTitle(%+v) = %q, want %q", tt.gi, got, tt.want)
		}
	}
}

// TestClaudeInstallCmd drives install/uninstall through cobra against a
// temporary CLAUDE_CONFIG_DIR, never the real ~/.claude.
func TestClaudeInstallCmd(t *testing.T) {
	isolateApp(t)
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	StatusLineScript = []byte("#!/bin/bash\necho flow\n")
	t.Cleanup(func() { claudeDryRun, claudeStatusLine, StatusLineScript = false, false, scripts.ClaudeStatusLine })
	settings := filepath.Join(dir, "settings.json")
	orig := `{"model":"opus"}`
	if err := os.WriteFile(settings, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}

	out, _, err := executeCmd(rootCmd, "claude", "install", "--dry-run", "--statusline")
	if err != nil || !strings.Contains(out, "claude hook session-start") {
		t.Fatalf("dry run = %q, %v", out, err)
	}
	if data, _ := os.ReadFile(settings); string(data) != orig {
		t.Error("dry run wrote settings.json")
	}
	if _, err := os.Stat(filepath.Join(dir, "flow-statusline.sh")); err == nil {
		t.Error("dry run wrote the status line script")
	}

	claudeDryRun = false
	if _, _, err := executeCmd(rootCmd, "claude", "install", "--statusline"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(settings)
	if !strings.Contains(string(data), `"model": "opus"`) || !strings.Contains(string(data), "flow-statusline.sh") {
		t.Errorf("installed settings:\n%s", data)
	}
	if info, err := os.Stat(filepath.Join(dir, "flow-statusline.sh")); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Errorf("status line script not installed executable: %v", err)
	}

	claudeStatusLine = false
	if _, _, err := executeCmd(rootCmd, "claude", "uninstall"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(settings)
	var got map[string]interface{}
	_ = json.Unmarshal(data, &got)
	if len(got) != 1 || got["model"] != "opus" {
		t.Errorf("uninstall left %s", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "flow-statusline.sh")); err == nil {
		t.Error("uninstall left the status line script")
	}
}

// TestClaudeInstallCmd_KeepsUserStatusLine checks that --statusline neither
// replaces a user's status line nor leaves an unused script behind.
func TestClaudeInstallCmd_KeepsUserStatusLine(t *testing.T) {
	isolateApp(t)
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	t.Cleanup(func() { claudeDryRun, claudeStatusLine = false, false })
	settings := filepath.Join(dir, "settings.json")
	_ = os.WriteFile(settings, []byte(`{"statusLine":{"type":"command","command":"mine.sh"}}`), 0o600)

	out, _, err := executeCmd(rootCmd, "claude", "install", "--statusline")
	if err != nil || !strings.Contains(out, "left unchanged") {
		t.Fatalf("install = %q, %v", out, err)
	}
	if data, _ := os.ReadFile(settings); !strings.Contains(string(data), "mine.sh") {
		t.Errorf("user status line replaced: %s", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "flow-statusline.sh")); err == nil {
		t.Error("unused status line script written")
	}
}

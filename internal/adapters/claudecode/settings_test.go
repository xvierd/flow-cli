package claudecode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const flowCmd = "/usr/local/bin/flow"

func decode(t *testing.T, data []byte) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, data)
	}
	return m
}

// flowHookCount counts Flow hook entries under event.
func flowHookCount(t *testing.T, settings map[string]interface{}, event string) int {
	t.Helper()
	hooks, _ := settings["hooks"].(map[string]interface{})
	groups, _ := hooks[event].([]interface{})
	n := 0
	for _, g := range groups {
		for _, h := range g.(map[string]interface{})["hooks"].([]interface{}) {
			if isFlowHook(h) {
				n++
			}
		}
	}
	return n
}

const userSettings = `{
  "model": "opus",
  "permissions": {"allow": ["Bash(ls:*)"]},
  "statusLine": {"type": "command", "command": "~/my-line.sh"},
  "hooks": {
    "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "echo mine"}]}],
    "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "guard.sh", "timeout": 3}]}]
  }
}`

func TestInstall_PreservesKeysAndIsIdempotent(t *testing.T) {
	res, err := Install([]byte(userSettings), Options{FlowCommand: flowCmd, StatusLineCommand: "/h/.claude/flow-statusline.sh"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed {
		t.Fatal("first install should change the settings")
	}
	got := decode(t, res.Data)
	if got["model"] != "opus" || got["permissions"] == nil {
		t.Errorf("top-level keys lost: %v", got)
	}
	if sl := got["statusLine"].(map[string]interface{}); sl["command"] != "~/my-line.sh" {
		t.Errorf("user status line overwritten: %v", sl)
	}
	if len(res.Notes) == 0 {
		t.Error("expected a note about the status line left alone")
	}
	hooks := got["hooks"].(map[string]interface{})
	if pre := hooks["PreToolUse"].([]interface{}); len(pre) != 1 {
		t.Errorf("PreToolUse changed: %v", pre)
	}
	ups := hooks["UserPromptSubmit"].([]interface{})
	if len(ups) != 2 || !strings.Contains(string(res.Data), "echo mine") {
		t.Errorf("user's UserPromptSubmit hook lost: %v", ups)
	}
	if flowHookCount(t, got, EventSessionStart) != 1 || flowHookCount(t, got, EventUserPromptSubmit) != 1 {
		t.Errorf("flow hooks missing:\n%s", res.Data)
	}
	if !strings.Contains(string(res.Data), flowCmd+" claude hook session-start") {
		t.Errorf("session-start command missing:\n%s", res.Data)
	}

	again, err := Install(res.Data, Options{FlowCommand: flowCmd, StatusLineCommand: "/h/.claude/flow-statusline.sh"})
	if err != nil {
		t.Fatal(err)
	}
	if again.Changed {
		t.Errorf("second install changed the settings:\n%s", again.Data)
	}
	if flowHookCount(t, decode(t, again.Data), EventSessionStart) != 1 {
		t.Error("second install duplicated hooks")
	}
}

func TestInstall_ReplacesOldFlowCommand(t *testing.T) {
	first, _ := Install(nil, Options{FlowCommand: "/old/flow"})
	res, err := Install(first.Data, Options{FlowCommand: flowCmd})
	if err != nil || !res.Changed {
		t.Fatalf("Install() = %v, %v", res.Changed, err)
	}
	if strings.Contains(string(res.Data), "/old/flow") || flowHookCount(t, decode(t, res.Data), EventUserPromptSubmit) != 1 {
		t.Errorf("old command not replaced:\n%s", res.Data)
	}
}

func TestInstall_StatusLineWhenFree(t *testing.T) {
	res, err := Install([]byte(`{}`), Options{FlowCommand: flowCmd, StatusLineCommand: "/h/.claude/flow-statusline.sh"})
	if err != nil {
		t.Fatal(err)
	}
	sl, _ := decode(t, res.Data)["statusLine"].(map[string]interface{})
	if sl["command"] != "/h/.claude/flow-statusline.sh" || sl["type"] != "command" {
		t.Errorf("statusLine = %v", sl)
	}
}

func TestInstall_RejectsInvalid(t *testing.T) {
	for name, in := range map[string]string{
		"invalid json":      `{"model": `,
		"hooks not object":  `{"hooks": []}`,
		"event not array":   `{"hooks": {"SessionStart": {}}}`,
		"top-level not obj": `[1,2]`,
	} {
		if _, err := Install([]byte(in), Options{FlowCommand: flowCmd}); err == nil {
			t.Errorf("%s: Install() should fail", name)
		}
	}
	if _, err := Install(nil, Options{}); err == nil {
		t.Error("empty flow command should fail")
	}
}

func TestUninstall_RemovesOnlyFlow(t *testing.T) {
	installed, err := Install([]byte(userSettings), Options{FlowCommand: flowCmd})
	if err != nil {
		t.Fatal(err)
	}
	res, err := Uninstall(installed.Data)
	if err != nil || !res.Changed {
		t.Fatalf("Uninstall() = %v, %v", res.Changed, err)
	}
	got := decode(t, res.Data)
	want := decode(t, []byte(userSettings))
	gb, _ := json.Marshal(got)
	wb, _ := json.Marshal(want)
	if string(gb) != string(wb) {
		t.Errorf("uninstall did not restore the original settings:\n got %s\nwant %s", gb, wb)
	}

	again, _ := Uninstall(res.Data)
	if again.Changed {
		t.Error("second uninstall changed the settings")
	}
}

func TestUninstall_RemovesFlowStatusLineAndEmptyHooks(t *testing.T) {
	installed, _ := Install(nil, Options{FlowCommand: flowCmd, StatusLineCommand: "/x/flow-statusline.sh"})
	res, err := Uninstall(installed.Data)
	if err != nil {
		t.Fatal(err)
	}
	if got := decode(t, res.Data); len(got) != 0 {
		t.Errorf("expected empty settings, got %v", got)
	}
}

func TestEditFile_BackupDryRunAndMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(userSettings), 0o640); err != nil {
		t.Fatal(err)
	}
	install := func(data []byte) (Result, error) { return Install(data, Options{FlowCommand: flowCmd}) }

	// Dry run writes nothing, not even a backup.
	res, backup, err := EditFile(path, true, install)
	if err != nil || !res.Changed || backup != "" {
		t.Fatalf("dry run = %v, %q, %v", res.Changed, backup, err)
	}
	if data, _ := os.ReadFile(path); string(data) != userSettings {
		t.Error("dry run modified the file")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("dry run left files behind: %v", entries)
	}

	res, backup, err = EditFile(path, false, install)
	if err != nil || !res.Changed || backup == "" {
		t.Fatalf("install = %v, %q, %v", res.Changed, backup, err)
	}
	if data, _ := os.ReadFile(backup); string(data) != userSettings {
		t.Error("backup does not hold the original content")
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640", info.Mode().Perm())
	}

	// Second run: nothing to change, no new backup.
	res, backup, err = EditFile(path, false, install)
	if err != nil || res.Changed || backup != "" {
		t.Fatalf("second install = %v, %q, %v", res.Changed, backup, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 2 {
		t.Errorf("want settings.json + one backup, got %d entries", len(entries))
	}
}

func TestEditFile_BackupsNeverOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	_ = os.WriteFile(path, []byte(`{"a":1}`), 0o600)
	_, b1, err := EditFile(path, false, func(d []byte) (Result, error) { return Install(d, Options{FlowCommand: flowCmd}) })
	if err != nil {
		t.Fatal(err)
	}
	_, b2, err := EditFile(path, false, Uninstall)
	if err != nil {
		t.Fatal(err)
	}
	if b1 == "" || b2 == "" || b1 == b2 {
		t.Fatalf("backups %q, %q", b1, b2)
	}
	if data, _ := os.ReadFile(b1); string(data) != `{"a":1}` {
		t.Errorf("first backup overwritten: %s", data)
	}
}

func TestEditFile_MissingAndInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "settings.json")
	install := func(data []byte) (Result, error) { return Install(data, Options{FlowCommand: flowCmd}) }
	if _, backup, err := EditFile(path, false, install); err != nil || backup != "" {
		t.Fatalf("missing file: backup %q, err %v", backup, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("settings not created: %v", err)
	}

	bad := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(bad, []byte("{nope"), 0o600)
	if _, _, err := EditFile(bad, false, install); err == nil {
		t.Error("invalid JSON should abort")
	}
	if data, _ := os.ReadFile(bad); string(data) != "{nope" {
		t.Error("invalid file was modified")
	}
}

func TestConfigDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/custom/claude")
	if dir, _ := ConfigDir(); dir != "/custom/claude" {
		t.Errorf("ConfigDir() = %q", dir)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("HOME", "/home/u")
	if dir, _ := ConfigDir(); dir != filepath.Join("/home/u", ".claude") {
		t.Errorf("ConfigDir() = %q", dir)
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/usr/bin/flow": "/usr/bin/flow",
		"/a b/flow":     "'/a b/flow'",
		"/it's/flow":    `'/it'\''s/flow'`,
		"":              "''",
	} {
		if got := ShellQuote(in); got != want {
			t.Errorf("ShellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

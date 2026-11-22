// Package claudecode edits Claude Code's user settings (settings.json) to
// install or remove Flow's hooks and status line.
//
// Edits are merges: every key Flow does not own is preserved, Flow's own
// entries are recognized by HookMarker in their command, so installing twice
// changes nothing and uninstalling leaves the user's other hooks alone.
package claudecode

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

// HookMarker identifies the hook commands Flow installs.
const HookMarker = " claude hook "

// StatusLineScript is the file name of the status line script Flow installs
// next to settings.json.
const StatusLineScript = "flow-statusline.sh"

// Hook event names, as Claude Code spells them, and the subcommand argument
// Flow uses for each.
const (
	EventSessionStart     = "SessionStart"
	EventUserPromptSubmit = "UserPromptSubmit"

	ArgSessionStart     = "session-start"
	ArgUserPromptSubmit = "user-prompt-submit"
)

// ConfigDir returns Claude Code's user configuration directory:
// $CLAUDE_CONFIG_DIR when set, else ~/.claude.
func ConfigDir() (string, error) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// Options describes what Install adds.
type Options struct {
	// FlowCommand is the shell invocation of flow the hooks run, e.g.
	// "/usr/local/bin/flow" or "flow --db '/tmp/x.db'".
	FlowCommand string
	// StatusLineCommand, when set, is installed as the statusLine command
	// unless the user already has a status line that is not Flow's.
	StatusLineCommand string
}

// Result reports what an edit did.
type Result struct {
	// Data is the new settings.json content.
	Data []byte
	// Changed is false when the settings already matched.
	Changed bool
	// Notes are human-readable remarks (e.g. a status line left alone).
	Notes []string
}

// ShellQuote quotes s for a POSIX shell when it contains anything beyond a
// conservative safe set.
func ShellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && !strings.ContainsRune("/._-+:=@%", r)
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// parse decodes settings.json content; empty content is an empty object.
func parse(data []byte) (map[string]interface{}, error) {
	settings := map[string]interface{}{}
	if len(bytes.TrimSpace(data)) == 0 {
		return settings, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&settings); err != nil {
		return nil, fmt.Errorf("settings.json is not a valid JSON object: %w", err)
	}
	if settings == nil {
		settings = map[string]interface{}{}
	}
	return settings, nil
}

// encode renders settings the way Claude Code writes them (2-space indent).
func encode(settings map[string]interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(settings); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// isFlowHook reports whether a hook entry was installed by Flow.
func isFlowHook(h interface{}) bool {
	m, ok := h.(map[string]interface{})
	if !ok {
		return false
	}
	cmd, _ := m["command"].(string)
	return strings.Contains(cmd, HookMarker)
}

// isFlowStatusLine reports whether a statusLine value was installed by Flow.
func isFlowStatusLine(v interface{}) bool {
	m, ok := v.(map[string]interface{})
	if !ok {
		return false
	}
	cmd, _ := m["command"].(string)
	return strings.Contains(cmd, StatusLineScript)
}

// HasFlowStatusLine reports whether settings.json content has Flow's status
// line installed.
func HasFlowStatusLine(data []byte) bool {
	settings, err := parse(data)
	return err == nil && isFlowStatusLine(settings["statusLine"])
}

// removeFlowHooks drops Flow's hook entries from every event, then drops
// matcher groups and events left empty. Entries it does not recognize are
// kept untouched.
func removeFlowHooks(settings map[string]interface{}) {
	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		return
	}
	for event, raw := range hooks {
		groups, ok := raw.([]interface{})
		if !ok {
			continue
		}
		kept := make([]interface{}, 0, len(groups))
		for _, g := range groups {
			group, ok := g.(map[string]interface{})
			if !ok {
				kept = append(kept, g)
				continue
			}
			entries, ok := group["hooks"].([]interface{})
			if !ok {
				kept = append(kept, g)
				continue
			}
			var rest []interface{}
			for _, h := range entries {
				if !isFlowHook(h) {
					rest = append(rest, h)
				}
			}
			if len(rest) == len(entries) {
				kept = append(kept, g)
				continue
			}
			if len(rest) == 0 {
				continue
			}
			group["hooks"] = rest
			kept = append(kept, group)
		}
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = kept
		}
	}
	if len(hooks) == 0 {
		delete(settings, "hooks")
	}
}

// flowHookGroups returns the matcher groups Flow installs, keyed by event.
func flowHookGroups(flowCommand string) map[string]map[string]interface{} {
	entry := func(arg string, timeout int) map[string]interface{} {
		return map[string]interface{}{
			"type":    "command",
			"command": flowCommand + HookMarker + arg,
			"timeout": json.Number(fmt.Sprint(timeout)),
		}
	}
	return map[string]map[string]interface{}{
		EventSessionStart: {
			"hooks": []interface{}{entry(ArgSessionStart, 10)},
		},
		EventUserPromptSubmit: {
			"hooks": []interface{}{entry(ArgUserPromptSubmit, 5)},
		},
	}
}

// Install merges Flow's hooks (and optionally its status line) into the
// settings.json content data.
func Install(data []byte, opts Options) (Result, error) {
	if strings.TrimSpace(opts.FlowCommand) == "" {
		return Result{}, errors.New("flow command is empty")
	}
	settings, err := parse(data)
	if err != nil {
		return Result{}, err
	}
	original, _ := parse(data)

	if raw, exists := settings["hooks"]; exists {
		if _, ok := raw.(map[string]interface{}); !ok {
			return Result{}, errors.New(`settings.json "hooks" is not an object; fix it before installing`)
		}
	}
	removeFlowHooks(settings)
	hooks, _ := settings["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = map[string]interface{}{}
	}
	for event, group := range flowHookGroups(opts.FlowCommand) {
		existing, _ := hooks[event].([]interface{})
		if _, present := hooks[event]; present && existing == nil {
			return Result{}, fmt.Errorf(`settings.json "hooks.%s" is not an array; fix it before installing`, event)
		}
		hooks[event] = append(existing, group)
	}
	settings["hooks"] = hooks

	var notes []string
	if opts.StatusLineCommand != "" {
		current, has := settings["statusLine"]
		switch {
		case has && !isFlowStatusLine(current):
			notes = append(notes, "statusLine already set to another command; left unchanged")
		default:
			settings["statusLine"] = map[string]interface{}{
				"type":    "command",
				"command": opts.StatusLineCommand,
			}
		}
	}

	return finish(original, settings, notes)
}

// Uninstall removes Flow's hooks and, if it is Flow's, the status line.
func Uninstall(data []byte) (Result, error) {
	settings, err := parse(data)
	if err != nil {
		return Result{}, err
	}
	original, _ := parse(data)
	removeFlowHooks(settings)
	if isFlowStatusLine(settings["statusLine"]) {
		delete(settings, "statusLine")
	}
	return finish(original, settings, nil)
}

// finish encodes settings and reports whether they differ from original.
func finish(original, settings map[string]interface{}, notes []string) (Result, error) {
	out, err := encode(settings)
	if err != nil {
		return Result{}, err
	}
	return Result{Data: out, Changed: !reflect.DeepEqual(normalize(original), normalize(settings)), Notes: notes}, nil
}

// normalize round-trips v through JSON so values built in Go compare equal
// to values decoded from a file.
func normalize(v interface{}) interface{} {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out interface{}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	_ = dec.Decode(&out)
	return out
}

// EditFile applies edit to the settings file at path. When the result
// changes the settings and dryRun is false, the existing file is first copied
// to a timestamped backup (returned as backup) and the new content is
// written atomically, keeping the file's mode. A missing file is treated as
// empty settings. Invalid JSON aborts without writing anything.
func EditFile(path string, dryRun bool, edit func([]byte) (Result, error)) (res Result, backup string, err error) {
	// Edit the target of a symlinked settings.json (dotfile repos) instead of
	// replacing the link with a regular file.
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	data, err := os.ReadFile(path) // #nosec G304 -- the user's own settings file
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Result{}, "", err
	}
	res, err = edit(data)
	if err != nil {
		return Result{}, "", err
	}
	if !res.Changed || dryRun {
		return res, "", nil
	}

	mode := os.FileMode(0o600)
	if exists {
		if info, err := os.Stat(path); err == nil {
			mode = info.Mode().Perm()
		}
		if backup, err = writeBackup(path, data, mode); err != nil {
			return Result{}, "", fmt.Errorf("failed to write backup: %w", err)
		}
	}
	if err := WriteAtomic(path, res.Data, mode); err != nil {
		return Result{}, backup, err
	}
	return res, backup, nil
}

// writeBackup copies data to a new timestamped file next to path, never
// overwriting an earlier backup (two edits can happen within a second).
func writeBackup(path string, data []byte, mode os.FileMode) (string, error) {
	base := fmt.Sprintf("%s.flow-backup-%s", path, time.Now().Format("20060102-150405"))
	for i := 0; i < 100; i++ {
		name := base
		if i > 0 {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode) // #nosec G304 -- next to the user's settings file
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := f.Write(data); err != nil {
			_ = f.Close()
			return "", err
		}
		return name, f.Close()
	}
	return "", errors.New("too many backups")
}

// WriteAtomic writes data to path through a temporary file and a rename.
func WriteAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path) // #nosec G703 -- path is the user's own settings file
}

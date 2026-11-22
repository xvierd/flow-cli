package integrations

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Marker is written into every file flow generates, so disable only ever
// removes files flow owns.
const Marker = "managed by flow integrate"

// ChangeKind is what a Change does to its file.
type ChangeKind int

const (
	// WriteFile creates or replaces a whole file flow owns.
	WriteFile ChangeKind = iota
	// RemoveFile deletes a file flow owns.
	RemoveFile
	// UpsertBlock inserts or replaces a marked block in a user file.
	UpsertBlock
	// RemoveBlock removes a marked block from a user file.
	RemoveBlock
)

// Change is one file edit of a Plan.
type Change struct {
	Kind    ChangeKind
	Path    string
	Content string // file content (WriteFile) or block body (UpsertBlock)
	Block   string // block name for UpsertBlock / RemoveBlock
	Mode    os.FileMode
	// Confirm asks the user before applying: the file is theirs (e.g.
	// ~/.tmux.conf) or would replace a file flow did not write.
	Confirm bool
}

// Describe returns an English one-line summary (for i18n.T) and its args.
func (c Change) Describe() (string, []any) {
	switch c.Kind {
	case WriteFile:
		return "write %s", []any{c.Path}
	case RemoveFile:
		return "remove %s", []any{c.Path}
	case UpsertBlock:
		return "add the flow block to %s", []any{c.Path}
	default:
		return "remove the flow block from %s", []any{c.Path}
	}
}

// Note is an English message (for i18n.T) with printf args.
type Note struct {
	Text string
	Args []any
}

// Plan lists what enable/disable will change.
type Plan struct {
	Changes []Change
	// Notes are shown after the changes: manual steps, snippets to paste.
	Notes []Note
}

func (p *Plan) note(text string, args ...any) {
	p.Notes = append(p.Notes, Note{Text: text, Args: args})
}

// Apply performs one change.
func (c Change) Apply() error {
	switch c.Kind {
	case WriteFile:
		if err := os.MkdirAll(filepath.Dir(c.Path), 0750); err != nil {
			return err
		}
		mode := c.Mode
		if mode == 0 {
			mode = 0600
		}
		if err := os.WriteFile(c.Path, []byte(c.Content), mode); err != nil {
			return err
		}
		return os.Chmod(c.Path, mode)
	case RemoveFile:
		err := os.Remove(c.Path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	case UpsertBlock, RemoveBlock:
		data, err := os.ReadFile(c.Path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		var out string
		if c.Kind == UpsertBlock {
			out = upsertBlock(string(data), c.Block, c.Content)
		} else {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			out = removeBlock(string(data), c.Block)
		}
		if out == string(data) {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(c.Path), 0750); err != nil {
			return err
		}
		mode := os.FileMode(0644)
		if info, statErr := os.Stat(c.Path); statErr == nil {
			mode = info.Mode().Perm()
		}
		return os.WriteFile(c.Path, []byte(out), mode)
	}
	return fmt.Errorf("unknown change kind %d", c.Kind)
}

func blockMarkers(name string) (begin, end string) {
	return "# >>> flow integrate " + name + " >>>", "# <<< flow integrate " + name + " <<<"
}

// upsertBlock returns text with the named block set to body, replacing an
// existing block in place or appending a new one.
func upsertBlock(text, name, body string) string {
	begin, end := blockMarkers(name)
	block := begin + "\n" + strings.TrimRight(body, "\n") + "\n" + end + "\n"
	if i := strings.Index(text, begin); i >= 0 {
		if j := strings.Index(text[i:], end); j >= 0 {
			rest := text[i+j+len(end):]
			rest = strings.TrimPrefix(rest, "\n")
			return text[:i] + block + rest
		}
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" {
		text += "\n"
	}
	return text + block
}

// removeBlock returns text without the named block (and the blank line
// upsertBlock put before it).
func removeBlock(text, name string) string {
	begin, end := blockMarkers(name)
	i := strings.Index(text, begin)
	if i < 0 {
		return text
	}
	j := strings.Index(text[i:], end)
	if j < 0 {
		return text
	}
	head := text[:i]
	rest := strings.TrimPrefix(text[i+j+len(end):], "\n")
	if strings.HasSuffix(head, "\n\n") {
		head = strings.TrimSuffix(head, "\n")
	}
	return head + rest
}

// ownedByFlow reports whether path is missing or a file flow wrote.
func ownedByFlow(path string) (exists, owned bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, true
	}
	return true, strings.Contains(string(data), Marker)
}

// writeOwned returns a WriteFile change that asks before replacing a file
// flow did not write.
func writeOwned(path, content string, mode os.FileMode) Change {
	exists, owned := ownedByFlow(path)
	return Change{Kind: WriteFile, Path: path, Content: content, Mode: mode, Confirm: exists && !owned}
}

// removeOwned returns the change removing path, or false when there is
// nothing flow owns there.
func removeOwned(path string) (Change, bool) {
	exists, owned := ownedByFlow(path)
	if !exists || !owned {
		return Change{}, false
	}
	return Change{Kind: RemoveFile, Path: path}, true
}

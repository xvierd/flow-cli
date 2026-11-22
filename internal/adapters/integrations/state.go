package integrations

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

var errPanic = errors.New("integration panicked")

// stateFile is the name of the state file inside Env.StateDir.
const stateFile = "integrations.json"

// State remembers what the last start changed, so the end (usually in a
// different process: the TUI, the background watcher, `flow stop`) undoes
// exactly that and nothing the user set themselves.
type State struct {
	mu   sync.Mutex
	path string
	data stateData
}

type stateData struct {
	// DND is true when flow turned Do Not Disturb on.
	DND bool `json:"dnd,omitempty"`
	// Slack is true when flow set the Slack status.
	Slack bool `json:"slack,omitempty"`
	// Music lists the players flow paused.
	Music []string `json:"music,omitempty"`
}

// LoadState reads the state in dir. A missing or broken file is an empty
// state; an empty dir keeps the state in memory only.
func LoadState(dir string) *State {
	st := &State{}
	if dir == "" {
		return st
	}
	st.path = filepath.Join(dir, stateFile)
	if data, err := os.ReadFile(st.path); err == nil {
		_ = json.Unmarshal(data, &st.data)
	}
	return st
}

// Save writes the state, removing the file when nothing is pending.
func (s *State) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		return nil
	}
	if !s.data.DND && !s.data.Slack && len(s.data.Music) == 0 {
		err := os.Remove(s.path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	data, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}

func (s *State) update(f func(d *stateData)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(&s.data)
}

func (s *State) get() stateData {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.data
	d.Music = append([]string(nil), s.data.Music...)
	return d
}

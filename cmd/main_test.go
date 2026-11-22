package cmd

import (
	"os"
	"testing"
)

// TestMain keeps command tests from starting the detached background watcher
// (it would re-execute the test binary).
func TestMain(m *testing.M) {
	spawnWatcher = func() error { return nil }
	_ = os.Setenv(noWatcherEnv, "1")
	os.Exit(m.Run())
}

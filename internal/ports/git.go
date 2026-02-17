package ports

import (
	"context"
)

// GitInfo holds git repository context information.
type GitInfo struct {
	Branch     string
	Commit     string
	CommitMsg  string
	Modified   []string
	Untracked  []string
	IsClean    bool
	Repository string
}

// GitDetector defines the interface for git context detection.
// This is a driven port (implemented by adapters).
type GitDetector interface {
	// Detect scans the current directory for git context.
	Detect(ctx context.Context, workingDir string) (*GitInfo, error)

	// IsAvailable checks if git is available in the system.
	IsAvailable() bool
}

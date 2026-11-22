package ports

import (
	"context"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
)

// GitSummarizer computes what changed in a repository since a commit.
// This is a driven port (implemented by the git adapter). It runs git, so
// callers must never call it inside a storage transaction.
type GitSummarizer interface {
	// Summarize returns the commits and line changes in the repository at
	// workingDir ("" for the current directory) between startCommit and the
	// last commit on HEAD made no later than until (HEAD itself when until is
	// zero). With includeWorktree, and when that end commit is HEAD,
	// uncommitted changes to tracked files are counted too, except for the
	// files in dirtyAtStart (repository-relative paths already modified when
	// the session started: only their committed changes count). It returns
	// an error when git or the repository is unavailable, or startCommit is
	// not an ancestor of the end commit.
	Summarize(ctx context.Context, workingDir, startCommit string, until time.Time, includeWorktree bool, dirtyAtStart []string) (*domain.GitSummary, error)
}

// BranchDetector reads the current branch cheaply, without the worktree
// status scan GitDetector.Detect performs. It is optional: adapters that do
// not implement it fall back to Detect.
type BranchDetector interface {
	// CurrentBranch returns the branch checked out at workingDir ("" for the
	// current directory), or "HEAD detached" for a detached HEAD.
	CurrentBranch(ctx context.Context, workingDir string) (string, error)
}

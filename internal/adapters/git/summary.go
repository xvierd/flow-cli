package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

// Ensure Detector implements ports.GitSummarizer.
var _ ports.GitSummarizer = (*Detector)(nil)

// summaryTimeout bounds every git command run for a summary, so a huge or
// locked repository cannot hang the end of a session.
const summaryTimeout = 5 * time.Second

// maxTopDirs is how many of the most changed directories a summary keeps.
const maxTopDirs = 3

// errNoStartCommit is returned when a session has no recorded start commit.
var errNoStartCommit = errors.New("no start commit recorded")

// Summarize implements ports.GitSummarizer with the git command line: go-git
// has no cheap numstat, and the CLI handles linked worktrees natively.
func (d *Detector) Summarize(ctx context.Context, workingDir, startCommit string, until time.Time, includeWorktree bool, dirtyAtStart []string) (*domain.GitSummary, error) {
	if startCommit == "" {
		return nil, errNoStartCommit
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git not found: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, summaryTimeout)
	defer cancel()
	run := func(args ...string) (string, error) {
		return runGit(ctx, workingDir, args...)
	}

	if _, err := run("cat-file", "-e", startCommit+"^{commit}"); err != nil {
		return nil, fmt.Errorf("start commit %s not in this repository", GetShortCommit(startCommit))
	}
	head, err := run("rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve HEAD: %w", err)
	}

	end := head
	if !until.IsZero() {
		// Commits made after the session ended do not belong to it.
		end, err = run("rev-list", "-1", "--before="+strconv.FormatInt(until.Unix(), 10), "HEAD")
		if err != nil || end == "" {
			return nil, fmt.Errorf("no commit before session end")
		}
	}
	if _, err := run("merge-base", "--is-ancestor", startCommit, end); err != nil {
		return nil, fmt.Errorf("start commit %s is not an ancestor of %s", GetShortCommit(startCommit), GetShortCommit(end))
	}

	count, err := run("rev-list", "--count", startCommit+".."+end)
	if err != nil {
		return nil, fmt.Errorf("failed to count commits: %w", err)
	}
	summary := &domain.GitSummary{EndCommit: end}
	summary.Commits, _ = strconv.Atoi(count)

	committedOut, err := run("diff", "--numstat", "-z", "--no-renames", startCommit, end)
	if err != nil {
		return nil, fmt.Errorf("failed to diff: %w", err)
	}
	changes := parseNumstat(committedOut)

	if includeWorktree && end == head {
		worktreeOut, err := run("diff", "--numstat", "-z", "--no-renames", startCommit)
		if err != nil {
			return nil, fmt.Errorf("failed to diff working tree: %w", err)
		}
		dirty := make(map[string]bool, len(dirtyAtStart))
		for _, p := range dirtyAtStart {
			dirty[p] = true
		}
		// Files already dirty at the start keep their committed counts:
		// their uncommitted edits may predate the session.
		for p, wt := range parseNumstat(worktreeOut) {
			if dirty[p] {
				continue
			}
			if c, ok := changes[p]; !ok || c != wt {
				summary.Uncommitted = true
			}
			changes[p] = wt
		}
	}
	applyChanges(summary, changes)
	return summary, nil
}

// runGit runs git in dir and returns its trimmed stdout.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // fixed binary, arguments built here
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("git %s: %s", args[0], msg)
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// lineChange is one file's added and deleted line counts.
type lineChange struct{ added, deleted int }

// parseNumstat parses "git diff --numstat -z --no-renames" output:
// NUL-terminated "<added>\t<deleted>\t<path>" records ("-" counts for
// binary files, read as 0).
func parseNumstat(out string) map[string]lineChange {
	changes := map[string]lineChange{}
	for _, rec := range strings.Split(out, "\x00") {
		fields := strings.SplitN(rec, "\t", 3)
		if len(fields) != 3 || fields[2] == "" {
			continue
		}
		added, _ := strconv.Atoi(fields[0])
		deleted, _ := strconv.Atoi(fields[1])
		changes[fields[2]] = lineChange{added, deleted}
	}
	return changes
}

// applyNumstat fills summary's line and directory counts from numstat output.
func applyNumstat(summary *domain.GitSummary, out string) {
	applyChanges(summary, parseNumstat(out))
}

// applyChanges fills the line and directory counts of summary from per-file
// changes.
func applyChanges(summary *domain.GitSummary, changes map[string]lineChange) {
	dirs := map[string]*domain.DirChange{}
	for file, c := range changes {
		summary.FilesChanged++
		summary.Insertions += c.added
		summary.Deletions += c.deleted

		dir := path.Dir(file)
		if dir == "." {
			dir = "./"
		} else {
			dir += "/"
		}
		dc, ok := dirs[dir]
		if !ok {
			dc = &domain.DirChange{Path: dir}
			dirs[dir] = dc
		}
		dc.Files++
		dc.Insertions += c.added
		dc.Deletions += c.deleted
	}

	summary.DirsChanged = len(dirs)
	top := make([]domain.DirChange, 0, len(dirs))
	for _, dc := range dirs {
		top = append(top, *dc)
	}
	sort.Slice(top, func(i, j int) bool {
		ci, cj := top[i].Insertions+top[i].Deletions, top[j].Insertions+top[j].Deletions
		if ci != cj {
			return ci > cj
		}
		if top[i].Files != top[j].Files {
			return top[i].Files > top[j].Files
		}
		return top[i].Path < top[j].Path
	})
	if len(top) > maxTopDirs {
		top = top[:maxTopDirs]
	}
	if len(top) > 0 {
		summary.TopDirs = top
	}
}

// Ensure Detector implements ports.BranchDetector.
var _ ports.BranchDetector = (*Detector)(nil)

// CurrentBranch implements ports.BranchDetector: it reads HEAD only, without
// scanning the worktree.
func (d *Detector) CurrentBranch(ctx context.Context, workingDir string) (string, error) {
	if workingDir == "" {
		var err error
		if workingDir, err = os.Getwd(); err != nil {
			return "", fmt.Errorf("failed to get working directory: %w", err)
		}
	}
	repoPath, err := findGitRepo(workingDir)
	if err != nil {
		return "", fmt.Errorf("git repository not found: %w", err)
	}
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to open git repository: %w", err)
	}
	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}
	if !head.Name().IsBranch() {
		return "HEAD detached", nil
	}
	return head.Name().Short(), nil
}

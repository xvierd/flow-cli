// Package git provides git context detection using go-git.
package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dvidx/flow-cli/internal/ports"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// Detector implements the ports.GitDetector interface using go-git.
type Detector struct{}

// NewDetector creates a new git detector.
func NewDetector() *Detector {
	return &Detector{}
}

// Ensure Detector implements ports.GitDetector.
var _ ports.GitDetector = (*Detector)(nil)

// Detect scans the working directory for git context.
func (d *Detector) Detect(ctx context.Context, workingDir string) (*ports.GitInfo, error) {
	if workingDir == "" {
		var err error
		workingDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	// Find the git repository by traversing up the directory tree
	repoPath, err := findGitRepo(workingDir)
	if err != nil {
		return nil, fmt.Errorf("git repository not found: %w", err)
	}

	// Open the repository
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}

	// Get current HEAD
	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}

	// Get the current branch name
	branch := head.Name().Short()
	if branch == "HEAD" {
		branch = "HEAD detached"
	}

	// Get commit hash and message
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}

	commitHash := head.Hash().String()
	commitMsg := strings.Split(commit.Message, "\n")[0] // First line only

	// Get repository name from remote URL
	repoName := ""
	remotes, err := repo.Remotes()
	if err == nil && len(remotes) > 0 {
		// Get the first remote's URL
		urls := remotes[0].Config().URLs
		if len(urls) > 0 {
			repoName = extractRepoName(urls[0])
		}
	}

	// Get worktree status for modified and untracked files
	worktree, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	status, err := worktree.Status()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree status: %w", err)
	}

	var modified []string
	var untracked []string
	isClean := status.IsClean()

	for file, s := range status {
		// Check for modified files (Staging != Untracked or Worktree != Unmodified)
		if s.Staging != git.Untracked || s.Worktree != git.Unmodified {
			if s.Worktree == git.Untracked {
				untracked = append(untracked, file)
			} else {
				modified = append(modified, file)
			}
		}
	}

	return &ports.GitInfo{
		Branch:     branch,
		Commit:     commitHash,
		CommitMsg:  commitMsg,
		Modified:   modified,
		Untracked:  untracked,
		IsClean:    isClean,
		Repository: repoName,
	}, nil
}

// IsAvailable checks if git is available in the system.
func (d *Detector) IsAvailable() bool {
	// Try to find a git repository from current directory
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}

	_, err = findGitRepo(cwd)
	return err == nil
}

// findGitRepo traverses up the directory tree to find a .git directory.
func findGitRepo(startPath string) (string, error) {
	currentPath := startPath

	for {
		gitPath := filepath.Join(currentPath, ".git")
		info, err := os.Stat(gitPath)
		if err == nil && info.IsDir() {
			return currentPath, nil
		}

		// Check if this is a git worktree (file containing gitdir reference)
		if err == nil && !info.IsDir() {
			// It's a file, likely a git worktree reference
			content, err := os.ReadFile(gitPath)
			if err == nil && strings.HasPrefix(string(content), "gitdir: ") {
				return currentPath, nil
			}
		}

		// Move up to parent directory
		parent := filepath.Dir(currentPath)
		if parent == currentPath {
			// We've reached the root
			break
		}
		currentPath = parent
	}

	return "", fmt.Errorf("no .git directory found")
}

// extractRepoName extracts the repository name from a git URL.
func extractRepoName(url string) string {
	// Handle SSH URLs like git@github.com:user/repo.git
	if strings.HasPrefix(url, "git@") {
		parts := strings.Split(url, ":")
		if len(parts) >= 2 {
			path := parts[len(parts)-1]
			path = strings.TrimSuffix(path, ".git")
			return path
		}
	}

	// Handle HTTPS URLs like https://github.com/user/repo.git
	if strings.HasPrefix(url, "http") {
		parts := strings.Split(url, "/")
		if len(parts) >= 2 {
			repo := parts[len(parts)-1]
			repo = strings.TrimSuffix(repo, ".git")
			return parts[len(parts)-2] + "/" + repo
		}
	}

	return url
}

// GetShortCommit returns a shortened commit hash.
func GetShortCommit(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

// IsValidCommitHash checks if a string is a valid commit hash.
func IsValidCommitHash(hash string) bool {
	if len(hash) < 7 {
		return false
	}
	for _, c := range hash {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// GetHeadHash returns the hash of the current HEAD commit.
func (d *Detector) GetHeadHash(ctx context.Context, workingDir string) (plumbing.Hash, error) {
	if workingDir == "" {
		var err error
		workingDir, err = os.Getwd()
		if err != nil {
			return plumbing.ZeroHash, fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	repoPath, err := findGitRepo(workingDir)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("failed to open git repository: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("failed to get HEAD: %w", err)
	}

	return head.Hash(), nil
}

package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestNewDetector(t *testing.T) {
	d := NewDetector()
	if d == nil {
		t.Fatal("NewDetector() returned nil")
	}
}

func TestDetector_IsAvailable(t *testing.T) {
	d := NewDetector()

	// This test may pass or fail depending on whether we're in a git repo
	// We just verify it doesn't panic
	_ = d.IsAvailable()
}

func TestDetector_Detect(t *testing.T) {
	// Create a temporary directory with a git repo
	tmpDir, err := os.MkdirTemp("", "flow-git-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize a git repo
	repo, err := git.PlainInit(tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Get worktree
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Failed to get worktree: %v", err)
	}

	// Add the file
	if _, err := worktree.Add("test.txt"); err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}

	// Create initial commit
	commit, err := worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create commit: %v", err)
	}

	d := NewDetector()
	ctx := context.Background()

	info, err := d.Detect(ctx, tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if info == nil {
		t.Fatal("Detect() returned nil info")
	}

	// Verify the commit hash matches
	if info.Commit != commit.String() {
		t.Errorf("Expected commit %s, got %s", commit.String(), info.Commit)
	}

	// Verify we're on main/master branch (go-git defaults to master)
	if info.Branch != "master" && info.Branch != "main" {
		t.Errorf("Unexpected branch: %s", info.Branch)
	}

	if info.CommitMsg != "Initial commit" {
		t.Errorf("Expected commit message 'Initial commit', got '%s'", info.CommitMsg)
	}

	if !info.IsClean {
		t.Error("Expected clean worktree after commit")
	}
}

func TestDetector_Detect_WithModifiedFiles(t *testing.T) {
	// Create a temporary directory with a git repo
	tmpDir, err := os.MkdirTemp("", "flow-git-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize a git repo
	repo, err := git.PlainInit(tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Create and commit a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Failed to get worktree: %v", err)
	}

	if _, err := worktree.Add("test.txt"); err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}

	if _, err := worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
		},
	}); err != nil {
		t.Fatalf("Failed to create commit: %v", err)
	}

	// Modify the file
	if err := os.WriteFile(testFile, []byte("modified content"), 0644); err != nil {
		t.Fatalf("Failed to modify test file: %v", err)
	}

	// Create a new untracked file
	untrackedFile := filepath.Join(tmpDir, "untracked.txt")
	if err := os.WriteFile(untrackedFile, []byte("untracked"), 0644); err != nil {
		t.Fatalf("Failed to create untracked file: %v", err)
	}

	d := NewDetector()
	ctx := context.Background()

	info, err := d.Detect(ctx, tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if info.IsClean {
		t.Error("Expected dirty worktree with modified files")
	}

	foundModified := false
	for _, f := range info.Modified {
		if f == "test.txt" {
			foundModified = true
			break
		}
	}
	if !foundModified {
		t.Error("Expected test.txt in modified files")
	}

	foundUntracked := false
	for _, f := range info.Untracked {
		if f == "untracked.txt" {
			foundUntracked = true
			break
		}
	}
	if !foundUntracked {
		t.Error("Expected untracked.txt in untracked files")
	}
}

func TestDetector_Detect_NoGitRepo(t *testing.T) {
	// Create a temporary directory without git
	tmpDir, err := os.MkdirTemp("", "flow-git-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	d := NewDetector()
	ctx := context.Background()

	_, err = d.Detect(ctx, tmpDir)
	if err == nil {
		t.Error("Expected error when no git repo exists")
	}
}

func TestFindGitRepo(t *testing.T) {
	// Create a temporary directory structure
	tmpDir, err := os.MkdirTemp("", "flow-git-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a subdirectory structure
	subDir := filepath.Join(tmpDir, "level1", "level2")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	// Initialize git in root
	if _, err := git.PlainInit(tmpDir, false); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Try to find repo from subdirectory
	found, err := findGitRepo(subDir)
	if err != nil {
		t.Fatalf("findGitRepo() error = %v", err)
	}

	if found != tmpDir {
		t.Errorf("Expected repo at %s, found at %s", tmpDir, found)
	}
}

func TestFindGitRepo_NotFound(t *testing.T) {
	// Create a temporary directory without git
	tmpDir, err := os.MkdirTemp("", "flow-git-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	_, err = findGitRepo(tmpDir)
	if err == nil {
		t.Error("Expected error when no git repo exists")
	}
}

func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"git@github.com:user/repo.git", "user/repo"},
		{"https://github.com/user/repo.git", "user/repo"},
		{"https://gitlab.com/org/project.git", "org/project"},
		{"git@bitbucket.org:team/repo.git", "team/repo"},
		{"/path/to/repo", "/path/to/repo"}, // Local path fallback
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := extractRepoName(tt.url)
			if result != tt.expected {
				t.Errorf("extractRepoName(%q) = %q, want %q", tt.url, result, tt.expected)
			}
		})
	}
}

func TestGetShortCommit(t *testing.T) {
	tests := []struct {
		commit   string
		expected string
	}{
		{"abcdef1234567890abcdef1234567890abcdef12", "abcdef1"},
		{"short", "short"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.commit, func(t *testing.T) {
			result := GetShortCommit(tt.commit)
			if result != tt.expected {
				t.Errorf("GetShortCommit(%q) = %q, want %q", tt.commit, result, tt.expected)
			}
		})
	}
}

func TestIsValidCommitHash(t *testing.T) {
	tests := []struct {
		hash     string
		expected bool
	}{
		{"abcdef1234567890abcdef1234567890abcdef12", true},
		{"ABCDEF1234567890ABCDEF1234567890ABCDEF12", true},
		{"1234567890abcdef", true},
		{"short", false},
		{"", false},
		{"notahexstring!", false},
	}

	for _, tt := range tests {
		t.Run(tt.hash, func(t *testing.T) {
			result := IsValidCommitHash(tt.hash)
			if result != tt.expected {
				t.Errorf("IsValidCommitHash(%q) = %v, want %v", tt.hash, result, tt.expected)
			}
		})
	}
}

package git

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/xvierd/flow-cli/internal/ports"
)

// QuickInfo reads the repository name, branch and HEAD commit of dir straight
// from the .git files, without opening the repository or computing the
// worktree status. It is meant for latency-sensitive callers (the Claude Code
// hooks) and understands linked worktrees. It returns nil outside a
// repository. Modified/Untracked are never filled in.
func QuickInfo(dir string) *ports.GitInfo {
	top, err := findGitRepo(dir)
	if err != nil {
		return nil
	}
	gitDir := filepath.Join(top, ".git")
	if info, err := os.Stat(gitDir); err == nil && !info.IsDir() {
		content, err := os.ReadFile(gitDir) // #nosec G304 -- .git file inside the repository
		if err != nil {
			return nil
		}
		ref := strings.TrimSpace(strings.TrimPrefix(string(content), "gitdir:"))
		if !filepath.IsAbs(ref) {
			ref = filepath.Join(top, ref)
		}
		gitDir = ref
	}

	// Linked worktrees keep HEAD in their own git dir and refs/config in the
	// common one.
	commonDir := gitDir
	if content, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil { // #nosec G304 G703 -- files inside the repo's own .git dir
		c := strings.TrimSpace(string(content))
		if !filepath.IsAbs(c) {
			c = filepath.Join(gitDir, c)
		}
		commonDir = filepath.Clean(c)
	}

	info := &ports.GitInfo{}
	if head, err := os.ReadFile(filepath.Join(gitDir, "HEAD")); err == nil { // #nosec G304 G703 -- files inside the repo's own .git dir
		h := strings.TrimSpace(string(head))
		if ref, ok := strings.CutPrefix(h, "ref: "); ok {
			info.Branch = strings.TrimPrefix(ref, "refs/heads/")
			info.Commit = resolveRef(gitDir, commonDir, ref)
		} else {
			info.Branch = "HEAD detached"
			info.Commit = h
		}
	}

	info.Repository = originName(filepath.Join(commonDir, "config"))
	if info.Repository == "" {
		// The main checkout's directory names the repository (the common dir
		// is <checkout>/.git for worktrees too).
		info.Repository = filepath.Base(filepath.Dir(commonDir))
		if filepath.Base(commonDir) != ".git" {
			info.Repository = strings.TrimSuffix(filepath.Base(commonDir), ".git")
		}
	}
	return info
}

// resolveRef returns the commit a ref points to, from loose refs or
// packed-refs, or "" when it cannot be found.
func resolveRef(gitDir, commonDir, ref string) string {
	for _, d := range []string{gitDir, commonDir} {
		if b, err := os.ReadFile(filepath.Join(d, filepath.FromSlash(ref))); err == nil { // #nosec G304 G703 -- files inside the repo's own .git dir
			return strings.TrimSpace(string(b))
		}
	}
	f, err := os.Open(filepath.Join(commonDir, "packed-refs")) // #nosec G304 G703 -- files inside the repo's own .git dir
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if hash, name, ok := strings.Cut(sc.Text(), " "); ok && name == ref {
			return hash
		}
	}
	return ""
}

// originName returns the repository name (owner/repo) of the "origin"
// remote, or of the first remote, in a git config file.
func originName(configPath string) string {
	f, err := os.Open(configPath) // #nosec G304 G703 -- files inside the repo's own .git dir
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	var section, first, origin string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			section = line
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "url" || !strings.HasPrefix(section, "[remote ") {
			continue
		}
		val = strings.TrimSpace(val)
		if first == "" {
			first = val
		}
		if section == `[remote "origin"]` {
			origin = val
		}
	}
	if origin == "" {
		origin = first
	}
	if origin == "" {
		return ""
	}
	return extractRepoName(origin)
}

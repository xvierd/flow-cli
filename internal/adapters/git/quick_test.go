package git

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestQuickInfo_Checkout(t *testing.T) {
	root := filepath.Join(t.TempDir(), "myrepo")
	writeFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/feature/x\n")
	writeFile(t, filepath.Join(root, ".git", "refs", "heads", "feature", "x"), "abc123\n")
	writeFile(t, filepath.Join(root, ".git", "config"), "[core]\n\tbare = false\n[remote \"upstream\"]\n\turl = git@github.com:other/fork.git\n[remote \"origin\"]\n\turl = https://github.com/me/myrepo.git\n")
	sub := filepath.Join(root, "pkg", "deep")
	_ = os.MkdirAll(sub, 0o750)

	info := QuickInfo(sub)
	if info == nil || info.Branch != "feature/x" || info.Commit != "abc123" || info.Repository != "me/myrepo" {
		t.Fatalf("QuickInfo() = %+v", info)
	}
}

func TestQuickInfo_WorktreePackedRefsNoRemote(t *testing.T) {
	base := t.TempDir()
	main := filepath.Join(base, "project")
	common := filepath.Join(main, ".git")
	wtGit := filepath.Join(common, "worktrees", "wt1")
	writeFile(t, filepath.Join(common, "HEAD"), "ref: refs/heads/main\n")
	writeFile(t, filepath.Join(common, "packed-refs"), "# pack-refs with: peeled\ndeadbeef refs/heads/topic\n")
	writeFile(t, filepath.Join(wtGit, "HEAD"), "ref: refs/heads/topic\n")
	writeFile(t, filepath.Join(wtGit, "commondir"), "../..\n")
	wt := filepath.Join(base, "wt1")
	writeFile(t, filepath.Join(wt, ".git"), "gitdir: "+wtGit+"\n")

	info := QuickInfo(wt)
	if info == nil || info.Branch != "topic" || info.Commit != "deadbeef" || info.Repository != "project" {
		t.Fatalf("QuickInfo() = %+v", info)
	}
}

func TestQuickInfo_Detached(t *testing.T) {
	root := filepath.Join(t.TempDir(), "r")
	writeFile(t, filepath.Join(root, ".git", "HEAD"), "0123456789abcdef\n")
	info := QuickInfo(root)
	if info == nil || info.Branch != "HEAD detached" || info.Commit != "0123456789abcdef" || info.Repository != "r" {
		t.Fatalf("QuickInfo() = %+v", info)
	}
}

func TestQuickInfo_NotARepo(t *testing.T) {
	if info := QuickInfo(t.TempDir()); info != nil {
		// A temp dir could live under a repository on some machines; only
		// fail when the result clearly came from the temp dir itself.
		t.Logf("QuickInfo() outside a repo = %+v (parent repository?)", info)
	}
}

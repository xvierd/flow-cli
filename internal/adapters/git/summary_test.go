package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
)

// testRepo is a throwaway git repository driven by the git CLI.
type testRepo struct {
	t   *testing.T
	dir string
}

func newTestRepo(t *testing.T) *testRepo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	r := &testRepo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "feat/auth-refresh")
	r.git("config", "user.email", "test@example.com")
	r.git("config", "user.name", "Test")
	r.git("config", "commit.gpgsign", "false")
	return r
}

// git runs git in the repository; env pins commit dates when given.
func (r *testRepo) git(args ...string) string {
	r.t.Helper()
	return r.gitEnv(nil, args...)
}

func (r *testRepo) gitEnv(env []string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...) //nolint:gosec // test helper, fixed binary
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r *testRepo) write(name, content string) {
	r.t.Helper()
	path := filepath.Join(r.dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		r.t.Fatal(err)
	}
}

// commit commits everything at the given time and returns the hash.
func (r *testRepo) commit(msg string, at time.Time) string {
	r.t.Helper()
	date := at.Format(time.RFC3339)
	r.git("add", "-A")
	r.gitEnv([]string{"GIT_AUTHOR_DATE=" + date, "GIT_COMMITTER_DATE=" + date}, "commit", "-q", "-m", msg)
	return r.git("rev-parse", "HEAD")
}

func lines(n int) string { return strings.Repeat("x\n", n) }

func TestSummarize(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	base := time.Now().Add(-2 * time.Hour).Truncate(time.Second)

	r.write("README.md", lines(10))
	r.write("internal/auth/token.go", lines(40))
	start := r.commit("initial", base)

	// During the session: two commits touching internal/auth and cmd.
	r.write("internal/auth/token.go", lines(100))       // +60
	r.write("internal/auth/refresh.go", lines(60))      // +60
	r.commit("feat: refresh", base.Add(10*time.Minute)) // commit 1
	r.write("cmd/login.go", lines(5))                   // +5
	r.write("README.md", lines(6))                      // −4
	sessionEnd := base.Add(30 * time.Minute)
	r.commit("feat: login", base.Add(20*time.Minute)) // commit 2

	// After the session: a commit that must not count when bounded.
	r.write("docs/later.md", lines(7))
	r.commit("docs", base.Add(60*time.Minute))

	d := NewDetector()

	t.Run("bounded by session end", func(t *testing.T) {
		got, err := d.Summarize(ctx, r.dir, start, sessionEnd, false, nil)
		if err != nil {
			t.Fatalf("Summarize() error = %v", err)
		}
		if got.Commits != 2 || got.FilesChanged != 4 || got.Insertions != 125 || got.Deletions != 4 || got.DirsChanged != 3 {
			t.Errorf("summary = %+v, want 2 commits, 4 files, +125 −4, 3 dirs", got)
		}
		if len(got.TopDirs) == 0 || got.TopDirs[0].Path != "internal/auth/" || got.TopDirs[0].Insertions != 120 {
			t.Errorf("top dirs = %+v, want internal/auth/ first with +120", got.TopDirs)
		}
		if got.Uncommitted {
			t.Error("bounded summary should not include the worktree")
		}
		if want := "2 commits · internal/auth/ +2 more (+125 −4)"; got.String() != want {
			t.Errorf("String() = %q, want %q", got.String(), want)
		}
	})

	t.Run("to HEAD with uncommitted changes", func(t *testing.T) {
		r.write("internal/auth/token.go", lines(103)) // +3 uncommitted
		r.write("untracked.txt", "ignored\n")         // untracked files are not counted
		got, err := d.Summarize(ctx, r.dir, start, time.Time{}, true, nil)
		if err != nil {
			t.Fatalf("Summarize() error = %v", err)
		}
		if got.Commits != 3 || !got.Uncommitted || got.Insertions != 135 {
			t.Errorf("summary = %+v, want 3 commits, uncommitted, +135", got)
		}
		if head := r.git("rev-parse", "HEAD"); got.EndCommit != head {
			t.Errorf("EndCommit = %s, want HEAD %s", got.EndCommit, head)
		}
	})

	t.Run("files dirty at start keep committed counts", func(t *testing.T) {
		// token.go has 3 uncommitted lines (from the previous subtest);
		// if it was already dirty when the session started they are not
		// the session's work.
		got, err := d.Summarize(ctx, r.dir, start, time.Time{}, true, []string{"internal/auth/token.go"})
		if err != nil {
			t.Fatalf("Summarize() error = %v", err)
		}
		if got.Insertions != 132 || got.Uncommitted {
			t.Errorf("summary = %+v, want +132 and no uncommitted changes", got)
		}
	})

	t.Run("no changes", func(t *testing.T) {
		head := r.git("rev-parse", "HEAD")
		got, err := d.Summarize(ctx, r.dir, head, time.Time{}, false, nil)
		if err != nil {
			t.Fatalf("Summarize() error = %v", err)
		}
		if !got.IsEmpty() {
			t.Errorf("summary = %+v, want empty", got)
		}
	})

	t.Run("unknown start commit", func(t *testing.T) {
		if _, err := d.Summarize(ctx, r.dir, strings.Repeat("a", 40), time.Time{}, true, nil); err == nil {
			t.Error("Summarize() with a foreign commit should fail")
		}
	})

	t.Run("start not an ancestor", func(t *testing.T) {
		r.git("checkout", "-q", "--orphan", "other")
		r.git("rm", "-rqf", "--cached", ".")
		r.write("other.txt", "x\n")
		r.git("add", "other.txt")
		r.commit("orphan", base.Add(90*time.Minute))
		if _, err := d.Summarize(ctx, r.dir, start, time.Time{}, false, nil); err == nil {
			t.Error("Summarize() across unrelated history should fail")
		}
	})

	t.Run("empty start commit", func(t *testing.T) {
		if _, err := d.Summarize(ctx, r.dir, "", time.Time{}, true, nil); err == nil {
			t.Error("Summarize() without a start commit should fail")
		}
	})
}

func TestSummarize_NotARepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	if _, err := NewDetector().Summarize(context.Background(), dir, strings.Repeat("a", 40), time.Time{}, true, nil); err == nil {
		t.Error("Summarize() outside a repository should fail")
	}
}

func TestCurrentBranch(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("a.txt", "a\n")
	first := r.commit("a", time.Now())

	d := NewDetector()
	if got, err := d.CurrentBranch(ctx, r.dir); err != nil || got != "feat/auth-refresh" {
		t.Errorf("CurrentBranch() = %q, %v, want feat/auth-refresh", got, err)
	}
	r.git("checkout", "-q", "--detach", first)
	if got, err := d.CurrentBranch(ctx, r.dir); err != nil || got != "HEAD detached" {
		t.Errorf("CurrentBranch() detached = %q, %v, want HEAD detached", got, err)
	}
	if _, err := d.CurrentBranch(ctx, t.TempDir()); err == nil {
		t.Error("CurrentBranch() outside a repository should fail")
	}
}

func TestApplyNumstat_Binary(t *testing.T) {
	summary := &domain.GitSummary{}
	applyNumstat(summary, "-\t-\tassets/logo.png\x003\t1\tmain.go\x00")
	if summary.FilesChanged != 2 || summary.Insertions != 3 || summary.Deletions != 1 {
		t.Errorf("summary = %+v, want 2 files, +3 −1", summary)
	}
	if summary.DirsChanged != 2 || summary.TopDirs[0].Path != "./" {
		t.Errorf("dirs = %d %+v, want root first", summary.DirsChanged, summary.TopDirs)
	}
}

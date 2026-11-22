package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

// fakeSummarizer is a git detector and summarizer that fails the test when
// git runs inside a transaction.
type fakeSummarizer struct {
	txCheckingDetector
	summary *domain.GitSummary
	err     error
	calls   int
	// lastUntil / lastWorktree record the latest Summarize arguments.
	lastUntil    time.Time
	lastWorktree bool
}

func (f *fakeSummarizer) Summarize(_ context.Context, _, startCommit string, until time.Time, includeWorktree bool, _ []string) (*domain.GitSummary, error) {
	f.calls++
	if *f.inTx {
		f.t.Error("git summary ran inside a transaction")
	}
	if startCommit != "abc123" {
		f.t.Errorf("Summarize start commit = %q, want abc123", startCommit)
	}
	f.lastUntil, f.lastWorktree = until, includeWorktree
	if f.err != nil {
		return nil, f.err
	}
	s := *f.summary
	return &s, nil
}

func newSummaryFixture(t *testing.T) (*PomodoroService, ports.Storage, *fakeSummarizer) {
	t.Helper()
	base, cleanup := setupTestStorage(t)
	t.Cleanup(cleanup)
	inTx := false
	store := txFlagStorage{Storage: base, inTx: &inTx}
	fake := &fakeSummarizer{
		txCheckingDetector: txCheckingDetector{t: t, inTx: &inTx},
		summary: &domain.GitSummary{
			Commits: 3, FilesChanged: 4, Insertions: 120, Deletions: 40, DirsChanged: 1,
			TopDirs: []domain.DirChange{{Path: "internal/auth/", Files: 4, Insertions: 120, Deletions: 40}},
		},
	}
	return NewPomodoroService(store, fake), base, fake
}

// TestPomodoroService_EndOfSessionGitSummary checks that every way a work
// session ends records its git summary, computed outside the transaction.
func TestPomodoroService_EndOfSessionGitSummary(t *testing.T) {
	ctx := context.Background()
	ends := map[string]func(*PomodoroService, ports.Storage) (*domain.PomodoroSession, error){
		"stop": func(p *PomodoroService, _ ports.Storage) (*domain.PomodoroSession, error) {
			return p.StopSession(ctx)
		},
		"void": func(p *PomodoroService, _ ports.Storage) (*domain.PomodoroSession, error) {
			return p.VoidSession(ctx)
		},
		"expire": func(p *PomodoroService, store ports.Storage) (*domain.PomodoroSession, error) {
			active, _ := store.Sessions().FindActive(ctx)
			active.StartedAt = time.Now().Add(-2 * active.Duration)
			if err := store.Sessions().Update(ctx, active); err != nil {
				return nil, err
			}
			return p.CompleteIfExpired(ctx)
		},
	}
	for name, end := range ends {
		t.Run(name, func(t *testing.T) {
			service, store, fake := newSummaryFixture(t)
			if _, err := service.StartPomodoro(ctx, StartPomodoroRequest{WorkingDir: "."}); err != nil {
				t.Fatalf("StartPomodoro() error = %v", err)
			}
			ended, err := end(service, store)
			if err != nil || ended == nil {
				t.Fatalf("end session = %v, %v", ended, err)
			}
			if fake.calls != 1 {
				t.Errorf("Summarize called %d times, want 1", fake.calls)
			}
			if !fake.lastWorktree {
				t.Error("a just-ended session should count uncommitted changes")
			}
			if ended.GitSummary == nil || ended.GitSummary.Commits != 3 {
				t.Errorf("returned GitSummary = %+v, want 3 commits", ended.GitSummary)
			}
			stored, err := store.Sessions().FindByID(ctx, ended.ID)
			if err != nil {
				t.Fatalf("FindByID() error = %v", err)
			}
			if stored.GitSummary == nil || stored.GitSummary.Insertions != 120 {
				t.Fatalf("stored GitSummary = %+v, want +120", stored.GitSummary)
			}
			if got, want := stored.GitSummary.String(), "3 commits · internal/auth/ (+120 −40)"; got != want {
				t.Errorf("String() = %q, want %q", got, want)
			}
		})
	}
}

// TestPomodoroService_GitSummaryTolerated checks that a failing summarizer
// (no git, foreign repository) and a session without a start commit end
// normally, without a summary.
func TestPomodoroService_GitSummaryTolerated(t *testing.T) {
	ctx := context.Background()
	service, store, fake := newSummaryFixture(t)
	fake.err = errors.New("git repository not found")
	if _, err := service.StartPomodoro(ctx, StartPomodoroRequest{WorkingDir: "."}); err != nil {
		t.Fatalf("StartPomodoro() error = %v", err)
	}
	ended, err := service.StopSession(ctx)
	if err != nil {
		t.Fatalf("StopSession() error = %v", err)
	}
	if ended.GitSummary != nil {
		t.Errorf("GitSummary = %+v, want nil", ended.GitSummary)
	}

	// No start commit (git unavailable at start): no git call at all.
	fake.calls = 0
	session := domain.NewPomodoroSession(domain.DefaultPomodoroConfig(), nil)
	session.Complete()
	if err := store.Sessions().Save(ctx, session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	service.EnsureGitSummaries(ctx, []*domain.PomodoroSession{session})
	if fake.calls != 0 {
		t.Errorf("Summarize called %d times for a session without a commit", fake.calls)
	}
}

// TestPomodoroService_EnsureGitSummariesBackfill checks that an old ended
// session without a summary gets one bounded by its end time, committed
// changes only, and that cancelled sessions and breaks are skipped.
func TestPomodoroService_EnsureGitSummariesBackfill(t *testing.T) {
	ctx := context.Background()
	service, store, fake := newSummaryFixture(t)

	ended := time.Now().Add(-3 * time.Hour)
	old := domain.NewPomodoroSession(domain.DefaultPomodoroConfig(), nil)
	old.SetGitContext("feat/auth-refresh", "abc123", nil)
	old.StartedAt = ended.Add(-25 * time.Minute)
	old.Status = domain.SessionStatusCompleted
	old.CompletedAt = &ended
	cancelled := domain.NewPomodoroSession(domain.DefaultPomodoroConfig(), nil)
	cancelled.SetGitContext("feat/x", "abc123", nil)
	cancelled.Status = domain.SessionStatusCancelled
	for _, s := range []*domain.PomodoroSession{old, cancelled} {
		if err := store.Sessions().Save(ctx, s); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}

	recent, err := service.GetRecentSessions(ctx, 10)
	if err != nil {
		t.Fatalf("GetRecentSessions() error = %v", err)
	}
	if fake.calls != 1 {
		t.Fatalf("Summarize called %d times, want 1 (cancelled skipped)", fake.calls)
	}
	if !fake.lastUntil.Equal(ended) || fake.lastWorktree {
		t.Errorf("Summarize(until=%v, worktree=%v), want until=%v, worktree=false", fake.lastUntil, fake.lastWorktree, ended)
	}
	var got *domain.PomodoroSession
	for _, s := range recent {
		if s.ID == old.ID {
			got = s
		}
	}
	if got == nil || got.GitSummary == nil {
		t.Fatalf("returned session has no summary: %+v", got)
	}

	// Stored: a second read does not run git again.
	if _, err := service.GetRecentSessions(ctx, 10); err != nil {
		t.Fatalf("GetRecentSessions() error = %v", err)
	}
	if fake.calls != 1 {
		t.Errorf("Summarize called %d times after the summary was stored, want 1", fake.calls)
	}
}

// TestPomodoroService_SuggestedTaskTitle checks the branch-derived title,
// including no suggestion on main.
func TestPomodoroService_SuggestedTaskTitle(t *testing.T) {
	service, _, _ := newSummaryFixture(t)
	// txCheckingDetector reports branch "main".
	if got := service.SuggestedTaskTitle(context.Background(), "."); got != "" {
		t.Errorf("SuggestedTaskTitle on main = %q, want \"\"", got)
	}
}

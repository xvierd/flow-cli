package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/xvierd/flow-cli/internal/domain"
)

// newV3DB builds a database migrated up to version 3, the schema before
// sessions.git_summary existed.
func newV3DB(t *testing.T) (*sql.DB, *sqliteStorage) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	s := &sqliteStorage{db: db, taskRepo: newTaskRepository(db), sessionRepo: newSessionRepository(db)}
	for _, step := range []func() error{s.migrateBase, s.migrateTags, s.migrateOneActive} {
		if err := step(); err != nil {
			t.Fatalf("migrate to v3 error = %v", err)
		}
	}
	if v, _ := s.userVersion(); v != schemaVersionOneActive {
		t.Fatalf("fixture user_version = %d, want %d", v, schemaVersionOneActive)
	}
	return db, s
}

func TestMigrate_V3ToGitSummary(t *testing.T) {
	ctx := context.Background()
	db, s := newV3DB(t)

	// A legacy row: NULL git columns, as written by very old versions.
	if _, err := db.Exec(`INSERT INTO sessions (id, type, status, duration_ms, started_at, git_branch, git_commit)
		VALUES ('legacy', 'work', 'completed', 1500000, '2026-01-01 10:00:00', NULL, NULL)`); err != nil {
		t.Fatalf("insert legacy session error = %v", err)
	}

	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if v, _ := s.userVersion(); v != schemaVersionGitSummary {
		t.Errorf("user_version = %d, want %d", v, schemaVersionGitSummary)
	}

	legacy, err := s.Sessions().FindByID(ctx, "legacy")
	if err != nil {
		t.Fatalf("FindByID(legacy) error = %v", err)
	}
	if legacy.GitSummary != nil || legacy.GitBranch != "" {
		t.Errorf("legacy session git = %q / %+v, want empty / nil", legacy.GitBranch, legacy.GitSummary)
	}

	// Re-running is a no-op.
	if err := s.Migrate(); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}
}

func TestMigrate_GitSummaryColumnAlreadyPresent(t *testing.T) {
	// A step interrupted after the ALTER but before user_version moved.
	db, s := newV3DB(t)
	if _, err := db.Exec("ALTER TABLE sessions ADD COLUMN git_summary TEXT"); err != nil {
		t.Fatalf("pre-add column error = %v", err)
	}
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if v, _ := s.userVersion(); v != schemaVersionGitSummary {
		t.Errorf("user_version = %d, want %d", v, schemaVersionGitSummary)
	}
}

func TestSessionRepository_GitSummaryRoundTrip(t *testing.T) {
	ctx := context.Background()
	store, err := NewMemory()
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	defer func() { _ = store.Close() }()

	session := domain.NewPomodoroSession(domain.DefaultPomodoroConfig(), nil)
	session.SetGitContext("feat/auth-refresh", "abc123", nil)
	session.Complete()
	if err := store.Sessions().Save(ctx, session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	summary := &domain.GitSummary{
		EndCommit: "def456", Commits: 3, FilesChanged: 4, Insertions: 120, Deletions: 40, DirsChanged: 1,
		TopDirs: []domain.DirChange{{Path: "internal/auth/", Files: 4, Insertions: 120, Deletions: 40}},
	}
	if err := store.Sessions().UpdateGitSummary(ctx, session.ID, summary); err != nil {
		t.Fatalf("UpdateGitSummary() error = %v", err)
	}

	// A later full Update (focus score, ritual, …) keeps the summary.
	score := 4
	session.FocusScore = &score
	if err := store.Sessions().Update(ctx, session); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := store.Sessions().FindByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if got.GitSummary == nil || got.GitSummary.Commits != 3 || got.GitSummary.EndCommit != "def456" ||
		len(got.GitSummary.TopDirs) != 1 || got.GitSummary.TopDirs[0].Path != "internal/auth/" {
		t.Errorf("GitSummary = %+v", got.GitSummary)
	}

	recent, err := store.Sessions().FindRecent(ctx, session.StartedAt.Add(-1))
	if err != nil || len(recent) != 1 || recent[0].GitSummary == nil {
		t.Errorf("FindRecent() = %v, %v, want the session with its summary", recent, err)
	}

	// nil clears it.
	if err := store.Sessions().UpdateGitSummary(ctx, session.ID, nil); err != nil {
		t.Fatalf("UpdateGitSummary(nil) error = %v", err)
	}
	if got, _ := store.Sessions().FindByID(ctx, session.ID); got.GitSummary != nil {
		t.Errorf("GitSummary after clear = %+v, want nil", got.GitSummary)
	}

	if err := store.Sessions().UpdateGitSummary(ctx, "missing", summary); !errors.Is(err, domain.ErrSessionNotFound) {
		t.Errorf("UpdateGitSummary(missing) error = %v, want ErrSessionNotFound", err)
	}
}

func TestSessionRepository_UnreadableGitSummary(t *testing.T) {
	ctx := context.Background()
	store, err := NewMemory()
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	defer func() { _ = store.Close() }()
	session := domain.NewPomodoroSession(domain.DefaultPomodoroConfig(), nil)
	if err := store.Sessions().Save(ctx, session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := store.(*sqliteStorage).db.Exec("UPDATE sessions SET git_summary = 'not json' WHERE id = ?", session.ID); err != nil {
		t.Fatalf("corrupt column error = %v", err)
	}
	got, err := store.Sessions().FindByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if got.GitSummary != nil {
		t.Errorf("GitSummary = %+v, want nil for unreadable JSON", got.GitSummary)
	}
}

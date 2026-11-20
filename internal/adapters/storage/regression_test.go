package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

// pragmaInt reads an integer PRAGMA through the given executor.
func pragmaInt(t *testing.T, ex executor, name string) int {
	t.Helper()
	var v int
	if err := ex.QueryRowContext(context.Background(), "PRAGMA "+name).Scan(&v); err != nil {
		t.Fatalf("PRAGMA %s error = %v", name, err)
	}
	return v
}

func TestNew_PragmasApplyToEveryConnection(t *testing.T) {
	// Special characters in the directory and file name must not be eaten by
	// DSN parsing ('?' starts the driver's query string).
	dir := filepath.Join(t.TempDir(), "we?ird #dir% &x")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	dbPath := filepath.Join(dir, "flow?v=1#a%20b.db")

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = store.Close() }()

	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database not created at the exact path %q: %v", dbPath, err)
	}

	db := store.(*sqliteStorage).db
	check := func(stage string) {
		t.Helper()
		if got := pragmaInt(t, db, "foreign_keys"); got != 1 {
			t.Errorf("%s: foreign_keys = %d, want 1", stage, got)
		}
		if got := pragmaInt(t, db, "busy_timeout"); got < 1000 {
			t.Errorf("%s: busy_timeout = %d, want >= 1000", stage, got)
		}
		var mode string
		if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
			t.Fatalf("%s: journal_mode error = %v", stage, err)
		}
		if !strings.EqualFold(mode, "wal") {
			t.Errorf("%s: journal_mode = %q, want wal", stage, mode)
		}
	}
	check("initial")

	// Turn foreign keys off on the live connection, then make database/sql
	// discard it. The replacement connection must get the pragmas again.
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("Conn() error = %v", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("disable foreign keys error = %v", err)
	}
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = conn.Close()

	check("after reconnect")

	// The cascade still works on the new connection.
	task, _ := domain.NewTask("cascade")
	task.Tags = []string{"x"}
	if err := store.Tasks().Save(ctx, task); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := store.Tasks().Delete(ctx, task.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if got := tagsFor(t, db, "SELECT tag FROM task_tags WHERE task_id = ?", task.ID); len(got) != 0 {
		t.Errorf("orphan task_tags after delete = %v, want none", got)
	}
}

func TestNew_MemoryStillWorks(t *testing.T) {
	store, err := NewMemory()
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	defer func() { _ = store.Close() }()

	db := store.(*sqliteStorage).db
	if got := pragmaInt(t, db, "foreign_keys"); got != 1 {
		t.Errorf("foreign_keys = %d, want 1", got)
	}
	if _, err := os.Stat(":memory:"); err == nil {
		t.Error(`a file named ":memory:" was created`)
	}
}

func TestNew_SecondProcessWaitsForWriteLock(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "flow.db")
	a, err := New(dbPath)
	if err != nil {
		t.Fatalf("New(a) error = %v", err)
	}
	defer func() { _ = a.Close() }()
	b, err := New(dbPath)
	if err != nil {
		t.Fatalf("New(b) error = %v", err)
	}
	defer func() { _ = b.Close() }()

	ctx := context.Background()
	done := make(chan error, 1)
	err = a.WithTx(ctx, func(tx ports.Storage) error {
		task, _ := domain.NewTask("a")
		if err := tx.Tasks().Save(ctx, task); err != nil {
			return err
		}
		go func() {
			other, _ := domain.NewTask("b")
			done <- b.Tasks().Save(ctx, other)
		}()
		time.Sleep(150 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Fatalf("WithTx(a) error = %v", err)
	}
	if err := <-done; err != nil {
		t.Errorf("Save(b) while a held the write lock error = %v, want it to wait", err)
	}
}

func TestWithTx_ReadThenWriteIsNotBusySnapshot(t *testing.T) {
	// A transaction that reads first and writes later must not fail when
	// another connection commits in between (SQLITE_BUSY_SNAPSHOT with
	// DEFERRED transactions).
	dbPath := filepath.Join(t.TempDir(), "flow.db")
	a, err := New(dbPath)
	if err != nil {
		t.Fatalf("New(a) error = %v", err)
	}
	defer func() { _ = a.Close() }()
	b, err := New(dbPath)
	if err != nil {
		t.Fatalf("New(b) error = %v", err)
	}
	defer func() { _ = b.Close() }()

	ctx := context.Background()
	done := make(chan error, 1)
	err = a.WithTx(ctx, func(tx ports.Storage) error {
		if _, err := tx.Sessions().FindActive(ctx); err != nil {
			return err
		}
		go func() {
			other, _ := domain.NewTask("b")
			done <- b.Tasks().Save(ctx, other)
		}()
		time.Sleep(100 * time.Millisecond)
		task, _ := domain.NewTask("a")
		return tx.Tasks().Save(ctx, task)
	})
	if err != nil {
		t.Fatalf("WithTx(a) error = %v", err)
	}
	if err := <-done; err != nil {
		t.Errorf("Save(b) error = %v", err)
	}
}

func TestSessionRepository_GetPeriodStats_DistractionCount(t *testing.T) {
	store, _ := NewMemory()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	repo := store.Sessions()
	db := store.(*sqliteStorage).db

	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 0, 1)

	seed := func(id string, distractions []domain.Distraction) {
		t.Helper()
		s := &domain.PomodoroSession{
			ID:           id,
			Type:         domain.SessionTypeWork,
			Status:       domain.SessionStatusCompleted,
			Duration:     25 * time.Minute,
			StartedAt:    start.Add(10 * time.Hour),
			Distractions: distractions,
		}
		if err := repo.Save(ctx, s); err != nil {
			t.Fatalf("Save(%s) error = %v", id, err)
		}
	}

	// Sessions without distractions are stored as JSON 'null' and count 0.
	seed("n1", nil)
	seed("n2", nil)
	seed("n3", nil)

	stats, err := repo.GetPeriodStats(ctx, start, end)
	if err != nil {
		t.Fatalf("GetPeriodStats() error = %v", err)
	}
	if stats.DistractionCount != 0 {
		t.Errorf("DistractionCount with no distractions = %d, want 0", stats.DistractionCount)
	}

	seed("d1", []domain.Distraction{{Text: "slack"}, {Text: "phone"}})
	// Legacy newline text that starts with '[' is not JSON.
	seed("l1", nil)
	if _, err := db.Exec("UPDATE sessions SET distractions = ? WHERE id = 'l1'", "[slack] ping\nphone"); err != nil {
		t.Fatalf("seed legacy distractions error = %v", err)
	}

	stats, err = repo.GetPeriodStats(ctx, start, end)
	if err != nil {
		t.Fatalf("GetPeriodStats() error = %v", err)
	}
	if stats.DistractionCount != 4 {
		t.Errorf("DistractionCount = %d, want 4 (2 JSON + 2 legacy)", stats.DistractionCount)
	}
}

func TestSessionRepository_OneActiveSession_RunningPlusPaused(t *testing.T) {
	store, _ := NewMemory()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	repo := store.Sessions()
	config := domain.DefaultPomodoroConfig()

	paused := domain.NewPomodoroSession(config, nil)
	paused.Pause()
	if err := repo.Save(ctx, paused); err != nil {
		t.Fatalf("Save(paused) error = %v", err)
	}

	running := domain.NewPomodoroSession(config, nil)
	err := repo.Save(ctx, running)
	if err == nil {
		t.Fatal("Save() of a running session next to a paused one should violate idx_sessions_one_active")
	}
	if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
		t.Errorf("Save() error = %v, want UNIQUE constraint violation", err)
	}

	// Several finished sessions are still fine.
	for i := 0; i < 2; i++ {
		s := domain.NewPomodoroSession(config, nil)
		s.Complete()
		if err := repo.Save(ctx, s); err != nil {
			t.Fatalf("Save(completed %d) error = %v", i, err)
		}
	}
}

// activeIDs returns the ids of the active sessions.
func activeIDs(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query("SELECT id FROM sessions WHERE status IN ('running','paused')")
	if err != nil {
		t.Fatalf("query active error = %v", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan error = %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestMigrate_PreIndexDBWithTwoActiveSessions(t *testing.T) {
	// A version-0 database from before the one-active index, holding two
	// running sessions, must still migrate; the newest stays active.
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if _, err := raw.Exec(baseSchema); err != nil {
		t.Fatalf("base schema error = %v", err)
	}
	if _, err := raw.Exec("DROP INDEX IF EXISTS idx_sessions_one_active"); err != nil {
		t.Fatalf("drop index error = %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO sessions (id, type, status, duration_ms, started_at) VALUES
		('old', 'work', 'running', 1000, '2026-01-01 09:00:00 +0000 UTC'),
		('new', 'work', 'running', 1000, '2026-01-01 10:00:00 +0000 UTC')`); err != nil {
		t.Fatalf("seed sessions error = %v", err)
	}
	_ = raw.Close()

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() on legacy DB with two active sessions error = %v", err)
	}
	defer func() { _ = store.Close() }()

	s := store.(*sqliteStorage)
	if got := activeIDs(t, s.db); !equalStrings(got, []string{"new"}) {
		t.Errorf("active sessions after migration = %v, want [new]", got)
	}
	var oldStatus string
	if err := s.db.QueryRow("SELECT status FROM sessions WHERE id = 'old'").Scan(&oldStatus); err != nil {
		t.Fatalf("read old status error = %v", err)
	}
	if oldStatus != string(domain.SessionStatusCancelled) {
		t.Errorf("old session status = %s, want cancelled", oldStatus)
	}
	if v, _ := s.userVersion(); v != schemaVersionLatest {
		t.Errorf("user_version = %d, want %d", v, schemaVersionLatest)
	}
}

func TestMigrate_V2RunningPlusPausedUpgrade(t *testing.T) {
	// A version-2 database with the old per-status index can hold one
	// running and one paused session; v3 demotes the older one and installs
	// the corrected index.
	db := newPreV2DB(t)
	if _, err := db.Exec(tagsSchema); err != nil {
		t.Fatalf("tags schema error = %v", err)
	}
	if _, err := db.Exec("DROP INDEX IF EXISTS idx_sessions_one_active"); err != nil {
		t.Fatalf("drop index error = %v", err)
	}
	if _, err := db.Exec("CREATE UNIQUE INDEX idx_sessions_one_active ON sessions(status) WHERE status IN ('running','paused')"); err != nil {
		t.Fatalf("old index error = %v", err)
	}
	if _, err := db.Exec("PRAGMA user_version = 2"); err != nil {
		t.Fatalf("set user_version error = %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sessions (id, type, status, duration_ms, started_at) VALUES
		('p', 'work', 'paused', 1000, '2026-01-01 09:00:00 +0000 UTC'),
		('r', 'work', 'running', 1000, '2026-01-01 10:00:00 +0000 UTC')`); err != nil {
		t.Fatalf("seed sessions error = %v", err)
	}

	s := &sqliteStorage{db: db}
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if got := activeIDs(t, db); !equalStrings(got, []string{"r"}) {
		t.Errorf("active sessions after migration = %v, want [r]", got)
	}

	// The new index rejects running + paused.
	_, err := db.Exec("UPDATE sessions SET status = 'paused' WHERE id = 'p'")
	if err == nil || !strings.Contains(err.Error(), "UNIQUE constraint failed") {
		t.Errorf("re-activating p next to r error = %v, want UNIQUE constraint violation", err)
	}

	// Re-running is a no-op.
	if err := s.Migrate(); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}
}

func TestMigrate_RejectsNewerSchemaVersion(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "future.db")
	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := store.(*sqliteStorage).db.Exec("PRAGMA user_version = 99"); err != nil {
		t.Fatalf("set user_version error = %v", err)
	}
	_ = store.Close()

	_, err = New(dbPath)
	if err == nil {
		t.Fatal("New() on a database with a newer schema version should fail")
	}
	if !errors.Is(err, errSchemaTooNew) {
		t.Errorf("New() error = %v, want errSchemaTooNew", err)
	}
}

// failTagInserts makes any insert of the tag "boom" into the given tag table
// fail, simulating a transient error halfway through a tag rewrite.
func failTagInserts(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	//nolint:gosec // test helper: table is a fixed name chosen by the test
	q := "CREATE TRIGGER fail_" + table + " BEFORE INSERT ON " + table +
		" WHEN NEW.tag = 'boom' BEGIN SELECT RAISE(ABORT, 'injected failure'); END"
	if _, err := db.Exec(q); err != nil {
		t.Fatalf("create trigger error = %v", err)
	}
}

func TestTaskRepository_SaveUpdateAtomicWithTags(t *testing.T) {
	store, _ := NewMemory()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	repo := store.Tasks()
	db := store.(*sqliteStorage).db
	failTagInserts(t, db, "task_tags")

	// Save: a failing tag insert leaves no task row behind.
	bad, _ := domain.NewTask("bad")
	bad.Tags = []string{"a", "boom"}
	if err := repo.Save(ctx, bad); err == nil {
		t.Fatal("Save() with failing tag insert should error")
	}
	if _, err := repo.FindByID(ctx, bad.ID); !errors.Is(err, domain.ErrTaskNotFound) {
		t.Errorf("FindByID(bad) error = %v, want ErrTaskNotFound (row rolled back)", err)
	}

	// Update: the row and its tags stay as they were.
	task, _ := domain.NewTask("original")
	task.Tags = []string{"a", "b"}
	if err := repo.Save(ctx, task); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	task.Title = "renamed"
	task.Tags = []string{"c", "boom"}
	if err := repo.Update(ctx, task); err == nil {
		t.Fatal("Update() with failing tag insert should error")
	}
	found, err := repo.FindByID(ctx, task.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if found.Title != "original" || !equalStrings(found.Tags, []string{"a", "b"}) {
		t.Errorf("after failed Update: title=%q tags=%v, want original [a b]", found.Title, found.Tags)
	}

	// Inside WithTx the repository reuses the caller's transaction.
	err = store.WithTx(ctx, func(tx ports.Storage) error {
		task.Title = "in tx"
		task.Tags = []string{"x"}
		return tx.Tasks().Update(ctx, task)
	})
	if err != nil {
		t.Fatalf("WithTx Update() error = %v", err)
	}
}

func TestSessionRepository_SaveUpdateAtomicWithTags(t *testing.T) {
	store, _ := NewMemory()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	repo := store.Sessions()
	db := store.(*sqliteStorage).db
	failTagInserts(t, db, "session_tags")
	config := domain.DefaultPomodoroConfig()

	bad := domain.NewPomodoroSession(config, nil)
	bad.Tags = []string{"a", "boom"}
	if err := repo.Save(ctx, bad); err == nil {
		t.Fatal("Save() with failing tag insert should error")
	}
	if got, err := repo.FindByID(ctx, bad.ID); err != nil || got != nil {
		t.Errorf("FindByID(bad) = %v, %v; want nil (row rolled back)", got, err)
	}

	session := domain.NewPomodoroSession(config, nil)
	session.Tags = []string{"a", "b"}
	if err := repo.Save(ctx, session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	session.Notes = "changed"
	session.Tags = []string{"boom"}
	if err := repo.Update(ctx, session); err == nil {
		t.Fatal("Update() with failing tag insert should error")
	}
	found, err := repo.FindByID(ctx, session.ID)
	if err != nil || found == nil {
		t.Fatalf("FindByID() = %v, %v", found, err)
	}
	if found.Notes != "" || !equalStrings(found.Tags, []string{"a", "b"}) {
		t.Errorf("after failed Update: notes=%q tags=%v, want \"\" [a b]", found.Notes, found.Tags)
	}
}

func TestDayWindows_UseCalendarDays(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	ctx := context.Background()

	// 2026-10-25 is a 25-hour day in Madrid; 2026-03-29 a 23-hour day.
	fallBack := time.Date(2026, 10, 25, 12, 0, 0, 0, loc)
	springForward := time.Date(2026, 3, 29, 12, 0, 0, 0, loc)

	t.Run("GetDailyStats", func(t *testing.T) {
		store, _ := NewMemory()
		defer func() { _ = store.Close() }()
		repo := store.Sessions()

		save := func(id string, at time.Time) {
			c := at.Add(25 * time.Minute)
			s := &domain.PomodoroSession{ID: id, Type: domain.SessionTypeWork, Status: domain.SessionStatusCompleted,
				Duration: 25 * time.Minute, StartedAt: at, CompletedAt: &c}
			if err := repo.Save(ctx, s); err != nil {
				t.Fatalf("Save(%s) error = %v", id, err)
			}
		}
		save("late", time.Date(2026, 10, 25, 23, 30, 0, 0, loc))
		save("early", time.Date(2026, 3, 30, 0, 30, 0, 0, loc))

		stats, err := repo.GetDailyStats(ctx, fallBack)
		if err != nil {
			t.Fatalf("GetDailyStats() error = %v", err)
		}
		if stats.WorkSessions != 1 {
			t.Errorf("fall-back day WorkSessions = %d, want 1", stats.WorkSessions)
		}
		stats, err = repo.GetDailyStats(ctx, springForward)
		if err != nil {
			t.Fatalf("GetDailyStats() error = %v", err)
		}
		if stats.WorkSessions != 0 {
			t.Errorf("spring-forward day WorkSessions = %d, want 0 (session belongs to next day)", stats.WorkSessions)
		}
	})

	t.Run("CountCompleted", func(t *testing.T) {
		store, _ := NewMemory()
		defer func() { _ = store.Close() }()
		repo := store.Tasks()

		task, _ := domain.NewTask("late")
		done := time.Date(2026, 10, 25, 23, 30, 0, 0, loc)
		task.Status = domain.StatusCompleted
		task.CompletedAt = &done
		if err := repo.Save(ctx, task); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
		n, err := repo.CountCompleted(ctx, fallBack)
		if err != nil {
			t.Fatalf("CountCompleted() error = %v", err)
		}
		if n != 1 {
			t.Errorf("CountCompleted(fall-back day) = %d, want 1", n)
		}
	})

	t.Run("FindTodayHighlight", func(t *testing.T) {
		store, _ := NewMemory()
		defer func() { _ = store.Close() }()
		repo := store.Tasks()

		task, _ := domain.NewTask("late highlight")
		h := time.Date(2026, 10, 25, 23, 30, 0, 0, loc)
		task.HighlightDate = &h
		if err := repo.Save(ctx, task); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
		found, err := repo.FindTodayHighlight(ctx, fallBack)
		if err != nil {
			t.Fatalf("FindTodayHighlight() error = %v", err)
		}
		if found == nil || found.ID != task.ID {
			t.Errorf("FindTodayHighlight(fall-back day) = %v, want the task", found)
		}
	})

	t.Run("FindYesterdayHighlight", func(t *testing.T) {
		store, _ := NewMemory()
		defer func() { _ = store.Close() }()
		repo := store.Tasks()

		task, _ := domain.NewTask("yesterday highlight")
		h := time.Date(2026, 10, 25, 0, 30, 0, 0, loc)
		task.HighlightDate = &h
		if err := repo.Save(ctx, task); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
		found, err := repo.FindYesterdayHighlight(ctx, time.Date(2026, 10, 26, 10, 0, 0, 0, loc))
		if err != nil {
			t.Fatalf("FindYesterdayHighlight() error = %v", err)
		}
		if found == nil || found.ID != task.ID {
			t.Errorf("FindYesterdayHighlight(day after fall-back) = %v, want the task", found)
		}
	})
}

func TestTags_CommaIsASeparator(t *testing.T) {
	if got := normalizeTags([]string{"Backend,Urgent", " a , ,b", "urgent"}); !equalStrings(got, []string{"backend", "urgent", "a", "b"}) {
		t.Errorf("normalizeTags() = %v, want [backend urgent a b]", got)
	}

	store, _ := NewMemory()
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	task, _ := domain.NewTask("comma task")
	task.Tags = []string{"backend,urgent"}
	if err := store.Tasks().Save(ctx, task); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	db := store.(*sqliteStorage).db
	if got := tagsFor(t, db, "SELECT tag FROM task_tags WHERE task_id = ?", task.ID); !equalStrings(got, []string{"backend", "urgent"}) {
		t.Errorf("task_tags rows = %v, want [backend urgent]", got)
	}

	session := domain.NewPomodoroSession(domain.DefaultPomodoroConfig(), nil)
	session.Tags = []string{"backend,urgent"}
	if err := store.Sessions().Save(ctx, session); err != nil {
		t.Fatalf("Save() session error = %v", err)
	}
	if got := tagsFor(t, db, "SELECT tag FROM session_tags WHERE session_id = ?", session.ID); !equalStrings(got, []string{"backend", "urgent"}) {
		t.Errorf("session_tags rows = %v, want [backend urgent]", got)
	}
}

func TestMigrate_V3ResplitsCommaTags(t *testing.T) {
	db := newPreV2DB(t)
	s := &sqliteStorage{db: db}
	// Bring the fixture to v2, then plant tag rows written by the old code.
	if err := s.migrateTags(); err != nil {
		t.Fatalf("migrateTags() error = %v", err)
	}
	if _, err := db.Exec("INSERT INTO tasks (id, title, status, created_at, updated_at) VALUES ('t1', 'a', 'pending', 0, 0)"); err != nil {
		t.Fatalf("insert task error = %v", err)
	}
	if _, err := db.Exec("INSERT INTO task_tags (task_id, tag) VALUES ('t1', 'backend,urgent'), ('t1', 'backend'), ('t1', 'solo')"); err != nil {
		t.Fatalf("insert task_tags error = %v", err)
	}
	if _, err := db.Exec("INSERT INTO sessions (id, type, status, duration_ms, started_at) VALUES ('s1', 'work', 'completed', 1000, 0)"); err != nil {
		t.Fatalf("insert session error = %v", err)
	}
	if _, err := db.Exec("INSERT INTO session_tags (session_id, tag) VALUES ('s1', 'x, y,,')"); err != nil {
		t.Fatalf("insert session_tags error = %v", err)
	}

	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if got := tagsFor(t, db, "SELECT tag FROM task_tags WHERE task_id = ?", "t1"); !equalStrings(got, []string{"backend", "solo", "urgent"}) {
		t.Errorf("task_tags after v3 = %v, want [backend solo urgent]", got)
	}
	if got := tagsFor(t, db, "SELECT tag FROM session_tags WHERE session_id = ?", "s1"); !equalStrings(got, []string{"x", "y"}) {
		t.Errorf("session_tags after v3 = %v, want [x y]", got)
	}
}

// TestSessionRepository_ScanNullGitColumns guards that rows written by very
// old versions, whose git_branch, git_commit and git_modified are NULL, are
// read back by both the single-row and the multi-row scan.
func TestSessionRepository_ScanNullGitColumns(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "flow.db"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	started := time.Now().Add(-time.Hour)
	if _, err := store.(*sqliteStorage).db.ExecContext(ctx, `
		INSERT INTO sessions (id, task_id, type, status, duration_ms, started_at,
			completed_at, git_branch, git_commit, git_modified, notes)
		VALUES ('old', NULL, 'work', 'completed', 1500000, ?, ?, NULL, NULL, NULL, NULL)`,
		started, started.Add(25*time.Minute)); err != nil {
		t.Fatalf("insert legacy row error = %v", err)
	}

	got, err := store.Sessions().FindByID(ctx, "old")
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if got == nil || got.GitBranch != "" || got.GitCommit != "" || got.GitModified != nil {
		t.Fatalf("FindByID() = %+v, want the row with empty git context", got)
	}

	recent, err := store.Sessions().FindRecent(ctx, started.Add(-time.Minute))
	if err != nil {
		t.Fatalf("FindRecent() error = %v", err)
	}
	if len(recent) != 1 || recent[0].ID != "old" || recent[0].GitBranch != "" {
		t.Fatalf("FindRecent() = %+v, want the legacy row", recent)
	}
}

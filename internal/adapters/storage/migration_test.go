package storage

import (
	"database/sql"
	"sort"
	"testing"
)

// newPreV2DB builds a database in the pre-v2 state: base schema, legacy CSV
// tag columns, and user_version already at 1.
func newPreV2DB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys error = %v", err)
	}
	if _, err := db.Exec(baseSchema); err != nil {
		t.Fatalf("base schema error = %v", err)
	}
	// The sessions tag column arrived via a legacy ALTER.
	if _, err := db.Exec("ALTER TABLE sessions ADD COLUMN tags TEXT"); err != nil {
		t.Fatalf("add sessions.tags error = %v", err)
	}
	if _, err := db.Exec("PRAGMA user_version = 1"); err != nil {
		t.Fatalf("set user_version error = %v", err)
	}
	return db
}

// tagsFor reads the normalized tag set for one entity from a tag table.
func tagsFor(t *testing.T, db *sql.DB, query, id string) []string {
	t.Helper()

	rows, err := db.Query(query, id)
	if err != nil {
		t.Fatalf("query tags error = %v", err)
	}
	defer func() { _ = rows.Close() }()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			t.Fatalf("scan tag error = %v", err)
		}
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	as := append([]string(nil), a...)
	bs := append([]string(nil), b...)
	sort.Strings(as)
	sort.Strings(bs)
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}

func TestMigrate_FreshDBGetsVersion2(t *testing.T) {
	storage, err := NewMemory()
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	s := storage.(*sqliteStorage)
	version, err := s.userVersion()
	if err != nil {
		t.Fatalf("userVersion() error = %v", err)
	}
	if version != schemaVersionTags {
		t.Errorf("user_version = %d, want %d", version, schemaVersionTags)
	}

	// Tag tables exist and are writable.
	if _, err := s.db.Exec("INSERT INTO tasks (id, title, status, created_at, updated_at) VALUES ('t1', 'x', 'pending', 0, 0)"); err != nil {
		t.Fatalf("insert task error = %v", err)
	}
	if _, err := s.db.Exec("INSERT INTO task_tags (task_id, tag) VALUES ('t1', 'deep')"); err != nil {
		t.Errorf("insert into task_tags error = %v", err)
	}
}

func TestMigrate_PreV2Backfill(t *testing.T) {
	db := newPreV2DB(t)

	// Messy legacy CSV: spaces, mixed case, empties, duplicates.
	if _, err := db.Exec("INSERT INTO tasks (id, title, status, tags, created_at, updated_at) VALUES ('t1', 'a', 'pending', ' Deep , focus,DEEP,,  ', 0, 0)"); err != nil {
		t.Fatalf("insert task t1 error = %v", err)
	}
	if _, err := db.Exec("INSERT INTO tasks (id, title, status, tags, created_at, updated_at) VALUES ('t2', 'b', 'pending', '', 0, 0)"); err != nil {
		t.Fatalf("insert task t2 error = %v", err)
	}
	if _, err := db.Exec("INSERT INTO sessions (id, type, status, duration_ms, started_at, tags) VALUES ('s1', 'work', 'completed', 1000, 0, 'Work, urgent ,WORK')"); err != nil {
		t.Fatalf("insert session s1 error = %v", err)
	}

	s := &sqliteStorage{db: db}
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	if got := tagsFor(t, db, "SELECT tag FROM task_tags WHERE task_id = ?", "t1"); !equalStrings(got, []string{"deep", "focus"}) {
		t.Errorf("task_tags for t1 = %v, want [deep focus]", got)
	}
	if got := tagsFor(t, db, "SELECT tag FROM task_tags WHERE task_id = ?", "t2"); len(got) != 0 {
		t.Errorf("task_tags for t2 = %v, want empty", got)
	}
	if got := tagsFor(t, db, "SELECT tag FROM session_tags WHERE session_id = ?", "s1"); !equalStrings(got, []string{"urgent", "work"}) {
		t.Errorf("session_tags for s1 = %v, want [urgent work]", got)
	}

	version, err := s.userVersion()
	if err != nil {
		t.Fatalf("userVersion() error = %v", err)
	}
	if version != schemaVersionTags {
		t.Errorf("user_version = %d, want %d", version, schemaVersionTags)
	}

	// Re-running Migrate is a no-op and does not duplicate rows.
	if err := s.Migrate(); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}
	if got := tagsFor(t, db, "SELECT tag FROM task_tags WHERE task_id = ?", "t1"); !equalStrings(got, []string{"deep", "focus"}) {
		t.Errorf("task_tags for t1 after re-migrate = %v, want [deep focus]", got)
	}
}

func TestMigrate_PreV1Backfill(t *testing.T) {
	// A version-0 database gets the base schema AND the tag migration.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	s := &sqliteStorage{db: db}
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	version, err := s.userVersion()
	if err != nil {
		t.Fatalf("userVersion() error = %v", err)
	}
	if version != schemaVersionTags {
		t.Errorf("user_version = %d, want %d", version, schemaVersionTags)
	}

	var oneActive int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_sessions_one_active'").Scan(&oneActive)
	if err != nil {
		t.Fatalf("check index error = %v", err)
	}
	if oneActive != 1 {
		t.Error("idx_sessions_one_active missing after base migration")
	}
}

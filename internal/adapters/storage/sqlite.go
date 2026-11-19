// Package storage provides SQLite implementations of the storage ports.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/xvierd/flow-cli/internal/ports"
	_ "modernc.org/sqlite"
)

// executor is satisfied by both *sql.DB and *sql.Tx, so repositories can run
// either directly against the connection pool or inside a transaction.
type executor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// errNestedTx is returned by WithTx on a transaction-scoped storage;
// nested transactions are not supported.
var errNestedTx = errors.New("storage: nested transactions are not supported")

// errMigrateInTx is returned by Migrate on a transaction-scoped storage.
var errMigrateInTx = errors.New("storage: cannot run migrations inside a transaction")

// sqliteStorage implements the ports.Storage interface using SQLite.
type sqliteStorage struct {
	db          *sql.DB
	taskRepo    ports.TaskRepository
	sessionRepo ports.SessionRepository
	// inTx marks a transaction-scoped storage created by WithTx.
	inTx bool
}

// Ensure sqliteStorage implements ports.Storage.
var _ ports.Storage = (*sqliteStorage)(nil)

// New creates a new SQLite storage instance.
func New(dbPath string) (ports.Storage, error) {
	db, err := sql.Open("sqlite", dsn(dbPath))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Pin the pool to a single connection: with ":memory:" each pooled
	// connection would get its own private database, and a transaction
	// pinning one connection must see the same data as plain queries.
	db.SetMaxOpenConns(1)

	// Open the first connection now so DSN/pragma errors surface here.
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	storage := &sqliteStorage{
		db:          db,
		taskRepo:    newTaskRepository(db),
		sessionRepo: newSessionRepository(db),
	}

	if err := storage.Migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return storage, nil
}

// dsnParams are applied by the driver to every new connection, so they
// survive database/sql replacing a pooled connection. busy_timeout makes a
// second flow process wait for the write lock instead of failing at once;
// _txlock=immediate takes that lock at BEGIN, so a read-then-write
// transaction cannot fail with SQLITE_BUSY_SNAPSHOT.
const dsnParams = "_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_txlock=immediate"

// dsn builds the driver DSN for dbPath. The driver cuts a plain path at its
// first '?', so a path containing '?' (or already in URI form) is passed as
// a percent-encoded "file:" URI instead.
func dsn(dbPath string) string {
	if strings.HasPrefix(dbPath, "file:") {
		sep := "?"
		if strings.Contains(dbPath, "?") {
			sep = "&"
		}
		return dbPath + sep + dsnParams
	}
	if strings.Contains(dbPath, "?") {
		u := url.URL{Path: filepath.ToSlash(dbPath)}
		return "file:" + u.EscapedPath() + "?" + dsnParams
	}
	return dbPath + "?" + dsnParams
}

// NewMemory creates a new in-memory SQLite storage instance for testing.
func NewMemory() (ports.Storage, error) {
	return New(":memory:")
}

// Tasks returns the task repository.
func (s *sqliteStorage) Tasks() ports.TaskRepository {
	return s.taskRepo
}

// Sessions returns the session repository.
func (s *sqliteStorage) Sessions() ports.SessionRepository {
	return s.sessionRepo
}

// Close closes the database connection. On a transaction-scoped storage it is
// a no-op: the underlying pool is owned by the root storage and stays open.
func (s *sqliteStorage) Close() error {
	if s.inTx {
		return nil
	}
	return s.db.Close()
}

// WithTx runs fn within a single database transaction. The Storage passed to
// fn wraps the same *sql.Tx for both repositories and must not be used after
// fn returns. If fn returns an error or panics, the transaction is rolled
// back (a panic is re-thrown after the rollback).
func (s *sqliteStorage) WithTx(ctx context.Context, fn func(tx ports.Storage) error) error {
	if s.inTx {
		return errNestedTx
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	txStorage := &sqliteStorage{
		db:          s.db,
		taskRepo:    newTaskRepository(tx),
		sessionRepo: newSessionRepository(tx),
		inTx:        true,
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(txStorage); err != nil {
		_ = tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Schema versions tracked via PRAGMA user_version.
const (
	// schemaVersionBase is the original schema plus the legacy ALTERs.
	schemaVersionBase = 1
	// schemaVersionTags adds the task_tags/session_tags tables and backfill.
	schemaVersionTags = 2
	// schemaVersionOneActive replaces the per-status "one active session"
	// index with one on a constant, and re-splits comma-joined tags.
	schemaVersionOneActive = 3
	// schemaVersionLatest is the newest schema this binary understands.
	schemaVersionLatest = schemaVersionOneActive
)

// errSchemaTooNew is returned by Migrate when the database was migrated by a
// newer flow binary.
var errSchemaTooNew = errors.New("database schema is newer than this flow binary; upgrade flow")

// baseSchema is the version-1 schema. The "one active session" index is not
// part of it: version 3 creates it after demoting surplus active sessions,
// so a pre-index database that already holds two of them still migrates.
const baseSchema = `
	CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT,
		status TEXT NOT NULL,
		tags TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		completed_at DATETIME
	);

	CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
	CREATE INDEX IF NOT EXISTS idx_tasks_updated ON tasks(updated_at);

	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		task_id TEXT,
		type TEXT NOT NULL,
		status TEXT NOT NULL,
		duration_ms INTEGER NOT NULL,
		started_at DATETIME NOT NULL,
		paused_at DATETIME,
		completed_at DATETIME,
		git_branch TEXT,
		git_commit TEXT,
		git_modified TEXT,
		notes TEXT,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE SET NULL
	);

	CREATE INDEX IF NOT EXISTS idx_sessions_task ON sessions(task_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_started ON sessions(started_at);
	CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
	`

// baseAlters are the unversioned legacy migrations. They are idempotent by
// luck (duplicate-column errors are ignored); version 1 makes them run once.
var baseAlters = []string{
	"ALTER TABLE sessions ADD COLUMN methodology TEXT DEFAULT 'pomodoro'",
	"ALTER TABLE sessions ADD COLUMN focus_score INTEGER",
	"ALTER TABLE sessions ADD COLUMN distractions TEXT",
	"ALTER TABLE sessions ADD COLUMN accomplishment TEXT",
	"ALTER TABLE sessions ADD COLUMN intended_outcome TEXT",
	"ALTER TABLE tasks ADD COLUMN highlight_date DATETIME",
	"ALTER TABLE sessions ADD COLUMN tags TEXT",
	"ALTER TABLE sessions ADD COLUMN energize_activity TEXT",
	"ALTER TABLE sessions ADD COLUMN shutdown_ritual TEXT",
	"ALTER TABLE sessions ADD COLUMN outcome_achieved TEXT",
}

// tagsSchema is the version-2 schema: normalized tag tables. The legacy CSV
// columns stay in place (unread, unwritten) for downgrade safety.
const tagsSchema = `
	CREATE TABLE IF NOT EXISTS task_tags (
		task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		tag TEXT NOT NULL,
		PRIMARY KEY (task_id, tag)
	);

	CREATE TABLE IF NOT EXISTS session_tags (
		session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
		tag TEXT NOT NULL,
		PRIMARY KEY (session_id, tag)
	);

	CREATE INDEX IF NOT EXISTS idx_session_tags_tag ON session_tags(tag);
	`

// Migrate brings the database to the latest schema version, one versioned
// step at a time, each in its own transaction. It is rejected on a
// transaction-scoped storage: schema changes belong to startup, not to
// business transactions.
func (s *sqliteStorage) Migrate() error {
	if s.inTx {
		return errMigrateInTx
	}

	version, err := s.userVersion()
	if err != nil {
		return err
	}
	if version > schemaVersionLatest {
		return fmt.Errorf("%w (database v%d, supported up to v%d)", errSchemaTooNew, version, schemaVersionLatest)
	}

	if version < schemaVersionBase {
		if err := s.migrateBase(); err != nil {
			return err
		}
	}
	if version < schemaVersionTags {
		if err := s.migrateTags(); err != nil {
			return err
		}
	}
	if version < schemaVersionOneActive {
		if err := s.migrateOneActive(); err != nil {
			return err
		}
	}

	return nil
}

// userVersion reads PRAGMA user_version.
func (s *sqliteStorage) userVersion() (int, error) {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("failed to read schema version: %w", err)
	}
	return version, nil
}

// migrateBase runs the version-1 schema and legacy ALTERs in one transaction.
func (s *sqliteStorage) migrateBase() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin base migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	if _, err := tx.Exec(baseSchema); err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}

	for _, m := range baseAlters {
		// Ignore errors — column may already exist
		_, _ = tx.Exec(m)
	}

	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersionBase)); err != nil {
		return fmt.Errorf("failed to set schema version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit base migration: %w", err)
	}
	return nil
}

// migrateTags creates the tag tables and backfills them from the legacy CSV
// columns, in one transaction. INSERT OR IGNORE keeps it safe to re-run.
func (s *sqliteStorage) migrateTags() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin tag migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	if _, err := tx.Exec(tagsSchema); err != nil {
		return fmt.Errorf("failed to create tag tables: %w", err)
	}

	if err := backfillTags(tx,
		"SELECT id, tags FROM tasks WHERE tags IS NOT NULL AND tags != ''",
		"INSERT OR IGNORE INTO task_tags (task_id, tag) VALUES (?, ?)"); err != nil {
		return err
	}
	if err := backfillTags(tx,
		"SELECT id, tags FROM sessions WHERE tags IS NOT NULL AND tags != ''",
		"INSERT OR IGNORE INTO session_tags (session_id, tag) VALUES (?, ?)"); err != nil {
		return err
	}

	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersionTags)); err != nil {
		return fmt.Errorf("failed to set schema version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit tag migration: %w", err)
	}
	return nil
}

// oneActiveSchema is the version-3 schema step. Surplus active sessions are
// demoted to cancelled first, keeping the newest (what FindActive returns),
// so the index can always be created. The index is on a constant, so all
// active rows share one key: at most one session may be running or paused.
// Breaks share this table, so it also prevents a break from overlapping a
// work session.
const oneActiveSchema = `
	UPDATE sessions SET status = 'cancelled'
	WHERE status IN ('running','paused')
	  AND id != (SELECT id FROM sessions WHERE status IN ('running','paused')
	             ORDER BY started_at DESC, id DESC LIMIT 1);

	DROP INDEX IF EXISTS idx_sessions_one_active;
	CREATE UNIQUE INDEX idx_sessions_one_active ON sessions((1)) WHERE status IN ('running','paused');
	`

// migrateOneActive runs the version-3 schema step and re-splits tags that
// were stored with commas, in one transaction. Both are safe to re-run.
func (s *sqliteStorage) migrateOneActive() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin one-active migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	if _, err := tx.Exec(oneActiveSchema); err != nil {
		return fmt.Errorf("failed to recreate one-active index: %w", err)
	}

	if err := resplitTags(tx,
		"SELECT task_id, tag FROM task_tags WHERE instr(tag, ',') > 0",
		"DELETE FROM task_tags WHERE task_id = ? AND tag = ?",
		"INSERT OR IGNORE INTO task_tags (task_id, tag) VALUES (?, ?)"); err != nil {
		return err
	}
	if err := resplitTags(tx,
		"SELECT session_id, tag FROM session_tags WHERE instr(tag, ',') > 0",
		"DELETE FROM session_tags WHERE session_id = ? AND tag = ?",
		"INSERT OR IGNORE INTO session_tags (session_id, tag) VALUES (?, ?)"); err != nil {
		return err
	}

	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersionOneActive)); err != nil {
		return fmt.Errorf("failed to set schema version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit one-active migration: %w", err)
	}
	return nil
}

// resplitTags replaces each tag row containing a comma with its normalized
// parts, matching how normalizeTags now treats commas.
func resplitTags(tx *sql.Tx, selectQuery, deleteQuery, insertQuery string) error {
	rows, err := tx.Query(selectQuery)
	if err != nil {
		return fmt.Errorf("failed to read comma tags: %w", err)
	}

	// Collect first: the single transaction connection cannot run the
	// writes below while this cursor is still open.
	type row struct {
		id  string
		tag string
	}
	var pending []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.tag); err != nil {
			_ = rows.Close()
			return fmt.Errorf("failed to scan comma tags: %w", err)
		}
		pending = append(pending, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("failed to iterate comma tags: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("failed to close rows for comma tags: %w", err)
	}

	for _, r := range pending {
		if _, err := tx.Exec(deleteQuery, r.id, r.tag); err != nil {
			return fmt.Errorf("failed to remove comma tag: %w", err)
		}
		for _, tag := range normalizeTags([]string{r.tag}) {
			if _, err := tx.Exec(insertQuery, r.id, tag); err != nil {
				return fmt.Errorf("failed to insert split tag: %w", err)
			}
		}
	}
	return nil
}

// backfillTags copies normalized tags from a legacy CSV column into its tag
// table. Normalization lowercases, so tags that differed only by case merge
// into one — accepted and intentional.
func backfillTags(tx *sql.Tx, selectQuery, insertQuery string) error {
	rows, err := tx.Query(selectQuery)
	if err != nil {
		return fmt.Errorf("failed to read legacy tags for backfill: %w", err)
	}

	// Collect first: the single transaction connection cannot run the
	// inserts below while this cursor is still open.
	type row struct {
		id   string
		tags string
	}
	var pending []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.tags); err != nil {
			_ = rows.Close()
			return fmt.Errorf("failed to scan legacy tags for backfill: %w", err)
		}
		pending = append(pending, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("failed to iterate legacy tags for backfill: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("failed to close rows for tag backfill: %w", err)
	}

	for _, r := range pending {
		for _, tag := range normalizeTags(strings.Split(r.tags, ",")) {
			if _, err := tx.Exec(insertQuery, r.id, tag); err != nil {
				return fmt.Errorf("failed to backfill tags: %w", err)
			}
		}
	}
	return nil
}

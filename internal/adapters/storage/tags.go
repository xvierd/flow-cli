package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// normalizeTags splits on commas, trims, lowercases, dedupes (first
// occurrence wins), and drops empty tags. Normalization happens at write time
// so stats never fragment on case or whitespace differences; lowercasing can
// merge tags that previously differed only by case — accepted and
// intentional. Commas separate tags because readers rebuild the tag list
// from GROUP_CONCAT(tag), so a stored tag must never contain one.
func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		for _, tag := range strings.Split(raw, ",") {
			tag = strings.ToLower(strings.TrimSpace(tag))
			if tag == "" {
				continue
			}
			if _, ok := seen[tag]; ok {
				continue
			}
			seen[tag] = struct{}{}
			out = append(out, tag)
		}
	}
	return out
}

// atomically runs fn in a transaction. When ex is the connection pool it
// begins one and commits or rolls back; when ex is already a transaction fn
// joins it, leaving commit/rollback to the owner.
func atomically(ctx context.Context, ex executor, fn func(ex executor) error) error {
	db, ok := ex.(*sql.DB)
	if !ok {
		return fn(ex)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// replaceTaskTags rewrites a task's tag rows through the given executor, so
// it composes into any surrounding transaction.
func replaceTaskTags(ctx context.Context, ex executor, taskID string, tags []string) error {
	return replaceTags(ctx, ex,
		"DELETE FROM task_tags WHERE task_id = ?",
		"INSERT OR IGNORE INTO task_tags (task_id, tag) VALUES (?, ?)",
		taskID, tags)
}

// replaceSessionTags rewrites a session's tag rows through the given
// executor, so it composes into any surrounding transaction.
func replaceSessionTags(ctx context.Context, ex executor, sessionID string, tags []string) error {
	return replaceTags(ctx, ex,
		"DELETE FROM session_tags WHERE session_id = ?",
		"INSERT OR IGNORE INTO session_tags (session_id, tag) VALUES (?, ?)",
		sessionID, tags)
}

// replaceTags deletes all tag rows for an entity and inserts the normalized
// set, removing stale rows left by earlier writes.
func replaceTags(ctx context.Context, ex executor, deleteQuery, insertQuery, id string, tags []string) error {
	if _, err := ex.ExecContext(ctx, deleteQuery, id); err != nil {
		return fmt.Errorf("failed to clear tags: %w", err)
	}
	for _, tag := range normalizeTags(tags) {
		if _, err := ex.ExecContext(ctx, insertQuery, id, tag); err != nil {
			return fmt.Errorf("failed to insert tag: %w", err)
		}
	}
	return nil
}

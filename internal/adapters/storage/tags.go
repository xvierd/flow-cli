package storage

import (
	"context"
	"fmt"
	"strings"
)

// normalizeTags trims, lowercases, dedupes (first occurrence wins), and drops
// empty tags. Normalization happens at write time so stats never fragment on
// case or whitespace differences; lowercasing can merge tags that previously
// differed only by case — accepted and intentional.
func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
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
	return out
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

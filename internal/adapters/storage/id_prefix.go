package storage

import (
	"context"
	"fmt"

	"github.com/xvierd/flow-cli/internal/domain"
)

// minIDPrefix is the shortest ID prefix accepted in place of a full ID.
// `flow list` shows 8 characters; shorter prefixes are allowed but not
// below this, so a stray character never matches an arbitrary record.
const minIDPrefix = 4

// fullIDLength is the length of a canonical UUID string.
const fullIDLength = 36

// isIDPrefix reports whether s can be a prefix of a canonical UUID: only
// lowercase hex digits and dashes, which also keeps LIKE wildcards out.
func isIDPrefix(s string) bool {
	if len(s) < minIDPrefix || len(s) >= fullIDLength {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && c != '-' {
			return false
		}
	}
	return true
}

// resolveID maps id to the full ID of a row in table. An exact match wins;
// otherwise a unique prefix of at least minIDPrefix characters is accepted.
// It returns notFound when nothing matches and domain.ErrAmbiguousID when
// the prefix matches more than one row.
func resolveID(ctx context.Context, ex executor, table, id string, notFound error) (string, error) {
	if !isIDPrefix(id) {
		return id, nil
	}

	// table is one of a fixed set of internal names, never user input.
	rows, err := ex.QueryContext(ctx, fmt.Sprintf(`SELECT id FROM %s WHERE id = ? OR id LIKE ? || '%%' ORDER BY id = ? DESC LIMIT 3`, table), id, id, id) // #nosec G201
	if err != nil {
		return "", fmt.Errorf("failed to resolve ID: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var matches []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return "", fmt.Errorf("failed to resolve ID: %w", err)
		}
		if m == id {
			return m, nil
		}
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("failed to resolve ID: %w", err)
	}

	switch len(matches) {
	case 0:
		return "", notFound
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("%w: %s", domain.ErrAmbiguousID, id)
	}
}

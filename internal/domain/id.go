package domain

import "github.com/google/uuid"

// generateID creates a new unique identifier.
func generateID() string {
	return uuid.New().String()
}

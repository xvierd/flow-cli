package cmd

import (
	"context"
	"strings"

	"github.com/xvierd/flow-cli/internal/domain"
)

// branchTaskTitle returns the task title suggested by the git branch checked
// out at workingDir ("feat/auth-refresh" → "auth refresh"), or "" when there
// is none (no repository, main/master, detached HEAD).
func branchTaskTitle(workingDir string) string {
	if app.pomodoro == nil {
		return ""
	}
	return app.pomodoro.SuggestedTaskTitle(context.Background(), workingDir)
}

// suggestionUnlessRecent returns suggestion unless one of the recent tasks
// already has that title, in which case picking it avoids a duplicate.
func suggestionUnlessRecent(suggestion string, recent []*domain.Task) string {
	for _, t := range recent {
		if strings.EqualFold(t.Title, suggestion) {
			return ""
		}
	}
	return suggestion
}

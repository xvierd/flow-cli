package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// gitSummaryMsg carries the git summary line of an ended session, fetched off
// the update loop because computing it runs git.
type gitSummaryMsg struct {
	sessionID string
	line      string
}

// gitSummaryCmd fetches the git summary of the session that just completed.
// It returns nil when no fetcher is configured or nothing is pending, and
// clears the pending flag.
func (c *completionState) gitSummaryCmd() tea.Cmd {
	if !c.gitSummaryPending {
		return nil
	}
	c.gitSummaryPending = false
	fetch, id := c.fetchGitSummary, c.completedSessionID
	if fetch == nil || id == "" {
		return nil
	}
	return func() tea.Msg {
		return gitSummaryMsg{sessionID: id, line: fetch(id)}
	}
}

// applyGitSummary shows a fetched summary if it still belongs to the session
// on the completion screen (a new session may have started meanwhile).
func (c *completionState) applyGitSummary(msg gitSummaryMsg) {
	if c.completed && msg.sessionID == c.completedSessionID {
		c.gitSummary = msg.line
	}
}

package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
)

// TestCompletion_GitSummaryFetchedForWorkSessions checks that completing a
// work session queues one git summary fetch for that session, that a late
// result for another session is ignored, and that reset clears the line.
func TestCompletion_GitSummaryFetchedForWorkSessions(t *testing.T) {
	var fetched []string
	c := &completionState{fetchGitSummary: func(id string) string {
		fetched = append(fetched, id)
		return "3 commits · internal/auth/ (+120 −40)"
	}}

	work := &domain.PomodoroSession{ID: "s1", Type: domain.SessionTypeWork, Duration: time.Minute, StartedAt: time.Now()}
	c.captureCompletion(work)
	cmd := c.gitSummaryCmd()
	if cmd == nil {
		t.Fatal("gitSummaryCmd() = nil after a work session completed")
	}
	if c.gitSummaryCmd() != nil {
		t.Error("gitSummaryCmd() queued twice for one completion")
	}
	msg, ok := cmd().(gitSummaryMsg)
	if !ok || msg.sessionID != "s1" || len(fetched) != 1 {
		t.Fatalf("fetch = %+v, %v (fetched %v)", msg, ok, fetched)
	}

	c.applyGitSummary(gitSummaryMsg{sessionID: "other", line: "stale"})
	if c.gitSummary != "" {
		t.Errorf("stale summary applied: %q", c.gitSummary)
	}
	c.applyGitSummary(msg)
	if c.gitSummary != "3 commits · internal/auth/ (+120 −40)" {
		t.Errorf("gitSummary = %q", c.gitSummary)
	}

	c.reset()
	if c.gitSummary != "" {
		t.Errorf("gitSummary after reset = %q", c.gitSummary)
	}

	brk := &domain.PomodoroSession{ID: "b1", Type: domain.SessionTypeShortBreak, Duration: time.Minute, StartedAt: time.Now()}
	c.captureCompletion(brk)
	if c.gitSummaryCmd() != nil {
		t.Error("a break should not fetch a git summary")
	}
}

// TestInline_DefaultTaskNameFromBranch checks the branch suggestion pre-fills
// a new task's name unless a chained task or a same-named recent task wins.
func TestInline_DefaultTaskNameFromBranch(t *testing.T) {
	m := InlineModel{suggestedTaskTitle: "auth refresh"}
	if got := m.defaultTaskName(); got != "auth refresh" {
		t.Errorf("defaultTaskName() = %q, want auth refresh", got)
	}
	m.recentTasks = []*domain.Task{{Title: "Auth Refresh"}}
	if got := m.defaultTaskName(); got != "" {
		t.Errorf("with a same-named recent task = %q, want \"\"", got)
	}
	m.lastTaskTitle = "previous"
	if got := m.defaultTaskName(); got != "previous" {
		t.Errorf("with a chained task = %q, want previous", got)
	}
}

// TestCompletionScreens_ShowGitSummary drives a completion through Update in
// both TUIs and checks the fetched summary line is rendered.
func TestCompletionScreens_ShowGitSummary(t *testing.T) {
	const line = "3 commits · internal/auth/ (+120 −40)"
	fetch := func(string) string { return line }

	running := stateWithSession()
	id := running.ActiveSession.ID

	m := sizedModel(running)
	m.fetchGitSummary = fetch
	r, cmd := send(t, m, stateMsg{state: stateNoSession()})
	if cmd == nil || r.(Model).gitSummaryPending {
		t.Fatal("completion should queue the git summary fetch")
	}
	r, _ = send(t, r, gitSummaryMsg{sessionID: id, line: line})
	if view := r.View(); !strings.Contains(view, line) {
		t.Errorf("fullscreen completion view lacks the git summary:\n%s", view)
	}

	im := NewInlineModel(running, nil, nil)
	im.fetchGitSummary = fetch
	ir, _ := send(t, im, stateMsg{state: stateNoSession()})
	ir, _ = send(t, ir, gitSummaryMsg{sessionID: id, line: line})
	if view := ir.View(); !strings.Contains(view, line) {
		t.Errorf("inline completion view lacks the git summary:\n%s", view)
	}
}

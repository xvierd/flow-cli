package services

import (
	"context"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

// freshSummaryWindow is how recently a session must have ended for its
// summary to include uncommitted changes: right after the session they are
// the session's work; later they may be anyone's.
const freshSummaryWindow = 5 * time.Minute

// summarizable reports whether s is an ended work session whose git summary
// can be computed: it recorded a start commit and was completed or voided.
func summarizable(s *domain.PomodoroSession) bool {
	if s == nil || !s.IsWorkSession() || s.GitCommit == "" {
		return false
	}
	return s.Status == domain.SessionStatusCompleted || s.Status == domain.SessionStatusInterrupted
}

// computeGitSummary runs git for an ended session, bounded by the time it
// ended. It returns nil when git, the repository or the start commit is not
// available here. It runs git: never call it inside a transaction.
func (s *PomodoroService) computeGitSummary(ctx context.Context, session *domain.PomodoroSession) *domain.GitSummary {
	if s.gitSummarizer == nil || !summarizable(session) {
		return nil
	}
	until := time.Now()
	if session.CompletedAt != nil {
		until = *session.CompletedAt
	}
	fresh := time.Since(until) < freshSummaryWindow
	summary, err := s.gitSummarizer.Summarize(ctx, "", session.GitCommit, until, fresh, session.GitModified)
	if err != nil {
		return nil
	}
	return summary
}

// recordGitSummary computes and stores the git summary of a session that has
// just ended, setting session.GitSummary. It is best-effort: a repository
// without git or a failed write leave the session without a summary. Call it
// after the transaction that ended the session has committed.
func (s *PomodoroService) recordGitSummary(ctx context.Context, session *domain.PomodoroSession) {
	if session == nil || session.GitSummary != nil {
		return
	}
	summary := s.computeGitSummary(ctx, session)
	if summary == nil {
		return
	}
	if err := s.storage.Sessions().UpdateGitSummary(ctx, session.ID, summary); err != nil {
		return
	}
	session.GitSummary = summary
}

// EnsureGitSummaries computes and stores the git summary of every ended work
// session in sessions that lacks one (for example a session that expired
// while nothing was watching, or one recorded before summaries existed).
// Sessions from another repository, or without git, are left without one.
// It runs git: never call it inside a transaction.
func (s *PomodoroService) EnsureGitSummaries(ctx context.Context, sessions []*domain.PomodoroSession) {
	for _, session := range sessions {
		if session.GitSummary == nil && summarizable(session) {
			s.recordGitSummary(ctx, session)
		}
	}
}

// GitSummaryFor returns the git summary of the given session, computing and
// storing it first when the session has ended without one. It returns nil
// when there is none.
func (s *PomodoroService) GitSummaryFor(ctx context.Context, sessionID string) *domain.GitSummary {
	session, err := s.storage.Sessions().FindByID(ctx, sessionID)
	if err != nil || session == nil {
		return nil
	}
	s.recordGitSummary(ctx, session)
	return session.GitSummary
}

// LiveGitSummary returns what has changed in git since the active session
// started, uncommitted changes included, or nil without git. It is not
// stored.
func (s *PomodoroService) LiveGitSummary(ctx context.Context, session *domain.PomodoroSession) *domain.GitSummary {
	if s.gitSummarizer == nil || session == nil || !session.IsWorkSession() || session.GitCommit == "" {
		return nil
	}
	summary, err := s.gitSummarizer.Summarize(ctx, "", session.GitCommit, time.Time{}, true, session.GitModified)
	if err != nil {
		return nil
	}
	return summary
}

// LastEndedWorkSession returns the most recent completed or voided work
// session of the last week, with its git summary ensured, or nil.
func (s *PomodoroService) LastEndedWorkSession(ctx context.Context) *domain.PomodoroSession {
	sessions, err := s.storage.Sessions().FindRecent(ctx, time.Now().AddDate(0, 0, -7))
	if err != nil {
		return nil
	}
	for _, session := range sessions {
		if session.IsWorkSession() && (session.Status == domain.SessionStatusCompleted || session.Status == domain.SessionStatusInterrupted) {
			s.recordGitSummary(ctx, session)
			return session
		}
	}
	return nil
}

// SuggestedTaskTitle returns a task title derived from the branch checked out
// at workingDir ("feat/auth-refresh" → "auth refresh"), or "" outside a
// repository, on a default branch (main, master, …) or a detached HEAD.
func (s *PomodoroService) SuggestedTaskTitle(ctx context.Context, workingDir string) string {
	if s.gitDetector == nil {
		return ""
	}
	if bd, ok := s.gitDetector.(ports.BranchDetector); ok {
		branch, err := bd.CurrentBranch(ctx, workingDir)
		if err != nil {
			return ""
		}
		return domain.TaskTitleFromBranch(branch)
	}
	if info := s.DetectGit(ctx, workingDir); info != nil {
		return domain.TaskTitleFromBranch(info.Branch)
	}
	return ""
}

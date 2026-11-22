package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/i18n"
)

func TestTaskTitleFromBranch(t *testing.T) {
	tests := []struct{ branch, want string }{
		{"feat/auth-refresh", "auth refresh"},
		{"fix/ABC-123-login-bug", "login bug"},
		{"feature/PROJ-42_rate_limiter", "rate limiter"},
		{"chore/123-bump-deps", "bump deps"},
		{"xavier/feat/search-index", "search index"},
		{"refactor/storage.layer", "storage layer"},
		{"feat-auth-refresh", "auth refresh"},
		{"auth-refresh", "auth refresh"},
		{"ABC-123", ""},
		{"auth-2fa", "auth 2fa"},
		{"v2-api", "v2 api"},
		{"main", ""},
		{"master", ""},
		{"HEAD", ""},
		{"HEAD detached", ""},
		{"", ""},
		{"feat/", ""},
		{"feat", ""},
	}
	for _, tt := range tests {
		if got := TaskTitleFromBranch(tt.branch); got != tt.want {
			t.Errorf("TaskTitleFromBranch(%q) = %q, want %q", tt.branch, got, tt.want)
		}
	}
}

func TestGitSummaryString(t *testing.T) {
	tests := []struct {
		name string
		g    *GitSummary
		want string
	}{
		{"nil", nil, ""},
		{"empty", &GitSummary{}, ""},
		{"one dir", &GitSummary{Commits: 3, FilesChanged: 4, Insertions: 120, Deletions: 40, DirsChanged: 1,
			TopDirs: []DirChange{{Path: "internal/auth/"}}}, "3 commits · internal/auth/ (+120 −40)"},
		{"several dirs", &GitSummary{Commits: 1, FilesChanged: 5, Insertions: 10, Deletions: 2, DirsChanged: 4,
			TopDirs: []DirChange{{Path: "cmd/"}, {Path: "internal/"}}}, "1 commit · cmd/ +3 more (+10 −2)"},
		{"uncommitted only", &GitSummary{FilesChanged: 1, Insertions: 5, DirsChanged: 1, Uncommitted: true,
			TopDirs: []DirChange{{Path: "./"}}}, "no commits · ./ (+5 −0) · uncommitted changes"},
		{"commits without diff", &GitSummary{Commits: 2}, "2 commits"},
	}
	for _, tt := range tests {
		if got := tt.g.String(); got != tt.want {
			t.Errorf("%s: String() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestGitSummaryString_Spanish(t *testing.T) {
	i18n.SetLanguage("es")
	defer i18n.SetLanguage("en")
	g := &GitSummary{FilesChanged: 1, Insertions: 1, DirsChanged: 2, Uncommitted: true, TopDirs: []DirChange{{Path: "a/"}}}
	if got, want := g.String(), "sin commits · a/ +1 más (+1 −0) · cambios sin commitear"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestSessionGitLine(t *testing.T) {
	s := &PomodoroSession{Duration: 38 * time.Minute}
	if got := SessionGitLine(s); got != "" {
		t.Errorf("without summary = %q, want \"\"", got)
	}
	s.GitSummary = &GitSummary{Commits: 3, FilesChanged: 2, Insertions: 120, Deletions: 40, DirsChanged: 1,
		TopDirs: []DirChange{{Path: "internal/auth/"}}}
	if got, want := SessionGitLine(s), "38m · 3 commits · internal/auth/ (+120 −40)"; got != want {
		t.Errorf("SessionGitLine() = %q, want %q", got, want)
	}
	s.Duration = 65 * time.Minute
	if got := SessionGitLine(s); !strings.HasPrefix(got, "1h05m · ") {
		t.Errorf("SessionGitLine() = %q, want 1h05m prefix", got)
	}
}

func TestBranchStats(t *testing.T) {
	work := func(branch string, d time.Duration, status SessionStatus, commits int) *PomodoroSession {
		s := &PomodoroSession{Type: SessionTypeWork, Status: status, Duration: d, GitBranch: branch}
		if commits > 0 {
			s.GitSummary = &GitSummary{Commits: commits, Insertions: 10 * commits}
		}
		return s
	}
	sessions := []*PomodoroSession{
		work("feat/auth-refresh", 50*time.Minute, SessionStatusCompleted, 2),
		work("feat/auth-refresh", 40*time.Minute, SessionStatusCompleted, 0),
		work("feat/auth-refresh", 40*time.Minute, SessionStatusCompleted, 1),
		work("feat/auth-refresh", 40*time.Minute, SessionStatusCompleted, 0),
		work("fix/typo", 25*time.Minute, SessionStatusCompleted, 1),
		work("fix/typo", 25*time.Minute, SessionStatusInterrupted, 1), // voided: not focus
		work("", 25*time.Minute, SessionStatusCompleted, 0),           // no git
		{Type: SessionTypeShortBreak, Status: SessionStatusCompleted, Duration: 5 * time.Minute, GitBranch: "main"},
	}
	got := BranchStats(sessions)
	if len(got) != 2 {
		t.Fatalf("BranchStats() = %+v, want 2 branches", got)
	}
	auth := got[0]
	if auth.Branch != "feat/auth-refresh" || auth.SessionCount != 4 || auth.TotalTime != 170*time.Minute || auth.Commits != 3 || auth.Insertions != 30 {
		t.Errorf("auth branch = %+v", auth)
	}
	if got[1].Branch != "fix/typo" || got[1].SessionCount != 1 {
		t.Errorf("typo branch = %+v", got[1])
	}

	data, err := json.Marshal(auth)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !strings.Contains(string(data), `"total_time":"2h50m0s"`) || !strings.Contains(string(data), `"total_minutes":170`) {
		t.Errorf("JSON = %s", data)
	}
}

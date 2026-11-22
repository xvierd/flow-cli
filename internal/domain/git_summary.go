package domain

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xvierd/flow-cli/internal/i18n"
)

// GitSummary describes what changed in the repository during a work session:
// the commits made between the session's start commit and its end, and the
// lines changed (optionally including changes left uncommitted at the end).
type GitSummary struct {
	// EndCommit is the commit the session ended on (the diff's upper bound).
	EndCommit    string `json:"end_commit,omitempty"`
	Commits      int    `json:"commits"`
	FilesChanged int    `json:"files_changed"`
	Insertions   int    `json:"insertions"`
	Deletions    int    `json:"deletions"`
	DirsChanged  int    `json:"dirs_changed"`
	// TopDirs are the most changed directories, most changed first.
	TopDirs []DirChange `json:"top_dirs,omitempty"`
	// Uncommitted reports that the line counts include working-tree changes
	// that were not committed when the session ended.
	Uncommitted bool `json:"uncommitted,omitempty"`
}

// DirChange is the churn within one directory (repository-relative, with a
// trailing slash; "./" is the repository root).
type DirChange struct {
	Path       string `json:"path"`
	Files      int    `json:"files"`
	Insertions int    `json:"insertions"`
	Deletions  int    `json:"deletions"`
}

// IsEmpty reports whether the summary records no commits and no changes.
func (g *GitSummary) IsEmpty() bool {
	return g == nil || (g.Commits == 0 && g.FilesChanged == 0)
}

// String renders the summary on one line, for example
// "3 commits · internal/auth/ (+120 −40)". The line counts are the session's
// totals; the directory is the most changed one. It is "" for an empty summary.
func (g *GitSummary) String() string {
	if g.IsEmpty() {
		return ""
	}
	var parts []string
	switch {
	case g.Commits == 1:
		parts = append(parts, i18n.T("1 commit"))
	case g.Commits > 1:
		parts = append(parts, i18n.T("%d commits", g.Commits))
	default:
		parts = append(parts, i18n.T("no commits"))
	}
	if g.FilesChanged > 0 {
		where := ""
		if len(g.TopDirs) > 0 {
			where = g.TopDirs[0].Path
			if g.DirsChanged > 1 {
				where += " " + i18n.T("+%d more", g.DirsChanged-1)
			}
			where += " "
		}
		parts = append(parts, where+"(+"+itoa(g.Insertions)+" −"+itoa(g.Deletions)+")")
	}
	if g.Uncommitted {
		parts = append(parts, i18n.T("uncommitted changes"))
	}
	return strings.Join(parts, " · ")
}

// SessionGitLine renders "38m · 3 commits · internal/auth/ (+120 −40)" for an
// ended session: its worked time followed by its git summary. It is "" when
// the session has no git summary or the summary is empty.
func SessionGitLine(s *PomodoroSession) string {
	if s == nil || s.GitSummary.IsEmpty() {
		return ""
	}
	return formatShortDuration(s.Duration) + " · " + s.GitSummary.String()
}

// formatShortDuration renders d as "38m" or "1h05m".
func formatShortDuration(d time.Duration) string {
	m := int(d.Round(time.Minute).Minutes())
	if m < 60 {
		return itoa(m) + "m"
	}
	rest := m % 60
	if rest < 10 {
		return itoa(m/60) + "h0" + itoa(rest) + "m"
	}
	return itoa(m/60) + "h" + itoa(rest) + "m"
}

func itoa(n int) string { return strconv.Itoa(n) }

// defaultBranches never yield a task title: work on them is not a topic.
var defaultBranches = map[string]bool{
	"main": true, "master": true, "develop": true, "trunk": true,
	"HEAD": true, "HEAD detached": true,
}

// branchTypePrefixes are conventional branch-type segments ("feat/...").
var branchTypePrefixes = map[string]bool{
	"feat": true, "feature": true, "features": true, "fix": true, "bugfix": true,
	"hotfix": true, "chore": true, "refactor": true, "docs": true, "doc": true,
	"test": true, "tests": true, "perf": true, "ci": true, "build": true,
	"style": true, "release": true, "wip": true, "spike": true, "exp": true,
}

// ticketPrefix matches a leading ticket ID such as "ABC-123-" or "123_".
var ticketPrefix = regexp.MustCompile(`^(?:[A-Za-z][A-Za-z0-9]*[-_])?\d+(?:[-_]|$)`)

// TaskTitleFromBranch derives a task title from a git branch name:
// "feat/auth-refresh" → "auth refresh", "fix/ABC-123-login-bug" → "login bug".
// Leading branch-type segments (feat/, fix/, chore/, …) and ticket IDs are
// dropped; with several remaining segments the last one is used (the others
// are usually a user or scope). It returns "" for default branches (main,
// master, develop, trunk), a detached HEAD, or a name with nothing left.
func TaskTitleFromBranch(branch string) string {
	branch = strings.TrimSpace(branch)
	if branch == "" || defaultBranches[branch] {
		return ""
	}
	segments := strings.Split(branch, "/")
	name := segments[len(segments)-1]
	if branchTypePrefixes[strings.ToLower(name)] {
		return ""
	}
	// A type glued to the name: "feat-auth-refresh".
	if len(segments) == 1 {
		if i := strings.IndexAny(name, "-_"); i > 0 && branchTypePrefixes[strings.ToLower(name[:i])] {
			name = name[i+1:]
		}
	}
	name = ticketPrefix.ReplaceAllString(name, "")
	name = strings.NewReplacer("-", " ", "_", " ", ".", " ").Replace(name)
	return strings.Join(strings.Fields(name), " ")
}

// BranchStat aggregates completed work sessions per git branch.
type BranchStat struct {
	Branch       string        `json:"branch"`
	SessionCount int           `json:"session_count"`
	TotalTime    time.Duration `json:"total_time"`
	Commits      int           `json:"commits"`
	Insertions   int           `json:"insertions"`
	Deletions    int           `json:"deletions"`
}

// BranchStats aggregates the completed work sessions among sessions by the
// git branch they started on, most focus time first (ties by branch name).
// Sessions without a branch (no git, or recorded by old versions) are left
// out. Commits and line counts come from the sessions' git summaries.
func BranchStats(sessions []*PomodoroSession) []BranchStat {
	byBranch := map[string]*BranchStat{}
	for _, s := range sessions {
		if s == nil || !s.IsWorkSession() || s.Status != SessionStatusCompleted || s.GitBranch == "" {
			continue
		}
		b, ok := byBranch[s.GitBranch]
		if !ok {
			b = &BranchStat{Branch: s.GitBranch}
			byBranch[s.GitBranch] = b
		}
		b.SessionCount++
		b.TotalTime += s.Duration
		if s.GitSummary != nil {
			b.Commits += s.GitSummary.Commits
			b.Insertions += s.GitSummary.Insertions
			b.Deletions += s.GitSummary.Deletions
		}
	}
	stats := make([]BranchStat, 0, len(byBranch))
	for _, b := range byBranch {
		stats = append(stats, *b)
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].TotalTime != stats[j].TotalTime {
			return stats[i].TotalTime > stats[j].TotalTime
		}
		return stats[i].Branch < stats[j].Branch
	})
	return stats
}

// MarshalJSON implements json.Marshaler (durations as in report_json.go).
func (b BranchStat) MarshalJSON() ([]byte, error) {
	type plain BranchStat
	return json.Marshal(struct {
		plain
		TotalTime    string  `json:"total_time"`
		TotalMinutes float64 `json:"total_minutes"`
	}{plain(b), b.TotalTime.String(), jsonMinutes(b.TotalTime)})
}

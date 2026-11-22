package domain

import "time"

// InsightKind names the detector that produced an insight.
type InsightKind string

const (
	InsightBestTime        InsightKind = "best_time"
	InsightBestDay         InsightKind = "best_day"
	InsightAbandonTime     InsightKind = "abandon_time"
	InsightAbandonDay      InsightKind = "abandon_day"
	InsightTagEffort       InsightKind = "tag_effort"
	InsightOptimalDuration InsightKind = "optimal_duration"
	InsightTrend           InsightKind = "trend"
	InsightStreak          InsightKind = "streak"
	InsightDistractions    InsightKind = "distractions"
)

// Insight is one actionable finding derived from session history.
type Insight struct {
	Kind InsightKind `json:"kind"`
	// Message is the actionable, localized sentence.
	Message string `json:"message"`
	// Evidence is the localized numbers that back the message.
	Evidence string `json:"evidence"`
	// Score ranks insights by relevance (higher first), roughly 0..1.
	Score float64 `json:"score"`
	// Data carries the raw figures behind the insight for machine consumers.
	Data map[string]any `json:"data,omitempty"`
}

// InsightGap reports a detector that did not have enough data to speak.
type InsightGap struct {
	Kind InsightKind `json:"kind"`
	// Missing is how many more sessions the detector needs.
	Missing int `json:"missing"`
	// Message is the localized explanation.
	Message string `json:"message"`
}

// InsightsReport is the result of analyzing a window of session history.
type InsightsReport struct {
	Period string    `json:"period"`
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	// SessionsAnalyzed counts work sessions that ended in the window
	// (completed, voided or cancelled).
	SessionsAnalyzed int          `json:"sessions_analyzed"`
	Insights         []Insight    `json:"insights"`
	Gaps             []InsightGap `json:"gaps"`
}

// Top returns the most relevant insight, or nil when there is none.
func (r *InsightsReport) Top() *Insight {
	if r == nil || len(r.Insights) == 0 {
		return nil
	}
	return &r.Insights[0]
}

// InsightSession is the projection of a finished work session that the
// insight detectors analyze. StartedAt is in local time.
type InsightSession struct {
	ID           string
	TaskID       string
	TaskDone     bool
	Status       SessionStatus
	Methodology  Methodology
	StartedAt    time.Time
	Duration     time.Duration
	FocusScore   *int
	Distractions []Distraction
	// Tags is the union of the session's tags and its task's tags.
	Tags []string
}

// Abandoned reports whether the session was voided or cancelled.
func (s InsightSession) Abandoned() bool {
	return s.Status == SessionStatusInterrupted || s.Status == SessionStatusCancelled
}

package domain

import (
	"time"
)

// Report is the fully aggregated view of a period, produced by ReportService.
type Report struct {
	Period        string            `json:"period"`
	Label         string            `json:"label"`
	Start         time.Time         `json:"start"`
	End           time.Time         `json:"end"`
	Summary       ReportSummary     `json:"summary"`
	Daily         []DayRow          `json:"daily"`
	Heatmap       []HourBucket      `json:"heatmap"`
	Tags          []TagStat         `json:"tags"`
	Methodologies []MethodologyStat `json:"methodologies"`
	Streaks       StreakInfo        `json:"streaks"`
}

// ReportSummary aggregates the whole period.
type ReportSummary struct {
	TotalSessions    int           `json:"total_sessions"`
	TotalWorkTime    time.Duration `json:"total_work_time"`
	AvgFocusScore    float64       `json:"avg_focus_score"`
	FocusScoreCount  int           `json:"focus_score_count"`
	DistractionCount int           `json:"distraction_count"`
	HighlightDays    int           `json:"highlight_days"`
	HighlightHitRate float64       `json:"highlight_hit_rate"`
}

// DayRow holds one day's activity.
type DayRow struct {
	Date          string        `json:"date"`
	Day           string        `json:"day"`
	WorkSessions  int           `json:"work_sessions"`
	Breaks        int           `json:"breaks"`
	WorkTime      time.Duration `json:"work_time"`
	AvgFocusScore float64       `json:"avg_focus_score"`
	FocusCount    int           `json:"focus_count"`
	Distractions  int           `json:"distractions"`
	HasHighlight  bool          `json:"has_highlight"`
}

// HourBucket holds aggregated work time for one hour-of-day bucket (24h heatmap).
type HourBucket struct {
	Hour    string        `json:"hour"`
	Minutes time.Duration `json:"minutes"`
}

// TagStat aggregates time and focus across sessions carrying a tag.
type TagStat struct {
	Tag             string        `json:"tag"`
	SessionCount    int           `json:"session_count"`
	TotalTime       time.Duration `json:"total_time"`
	AvgFocusScore   float64       `json:"avg_focus_score"`
	FocusScoreCount int           `json:"focus_score_count"`
}

// MethodologyStat aggregates sessions per methodology.
type MethodologyStat struct {
	Methodology     Methodology   `json:"methodology"`
	Label           string        `json:"label"`
	SessionCount    int           `json:"session_count"`
	TotalTime       time.Duration `json:"total_time"`
	AvgFocusScore   float64       `json:"avg_focus_score"`
	FocusScoreCount int           `json:"focus_score_count"`
}

// StreakInfo describes consecutive-day working streaks.
type StreakInfo struct {
	CurrentDays int `json:"current_days"`
	LongestDays int `json:"longest_days"`
}

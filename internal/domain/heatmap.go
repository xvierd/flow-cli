package domain

import "time"

// HeatmapLevels is the number of intensity levels in a year heatmap (0 = none).
const HeatmapLevels = 5

// HeatmapCell is one local calendar day in a year heatmap.
type HeatmapCell struct {
	Date    time.Time `json:"date"`    // local midnight
	Minutes int       `json:"minutes"` // completed focus minutes that day
	Level   int       `json:"level"`   // 0..HeatmapLevels-1
	Future  bool      `json:"future"`  // after the reference day (rendered blank)
}

// YearHeatmap is a GitHub-style contribution grid: 53 week columns of 7 days
// (Monday first), ending with the week that contains the reference day.
type YearHeatmap struct {
	Start        time.Time        `json:"start"` // Monday of the first column
	End          time.Time        `json:"end"`   // reference day (inclusive)
	Weeks        [][7]HeatmapCell `json:"weeks"`
	MaxMinutes   int              `json:"max_minutes"`
	TotalMinutes int              `json:"total_minutes"`
	ActiveDays   int              `json:"active_days"`
}

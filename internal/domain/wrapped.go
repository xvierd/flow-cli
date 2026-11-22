package domain

import "time"

// FunFactKind identifies which "fun fact" a Wrapped summary highlights. The
// presentation layer turns the kind and its value into localized text.
type FunFactKind string

const (
	FunFactNone        FunFactKind = ""
	FunFactNightOwl    FunFactKind = "night_owl"   // Value: percent of focus started at or after 20:00
	FunFactEarlyBird   FunFactKind = "early_bird"  // Value: percent of focus started before 09:00
	FunFactWeekend     FunFactKind = "weekend"     // Value: percent of focus on Saturday/Sunday
	FunFactMarathon    FunFactKind = "marathon"    // Value: longest single session in minutes
	FunFactMovies      FunFactKind = "movies"      // Value: number of 2h movies the focus time equals
	FunFactFirstSteps  FunFactKind = "first_steps" // Value: total minutes (small totals)
	FunFactConsistency FunFactKind = "consistency" // Value: percent of days in the period with focus
)

// FunFact is a single highlighted curiosity about a Wrapped period.
type FunFact struct {
	Kind  FunFactKind `json:"kind"`
	Value int         `json:"value"`
}

// Wrapped is a shareable "year in review"-style summary of focus over a period.
// All day/hour attributions use the local time zone of the reference time.
type Wrapped struct {
	Period string    `json:"period"`
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`

	TotalFocus time.Duration `json:"total_focus"`
	Sessions   int           `json:"sessions"`
	ActiveDays int           `json:"active_days"`
	PeriodDays int           `json:"period_days"`

	// CurrentStreak counts consecutive days with focus ending at the reference
	// day (a quiet "today" does not break it). LongestStreak is the longest run
	// inside the period.
	CurrentStreak int `json:"current_streak"`
	LongestStreak int `json:"longest_streak"`

	BestDay      time.Time     `json:"best_day"` // zero when there is no focus
	BestDayFocus time.Duration `json:"best_day_focus"`

	BestHour      int           `json:"best_hour"` // 0-23, -1 when there is no focus
	BestHourFocus time.Duration `json:"best_hour_focus"`

	TopTag      string        `json:"top_tag,omitempty"`
	TopTagFocus time.Duration `json:"top_tag_focus"`

	TopTask      string        `json:"top_task,omitempty"`
	TopTaskFocus time.Duration `json:"top_task_focus"`

	LongestSession time.Duration `json:"longest_session"`

	FunFact FunFact `json:"fun_fact"`
}

// Package services implements the application layer (use cases)
// following hexagonal architecture principles.
package services

import (
	"fmt"
	"time"
)

// ReportPeriod names a supported aggregation period.
type ReportPeriod string

const (
	ReportPeriodWeek  ReportPeriod = "week"
	ReportPeriodMonth ReportPeriod = "month"
)

// PeriodRange returns the half-open [start, end) range for a report period
// relative to the reference time. Weeks start on Monday; months on the 1st.
func PeriodRange(p ReportPeriod, ref time.Time) (time.Time, time.Time, error) {
	switch p {
	case ReportPeriodWeek:
		weekday := int(ref.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start := time.Date(ref.Year(), ref.Month(), ref.Day()-(weekday-1), 0, 0, 0, 0, ref.Location())
		return start, start.AddDate(0, 0, 7), nil
	case ReportPeriodMonth:
		start := time.Date(ref.Year(), ref.Month(), 1, 0, 0, 0, 0, ref.Location())
		return start, start.AddDate(0, 1, 0), nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unsupported period %q", p)
	}
}

// startOfDay returns the local midnight boundary for the given time.
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

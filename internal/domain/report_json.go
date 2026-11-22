package domain

import (
	"encoding/json"
	"math"
	"time"
)

// Report durations are time.Duration in Go, which encoding/json writes as
// integer nanoseconds. The JSON form instead carries each duration twice: as
// a Go duration string (the same format as `flow status --json` and the MCP
// tools) and as a number of minutes under a matching "_minutes" key.

// jsonMinutes converts d to minutes rounded to one decimal place.
func jsonMinutes(d time.Duration) float64 {
	return math.Round(d.Minutes()*10) / 10
}

// MarshalJSON implements json.Marshaler.
func (s ReportSummary) MarshalJSON() ([]byte, error) {
	type plain ReportSummary
	return json.Marshal(struct {
		plain
		TotalWorkTime    string  `json:"total_work_time"`
		TotalWorkMinutes float64 `json:"total_work_minutes"`
	}{plain(s), s.TotalWorkTime.String(), jsonMinutes(s.TotalWorkTime)})
}

// MarshalJSON implements json.Marshaler.
func (d DayRow) MarshalJSON() ([]byte, error) {
	type plain DayRow
	return json.Marshal(struct {
		plain
		WorkTime    string  `json:"work_time"`
		WorkMinutes float64 `json:"work_minutes"`
	}{plain(d), d.WorkTime.String(), jsonMinutes(d.WorkTime)})
}

// MarshalJSON implements json.Marshaler. The bucket's "minutes" key is a
// number of minutes.
func (h HourBucket) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Hour    string  `json:"hour"`
		Minutes float64 `json:"minutes"`
	}{h.Hour, jsonMinutes(h.Minutes)})
}

// MarshalJSON implements json.Marshaler.
func (t TagStat) MarshalJSON() ([]byte, error) {
	type plain TagStat
	return json.Marshal(struct {
		plain
		TotalTime    string  `json:"total_time"`
		TotalMinutes float64 `json:"total_minutes"`
	}{plain(t), t.TotalTime.String(), jsonMinutes(t.TotalTime)})
}

// MarshalJSON implements json.Marshaler.
func (m MethodologyStat) MarshalJSON() ([]byte, error) {
	type plain MethodologyStat
	return json.Marshal(struct {
		plain
		TotalTime    string  `json:"total_time"`
		TotalMinutes float64 `json:"total_minutes"`
	}{plain(m), m.TotalTime.String(), jsonMinutes(m.TotalTime)})
}

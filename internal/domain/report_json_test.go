package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestReportJSON_Durations(t *testing.T) {
	r := Report{
		Summary:       ReportSummary{TotalSessions: 2, TotalWorkTime: 75 * time.Minute},
		Daily:         []DayRow{{Date: "2026-11-22", WorkTime: 25 * time.Minute}},
		Heatmap:       []HourBucket{{Hour: "09:00", Minutes: 18 * time.Second}},
		Tags:          []TagStat{{Tag: "coding", TotalTime: time.Hour}},
		Methodologies: []MethodologyStat{{Methodology: MethodologyPomodoro, TotalTime: 30 * time.Minute}},
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var got struct {
		Summary map[string]interface{}   `json:"summary"`
		Daily   []map[string]interface{} `json:"daily"`
		Heatmap []map[string]interface{} `json:"heatmap"`
		Tags    []map[string]interface{} `json:"tags"`
		Methods []map[string]interface{} `json:"methodologies"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	checks := []struct {
		name string
		m    map[string]interface{}
		key  string
		want interface{}
	}{
		{"summary string", got.Summary, "total_work_time", "1h15m0s"},
		{"summary minutes", got.Summary, "total_work_minutes", 75.0},
		{"summary other fields kept", got.Summary, "total_sessions", 2.0},
		{"daily string", got.Daily[0], "work_time", "25m0s"},
		{"daily minutes", got.Daily[0], "work_minutes", 25.0},
		{"daily other fields kept", got.Daily[0], "date", "2026-11-22"},
		{"heatmap minutes", got.Heatmap[0], "minutes", 0.3},
		{"tag minutes", got.Tags[0], "total_minutes", 60.0},
		{"tag string", got.Tags[0], "total_time", "1h0m0s"},
		{"methodology minutes", got.Methods[0], "total_minutes", 30.0},
		{"methodology kept", got.Methods[0], "methodology", "pomodoro"},
	}
	for _, c := range checks {
		if c.m[c.key] != c.want {
			t.Errorf("%s: %s = %v (%T), want %v", c.name, c.key, c.m[c.key], c.m[c.key], c.want)
		}
	}
}

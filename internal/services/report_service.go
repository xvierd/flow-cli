package services

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/ports"
)

// ReportService computes aggregated reports over session history.
type ReportService struct {
	storage ports.Storage
}

// NewReportService creates a new report service.
func NewReportService(storage ports.Storage) *ReportService {
	return &ReportService{storage: storage}
}

// GetReport builds an aggregated report for the given period relative to now.
func (s *ReportService) GetReport(ctx context.Context, period ReportPeriod) (*domain.Report, error) {
	now := time.Now()
	start, end, err := PeriodRange(period, now)
	if err != nil {
		return nil, err
	}
	return s.GetReportForRange(ctx, period, start, end, now)
}

// GetReportForRange builds an aggregated report over an explicit [start, end) range.
func (s *ReportService) GetReportForRange(ctx context.Context, period ReportPeriod, start, end, ref time.Time) (*domain.Report, error) {
	// Streaks are computed over full history (365 days back from ref), not
	// capped to the report window, so load sessions from the earlier bound.
	since := start
	if streakSince := ref.AddDate(0, 0, -365); streakSince.Before(since) {
		since = streakSince
	}
	sessions, err := s.storage.Sessions().FindRecent(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to load sessions: %w", err)
	}

	report := &domain.Report{
		Period: string(period),
		Label:  periodLabel(period, start),
		Start:  start,
		End:    end,
	}

	daily := map[string]*domain.DayRow{}
	heat := make(map[int]time.Duration)
	methods := make(map[domain.Methodology]*domain.MethodologyStat)
	workDays := make(map[string]bool)   // in-window days with completed work (highlight hit rate)
	streakDays := make(map[string]bool) // full-history days with completed work

	for _, s := range sessions {
		isWork := s.IsWorkSession()
		isCompleted := s.Status == domain.SessionStatusCompleted
		dayKey := s.StartedAt.Format("2006-01-02")

		if isWork && isCompleted {
			streakDays[dayKey] = true
		}

		if s.StartedAt.Before(start) || !s.StartedAt.Before(end) {
			continue
		}
		// Only completed sessions count, matching GetPeriodStats semantics.
		if !isCompleted {
			continue
		}

		if isWork {
			workDays[dayKey] = true
			report.Summary.TotalSessions++
			report.Summary.TotalWorkTime += s.Duration
			heat[s.StartedAt.Local().Hour()] += s.Duration

			m := s.Methodology
			if m == "" {
				m = domain.MethodologyPomodoro
			}
			meth, ok := methods[m]
			if !ok {
				meth = &domain.MethodologyStat{Methodology: m, Label: m.Label()}
				methods[m] = meth
			}
			meth.SessionCount++
			meth.TotalTime += s.Duration
			if s.FocusScore != nil {
				meth.FocusScoreCount++
				meth.AvgFocusScore += float64(*s.FocusScore)
			}
			if s.FocusScore != nil {
				report.Summary.FocusScoreCount++
				report.Summary.AvgFocusScore += float64(*s.FocusScore)
			}
			report.Summary.DistractionCount += len(s.Distractions)
		}

		row, ok := daily[dayKey]
		if !ok {
			day := fmt.Sprintf("%s %s %d", i18n.DayName(s.StartedAt.Weekday()), i18n.MonthName(s.StartedAt.Month()), s.StartedAt.Day())
			row = &domain.DayRow{Date: dayKey, Day: day}
			daily[dayKey] = row
		}
		switch {
		case isWork:
			row.WorkSessions++
			row.WorkTime += s.Duration
			if s.FocusScore != nil {
				row.FocusCount++
				row.AvgFocusScore += float64(*s.FocusScore)
			}
			row.Distractions += len(s.Distractions)
		case s.IsBreakSession():
			row.Breaks++
		}
	}

	report.Daily = finalizeDays(daily)
	report.Heatmap = finalizeHeatmap(heat)
	report.Methodologies = finalizeMethodologies(methods)
	if report.Summary.FocusScoreCount > 0 {
		report.Summary.AvgFocusScore /= float64(report.Summary.FocusScoreCount)
	}

	highlightDays := 0
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		hl, err := s.storage.Tasks().FindTodayHighlight(ctx, d)
		if err == nil && hl != nil {
			highlightDays++
			for i := range report.Daily {
				if report.Daily[i].Date == d.Format("2006-01-02") {
					report.Daily[i].HasHighlight = true
					break
				}
			}
		}
	}
	report.Summary.HighlightDays = highlightDays
	if len(workDays) > 0 {
		report.Summary.HighlightHitRate = float64(highlightDays) / float64(len(workDays))
	}

	report.Streaks = computeStreaks(streakDays, ref)

	tags, err := s.storage.Sessions().GetTagStats(ctx, start, end)
	if err == nil {
		report.Tags = tags
	}

	return report, nil
}

// periodLabel renders a human label for a period's start.
func periodLabel(p ReportPeriod, start time.Time) string {
	switch p {
	case ReportPeriodMonth:
		return fmt.Sprintf("%s %d", i18n.FullMonthName(start.Month()), start.Year())
	default:
		return i18n.T("Week of %s", fmt.Sprintf("%s %d", i18n.MonthName(start.Month()), start.Day()))
	}
}

// finalizeDays sorts day rows by date and computes per-row focus averages.
func finalizeDays(daily map[string]*domain.DayRow) []domain.DayRow {
	rows := make([]domain.DayRow, 0, len(daily))
	for _, row := range daily {
		if row.FocusCount > 0 {
			row.AvgFocusScore /= float64(row.FocusCount)
		}
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Date < rows[j].Date })
	return rows
}

// finalizeHeatmap expands hourly buckets into a full 24h timeline.
func finalizeHeatmap(heat map[int]time.Duration) []domain.HourBucket {
	buckets := make([]domain.HourBucket, 0, len(heat))
	for hour, minutes := range heat {
		buckets = append(buckets, domain.HourBucket{
			Hour:    fmt.Sprintf("%02d:00", hour),
			Minutes: minutes,
		})
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].Hour < buckets[j].Hour })
	return buckets
}

// finalizeMethodologies sorts methodology stats by total time descending.
func finalizeMethodologies(methods map[domain.Methodology]*domain.MethodologyStat) []domain.MethodologyStat {
	stats := make([]domain.MethodologyStat, 0, len(methods))
	for _, m := range methods {
		if m.FocusScoreCount > 0 {
			m.AvgFocusScore /= float64(m.FocusScoreCount)
		}
		stats = append(stats, *m)
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].TotalTime > stats[j].TotalTime })
	return stats
}

// computeStreaks derives current and longest streaks of consecutive days with
// completed work sessions, walking up to 365 days back from ref. The workDays
// map must cover full history, not just the report window.
func computeStreaks(workDays map[string]bool, ref time.Time) domain.StreakInfo {
	dayKey := func(t time.Time) string { return t.Format("2006-01-02") }

	longest, current := 0, 0
	for d := ref.AddDate(0, 0, -364); !d.After(ref); d = d.AddDate(0, 0, 1) {
		if workDays[dayKey(d)] {
			current++
			if current > longest {
				longest = current
			}
		} else {
			current = 0
		}
	}

	running := 0
	for i := 0; i < 366; i++ {
		d := ref.AddDate(0, 0, -i)
		if workDays[dayKey(d)] {
			running++
		} else if i == 0 {
			continue
		} else {
			break
		}
	}

	return domain.StreakInfo{CurrentDays: running, LongestDays: longest}
}

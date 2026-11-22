package services

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

// HeatmapWeeks is the number of week columns in the year heatmap.
const HeatmapWeeks = 53

// WrappedService builds shareable summaries (wrapped card, year heatmap,
// badge totals) from session history.
type WrappedService struct {
	storage ports.Storage
}

// NewWrappedService creates a new wrapped service.
func NewWrappedService(storage ports.Storage) *WrappedService {
	return &WrappedService{storage: storage}
}

// GetWrapped loads history and builds the Wrapped summary for the period that
// contains ref.
func (s *WrappedService) GetWrapped(ctx context.Context, period ReportPeriod, ref time.Time) (*domain.Wrapped, error) {
	start, end, err := PeriodRange(period, ref)
	if err != nil {
		return nil, err
	}
	// Load a year of history before the window too, so the current streak is
	// not capped at the period start.
	since := start
	if s := ref.AddDate(-1, 0, -1); s.Before(since) {
		since = s
	}
	sessions, err := s.storage.Sessions().FindRecent(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to load sessions: %w", err)
	}
	titles := map[string]string{}
	if tasks, err := s.storage.Tasks().FindAll(ctx, nil); err == nil {
		for _, t := range tasks {
			titles[t.ID] = t.Title
		}
	}
	return BuildWrapped(sessions, titles, period, start, end, ref), nil
}

// GetYearHeatmap loads the last 53 weeks of history and builds the heatmap.
func (s *WrappedService) GetYearHeatmap(ctx context.Context, ref time.Time) (*domain.YearHeatmap, error) {
	first := heatmapStart(ref)
	sessions, err := s.storage.Sessions().FindRecent(ctx, first.AddDate(0, 0, -1))
	if err != nil {
		return nil, fmt.Errorf("failed to load sessions: %w", err)
	}
	return BuildYearHeatmap(sessions, ref), nil
}

// FocusTotal returns the completed focus time in the period containing ref.
func (s *WrappedService) FocusTotal(ctx context.Context, period ReportPeriod, ref time.Time) (time.Duration, error) {
	start, end, err := PeriodRange(period, ref)
	if err != nil {
		return 0, err
	}
	sessions, err := s.storage.Sessions().FindRecent(ctx, start)
	if err != nil {
		return 0, fmt.Errorf("failed to load sessions: %w", err)
	}
	var total time.Duration
	for _, sess := range sessions {
		if countsAsFocus(sess) && inRange(sess.StartedAt, start, end) {
			total += sess.Duration
		}
	}
	return total, nil
}

// countsAsFocus reports whether a session contributes focus time: completed
// work sessions only, matching the report and stats semantics.
func countsAsFocus(s *domain.PomodoroSession) bool {
	return s != nil && s.IsWorkSession() && s.Status == domain.SessionStatusCompleted
}

func inRange(t, start, end time.Time) bool {
	return !t.Before(start) && t.Before(end)
}

// dayOf returns local midnight of t in loc.
func dayOf(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// BuildWrapped aggregates sessions into a Wrapped summary over [start, end).
// titles maps task IDs to titles. Days and hours are attributed in the local
// time zone of ref. Ties are broken deterministically (earliest day/hour,
// alphabetical tag/task).
func BuildWrapped(sessions []*domain.PomodoroSession, titles map[string]string, period ReportPeriod, start, end, ref time.Time) *domain.Wrapped {
	loc := ref.Location()
	w := &domain.Wrapped{
		Period:   string(period),
		Start:    start,
		End:      end,
		BestHour: -1,
	}

	perDay := map[time.Time]time.Duration{}
	historyDays := map[time.Time]bool{}
	var perHour [24]time.Duration
	perTag := map[string]time.Duration{}
	perTask := map[string]time.Duration{}
	var night, early, weekend time.Duration

	for _, s := range sessions {
		if !countsAsFocus(s) {
			continue
		}
		day := dayOf(s.StartedAt, loc)
		historyDays[day] = true
		if !inRange(s.StartedAt, start, end) {
			continue
		}
		local := s.StartedAt.In(loc)
		w.Sessions++
		w.TotalFocus += s.Duration
		perDay[day] += s.Duration
		perHour[local.Hour()] += s.Duration
		if s.Duration > w.LongestSession {
			w.LongestSession = s.Duration
		}
		for _, tag := range s.Tags {
			if tag != "" {
				perTag[tag] += s.Duration
			}
		}
		if s.TaskID != nil {
			if title := titles[*s.TaskID]; title != "" {
				perTask[title] += s.Duration
			}
		}
		switch {
		case local.Hour() >= 20:
			night += s.Duration
		case local.Hour() < 9:
			early += s.Duration
		}
		if wd := local.Weekday(); wd == time.Saturday || wd == time.Sunday {
			weekend += s.Duration
		}
	}

	// Period length in local calendar days, clamped to ref for the current
	// period so "consistency" is measured against days that have happened.
	lastDay := dayOf(end.Add(-time.Nanosecond), loc)
	if refDay := dayOf(ref, loc); refDay.Before(lastDay) && !refDay.Before(dayOf(start, loc)) {
		lastDay = refDay
	}
	run := 0
	for d := dayOf(start, loc); !d.After(lastDay); d = d.AddDate(0, 0, 1) {
		w.PeriodDays++
		focus, ok := perDay[d]
		if ok {
			w.ActiveDays++
			run++
			if run > w.LongestStreak {
				w.LongestStreak = run
			}
			if focus > w.BestDayFocus {
				w.BestDay, w.BestDayFocus = d, focus
			}
		} else {
			run = 0
		}
	}

	for h, d := range perHour {
		if d > w.BestHourFocus {
			w.BestHour, w.BestHourFocus = h, d
		}
	}
	w.TopTag, w.TopTagFocus = topKey(perTag)
	w.TopTask, w.TopTaskFocus = topKey(perTask)
	w.CurrentStreak = currentStreak(historyDays, dayOf(ref, loc))
	w.FunFact = pickFunFact(w, night, early, weekend)
	return w
}

// topKey returns the key with the largest duration; ties go to the
// alphabetically first key.
func topKey(m map[string]time.Duration) (string, time.Duration) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	best, bestD := "", time.Duration(0)
	for _, k := range keys {
		if m[k] > bestD {
			best, bestD = k, m[k]
		}
	}
	return best, bestD
}

// currentStreak counts consecutive local days with focus ending at today.
// A day without focus yet today does not break a streak that ran until
// yesterday.
func currentStreak(days map[time.Time]bool, today time.Time) int {
	d := today
	if !days[d] {
		d = d.AddDate(0, 0, -1)
	}
	n := 0
	for days[d] {
		n++
		d = d.AddDate(0, 0, -1)
	}
	return n
}

// pickFunFact chooses the most striking curiosity, in priority order.
func pickFunFact(w *domain.Wrapped, night, early, weekend time.Duration) domain.FunFact {
	if w.TotalFocus <= 0 {
		return domain.FunFact{}
	}
	pct := func(d time.Duration) int {
		return int(math.Round(float64(d) / float64(w.TotalFocus) * 100))
	}
	switch {
	case w.TotalFocus < time.Hour:
		return domain.FunFact{Kind: domain.FunFactFirstSteps, Value: int(w.TotalFocus.Minutes())}
	case pct(night) >= 30:
		return domain.FunFact{Kind: domain.FunFactNightOwl, Value: pct(night)}
	case pct(early) >= 30:
		return domain.FunFact{Kind: domain.FunFactEarlyBird, Value: pct(early)}
	case pct(weekend) >= 35:
		return domain.FunFact{Kind: domain.FunFactWeekend, Value: pct(weekend)}
	case w.LongestSession >= 90*time.Minute:
		return domain.FunFact{Kind: domain.FunFactMarathon, Value: int(w.LongestSession.Minutes())}
	case w.PeriodDays >= 7 && w.ActiveDays*100/w.PeriodDays >= 70:
		return domain.FunFact{Kind: domain.FunFactConsistency, Value: w.ActiveDays * 100 / w.PeriodDays}
	case w.TotalFocus >= 2*time.Hour:
		return domain.FunFact{Kind: domain.FunFactMovies, Value: int(w.TotalFocus / (2 * time.Hour))}
	default:
		return domain.FunFact{Kind: domain.FunFactConsistency, Value: w.ActiveDays * 100 / max(w.PeriodDays, 1)}
	}
}

// heatmapStart returns the Monday that begins the first of the 53 columns
// ending with the week that contains ref.
func heatmapStart(ref time.Time) time.Time {
	today := dayOf(ref, ref.Location())
	offset := (int(today.Weekday()) + 6) % 7 // Monday = 0
	lastMonday := today.AddDate(0, 0, -offset)
	return lastMonday.AddDate(0, 0, -7*(HeatmapWeeks-1))
}

// HeatmapLevel maps a day's minutes to an intensity level 0..4 relative to the
// busiest day: 0 means no focus; 1..4 split (0, max] into quarters.
func HeatmapLevel(minutes, maxMinutes int) int {
	if minutes <= 0 || maxMinutes <= 0 {
		return 0
	}
	top := domain.HeatmapLevels - 1
	lvl := int(math.Ceil(float64(minutes) * float64(top) / float64(maxMinutes)))
	return min(max(lvl, 1), top)
}

// BuildYearHeatmap builds a 53x7 grid of daily focus minutes ending with the
// week containing ref. Days are local calendar days in ref's time zone;
// stepping uses calendar arithmetic so DST changes and Feb 29 are handled.
func BuildYearHeatmap(sessions []*domain.PomodoroSession, ref time.Time) *domain.YearHeatmap {
	loc := ref.Location()
	first := heatmapStart(ref)
	today := dayOf(ref, loc)

	perDay := map[time.Time]time.Duration{}
	for _, s := range sessions {
		if countsAsFocus(s) {
			perDay[dayOf(s.StartedAt, loc)] += s.Duration
		}
	}

	hm := &domain.YearHeatmap{Start: first, End: today, Weeks: make([][7]domain.HeatmapCell, HeatmapWeeks)}
	for w := 0; w < HeatmapWeeks; w++ {
		for d := 0; d < 7; d++ {
			date := first.AddDate(0, 0, w*7+d)
			cell := domain.HeatmapCell{Date: date, Future: date.After(today)}
			if !cell.Future {
				cell.Minutes = int(perDay[date].Minutes())
				hm.TotalMinutes += cell.Minutes
				if cell.Minutes > 0 {
					hm.ActiveDays++
				}
				hm.MaxMinutes = max(hm.MaxMinutes, cell.Minutes)
			}
			hm.Weeks[w][d] = cell
		}
	}
	for w := range hm.Weeks {
		for d := range hm.Weeks[w] {
			hm.Weeks[w][d].Level = HeatmapLevel(hm.Weeks[w][d].Minutes, hm.MaxMinutes)
		}
	}
	return hm
}

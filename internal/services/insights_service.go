package services

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/ports"
)

// MaxInsights caps how many findings a report carries.
const MaxInsights = 5

// Minimum data each detector needs before it speaks. Below these, the
// detector reports how much is missing instead of guessing.
const (
	minCompletedForSlots    = 8  // best time slot / best day
	minEndedForAbandon      = 10 // void patterns by time / day
	minSideForAbandonSplit  = 4  // sessions on each side of an hour split
	minDaySessionsAbandon   = 3  // sessions on a weekday to rate it
	minTasksPerTag          = 2  // finished tasks per tag for tag effort
	minEndedForDuration     = 10 // optimal session length
	minBandSessions         = 3  // sessions per length band
	minCompletedForTrend    = 3  // completed sessions in each period
	minStreakDays           = 3  // streak worth mentioning
	minCategorizedDistracts = 5  // categorized Deep Work distractions
)

// historyDays is how far back streaks are computed.
const historyDays = 365

// InsightsService turns session history into a few actionable findings.
type InsightsService struct {
	storage ports.Storage
}

// NewInsightsService creates a new insights service.
func NewInsightsService(storage ports.Storage) *InsightsService {
	return &InsightsService{storage: storage}
}

// InsightWindow returns the rolling [start, end) window for a period: the
// last 7 (week) or 30 (month) local days, today included. Rolling windows
// keep the trend comparison fair early in a calendar week or month.
func InsightWindow(p ReportPeriod, ref time.Time) (time.Time, time.Time, error) {
	days, err := insightDays(p)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end := startOfDay(ref).AddDate(0, 0, 1)
	return end.AddDate(0, 0, -days), end, nil
}

func insightDays(p ReportPeriod) (int, error) {
	switch p {
	case ReportPeriodWeek:
		return 7, nil
	case ReportPeriodMonth:
		return 30, nil
	default:
		return 0, fmt.Errorf("unsupported period %q", p)
	}
}

// GetInsights analyzes the rolling window for period ending today.
func (s *InsightsService) GetInsights(ctx context.Context, period ReportPeriod) (*domain.InsightsReport, error) {
	return s.GetInsightsAt(ctx, period, time.Now())
}

// GetInsightsAt analyzes the rolling window for period ending on ref's day.
func (s *InsightsService) GetInsightsAt(ctx context.Context, period ReportPeriod, ref time.Time) (*domain.InsightsReport, error) {
	start, end, err := InsightWindow(period, ref)
	if err != nil {
		return nil, err
	}
	history, err := s.storage.Sessions().GetInsightSessions(ctx, end.AddDate(0, 0, -historyDays), end)
	if err != nil {
		return nil, fmt.Errorf("failed to load sessions: %w", err)
	}
	return AnalyzeInsights(period, start, end, ref, history), nil
}

// TopInsight returns the most relevant insight's message for period, or ""
// when there is none (or on error: the top line is decoration, never fatal).
func (s *InsightsService) TopInsight(ctx context.Context, period ReportPeriod) string {
	report, err := s.GetInsights(ctx, period)
	if err != nil {
		return ""
	}
	if top := report.Top(); top != nil {
		return top.Message
	}
	return ""
}

// newInsightInput splits history into the window, its completed subset
// and the previous window's completed sessions.
func newInsightInput(start, end, ref time.Time, history []domain.InsightSession) *insightInput {
	days := int(math.Round(end.Sub(start).Hours() / 24))
	prevStart := start.AddDate(0, 0, -days)
	in := &insightInput{start: start, end: end, ref: ref, days: days, history: history}
	for _, s := range history {
		switch {
		case !s.StartedAt.Before(end):
			continue
		case !s.StartedAt.Before(start):
			in.window = append(in.window, s)
			if s.Status == domain.SessionStatusCompleted {
				in.completed = append(in.completed, s)
			}
		case !s.StartedAt.Before(prevStart) && s.Status == domain.SessionStatusCompleted:
			in.prev = append(in.prev, s)
		}
	}
	return in
}

// insightInput is what every detector sees.
type insightInput struct {
	start, end time.Time
	ref        time.Time
	days       int
	window     []domain.InsightSession // ended work sessions in [start, end)
	completed  []domain.InsightSession // completed subset of window
	prev       []domain.InsightSession // completed sessions in the previous window
	history    []domain.InsightSession // everything loaded (streaks)
}

type detector func(in *insightInput) (*domain.Insight, *domain.InsightGap)

var detectors = []detector{
	detectBestTime,
	detectBestDay,
	detectAbandonTime,
	detectAbandonDay,
	detectTagEffort,
	detectOptimalDuration,
	detectTrend,
	detectStreak,
	detectDistractions,
}

// AnalyzeInsights runs every detector over history and keeps the most
// relevant findings. history may extend before start (for streaks and the
// previous-period comparison); sessions at or after end are ignored.
func AnalyzeInsights(period ReportPeriod, start, end, ref time.Time, history []domain.InsightSession) *domain.InsightsReport {
	in := newInsightInput(start, end, ref, history)

	report := &domain.InsightsReport{
		Period:           string(period),
		Start:            start,
		End:              end,
		SessionsAnalyzed: len(in.window),
		Insights:         []domain.Insight{},
		Gaps:             []domain.InsightGap{},
	}
	for _, d := range detectors {
		insight, gap := d(in)
		if insight != nil {
			// Messages may start with a lowercase localized day name.
			insight.Message = upperFirst(insight.Message)
			insight.Evidence = upperFirst(insight.Evidence)
			report.Insights = append(report.Insights, *insight)
		}
		if gap != nil {
			report.Gaps = append(report.Gaps, *gap)
		}
	}
	sort.SliceStable(report.Insights, func(i, j int) bool { return report.Insights[i].Score > report.Insights[j].Score })
	if len(report.Insights) > MaxInsights {
		report.Insights = report.Insights[:MaxInsights]
	}
	sort.SliceStable(report.Gaps, func(i, j int) bool { return report.Gaps[i].Missing < report.Gaps[j].Missing })
	return report
}

// -- helpers -----------------------------------------------------------------

func upperFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func minutes(d time.Duration) float64 { return d.Minutes() }

func pct(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return float64(part) / float64(whole) * 100
}

func hoursLabel(d time.Duration) string { return fmt.Sprintf("%.1fh", d.Hours()) }

// slotOf maps a local hour to a 3-hour slot (0..7).
func slotOf(t time.Time) int { return t.Hour() / 3 }

func slotLabel(slot int) string { return fmt.Sprintf("%02d–%02dh", slot*3, slot*3+3) }

type focusAcc struct {
	sum, n int
}

func (f *focusAcc) add(s domain.InsightSession) {
	if s.FocusScore != nil {
		f.sum += *s.FocusScore
		f.n++
	}
}

func (f focusAcc) avg() float64 {
	if f.n == 0 {
		return 0
	}
	return float64(f.sum) / float64(f.n)
}

// -- best time slot ----------------------------------------------------------

func detectBestTime(in *insightInput) (*domain.Insight, *domain.InsightGap) {
	if n := len(in.completed); n < minCompletedForSlots {
		return nil, &domain.InsightGap{Kind: domain.InsightBestTime, Missing: minCompletedForSlots - n,
			Message: i18n.T("Best time of day: %d more completed sessions needed", minCompletedForSlots-n)}
	}
	type slot struct {
		mins  time.Duration
		count int
		focus focusAcc
	}
	slots := map[int]*slot{}
	var total time.Duration
	var overall focusAcc
	for _, s := range in.completed {
		k := slotOf(s.StartedAt)
		if slots[k] == nil {
			slots[k] = &slot{}
		}
		slots[k].mins += s.Duration
		slots[k].count++
		slots[k].focus.add(s)
		overall.add(s)
		total += s.Duration
	}
	eligible := []int{}
	for k, v := range slots {
		if v.count >= 2 {
			eligible = append(eligible, k)
		}
	}
	if len(eligible) < 2 {
		return nil, &domain.InsightGap{Kind: domain.InsightBestTime, Missing: 2,
			Message: i18n.T("Best time of day: all your sessions fall in one time slot; %d sessions at another time are needed to compare", 2)}
	}
	sort.Ints(eligible)
	best := eligible[0]
	for _, k := range eligible {
		if slots[k].mins > slots[best].mins {
			best = k
		}
	}
	b := slots[best]
	share := float64(b.mins) / float64(total)
	lift := share * float64(len(slots))
	evidence := i18n.T("%.0f%% of your focus minutes (%.0f of %.0f) happen there", share*100, minutes(b.mins), minutes(total))
	if b.focus.n >= 2 && overall.n >= 3 {
		evidence += " · " + i18n.T("avg focus %.1f vs %.1f overall", b.focus.avg(), overall.avg())
	}
	return &domain.Insight{
		Kind:     domain.InsightBestTime,
		Message:  i18n.T("Protect %s for your hardest work: it's your strongest focus slot.", slotLabel(best)),
		Evidence: evidence,
		Score:    0.3 + 0.5*clamp01((lift-1)/2),
		Data: map[string]any{
			"slot": slotLabel(best), "slot_minutes": minutes(b.mins), "total_minutes": minutes(total),
			"share": share, "slot_avg_focus": b.focus.avg(), "overall_avg_focus": overall.avg(),
		},
	}, nil
}

// -- best day ----------------------------------------------------------------

func detectBestDay(in *insightInput) (*domain.Insight, *domain.InsightGap) {
	if n := len(in.completed); n < minCompletedForSlots {
		return nil, &domain.InsightGap{Kind: domain.InsightBestDay, Missing: minCompletedForSlots - n,
			Message: i18n.T("Best days: %d more completed sessions needed", minCompletedForSlots-n)}
	}
	mins := map[time.Weekday]time.Duration{}
	focus := map[time.Weekday]*focusAcc{}
	for _, s := range in.completed {
		d := s.StartedAt.Weekday()
		mins[d] += s.Duration
		if focus[d] == nil {
			focus[d] = &focusAcc{}
		}
		focus[d].add(s)
	}
	if len(mins) < 3 {
		return nil, &domain.InsightGap{Kind: domain.InsightBestDay, Missing: 3 - len(mins),
			Message: i18n.T("Best days: sessions on %d more weekdays are needed to compare", 3-len(mins))}
	}
	// Average per calendar occurrence of the weekday in the window (up to
	// today), so a weekday that appears more often is not favored.
	occ := map[time.Weekday]int{}
	last := startOfDay(in.ref)
	for d := in.start; d.Before(in.end) && !d.After(last); d = d.AddDate(0, 0, 1) {
		occ[d.Weekday()]++
	}
	avg := func(d time.Weekday) float64 {
		if occ[d] == 0 {
			return minutes(mins[d])
		}
		return minutes(mins[d]) / float64(occ[d])
	}
	days := make([]time.Weekday, 0, len(mins))
	for d := range mins {
		days = append(days, d)
	}
	sort.Slice(days, func(i, j int) bool { return days[i] < days[j] })
	best := days[0]
	var sumOthers float64
	for _, d := range days {
		if avg(d) > avg(best) {
			best = d
		}
	}
	for _, d := range days {
		if d != best {
			sumOthers += avg(d)
		}
	}
	othersAvg := sumOthers / float64(len(days)-1)
	lift := 1.0
	if othersAvg > 0 {
		lift = avg(best) / othersAvg
	}
	evidence := i18n.T("%.0f focus min per %s vs %.0f on your other active days", avg(best), i18n.FullDayName(best), othersAvg)
	if f := focus[best]; f != nil && f.n >= 2 {
		evidence += " · " + i18n.T("avg focus %.1f", f.avg())
	}
	return &domain.Insight{
		Kind:     domain.InsightBestDay,
		Message:  i18n.T("%s is your best focus day: plan your most demanding work for it.", i18n.FullDayName(best)),
		Evidence: evidence,
		Score:    0.25 + 0.5*clamp01((lift-1)/1.5),
		Data: map[string]any{
			"weekday": best.String(), "avg_minutes": avg(best), "others_avg_minutes": othersAvg, "lift": lift,
		},
	}, nil
}

// -- void patterns -----------------------------------------------------------

type rate struct{ abandoned, total int }

func (r rate) value() float64 {
	if r.total == 0 {
		return 0
	}
	return float64(r.abandoned) / float64(r.total)
}

// maxStatedRatio is the largest "Nx more" a message states; beyond it the
// other side's rate is too small for the multiple to mean much.
const maxStatedRatio = 5.0

// ratio returns how many times a's rate is b's, or false when b has none.
func ratio(a, b rate) (float64, bool) {
	if b.abandoned == 0 {
		return 0, false
	}
	return a.value() / b.value(), true
}

// statedRatio is ratio, but reports false when the multiple is too large
// to state meaningfully, so messages fall back to the qualitative wording.
func statedRatio(a, b rate) (float64, bool) {
	r, ok := ratio(a, b)
	return r, ok && r <= maxStatedRatio
}

func detectAbandonTime(in *insightInput) (*domain.Insight, *domain.InsightGap) {
	if n := len(in.window); n < minEndedForAbandon {
		return nil, &domain.InsightGap{Kind: domain.InsightAbandonTime, Missing: minEndedForAbandon - n,
			Message: i18n.T("Void patterns by time: %d more sessions needed", minEndedForAbandon-n)}
	}
	type split struct {
		hour          int
		worse, better rate
		after         bool
		diff          float64
	}
	var best *split
	for h := 6; h <= 22; h++ {
		var before, after rate
		for _, s := range in.window {
			r := &before
			if s.StartedAt.Hour() >= h {
				r = &after
			}
			r.total++
			if s.Abandoned() {
				r.abandoned++
			}
		}
		if before.total < minSideForAbandonSplit || after.total < minSideForAbandonSplit {
			continue
		}
		cand := split{hour: h, worse: after, better: before, after: true}
		if before.value() > after.value() {
			cand = split{hour: h, worse: before, better: after}
		}
		cand.diff = cand.worse.value() - cand.better.value()
		if cand.worse.abandoned < 2 {
			continue
		}
		if r, ok := ratio(cand.worse, cand.better); ok && r < 1.5 {
			continue
		}
		// On ties, an "after" split moves as late as possible and a "before"
		// split stays as early as possible: both hug the data boundary.
		if best == nil || cand.diff > best.diff || (cand.diff == best.diff && cand.after && best.after) {
			c := cand
			best = &c
		}
	}
	if best == nil {
		return nil, nil
	}
	var msg string
	r, finite := statedRatio(best.worse, best.better)
	switch {
	case best.after && finite:
		msg = i18n.T("After %02d:00 you void %.1fx more sessions: save that time for lighter work.", best.hour, r)
	case best.after:
		msg = i18n.T("After %02d:00 you void sessions you finish earlier: save that time for lighter work.", best.hour)
	case finite:
		msg = i18n.T("Before %02d:00 you void %.1fx more sessions: ease into the day with lighter work.", best.hour, r)
	default:
		msg = i18n.T("Before %02d:00 you void sessions you finish later: ease into the day with lighter work.", best.hour)
	}
	side, other := i18n.T("after %02d:00", best.hour), i18n.T("before")
	if !best.after {
		side, other = i18n.T("before %02d:00", best.hour), i18n.T("after")
	}
	return &domain.Insight{
		Kind:    domain.InsightAbandonTime,
		Message: msg,
		Evidence: i18n.T("%d of %d voided or cancelled %s (%.0f%%) vs %d of %d %s (%.0f%%)",
			best.worse.abandoned, best.worse.total, side, best.worse.value()*100,
			best.better.abandoned, best.better.total, other, best.better.value()*100),
		Score: 0.4 + 0.6*clamp01(best.diff*2),
		Data: map[string]any{
			"split_hour": best.hour, "worse_side": map[bool]string{true: "after", false: "before"}[best.after],
			"worse_abandoned": best.worse.abandoned, "worse_total": best.worse.total,
			"better_abandoned": best.better.abandoned, "better_total": best.better.total,
		},
	}, nil
}

func detectAbandonDay(in *insightInput) (*domain.Insight, *domain.InsightGap) {
	if n := len(in.window); n < minEndedForAbandon {
		return nil, &domain.InsightGap{Kind: domain.InsightAbandonDay, Missing: minEndedForAbandon - n,
			Message: i18n.T("Void patterns by day: %d more sessions needed", minEndedForAbandon-n)}
	}
	byDay := map[time.Weekday]*rate{}
	var all rate
	for _, s := range in.window {
		d := s.StartedAt.Weekday()
		if byDay[d] == nil {
			byDay[d] = &rate{}
		}
		byDay[d].total++
		all.total++
		if s.Abandoned() {
			byDay[d].abandoned++
			all.abandoned++
		}
	}
	var worst time.Weekday = -1
	rated := 0
	for d := time.Sunday; d <= time.Saturday; d++ {
		r := byDay[d]
		if r == nil || r.total < minDaySessionsAbandon {
			continue
		}
		rated++
		if worst < 0 || r.value() > byDay[worst].value() {
			worst = d
		}
	}
	if rated < 2 || byDay[worst].abandoned < 2 {
		return nil, nil
	}
	w := *byDay[worst]
	rest := rate{abandoned: all.abandoned - w.abandoned, total: all.total - w.total}
	diff := w.value() - rest.value()
	if r, ok := ratio(w, rest); ok && r < 1.5 {
		return nil, nil
	}
	r, finite := statedRatio(w, rest)
	day := i18n.FullDayName(worst)
	msg := i18n.T("%s: you void sessions you finish on other days; keep it for shallow work.", day)
	if finite {
		msg = i18n.T("%s: you void %.1fx more sessions than on other days; keep it for shallow work.", day, r)
	}
	return &domain.Insight{
		Kind:    domain.InsightAbandonDay,
		Message: msg,
		Evidence: i18n.T("%d of %d voided or cancelled on %s (%.0f%%) vs %d of %d on other days (%.0f%%)",
			w.abandoned, w.total, day, w.value()*100, rest.abandoned, rest.total, rest.value()*100),
		Score: 0.9 * (0.4 + 0.6*clamp01(diff*2)),
		Data: map[string]any{
			"weekday": worst.String(), "abandoned": w.abandoned, "total": w.total,
			"others_abandoned": rest.abandoned, "others_total": rest.total,
		},
	}, nil
}

// -- tag effort --------------------------------------------------------------

func detectTagEffort(in *insightInput) (*domain.Insight, *domain.InsightGap) {
	type tagAcc struct {
		tasks    map[string]bool
		sessions int
	}
	tags := map[string]*tagAcc{}
	for _, s := range in.completed {
		if s.TaskID == "" || !s.TaskDone {
			continue
		}
		for _, tag := range s.Tags {
			if tags[tag] == nil {
				tags[tag] = &tagAcc{tasks: map[string]bool{}}
			}
			tags[tag].tasks[s.TaskID] = true
			tags[tag].sessions++
		}
	}
	names := make([]string, 0, len(tags))
	for name := range tags {
		names = append(names, name)
	}
	// Most finished tasks first; ties alphabetical, for stable output.
	sort.Slice(names, func(i, j int) bool {
		a, b := len(tags[names[i]].tasks), len(tags[names[j]].tasks)
		if a != b {
			return a > b
		}
		return names[i] < names[j]
	})
	var eligible []string
	for _, name := range names {
		if len(tags[name].tasks) >= minTasksPerTag {
			eligible = append(eligible, name)
		}
	}
	if len(eligible) < 2 {
		missing := 0
		for i := 0; i < 2; i++ {
			have := 0
			if i < len(names) {
				have = len(tags[names[i]].tasks)
			}
			if have < minTasksPerTag {
				missing += minTasksPerTag - have
			}
		}
		return nil, &domain.InsightGap{Kind: domain.InsightTagEffort, Missing: missing,
			Message: i18n.T("Tag effort: finish %d more tagged tasks (with sessions) to compare tags", missing)}
	}
	per := func(name string) float64 { return float64(tags[name].sessions) / float64(len(tags[name].tasks)) }
	most, least := eligible[0], eligible[0]
	for _, name := range eligible {
		if per(name) > per(most) {
			most = name
		}
		if per(name) < per(least) {
			least = name
		}
	}
	r := per(most) / per(least)
	if r < 1.5 {
		return nil, nil
	}
	return &domain.Insight{
		Kind:    domain.InsightTagEffort,
		Message: i18n.T("#%s tasks take %.1fx the sessions of #%s: split them smaller or budget more blocks.", most, r, least),
		Evidence: i18n.T("#%s: %.1f sessions per finished task (%d tasks) · #%s: %.1f (%d tasks)",
			most, per(most), len(tags[most].tasks), least, per(least), len(tags[least].tasks)),
		Score: 0.3 + 0.5*clamp01((r-1)/2),
		Data: map[string]any{
			"most_tag": most, "most_sessions_per_task": per(most), "most_tasks": len(tags[most].tasks),
			"least_tag": least, "least_sessions_per_task": per(least), "least_tasks": len(tags[least].tasks),
		},
	}, nil
}

// -- optimal duration --------------------------------------------------------

var durationBands = []struct {
	max   time.Duration // inclusive upper bound; 0 = unbounded
	label string
}{
	{20 * time.Minute, "≤20 min"},
	{35 * time.Minute, "21–35 min"},
	{60 * time.Minute, "36–60 min"},
	{0, ">60 min"},
}

func bandOf(d time.Duration) int {
	for i, b := range durationBands {
		if b.max == 0 || d <= b.max {
			return i
		}
	}
	return len(durationBands) - 1
}

// plannedDuration estimates what a session was planned for. Completed and
// cancelled sessions keep their planned duration; a voided session stores
// only the time it lasted, so it is attributed to the length the user most
// often finishes that is longer than what it lasted.
func plannedDuration(s domain.InsightSession, finished map[time.Duration]int) time.Duration {
	if s.Status != domain.SessionStatusInterrupted {
		return s.Duration
	}
	var guess time.Duration
	best := 0
	for d, n := range finished {
		if d <= s.Duration {
			continue
		}
		if n > best || (n == best && d < guess) {
			guess, best = d, n
		}
	}
	if best == 0 {
		return s.Duration
	}
	return guess
}

func detectOptimalDuration(in *insightInput) (*domain.Insight, *domain.InsightGap) {
	if n := len(in.window); n < minEndedForDuration {
		return nil, &domain.InsightGap{Kind: domain.InsightOptimalDuration, Missing: minEndedForDuration - n,
			Message: i18n.T("Optimal session length: %d more sessions needed", minEndedForDuration-n)}
	}
	finished := map[time.Duration]int{}
	for _, s := range in.completed {
		finished[s.Duration.Round(time.Minute)]++
	}
	bands := make([]rate, len(durationBands))
	for _, s := range in.window {
		b := bandOf(plannedDuration(s, finished))
		bands[b].total++
		if s.Status != domain.SessionStatusCompleted {
			bands[b].abandoned++
		}
	}
	var rated []int
	for i, b := range bands {
		if b.total >= minBandSessions {
			rated = append(rated, i)
		}
	}
	if len(rated) < 2 {
		// Need a second band with enough sessions: count what the closest
		// runner-up band is missing.
		totals := make([]int, len(bands))
		for i, b := range bands {
			totals[i] = b.total
		}
		sort.Sort(sort.Reverse(sort.IntSlice(totals)))
		missing := minBandSessions - totals[1]
		return nil, &domain.InsightGap{Kind: domain.InsightOptimalDuration, Missing: missing,
			Message: i18n.T("Optimal session length: try %d more sessions of a different length to compare", missing)}
	}
	finishRate := func(i int) float64 { return 1 - bands[i].value() }
	best, worst := rated[0], rated[0]
	for _, i := range rated {
		if finishRate(i) > finishRate(best) || (finishRate(i) == finishRate(best) && bands[i].total > bands[best].total) {
			best = i
		}
		if finishRate(i) < finishRate(worst) {
			worst = i
		}
	}
	gap := finishRate(best) - finishRate(worst)
	if gap < 0.15 {
		return nil, nil
	}
	done := func(i int) int { return bands[i].total - bands[i].abandoned }
	return &domain.Insight{
		Kind:    domain.InsightOptimalDuration,
		Message: i18n.T("Your sweet spot is %s sessions: you finish %.0f%% of them. Plan blocks of that length.", durationBands[best].label, finishRate(best)*100),
		Evidence: i18n.T("%s: %d of %d finished · %s: %d of %d finished",
			durationBands[best].label, done(best), bands[best].total,
			durationBands[worst].label, done(worst), bands[worst].total),
		Score: 0.3 + 0.6*clamp01(gap*2),
		Data: map[string]any{
			"best_band": durationBands[best].label, "best_finished": done(best), "best_total": bands[best].total,
			"worst_band": durationBands[worst].label, "worst_finished": done(worst), "worst_total": bands[worst].total,
		},
	}, nil
}

// -- trend -------------------------------------------------------------------

func detectTrend(in *insightInput) (*domain.Insight, *domain.InsightGap) {
	cur, prev := len(in.completed), len(in.prev)
	if cur < minCompletedForTrend || prev < minCompletedForTrend {
		missing := max(0, minCompletedForTrend-cur) + max(0, minCompletedForTrend-prev)
		return nil, &domain.InsightGap{Kind: domain.InsightTrend, Missing: missing,
			Message: i18n.T("Trend: %d completed sessions in each %d-day period are needed (%d missing)", minCompletedForTrend, in.days, missing)}
	}
	var curMins, prevMins time.Duration
	for _, s := range in.completed {
		curMins += s.Duration
	}
	for _, s := range in.prev {
		prevMins += s.Duration
	}
	change := (minutes(curMins) - minutes(prevMins)) / minutes(prevMins)
	evidence := i18n.T("%s vs %s of completed focus (%d vs %d sessions)", hoursLabel(curMins), hoursLabel(prevMins), cur, prev)
	insight := &domain.Insight{
		Kind:     domain.InsightTrend,
		Evidence: evidence,
		Data: map[string]any{
			"current_minutes": minutes(curMins), "previous_minutes": minutes(prevMins), "change": change,
			"current_sessions": cur, "previous_sessions": prev,
		},
	}
	switch {
	case change >= 0.1:
		insight.Message = i18n.T("Focus time is up %.0f%% vs the previous %d days: keep the rhythm.", change*100, in.days)
		insight.Score = 0.2 + 0.4*clamp01(change)
	case change <= -0.1:
		insight.Message = i18n.T("Focus time is down %.0f%% vs the previous %d days: book your focus blocks now.", -change*100, in.days)
		insight.Score = 0.3 + 0.5*clamp01(-change)
	default:
		insight.Message = i18n.T("Focus time is steady vs the previous %d days.", in.days)
		insight.Score = 0.1
	}
	return insight, nil
}

// -- streak ------------------------------------------------------------------

func detectStreak(in *insightInput) (*domain.Insight, *domain.InsightGap) {
	workDays := map[string]bool{}
	for _, s := range in.history {
		if s.Status == domain.SessionStatusCompleted && s.StartedAt.Before(in.end) {
			workDays[s.StartedAt.Format("2006-01-02")] = true
		}
	}
	streak := computeStreaks(workDays, in.ref)
	workedToday := workDays[in.ref.Format("2006-01-02")]
	data := map[string]any{"current_days": streak.CurrentDays, "longest_days": streak.LongestDays, "worked_today": workedToday}
	evidence := i18n.T("Longest streak: %d days", streak.LongestDays)
	switch {
	case streak.CurrentDays >= minStreakDays && !workedToday:
		return &domain.Insight{
			Kind:     domain.InsightStreak,
			Message:  i18n.T("You're on a %d-day streak: one session today keeps it alive.", streak.CurrentDays),
			Evidence: evidence,
			Score:    0.25 + 0.35*clamp01(float64(streak.CurrentDays)/14),
			Data:     data,
		}, nil
	case streak.CurrentDays >= minStreakDays:
		return &domain.Insight{
			Kind:     domain.InsightStreak,
			Message:  i18n.T("%d-day focus streak and counting.", streak.CurrentDays),
			Evidence: evidence,
			Score:    0.15 + 0.3*clamp01(float64(streak.CurrentDays)/14),
			Data:     data,
		}, nil
	case streak.CurrentDays == 0 && streak.LongestDays >= 5:
		return &domain.Insight{
			Kind:     domain.InsightStreak,
			Message:  i18n.T("Your record is a %d-day streak: one session today starts a new one.", streak.LongestDays),
			Evidence: evidence,
			Score:    0.2,
			Data:     data,
		}, nil
	}
	return nil, nil
}

// -- Deep Work distractions --------------------------------------------------

func detectDistractions(in *insightInput) (*domain.Insight, *domain.InsightGap) {
	var deepSessions, internal, external int
	for _, s := range in.window {
		if s.Methodology != domain.MethodologyDeepWork {
			continue
		}
		deepSessions++
		for _, d := range s.Distractions {
			switch strings.ToLower(d.Category) {
			case "internal":
				internal++
			case "external":
				external++
			}
		}
	}
	if deepSessions == 0 {
		return nil, nil // not a Deep Work user in this window: nothing to nag about
	}
	n := internal + external
	if n < minCategorizedDistracts {
		return nil, &domain.InsightGap{Kind: domain.InsightDistractions, Missing: minCategorizedDistracts - n,
			Message: i18n.T("Deep Work distractions: %d more categorized distractions needed", minCategorizedDistracts-n)}
	}
	share := float64(internal) / float64(n)
	evidence := i18n.T("%d internal · %d external across %d Deep Work sessions", internal, external, deepSessions)
	data := map[string]any{"internal": internal, "external": external, "deep_work_sessions": deepSessions}
	switch {
	case share >= 0.6:
		return &domain.Insight{
			Kind:     domain.InsightDistractions,
			Message:  i18n.T("%.0f%% of your Deep Work distractions are internal: park stray thoughts on a list and keep going.", share*100),
			Evidence: evidence, Score: 0.3 + 0.5*clamp01((share-0.5)*2), Data: data,
		}, nil
	case share <= 0.4:
		return &domain.Insight{
			Kind:     domain.InsightDistractions,
			Message:  i18n.T("%.0f%% of your Deep Work distractions are external: silence notifications before you start.", (1-share)*100),
			Evidence: evidence, Score: 0.3 + 0.5*clamp01((0.5-share)*2), Data: data,
		}, nil
	}
	return nil, nil
}

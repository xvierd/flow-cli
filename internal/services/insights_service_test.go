package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
)

// Reference point for seeded data: Wednesday 2026-11-18, 20:00 local.
// The weekly window is then Thu 2026-11-12 .. Wed 2026-11-18 (offsets 0..6).
var insightRef = time.Date(2026, 11, 18, 20, 0, 0, 0, time.Local)

type seed struct {
	history []domain.InsightSession
	n       int
}

// add appends a work session dayOffset days after the window start (negative
// offsets fall in the previous window), at hour, lasting mins.
func (s *seed) add(dayOffset, hour, mins int, status domain.SessionStatus, opts ...func(*domain.InsightSession)) {
	start, _, _ := InsightWindow(ReportPeriodWeek, insightRef)
	s.n++
	is := domain.InsightSession{
		ID:          "s" + string(rune('A'+s.n%26)) + strings.Repeat("x", s.n/26),
		Status:      status,
		Methodology: domain.MethodologyPomodoro,
		StartedAt:   start.AddDate(0, 0, dayOffset).Add(time.Duration(hour) * time.Hour),
		Duration:    time.Duration(mins) * time.Minute,
	}
	for _, o := range opts {
		o(&is)
	}
	s.history = append(s.history, is)
}

func (s *seed) repeat(times, dayOffset, hour, mins int, status domain.SessionStatus, opts ...func(*domain.InsightSession)) {
	for i := 0; i < times; i++ {
		s.add(dayOffset, hour, mins, status, opts...)
	}
}

func withTask(id string, done bool, tags ...string) func(*domain.InsightSession) {
	return func(is *domain.InsightSession) {
		is.TaskID, is.TaskDone, is.Tags = id, done, tags
	}
}

func withFocus(score int) func(*domain.InsightSession) {
	return func(is *domain.InsightSession) { is.FocusScore = &score }
}

func deepWork(categories ...string) func(*domain.InsightSession) {
	return func(is *domain.InsightSession) {
		is.Methodology = domain.MethodologyDeepWork
		for _, c := range categories {
			is.Distractions = append(is.Distractions, domain.Distraction{Text: "x", Category: c})
		}
	}
}

func (s *seed) analyze() *domain.InsightsReport {
	start, end, _ := InsightWindow(ReportPeriodWeek, insightRef)
	return AnalyzeInsights(ReportPeriodWeek, start, end, insightRef, s.history)
}

func findGap(r *domain.InsightsReport, kind domain.InsightKind) *domain.InsightGap {
	for i := range r.Gaps {
		if r.Gaps[i].Kind == kind {
			return &r.Gaps[i]
		}
	}
	return nil
}

// detect runs a single detector, so tests can inspect its finding even when
// others would outrank it in the capped report.
func (s *seed) detect(d detector) (*domain.Insight, *domain.InsightGap) {
	start, end, _ := InsightWindow(ReportPeriodWeek, insightRef)
	return d(newInsightInput(start, end, insightRef, s.history))
}

const (
	done   = domain.SessionStatusCompleted
	voided = domain.SessionStatusInterrupted
	cancel = domain.SessionStatusCancelled
)

func TestInsightWindow(t *testing.T) {
	start, end, err := InsightWindow(ReportPeriodWeek, insightRef)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 11, 12, 0, 0, 0, 0, time.Local); !start.Equal(want) {
		t.Errorf("week start = %v, want %v", start, want)
	}
	if want := time.Date(2026, 11, 19, 0, 0, 0, 0, time.Local); !end.Equal(want) {
		t.Errorf("week end = %v, want %v", end, want)
	}
	start, _, _ = InsightWindow(ReportPeriodMonth, insightRef)
	if want := time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local); !start.Equal(want) {
		t.Errorf("month start = %v, want %v", start, want)
	}
	if _, _, err := InsightWindow("year", insightRef); err == nil {
		t.Error("expected error for unsupported period")
	}
}

func TestAnalyzeInsights_EmptyHistoryReportsGaps(t *testing.T) {
	var s seed
	r := s.analyze()
	if len(r.Insights) != 0 {
		t.Fatalf("insights = %+v, want none", r.Insights)
	}
	wantMissing := map[domain.InsightKind]int{
		domain.InsightBestTime:        minCompletedForSlots,
		domain.InsightBestDay:         minCompletedForSlots,
		domain.InsightAbandonTime:     minEndedForAbandon,
		domain.InsightAbandonDay:      minEndedForAbandon,
		domain.InsightOptimalDuration: minEndedForDuration,
		domain.InsightTrend:           2 * minCompletedForTrend,
		domain.InsightTagEffort:       2 * minTasksPerTag,
	}
	for kind, want := range wantMissing {
		g := findGap(r, kind)
		if g == nil {
			t.Errorf("missing gap for %s", kind)
			continue
		}
		if g.Missing != want {
			t.Errorf("%s gap missing = %d, want %d (%s)", kind, g.Missing, want, g.Message)
		}
	}
	if g := findGap(r, domain.InsightDistractions); g != nil {
		t.Errorf("no Deep Work sessions: distractions gap should be silent, got %+v", g)
	}
	for i := 1; i < len(r.Gaps); i++ {
		if r.Gaps[i-1].Missing > r.Gaps[i].Missing {
			t.Errorf("gaps not sorted by missing: %+v", r.Gaps)
		}
	}
}

func TestDetectBestTime(t *testing.T) {
	var s seed
	s.repeat(6, 1, 9, 50, done, withFocus(5))
	s.repeat(2, 2, 15, 25, done, withFocus(2))
	in, gap := s.detect(detectBestTime)
	if gap != nil || in == nil {
		t.Fatalf("want insight, got gap %+v", gap)
	}
	if in.Data["slot"] != "09–12h" {
		t.Errorf("slot = %v, want 09–12h", in.Data["slot"])
	}
	if !strings.Contains(in.Message, "09–12h") || !strings.Contains(in.Evidence, "avg focus 5.0") {
		t.Errorf("message/evidence = %q / %q", in.Message, in.Evidence)
	}
}

func TestDetectBestTime_Thresholds(t *testing.T) {
	var s seed
	s.repeat(7, 1, 9, 25, done)
	s.repeat(3, 1, 18, 25, voided) // voided sessions do not count
	_, gap := s.detect(detectBestTime)
	if gap == nil || gap.Missing != 1 {
		t.Fatalf("gap = %+v, want 1 missing", gap)
	}

	var one seed
	one.repeat(8, 1, 9, 25, done)
	_, gap = one.detect(detectBestTime)
	if gap == nil || gap.Missing != 2 || !strings.Contains(gap.Message, "one time slot") {
		t.Fatalf("single-slot gap = %+v", gap)
	}
}

func TestDetectBestDay(t *testing.T) {
	var s seed
	s.repeat(5, 4, 9, 50, done) // Monday 2026-11-16
	s.repeat(2, 0, 9, 25, done) // Thursday
	s.repeat(2, 1, 9, 25, done) // Friday
	in, gap := s.detect(detectBestDay)
	if gap != nil || in == nil {
		t.Fatalf("want insight, got gap %+v", gap)
	}
	if in.Data["weekday"] != "Monday" || !strings.HasPrefix(in.Message, "Monday") {
		t.Errorf("best day = %v (%q), want Monday", in.Data["weekday"], in.Message)
	}

	var two seed
	two.repeat(4, 4, 9, 25, done)
	two.repeat(4, 5, 9, 25, done)
	_, gap = two.detect(detectBestDay)
	if gap == nil || gap.Missing != 1 {
		t.Fatalf("two-weekday gap = %+v, want 1 more weekday", gap)
	}
}

func TestDetectAbandonTime(t *testing.T) {
	var s seed
	s.repeat(3, 1, 9, 25, done)
	s.repeat(3, 2, 10, 25, done)
	s.repeat(2, 1, 17, 25, done)
	s.repeat(3, 2, 17, 12, voided)
	s.add(3, 18, 25, cancel)
	in, gap := s.detect(detectAbandonTime)
	if gap != nil || in == nil {
		t.Fatalf("want insight, got gap %+v", gap)
	}
	if in.Data["split_hour"] != 17 || in.Data["worse_side"] != "after" {
		t.Errorf("split = %v %v, want after 17", in.Data["worse_side"], in.Data["split_hour"])
	}
	if !strings.HasPrefix(in.Message, "After 17:00") {
		t.Errorf("message = %q", in.Message)
	}
	if !strings.Contains(in.Evidence, "4 of 6 voided or cancelled after 17:00") {
		t.Errorf("evidence = %q", in.Evidence)
	}

	// One void before the split: 67% vs 12% is over maxStatedRatio, so the
	// message stays qualitative instead of claiming "5.3x".
	s.add(3, 9, 10, voided)
	s.add(3, 9, 25, done)
	in, _ = s.detect(detectAbandonTime)
	if in == nil || strings.Contains(in.Message, "x more") {
		t.Fatalf("want qualitative message, got %+v", in)
	}

	// More voids before the split: 67% vs 20% is stated as a ratio.
	s.add(3, 10, 10, voided)
	s.add(3, 10, 25, done)
	in, _ = s.detect(detectAbandonTime)
	if in == nil || !strings.Contains(in.Message, "you void 3.3x more sessions") {
		t.Fatalf("want ratio message, got %+v", in)
	}
}

func TestDetectAbandonTime_Thresholds(t *testing.T) {
	var s seed
	s.repeat(9, 1, 17, 10, voided)
	_, gap := s.detect(detectAbandonTime)
	if gap == nil || gap.Missing != 1 {
		t.Fatalf("gap = %+v, want 1 missing", gap)
	}
	// Enough sessions but voids spread evenly: no finding, no gap.
	var even seed
	for i := 0; i < 6; i++ {
		even.add(1, 9, 25, done)
		even.add(1, 18, 25, done)
	}
	even.add(1, 9, 5, voided)
	even.add(1, 18, 5, voided)
	if in, gap := even.detect(detectAbandonTime); in != nil || gap != nil {
		t.Errorf("even voids: got %+v / %+v, want nothing", in, gap)
	}
}

func TestDetectAbandonDay(t *testing.T) {
	var s seed
	s.repeat(3, 1, 10, 25, voided) // Friday
	s.add(1, 11, 25, done)
	s.repeat(4, 4, 10, 25, done) // Monday
	s.repeat(4, 5, 10, 25, done) // Tuesday
	s.add(5, 15, 25, voided)
	s.add(5, 16, 25, voided)
	s.add(4, 16, 25, voided)
	in, gap := s.detect(detectAbandonDay)
	if gap != nil || in == nil {
		t.Fatalf("want insight, got gap %+v", gap)
	}
	if in.Data["weekday"] != "Friday" || !strings.HasPrefix(in.Message, "Friday: you void") {
		t.Errorf("worst day = %v (%q)", in.Data["weekday"], in.Message)
	}
	if !strings.Contains(in.Message, "x more sessions") {
		t.Errorf("want ratio in message: %q", in.Message)
	}
}

func TestDetectTagEffort(t *testing.T) {
	var s seed
	s.repeat(4, 1, 9, 25, done, withTask("A", true, "backend"))
	s.repeat(4, 2, 9, 25, done, withTask("B", true, "backend"))
	s.add(3, 9, 25, done, withTask("C", true, "docs"))
	s.add(3, 10, 25, done, withTask("D", true, "docs"))
	s.repeat(5, 3, 11, 25, done, withTask("E", false, "docs")) // unfinished: ignored
	in, gap := s.detect(detectTagEffort)
	if gap != nil || in == nil {
		t.Fatalf("want insight, got gap %+v", gap)
	}
	if in.Data["most_tag"] != "backend" || in.Data["least_tag"] != "docs" {
		t.Errorf("tags = %v / %v", in.Data["most_tag"], in.Data["least_tag"])
	}
	if !strings.HasPrefix(in.Message, "#backend tasks take 4.0x the sessions of #docs") {
		t.Errorf("message = %q", in.Message)
	}
}

func TestDetectTagEffort_Thresholds(t *testing.T) {
	var s seed
	s.repeat(3, 1, 9, 25, done, withTask("A", true, "backend"))
	s.repeat(3, 1, 10, 25, done, withTask("B", true, "backend"))
	s.add(2, 9, 25, done, withTask("C", true, "docs"))
	_, gap := s.detect(detectTagEffort)
	if gap == nil || gap.Missing != 1 {
		t.Fatalf("gap = %+v, want 1 more #docs task", gap)
	}
}

func TestDetectOptimalDuration(t *testing.T) {
	var s seed
	s.repeat(9, 1, 9, 25, done)
	s.add(1, 15, 25, cancel)
	s.repeat(2, 2, 9, 90, done)
	// Voided 90-minute blocks only store the 30 minutes they lasted; they
	// must count against the 90-minute band, not the 25-minute one.
	s.repeat(3, 3, 9, 30, voided)
	in, gap := s.detect(detectOptimalDuration)
	if gap != nil || in == nil {
		t.Fatalf("want insight, got gap %+v", gap)
	}
	if in.Data["best_band"] != "21–35 min" || in.Data["best_finished"] != 9 || in.Data["best_total"] != 10 {
		t.Errorf("best = %v %v/%v", in.Data["best_band"], in.Data["best_finished"], in.Data["best_total"])
	}
	if in.Data["worst_band"] != ">60 min" || in.Data["worst_finished"] != 2 || in.Data["worst_total"] != 5 {
		t.Errorf("worst = %v %v/%v", in.Data["worst_band"], in.Data["worst_finished"], in.Data["worst_total"])
	}
}

func TestDetectOptimalDuration_Thresholds(t *testing.T) {
	var s seed
	s.repeat(9, 1, 9, 25, done)
	_, gap := s.detect(detectOptimalDuration)
	if gap == nil || gap.Missing != 1 {
		t.Fatalf("gap = %+v, want 1 missing", gap)
	}
	var one seed
	one.repeat(10, 1, 9, 25, done)
	one.add(1, 9, 50, done)
	_, gap = one.detect(detectOptimalDuration)
	if gap == nil || gap.Missing != 2 {
		t.Fatalf("single-band gap = %+v, want 2 more sessions at another length", gap)
	}
}

func TestDetectTrend(t *testing.T) {
	var up seed
	up.repeat(4, -3, 9, 25, done)
	up.repeat(8, 2, 9, 25, done)
	in, gap := up.detect(detectTrend)
	if gap != nil || in == nil {
		t.Fatalf("want insight, got gap %+v", gap)
	}
	if !strings.HasPrefix(in.Message, "Focus time is up 100% vs the previous 7 days") {
		t.Errorf("message = %q", in.Message)
	}

	var down seed
	down.repeat(8, -3, 9, 25, done)
	down.repeat(4, 2, 9, 25, done)
	in, _ = down.detect(detectTrend)
	if in == nil || !strings.HasPrefix(in.Message, "Focus time is down 50%") {
		t.Fatalf("down = %+v", in)
	}

	var thin seed
	thin.repeat(2, -3, 9, 25, done)
	thin.repeat(5, 2, 9, 25, done)
	_, gap = thin.detect(detectTrend)
	if gap == nil || gap.Missing != 1 {
		t.Fatalf("gap = %+v, want 1 missing in previous period", gap)
	}
}

func TestDetectStreak(t *testing.T) {
	var s seed
	for d := 2; d <= 6; d++ { // Sat..Wed, today included
		s.add(d, 9, 25, done)
	}
	in, _ := s.detect(detectStreak)
	if in == nil || !strings.HasPrefix(in.Message, "5-day focus streak") {
		t.Fatalf("streak = %+v", in)
	}

	var pending seed
	for d := 2; d <= 5; d++ { // not today yet
		pending.add(d, 9, 25, done)
	}
	in, _ = pending.detect(detectStreak)
	if in == nil || !strings.Contains(in.Message, "one session today keeps it alive") {
		t.Fatalf("pending streak = %+v", in)
	}

	var short seed
	short.add(6, 9, 25, done)
	if in, gap := short.detect(detectStreak); in != nil || gap != nil {
		t.Errorf("1-day streak: got %+v / %+v, want nothing", in, gap)
	}
}

func TestDetectDistractions(t *testing.T) {
	var s seed
	s.add(1, 9, 90, done, deepWork("internal", "internal"))
	s.add(2, 9, 90, done, deepWork("internal", "external"))
	s.add(3, 9, 90, done, deepWork("internal", ""))
	in, gap := s.detect(detectDistractions)
	if gap != nil || in == nil {
		t.Fatalf("want insight, got gap %+v", gap)
	}
	if !strings.HasPrefix(in.Message, "80% of your Deep Work distractions are internal") {
		t.Errorf("message = %q", in.Message)
	}

	var ext seed
	ext.add(1, 9, 90, done, deepWork("external", "external", "external", "external", "internal"))
	in, _ = ext.detect(detectDistractions)
	if in == nil || !strings.Contains(in.Message, "external") {
		t.Fatalf("external = %+v", in)
	}

	var thin seed
	thin.add(1, 9, 90, done, deepWork("internal", "external"))
	_, gap = thin.detect(detectDistractions)
	if gap == nil || gap.Missing != 3 {
		t.Fatalf("gap = %+v, want 3 missing", gap)
	}
}

func TestAnalyzeInsights_RanksAndCaps(t *testing.T) {
	var s seed
	// Enough history for nearly every detector at once.
	s.repeat(4, -3, 9, 25, done)
	for d := 0; d <= 6; d++ {
		s.repeat(2, d, 9, 25, done, withTask("A", true, "backend"), withFocus(5))
	}
	s.repeat(3, 4, 10, 50, done, withTask("B", true, "backend"))
	s.add(1, 11, 25, done, withTask("C", true, "docs"))
	s.add(2, 11, 25, done, withTask("D", true, "docs"))
	s.repeat(4, 3, 18, 20, voided)
	s.add(5, 9, 90, done, deepWork("internal", "internal", "internal", "internal", "external"))

	r := s.analyze()
	if len(r.Insights) < 3 || len(r.Insights) > MaxInsights {
		t.Fatalf("got %d insights, want 3..%d: %+v", len(r.Insights), MaxInsights, r.Insights)
	}
	for i := 1; i < len(r.Insights); i++ {
		if r.Insights[i-1].Score < r.Insights[i].Score {
			t.Errorf("insights not sorted by score: %+v", r.Insights)
		}
	}
	if r.Top() != &r.Insights[0] {
		t.Error("Top() should return the first insight")
	}
	if r.SessionsAnalyzed == 0 {
		t.Error("SessionsAnalyzed should count window sessions")
	}
	for _, in := range r.Insights {
		if in.Message == "" || in.Evidence == "" {
			t.Errorf("insight %s without message or evidence: %+v", in.Kind, in)
		}
	}
}

func TestAnalyzeInsights_Spanish(t *testing.T) {
	i18n.SetLanguage("es")
	defer i18n.SetLanguage("en")
	var s seed
	s.repeat(6, 1, 9, 50, done)
	s.repeat(2, 2, 15, 25, done)
	in, _ := s.detect(detectBestTime)
	if in == nil || !strings.Contains(in.Message, "Protege") {
		t.Fatalf("spanish best time = %+v", in)
	}
	r := (&seed{}).analyze()
	for _, g := range r.Gaps {
		if strings.Contains(g.Message, "%!") || strings.Contains(g.Message, "needed") {
			t.Errorf("gap not translated cleanly: %q", g.Message)
		}
	}
}

func TestInsightsService_FromStorage(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	task, _ := domain.NewTask("Write API")
	task.Tags = []string{"backend"}
	if err := store.Tasks().Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	start, _, _ := InsightWindow(ReportPeriodWeek, insightRef)
	save := func(id string, typ domain.SessionType, status domain.SessionStatus, at time.Time) {
		taskID := task.ID
		s := &domain.PomodoroSession{
			ID: id, TaskID: &taskID, Type: typ, Status: status,
			Duration: 25 * time.Minute, StartedAt: at, Methodology: domain.MethodologyPomodoro,
		}
		if err := store.Sessions().Save(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 8; i++ {
		save("w"+string(rune('a'+i)), domain.SessionTypeWork, domain.SessionStatusCompleted, start.AddDate(0, 0, i%3).Add(9*time.Hour))
	}
	save("v1", domain.SessionTypeWork, domain.SessionStatusInterrupted, start.Add(18*time.Hour))
	save("b1", domain.SessionTypeShortBreak, domain.SessionStatusCompleted, start.Add(10*time.Hour))
	save("old", domain.SessionTypeWork, domain.SessionStatusCompleted, start.AddDate(0, 0, -30))

	r, err := NewInsightsService(store).GetInsightsAt(ctx, ReportPeriodWeek, insightRef)
	if err != nil {
		t.Fatal(err)
	}
	if r.SessionsAnalyzed != 9 {
		t.Errorf("SessionsAnalyzed = %d, want 9 (8 completed + 1 voided, no breaks, nothing outside the window)", r.SessionsAnalyzed)
	}
	if g := findGap(r, domain.InsightBestTime); g == nil || !strings.Contains(g.Message, "one time slot") {
		t.Errorf("all work at 09h local: want single-slot gap, got %+v", g)
	}
}

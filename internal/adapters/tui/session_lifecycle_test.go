package tui

// Regression tests for the session lifecycle: tick chains, state refresh under
// overlays, void, auto-break gating, worked time, highlight detection,
// per-session reset and inline task reuse.

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/methodology"
	"github.com/xvierd/flow-cli/internal/ports"
)

func deepWorkMode() methodology.Mode {
	return methodology.ForMethodology(domain.MethodologyDeepWork, nil)
}
func makeTimeMode() methodology.Mode {
	return methodology.ForMethodology(domain.MethodologyMakeTime, nil)
}

func sizedModel(st *domain.CurrentState) Model {
	m := NewModel(st, nil, nil)
	m.width, m.height = 120, 40
	return m
}

func send(t *testing.T, m tea.Model, msgs ...tea.Msg) (tea.Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, msg := range msgs {
		m, cmd = m.Update(msg)
	}
	return m, cmd
}

// --- T1/T14: tick chain survives overlays, exactly one live chain ---------

func TestModel_TickAndStateProcessedUnderDistractionOverlay(t *testing.T) {
	m := sizedModel(stateWithSession())
	m.mode = deepWorkMode()
	m.fetchState = stateWithSession
	r, _ := send(t, m, key("d"))
	if !r.(Model).distractionMode {
		t.Fatal("expected distraction overlay")
	}
	r, cmd := send(t, r, tickMsg{gen: r.(Model).tickGen})
	if cmd == nil {
		t.Fatal("current-generation tick must re-arm while the overlay is open")
	}
	if !r.(Model).distractionMode {
		t.Fatal("tick must not close the overlay")
	}
	// Closing the overlay must not start a second chain.
	_, cmd = send(t, r, key("esc"))
	if cmd != nil {
		t.Fatal("closing the distraction overlay must not return a tick")
	}
}

func TestModel_CompletionDetectedUnderOverlayClosesDistractionInput(t *testing.T) {
	m := sizedModel(stateWithSession())
	m.mode = deepWorkMode()
	r, _ := send(t, m, key("d"), stateMsg{state: stateNoSession()})
	got := r.(Model)
	if !got.completed {
		t.Fatal("stateMsg under the overlay must still detect completion")
	}
	if got.distractionMode {
		t.Fatal("distraction input must close when the session ends")
	}
}

func TestModel_StateRefreshDuringRitualShowsNewBreak(t *testing.T) {
	cb, _ := commandTracker()
	m := sizedModel(stateWithSession())
	m.mode = deepWorkMode()
	m.commandCallback = cb
	r, _ := send(t, m, stateMsg{state: stateNoSession()}, key("a"))
	if !r.(Model).shutdownRitualMode {
		t.Fatal("expected ritual")
	}
	if _, cmd := send(t, r, tickMsg{gen: r.(Model).tickGen}); cmd == nil {
		t.Fatal("tick must keep re-arming during the ritual")
	}
	r, _ = send(t, r, key("enter"), key("enter"), key("enter"), key("enter"), key("b"))
	r, _ = send(t, r, stateMsg{state: breakSession()})
	v := r.(Model).View()
	if !strings.Contains(v, "[s]kip") {
		t.Errorf("break view expected after ritual + [b], got:\n%s", v)
	}
}

func TestModel_StaleTickDropped(t *testing.T) {
	m := sizedModel(stateWithSession())
	m.tickGen = 2
	m.energizeTicks = 5
	r, cmd := send(t, m, tickMsg{gen: 1})
	if cmd != nil {
		t.Fatal("stale tick must not re-arm")
	}
	if r.(Model).energizeTicks != 5 {
		t.Fatal("stale tick must not change state")
	}
}

func TestModel_SummaryRestartsSingleChain(t *testing.T) {
	st := stateWithSession()
	st.TodayStats.WorkSessions = 2
	m := sizedModel(st)
	r, cmd := send(t, m, key("c"))
	got := r.(Model)
	if cmd == nil || !got.showingSummary {
		t.Fatal("expected summary with a tick")
	}
	if got.tickGen == m.tickGen {
		t.Fatal("summary must retire the previous chain")
	}
	// The pending tick from the original chain is dropped.
	if _, c := send(t, got, tickMsg{gen: m.tickGen}); c != nil {
		t.Fatal("old chain must die")
	}
	r, _ = send(t, got, tickMsg{gen: got.tickGen})
	if r.(Model).summaryTicks != 2 {
		t.Fatalf("summary should count down once per tick, got %d", r.(Model).summaryTicks)
	}
}

func TestInlineModel_TickProcessedUnderOverlay_NoDuplicateOnExit(t *testing.T) {
	m := baseInlineModel()
	m.mode = deepWorkMode()
	r, _ := send(t, m, key("d"))
	if _, cmd := send(t, r, tickMsg{gen: r.(InlineModel).tickGen}); cmd == nil {
		t.Fatal("tick must re-arm under the inline distraction overlay")
	}
	if _, cmd := send(t, r, key("esc")); cmd != nil {
		t.Fatal("esc must not start an extra tick chain")
	}
}

func TestInlineModel_StartSessionRetiresOldChain(t *testing.T) {
	m := baseInlineModel()
	m.onStartSession = func(int, string, string) error { return nil }
	oldGen := m.tickGen
	r, cmd := m.startSession("", "")
	got := r.(InlineModel)
	if cmd == nil || got.tickGen == oldGen {
		t.Fatal("startSession must start a fresh chain")
	}
	if _, c := send(t, got, tickMsg{gen: oldGen}); c != nil {
		t.Fatal("in-flight tick from the previous chain must be dropped")
	}
}

// --- T2: void is not a completion ------------------------------------------

func TestVoid_ReturnsToIdleWithoutCompletion(t *testing.T) {
	for _, inline := range []bool{false, true} {
		cb, cmds := commandTracker()
		notified := 0
		onDone := func(domain.SessionType, time.Duration) { notified++ }
		var tm tea.Model
		if inline {
			m := baseInlineModel()
			m.commandCallback, m.autoBreak, m.mode, m.onSessionComplete = cb, true, makeTimeMode(), onDone
			tm = m
		} else {
			m := sizedModel(stateWithSession())
			m.commandCallback, m.autoBreak, m.mode, m.onSessionComplete = cb, true, makeTimeMode(), onDone
			tm = m
		}
		tm, _ = send(t, tm, key("v"), stateMsg{state: stateNoSession()})
		gen := 0
		completed := false
		switch x := tm.(type) {
		case Model:
			gen, completed = x.tickGen, x.completed
		case InlineModel:
			gen, completed = x.tickGen, x.completed
		}
		for i := 0; i < 4; i++ {
			tm, _ = send(t, tm, tickMsg{gen: gen})
		}
		if completed || notified != 0 {
			t.Errorf("inline=%v: void treated as completion (completed=%v notified=%d)", inline, completed, notified)
		}
		for _, c := range *cmds {
			if c == ports.CmdBreak {
				t.Errorf("inline=%v: void must not auto-start a break", inline)
			}
		}
		if v := tm.View(); strings.Contains(v, "focused") || !strings.Contains(v, "[s]tart") {
			t.Errorf("inline=%v: expected idle view, got:\n%s", inline, v)
		}
	}
}

// --- T6: auto-break only after prompts, with a visible countdown ----------

func TestAutoBreak_WaitsForMakeTimePrompts(t *testing.T) {
	for _, inline := range []bool{false, true} {
		cb, cmds := commandTracker()
		var tm tea.Model
		if inline {
			m := baseInlineModel()
			m.commandCallback, m.autoBreak, m.mode = cb, true, makeTimeMode()
			tm = m
		} else {
			m := sizedModel(stateWithSession())
			m.commandCallback, m.autoBreak, m.mode = cb, true, makeTimeMode()
			tm = m
		}
		ticks := func(tm tea.Model) int {
			if x, ok := tm.(Model); ok {
				return x.autoBreakTicks
			}
			return tm.(InlineModel).autoBreakTicks
		}
		tm, _ = send(t, tm, stateMsg{state: stateNoSession()})
		if ticks(tm) != 0 {
			t.Fatalf("inline=%v: auto-break armed before the focus score", inline)
		}
		tm, _ = send(t, tm, key("3"), key("n"))
		if ticks(tm) != 3 {
			t.Fatalf("inline=%v: auto-break should arm once prompts are done, ticks=%d", inline, ticks(tm))
		}
		if v := tm.View(); !strings.Contains(v, "Break starting in 3s") {
			t.Errorf("inline=%v: countdown must be visible, got:\n%s", inline, v)
		}
		gen := 0
		if x, ok := tm.(Model); ok {
			gen = x.tickGen
		} else {
			gen = tm.(InlineModel).tickGen
		}
		for i := 0; i < 3; i++ {
			tm, _ = send(t, tm, tickMsg{gen: gen})
		}
		if len(*cmds) != 1 || (*cmds)[0] != ports.CmdBreak {
			t.Errorf("inline=%v: expected exactly one CmdBreak, got %v", inline, *cmds)
		}
	}
}

func TestAutoBreak_CancelledCountdownNotRearmed(t *testing.T) {
	cb, cmds := commandTracker()
	m := sizedModel(stateWithSession())
	m.commandCallback, m.autoBreak, m.mode = cb, true, methodology.ForMethodology(domain.MethodologyPomodoro, nil)
	r, _ := send(t, m, stateMsg{state: stateNoSession()})
	if r.(Model).autoBreakTicks != 3 {
		t.Fatal("Pomodoro has no prompts: auto-break arms at completion")
	}
	r, _ = send(t, r, key("x"))
	for i := 0; i < 4; i++ {
		r, _ = send(t, r, tickMsg{gen: r.(Model).tickGen})
	}
	if r.(Model).autoBreakTicks != 0 || len(*cmds) != 0 {
		t.Errorf("cancelled countdown re-armed: ticks=%d cmds=%v", r.(Model).autoBreakTicks, *cmds)
	}
}

// --- T7: real worked time ---------------------------------------------------

func TestFinishEarly_ShowsActualWorkedTime(t *testing.T) {
	s := activeSession()
	s.StartedAt = time.Now().Add(-10 * time.Minute)
	cb, _ := commandTracker()
	m := sizedModel(&domain.CurrentState{ActiveSession: s})
	m.commandCallback = cb
	r, _ := send(t, m, key("f"), key("f"), stateMsg{state: stateNoSession()})
	if v := r.(Model).View(); !strings.Contains(v, "10:00 worked") {
		t.Errorf("fullscreen should show 10:00 worked, got:\n%s", v)
	}

	s2 := activeSession()
	s2.StartedAt = time.Now().Add(-10 * time.Minute)
	im := NewInlineModel(&domain.CurrentState{ActiveSession: s2}, nil, nil)
	im.phase = phaseTimer
	im.commandCallback = cb
	var worked time.Duration
	im.onSessionComplete = func(_ domain.SessionType, w time.Duration) { worked = w }
	ri, _ := send(t, im, key("f"), key("f"), stateMsg{state: stateNoSession()})
	if v := ri.View(); !strings.Contains(v, "10m worked") {
		t.Errorf("inline should show 10m worked, got %q", v)
	}
	if worked < 10*time.Minute || worked > 11*time.Minute {
		t.Errorf("notification should get the worked time, got %v", worked)
	}
}

func TestWorkedTime_PausedSessionAndCap(t *testing.T) {
	var cs completionState
	s := activeSession()
	s.StartedAt = time.Now().Add(-40 * time.Minute) // overran the 25m plan
	if got := cs.captureCompletion(s); got != s.Duration {
		t.Errorf("worked time must be capped at the planned duration, got %v", got)
	}
	p := activeSession()
	p.StartedAt = time.Now().Add(-20 * time.Minute)
	pausedAt := p.StartedAt.Add(5 * time.Minute)
	p.PausedAt, p.Status = &pausedAt, domain.SessionStatusPaused
	if got := cs.captureCompletion(p); got != 5*time.Minute {
		t.Errorf("paused time is not work: got %v, want 5m", got)
	}
}

// --- T10: highlight line ----------------------------------------------------

func TestMakeTime_HighlightLineShownForHighlightSession(t *testing.T) {
	today := time.Now()
	task := &domain.Task{ID: "t1", Title: "Write", HighlightDate: &today}
	newState := func() *domain.CurrentState {
		s := activeSession()
		s.TaskID = &task.ID
		return &domain.CurrentState{ActiveSession: s, ActiveTask: task, TodayHighlight: task}
	}
	m := sizedModel(newState())
	m.mode = makeTimeMode()
	r, _ := send(t, m, stateMsg{state: &domain.CurrentState{TodayHighlight: task}})
	if v := r.View(); !strings.Contains(v, "You made time for your Highlight") {
		t.Errorf("fullscreen highlight line missing:\n%s", v)
	}
	im := NewInlineModel(newState(), nil, nil)
	im.phase, im.mode = phaseTimer, makeTimeMode()
	ri, _ := send(t, im, stateMsg{state: &domain.CurrentState{TodayHighlight: task}})
	if v := ri.View(); !strings.Contains(v, "You made time for your Highlight") {
		t.Errorf("inline highlight line missing:\n%s", v)
	}
	// A different task than today's Highlight does not get the line.
	other := &domain.Task{ID: "t2", Title: "Other"}
	ri, _ = send(t, im, stateMsg{state: &domain.CurrentState{TodayHighlight: other}})
	if strings.Contains(ri.View(), "You made time for your Highlight") {
		t.Error("highlight line shown for a non-highlight session")
	}
}

// --- T11: status rendering --------------------------------------------------

func TestRenderStatus_NotLoading(t *testing.T) {
	out := renderStatus(stateWithSession(), nil, 80)
	if strings.Contains(out, "Loading") || !strings.Contains(out, "Status:") {
		t.Errorf("status should render the session, got:\n%s", out)
	}
}

// --- T12: per-session state does not leak -----------------------------------

func TestModel_DistractionsResetAfterStopBreakSkip(t *testing.T) {
	cb, _ := commandTracker()
	m := sizedModel(stateWithSession())
	m.mode = deepWorkMode()
	m.commandCallback = cb
	r, _ := send(t, m, key("d"), key("x"), key("enter"), key("enter"))
	if len(r.(Model).distractions) != 1 {
		t.Fatal("expected one distraction")
	}
	r, _ = send(t, r, key("b"), key("b"), stateMsg{state: breakSession()}, key("s"), stateMsg{state: stateWithSession()})
	if d := r.(Model).distractions; len(d) != 0 {
		t.Errorf("distractions leaked into the next session: %v", d)
	}
}

func TestInlineModel_EnergizeReminderRearmsInChainedSession(t *testing.T) {
	cb, _ := commandTracker()
	half := func() *domain.CurrentState {
		s := activeSession()
		s.StartedAt = time.Now().Add(-15 * time.Minute)
		return &domain.CurrentState{ActiveSession: s}
	}
	m := NewInlineModel(half(), nil, nil)
	m.phase, m.mode, m.commandCallback = phaseTimer, makeTimeMode(), cb
	m.onStartSession = func(int, string, string) error { return nil }
	r, _ := send(t, m, tickMsg{gen: m.tickGen})
	if !r.(InlineModel).energizeShown {
		t.Fatal("reminder expected in first session")
	}
	r, _ = send(t, r, stateMsg{state: stateNoSession()}, key("3"), key("n"), key("n"))
	r, _ = send(t, r, key("enter"), key("enter"), key("enter")) // duration, checklist, task
	if r.(InlineModel).phase != phaseTimer {
		t.Fatalf("expected timer phase, got %v", r.(InlineModel).phase)
	}
	r, _ = send(t, r, stateMsg{state: half()})
	r, _ = send(t, r, tickMsg{gen: r.(InlineModel).tickGen})
	if r.(InlineModel).energizeTicks == 0 {
		t.Error("energize reminder must fire again in the chained session")
	}
}

// --- T3: inline reuses picked tasks ----------------------------------------

func TestInlineModel_RecentAndCarryOverTasksReuseID(t *testing.T) {
	yesterday := time.Now().AddDate(0, 0, -1)
	carry := &domain.Task{ID: "hl", Title: "Old highlight", HighlightDate: &yesterday}
	recent := &domain.Task{ID: "r1", Title: "Recent"}
	newModel := func() (InlineModel, *[]string) {
		var got []string
		m := NewInlineModel(stateNoSession(), nil, nil)
		m.mode = methodology.ForMethodology(domain.MethodologyPomodoro, nil)
		m.presets = m.mode.Presets()
		m.phase = phasePickDuration
		m.fetchRecentTasks = func(int) []*domain.Task { return []*domain.Task{recent} }
		m.onStartSessionForTask = func(_ int, id, name, _ string) error {
			got = append(got, id+"|"+name)
			return nil
		}
		return m, &got
	}

	m, got := newModel()
	send(t, m, key("enter"), key("1"))
	if len(*got) != 1 || (*got)[0] != "r1|Recent" {
		t.Errorf("recent task should be reused by ID, got %v", *got)
	}

	m, got = newModel()
	m.mode = makeTimeMode()
	m.presets = m.mode.Presets()
	m.fetchYesterdayHighlight = func() *domain.Task { return carry }
	send(t, m, key("enter"), key("enter"), key("enter")) // duration, checklist, carry-over
	if len(*got) != 1 || (*got)[0] != "hl|Old highlight" {
		t.Errorf("carry-over should reuse the existing task, got %v", *got)
	}
	if !carry.HighlightDate.Equal(yesterday) {
		t.Error("carry-over must not mutate the task in memory; persistence belongs to the callback")
	}

	// Deep Work: picking a task, going back and retitling it creates a new task.
	m, got = newModel()
	m.mode = deepWorkMode()
	m.presets = m.mode.Presets()
	r, _ := send(t, m, key("enter"), key("1"), key("esc"), key("!"), key("enter"), key("enter"))
	_ = r
	if len(*got) != 1 || (*got)[0] != "|Recent!" {
		t.Errorf("retitled task must not reuse the picked ID, got %v", *got)
	}
}

// --- T8: completion/break info recomputed ----------------------------------

func TestCompletionInfoRefreshedAtCompletion(t *testing.T) {
	calls := 0
	fetch := func(st *domain.CurrentState) *domain.CompletionInfo {
		calls++
		return &domain.CompletionInfo{NextBreakType: domain.SessionTypeLongBreak, NextBreakDuration: 15 * time.Minute, SessionsBeforeLong: 4}
	}
	stale := &domain.CompletionInfo{NextBreakType: domain.SessionTypeShortBreak, NextBreakDuration: 5 * time.Minute, SessionsBeforeLong: 4, SessionsUntilLong: 3}
	m := NewModel(stateWithSession(), stale, nil)
	m.width, m.height = 120, 40
	m.mode = methodology.ForMethodology(domain.MethodologyPomodoro, nil)
	m.fetchCompletionInfo = fetch
	r, _ := send(t, m, stateMsg{state: stateNoSession()})
	if calls != 1 || !strings.Contains(r.View(), "15:00") {
		t.Errorf("completion info should be recomputed (calls=%d):\n%s", calls, r.View())
	}
	im := NewInlineModel(stateWithSession(), stale, nil)
	im.phase, im.mode, im.fetchCompletionInfo = phaseTimer, m.mode, fetch
	ri, _ := send(t, im, stateMsg{state: stateNoSession()})
	if calls != 2 || !strings.Contains(ri.View(), "15:00 Long Break") {
		t.Errorf("inline completion info should be recomputed:\n%s", ri.View())
	}
}

func TestInlineModel_BreakInfoRefreshedOnModeSelect(t *testing.T) {
	m := NewInlineModel(stateNoSession(), nil, nil)
	m.phase = phasePickMode
	m.breakInfo = "stale"
	m.fetchBreakInfo = func() string { return "fresh" }
	r, _ := m.selectMode()
	if r.(InlineModel).breakInfo != "fresh" {
		t.Error("break info must follow the selected mode")
	}
}

func TestTimerConfigure_LegacyOnSessionCompleteStillCalled(t *testing.T) {
	tm := NewInlineTimer(nil)
	var got domain.SessionType
	tm.Configure(TimerConfig{OnSessionComplete: func(st domain.SessionType) { got = st }})
	tm.onSessionComplete(domain.SessionTypeWork, time.Minute)
	if got != domain.SessionTypeWork {
		t.Error("legacy OnSessionComplete must still be invoked")
	}
}

// --- UX ---------------------------------------------------------------------

func TestOutcomeReview_EscSkips(t *testing.T) {
	s := activeSession()
	s.IntendedOutcome = "ship"
	m := NewInlineModel(&domain.CurrentState{ActiveSession: s}, nil, nil)
	m.phase, m.mode = phaseTimer, deepWorkMode()
	r, _ := send(t, m, stateMsg{state: stateNoSession()}, key("a"), key("enter"), key("enter"), key("enter"), key("enter"))
	if !r.(InlineModel).outcomeReviewMode {
		t.Fatal("expected outcome review")
	}
	r, _ = send(t, r, key("esc"))
	got := r.(InlineModel)
	if got.outcomeReviewMode || !got.outcomeReviewDone {
		t.Error("esc should skip the outcome review")
	}
}

func TestStrict_EnergizeReminderHidesBreakHint(t *testing.T) {
	// reminderHint reports whether the energize reminder offers [b]reak: a line
	// of its own in fullscreen, the reminder line itself inline.
	reminderHint := func(view string) bool {
		for _, line := range strings.Split(view, "\n") {
			l := strings.TrimSpace(line)
			if l == "[b]reak" || (strings.Contains(l, "Quick stretch") && strings.Contains(l, "[b]reak")) {
				return true
			}
		}
		return false
	}
	m := sizedModel(stateWithSession())
	m.mode, m.energizeTicks = makeTimeMode(), 10
	if !reminderHint(m.View()) {
		t.Fatal("non-strict reminder should offer [b]reak")
	}
	m.strict = true
	if reminderHint(m.View()) {
		t.Errorf("strict mode must hide the reminder's [b]reak hint:\n%s", m.View())
	}
	im := baseInlineModel()
	im.mode, im.energizeTicks = makeTimeMode(), 10
	if !reminderHint(im.View()) {
		t.Fatal("non-strict inline reminder should offer [b]reak")
	}
	im.strict = true
	if reminderHint(im.View()) {
		t.Errorf("inline strict mode must hide the reminder's [b]reak hint:\n%s", im.View())
	}
}

func TestPicker_HelpMatchesEscBehavior(t *testing.T) {
	p := pickerModel{items: []PickerItem{{Label: "a"}}}
	if !strings.Contains(p.View(), "esc cancel") {
		t.Error("picker help should say esc cancels")
	}
	r, _ := p.Update(key("esc"))
	if !r.(pickerModel).aborted {
		t.Error("esc aborts the picker")
	}
}

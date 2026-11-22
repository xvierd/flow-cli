package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/xvierd/flow-cli/internal/domain"
)

// readResource reads uri through the full JSON-RPC pipeline and decodes the
// JSON text content.
func readResource(t *testing.T, s *Server, uri string) map[string]interface{} {
	t.Helper()
	msg, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "resources/read",
		"params": map[string]interface{}{"uri": uri},
	})
	resp := s.server.HandleMessage(context.Background(), msg)
	r, ok := resp.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("resources/read %s: unexpected response %#v", uri, resp)
	}
	raw, _ := json.Marshal(r.Result)
	var result struct {
		Contents []struct {
			URI      string `json:"uri"`
			MIMEType string `json:"mimeType"`
			Text     string `json:"text"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || len(result.Contents) != 1 {
		t.Fatalf("decode %s: %v (%s)", uri, err, raw)
	}
	c := result.Contents[0]
	if c.URI != uri || c.MIMEType != "application/json" {
		t.Errorf("content uri/mime = %q/%q", c.URI, c.MIMEType)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(c.Text), &out); err != nil {
		t.Fatalf("resource text is not JSON: %v (%s)", err, c.Text)
	}
	return out
}

func fixedResourceClock(t *testing.T, now time.Time) {
	t.Helper()
	old := resourceNow
	resourceNow = func() time.Time { return now }
	t.Cleanup(func() { resourceNow = old })
}

func TestResources_Listed(t *testing.T) {
	s := newTestServer(&mockStateProvider{})
	msg := []byte(`{"jsonrpc":"2.0","id":1,"method":"resources/list"}`)
	raw, _ := json.Marshal(s.server.HandleMessage(context.Background(), msg))
	for _, uri := range []string{FocusResourceURI, TodayResourceURI} {
		if !strings.Contains(string(raw), uri) {
			t.Errorf("resources/list lacks %s: %s", uri, raw)
		}
	}
}

func TestFocusResource_Idle(t *testing.T) {
	fixedResourceClock(t, time.Now())
	got := readResource(t, newTestServer(&mockStateProvider{}), FocusResourceURI)
	if got["mode"] != "idle" || got["active"] != false || got["session"] != nil {
		t.Errorf("idle focus = %v", got)
	}
	if g, _ := got["guidance"].(string); !strings.Contains(g, "No focus session") {
		t.Errorf("idle guidance = %q", g)
	}
}

func TestFocusResource_DeepWork(t *testing.T) {
	now := time.Now()
	fixedResourceClock(t, now)
	session := &domain.PomodoroSession{
		ID: "sess-1", Type: domain.SessionTypeWork, Status: domain.SessionStatusRunning,
		Methodology: domain.MethodologyDeepWork, Duration: 90 * time.Minute,
		StartedAt: now.Add(-50 * time.Minute), IntendedOutcome: "tests pass",
		Distractions: []domain.Distraction{{Text: "email"}},
	}
	m := &mockStateProvider{currentState: &domain.CurrentState{
		ActiveSession: session,
		ActiveTask:    &domain.Task{ID: "task-1", Title: "Refactor auth"},
	}}
	got := readResource(t, newTestServer(m), FocusResourceURI)

	if got["mode"] != "deep_work" || got["active"] != true {
		t.Errorf("mode/active = %v/%v", got["mode"], got["active"])
	}
	g, _ := got["guidance"].(string)
	for _, want := range []string{"deep focus", "40 min left", "Refactor auth", "log_distraction"} {
		if !strings.Contains(g, want) {
			t.Errorf("guidance %q lacks %q", g, want)
		}
	}
	sess, _ := got["session"].(map[string]interface{})
	if sess["remaining_seconds"] != float64(40*60) || sess["ends_at"] == nil || sess["distractions"] != float64(1) {
		t.Errorf("session = %v", sess)
	}
	if task, _ := got["task"].(map[string]interface{}); task["title"] != "Refactor auth" {
		t.Errorf("task = %v", got["task"])
	}
}

func TestFocusResource_Break(t *testing.T) {
	now := time.Now()
	fixedResourceClock(t, now)
	m := &mockStateProvider{currentState: &domain.CurrentState{ActiveSession: &domain.PomodoroSession{
		ID: "b", Type: domain.SessionTypeShortBreak, Status: domain.SessionStatusRunning,
		Duration: 5 * time.Minute, StartedAt: now.Add(-time.Minute),
	}}}
	got := readResource(t, newTestServer(m), FocusResourceURI)
	if got["mode"] != "break" {
		t.Errorf("mode = %v", got["mode"])
	}
	if g, _ := got["guidance"].(string); !strings.Contains(g, "rest") {
		t.Errorf("break guidance = %q", g)
	}
}

func TestTodayResource(t *testing.T) {
	now := time.Now()
	fixedResourceClock(t, now)
	hl := "task-hl"
	m := &mockStateProvider{
		currentState: &domain.CurrentState{TodayStats: domain.DailyStats{BreaksTaken: 2}},
		focusReport: &domain.FocusReport{
			WorkSessions: 3, TotalWorkTime: 75 * time.Minute, DistractionCount: 4,
			AvgFocusScore: 4.5, FocusScoreCount: 2, HighlightID: &hl, HighlightTitle: "Ship v2",
		},
		tasks: []*domain.Task{
			{ID: "a", Title: "Pending", Status: domain.StatusPending},
			{ID: "b", Title: "Doing", Status: domain.StatusInProgress},
			{ID: "c", Title: "Done", Status: domain.StatusCompleted},
		},
	}
	got := readResource(t, newTestServer(m), TodayResourceURI)
	if got["date"] != now.Format("2006-01-02") || got["work_sessions"] != float64(3) ||
		got["focused_minutes"] != float64(75) || got["breaks_taken"] != float64(2) ||
		got["distractions"] != float64(4) || got["avg_focus_score"] != 4.5 {
		t.Errorf("today = %v", got)
	}
	if h, _ := got["highlight"].(map[string]interface{}); h["title"] != "Ship v2" {
		t.Errorf("highlight = %v", got["highlight"])
	}
	open, _ := got["open_tasks"].([]interface{})
	if len(open) != 2 || open[0].(map[string]interface{})["title"] != "Doing" {
		t.Errorf("open_tasks = %v (want in-progress first, completed excluded)", open)
	}
	if _, ok := got["guidance"].(string); !ok {
		t.Errorf("today lacks guidance")
	}
}

func TestFocusResource_ReadError(t *testing.T) {
	m := &mockStateProvider{readErr: context.DeadlineExceeded}
	s := newTestServer(m)
	msg := []byte(`{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"flow://focus"}}`)
	if _, ok := s.server.HandleMessage(context.Background(), msg).(mcp.JSONRPCError); !ok {
		t.Error("read error should be a JSON-RPC error")
	}
}

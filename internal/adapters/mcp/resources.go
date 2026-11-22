package mcp

// MCP resources: read-only views of the user's focus state that agents can
// load into context (flow://focus, flow://today).
//
// Change notifications: mcp-go v0.43.2 does not dispatch resources/subscribe,
// so the server does not advertise subscriptions and sends no
// notifications/resources/updated. Clients re-read the resource instead; the
// Claude Code hooks (`flow claude install`) inject the same guidance on every
// prompt.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/xvierd/flow-cli/internal/domain"
)

// Resource URIs exposed by the server.
const (
	FocusResourceURI = "flow://focus"
	TodayResourceURI = "flow://today"
)

// resourceNow is the clock used by resource handlers; tests replace it.
var resourceNow = time.Now

// registerResources registers the focus and today resources.
func (s *Server) registerResources() {
	s.server.AddResource(
		mcp.NewResource(
			FocusResourceURI,
			"Current focus",
			mcp.WithResourceDescription("The user's current focus state (mode, task, time left) and a 'guidance' text telling the agent how to behave: e.g. be concise and stay on task during Deep Work, encourage rest during a break. Read it before proposing work."),
			mcp.WithMIMEType("application/json"),
		),
		s.handleFocusResource,
	)
	s.server.AddResource(
		mcp.NewResource(
			TodayResourceURI,
			"Today",
			mcp.WithResourceDescription("Today's focus summary: completed sessions, focused time, breaks, distractions, highlight and open tasks, plus the current focus guidance."),
			mcp.WithMIMEType("application/json"),
		),
		s.handleTodayResource,
	)
}

// resourceMiddleware serializes resource reads with tool calls (reading the
// state can complete an expired session, a write) and turns a handler panic
// into an error.
func (s *Server) resourceMiddleware(next server.ResourceHandlerFunc) server.ResourceHandlerFunc {
	return func(ctx context.Context, request mcp.ReadResourceRequest) (contents []mcp.ResourceContents, err error) {
		s.toolMu.Lock()
		defer s.toolMu.Unlock()
		defer func() {
			if r := recover(); r != nil {
				contents, err = nil, fmt.Errorf("internal error reading %s: %v", request.Params.URI, r)
			}
		}()
		return next(ctx, request)
	}
}

// focusPayload builds the flow://focus document from state at now.
func focusPayload(state *domain.CurrentState, now time.Time) map[string]interface{} {
	mode := domain.CurrentFocusMode(state)
	payload := map[string]interface{}{
		"mode":       string(mode),
		"active":     mode != domain.FocusModeIdle,
		"guidance":   domain.FocusGuidance(state, now),
		"task":       nil,
		"session":    nil,
		"updated_at": now.Format(time.RFC3339),
	}
	if mode == domain.FocusModeIdle {
		return payload
	}
	s := state.ActiveSession
	remaining := s.RemainingAt(now)
	session := map[string]interface{}{
		"id":                s.ID,
		"type":              string(s.Type),
		"status":            string(s.Status),
		"methodology":       string(s.Methodology),
		"duration_minutes":  int(s.Duration / time.Minute),
		"remaining_seconds": int(remaining / time.Second),
		"started_at":        s.StartedAt.Format(time.RFC3339),
	}
	if s.Status == domain.SessionStatusRunning {
		session["ends_at"] = now.Add(remaining).Format(time.RFC3339)
	}
	if s.IntendedOutcome != "" {
		session["intended_outcome"] = s.IntendedOutcome
	}
	if s.GitBranch != "" {
		session["git_branch"] = s.GitBranch
	}
	if len(s.Tags) > 0 {
		session["tags"] = s.Tags
	}
	session["distractions"] = len(s.Distractions)
	payload["session"] = session
	if t := state.ActiveTask; t != nil {
		payload["task"] = map[string]interface{}{"id": t.ID, "title": t.Title}
	}
	return payload
}

// handleFocusResource serves flow://focus.
func (s *Server) handleFocusResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	state, err := s.stateProvider.GetCurrentState(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current state: %w", err)
	}
	return jsonResource(FocusResourceURI, focusPayload(state, resourceNow()))
}

// handleTodayResource serves flow://today.
func (s *Server) handleTodayResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	now := resourceNow()
	state, err := s.stateProvider.GetCurrentState(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current state: %w", err)
	}
	report, err := s.stateProvider.GetFocusReport(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get focus report: %w", err)
	}

	payload := map[string]interface{}{
		"date":               now.Format("2006-01-02"),
		"work_sessions":      report.WorkSessions,
		"focused_minutes":    int(report.TotalWorkTime / time.Minute),
		"breaks_taken":       state.TodayStats.BreaksTaken,
		"distractions":       report.DistractionCount,
		"deep_work_streak":   report.DeepWorkStreak,
		"avg_focus_score":    nil,
		"highlight":          nil,
		"open_tasks":         []map[string]interface{}{},
		"focus":              focusPayload(state, now),
		"guidance":           domain.FocusGuidance(state, now),
		"tasks_completed":    state.TodayStats.TasksCompleted,
		"current_focus_mode": string(domain.CurrentFocusMode(state)),
	}
	if report.FocusScoreCount > 0 {
		payload["avg_focus_score"] = report.AvgFocusScore
	}
	if report.HighlightID != nil {
		payload["highlight"] = map[string]interface{}{"id": *report.HighlightID, "title": report.HighlightTitle}
	}

	const maxOpenTasks = 10
	open := []map[string]interface{}{}
	if tasks, err := s.stateProvider.ListTasks(ctx, nil); err == nil {
		for _, st := range []domain.TaskStatus{domain.StatusInProgress, domain.StatusPending} {
			for _, t := range tasks {
				if len(open) < maxOpenTasks && t.Status == st {
					open = append(open, map[string]interface{}{"id": t.ID, "title": t.Title, "status": string(t.Status)})
				}
			}
		}
	}
	payload["open_tasks"] = open

	return jsonResource(TodayResourceURI, payload)
}

// jsonResource wraps data as a single JSON text resource.
func jsonResource(uri string, data map[string]interface{}) ([]mcp.ResourceContents, error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %s: %w", uri, err)
	}
	return []mcp.ResourceContents{mcp.TextResourceContents{URI: uri, MIMEType: "application/json", Text: string(b)}}, nil
}

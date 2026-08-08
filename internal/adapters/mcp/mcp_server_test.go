package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

// mockStateProvider is a configurable mock implementation of ports.MCPStateProvider.
type mockStateProvider struct {
	currentState   *domain.CurrentState
	tasks          []*domain.Task
	taskHistory    map[string][]*domain.PomodoroSession
	recentSessions []*domain.PomodoroSession
	dailyStats     *domain.DailyStats
	periodStats    *domain.PeriodStats
	focusReport    *domain.FocusReport
	highlight      *domain.Task

	created          map[string]string // taskID -> title
	distractionCalls []string
	lastNotes        string

	// err, when set, is returned by session lifecycle ops to simulate failures
	// (e.g. strict focus mode blocks).
	err error
}

func newMockSession() *domain.PomodoroSession {
	s := domain.NewPomodoroSession(domain.DefaultPomodoroConfig(), nil)
	s.Complete()
	return s
}

func (m *mockStateProvider) GetCurrentState(ctx context.Context) (*domain.CurrentState, error) {
	if m.currentState == nil {
		m.currentState = &domain.CurrentState{}
	}
	return m.currentState, nil
}

func (m *mockStateProvider) ListTasks(ctx context.Context, status *domain.TaskStatus) ([]*domain.Task, error) {
	return m.tasks, nil
}

func (m *mockStateProvider) GetTaskHistory(ctx context.Context, taskID string) ([]*domain.PomodoroSession, error) {
	if sessions, ok := m.taskHistory[taskID]; ok {
		return sessions, nil
	}
	return []*domain.PomodoroSession{}, nil
}

func (m *mockStateProvider) GetRecentSessions(ctx context.Context, limit int) ([]*domain.PomodoroSession, error) {
	if len(m.recentSessions) > limit {
		return m.recentSessions[:limit], nil
	}
	return m.recentSessions, nil
}

func (m *mockStateProvider) StartPomodoro(ctx context.Context, taskID *string, durationMinutes *int) (*domain.PomodoroSession, error) {
	if m.err != nil {
		return nil, m.err
	}
	config := domain.DefaultPomodoroConfig()
	if durationMinutes != nil {
		config.WorkDuration = time.Duration(*durationMinutes) * time.Minute
	}
	return domain.NewPomodoroSession(config, taskID), nil
}

func (m *mockStateProvider) StartSession(ctx context.Context, req ports.StartSessionRequest) (*domain.PomodoroSession, error) {
	config := domain.DefaultPomodoroConfig()
	if req.DurationMinutes != nil {
		config.WorkDuration = time.Duration(*req.DurationMinutes) * time.Minute
	}
	session := domain.NewPomodoroSession(config, req.TaskID)
	session.Methodology = req.Methodology
	session.IntendedOutcome = req.IntendedOutcome
	session.Tags = req.Tags
	if req.TaskTitle != "" && req.TaskID == nil {
		task, _ := domain.NewTask(req.TaskTitle)
		session.TaskID = &task.ID
	}
	return session, nil
}

func (m *mockStateProvider) StartBreak(ctx context.Context) (*domain.PomodoroSession, error) {
	return domain.NewBreakSession(domain.DefaultPomodoroConfig(), 1), nil
}

func (m *mockStateProvider) StopPomodoro(ctx context.Context) (*domain.PomodoroSession, error) {
	if m.err != nil {
		return nil, m.err
	}
	return newMockSession(), nil
}

func (m *mockStateProvider) PausePomodoro(ctx context.Context) (*domain.PomodoroSession, error) {
	if m.err != nil {
		return nil, m.err
	}
	s := newMockSession()
	s.Pause()
	return s, nil
}

func (m *mockStateProvider) ResumePomodoro(ctx context.Context) (*domain.PomodoroSession, error) {
	if m.err != nil {
		return nil, m.err
	}
	return newMockSession(), nil
}

func (m *mockStateProvider) CancelSession(ctx context.Context) error {
	return m.err
}

func (m *mockStateProvider) VoidSession(ctx context.Context) (*domain.PomodoroSession, error) {
	if m.err != nil {
		return nil, m.err
	}
	s := newMockSession()
	s.Interrupt()
	return s, nil
}

func (m *mockStateProvider) CreateTask(ctx context.Context, title string, description *string, tags []string) (*domain.Task, error) {
	task, err := domain.NewTask(title)
	if err != nil {
		return nil, err
	}
	task.Tags = tags
	if m.created == nil {
		m.created = map[string]string{}
	}
	m.created[task.ID] = title
	return task, nil
}

func (m *mockStateProvider) CompleteTask(ctx context.Context, taskID string) (*domain.Task, error) {
	if m.err != nil {
		return nil, m.err
	}
	task, _ := domain.NewTask("Completed Task")
	task.ID = taskID
	task.Complete()
	return task, nil
}

func (m *mockStateProvider) GetTask(ctx context.Context, taskID string) (*domain.Task, error) {
	for _, t := range m.tasks {
		if t.ID == taskID {
			return t, nil
		}
	}
	return nil, domain.ErrTaskNotFound
}

func (m *mockStateProvider) DeleteTask(ctx context.Context, taskID string) error {
	return nil
}

func (m *mockStateProvider) StartTask(ctx context.Context, taskID string) error {
	return nil
}

func (m *mockStateProvider) AddSessionNotes(ctx context.Context, sessionID string, notes string) (*domain.PomodoroSession, error) {
	if m.err != nil {
		return nil, m.err
	}
	s := newMockSession()
	s.ID = sessionID
	s.Notes = notes
	m.lastNotes = notes
	return s, nil
}

func (m *mockStateProvider) LogDistraction(ctx context.Context, sessionID string, text string, category string) error {
	m.distractionCalls = append(m.distractionCalls, text+":"+category)
	return nil
}

func (m *mockStateProvider) SetFocusScore(ctx context.Context, sessionID string, score int) error {
	return m.err
}

func (m *mockStateProvider) SetAccomplishment(ctx context.Context, sessionID string, text string) error {
	return m.err
}

func (m *mockStateProvider) SetShutdownRitual(ctx context.Context, sessionID string, ritual domain.ShutdownRitual) error {
	return m.err
}

func (m *mockStateProvider) SetEnergizeActivity(ctx context.Context, sessionID string, activity string) error {
	return m.err
}

func (m *mockStateProvider) SetOutcomeAchieved(ctx context.Context, sessionID string, achieved string) error {
	return m.err
}

func (m *mockStateProvider) GetTodayHighlight(ctx context.Context) (*domain.Task, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.highlight, nil
}

func (m *mockStateProvider) SetHighlight(ctx context.Context, taskID string) (*domain.Task, error) {
	if m.err != nil {
		return nil, m.err
	}
	task, _ := domain.NewTask("Highlight")
	task.ID = taskID
	task.SetAsHighlight()
	return task, nil
}

func (m *mockStateProvider) GetDailySummary(ctx context.Context, date time.Time) (*domain.DailyStats, error) {
	if m.dailyStats == nil {
		m.dailyStats = &domain.DailyStats{Date: date}
	}
	return m.dailyStats, nil
}

func (m *mockStateProvider) GetPeriodStats(ctx context.Context, start, end time.Time) (*domain.PeriodStats, error) {
	if m.periodStats == nil {
		m.periodStats = &domain.PeriodStats{}
	}
	return m.periodStats, nil
}

func (m *mockStateProvider) GetFocusReport(ctx context.Context) (*domain.FocusReport, error) {
	if m.focusReport == nil {
		m.focusReport = &domain.FocusReport{}
	}
	return m.focusReport, nil
}

func newTestServer(m *mockStateProvider) *Server {
	return NewServer(m, "test")
}

func resultToText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil {
		t.Fatal("nil result")
	}
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func request(args map[string]interface{}) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: args},
	}
}

func expectedTools() map[string]bool {
	names := []string{
		"get_current_state", "list_tasks", "get_task_history", "start_pomodoro",
		"stop_pomodoro", "pause_pomodoro", "resume_pomodoro", "create_task",
		"complete_task", "log_distraction", "set_focus_score", "get_today_highlight",
		"set_highlight", "add_session_notes",
		"start_session", "start_break", "cancel_session", "void_session",
		"set_accomplishment", "set_shutdown_ritual", "set_energize_activity",
		"set_outcome_achieved", "get_recent_sessions", "get_daily_summary",
		"get_period_stats", "get_focus_report", "get_task", "delete_task", "start_task",
	}
	result := make(map[string]bool, len(names))
	for _, n := range names {
		result[n] = true
	}
	return result
}

func TestNewServer(t *testing.T) {
	mock := &mockStateProvider{}
	server := NewServer(mock, "test")

	if server == nil {
		t.Fatal("NewServer() returned nil")
	}
	if server.stateProvider != mock {
		t.Error("NewServer() did not set state provider correctly")
	}
	if server.server == nil {
		t.Error("NewServer() did not create MCP server")
	}
}

func TestNewServer_RegistersAllTools(t *testing.T) {
	mock := &mockStateProvider{}
	server := NewServer(mock, "test")

	got := server.server.ListTools()
	for name := range expectedTools() {
		if _, ok := got[name]; !ok {
			t.Errorf("tool %q not registered", name)
		}
	}
}

func TestNewServer_HandlesEmptyVersion(t *testing.T) {
	if NewServer(&mockStateProvider{}, "") == nil {
		t.Fatal("NewServer('') returned nil")
	}
}

func TestServer_handleCreateTask_TagsAsArray(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleCreateTask(context.Background(), request(map[string]interface{}{
		"title":       "Build feature",
		"description": "desc",
		"tags":        []interface{}{"backend", "urgent"},
	}))
	if err != nil {
		t.Fatalf("handleCreateTask() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("handleCreateTask() returned an error result")
	}
	if len(mock.created) != 1 {
		t.Fatalf("expected 1 created task, got %d", len(mock.created))
	}
	body := resultToText(t, result)
	if !strings.Contains(body, "backend") || !strings.Contains(body, "urgent") {
		t.Errorf("array tags missing from result: %s", body)
	}
}

func TestServer_handleCreateTask_TagsAsCSV(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleCreateTask(context.Background(), request(map[string]interface{}{
		"title": "Build a task",
		"tags":  " backend , urgent ",
	}))
	if err != nil {
		t.Fatalf("handleCreateTask() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleCreateTask() error result: %+v", result)
	}
	body := resultToText(t, result)
	if !strings.Contains(body, "backend") || !strings.Contains(body, "urgent") {
		t.Errorf("CSV tags not parsed into array: %s", body)
	}
}

func TestServer_handleCreateTask_MissingTitle(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleCreateTask(context.Background(), request(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("handleCreateTask() error = %v", err)
	}
	if !result.IsError {
		t.Error("create_task without title should error")
	}
}

func TestServer_handleLogDistraction_Category(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleLogDistraction(context.Background(), request(map[string]interface{}{
		"session_id": "s1",
		"text":       "phone rings",
		"category":   "external",
	}))
	if err != nil {
		t.Fatalf("handleLogDistraction() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleLogDistraction() error result: %+v", result)
	}
	if len(mock.distractionCalls) != 1 || mock.distractionCalls[0] != "phone rings:external" {
		t.Errorf("distraction not logged with category: %v", mock.distractionCalls)
	}
}

func TestServer_handleLogDistraction_InvalidCategory(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleLogDistraction(context.Background(), request(map[string]interface{}{
		"session_id": "s1",
		"text":       "phone",
		"category":   "nonsense",
	}))
	if err != nil {
		t.Fatalf("handleLogDistraction() error = %v", err)
	}
	if !result.IsError {
		t.Error("log_distraction should reject invalid category")
	}
}

func TestServer_handleLogDistraction_MissingText(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleLogDistraction(context.Background(), request(map[string]interface{}{
		"session_id": "s1",
	}))
	if err != nil {
		t.Fatalf("handleLogDistraction() error = %v", err)
	}
	if !result.IsError {
		t.Error("log_distraction without text should error")
	}
}

func TestServer_handleSetFocusScore_Bounds(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	for _, score := range []float64{0, 6} {
		result, err := server.handleSetFocusScore(context.Background(), request(map[string]interface{}{
			"session_id": "s1",
			"score":      score,
		}))
		if err != nil {
			t.Fatalf("handleSetFocusScore() error = %v", err)
		}
		if !result.IsError {
			t.Errorf("score %.0f should be rejected", score)
		}
	}
}

func TestServer_handleGetCurrentState(t *testing.T) {
	config := domain.DefaultPomodoroConfig()
	task, _ := domain.NewTask("Test Task")
	session := domain.NewPomodoroSession(config, &task.ID)

	mock := &mockStateProvider{
		currentState: &domain.CurrentState{
			ActiveTask:    task,
			ActiveSession: session,
			TodayStats: domain.DailyStats{
				WorkSessions:  2,
				BreaksTaken:   1,
				TotalWorkTime: 50 * time.Minute,
			},
		},
	}
	server := newTestServer(mock)

	result, err := server.handleGetCurrentState(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("handleGetCurrentState() error = %v", err)
	}
	if result == nil {
		t.Fatal("handleGetCurrentState() returned nil result")
	}
	if len(result.Content) == 0 {
		t.Error("handleGetCurrentState() returned empty content")
	}
}

func TestServer_handleGetCurrentState_NoActiveSession(t *testing.T) {
	mock := &mockStateProvider{
		currentState: &domain.CurrentState{
			ActiveTask:    nil,
			ActiveSession: nil,
			TodayStats:    domain.DailyStats{},
		},
	}
	server := newTestServer(mock)

	result, err := server.handleGetCurrentState(context.Background(), request(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("handleGetCurrentState() error = %v", err)
	}
	if result == nil {
		t.Fatal("handleGetCurrentState() returned nil result")
	}
}

func TestServer_handleListTasks_WithStatusFilter(t *testing.T) {
	task1, _ := domain.NewTask("Task 1")
	task2, _ := domain.NewTask("Task 2")
	task2.Start()

	mock := &mockStateProvider{tasks: []*domain.Task{task1, task2}}
	server := newTestServer(mock)

	result, err := server.handleListTasks(context.Background(), request(map[string]interface{}{
		"status": "in_progress",
	}))
	if err != nil {
		t.Fatalf("handleListTasks() error = %v", err)
	}
	if result == nil {
		t.Fatal("handleListTasks() returned nil result")
	}
	body := resultToText(t, result)
	if !strings.Contains(body, task2.ID) || strings.Contains(body, task1.ID) {
		t.Errorf("status filter not applied: %s", body)
	}
}

func TestServer_handleGetTaskHistory_MissingTaskID(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleGetTaskHistory(context.Background(), request(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("handleGetTaskHistory() error = %v", err)
	}
	if !result.IsError {
		t.Error("get_task_history should error without task_id")
	}
}

func TestServer_handleStartSession(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleStartSession(context.Background(), request(map[string]interface{}{
		"methodology":      "deepwork",
		"task_title":       "Write proposal",
		"duration_minutes": 90.0,
		"tags":             []interface{}{"writing"},
		"intended_outcome": "Finish draft",
	}))
	if err != nil {
		t.Fatalf("handleStartSession() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleStartSession() error result: %+v", result)
	}
	body := resultToText(t, result)
	if !strings.Contains(body, "deepwork") || !strings.Contains(body, "Finish draft") {
		t.Errorf("start_session missing methodology/outcome: %s", body)
	}
}

func TestServer_handleStartSession_TagsAsCSV(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleStartSession(context.Background(), request(map[string]interface{}{
		"task_title": "Write proposal",
		"tags":       "a, b",
	}))
	if err != nil {
		t.Fatalf("handleStartSession() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleStartSession() error result: %+v", result)
	}
	if !strings.Contains(resultToText(t, result), "a") {
		t.Error("CSV tags not propagated to session")
	}
}

func TestServer_handleStartBreak(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleStartBreak(context.Background(), request(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("handleStartBreak() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleStartBreak() error result: %+v", result)
	}
}

func TestServer_handleCancelSession(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleCancelSession(context.Background(), request(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("handleCancelSession() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleCancelSession() error result: %+v", result)
	}
}

func TestServer_handleVoidSession(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleVoidSession(context.Background(), request(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("handleVoidSession() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleVoidSession() error result: %+v", result)
	}
}

func TestServer_StrictFocusErrorSurfaced(t *testing.T) {
	mock := &mockStateProvider{err: domain.ErrStrictFocusBlocked}
	server := newTestServer(mock)

	// Each handler must surface the strict focus error as an error tool result
	// (never a transport error), carrying the descriptive message to the client.
	tests := []struct {
		name    string
		handler func(context.Context) (*mcp.CallToolResult, error)
	}{
		{"stop", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return server.handleStopPomodoro(ctx, request(map[string]interface{}{}))
		}},
		{"pause", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return server.handlePausePomodoro(ctx, request(map[string]interface{}{}))
		}},
		{"void", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return server.handleVoidSession(ctx, request(map[string]interface{}{}))
		}},
		{"cancel", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return server.handleCancelSession(ctx, request(map[string]interface{}{}))
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.handler(context.Background())
			if err != nil {
				t.Fatalf("expected a tool error result, got transport error: %v", err)
			}
			if result == nil || !result.IsError {
				t.Fatal("expected IsError=true result conveying the strict focus block")
			}
			if len(result.Content) == 0 {
				t.Fatal("expected error content in result")
			}
			if text, ok := result.Content[0].(mcp.TextContent); ok && !strings.Contains(text.Text, "strict focus mode") {
				t.Errorf("error message = %q, want it to mention strict focus mode", text.Text)
			}
		})
	}
}

func TestServer_handleSetShutdownRitual(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleSetShutdownRitual(context.Background(), request(map[string]interface{}{
		"session_id":           "s1",
		"pending_tasks_review": "todo",
		"calendar_review":      "free",
		"tomorrow_plan":        "ship",
		"closing_phrase":       "Shutdown complete",
	}))
	if err != nil {
		t.Fatalf("handleSetShutdownRitual() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleSetShutdownRitual() error result: %+v", result)
	}
	body := resultToText(t, result)
	if !strings.Contains(body, "Shutdown complete") {
		t.Errorf("shutdown ritual fields missing: %s", body)
	}
}

func TestServer_handleSetOutcomeAchieved_Validation(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleSetOutcomeAchieved(context.Background(), request(map[string]interface{}{
		"session_id": "s1",
		"outcome":    "maybe",
	}))
	if err != nil {
		t.Fatalf("handleSetOutcomeAchieved() error = %v", err)
	}
	if !result.IsError {
		t.Error("set_outcome_achieved should reject invalid outcome")
	}
}

func TestServer_handleSetEnergizeActivity_Validation(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleSetEnergizeActivity(context.Background(), request(map[string]interface{}{
		"session_id": "s1",
		"activity":   "binge",
	}))
	if err != nil {
		t.Fatalf("handleSetEnergizeActivity() error = %v", err)
	}
	if !result.IsError {
		t.Error("set_energize_activity should reject invalid activity")
	}
}

func TestServer_handleGetRecentSessions_Limit(t *testing.T) {
	mock := &mockStateProvider{
		recentSessions: []*domain.PomodoroSession{
			newMockSession(), newMockSession(), newMockSession(),
		},
	}
	server := newTestServer(mock)

	result, err := server.handleGetRecentSessions(context.Background(), request(map[string]interface{}{
		"limit": 2.0,
	}))
	if err != nil {
		t.Fatalf("handleGetRecentSessions() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleGetRecentSessions() error result: %+v", result)
	}
	if !strings.Contains(resultToText(t, result), `"total_count": 2`) {
		t.Errorf("limit not respected: %s", resultToText(t, result))
	}
}

func TestServer_handleGetDailySummary(t *testing.T) {
	mock := &mockStateProvider{
		dailyStats: &domain.DailyStats{
			WorkSessions:  3,
			BreaksTaken:   1,
			TotalWorkTime: 75 * time.Minute,
		},
	}
	server := newTestServer(mock)

	result, err := server.handleGetDailySummary(context.Background(), request(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("handleGetDailySummary() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleGetDailySummary() error result: %+v", result)
	}
	if !strings.Contains(resultToText(t, result), `"work_sessions": 3`) {
		t.Errorf("daily summary missing stats: %s", resultToText(t, result))
	}
}

func TestServer_handleGetDailySummary_InvalidDate(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleGetDailySummary(context.Background(), request(map[string]interface{}{
		"date": "not-a-date",
	}))
	if err != nil {
		t.Fatalf("handleGetDailySummary() error = %v", err)
	}
	if !result.IsError {
		t.Error("get_daily_summary should reject invalid date")
	}
}

func TestServer_handleGetPeriodStats(t *testing.T) {
	mock := &mockStateProvider{
		periodStats: &domain.PeriodStats{
			TotalSessions:    5,
			TotalWorkTime:    2 * time.Hour,
			AvgFocusScore:    4.0,
			FocusScoreCount:  4,
			DistractionCount: 2,
		},
	}
	server := newTestServer(mock)

	result, err := server.handleGetPeriodStats(context.Background(), request(map[string]interface{}{
		"period": "week",
	}))
	if err != nil {
		t.Fatalf("handleGetPeriodStats() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleGetPeriodStats() error result: %+v", result)
	}
	body := resultToText(t, result)
	if !strings.Contains(body, `"period": "week"`) || !strings.Contains(body, `"total_sessions": 5`) {
		t.Errorf("period stats mismatch: %s", body)
	}
}

func TestServer_handleGetPeriodStats_Month(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleGetPeriodStats(context.Background(), request(map[string]interface{}{
		"period": "month",
	}))
	if err != nil {
		t.Fatalf("handleGetPeriodStats() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleGetPeriodStats() error result: %+v", result)
	}
	if !strings.Contains(resultToText(t, result), `"period": "month"`) {
		t.Errorf("month period not honored: %s", resultToText(t, result))
	}
}

func TestServer_handleGetPeriodStats_InvalidPeriod(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleGetPeriodStats(context.Background(), request(map[string]interface{}{
		"period": "year",
	}))
	if err != nil {
		t.Fatalf("handleGetPeriodStats() error = %v", err)
	}
	if !result.IsError {
		t.Error("get_period_stats should reject invalid period")
	}
}

func TestServer_handleGetFocusReport(t *testing.T) {
	title := "Focus"
	report := &domain.FocusReport{
		Date:            "2026-08-07T00:00:00Z",
		WorkSessions:    4,
		TotalWorkTime:   4 * time.Hour,
		AvgFocusScore:   3.5,
		FocusScoreCount: 4,
		DeepWorkStreak:  2,
		HighlightID:     &title,
		HighlightTitle:  title,
	}
	mock := &mockStateProvider{focusReport: report}
	server := newTestServer(mock)

	result, err := server.handleGetFocusReport(context.Background(), request(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("handleGetFocusReport() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleGetFocusReport() error result: %+v", result)
	}
	body := resultToText(t, result)
	if !strings.Contains(body, `"work_sessions": 4`) || !strings.Contains(body, `"deep_work_streak": 2`) {
		t.Errorf("focus report mismatch: %s", body)
	}
}

func TestServer_handleGetTask(t *testing.T) {
	task, _ := domain.NewTask("Find me")
	mock := &mockStateProvider{tasks: []*domain.Task{task}}
	server := newTestServer(mock)

	result, err := server.handleGetTask(context.Background(), request(map[string]interface{}{
		"task_id": task.ID,
	}))
	if err != nil {
		t.Fatalf("handleGetTask() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleGetTask() error result: %+v", result)
	}
	if !strings.Contains(resultToText(t, result), "Find me") {
		t.Errorf("get_task did not return task: %s", resultToText(t, result))
	}
}

func TestServer_handleGetTask_NotFound(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleGetTask(context.Background(), request(map[string]interface{}{
		"task_id": "missing",
	}))
	if err != nil {
		t.Fatalf("handleGetTask() error = %v", err)
	}
	if !result.IsError {
		t.Error("get_task should error for unknown task")
	}
}

func TestServer_handleDeleteTask_RequiresID(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleDeleteTask(context.Background(), request(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("handleDeleteTask() error = %v", err)
	}
	if !result.IsError {
		t.Error("delete_task should require task_id")
	}
}

func TestServer_handleStartTask_RequiresID(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleStartTask(context.Background(), request(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("handleStartTask() error = %v", err)
	}
	if !result.IsError {
		t.Error("start_task should require task_id")
	}
}

func TestServer_handleGetTodayHighlight_NoHighlight(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleGetTodayHighlight(context.Background(), request(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("handleGetTodayHighlight() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleGetTodayHighlight() error result: %+v", result)
	}
	if !strings.Contains(resultToText(t, result), "No highlight set") {
		t.Errorf("expected no-highlight message: %s", resultToText(t, result))
	}
}

func TestServer_handleGetTodayHighlight_WithHighlight(t *testing.T) {
	task, _ := domain.NewTask("Daily highlight")
	mock := &mockStateProvider{highlight: task}
	server := newTestServer(mock)

	result, err := server.handleGetTodayHighlight(context.Background(), request(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("handleGetTodayHighlight() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleGetTodayHighlight() error result: %+v", result)
	}
	if !strings.Contains(resultToText(t, result), "Daily highlight") {
		t.Errorf("highlight missing from result: %s", resultToText(t, result))
	}
}

func TestServer_handleSetHighlight_MissingTaskID(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleSetHighlight(context.Background(), request(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("handleSetHighlight() error = %v", err)
	}
	if !result.IsError {
		t.Error("set_highlight should require task_id")
	}
}

func TestServer_handleAddSessionNotes_Success(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleAddSessionNotes(context.Background(), request(map[string]interface{}{
		"session_id": "s1",
		"notes":      "finished the design review",
	}))
	if err != nil {
		t.Fatalf("handleAddSessionNotes() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleAddSessionNotes() error result: %+v", result)
	}
	if !strings.Contains(resultToText(t, result), "finished the design review") {
		t.Errorf("notes missing from result: %s", resultToText(t, result))
	}
	if mock.lastNotes != "finished the design review" {
		t.Errorf("notes not forwarded to provider: %q", mock.lastNotes)
	}
}

func TestServer_handleAddSessionNotes_MissingArgs(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	for name, args := range map[string]map[string]interface{}{
		"missing-session-id": {"notes": "x"},
		"missing-notes":      {"session_id": "s1"},
	} {
		result, err := server.handleAddSessionNotes(context.Background(), request(args))
		if err != nil {
			t.Fatalf("handleAddSessionNotes(%s) error = %v", name, err)
		}
		if !result.IsError {
			t.Errorf("%s: expected error result", name)
		}
	}
}

func TestServer_handleStartPomodoro_WithArgs(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleStartPomodoro(context.Background(), request(map[string]interface{}{
		"task_id":          "t1",
		"duration_minutes": 50.0,
	}))
	if err != nil {
		t.Fatalf("handleStartPomodoro() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleStartPomodoro() error result: %+v", result)
	}
	body := resultToText(t, result)
	if !strings.Contains(body, `"task_id": "t1"`) || !strings.Contains(body, "50m0s") {
		t.Errorf("start_pomodoro args not honored: %s", body)
	}
}

func TestServer_handleStartPomodoro_DurationAsString(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleStartPomodoro(context.Background(), request(map[string]interface{}{
		"duration_minutes": "40",
	}))
	if err != nil {
		t.Fatalf("handleStartPomodoro() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleStartPomodoro() error result: %+v", result)
	}
	if !strings.Contains(resultToText(t, result), "40m0s") {
		t.Errorf("duration string not parsed: %s", resultToText(t, result))
	}
}

func TestServer_handleResumePomodoro(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleResumePomodoro(context.Background(), request(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("handleResumePomodoro() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleResumePomodoro() error result: %+v", result)
	}
}

func TestServer_handleCompleteTask_RequiresID(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleCompleteTask(context.Background(), request(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("handleCompleteTask() error = %v", err)
	}
	if !result.IsError {
		t.Error("complete_task should require task_id")
	}
}

func TestServer_handleCompleteTask_Success(t *testing.T) {
	mock := &mockStateProvider{}
	server := newTestServer(mock)

	result, err := server.handleCompleteTask(context.Background(), request(map[string]interface{}{
		"task_id": "t1",
	}))
	if err != nil {
		t.Fatalf("handleCompleteTask() error = %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handleCompleteTask() error result: %+v", result)
	}
	if !strings.Contains(resultToText(t, result), "completed") {
		t.Errorf("completion status missing: %s", resultToText(t, result))
	}
}

func TestServer_ProviderErrorSurfacedAsToolResult(t *testing.T) {
	mock := &mockStateProvider{err: domain.ErrNoActiveSession}
	server := newTestServer(mock)

	tests := []struct {
		name    string
		handler func(context.Context) (*mcp.CallToolResult, error)
	}{
		{"start_pomodoro", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return server.handleStartPomodoro(ctx, request(map[string]interface{}{}))
		}},
		{"resume_pomodoro", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return server.handleResumePomodoro(ctx, request(map[string]interface{}{}))
		}},
		{"complete_task", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return server.handleCompleteTask(ctx, request(map[string]interface{}{"task_id": "t1"}))
		}},
		{"set_highlight", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return server.handleSetHighlight(ctx, request(map[string]interface{}{"task_id": "t1"}))
		}},
		{"get_today_highlight", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return server.handleGetTodayHighlight(ctx, request(map[string]interface{}{}))
		}},
		{"add_session_notes", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return server.handleAddSessionNotes(ctx, request(map[string]interface{}{
				"session_id": "s1",
				"notes":      "x",
			}))
		}},
		{"set_accomplishment", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return server.handleSetAccomplishment(ctx, request(map[string]interface{}{
				"session_id": "s1",
				"text":       "shipped",
			}))
		}},
		{"set_shutdown_ritual", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return server.handleSetShutdownRitual(ctx, request(map[string]interface{}{"session_id": "s1"}))
		}},
		{"set_energize_activity", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return server.handleSetEnergizeActivity(ctx, request(map[string]interface{}{
				"session_id": "s1",
				"activity":   "walk",
			}))
		}},
		{"set_outcome_achieved", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return server.handleSetOutcomeAchieved(ctx, request(map[string]interface{}{
				"session_id": "s1",
				"outcome":    "y",
			}))
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.handler(context.Background())
			if err != nil {
				t.Fatalf("expected tool error result, got transport error: %v", err)
			}
			if result == nil || !result.IsError {
				t.Fatal("expected IsError=true result conveying the provider error")
			}
		})
	}
}

func TestServer_Stop(t *testing.T) {
	mock := &mockStateProvider{}
	server := NewServer(mock, "test")

	// Stop before Start should not panic
	if err := server.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/dvidx/flow-cli/internal/domain"
	"github.com/mark3labs/mcp-go/mcp"
)

// mockStateProvider is a mock implementation of ports.MCPStateProvider for testing.
type mockStateProvider struct {
	currentState   *domain.CurrentState
	tasks          []*domain.Task
	taskHistory    map[string][]*domain.PomodoroSession
	recentSessions []*domain.PomodoroSession
}

func (m *mockStateProvider) GetCurrentState(ctx context.Context) (*domain.CurrentState, error) {
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
	config := domain.DefaultPomodoroConfig()
	if durationMinutes != nil {
		config.WorkDuration = time.Duration(*durationMinutes) * time.Minute
	}
	session := domain.NewPomodoroSession(config, taskID)
	return session, nil
}

func (m *mockStateProvider) StopPomodoro(ctx context.Context) (*domain.PomodoroSession, error) {
	return nil, nil
}

func (m *mockStateProvider) PausePomodoro(ctx context.Context) (*domain.PomodoroSession, error) {
	return nil, nil
}

func (m *mockStateProvider) ResumePomodoro(ctx context.Context) (*domain.PomodoroSession, error) {
	return nil, nil
}

func (m *mockStateProvider) CreateTask(ctx context.Context, title string, description *string, tags []string) (*domain.Task, error) {
	return domain.NewTask(title)
}

func (m *mockStateProvider) CompleteTask(ctx context.Context, taskID string) (*domain.Task, error) {
	return nil, nil
}

func (m *mockStateProvider) AddSessionNotes(ctx context.Context, sessionID string, notes string) (*domain.PomodoroSession, error) {
	return nil, nil
}

func TestNewServer(t *testing.T) {
	mock := &mockStateProvider{}
	server := NewServer(mock)

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

func TestServer_IsRunning(t *testing.T) {
	mock := &mockStateProvider{}
	server := NewServer(mock)

	if server.IsRunning() {
		t.Error("IsRunning() should return false before Start()")
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

	server := NewServer(mock)
	request := mcp.CallToolRequest{}

	result, err := server.handleGetCurrentState(context.Background(), request)
	if err != nil {
		t.Fatalf("handleGetCurrentState() error = %v", err)
	}

	if result == nil {
		t.Fatal("handleGetCurrentState() returned nil result")
	}

	// Check that the result contains text content
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

	server := NewServer(mock)
	request := mcp.CallToolRequest{}

	result, err := server.handleGetCurrentState(context.Background(), request)
	if err != nil {
		t.Fatalf("handleGetCurrentState() error = %v", err)
	}

	if result == nil {
		t.Fatal("handleGetCurrentState() returned nil result")
	}
}

func TestServer_handleListTasks(t *testing.T) {
	task1, _ := domain.NewTask("Task 1")
	task2, _ := domain.NewTask("Task 2")
	task2.Start()

	mock := &mockStateProvider{
		tasks: []*domain.Task{task1, task2},
	}

	server := NewServer(mock)
	request := mcp.CallToolRequest{}

	result, err := server.handleListTasks(context.Background(), request)
	if err != nil {
		t.Fatalf("handleListTasks() error = %v", err)
	}

	if result == nil {
		t.Fatal("handleListTasks() returned nil result")
	}

	if len(result.Content) == 0 {
		t.Error("handleListTasks() returned empty content")
	}
}

func TestServer_handleListTasks_WithStatusFilter(t *testing.T) {
	task1, _ := domain.NewTask("Task 1")
	task2, _ := domain.NewTask("Task 2")
	task2.Start()

	mock := &mockStateProvider{
		tasks: []*domain.Task{task1, task2},
	}

	server := NewServer(mock)
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"status": "in_progress",
			},
		},
	}

	result, err := server.handleListTasks(context.Background(), request)
	if err != nil {
		t.Fatalf("handleListTasks() error = %v", err)
	}

	if result == nil {
		t.Fatal("handleListTasks() returned nil result")
	}
}

func TestServer_handleGetTaskHistory(t *testing.T) {
	config := domain.DefaultPomodoroConfig()
	task, _ := domain.NewTask("Test Task")
	session1 := domain.NewPomodoroSession(config, &task.ID)
	session2 := domain.NewPomodoroSession(config, &task.ID)

	mock := &mockStateProvider{
		taskHistory: map[string][]*domain.PomodoroSession{
			task.ID: {session1, session2},
		},
	}

	server := NewServer(mock)
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"task_id": task.ID,
			},
		},
	}

	result, err := server.handleGetTaskHistory(context.Background(), request)
	if err != nil {
		t.Fatalf("handleGetTaskHistory() error = %v", err)
	}

	if result == nil {
		t.Fatal("handleGetTaskHistory() returned nil result")
	}

	if result.IsError {
		t.Error("handleGetTaskHistory() returned error result")
	}
}

func TestServer_handleGetTaskHistory_MissingTaskID(t *testing.T) {
	mock := &mockStateProvider{}
	server := NewServer(mock)
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{},
		},
	}

	result, err := server.handleGetTaskHistory(context.Background(), request)
	if err != nil {
		t.Fatalf("handleGetTaskHistory() error = %v", err)
	}

	if !result.IsError {
		t.Error("handleGetTaskHistory() should return error for missing task_id")
	}
}

func TestServer_Stop(t *testing.T) {
	mock := &mockStateProvider{}
	server := NewServer(mock)

	// Stop before Start should not panic
	err := server.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

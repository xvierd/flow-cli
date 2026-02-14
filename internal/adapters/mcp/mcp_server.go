// Package mcp provides the MCP (Model Context Protocol) server implementation.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/dvidx/flow-cli/internal/ports"
)

// Server implements the MCP server using mark3labs/mcp-go.
type Server struct {
	server        *server.MCPServer
	stateProvider ports.MCPStateProvider
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewServer creates a new MCP server instance.
func NewServer(stateProvider ports.MCPStateProvider) *Server {
	s := &Server{
		stateProvider: stateProvider,
	}

	// Create the MCP server
	s.server = server.NewMCPServer(
		"flow-pomodoro",
		"1.0.0",
		server.WithLogging(),
	)

	// Register tools
	s.registerTools()

	return s
}

// registerTools registers all available MCP tools.
func (s *Server) registerTools() {
	// Tool: get_current_state
	s.server.AddTool(
		mcp.NewTool(
			"get_current_state",
			mcp.WithDescription("Get the current Flow pomodoro state including active task, session, and daily stats"),
		),
		s.handleGetCurrentState,
	)

	// Tool: list_tasks
	tasksTool := mcp.NewTool(
		"list_tasks",
		mcp.WithDescription("List all tasks, optionally filtered by status"),
		mcp.WithString(
			"status",
			mcp.Description("Filter tasks by status: pending, in_progress, completed, cancelled"),
			mcp.Enum("pending", "in_progress", "completed", "cancelled"),
		),
	)
	s.server.AddTool(tasksTool, s.handleListTasks)

	// Tool: get_task_history
	taskHistoryTool := mcp.NewTool(
		"get_task_history",
		mcp.WithDescription("Get pomodoro session history for a specific task"),
		mcp.WithString(
			"task_id",
			mcp.Required(),
			mcp.Description("The ID of the task to get history for"),
		),
	)
	s.server.AddTool(taskHistoryTool, s.handleGetTaskHistory)
}

// Start begins serving MCP requests via stdio.
func (s *Server) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	
	// Start the stdio server
	return server.ServeStdio(s.server)
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

// IsRunning returns true if the server is active.
func (s *Server) IsRunning() bool {
	if s.ctx == nil {
		return false
	}
	return s.ctx.Err() == nil
}

// Ensure Server implements ports.MCPHandler.
var _ ports.MCPHandler = (*Server)(nil)

// handleGetCurrentState handles the get_current_state tool.
func (s *Server) handleGetCurrentState(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state, err := s.stateProvider.GetCurrentState(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current state: %w", err)
	}

	result := map[string]interface{}{
		"active_task":    nil,
		"active_session": nil,
		"today_stats": map[string]interface{}{
			"work_sessions":   state.TodayStats.WorkSessions,
			"breaks_taken":    state.TodayStats.BreaksTaken,
			"total_work_time": state.TodayStats.TotalWorkTime.String(),
		},
	}

	if state.ActiveTask != nil {
		result["active_task"] = map[string]interface{}{
			"id":          state.ActiveTask.ID,
			"title":       state.ActiveTask.Title,
			"description": state.ActiveTask.Description,
			"status":      string(state.ActiveTask.Status),
			"tags":        state.ActiveTask.Tags,
		}
	}

	if state.ActiveSession != nil {
		session := state.ActiveSession
		sessionData := map[string]interface{}{
			"id":             session.ID,
			"type":           string(session.Type),
			"status":         string(session.Status),
			"duration":       session.Duration.String(),
			"remaining_time": session.RemainingTime().String(),
			"progress":       session.Progress(),
			"started_at":     session.StartedAt.Format("2006-01-02T15:04:05"),
			"git_branch":     session.GitBranch,
			"git_commit":     session.GitCommit,
		}
		if session.TaskID != nil {
			sessionData["task_id"] = *session.TaskID
		}
		result["active_session"] = sessionData
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleListTasks handles the list_tasks tool.
func (s *Server) handleListTasks(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	status := request.GetString("status", "")

	tasks, err := s.stateProvider.ListTasks(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}

	// Filter by status if provided
	var filteredTasks []map[string]interface{}
	for _, task := range tasks {
		if status != "" && string(task.Status) != status {
			continue
		}
		filteredTasks = append(filteredTasks, map[string]interface{}{
			"id":          task.ID,
			"title":       task.Title,
			"description": task.Description,
			"status":      string(task.Status),
			"tags":        task.Tags,
			"created_at":  task.CreatedAt.Format("2006-01-02T15:04:05"),
		})
	}

	result := map[string]interface{}{
		"tasks":       filteredTasks,
		"total_count": len(filteredTasks),
	}

	if status != "" {
		result["filter_status"] = status
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tasks: %w", err)
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleGetTaskHistory handles the get_task_history tool.
func (s *Server) handleGetTaskHistory(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID, err := request.RequireString("task_id")
	if err != nil {
		return mcp.NewToolResultError("task_id is required: " + err.Error()), nil
	}

	sessions, err := s.stateProvider.GetTaskHistory(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task history: %w", err)
	}

	var sessionList []map[string]interface{}
	var totalWorkTime int64 // in seconds

	for _, session := range sessions {
		sessionData := map[string]interface{}{
			"id":         session.ID,
			"type":       string(session.Type),
			"status":     string(session.Status),
			"duration":   session.Duration.String(),
			"started_at": session.StartedAt.Format("2006-01-02T15:04:05"),
		}

		if session.CompletedAt != nil {
			sessionData["completed_at"] = session.CompletedAt.Format("2006-01-02T15:04:05")
		}
		if session.GitBranch != "" {
			sessionData["git_branch"] = session.GitBranch
		}
		if session.GitCommit != "" {
			sessionData["git_commit"] = session.GitCommit
		}

		sessionList = append(sessionList, sessionData)

		if session.Type == "work" && session.Status == "completed" {
			totalWorkTime += int64(session.Duration.Seconds())
		}
	}

	result := map[string]interface{}{
		"task_id":         taskID,
		"sessions":        sessionList,
		"total_sessions":  len(sessionList),
		"total_work_time": fmt.Sprintf("%ds", totalWorkTime),
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task history: %w", err)
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// Package mcp provides the MCP (Model Context Protocol) server implementation.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/xvierd/flow-cli/internal/ports"
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

	// Tool: start_pomodoro
	startPomodoroTool := mcp.NewTool(
		"start_pomodoro",
		mcp.WithDescription("Start a new pomodoro work session"),
		mcp.WithString(
			"task_id",
			mcp.Description("Optional task ID to associate with the session"),
		),
		mcp.WithNumber(
			"duration_minutes",
			mcp.Description("Optional custom duration in minutes (default: 25)"),
		),
	)
	s.server.AddTool(startPomodoroTool, s.handleStartPomodoro)

	// Tool: stop_pomodoro
	s.server.AddTool(
		mcp.NewTool(
			"stop_pomodoro",
			mcp.WithDescription("Complete the current pomodoro session"),
		),
		s.handleStopPomodoro,
	)

	// Tool: pause_pomodoro
	s.server.AddTool(
		mcp.NewTool(
			"pause_pomodoro",
			mcp.WithDescription("Pause the current pomodoro session"),
		),
		s.handlePausePomodoro,
	)

	// Tool: resume_pomodoro
	s.server.AddTool(
		mcp.NewTool(
			"resume_pomodoro",
			mcp.WithDescription("Resume a paused pomodoro session"),
		),
		s.handleResumePomodoro,
	)

	// Tool: create_task
	createTaskTool := mcp.NewTool(
		"create_task",
		mcp.WithDescription("Create a new task"),
		mcp.WithString(
			"title",
			mcp.Required(),
			mcp.Description("The title of the task"),
		),
		mcp.WithString(
			"description",
			mcp.Description("Optional description of the task"),
		),
		mcp.WithArray(
			"tags",
			mcp.Description("Optional array of tags"),
		),
	)
	s.server.AddTool(createTaskTool, s.handleCreateTask)

	// Tool: complete_task
	completeTaskTool := mcp.NewTool(
		"complete_task",
		mcp.WithDescription("Mark a task as completed"),
		mcp.WithString(
			"task_id",
			mcp.Required(),
			mcp.Description("The ID of the task to complete"),
		),
	)
	s.server.AddTool(completeTaskTool, s.handleCompleteTask)

	// Tool: log_distraction
	logDistractionTool := mcp.NewTool(
		"log_distraction",
		mcp.WithDescription("Log a distraction during a Deep Work session"),
		mcp.WithString(
			"session_id",
			mcp.Required(),
			mcp.Description("The ID of the active session"),
		),
		mcp.WithString(
			"text",
			mcp.Required(),
			mcp.Description("Description of the distraction"),
		),
	)
	s.server.AddTool(logDistractionTool, s.handleLogDistraction)

	// Tool: set_focus_score
	setFocusScoreTool := mcp.NewTool(
		"set_focus_score",
		mcp.WithDescription("Set the focus score for a Make Time session (1-5)"),
		mcp.WithString(
			"session_id",
			mcp.Required(),
			mcp.Description("The ID of the session"),
		),
		mcp.WithNumber(
			"score",
			mcp.Required(),
			mcp.Description("Focus score from 1 (distracted) to 5 (fully focused)"),
		),
	)
	s.server.AddTool(setFocusScoreTool, s.handleSetFocusScore)

	// Tool: get_today_highlight
	s.server.AddTool(
		mcp.NewTool(
			"get_today_highlight",
			mcp.WithDescription("Get today's highlight task (Make Time mode)"),
		),
		s.handleGetTodayHighlight,
	)

	// Tool: set_highlight
	setHighlightTool := mcp.NewTool(
		"set_highlight",
		mcp.WithDescription("Mark a task as today's highlight (Make Time mode)"),
		mcp.WithString(
			"task_id",
			mcp.Required(),
			mcp.Description("The ID of the task to set as today's highlight"),
		),
	)
	s.server.AddTool(setHighlightTool, s.handleSetHighlight)

	// Tool: add_session_notes
	addNotesTool := mcp.NewTool(
		"add_session_notes",
		mcp.WithDescription("Add notes to a pomodoro session"),
		mcp.WithString(
			"session_id",
			mcp.Required(),
			mcp.Description("The ID of the session to add notes to"),
		),
		mcp.WithString(
			"notes",
			mcp.Required(),
			mcp.Description("The notes to add"),
		),
	)
	s.server.AddTool(addNotesTool, s.handleAddSessionNotes)
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
			"id":              session.ID,
			"type":            string(session.Type),
			"status":          string(session.Status),
			"duration":        session.Duration.String(),
			"remaining_time":  session.RemainingTime().String(),
			"progress":        session.Progress(),
			"started_at":      session.StartedAt.Format("2006-01-02T15:04:05"),
			"git_branch":      session.GitBranch,
			"git_commit":      session.GitCommit,
			"notes":           session.Notes,
			"methodology":     string(session.Methodology),
			"distractions":    session.Distractions,
			"accomplishment":  session.Accomplishment,
			"intended_outcome": session.IntendedOutcome,
			"session_tags":     session.Tags,
		}
		if session.TaskID != nil {
			sessionData["task_id"] = *session.TaskID
		}
		if session.FocusScore != nil {
			sessionData["focus_score"] = *session.FocusScore
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
			"notes":      session.Notes,
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
		if session.Methodology != "" {
			sessionData["methodology"] = string(session.Methodology)
		}
		if session.FocusScore != nil {
			sessionData["focus_score"] = *session.FocusScore
		}
		if len(session.Distractions) > 0 {
			sessionData["distractions"] = session.Distractions
		}
		if session.Accomplishment != "" {
			sessionData["accomplishment"] = session.Accomplishment
		}
		if session.IntendedOutcome != "" {
			sessionData["intended_outcome"] = session.IntendedOutcome
		}
		if len(session.Tags) > 0 {
			sessionData["session_tags"] = session.Tags
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

// handleStartPomodoro handles the start_pomodoro tool.
func (s *Server) handleStartPomodoro(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var taskID *string
	if t := request.GetString("task_id", ""); t != "" {
		taskID = &t
	}

	var durationMinutes *int
	// Try to get duration from arguments using GetFloat first (JSON numbers are float64)
	if d := request.GetFloat("duration_minutes", 0); d > 0 {
		m := int(d)
		durationMinutes = &m
	} else if rawDuration := request.GetString("duration_minutes", ""); rawDuration != "" {
		if m, err := strconv.Atoi(rawDuration); err == nil {
			durationMinutes = &m
		}
	}

	session, err := s.stateProvider.StartPomodoro(ctx, taskID, durationMinutes)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to start pomodoro: %v", err)), nil
	}

	result := map[string]interface{}{
		"id":         session.ID,
		"type":       string(session.Type),
		"status":     string(session.Status),
		"duration":   session.Duration.String(),
		"started_at": session.StartedAt.Format("2006-01-02T15:04:05"),
	}

	if session.TaskID != nil {
		result["task_id"] = *session.TaskID
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleStopPomodoro handles the stop_pomodoro tool.
func (s *Server) handleStopPomodoro(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	session, err := s.stateProvider.StopPomodoro(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to stop pomodoro: %v", err)), nil
	}

	result := map[string]interface{}{
		"id":         session.ID,
		"type":       string(session.Type),
		"status":     string(session.Status),
		"duration":   session.Duration.String(),
		"started_at": session.StartedAt.Format("2006-01-02T15:04:05"),
	}

	if session.TaskID != nil {
		result["task_id"] = *session.TaskID
	}
	if session.CompletedAt != nil {
		result["completed_at"] = session.CompletedAt.Format("2006-01-02T15:04:05")
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handlePausePomodoro handles the pause_pomodoro tool.
func (s *Server) handlePausePomodoro(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	session, err := s.stateProvider.PausePomodoro(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to pause pomodoro: %v", err)), nil
	}

	result := map[string]interface{}{
		"id":             session.ID,
		"type":           string(session.Type),
		"status":         string(session.Status),
		"duration":       session.Duration.String(),
		"remaining_time": session.RemainingTime().String(),
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleResumePomodoro handles the resume_pomodoro tool.
func (s *Server) handleResumePomodoro(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	session, err := s.stateProvider.ResumePomodoro(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to resume pomodoro: %v", err)), nil
	}

	result := map[string]interface{}{
		"id":             session.ID,
		"type":           string(session.Type),
		"status":         string(session.Status),
		"duration":       session.Duration.String(),
		"remaining_time": session.RemainingTime().String(),
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleCreateTask handles the create_task tool.
func (s *Server) handleCreateTask(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title, err := request.RequireString("title")
	if err != nil {
		return mcp.NewToolResultError("title is required: " + err.Error()), nil
	}

	var description *string
	if d := request.GetString("description", ""); d != "" {
		description = &d
	}

	var tags []string
	if rawTags := request.GetString("tags", ""); rawTags != "" {
		// Parse comma-separated tags
		for _, tag := range strings.Split(rawTags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}

	task, err := s.stateProvider.CreateTask(ctx, title, description, tags)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create task: %v", err)), nil
	}

	result := map[string]interface{}{
		"id":          task.ID,
		"title":       task.Title,
		"description": task.Description,
		"status":      string(task.Status),
		"tags":        task.Tags,
		"created_at":  task.CreatedAt.Format("2006-01-02T15:04:05"),
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task: %w", err)
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleCompleteTask handles the complete_task tool.
func (s *Server) handleCompleteTask(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID, err := request.RequireString("task_id")
	if err != nil {
		return mcp.NewToolResultError("task_id is required: " + err.Error()), nil
	}

	task, err := s.stateProvider.CompleteTask(ctx, taskID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to complete task: %v", err)), nil
	}

	result := map[string]interface{}{
		"id":          task.ID,
		"title":       task.Title,
		"description": task.Description,
		"status":      string(task.Status),
		"tags":        task.Tags,
		"completed_at": task.CompletedAt.Format("2006-01-02T15:04:05"),
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task: %w", err)
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleLogDistraction handles the log_distraction tool.
func (s *Server) handleLogDistraction(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID, err := request.RequireString("session_id")
	if err != nil {
		return mcp.NewToolResultError("session_id is required: " + err.Error()), nil
	}

	text, err := request.RequireString("text")
	if err != nil {
		return mcp.NewToolResultError("text is required: " + err.Error()), nil
	}

	if err := s.stateProvider.LogDistraction(ctx, sessionID, text); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to log distraction: %v", err)), nil
	}

	result := map[string]interface{}{
		"session_id": sessionID,
		"logged":     text,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleSetFocusScore handles the set_focus_score tool.
func (s *Server) handleSetFocusScore(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID, err := request.RequireString("session_id")
	if err != nil {
		return mcp.NewToolResultError("session_id is required: " + err.Error()), nil
	}

	score := int(request.GetFloat("score", 0))
	if score < 1 || score > 5 {
		return mcp.NewToolResultError("score must be between 1 and 5"), nil
	}

	if err := s.stateProvider.SetFocusScore(ctx, sessionID, score); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to set focus score: %v", err)), nil
	}

	result := map[string]interface{}{
		"session_id": sessionID,
		"score":      score,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleGetTodayHighlight handles the get_today_highlight tool.
func (s *Server) handleGetTodayHighlight(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	task, err := s.stateProvider.GetTodayHighlight(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get today's highlight: %v", err)), nil
	}

	if task == nil {
		result := map[string]interface{}{
			"highlight": nil,
			"message":   "No highlight set for today",
		}
		jsonData, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(jsonData)), nil
	}

	result := map[string]interface{}{
		"highlight": map[string]interface{}{
			"id":          task.ID,
			"title":       task.Title,
			"description": task.Description,
			"status":      string(task.Status),
			"tags":        task.Tags,
		},
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal highlight: %w", err)
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleSetHighlight handles the set_highlight tool.
func (s *Server) handleSetHighlight(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID, err := request.RequireString("task_id")
	if err != nil {
		return mcp.NewToolResultError("task_id is required: " + err.Error()), nil
	}

	task, err := s.stateProvider.SetHighlight(ctx, taskID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to set highlight: %v", err)), nil
	}

	result := map[string]interface{}{
		"id":             task.ID,
		"title":          task.Title,
		"status":         string(task.Status),
		"highlight_date": task.HighlightDate.Format("2006-01-02"),
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task: %w", err)
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// handleAddSessionNotes handles the add_session_notes tool.
func (s *Server) handleAddSessionNotes(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID, err := request.RequireString("session_id")
	if err != nil {
		return mcp.NewToolResultError("session_id is required: " + err.Error()), nil
	}

	notes, err := request.RequireString("notes")
	if err != nil {
		return mcp.NewToolResultError("notes is required: " + err.Error()), nil
	}

	session, err := s.stateProvider.AddSessionNotes(ctx, sessionID, notes)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to add notes: %v", err)), nil
	}

	result := map[string]interface{}{
		"id":         session.ID,
		"type":       string(session.Type),
		"status":     string(session.Status),
		"duration":   session.Duration.String(),
		"started_at": session.StartedAt.Format("2006-01-02T15:04:05"),
		"notes":      session.Notes,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// Package mcp provides the MCP (Model Context Protocol) server implementation.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
	"github.com/xvierd/flow-cli/internal/services"
)

// Server implements the MCP server using mark3labs/mcp-go.
type Server struct {
	server        *server.MCPServer
	stateProvider ports.MCPStateProvider
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewServer creates a new MCP server instance.
func NewServer(stateProvider ports.MCPStateProvider, version string) *Server {
	if version == "" {
		version = "dev"
	}
	s := &Server{
		stateProvider: stateProvider,
	}

	// Create the MCP server
	s.server = server.NewMCPServer(
		"flow",
		version,
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

	// Tool: start_pomodoro (deprecated alias of start_session)
	startPomodoroTool := mcp.NewTool(
		"start_pomodoro",
		mcp.WithDescription("Start a work session. Deprecated alias for start_session; use start_session to set methodology, tags, task title, and intended outcome."),
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
		mcp.WithString(
			"category",
			mcp.Description("Category of the distraction: internal or external"),
			mcp.Enum("internal", "external"),
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

	// Tool: start_session
	startSessionTool := mcp.NewTool(
		"start_session",
		mcp.WithDescription("Start a work session with optional methodology, task title, duration, tags, and intended outcome"),
		mcp.WithString(
			"methodology",
			mcp.Description("Methodology for the session"),
			mcp.Enum("pomodoro", "deepwork", "maketime"),
		),
		mcp.WithString(
			"task_id",
			mcp.Description("Existing task ID (mutually exclusive with task_title)"),
		),
		mcp.WithString(
			"task_title",
			mcp.Description("Task title; matched against existing tasks or created if not found"),
		),
		mcp.WithNumber(
			"duration_minutes",
			mcp.Description("Optional custom duration in minutes"),
		),
		mcp.WithArray(
			"tags",
			mcp.Description("Optional session tags (array or comma-separated string)"),
		),
		mcp.WithString(
			"intended_outcome",
			mcp.Description("Intended outcome for Deep Work sessions"),
		),
	)
	s.server.AddTool(startSessionTool, s.handleStartSession)

	// Tool: start_break
	s.server.AddTool(
		mcp.NewTool(
			"start_break",
			mcp.WithDescription("Start a short or long break session"),
		),
		s.handleStartBreak,
	)

	// Tool: cancel_session
	s.server.AddTool(
		mcp.NewTool(
			"cancel_session",
			mcp.WithDescription("Cancel the active session"),
		),
		s.handleCancelSession,
	)

	// Tool: void_session
	s.server.AddTool(
		mcp.NewTool(
			"void_session",
			mcp.WithDescription("Void (invalidate) the active session due to interruption"),
		),
		s.handleVoidSession,
	)

	// Tool: set_accomplishment
	setAccomplishmentTool := mcp.NewTool(
		"set_accomplishment",
		mcp.WithDescription("Record what you accomplished in a session (Deep Work shutdown ritual)"),
		mcp.WithString(
			"session_id",
			mcp.Required(),
			mcp.Description("The ID of the session"),
		),
		mcp.WithString(
			"text",
			mcp.Required(),
			mcp.Description("Accomplishment text"),
		),
	)
	s.server.AddTool(setAccomplishmentTool, s.handleSetAccomplishment)

	// Tool: set_shutdown_ritual
	setRitualTool := mcp.NewTool(
		"set_shutdown_ritual",
		mcp.WithDescription("Record the 4-step Deep Work shutdown ritual for a session"),
		mcp.WithString(
			"session_id",
			mcp.Required(),
			mcp.Description("The ID of the session"),
		),
		mcp.WithString(
			"pending_tasks_review",
			mcp.Description("Review pending tasks"),
		),
		mcp.WithString(
			"calendar_review",
			mcp.Description("Review tomorrow's calendar"),
		),
		mcp.WithString(
			"tomorrow_plan",
			mcp.Description("Plan for tomorrow"),
		),
		mcp.WithString(
			"closing_phrase",
			mcp.Description("Closing phrase (e.g. 'Shutdown complete')"),
		),
	)
	s.server.AddTool(setRitualTool, s.handleSetShutdownRitual)

	// Tool: set_energize_activity
	setEnergizeTool := mcp.NewTool(
		"set_energize_activity",
		mcp.WithDescription("Record how you will recharge after a Make Time session"),
		mcp.WithString(
			"session_id",
			mcp.Required(),
			mcp.Description("The ID of the session"),
		),
		mcp.WithString(
			"activity",
			mcp.Required(),
			mcp.Description("Energize activity"),
			mcp.Enum("walk", "stretch", "exercise", "none"),
		),
	)
	s.server.AddTool(setEnergizeTool, s.handleSetEnergizeActivity)

	// Tool: set_outcome_achieved
	setOutcomeTool := mcp.NewTool(
		"set_outcome_achieved",
		mcp.WithDescription("Record whether a Deep Work session's intended outcome was achieved"),
		mcp.WithString(
			"session_id",
			mcp.Required(),
			mcp.Description("The ID of the session"),
		),
		mcp.WithString(
			"outcome",
			mcp.Required(),
			mcp.Description("Achievement status"),
			mcp.Enum("y", "n", "p"),
		),
	)
	s.server.AddTool(setOutcomeTool, s.handleSetOutcomeAchieved)

	// Tool: get_recent_sessions
	getRecentTool := mcp.NewTool(
		"get_recent_sessions",
		mcp.WithDescription("Get recent pomodoro sessions"),
		mcp.WithNumber(
			"limit",
			mcp.Description("Maximum number of sessions to return (default: 10)"),
		),
	)
	s.server.AddTool(getRecentTool, s.handleGetRecentSessions)

	// Tool: get_daily_summary
	getDailySummaryTool := mcp.NewTool(
		"get_daily_summary",
		mcp.WithDescription("Get aggregated statistics for a date"),
		mcp.WithString(
			"date",
			mcp.Description("Date in YYYY-MM-DD or RFC3339 format (default: today)"),
		),
	)
	s.server.AddTool(getDailySummaryTool, s.handleGetDailySummary)

	// Tool: get_period_stats
	getPeriodTool := mcp.NewTool(
		"get_period_stats",
		mcp.WithDescription("Get aggregated statistics for a period (current week or month)"),
		mcp.WithString(
			"period",
			mcp.Description("Time period"),
			mcp.Enum("week", "month"),
		),
	)
	s.server.AddTool(getPeriodTool, s.handleGetPeriodStats)

	// Tool: get_focus_report
	s.server.AddTool(
		mcp.NewTool(
			"get_focus_report",
			mcp.WithDescription("Get a daily focus summary: sessions, focus scores, distractions, strength, and today's highlight"),
		),
		s.handleGetFocusReport,
	)

	// Tool: get_task
	getTaskTool := mcp.NewTool(
		"get_task",
		mcp.WithDescription("Get a single task by ID"),
		mcp.WithString(
			"task_id",
			mcp.Required(),
			mcp.Description("The ID of the task"),
		),
	)
	s.server.AddTool(getTaskTool, s.handleGetTask)

	// Tool: delete_task
	deleteTaskTool := mcp.NewTool(
		"delete_task",
		mcp.WithDescription("Delete a task by ID"),
		mcp.WithString(
			"task_id",
			mcp.Required(),
			mcp.Description("The ID of the task to delete"),
		),
	)
	s.server.AddTool(deleteTaskTool, s.handleDeleteTask)

	// Tool: start_task
	startTaskTool := mcp.NewTool(
		"start_task",
		mcp.WithDescription("Mark a task as in progress"),
		mcp.WithString(
			"task_id",
			mcp.Required(),
			mcp.Description("The ID of the task to start"),
		),
	)
	s.server.AddTool(startTaskTool, s.handleStartTask)
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
			"id":               session.ID,
			"type":             string(session.Type),
			"status":           string(session.Status),
			"duration":         session.Duration.String(),
			"remaining_time":   session.RemainingTime().String(),
			"progress":         session.Progress(),
			"started_at":       session.StartedAt.Format(time.RFC3339),
			"git_branch":       session.GitBranch,
			"git_commit":       session.GitCommit,
			"notes":            session.Notes,
			"methodology":      string(session.Methodology),
			"distractions":     session.Distractions,
			"accomplishment":   session.Accomplishment,
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
			"created_at":  task.CreatedAt.Format(time.RFC3339),
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
			"started_at": session.StartedAt.Format(time.RFC3339),
			"notes":      session.Notes,
		}

		if session.CompletedAt != nil {
			sessionData["completed_at"] = session.CompletedAt.Format(time.RFC3339)
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
		"started_at": session.StartedAt.Format(time.RFC3339),
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
		"started_at": session.StartedAt.Format(time.RFC3339),
	}

	if session.TaskID != nil {
		result["task_id"] = *session.TaskID
	}
	if session.CompletedAt != nil {
		result["completed_at"] = session.CompletedAt.Format(time.RFC3339)
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

	tags := parseTagsArg(request, "tags")

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
		"created_at":  task.CreatedAt.Format(time.RFC3339),
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
		"id":           task.ID,
		"title":        task.Title,
		"description":  task.Description,
		"status":       string(task.Status),
		"tags":         task.Tags,
		"completed_at": task.CompletedAt.Format(time.RFC3339),
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

	category := request.GetString("category", "")
	if category != "" && category != "internal" && category != "external" {
		return mcp.NewToolResultError("category must be 'internal' or 'external'"), nil
	}

	if err := s.stateProvider.LogDistraction(ctx, sessionID, text, category); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to log distraction: %v", err)), nil
	}

	result := map[string]interface{}{
		"session_id": sessionID,
		"logged":     text,
	}

	if category != "" {
		result["category"] = category
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
		"started_at": session.StartedAt.Format(time.RFC3339),
		"notes":      session.Notes,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// ---- helpers ----

// parseTagsArg reads a tags argument accepting either an array or a comma-separated string.
func parseTagsArg(request mcp.CallToolRequest, key string) []string {
	raw, ok := request.GetArguments()[key]
	if !ok || raw == nil {
		return nil
	}

	switch v := raw.(type) {
	case []any:
		tags := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				if s := strings.TrimSpace(str); s != "" {
					tags = append(tags, s)
				}
			}
		}
		return tags
	case []string:
		return v
	case string:
		var tags []string
		for _, tag := range strings.Split(v, ",") {
			if s := strings.TrimSpace(tag); s != "" {
				tags = append(tags, s)
			}
		}
		return tags
	default:
		return nil
	}
}

// parseOptionalDate parses an optional YYYY-MM-DD or RFC3339 date argument.
func parseOptionalDate(request mcp.CallToolRequest, key string) (time.Time, bool, error) {
	raw := strings.TrimSpace(request.GetString(key, ""))
	if raw == "" {
		return time.Time{}, false, nil
	}
	now := time.Now()
	if t, err := time.ParseInLocation("2006-01-02", raw, now.Location()); err == nil {
		return t, true, nil
	}
	if t, err := time.ParseInLocation(time.RFC3339, raw, now.Location()); err == nil {
		return t, true, nil
	}
	return time.Time{}, false, fmt.Errorf("invalid date %q: expected YYYY-MM-DD or RFC3339", raw)
}

// sessionSummary builds a JSON-friendly map for a session.
func sessionSummary(s *domain.PomodoroSession) map[string]interface{} {
	sd := map[string]interface{}{
		"id":         s.ID,
		"type":       string(s.Type),
		"status":     string(s.Status),
		"duration":   s.Duration.String(),
		"started_at": s.StartedAt.Format(time.RFC3339),
	}
	if s.CompletedAt != nil {
		sd["completed_at"] = s.CompletedAt.Format(time.RFC3339)
	}
	if s.TaskID != nil {
		sd["task_id"] = *s.TaskID
	}
	if s.Methodology != "" {
		sd["methodology"] = string(s.Methodology)
	}
	if s.FocusScore != nil {
		sd["focus_score"] = *s.FocusScore
	}
	if len(s.Distractions) > 0 {
		ds := make([]map[string]interface{}, 0, len(s.Distractions))
		for _, d := range s.Distractions {
			dm := map[string]interface{}{"text": d.Text}
			if d.Category != "" {
				dm["category"] = d.Category
			}
			ds = append(ds, dm)
		}
		sd["distractions"] = ds
	}
	if s.ShutdownRitual != nil {
		sd["shutdown_ritual"] = map[string]interface{}{
			"pending_tasks_review": s.ShutdownRitual.PendingTasksReview,
			"calendar_review":      s.ShutdownRitual.CalendarReview,
			"tomorrow_plan":        s.ShutdownRitual.TomorrowPlan,
			"closing_phrase":       s.ShutdownRitual.ClosingPhrase,
		}
	}
	if s.Accomplishment != "" {
		sd["accomplishment"] = s.Accomplishment
	}
	if s.IntendedOutcome != "" {
		sd["intended_outcome"] = s.IntendedOutcome
	}
	if s.OutcomeAchieved != "" {
		sd["outcome_achieved"] = s.OutcomeAchieved
	}
	if s.EnergizeActivity != "" {
		sd["energize_activity"] = s.EnergizeActivity
	}
	if len(s.Tags) > 0 {
		sd["tags"] = s.Tags
	}
	return sd
}

// jsonResult marshals data into an MCP tool result.
func jsonResult(data map[string]interface{}) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}
	return mcp.NewToolResultText(string(b)), nil
}

// toolError returns a consistent error tool result.
func toolError(msg string, err error) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(fmt.Sprintf("%s: %v", msg, err)), nil
}

// ---- new tool handlers ----

// handleStartSession handles the start_session tool.
func (s *Server) handleStartSession(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	req := ports.StartSessionRequest{
		Methodology:     domain.Methodology(request.GetString("methodology", "")),
		TaskTitle:       request.GetString("task_title", ""),
		IntendedOutcome: request.GetString("intended_outcome", ""),
		Tags:            parseTagsArg(request, "tags"),
	}
	if t := request.GetString("task_id", ""); t != "" {
		req.TaskID = &t
	}
	if d := request.GetFloat("duration_minutes", 0); d > 0 {
		m := int(d)
		req.DurationMinutes = &m
	} else if rawDur := request.GetString("duration_minutes", ""); rawDur != "" {
		if m, err := strconv.Atoi(rawDur); err == nil && m > 0 {
			req.DurationMinutes = &m
		}
	}

	session, err := s.stateProvider.StartSession(ctx, req)
	if err != nil {
		return toolError("failed to start session", err)
	}
	return jsonResult(sessionSummary(session))
}

// handleStartBreak handles the start_break tool.
func (s *Server) handleStartBreak(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	session, err := s.stateProvider.StartBreak(ctx)
	if err != nil {
		return toolError("failed to start break", err)
	}
	return jsonResult(sessionSummary(session))
}

// handleCancelSession handles the cancel_session tool.
func (s *Server) handleCancelSession(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := s.stateProvider.CancelSession(ctx); err != nil {
		return toolError("failed to cancel session", err)
	}
	return jsonResult(map[string]interface{}{"cancelled": true})
}

// handleVoidSession handles the void_session tool.
func (s *Server) handleVoidSession(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	session, err := s.stateProvider.VoidSession(ctx)
	if err != nil {
		return toolError("failed to void session", err)
	}
	return jsonResult(sessionSummary(session))
}

// handleSetAccomplishment handles the set_accomplishment tool.
func (s *Server) handleSetAccomplishment(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID, err := request.RequireString("session_id")
	if err != nil {
		return mcp.NewToolResultError("session_id is required: " + err.Error()), nil
	}
	text, err := request.RequireString("text")
	if err != nil {
		return mcp.NewToolResultError("text is required: " + err.Error()), nil
	}

	if err := s.stateProvider.SetAccomplishment(ctx, sessionID, text); err != nil {
		return toolError("failed to set accomplishment", err)
	}
	return jsonResult(map[string]interface{}{
		"session_id":     sessionID,
		"accomplishment": text,
	})
}

// handleSetShutdownRitual handles the set_shutdown_ritual tool.
func (s *Server) handleSetShutdownRitual(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID, err := request.RequireString("session_id")
	if err != nil {
		return mcp.NewToolResultError("session_id is required: " + err.Error()), nil
	}

	ritual := domain.ShutdownRitual{
		PendingTasksReview: request.GetString("pending_tasks_review", ""),
		CalendarReview:     request.GetString("calendar_review", ""),
		TomorrowPlan:       request.GetString("tomorrow_plan", ""),
		ClosingPhrase:      request.GetString("closing_phrase", ""),
	}

	if err := s.stateProvider.SetShutdownRitual(ctx, sessionID, ritual); err != nil {
		return toolError("failed to set shutdown ritual", err)
	}
	return jsonResult(map[string]interface{}{
		"session_id":           sessionID,
		"pending_tasks_review": ritual.PendingTasksReview,
		"calendar_review":      ritual.CalendarReview,
		"tomorrow_plan":        ritual.TomorrowPlan,
		"closing_phrase":       ritual.ClosingPhrase,
	})
}

// handleSetEnergizeActivity handles the set_energize_activity tool.
func (s *Server) handleSetEnergizeActivity(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID, err := request.RequireString("session_id")
	if err != nil {
		return mcp.NewToolResultError("session_id is required: " + err.Error()), nil
	}
	activity, err := request.RequireString("activity")
	if err != nil {
		return mcp.NewToolResultError("activity is required: " + err.Error()), nil
	}
	switch activity {
	case "walk", "stretch", "exercise", "none":
	default:
		return mcp.NewToolResultError("activity must be one of: walk, stretch, exercise, none"), nil
	}

	if err := s.stateProvider.SetEnergizeActivity(ctx, sessionID, activity); err != nil {
		return toolError("failed to set energize activity", err)
	}
	return jsonResult(map[string]interface{}{
		"session_id": sessionID,
		"activity":   activity,
	})
}

// handleSetOutcomeAchieved handles the set_outcome_achieved tool.
func (s *Server) handleSetOutcomeAchieved(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID, err := request.RequireString("session_id")
	if err != nil {
		return mcp.NewToolResultError("session_id is required: " + err.Error()), nil
	}
	outcome, err := request.RequireString("outcome")
	if err != nil {
		return mcp.NewToolResultError("outcome is required: " + err.Error()), nil
	}
	switch outcome {
	case "y", "n", "p":
	default:
		return mcp.NewToolResultError("outcome must be 'y', 'n', or 'p'"), nil
	}

	if err := s.stateProvider.SetOutcomeAchieved(ctx, sessionID, outcome); err != nil {
		return toolError("failed to set outcome achieved", err)
	}
	return jsonResult(map[string]interface{}{
		"session_id": sessionID,
		"outcome":    outcome,
	})
}

// handleGetRecentSessions handles the get_recent_sessions tool.
func (s *Server) handleGetRecentSessions(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limit := int(request.GetFloat("limit", 10))
	if limit <= 0 {
		limit = 10
	}

	sessions, err := s.stateProvider.GetRecentSessions(ctx, limit)
	if err != nil {
		return toolError("failed to get recent sessions", err)
	}

	summaries := make([]map[string]interface{}, 0, len(sessions))
	for _, sess := range sessions {
		summaries = append(summaries, sessionSummary(sess))
	}
	return jsonResult(map[string]interface{}{
		"sessions":     summaries,
		"total_count":  len(summaries),
		"requested_at": time.Now().Format(time.RFC3339),
	})
}

// handleGetDailySummary handles the get_daily_summary tool.
func (s *Server) handleGetDailySummary(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	date, has, err := parseOptionalDate(request, "date")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if !has {
		date = time.Now()
	}

	stats, err := s.stateProvider.GetDailySummary(ctx, date)
	if err != nil {
		return toolError("failed to get daily summary", err)
	}
	return jsonResult(map[string]interface{}{
		"date":            date.Format("2006-01-02"),
		"work_sessions":   stats.WorkSessions,
		"break_sessions":  stats.BreaksTaken,
		"total_work_time": stats.TotalWorkTime.String(),
		"tasks_completed": stats.TasksCompleted,
	})
}

// handleGetPeriodStats handles the get_period_stats tool.
func (s *Server) handleGetPeriodStats(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	periodStr := request.GetString("period", "week")
	start, end, err := services.PeriodRange(services.ReportPeriod(periodStr), time.Now())
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	stats, err := s.stateProvider.GetPeriodStats(ctx, start, end)
	if err != nil {
		return toolError("failed to get period stats", err)
	}

	methodologies := make([]map[string]interface{}, 0, len(stats.ByMethodology))
	for _, m := range stats.ByMethodology {
		methodologies = append(methodologies, map[string]interface{}{
			"methodology":   string(m.Methodology),
			"session_count": m.SessionCount,
			"total_time":    m.TotalTime.String(),
		})
	}

	return jsonResult(map[string]interface{}{
		"period":            periodStr,
		"start":             start.Format(time.RFC3339),
		"end":               end.Format(time.RFC3339),
		"total_sessions":    stats.TotalSessions,
		"total_work_time":   stats.TotalWorkTime.String(),
		"avg_focus_score":   stats.AvgFocusScore,
		"focus_score_count": stats.FocusScoreCount,
		"distraction_count": stats.DistractionCount,
		"by_methodology":    methodologies,
	})
}

// handleGetFocusReport handles the get_focus_report tool.
func (s *Server) handleGetFocusReport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	report, err := s.stateProvider.GetFocusReport(ctx)
	if err != nil {
		return toolError("failed to get focus report", err)
	}

	highlight := map[string]interface{}{}
	if report.HighlightID != nil {
		highlight = map[string]interface{}{
			"id":    *report.HighlightID,
			"title": report.HighlightTitle,
		}
	}

	return jsonResult(map[string]interface{}{
		"date":              report.Date,
		"work_sessions":     report.WorkSessions,
		"total_work_time":   report.TotalWorkTime.String(),
		"avg_focus_score":   report.AvgFocusScore,
		"focus_score_count": report.FocusScoreCount,
		"distraction_count": report.DistractionCount,
		"deep_work_streak":  report.DeepWorkStreak,
		"highlight":         highlight,
	})
}

// handleGetTask handles the get_task tool.
func (s *Server) handleGetTask(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID, err := request.RequireString("task_id")
	if err != nil {
		return mcp.NewToolResultError("task_id is required: " + err.Error()), nil
	}

	task, err := s.stateProvider.GetTask(ctx, taskID)
	if err != nil {
		return toolError("failed to get task", err)
	}
	if task == nil {
		return mcp.NewToolResultError("task not found"), nil
	}

	return jsonResult(map[string]interface{}{
		"id":          task.ID,
		"title":       task.Title,
		"description": task.Description,
		"status":      string(task.Status),
		"tags":        task.Tags,
		"created_at":  task.CreatedAt.Format(time.RFC3339),
		"updated_at":  task.UpdatedAt.Format(time.RFC3339),
	})
}

// handleDeleteTask handles the delete_task tool.
func (s *Server) handleDeleteTask(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID, err := request.RequireString("task_id")
	if err != nil {
		return mcp.NewToolResultError("task_id is required: " + err.Error()), nil
	}

	if err := s.stateProvider.DeleteTask(ctx, taskID); err != nil {
		return toolError("failed to delete task", err)
	}
	return jsonResult(map[string]interface{}{"deleted": taskID})
}

// handleStartTask handles the start_task tool.
func (s *Server) handleStartTask(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID, err := request.RequireString("task_id")
	if err != nil {
		return mcp.NewToolResultError("task_id is required: " + err.Error()), nil
	}

	if err := s.stateProvider.StartTask(ctx, taskID); err != nil {
		return toolError("failed to start task", err)
	}
	return jsonResult(map[string]interface{}{"started": taskID})
}

package mcp

// Insights tool and the prompts built on it (plan_my_day, weekly_review).
// Kept in its own file, registered by one call from NewServer, so it merges
// cleanly with other additions to mcp_server.go.

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
	"github.com/xvierd/flow-cli/internal/services"
)

// insightsProvider returns the state provider's insights support, if any.
func (s *Server) insightsProvider() (ports.InsightsProvider, bool) {
	ip, ok := s.stateProvider.(ports.InsightsProvider)
	return ip, ok
}

// registerInsights registers get_insights and the planning/review prompts.
// They need a provider that implements ports.InsightsProvider.
func (s *Server) registerInsights() {
	if _, ok := s.insightsProvider(); !ok {
		return
	}

	s.server.AddTool(
		mcp.NewTool(
			"get_insights",
			mcp.WithDescription("Get 3-5 actionable findings from focus history, ranked by relevance, each with evidence: best time slot and days, when sessions get voided, tags that take more sessions per finished task, optimal session length, streak and trend vs the previous period, dominant Deep Work distractions. 'gaps' lists detectors that need more data and how many sessions they are missing."),
			mcp.WithString(
				"period",
				mcp.Description("Rolling window: week (last 7 days, default) or month (last 30 days)"),
				mcp.Enum("week", "month"),
			),
		),
		s.handleGetInsights,
	)

	s.server.AddPrompt(
		mcp.NewPrompt(
			"plan_my_day",
			mcp.WithPromptDescription("Propose today's focus blocks from Flow insights, pending tasks and today's highlight"),
			mcp.WithArgument("available_hours", mcp.ArgumentDescription("Hours available for focused work today (e.g. 4)")),
		),
		s.handlePlanMyDay,
	)

	s.server.AddPrompt(
		mcp.NewPrompt(
			"weekly_review",
			mcp.WithPromptDescription("Guided weekly reflection grounded in this week's Flow data and insights"),
		),
		s.handleWeeklyReview,
	)
}

// handleGetInsights handles the get_insights tool.
func (s *Server) handleGetInsights(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ip, _ := s.insightsProvider()
	period := request.GetString("period", "week")
	if period != string(services.ReportPeriodWeek) && period != string(services.ReportPeriodMonth) {
		return mcp.NewToolResultError(fmt.Sprintf("invalid period %q: use week or month", period)), nil
	}
	report, err := ip.GetInsights(ctx, period)
	if err != nil {
		return toolError("failed to get insights", err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal insights: %w", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

// handlePlanMyDay builds the plan_my_day prompt.
func (s *Server) handlePlanMyDay(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	s.toolMu.Lock()
	defer s.toolMu.Unlock()

	ip, _ := s.insightsProvider()
	// A month of history gives the planner the most reliable patterns.
	report, err := ip.GetInsights(ctx, string(services.ReportPeriodMonth))
	if err != nil {
		return nil, fmt.Errorf("failed to get insights: %w", err)
	}
	tasks, err := s.stateProvider.ListTasks(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}
	highlight, _ := s.stateProvider.GetTodayHighlight(ctx)

	var b strings.Builder
	b.WriteString("Help me plan today's focus blocks using my Flow data below.\n\n")
	if hours := strings.TrimSpace(request.Params.Arguments["available_hours"]); hours != "" {
		if h, err := strconv.ParseFloat(hours, 64); err == nil && h > 0 {
			fmt.Fprintf(&b, "Available focus time today: %.1f hours.\n", h)
		}
	}
	fmt.Fprintf(&b, "Now: %s.\n\n", time.Now().Format("Monday 2006-01-02 15:04"))

	b.WriteString("## Today's highlight\n")
	if highlight != nil {
		fmt.Fprintf(&b, "- %s (id %s)\n\n", highlight.Title, highlight.ID)
	} else {
		b.WriteString("- None set. Suggest one from the pending tasks.\n\n")
	}

	b.WriteString("## Pending tasks\n")
	pending := 0
	for _, t := range tasks {
		if t.Status != domain.StatusPending && t.Status != domain.StatusInProgress {
			continue
		}
		pending++
		fmt.Fprintf(&b, "- %s (id %s, %s", t.Title, t.ID, t.Status)
		if len(t.Tags) > 0 {
			fmt.Fprintf(&b, ", #%s", strings.Join(t.Tags, " #"))
		}
		b.WriteString(")\n")
	}
	if pending == 0 {
		b.WriteString("- No pending tasks.\n")
	}
	b.WriteString("\n")

	writeInsights(&b, report)

	b.WriteString(`## What I want
Propose a schedule of focus blocks for the rest of today:
1. Put the highlight (or the most important task) in my strongest time slot.
2. Size each block to my optimal session length; avoid the slots or days where I void sessions.
3. Budget more blocks for tags that take more sessions per finished task.
4. Add breaks between blocks and keep it realistic for the time I have.
For each block give the start time, length, task and a one-line intended outcome.
Ground every choice in the insights above; where data is missing, say so instead of guessing.
I can start each block with Flow's start_session tool.
`)

	return mcp.NewGetPromptResult(
		"Plan today's focus blocks",
		[]mcp.PromptMessage{mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(b.String()))},
	), nil
}

// handleWeeklyReview builds the weekly_review prompt.
func (s *Server) handleWeeklyReview(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	s.toolMu.Lock()
	defer s.toolMu.Unlock()

	ip, _ := s.insightsProvider()
	report, err := ip.GetInsights(ctx, string(services.ReportPeriodWeek))
	if err != nil {
		return nil, fmt.Errorf("failed to get insights: %w", err)
	}
	stats, err := s.stateProvider.GetPeriodStats(ctx, report.Start, report.End)
	if err != nil {
		return nil, fmt.Errorf("failed to get period stats: %w", err)
	}

	var b strings.Builder
	b.WriteString("Guide me through a weekly review of my focus, using my Flow data below.\n\n")
	fmt.Fprintf(&b, "## Last 7 days (%s → %s)\n", report.Start.Format("2006-01-02"), report.End.Add(-time.Second).Format("2006-01-02"))
	fmt.Fprintf(&b, "- Completed sessions: %d\n", stats.TotalSessions)
	fmt.Fprintf(&b, "- Focus time: %.1fh\n", stats.TotalWorkTime.Hours())
	if stats.FocusScoreCount > 0 {
		fmt.Fprintf(&b, "- Avg focus score: %.1f/5 (%d rated)\n", stats.AvgFocusScore, stats.FocusScoreCount)
	}
	fmt.Fprintf(&b, "- Distractions logged: %d\n", stats.DistractionCount)
	fmt.Fprintf(&b, "- Sessions analyzed (incl. voided/cancelled): %d\n\n", report.SessionsAnalyzed)

	writeInsights(&b, report)

	b.WriteString(`## How to guide me
Ask one question at a time and wait for my answer:
1. What went well this week? Point at the strongest insight as evidence.
2. What got in the way? Use the void patterns and distractions above.
3. Which pattern do I want to change next week, and what is one concrete experiment?
4. What is next week's single most important outcome, and when will I protect time for it?
Close with a short summary: three takeaways and next week's experiment.
Stay grounded in the data; where it is missing, say how many sessions are needed.
`)

	return mcp.NewGetPromptResult(
		"Weekly focus review",
		[]mcp.PromptMessage{mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(b.String()))},
	), nil
}

// writeInsights appends an insights report as a Markdown section.
func writeInsights(b *strings.Builder, r *domain.InsightsReport) {
	b.WriteString("## Insights from my history\n")
	if len(r.Insights) == 0 {
		b.WriteString("- None yet: not enough history.\n")
	}
	for _, in := range r.Insights {
		fmt.Fprintf(b, "- %s (%s)\n", in.Message, in.Evidence)
	}
	if len(r.Gaps) > 0 {
		b.WriteString("\nNot enough data yet:\n")
		for _, g := range r.Gaps {
			fmt.Fprintf(b, "- %s\n", g.Message)
		}
	}
	b.WriteString("\n")
}

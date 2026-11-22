package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/xvierd/flow-cli/internal/domain"
)

// insightsStateProvider adds ports.InsightsProvider to the base mock.
type insightsStateProvider struct {
	*mockStateProvider
	report     *domain.InsightsReport
	err        error
	lastPeriod string
}

func (m *insightsStateProvider) GetInsights(ctx context.Context, period string) (*domain.InsightsReport, error) {
	m.lastPeriod = period
	if m.err != nil {
		return nil, m.err
	}
	return m.report, nil
}

func newInsightsServer() (*Server, *insightsStateProvider) {
	start := time.Date(2026, 11, 12, 0, 0, 0, 0, time.Local)
	p := &insightsStateProvider{
		mockStateProvider: &mockStateProvider{
			tasks: []*domain.Task{
				{ID: "t1", Title: "Write API docs", Status: domain.StatusPending, Tags: []string{"docs"}},
				{ID: "t2", Title: "Old work", Status: domain.StatusCompleted},
			},
			highlight:   &domain.Task{ID: "t1", Title: "Write API docs"},
			periodStats: &domain.PeriodStats{TotalSessions: 12, TotalWorkTime: 5 * time.Hour},
		},
		report: &domain.InsightsReport{
			Period: "week", Start: start, End: start.AddDate(0, 0, 7), SessionsAnalyzed: 14,
			Insights: []domain.Insight{{
				Kind: domain.InsightBestTime, Message: "Protect 09–12h for your hardest work.",
				Evidence: "60% of your focus minutes", Score: 0.7,
			}},
			Gaps: []domain.InsightGap{{Kind: domain.InsightTagEffort, Missing: 2, Message: "Tag effort: finish 2 more tagged tasks"}},
		},
	}
	return NewServer(p, "test"), p
}

// rpc sends a raw JSON-RPC request and returns the decoded result.
func rpc(t *testing.T, s *Server, method string, params map[string]interface{}, out interface{}) {
	t.Helper()
	msg, err := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	resp := s.server.HandleMessage(context.Background(), msg)
	r, ok := resp.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("%s: unexpected response %T: %+v", method, resp, resp)
	}
	raw, err := json.Marshal(r.Result)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("%s: decode %s: %v", method, raw, err)
	}
}

func TestInsights_NotRegisteredWithoutProvider(t *testing.T) {
	s := newTestServer(&mockStateProvider{})
	var tools mcp.ListToolsResult
	rpc(t, s, "tools/list", map[string]interface{}{}, &tools)
	for _, tool := range tools.Tools {
		if tool.Name == "get_insights" {
			t.Fatal("get_insights registered for a provider without insights support")
		}
	}
}

func TestGetInsightsTool(t *testing.T) {
	s, p := newInsightsServer()

	var tools mcp.ListToolsResult
	rpc(t, s, "tools/list", map[string]interface{}{}, &tools)
	found := false
	for _, tool := range tools.Tools {
		found = found || tool.Name == "get_insights"
	}
	if !found {
		t.Fatal("get_insights not listed")
	}

	result := callTool(t, s, 2, "get_insights", map[string]interface{}{"period": "month"})
	if result.IsError {
		t.Fatalf("get_insights error: %s", resultToText(t, result))
	}
	if p.lastPeriod != "month" {
		t.Errorf("period = %q, want month", p.lastPeriod)
	}
	var got domain.InsightsReport
	if err := json.Unmarshal([]byte(resultToText(t, result)), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Insights) != 1 || got.Insights[0].Kind != domain.InsightBestTime || got.SessionsAnalyzed != 14 {
		t.Errorf("report = %+v", got)
	}
	if len(got.Gaps) != 1 || got.Gaps[0].Missing != 2 {
		t.Errorf("gaps = %+v", got.Gaps)
	}

	callTool(t, s, 3, "get_insights", nil)
	if p.lastPeriod != "week" {
		t.Errorf("default period = %q, want week", p.lastPeriod)
	}
}

func TestGetInsightsTool_Errors(t *testing.T) {
	s, p := newInsightsServer()
	if r := callTool(t, s, 2, "get_insights", map[string]interface{}{"period": "year"}); !r.IsError {
		t.Error("invalid period should be a tool error")
	}
	p.err = errors.New("db locked")
	r := callTool(t, s, 3, "get_insights", nil)
	if !r.IsError || !strings.Contains(resultToText(t, r), "db locked") {
		t.Errorf("provider error should surface as tool error, got %+v", r)
	}
}

func TestInsightPrompts_Listed(t *testing.T) {
	s, _ := newInsightsServer()
	var list mcp.ListPromptsResult
	rpc(t, s, "prompts/list", map[string]interface{}{}, &list)
	names := map[string]mcp.Prompt{}
	for _, p := range list.Prompts {
		names[p.Name] = p
	}
	for _, want := range []string{"plan_my_day", "weekly_review"} {
		if _, ok := names[want]; !ok {
			t.Errorf("prompt %s not listed (got %v)", want, list.Prompts)
		}
	}
	if args := names["plan_my_day"].Arguments; len(args) != 1 || args[0].Name != "available_hours" || args[0].Required {
		t.Errorf("plan_my_day arguments = %+v", args)
	}
}

// promptText fetches a prompt and returns its single user message text.
func promptText(t *testing.T, s *Server, name string, args map[string]string) string {
	t.Helper()
	var res struct {
		Description string `json:"description"`
		Messages    []struct {
			Role    string `json:"role"`
			Content struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	params := map[string]interface{}{"name": name}
	if args != nil {
		params["arguments"] = args
	}
	rpc(t, s, "prompts/get", params, &res)
	if len(res.Messages) != 1 || res.Messages[0].Role != "user" || res.Messages[0].Content.Type != "text" {
		t.Fatalf("%s: messages = %+v", name, res.Messages)
	}
	return res.Messages[0].Content.Text
}

func TestPlanMyDayPrompt(t *testing.T) {
	s, p := newInsightsServer()
	text := promptText(t, s, "plan_my_day", map[string]string{"available_hours": "4"})
	for _, want := range []string{
		"Available focus time today: 4.0 hours",
		"Write API docs (id t1)",                 // highlight
		"Write API docs (id t1, pending, #docs)", // pending task
		"Protect 09–12h for your hardest work. (60% of your focus minutes)",
		"Tag effort: finish 2 more tagged tasks",
		"focus blocks",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("plan_my_day missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Old work") {
		t.Error("completed tasks should not be offered for planning")
	}
	if p.lastPeriod != "month" {
		t.Errorf("plan_my_day should use a month of history, got %q", p.lastPeriod)
	}

	// No highlight and a bad hours argument still give a usable prompt.
	p.highlight = nil
	text = promptText(t, s, "plan_my_day", map[string]string{"available_hours": "lots"})
	if strings.Contains(text, "Available focus time") || !strings.Contains(text, "None set") {
		t.Errorf("fallbacks not applied:\n%s", text)
	}
}

func TestWeeklyReviewPrompt(t *testing.T) {
	s, p := newInsightsServer()
	text := promptText(t, s, "weekly_review", nil)
	for _, want := range []string{
		"2026-11-12 → 2026-11-18",
		"Completed sessions: 12",
		"Focus time: 5.0h",
		"Sessions analyzed (incl. voided/cancelled): 14",
		"Protect 09–12h",
		"one question at a time",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("weekly_review missing %q:\n%s", want, text)
		}
	}
	if p.lastPeriod != "week" {
		t.Errorf("weekly_review period = %q, want week", p.lastPeriod)
	}
}

func TestInsightPrompts_ProviderError(t *testing.T) {
	s, p := newInsightsServer()
	p.err = errors.New("db locked")
	msg, _ := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": "prompts/get", "params": map[string]interface{}{"name": "weekly_review"}})
	if _, ok := s.server.HandleMessage(context.Background(), msg).(mcp.JSONRPCError); !ok {
		t.Error("a provider failure should be a JSON-RPC error")
	}
}

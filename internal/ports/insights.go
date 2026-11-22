package ports

import (
	"context"

	"github.com/xvierd/flow-cli/internal/domain"
)

// InsightsProvider exposes history insights to driving adapters (MCP).
// It is kept apart from MCPStateProvider so providers without insights
// support keep compiling; the MCP server checks for it at startup.
type InsightsProvider interface {
	// GetInsights analyzes the rolling window for period ("week" or "month").
	GetInsights(ctx context.Context, period string) (*domain.InsightsReport, error)
}

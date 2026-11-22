package services

import (
	"context"

	"github.com/xvierd/flow-cli/internal/domain"
)

// GetInsights analyzes the rolling window for period ("week" or "month"),
// implementing ports.InsightsProvider for the MCP server.
func (s *StateService) GetInsights(ctx context.Context, period string) (*domain.InsightsReport, error) {
	if period == "" {
		period = string(ReportPeriodWeek)
	}
	return NewInsightsService(s.storage).GetInsights(ctx, ReportPeriod(period))
}

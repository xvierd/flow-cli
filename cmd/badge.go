package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/services"
	"github.com/xvierd/flow-cli/internal/share"
)

var (
	badgeMonth bool
	badgeYear  bool
	badgeOut   string
	badgeLabel string
)

var badgeCmd = &cobra.Command{
	Use:   "badge",
	Short: "Generate a shields-style SVG badge with your focus time",
	Long: `Generate an SVG badge such as "focus this week | 12h" that you can commit
to a README. Writes to stdout unless --out is given.`,
	Example: `  flow badge -o docs/focus.svg
  flow badge --month --label "deep work" > badge.svg`,
	RunE: runBadge,
}

func init() {
	badgeCmd.Flags().Bool("week", false, "Focus time this week (default)")
	badgeCmd.Flags().BoolVar(&badgeMonth, "month", false, "Focus time this month")
	badgeCmd.Flags().BoolVar(&badgeYear, "year", false, "Focus time this year")
	badgeCmd.Flags().StringVarP(&badgeOut, "out", "o", "", "Write the SVG to this file instead of stdout")
	badgeCmd.Flags().StringVar(&badgeLabel, "label", "", "Custom left-hand label (default: \"focus this week\")")
	badgeCmd.MarkFlagsMutuallyExclusive("week", "month", "year")
}

func runBadge(cmd *cobra.Command, args []string) error {
	period := periodFromFlags(badgeMonth, badgeYear)
	total, err := services.NewWrappedService(app.storage).FocusTotal(context.Background(), period, time.Now())
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to get stats"), err)
	}
	label := badgeLabel
	if label == "" {
		label = badgeDefaultLabel(period)
	}
	color := share.BadgeColorActive
	if total <= 0 {
		color = share.BadgeColorIdle
	}
	svg := share.Badge(label, formatHours(total.Hours()), color)

	if badgeOut == "" {
		_, err := os.Stdout.Write(svg)
		return err
	}
	if err := os.WriteFile(badgeOut, svg, 0o644); err != nil { // #nosec G306 -- the badge is meant to be committed and world-readable
		return fmt.Errorf("%s: %w", i18n.T("failed to create output file"), err)
	}
	if jsonOutput {
		return jsonOut(map[string]interface{}{"path": badgeOut, "label": label, "focus": total.String()})
	}
	fmt.Println(i18n.T("Badge written to %s", badgeOut))
	return nil
}

func badgeDefaultLabel(p services.ReportPeriod) string {
	switch p {
	case services.ReportPeriodYear:
		return i18n.T("focus this year")
	case services.ReportPeriodMonth:
		return i18n.T("focus this month")
	default:
		return i18n.T("focus this week")
	}
}

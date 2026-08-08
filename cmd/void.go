package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/i18n"
)

var voidCmd = &cobra.Command{
	Use:   "void",
	Short: "Void (invalidate) the current session due to interruption",
	Long: `Mark the current session as interrupted. Interrupted sessions are not
counted in productivity stats — use this when you were significantly
disrupted and the session no longer represents focused work.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		session, err := app.pomodoro.VoidSession(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to void session"), err)
		}

		fmt.Printf("%s\n", i18n.T("Session voided. Duration: %s (not counted in stats)", session.Duration))
		return nil
	},
}

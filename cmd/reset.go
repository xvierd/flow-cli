package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/i18n"
)

var resetForce bool

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Delete all sessions and tasks (wipes the database)",
	Long: `Permanently deletes the Flow database, removing all sessions, tasks, and history.
This cannot be undone. Use --force to skip the confirmation prompt.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		appCfg, err := config.Load()
		if err != nil {
			appCfg = config.DefaultConfig()
		}

		// Allow --db flag to override path
		path := dbPath
		if path == "" {
			path = config.GetDBPath(appCfg)
		}

		if !resetForce {
			fmt.Printf("%s\n", i18n.T("This will permanently delete: %s", path))
			fmt.Print(i18n.T("Are you sure? Type 'yes' to confirm: "))
			reader := bufio.NewReader(os.Stdin)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(strings.ToLower(input))
			if input != "yes" {
				fmt.Println(i18n.T("Aborted."))
				return nil
			}
		}

		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				fmt.Println(i18n.T("Nothing to reset — database does not exist."))
				return nil
			}
			return fmt.Errorf("%s: %w", i18n.T("failed to delete database"), err)
		}

		fmt.Println(i18n.T("Database deleted. Fresh start."))
		return nil
	},
}

func init() {
	resetCmd.Flags().BoolVarP(&resetForce, "force", "f", false, "Skip confirmation prompt")
}

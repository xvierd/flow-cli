package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/config"
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
			fmt.Printf("This will permanently delete: %s\n", path)
			fmt.Print("Are you sure? Type 'yes' to confirm: ")
			reader := bufio.NewReader(os.Stdin)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(strings.ToLower(input))
			if input != "yes" {
				fmt.Println("Aborted.")
				return nil
			}
		}

		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				fmt.Println("Nothing to reset — database does not exist.")
				return nil
			}
			return fmt.Errorf("failed to delete database: %w", err)
		}

		fmt.Println("Database deleted. Fresh start.")
		return nil
	},
}

func init() {
	resetCmd.Flags().BoolVarP(&resetForce, "force", "f", false, "Skip confirmation prompt")
}

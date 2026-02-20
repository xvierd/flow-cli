// Package cmd provides the CLI commands for the Flow application.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Version info (set at build time via ldflags)
	Version   = "dev"
	BuildDate = "unknown"
	GitCommit = "unknown"

	// Global flags
	dbPath     string
	jsonOutput bool
	inlineMode bool
	modeFlag   string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "flow",
	Short: "Flow - A Pomodoro CLI timer with task tracking",
	Long: `Flow is a command-line Pomodoro timer that helps you stay focused
and track your work sessions with optional git integration.

Run "flow" with no arguments to start a quick session interactively.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return initializeServices()
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		return cleanupServices()
	},
	RunE: runWizard,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "Path to the database file (default: ~/.flow/flow.db)")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output results in JSON format")
	rootCmd.PersistentFlags().BoolVarP(&inlineMode, "inline", "i", false, "Compact inline timer (no fullscreen)")
	rootCmd.PersistentFlags().StringVar(&modeFlag, "mode", "", "Productivity methodology: pomodoro, deepwork, maketime")

	// Set version - cobra handles --version automatically
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("Flow CLI\nVersion: {{.Version}}\n")

	// Add subcommands
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(breakCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(pauseCmd)
	rootCmd.AddCommand(resumeCmd)
	rootCmd.AddCommand(completeCmd)
	rootCmd.AddCommand(resetCmd)
	rootCmd.AddCommand(voidCmd)
}

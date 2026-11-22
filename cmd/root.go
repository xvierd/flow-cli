// Package cmd provides the CLI commands for the Flow application.
package cmd

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/i18n"
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
	strictFlag bool
)

// rootCmd represents the base command when called without any subcommands.
// NOTE: cobra metadata (Use/Short/Long) and flag descriptions are registered
// at init time, before the display language is resolved, so they stay English.
var rootCmd = &cobra.Command{
	Use:   "flow",
	Short: "Flow - focus sessions (Pomodoro, Deep Work, Make Time) from your terminal",
	Long: `Flow runs focus sessions from your terminal with three methodologies:
Pomodoro, Deep Work and Make Time. It tracks tasks, knows your git branch,
keeps everything in a local SQLite database, and exposes an MCP server so AI
agents can read and drive your focus state.

Run "flow" with no arguments to start a session interactively.
Task and session IDs can be shortened to the 8 characters "flow list" shows.`,
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

// resolveVersion returns the version injected at build time or, for binaries
// built with `go install module@version` (no ldflags), the module version
// recorded by the Go toolchain.
func resolveVersion(injected string) string {
	if injected != "dev" {
		return injected
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return injected
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.T("Error: %v", err))
		os.Exit(1)
	}
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "Path to the database file (default: ~/.flow/flow.db)")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output results in JSON format")
	rootCmd.PersistentFlags().BoolVarP(&inlineMode, "inline", "i", false, "Compact inline timer (no fullscreen)")
	rootCmd.PersistentFlags().StringVar(&modeFlag, "mode", "", "Productivity methodology: pomodoro, deepwork, maketime")
	rootCmd.PersistentFlags().BoolVar(&strictFlag, "strict", false, "Strict focus mode: lock pause/finish/void/cancel during work sessions (overrides config)")

	// Set version - cobra handles --version automatically
	Version = resolveVersion(Version)
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
	rootCmd.AddCommand(reportCmd)
	rootCmd.AddCommand(reflectCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(wrappedCmd)
	rootCmd.AddCommand(badgeCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(editCmd)
	rootCmd.AddCommand(watchCmd)
}

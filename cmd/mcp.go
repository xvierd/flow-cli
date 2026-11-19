package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/adapters/mcp"
	"github.com/xvierd/flow-cli/internal/i18n"
)

// mcpCmd represents the mcp command
var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the MCP server",
	Long: `Start the Model Context Protocol (MCP) server for integration with AI assistants.
The server speaks JSON-RPC over stdio and exposes tools to query Flow state and
history, manage tasks, and control sessions (start, pause, resume, stop, breaks).
Diagnostics are written to stderr; stdout carries only protocol messages.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// stdout is the JSON-RPC channel: anything human-readable must go to
		// stderr or it corrupts the protocol stream for the client.
		fmt.Fprintln(os.Stderr, i18n.T("🚀 Starting MCP server..."))
		fmt.Fprintln(os.Stderr, i18n.T("   The server will communicate via stdio"))
		fmt.Fprintln(os.Stderr, i18n.T("   Press Ctrl+C to stop"))

		ctx := context.Background()

		// Create and start the MCP server
		server := mcp.NewServer(app.state, Version)
		if err := server.Start(ctx); err != nil {
			return fmt.Errorf("%s: %w", i18n.T("MCP server error"), err)
		}

		return nil
	},
}

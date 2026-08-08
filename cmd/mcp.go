package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/adapters/mcp"
	"github.com/xvierd/flow-cli/internal/i18n"
)

// mcpCmd represents the mcp command
var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the MCP server",
	Long: `Start the Model Context Protocol (MCP) server for integration with AI assistants.
The server provides tools for querying Flow state and task history.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(i18n.T("🚀 Starting MCP server..."))
		fmt.Println(i18n.T("   The server will communicate via stdio"))
		fmt.Println(i18n.T("   Press Ctrl+C to stop"))

		ctx := context.Background()

		// Create and start the MCP server
		server := mcp.NewServer(app.state, Version)
		if err := server.Start(ctx); err != nil {
			return fmt.Errorf("%s: %w", i18n.T("MCP server error"), err)
		}

		return nil
	},
}

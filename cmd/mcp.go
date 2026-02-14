package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/dvidx/flow-cli/internal/adapters/mcp"
)

// mcpCmd represents the mcp command
var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the MCP server",
	Long: `Start the Model Context Protocol (MCP) server for integration with AI assistants.
The server provides tools for querying Flow state and task history.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("🚀 Starting MCP server...")
		fmt.Println("   The server will communicate via stdio")
		fmt.Println("   Press Ctrl+C to stop")

		ctx := context.Background()

		// Create and start the MCP server
		server := mcp.NewServer(stateService)
		if err := server.Start(ctx); err != nil {
			return fmt.Errorf("MCP server error: %w", err)
		}

		return nil
	},
}

package cmd

import (
	mcpserver "github.com/leona/kb/mcp"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server (stdio transport)",
	Long:  "Starts the Model Context Protocol server for AI coding agent integration. Communicates via stdin/stdout.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return mcpserver.Serve()
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

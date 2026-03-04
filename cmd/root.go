package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "kb",
	Short: "Knowledge base manager for AI coding assistants",
	Long:  "Manage markdown knowledge bases across projects with versioning, shared docs, and AI coding agent MCP integration.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return browseCmd.RunE(browseCmd, args)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

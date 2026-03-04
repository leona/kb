package cmd

import (
	"fmt"

	"github.com/leona/kb/internal/config"
	"github.com/leona/kb/internal/graph"
	"github.com/spf13/cobra"
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Show knowledge base reference graph",
	Long:  "Render an ASCII graph showing connections between projects and shared docs.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()

		g, err := graph.Build(kbRoot)
		if err != nil {
			return err
		}

		fmt.Print(graph.Render(g))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(graphCmd)
}

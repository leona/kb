package cmd

import (
	"fmt"

	"github.com/leona/kb/internal/config"
	"github.com/leona/kb/internal/git"
	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:   "log [scope]",
	Short: "Show version history",
	Long:  "Show git log. Optionally scope to a project name or shared doc slug.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()

		scopePath, err := resolveScopePath(kbRoot, args)
		if err != nil {
			return err
		}

		entries, err := git.Log(kbRoot, scopePath, 50)
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			fmt.Println("No history found.")
			return nil
		}

		for _, e := range entries {
			fmt.Println(e)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logCmd)
}

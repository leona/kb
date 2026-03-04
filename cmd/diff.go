package cmd

import (
	"fmt"

	"github.com/leona/kb/internal/config"
	"github.com/leona/kb/internal/git"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff [scope]",
	Short: "Show uncommitted changes",
	Long:  "Show uncommitted changes. Optionally scope to a project name or shared doc slug.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()

		scopePath, err := resolveScopePath(kbRoot, args)
		if err != nil {
			return err
		}

		output, err := git.Diff(kbRoot, scopePath)
		if err != nil {
			return err
		}

		fmt.Println(output)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(diffCmd)
}

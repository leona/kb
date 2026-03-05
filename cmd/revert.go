package cmd

import (
	"fmt"

	"github.com/leona/kb/internal/config"
	"github.com/leona/kb/internal/git"
	"github.com/spf13/cobra"
)

var revertCmd = &cobra.Command{
	Use:   "revert <ref> <path>",
	Short: "Restore a file to a previous version",
	Long:  "Restores a file from a previous git commit. Path is relative to KB root (e.g., projects/myapp/context.md).",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()
		ref := args[0]
		filePath := args[1]

		if err := git.RevertFile(kbRoot, ref, filePath); err != nil {
			return err
		}
		if err := git.CommitAndPush(kbRoot, fmt.Sprintf("revert: %s to %s", filePath, ref)); err != nil {
			return err
		}

		fmt.Printf("Reverted %s to %s\n", filePath, ref)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(revertCmd)
}

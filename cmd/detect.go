package cmd

import (
	"fmt"

	"github.com/leona/kb/internal/config"
	"github.com/leona/kb/internal/project"
	"github.com/spf13/cobra"
)

var detectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Auto-detect current project from working directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()

		name, err := project.Detect(kbRoot)
		if err != nil {
			return err
		}

		fmt.Println(name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(detectCmd)
}

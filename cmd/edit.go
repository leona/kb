package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/leona/kb/internal/config"
	"github.com/leona/kb/internal/fs"
	"github.com/leona/kb/internal/project"
	"github.com/spf13/cobra"
)

var editCmd = &cobra.Command{
	Use:   "edit <project>",
	Short: "Open project context.md in editor",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()
		name := args[0]

		contextPath := project.ContextPath(kbRoot, name)
		if !fs.FileExists(contextPath) {
			return fmt.Errorf("project %q not found or has no context.md", name)
		}

		cfg, err := config.Load(kbRoot)
		if err != nil {
			return err
		}

		editor := cfg.GetEditor()
		editorCmd := exec.Command(editor, contextPath)
		editorCmd.Stdin = os.Stdin
		editorCmd.Stdout = os.Stdout
		editorCmd.Stderr = os.Stderr

		return editorCmd.Run()
	},
}

func init() {
	rootCmd.AddCommand(editCmd)
}

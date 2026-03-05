package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leona/kb/internal/config"
	"github.com/leona/kb/internal/git"
	"github.com/leona/kb/internal/project"
	"github.com/leona/kb/internal/tui"
	"github.com/spf13/cobra"
)

var browseCmd = &cobra.Command{
	Use:   "browse",
	Short: "Interactive TUI browser for the knowledge base",
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()

		// Auto-detect current project from cwd.
		projectName, _ := project.Detect(kbRoot)

		m := tui.New(kbRoot, projectName)
		p := tea.NewProgram(m, tea.WithAltScreen())
		finalModel, err := p.Run()
		git.FlushDebounce()
		if err != nil {
			return fmt.Errorf("running TUI: %w", err)
		}

		// If the model exited with an error, report it.
		if fm, ok := finalModel.(tui.ErrorReporter); ok {
			if e := fm.Err(); e != nil {
				fmt.Fprintln(os.Stderr, e)
				os.Exit(1)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(browseCmd)
}

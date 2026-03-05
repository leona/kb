package cmd

import (
	"errors"
	"fmt"

	"github.com/leona/kb/internal/config"
	"github.com/leona/kb/internal/git"
	"github.com/leona/kb/internal/project"
	"github.com/leona/kb/internal/shared"
	"github.com/spf13/cobra"
)

var refCmd = &cobra.Command{
	Use:   "ref",
	Short: "Manage project references to shared docs",
}

var refAddCmd = &cobra.Command{
	Use:   "add <project> <shared-slug>",
	Short: "Link a project to a shared document",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()
		projectName := args[0]
		slug := args[1]
		inline, _ := cmd.Flags().GetBool("inline")

		if !project.Exists(kbRoot, projectName) {
			return fmt.Errorf("project %q not found", projectName)
		}
		if !shared.Exists(kbRoot, slug) {
			return fmt.Errorf("shared doc %q not found", slug)
		}

		if err := project.AddRef(kbRoot, projectName, slug, inline); errors.Is(err, project.ErrAlreadyLinked) {
			fmt.Printf("%s already linked to %s\n", projectName, slug)
			return nil
		} else if err != nil {
			return err
		}

		linkType := "ref"
		if inline {
			linkType = "inline"
		}
		if err := git.CommitAndPush(kbRoot, fmt.Sprintf("ref: link %s → %s (%s)", projectName, slug, linkType)); err != nil {
			return err
		}

		fmt.Printf("Linked %s → %s (%s)\n", projectName, slug, linkType)
		return nil
	},
}

var refRemoveCmd = &cobra.Command{
	Use:   "remove <project> <shared-slug>",
	Short: "Unlink a shared document from a project",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()
		projectName := args[0]
		slug := args[1]

		if err := project.RemoveRef(kbRoot, projectName, slug); errors.Is(err, project.ErrNotLinked) {
			fmt.Printf("%s is not linked to %s\n", projectName, slug)
			return nil
		} else if err != nil {
			return err
		}

		if err := git.CommitAndPush(kbRoot, fmt.Sprintf("ref: unlink %s → %s", projectName, slug)); err != nil {
			return err
		}

		fmt.Printf("Unlinked %s → %s\n", projectName, slug)
		return nil
	},
}

func init() {
	refAddCmd.Flags().Bool("inline", false, "Inline the shared doc content directly into context.md")
	refCmd.AddCommand(refAddCmd)
	refCmd.AddCommand(refRemoveCmd)
	rootCmd.AddCommand(refCmd)
}

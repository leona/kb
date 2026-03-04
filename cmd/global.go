package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/leona/kb/internal/config"
	"github.com/leona/kb/internal/git"
	"github.com/leona/kb/internal/project"
	"github.com/leona/kb/internal/shared"
	"github.com/spf13/cobra"
)

var globalCmd = &cobra.Command{
	Use:   "global",
	Short: "Manage globally shared docs (available to all projects)",
}

var globalAddCmd = &cobra.Command{
	Use:   "add <shared-slug>",
	Short: "Make a shared doc globally available to all projects",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()
		slug := args[0]
		inline, _ := cmd.Flags().GetBool("inline")

		if !shared.Exists(kbRoot, slug) {
			return fmt.Errorf("shared doc %q not found", slug)
		}

		if err := project.AddGlobal(kbRoot, slug, inline); errors.Is(err, project.ErrAlreadyGlobal) {
			fmt.Printf("%s is already global\n", slug)
			return nil
		} else if err != nil {
			return err
		}

		linkType := "ref"
		if inline {
			linkType = "inline"
		}
		if err := git.AutoCommit(kbRoot, fmt.Sprintf("global: add %s (%s)", slug, linkType)); err != nil {
			return err
		}

		fmt.Printf("Added %s as a global shared doc (%s)\n", slug, linkType)
		return nil
	},
}

var globalRemoveCmd = &cobra.Command{
	Use:   "remove <shared-slug>",
	Short: "Remove a shared doc from globals",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()
		slug := args[0]

		if err := project.RemoveGlobal(kbRoot, slug); errors.Is(err, project.ErrNotGlobal) {
			fmt.Printf("%s is not a global shared doc\n", slug)
			return nil
		} else if err != nil {
			return err
		}

		if err := git.AutoCommit(kbRoot, fmt.Sprintf("global: remove %s", slug)); err != nil {
			return err
		}

		fmt.Printf("Removed %s from globals\n", slug)
		return nil
	},
}

var globalListCmd = &cobra.Command{
	Use:   "list",
	Short: "List global shared docs",
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()

		cfg, err := config.Load(kbRoot)
		if err != nil {
			return err
		}

		if len(cfg.Globals) == 0 && len(cfg.InlineGlobals) == 0 {
			fmt.Println("No global shared docs configured.")
			fmt.Println("Use 'kb global add <slug>' to make a shared doc available to all projects.")
			return nil
		}

		fmt.Println("Global shared docs:")
		for _, slug := range cfg.Globals {
			info, err := shared.Get(kbRoot, slug)
			if err != nil {
				fmt.Printf("  %s (not found)\n", slug)
				continue
			}
			fmt.Printf("  %-25s %4d lines  %d files  used by: %s\n",
				info.DisplayTitle(), info.TotalLines, len(info.Files), strings.Join(info.UsedBy, ", "))
		}
		for _, slug := range cfg.InlineGlobals {
			info, err := shared.Get(kbRoot, slug)
			if err != nil {
				fmt.Printf("  %s (inline, not found)\n", slug)
				continue
			}
			fmt.Printf("  %-25s %4d lines  %d files  (inline)  used by: %s\n",
				info.DisplayTitle(), info.TotalLines, len(info.Files), strings.Join(info.UsedBy, ", "))
		}
		return nil
	},
}

func init() {
	globalAddCmd.Flags().Bool("inline", false, "Inline the shared doc content directly into all projects' context.md")
	globalCmd.AddCommand(globalAddCmd)
	globalCmd.AddCommand(globalRemoveCmd)
	globalCmd.AddCommand(globalListCmd)
	rootCmd.AddCommand(globalCmd)
}

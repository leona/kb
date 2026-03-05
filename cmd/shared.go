package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/leona/kb/internal/config"
	"github.com/leona/kb/internal/fs"
	"github.com/leona/kb/internal/git"
	"github.com/leona/kb/internal/shared"
	"github.com/spf13/cobra"
)

var sharedCmd = &cobra.Command{
	Use:   "shared",
	Short: "Manage shared documents",
}

var sharedAddCmd = &cobra.Command{
	Use:   "add <slug> <file...>",
	Short: "Add shared document(s)",
	Long:  "Copies file(s) into shared/<slug>/ for cross-project reference.",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()
		slug := args[0]
		files := args[1:]

		dir := shared.Dir(kbRoot, slug)
		if err := fs.EnsureDir(dir); err != nil {
			return err
		}

		totalLines := 0
		for _, src := range files {
			absSrc, err := filepath.Abs(src)
			if err != nil {
				return fmt.Errorf("resolving path %s: %w", src, err)
			}
			if !fs.FileExists(absSrc) {
				return fmt.Errorf("file not found: %s", absSrc)
			}

			dst := filepath.Join(dir, filepath.Base(absSrc))
			if err := fs.CopyFile(absSrc, dst); err != nil {
				return fmt.Errorf("copying %s: %w", src, err)
			}

			lines, _ := fs.CountLines(dst)
			totalLines += lines
		}

		if err := git.CommitAndPush(kbRoot, fmt.Sprintf("shared: add %s (%d file(s), %d lines)", slug, len(files), totalLines)); err != nil {
			return err
		}

		fmt.Printf("Added shared doc: %s (%d file(s), %d lines)\n", slug, len(files), totalLines)
		return nil
	},
}

var sharedListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all shared documents",
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()

		docs, err := shared.List(kbRoot)
		if err != nil {
			return err
		}

		if len(docs) == 0 {
			fmt.Println("No shared documents found.")
			return nil
		}

		for _, d := range docs {
			usedBy := ""
			if len(d.UsedBy) > 0 {
				usedBy = fmt.Sprintf("  used by: %s", strings.Join(d.UsedBy, ", "))
			}
			title := d.Slug
			if d.Title != "" {
				title = d.Title
			}
			fmt.Printf("%-25s %2d file(s) %6d lines%s\n", title, len(d.Files), d.TotalLines, usedBy)
		}
		return nil
	},
}

func init() {
	sharedCmd.AddCommand(sharedAddCmd)
	sharedCmd.AddCommand(sharedListCmd)
	rootCmd.AddCommand(sharedCmd)
}

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/leona/kb/internal/config"
	"github.com/leona/kb/internal/fs"
	"github.com/leona/kb/internal/git"
	"github.com/spf13/cobra"
)

var initDir string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new knowledge base",
	Long:  "Creates the knowledge base directory structure with kb.yml, shared/, projects/, and initializes git.",
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := initDir
		if kbRoot == "" {
			kbRoot = config.DefaultKBRoot()
		}

		if fs.FileExists(filepath.Join(kbRoot, "kb.yml")) {
			fmt.Printf("Knowledge base already initialized at %s\n", kbRoot)
			return nil
		}

		// Create directory structure
		for _, dir := range []string{
			kbRoot,
			filepath.Join(kbRoot, "shared"),
			filepath.Join(kbRoot, "projects"),
		} {
			if err := fs.EnsureDir(dir); err != nil {
				return fmt.Errorf("creating directory %s: %w", dir, err)
			}
		}

		// Write default kb.yml
		cfg := &config.Config{
			Version:  1,
			Projects: make(map[string]string),
		}

		// Try to detect editor from $EDITOR
		if editor := os.Getenv("EDITOR"); editor != "" {
			cfg.Editor = editor
		}

		if err := config.Save(kbRoot, cfg); err != nil {
			return fmt.Errorf("writing kb.yml: %w", err)
		}

		// Initialize git repo
		if err := git.Init(kbRoot); err != nil {
			return fmt.Errorf("initializing git: %w", err)
		}

		// Initial commit
		if err := git.AutoCommit(kbRoot, "init: knowledge base"); err != nil {
			return fmt.Errorf("initial commit: %w", err)
		}

		fmt.Printf("Initialized knowledge base at %s\n", kbRoot)
		return nil
	},
}

func init() {
	initCmd.Flags().StringVar(&initDir, "dir", "", "Knowledge base directory (default: ~/knowledge-base)")
	rootCmd.AddCommand(initCmd)
}

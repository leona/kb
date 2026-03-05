package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

		alreadyExists := fs.FileExists(filepath.Join(kbRoot, "kb.yml"))

		var cfg *config.Config

		if alreadyExists {
			var err error
			cfg, err = config.Load(kbRoot)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			fmt.Printf("Knowledge base already initialized at %s\n", kbRoot)
		} else {
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

			cfg = &config.Config{
				Version:  1,
				Projects: make(map[string]string),
			}

			if editor := os.Getenv("EDITOR"); editor != "" {
				cfg.Editor = editor
			}

			if err := config.Save(kbRoot, cfg); err != nil {
				return fmt.Errorf("writing kb.yml: %w", err)
			}

			if err := git.Init(kbRoot); err != nil {
				return fmt.Errorf("initializing git: %w", err)
			}

			if err := git.AutoCommit(kbRoot, "init: knowledge base"); err != nil {
				return fmt.Errorf("initial commit: %w", err)
			}

			fmt.Printf("Initialized knowledge base at %s\n", kbRoot)
		}

		// Remote setup
		hasRemote := git.HasRemote(kbRoot)
		reader := bufio.NewReader(os.Stdin)

		if hasRemote && cfg.AutoPush {
			fmt.Println("Remote and auto-push already configured.")
		} else {
			if !hasRemote {
				fmt.Print("Set up a git remote for backup? [y/N] ")
				answer, _ := reader.ReadString('\n')
				answer = strings.TrimSpace(strings.ToLower(answer))

				if answer == "y" || answer == "yes" {
					fmt.Print("Remote URL (e.g. git@github.com:user/knowledge-base.git): ")
					url, _ := reader.ReadString('\n')
					url = strings.TrimSpace(url)

					if url != "" {
						if err := git.AddRemote(kbRoot, url); err != nil {
							return fmt.Errorf("adding remote: %w", err)
						}
						fmt.Printf("Remote set to %s\n", url)
						hasRemote = true
					}
				}
			}

			if hasRemote && !cfg.AutoPush {
				fmt.Print("Enable auto-push after every commit? [Y/n] ")
				pushAnswer, _ := reader.ReadString('\n')
				pushAnswer = strings.TrimSpace(strings.ToLower(pushAnswer))

				if pushAnswer == "" || pushAnswer == "y" || pushAnswer == "yes" {
					cfg.AutoPush = true
					if err := config.Save(kbRoot, cfg); err != nil {
						return fmt.Errorf("saving config: %w", err)
					}
					if err := git.AutoCommit(kbRoot, "config: enable auto-push"); err != nil {
						return fmt.Errorf("committing config: %w", err)
					}
					fmt.Println("Auto-push enabled.")

					fmt.Println("Pushing to remote...")
					if err := git.PushWithUpstream(kbRoot); err != nil {
						return fmt.Errorf("initial push: %w", err)
					}
				}
			}
		}

		return nil
	},
}

func init() {
	initCmd.Flags().StringVar(&initDir, "dir", "", "Knowledge base directory (default: ~/knowledge-base)")
	rootCmd.AddCommand(initCmd)
}

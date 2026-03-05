package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/leona/kb/internal/config"
	"github.com/leona/kb/internal/fs"
	"github.com/leona/kb/internal/git"
	"github.com/leona/kb/internal/project"
	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage project knowledge",
}

var projectInitCmd = &cobra.Command{
	Use:   "init <name>",
	Short: "Create a project knowledge directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()
		name := args[0]

		if project.Exists(kbRoot, name) {
			return fmt.Errorf("project %q already exists", name)
		}

		projectPath, _ := cmd.Flags().GetString("path")
		if _, err := project.Register(kbRoot, name, projectPath); err != nil {
			return err
		}

		if err := git.CommitAndPush(kbRoot, fmt.Sprintf("project: init %s", name)); err != nil {
			return err
		}

		fmt.Printf("Created project %q at %s\n", name, project.Dir(kbRoot, name))
		return nil
	},
}

var projectImportCmd = &cobra.Command{
	Use:   "import <name>",
	Short: "Import existing instruction files into the knowledge base",
	Long:  "Copies CLAUDE.md and/or AGENTS.md content into context.md. Replaces the repo's CLAUDE.md with an @import pointer (AGENTS.md is preserved).",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()
		name := args[0]

		fromPath, _ := cmd.Flags().GetString("from")
		if fromPath == "" {
			return fmt.Errorf("--from flag is required (path to project directory or instruction file)")
		}

		fromPath, _ = filepath.Abs(fromPath)
		info, err := os.Stat(fromPath)
		if err != nil {
			return fmt.Errorf("cannot access %s: %w", fromPath, err)
		}

		var projectDir string
		var claudePath, agentsPath string
		var content string

		if info.IsDir() {
			projectDir = fromPath
			claudeCandidate := filepath.Join(fromPath, "CLAUDE.md")
			agentsCandidate := filepath.Join(fromPath, "AGENTS.md")
			agentCandidate := filepath.Join(fromPath, "AGENT.md")

			if fs.FileExists(claudeCandidate) {
				claudePath = claudeCandidate
			}
			if fs.FileExists(agentsCandidate) {
				agentsPath = agentsCandidate
			} else if fs.FileExists(agentCandidate) {
				agentsPath = agentCandidate
			}

			if claudePath == "" && agentsPath == "" {
				return fmt.Errorf("no CLAUDE.md, AGENT.md, or AGENTS.md found in %s", fromPath)
			}

			// Read and concatenate content
			var parts []string
			if claudePath != "" {
				c, err := fs.ReadFile(claudePath)
				if err != nil {
					return fmt.Errorf("reading CLAUDE.md: %w", err)
				}
				parts = append(parts, c)
			}
			if agentsPath != "" {
				c, err := fs.ReadFile(agentsPath)
				if err != nil {
					return fmt.Errorf("reading %s: %w", filepath.Base(agentsPath), err)
				}
				parts = append(parts, c)
			}
			content = strings.Join(parts, "\n")
		} else {
			projectDir = filepath.Dir(fromPath)
			base := filepath.Base(fromPath)
			if base == "AGENTS.md" || base == "AGENT.md" {
				agentsPath = fromPath
			} else {
				claudePath = fromPath
			}
			c, err := fs.ReadFile(fromPath)
			if err != nil {
				return fmt.Errorf("reading %s: %w", fromPath, err)
			}
			content = c
		}

		// Register project (creates dir, context.md, refs.yml, kb.yml entry)
		if _, err := project.Register(kbRoot, name, projectDir); err != nil {
			return err
		}

		// Overwrite context.md with imported content
		kbRelPath := project.KBRelPath(kbRoot, name)
		directive := fmt.Sprintf("<!-- KB managed: %s/context.md -->\n<!-- Always edit THIS file for project context. Do NOT edit the repo's CLAUDE.md. -->\n\n", kbRelPath)
		if err := os.WriteFile(project.ContextPath(kbRoot, name), []byte(directive+content), 0644); err != nil {
			return err
		}

		// Build commit message
		var sources []string
		if claudePath != "" {
			sources = append(sources, claudePath)
		}
		if agentsPath != "" {
			sources = append(sources, agentsPath)
		}
		if err := git.CommitAndPush(kbRoot, fmt.Sprintf("import: %s from %s", name, strings.Join(sources, ", "))); err != nil {
			return err
		}

		// Replace repo instruction files with @import pointers
		written, err := project.WriteImportPointers(projectDir, kbRoot, name)
		if err != nil {
			return err
		}
		for _, f := range written {
			fmt.Printf("Replaced %s with @import pointer\n", filepath.Join(projectDir, f))
		}

		fmt.Printf("Imported → %s/context.md\n", kbRelPath)
		if agentsPath != "" {
			fmt.Printf("%s content included (original preserved in context.md)\n", filepath.Base(agentsPath))
		}
		return nil
	},
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()

		projects, err := project.List(kbRoot)
		if err != nil {
			return err
		}

		if len(projects) == 0 {
			fmt.Println("No projects found.")
			return nil
		}

		for _, p := range projects {
			refInfo := ""
			if len(p.Refs) > 0 {
				refInfo = fmt.Sprintf("  refs: %s", strings.Join(p.Refs, ", "))
			}
			fmt.Printf("%-25s %4d lines%s\n", p.Name, p.ContextLines, refInfo)
		}
		return nil
	},
}

var projectShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show project details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()
		name := args[0]

		info, err := project.Get(kbRoot, name)
		if err != nil {
			return err
		}

		fmt.Printf("Project: %s\n", info.Name)
		fmt.Printf("Context: %d lines\n", info.ContextLines)

		if len(info.Refs) > 0 {
			fmt.Printf("Refs:    %s\n", strings.Join(info.Refs, ", "))
		}

		if len(info.Files) > 0 {
			fmt.Printf("Files:   %s\n", strings.Join(info.Files, ", "))
		}

		// Print context.md content
		contextPath := project.ContextPath(kbRoot, name)
		if fs.FileExists(contextPath) {
			content, err := fs.ReadFile(contextPath)
			if err == nil {
				fmt.Printf("\n--- context.md ---\n%s\n", content)
			}
		}

		return nil
	},
}

func init() {
	projectInitCmd.Flags().String("path", "", "Project directory path on disk")
	projectImportCmd.Flags().String("from", "", "Path to project directory or instruction file")
	projectCmd.AddCommand(projectInitCmd)
	projectCmd.AddCommand(projectImportCmd)
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectShowCmd)
	rootCmd.AddCommand(projectCmd)
}

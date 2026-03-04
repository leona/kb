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
	"github.com/leona/kb/internal/project"
	"github.com/spf13/cobra"
)

const slashCommandContent = `Set up the KB knowledge base MCP server for this project.

Steps:
1. Run ` + "`kb detect`" + ` to check if this project is already in the knowledge base
2. If not detected, ask the user for the project name (suggest the directory name) and run ` + "`kb project import <name> --from <cwd>`" + `
3. Run ` + "`kb setup`" + ` to configure the KB MCP server for this project
4. Tell the user to restart their editor to connect to the KB MCP server

If any step fails, show the error and suggest how to fix it.
`

var setupCmd = &cobra.Command{
	Use:   "setup [project]",
	Short: "Configure MCP server for a project",
	Long:  "Detects installed AI coding agents and creates per-project MCP config for each.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()

		// Ensure KB is initialized
		if !fs.DirExists(kbRoot) {
			return fmt.Errorf("KB not initialized; run 'kb init' first")
		}

		cfg, err := config.Load(kbRoot)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting cwd: %w", err)
		}

		// Determine project name and path
		var projectName, projectPath string
		if len(args) > 0 {
			projectName = args[0]
			projectPath = cfg.Projects[projectName]
			if projectPath == "" {
				projectPath = cwd
			}
		} else {
			// Try to detect from cwd
			projectName = cfg.DetectProject(cwd)
			if projectName == "" {
				// Not registered — use directory name
				projectName = filepath.Base(cwd)
			}
			projectPath = cwd
		}

		absPath, err := filepath.Abs(projectPath)
		if err != nil {
			return fmt.Errorf("resolving path: %w", err)
		}

		kbBinary := "kb"

		// Detect installed agents
		agents := detectAgents()
		if len(agents) == 0 {
			return fmt.Errorf("no supported AI coding agents detected")
		}

		// Build list of config files to write
		type configTarget struct {
			agent agent
			path  string
		}
		var targets []configTarget

		for _, a := range agents {
			switch a.Kind {
			case agentClaude:
				targets = append(targets, configTarget{
					agent: a,
					path:  filepath.Join(absPath, ".mcp.json"),
				})
			case agentCodex:
				targets = append(targets, configTarget{
					agent: a,
					path:  filepath.Join(absPath, ".codex", "config.toml"),
				})
			case agentOpenCode:
				targets = append(targets, configTarget{
					agent: a,
					path:  filepath.Join(absPath, "opencode.json"),
				})
			}
		}

		// Show what will happen and confirm
		needsRegister := !project.Exists(kbRoot, projectName)
		needsPathUpdate := cfg.Projects[projectName] != absPath
		kbContextPath := project.KBRelPath(kbRoot, projectName) + "/context.md"

		skipConfirm, _ := cmd.Flags().GetBool("yes")
		if !skipConfirm {
			if needsRegister {
				fmt.Printf("Will register project %q in KB\n", projectName)
			} else if needsPathUpdate {
				fmt.Printf("Will update path for project %q in KB\n", projectName)
			}
			for _, t := range targets {
				action := "Create"
				if _, err := os.Stat(t.path); err == nil {
					action = "Modify"
				}
				fmt.Printf("Will %s: %s (%s)\n", strings.ToLower(action), t.path, t.agent.Name)
				fmt.Printf("  Add MCP server \"kb\" → %s mcp\n", kbBinary)
			}
			fmt.Printf("Will write import pointers (CLAUDE.md + any AGENT.md/AGENTS.md) → %s\n", kbContextPath)
			fmt.Println()
			if !promptConfirm("Proceed?") {
				fmt.Println("Aborted.")
				return nil
			}
			fmt.Println()
		}

		// Register project (creates dir, context.md, refs.yml, kb.yml entry)
		created, err := project.Register(kbRoot, projectName, absPath)
		if err != nil {
			return err
		}
		if created {
			fmt.Printf("Registered project %q in KB\n", projectName)
		}

		// Auto-commit any KB changes
		if created || needsPathUpdate {
			if err := git.AutoCommit(kbRoot, fmt.Sprintf("setup: register %s", projectName)); err != nil {
				return fmt.Errorf("committing registration: %w", err)
			}
		}

		// Write CLAUDE.md (and AGENT.md/AGENTS.md if they exist) with KB import directive
		written, err := project.WriteImportPointers(absPath, kbRoot, projectName)
		if err != nil {
			return err
		}
		for _, f := range written {
			fmt.Printf("Wrote %s (KB import → %s)\n", filepath.Join(absPath, f), kbContextPath)
		}

		// Write MCP config for each detected agent
		for _, t := range targets {
			var writeErr error
			switch t.agent.Kind {
			case agentClaude:
				writeErr = writeClaudeMCPConfig(t.path, kbBinary)
			case agentCodex:
				writeErr = writeCodexMCPConfig(t.path, kbBinary)
			case agentOpenCode:
				writeErr = writeOpenCodeMCPConfig(t.path, kbBinary)
			}
			if writeErr != nil {
				return fmt.Errorf("writing %s config: %w", t.agent.Name, writeErr)
			}
			fmt.Printf("Wrote %s (%s)\n", t.path, t.agent.Name)
		}

		fmt.Printf("KB MCP server configured for project %q\n", projectName)
		fmt.Println("Restart your editor to pick up the new MCP server.")

		return nil
	},
}

func promptConfirm(prompt string) bool {
	fmt.Printf("%s [Y/n] ", prompt)
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "" || answer == "y" || answer == "yes"
}

func init() {
	setupCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompts")
	rootCmd.AddCommand(setupCmd)
}

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install AI coding agent integrations (hooks, commands, MCP config)",
	Long: `Detects installed AI coding agents and installs integrations for each:

  Claude Code (~/.claude/):
    - /kb-setup slash command (commands/kb-setup.md)
    - PostToolUse auto-commit hook (settings.json)

  OpenAI Codex (~/.codex/):
    - Global MCP server config (config.toml)

  OpenCode (~/.config/opencode/):
    - file_edited auto-commit hook (opencode.json)
    - kb-setup skill (skills/kb-setup/SKILL.md)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("could not determine home directory: %v", err)
		}

		kbBinary := "kb"

		agents := detectAgents()
		if len(agents) == 0 {
			return fmt.Errorf("no supported AI coding agents detected")
		}

		// Confirm before writing
		skipConfirm, _ := cmd.Flags().GetBool("yes")
		if !skipConfirm {
			for _, a := range agents {
				switch a.Kind {
				case agentClaude:
					commandPath := filepath.Join(home, ".claude", "commands", "kb-setup.md")
					settingsPath := filepath.Join(home, ".claude", "settings.json")
					fmt.Printf("Claude Code:\n")
					fmt.Printf("  Will %s: %s\n", actionLabel(commandPath), commandPath)
					fmt.Printf("    Install /kb-setup slash command\n")
					fmt.Printf("  Will %s: %s\n", actionLabel(settingsPath), settingsPath)
					fmt.Printf("    Add PostToolUse hook → %s commit --hook\n", kbBinary)
				case agentCodex:
					configPath := filepath.Join(home, ".codex", "config.toml")
					fmt.Printf("OpenAI Codex:\n")
					fmt.Printf("  Will %s: %s\n", actionLabel(configPath), configPath)
					fmt.Printf("    Add MCP server \"kb\" → %s mcp\n", kbBinary)
					fmt.Printf("    (Codex does not support hooks or slash commands)\n")
				case agentOpenCode:
					configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
					skillPath := filepath.Join(home, ".config", "opencode", "skills", "kb-setup", "SKILL.md")
					fmt.Printf("OpenCode:\n")
					fmt.Printf("  Will %s: %s\n", actionLabel(configPath), configPath)
					fmt.Printf("    Add file_edited hook → %s commit\n", kbBinary)
					fmt.Printf("  Will %s: %s\n", actionLabel(skillPath), skillPath)
					fmt.Printf("    Install kb-setup skill\n")
				}
			}
			fmt.Println()
			if !promptConfirm("Proceed?") {
				fmt.Println("Aborted.")
				return nil
			}
			fmt.Println()
		}

		// Install for each detected agent
		for _, a := range agents {
			switch a.Kind {
			case agentClaude:
				if err := installClaude(home, kbBinary); err != nil {
					return fmt.Errorf("Claude Code: %w", err)
				}
			case agentCodex:
				if err := installCodex(home, kbBinary); err != nil {
					return fmt.Errorf("OpenAI Codex: %w", err)
				}
			case agentOpenCode:
				if err := installOpenCode(home, kbBinary); err != nil {
					return fmt.Errorf("OpenCode: %w", err)
				}
			}
		}

		fmt.Println("\nDone. Restart your editor to pick up the changes.")
		return nil
	},
}

func actionLabel(path string) string {
	if _, err := os.Stat(path); err == nil {
		return "modify"
	}
	return "create"
}

func installClaude(home, kbBinary string) error {
	commandsDir := filepath.Join(home, ".claude", "commands")
	commandPath := filepath.Join(commandsDir, "kb-setup.md")
	settingsPath := filepath.Join(home, ".claude", "settings.json")

	// Install slash command
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return fmt.Errorf("could not create commands directory: %v", err)
	}

	if err := os.WriteFile(commandPath, []byte(slashCommandContent), 0644); err != nil {
		return fmt.Errorf("could not write slash command: %v", err)
	}
	fmt.Printf("Installed /kb-setup slash command at %s\n", commandPath)

	// Install auto-commit hook into ~/.claude/settings.json
	if err := installClaudeHook(settingsPath, kbBinary); err != nil {
		return fmt.Errorf("could not install hook: %v", err)
	}
	fmt.Printf("Installed auto-commit hook in %s\n", settingsPath)

	return nil
}

func installCodex(home, kbBinary string) error {
	configPath := filepath.Join(home, ".codex", "config.toml")
	if err := writeCodexMCPConfig(configPath, kbBinary); err != nil {
		return fmt.Errorf("could not write MCP config: %v", err)
	}
	fmt.Printf("Installed MCP config in %s\n", configPath)
	return nil
}

func installOpenCode(home, kbBinary string) error {
	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")

	// Install file_edited hook
	if err := installOpenCodeHook(configPath, kbBinary); err != nil {
		return fmt.Errorf("could not install hook: %v", err)
	}
	fmt.Printf("Installed file_edited hook in %s\n", configPath)

	// Install skill
	skillDir := filepath.Join(home, ".config", "opencode", "skills", "kb-setup")
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return fmt.Errorf("could not create skill directory: %v", err)
	}

	skillContent := "---\nname: kb-setup\ndescription: Set up the KB knowledge base MCP server for this project\n---\n" + slashCommandContent
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		return fmt.Errorf("could not write skill: %v", err)
	}
	fmt.Printf("Installed kb-setup skill at %s\n", skillPath)

	return nil
}

func installClaudeHook(settingsPath, kbBinary string) error {
	// Read existing settings
	var settings map[string]any
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parsing existing settings.json: %w", err)
		}
	}
	if settings == nil {
		settings = map[string]any{}
	}

	// Build the hook entry
	hookCommand := fmt.Sprintf("%s commit --hook", kbBinary)
	kbHook := map[string]any{
		"type":    "command",
		"command": hookCommand,
	}

	hookRule := map[string]any{
		"matcher": "Edit|Write",
		"hooks":   []any{kbHook},
	}

	// Get or create hooks section
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}

	// Get existing PostToolUse hooks
	existingRules, _ := hooks["PostToolUse"].([]any)

	// Check if we already have a kb commit hook — replace it if so
	found := false
	for i, rule := range existingRules {
		ruleMap, ok := rule.(map[string]any)
		if !ok {
			continue
		}
		ruleHooks, _ := ruleMap["hooks"].([]any)
		for _, h := range ruleHooks {
			hMap, ok := h.(map[string]any)
			if !ok {
				continue
			}
			cmd, _ := hMap["command"].(string)
			if containsKBCommit(cmd) {
				existingRules[i] = hookRule
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		existingRules = append(existingRules, hookRule)
	}

	hooks["PostToolUse"] = existingRules
	settings["hooks"] = hooks

	// Write back
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings: %w", err)
	}

	return os.WriteFile(settingsPath, append(data, '\n'), 0644)
}

func installOpenCodeHook(configPath, kbBinary string) error {
	var cfg map[string]any
	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("parsing existing config: %w", err)
		}
	}
	if cfg == nil {
		cfg = map[string]any{}
	}

	experimental, _ := cfg["experimental"].(map[string]any)
	if experimental == nil {
		experimental = map[string]any{}
	}

	hook, _ := experimental["hook"].(map[string]any)
	if hook == nil {
		hook = map[string]any{}
	}

	kbHookEntry := map[string]any{
		"command": []string{kbBinary, "commit"},
	}

	// Get existing file_edited hooks
	existingHooks, _ := hook["file_edited"].([]any)

	// Check if kb commit hook already exists
	found := false
	for i, h := range existingHooks {
		hMap, ok := h.(map[string]any)
		if !ok {
			continue
		}
		cmdArr, _ := hMap["command"].([]any)
		if len(cmdArr) >= 2 {
			cmd0, _ := cmdArr[0].(string)
			cmd1, _ := cmdArr[1].(string)
			if containsKBCommit(cmd0 + " " + cmd1) {
				existingHooks[i] = kbHookEntry
				found = true
				break
			}
		}
	}

	if !found {
		existingHooks = append(existingHooks, kbHookEntry)
	}

	hook["file_edited"] = existingHooks
	experimental["hook"] = hook
	cfg["experimental"] = experimental

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	return os.WriteFile(configPath, append(data, '\n'), 0644)
}

func containsKBCommit(cmd string) bool {
	return strings.Contains(cmd, "kb commit") || strings.Contains(cmd, "kb\" commit")
}

func init() {
	installCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompts")
	rootCmd.AddCommand(installCmd)
}

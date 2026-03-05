package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/leona/kb/internal/fs"
)

type agentKind int

const (
	agentClaude agentKind = iota
	agentCodex
	agentOpenCode
)

type agent struct {
	Kind agentKind
	Name string
}

func detectAgents() []agent {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	var agents []agent

	if fs.DirExists(filepath.Join(home, ".claude")) {
		agents = append(agents, agent{Kind: agentClaude, Name: "Claude Code"})
	}
	if fs.DirExists(filepath.Join(home, ".codex")) {
		agents = append(agents, agent{Kind: agentCodex, Name: "OpenAI Codex"})
	}
	if fs.DirExists(filepath.Join(home, ".config", "opencode")) {
		agents = append(agents, agent{Kind: agentOpenCode, Name: "OpenCode"})
	}

	return agents
}

// writeJSONMCPConfig reads a JSON config file, adds or updates the kb MCP server
// entry under the given serverKey, and writes it back.
func writeJSONMCPConfig(path, kbBinary, serverKey string) error {
	var cfg map[string]any
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("parsing existing config %s: %w", path, err)
		}
	}
	if cfg == nil {
		cfg = map[string]any{}
	}

	servers, ok := cfg[serverKey].(map[string]any)
	if !ok {
		servers = map[string]any{}
	}

	servers["kb"] = map[string]any{
		"command": kbBinary,
		"args":    []string{"mcp"},
	}
	cfg[serverKey] = servers

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	return os.WriteFile(path, append(data, '\n'), 0644)
}

// writeClaudeMCPConfig writes/updates .mcp.json with the kb MCP server entry.
func writeClaudeMCPConfig(path, kbBinary string) error {
	return writeJSONMCPConfig(path, kbBinary, "mcpServers")
}

// writeCodexMCPConfig writes/updates .codex/config.toml with the kb MCP server entry.
// Uses string manipulation to avoid a TOML dependency.
func writeCodexMCPConfig(path, kbBinary string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	content := ""
	if data, err := os.ReadFile(path); err == nil {
		content = string(data)
	}

	section := fmt.Sprintf("[mcp_servers.kb]\ncommand = %q\nargs = [\"mcp\"]\n", kbBinary)

	if idx := strings.Index(content, "[mcp_servers.kb]"); idx >= 0 {
		// Replace existing section: find next [header] or EOF
		rest := content[idx+len("[mcp_servers.kb]"):]
		endOffset := strings.Index(rest, "\n[")
		if endOffset >= 0 {
			end := idx + len("[mcp_servers.kb]") + endOffset + 1
			content = content[:idx] + section + content[end:]
		} else {
			content = content[:idx] + section
		}
	} else {
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		if content != "" {
			content += "\n"
		}
		content += section
	}

	return os.WriteFile(path, []byte(content), 0644)
}

// writeOpenCodeMCPConfig writes/updates opencode.json with the kb MCP server entry.
func writeOpenCodeMCPConfig(path, kbBinary string) error {
	return writeJSONMCPConfig(path, kbBinary, "mcp")
}

// readOnlyKBTools lists the MCP tool names that are safe to auto-allow.
var readOnlyKBTools = []string{
	"mcp__kb__kb_context",
	"mcp__kb__kb_list",
	"mcp__kb__kb_read",
	"mcp__kb__kb_search",
	"mcp__kb__kb_log",
	"mcp__kb__kb_diff",
	"mcp__kb__kb_show",
	"mcp__kb__kb_draft",
}

// writeClaudePermissions adds read-only KB tools to .claude/settings.json permissions.
func writeClaudePermissions(projectPath string) error {
	dir := filepath.Join(projectPath, ".claude")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(dir, "settings.json")
	var cfg map[string]any
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
	}
	if cfg == nil {
		cfg = map[string]any{}
	}

	perms, _ := cfg["permissions"].(map[string]any)
	if perms == nil {
		perms = map[string]any{}
	}

	// Merge with existing allow list
	existing := map[string]bool{}
	if allow, ok := perms["allow"].([]any); ok {
		for _, v := range allow {
			if s, ok := v.(string); ok {
				existing[s] = true
			}
		}
	}
	for _, tool := range readOnlyKBTools {
		existing[tool] = true
	}

	allow := make([]string, 0, len(existing))
	for tool := range existing {
		allow = append(allow, tool)
	}
	// Sort for stable output
	sort.Strings(allow)
	perms["allow"] = allow
	cfg["permissions"] = perms

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}


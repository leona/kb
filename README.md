# agent-knowledge-base (`kb`)

A CLI tool and MCP server for managing markdown knowledge bases across projects. Externalizes project context (CLAUDE.md/AGENT.md) and shared documentation (API docs, specs, workflows) into a central versioned directory, then exposes it to AI coding agents via MCP.

Supports [Claude Code](https://code.claude.com/docs/en/overview), [OpenAI Codex](https://github.com/openai/codex), and [OpenCode](https://opencode.ai/).

## Why

- **Context files are scattered** across repos with no way to share docs between projects
- **API documentation gets duplicated** — the same API docs copied into 3+ repos
- **No version history** — when you update project context, the old version is gone
- **Large reference docs waste context** — 9000-line API docs shouldn't load into every session

`kb` solves this by centralizing knowledge into `~/knowledge-base/` with:
- Per-project context that replaces in-repo CLAUDE.md files
- Shared docs that multiple projects reference without duplication
- Global shared docs that are available to all projects automatically
- Git versioning on every change (auto-committed)
- An MCP server so agents can search, read, and write docs on-demand
- A TUI browser for navigating and assigning refs to the knowledge base interactively

## Setup

### 1. Install

```bash
go install github.com/leona/kb@latest
```

Or download a prebuilt binary from the [latest release](https://github.com/leona/kb/releases/latest):

```bash
curl -Lo kb https://github.com/leona/kb/releases/latest/download/kb-linux-amd64
chmod +x kb
sudo mv kb /usr/local/bin/kb
```

Replace `kb-linux-amd64` with your platform: `kb-darwin-arm64` (macOS Apple Silicon), `kb-darwin-amd64` (macOS Intel), `kb-windows-amd64.exe`.

### 2. Initialize the knowledge base

```bash
kb init
```

This creates `~/knowledge-base/` with:
- `kb.yml` — configuration file
- `shared/` — directory for shared documents
- `projects/` — directory for per-project knowledge
- `.git/` — version history (auto-managed)

To use a different directory:

```bash
kb init --dir ~/my-knowledge-base
```

Then set `KB_ROOT` in your shell config:

```bash
export KB_ROOT="$HOME/my-knowledge-base"
```

### 3. Install agent integrations

```bash
kb install
```

Auto-detects installed agents and installs (with confirmation prompts):

| Agent | What gets installed |
|---|---|
| **Claude Code** | `/kb-setup` slash command into `~/.claude/commands/`, PostToolUse auto-commit hook into `~/.claude/settings.json` |
| **OpenAI Codex** | MCP server config into `~/.codex/config.toml` |
| **OpenCode** | `file_edited` auto-commit hook, `/kb-setup` skill into `~/.config/opencode/skills/` |

The hooks auto-commit KB changes whenever an agent edits a file inside the knowledge base.

Use `-y` to skip confirmation prompts.

### 4. Configure MCP for a project

From inside a project directory:

```bash
kb setup
```

Or specify the project name:

```bash
kb setup my-project
```

This auto-detects installed agents and:
1. Creates MCP config in the project directory (`.mcp.json` for Claude Code, `.codex/config.toml` for Codex, `opencode.json` for OpenCode)
2. Writes `@import` pointers in CLAUDE.md (and AGENTS.md/AGENT.md if they exist) pointing to the KB context

Shows what will be written and asks for confirmation (`-y` to skip).

Restart your agent after setup to connect to the MCP server.

Claude Code expands this `@import` automatically — the full context loads as before. When Claude modifies the context, it edits the KB file (because of the directive header), not the repo's CLAUDE.md.

### Browsing

Interactive TUI for navigating the knowledge base:

```bash
kb
```

- Launched outside a project → shows all projects and shared docs in dual-pane view
- Launched inside a registered project → jumps to that project's detail view
- Navigate with arrow keys or `h`/`l`, `tab` to switch panes, `/` to filter, Enter to open, Esc to go back
- `q` or `Ctrl+C` to quit

## Versioning

Every CLI write operation auto-commits to the KB's git repo with a descriptive message:

```
$ kb log
a1b2c3d 2026-03-04 02:26 ref: link my-project → rest-api-docs
e4f5a6b 2026-03-04 02:26 shared: add rest-api-docs (1 file(s), 500 lines)
c7d8e9f 2026-03-04 02:18 import: my-project from ~/repos/my-project/CLAUDE.md
0a6b0f2 2026-03-04 02:07 init: knowledge base
```

## MCP Server Tools

When configured, agents get 15 tools:

| Tool | Description |
|---|---|
| `kb_context` | Read project context.md (equivalent to CLAUDE.md). Auto-detects project from cwd. |
| `kb_list` | List project docs, linked shared docs, globals, and other available shared docs. |
| `kb_read` | Read a specific doc by path. Supports `offset`/`limit` for large files. |
| `kb_search` | Full-text search across KB. Scoped by project, shared docs, or all. |
| `kb_draft` | Preview content before writing — returns formatted preview without persisting. |
| `kb_write` | Create or update a file under `shared/` or `projects/`. Auto-commits. |
| `kb_delete` | Delete a shared doc (with cleanup of refs/globals) or a specific file. |
| `kb_log` | Show version history. Scopeable to a project or shared doc. |
| `kb_diff` | Show uncommitted changes in the knowledge base. |
| `kb_show` | Read a file's content at a specific commit. |
| `kb_revert` | Revert a file to a specific commit. Auto-commits the change. |
| `kb_ref_add` | Link a shared doc to a project. Auto-detects project from cwd. |
| `kb_ref_remove` | Unlink a shared doc from a project. |
| `kb_global_add` | Mark a shared doc as globally available to all projects. |
| `kb_global_remove` | Remove a shared doc from globals. |

The `kb_draft` → `kb_write` workflow ensures agents preview changes before persisting them.

Agents can create shared docs, link them, and manage globals entirely through MCP — no CLI needed.

## Knowledge Base Structure

```
~/knowledge-base/
  .git/                              # Auto-managed version history
  kb.yml                             # Config + project path mappings + globals
  shared/                            # Cross-project shared docs
    rest-api-docs/
      api-reference.md               # The actual documentation
      meta.yml                       # Optional: title, description, source, tags
    design-patterns/
      reactivity.md
  projects/                          # Per-project knowledge
    my-project/
      context.md                     # Project context (the "real" CLAUDE.md)
      refs.yml                       # Links to shared docs: [rest-api-docs]
      notes/                         # Additional project-specific docs
        architecture.md
```

### Configuration files

**`kb.yml`** — Global config with project path mappings and globals:

```yaml
version: 1
editor: hx                                    # Fallback: $EDITOR, then vi

globals:                                       # Shared docs available to all projects
  - style-guide
  - coding-standards

projects:
  my-project: ~/repos/my-project
  other-project: ~/documents/other-project
  work-api: /opt/work/api-server
```

Projects can be anywhere on disk — they don't need to be under a common directory.

**`refs.yml`** — Per-project shared doc references:

```yaml
refs:
  - rest-api-docs
  - design-patterns
```

**`meta.yml`** — Optional shared doc metadata:

```yaml
title: REST API Reference
description: API endpoints, authentication, and usage examples
source: https://docs.example.com/api
tags: [api, rest, reference]
```

## How CLAUDE.md integration works

### Before `kb`

```
~/repos/my-project/CLAUDE.md     ← 300 lines of context, edited in-repo
~/repos/other-project/CLAUDE.md  ← Duplicate API docs, diverges over time
```

### After `kb`

```
~/repos/my-project/CLAUDE.md     ← One line: @~/knowledge-base/projects/my-project/context.md
~/knowledge-base/
  projects/my-project/context.md ← The actual 300 lines, versioned, with edit directive
  shared/rest-api-docs/         ← API docs, shared by both projects
```

The repo's CLAUDE.md becomes a single `@import` pointer. Claude Code expands this at session start and sees the full content. The KB's `context.md` starts with:

```markdown
<!-- KB managed: ~/knowledge-base/projects/my-project/context.md -->
<!-- Always edit THIS file for project context. Do NOT edit the repo's CLAUDE.md. -->
```

This directive ensures agents edit the KB file (versioned, centralized) rather than the repo's pointer file.

## CLI Commands

| Command | Description |
|---|---|
| `kb` | Launch the TUI browser (default command) |
| `kb init [--dir PATH]` | Initialize a new knowledge base |
| `kb install [-y]` | Install agent integrations (hooks, slash commands) |
| `kb setup [PROJECT] [-y]` | Configure MCP + `@import` pointers for a project |
| `kb project init <NAME> --path <DIR>` | Create a new project in the KB |
| `kb project import <NAME> --from <DIR>` | Import existing CLAUDE.md/AGENTS.md into the KB |
| `kb project list` | List all projects |
| `kb project show <NAME>` | Show project details and context |
| `kb shared add <SLUG> <FILES...>` | Add shared documents to the KB |
| `kb shared list` | List shared documents with usage info |
| `kb ref add <PROJECT> <SLUG>` | Link a shared doc to a project |
| `kb ref remove <PROJECT> <SLUG>` | Unlink a shared doc from a project |
| `kb global add <SLUG>` | Make a shared doc available to all projects |
| `kb global remove <SLUG>` | Remove a shared doc from globals |
| `kb global list` | List global shared docs |
| `kb search <QUERY> [--project NAME]` | Full-text search across the KB |
| `kb edit <PROJECT>` | Open project context.md in `$EDITOR` |
| `kb detect` | Auto-detect current project from working directory |
| `kb log [SCOPE]` | Show version history |
| `kb diff [SCOPE]` | Show uncommitted changes |
| `kb revert <REF> <PATH>` | Revert a file to a previous version |
| `kb commit` | Manually commit pending KB changes |
| `kb version` | Print version information |
| `kb mcp` | Start MCP server (stdio transport) |

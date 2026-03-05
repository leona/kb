package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/leona/kb/internal/config"
	iofs "github.com/leona/kb/internal/fs"
	"github.com/leona/kb/internal/git"
	"github.com/leona/kb/internal/project"
	"github.com/leona/kb/internal/search"
	"github.com/leona/kb/internal/shared"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// toSlash normalizes a path to forward slashes for KB-semantic paths.
// KB paths (like "projects/foo/context.md") always use forward slashes —
// they're protocol/display strings, not filesystem paths. filepath.Clean()
// converts to backslashes on Windows, which breaks strings.HasPrefix checks.
func toSlash(p string) string { return strings.ReplaceAll(p, "\\", "/") }

// Serve creates and starts the MCP server on stdio.
func Serve() error {
	s := server.NewMCPServer(
		"agent-knowledgebase",
		"0.1.0",
		server.WithToolCapabilities(true),
	)

	s.AddTool(mcp.NewTool("kb_context",
		mcp.WithDescription("Get the main project context document (equivalent to CLAUDE.md). Auto-detects project from working directory if not specified."),
		mcp.WithString("project", mcp.Description("Project name. Auto-detected from cwd if omitted.")),
	), handleContext)

	s.AddTool(mcp.NewTool("kb_list",
		mcp.WithDescription("List available knowledge base documents for a project, including linked shared docs."),
		mcp.WithString("project", mcp.Description("Project name. Auto-detected from cwd if omitted.")),
		mcp.WithBoolean("include_shared", mcp.Description("Include linked shared docs in listing. Default: true.")),
	), handleList)

	s.AddTool(mcp.NewTool("kb_read",
		mcp.WithDescription("Read a knowledge base document by relative path. Use kb_list to discover available docs."),
		mcp.WithString("path", mcp.Required(), mcp.Description("Relative path within KB (e.g., 'projects/myapp/context.md' or 'shared/my-shared-doc/api-docs.md').")),
		mcp.WithNumber("offset", mcp.Description("Line offset to start reading from (0-based). For large files.")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of lines to return. 0 = all.")),
	), handleRead)

	s.AddTool(mcp.NewTool("kb_log",
		mcp.WithDescription("Show version history of the knowledge base. Returns commit log with hashes, dates, and messages."),
		mcp.WithString("scope", mcp.Description("Scope to a project name or shared doc slug. Omit for full history.")),
		mcp.WithNumber("max_entries", mcp.Description("Maximum log entries to return. Default: 20.")),
	), handleLog)

	s.AddTool(mcp.NewTool("kb_diff",
		mcp.WithDescription("Show uncommitted changes in the knowledge base. Returns a list of modified, added, or deleted files."),
		mcp.WithString("scope", mcp.Description("Scope to a project name or shared doc slug. Omit for all changes.")),
	), handleDiff)

	s.AddTool(mcp.NewTool("kb_show",
		mcp.WithDescription("Read a file's content at a specific commit. Use kb_log to find commit hashes."),
		mcp.WithString("ref", mcp.Required(), mcp.Description("Commit hash (short or full) from kb_log.")),
		mcp.WithString("path", mcp.Required(), mcp.Description("Relative path within KB (e.g., 'projects/myapp/context.md').")),
	), handleShow)

	s.AddTool(mcp.NewTool("kb_revert",
		mcp.WithDescription("Revert a file to its content at a specific commit. The change is auto-committed unless no_commit is true."),
		mcp.WithString("ref", mcp.Required(), mcp.Description("Commit hash (short or full) from kb_log.")),
		mcp.WithString("path", mcp.Required(), mcp.Description("Relative path within KB (e.g., 'projects/myapp/context.md').")),
		mcp.WithBoolean("no_commit", mcp.Description("Skip auto-commit. Use kb_commit to commit later.")),
	), handleRevert)

	s.AddTool(mcp.NewTool("kb_ref_add",
		mcp.WithDescription("Link a shared doc to a project so it appears in kb_list. Idempotent — safe to call if already linked. Auto-detects project from working directory if not specified. Changes are auto-committed unless no_commit is true."),
		mcp.WithString("project", mcp.Description("Project name. Auto-detected from cwd if omitted.")),
		mcp.WithString("shared", mcp.Required(), mcp.Description("Shared doc slug to link (e.g., 'my-shared-doc').")),
		mcp.WithBoolean("inline", mcp.Description("If true, inline the shared doc content directly into context.md instead of listing it as a reference.")),
		mcp.WithBoolean("no_commit", mcp.Description("Skip auto-commit. Use kb_commit to commit later.")),
	), handleRefAdd)

	s.AddTool(mcp.NewTool("kb_ref_remove",
		mcp.WithDescription("Unlink a shared doc from a project. Auto-detects project from working directory if not specified. Changes are auto-committed unless no_commit is true."),
		mcp.WithString("project", mcp.Description("Project name. Auto-detected from cwd if omitted.")),
		mcp.WithString("shared", mcp.Required(), mcp.Description("Shared doc slug to unlink (e.g., 'my-shared-doc').")),
		mcp.WithBoolean("no_commit", mcp.Description("Skip auto-commit. Use kb_commit to commit later.")),
	), handleRefRemove)

	s.AddTool(mcp.NewTool("kb_draft",
		mcp.WithDescription("Preview content before writing to the knowledge base. Returns a formatted preview of what would be written without making any changes. IMPORTANT: Always call this before kb_write so the user can review the content first. After calling this tool, display the full preview content to the user — do not summarize or truncate it."),
		mcp.WithString("path", mcp.Required(), mcp.Description("Relative path within KB (e.g., 'shared/my-shared-doc/api-docs.md' or 'projects/myapp/context.md').")),
		mcp.WithString("content", mcp.Required(), mcp.Description("The markdown content to preview.")),
		mcp.WithString("title", mcp.Description("Title for shared doc meta.yml.")),
		mcp.WithString("description", mcp.Description("Description for shared doc meta.yml (1-2 sentence summary).")),
	), handleDraft)

	s.AddTool(mcp.NewTool("kb_write",
		mcp.WithDescription("Create or update a file in the knowledge base. IMPORTANT: Always use kb_draft first to show the user a preview before calling this tool. Path must be under shared/ or projects/. Parent directories are created automatically. Changes are auto-committed unless no_commit is true."),
		mcp.WithString("path", mcp.Required(), mcp.Description("Relative path within KB (e.g., 'shared/my-shared-doc/api-docs.md' or 'projects/myapp/context.md').")),
		mcp.WithString("content", mcp.Required(), mcp.Description("The markdown content to write.")),
		mcp.WithString("title", mcp.Description("Title for shared doc meta.yml. Creates or updates the title.")),
		mcp.WithString("description", mcp.Description("Description for shared doc meta.yml (1-2 sentence summary). Helps agents discover relevant docs.")),
		mcp.WithBoolean("no_commit", mcp.Description("Skip auto-commit. Use kb_commit to commit later.")),
	), handleWrite)

	s.AddTool(mcp.NewTool("kb_global_add",
		mcp.WithDescription("Mark a shared doc as globally available to all projects. It will appear in kb_list and kb_search for every project without needing kb_ref_add. Idempotent. Changes are auto-committed unless no_commit is true."),
		mcp.WithString("shared", mcp.Required(), mcp.Description("Shared doc slug to make global (e.g., 'my-shared-doc').")),
		mcp.WithBoolean("inline", mcp.Description("If true, inline the shared doc content directly into all projects' context.md instead of listing as a reference.")),
		mcp.WithBoolean("no_commit", mcp.Description("Skip auto-commit. Use kb_commit to commit later.")),
	), handleGlobalAdd)

	s.AddTool(mcp.NewTool("kb_global_remove",
		mcp.WithDescription("Remove a shared doc from globals. It will no longer be automatically available to all projects (projects with explicit refs will keep it). Changes are auto-committed unless no_commit is true."),
		mcp.WithString("shared", mcp.Required(), mcp.Description("Shared doc slug to remove from globals (e.g., 'my-shared-doc').")),
		mcp.WithBoolean("no_commit", mcp.Description("Skip auto-commit. Use kb_commit to commit later.")),
	), handleGlobalRemove)

	s.AddTool(mcp.NewTool("kb_delete",
		mcp.WithDescription("Delete a shared doc or a specific file from the knowledge base. When deleting a shared doc slug, removes the entire directory, all refs to it, and any global entry. When deleting a specific file path, removes just that file. Changes are auto-committed unless no_commit is true."),
		mcp.WithString("shared", mcp.Description("Shared doc slug to delete entirely (e.g., 'my-shared-doc'). Removes directory, refs, and global entry.")),
		mcp.WithString("path", mcp.Description("Relative path of a specific file to delete (e.g., 'shared/my-shared-doc/old-notes.md').")),
		mcp.WithBoolean("no_commit", mcp.Description("Skip auto-commit. Use kb_commit to commit later.")),
	), handleDelete)

	s.AddTool(mcp.NewTool("kb_search",
		mcp.WithDescription("Full-text search across knowledge base markdown files. Returns matching lines with file paths and line numbers. Multi-word queries use AND matching (all terms must appear on the same line). Also matches shared doc tags and titles."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query (case-insensitive). Multiple words are AND-matched.")),
		mcp.WithString("project", mcp.Description("Scope to a project and its shared refs. Auto-detected from cwd if omitted.")),
		mcp.WithString("scope", mcp.Description("Search scope: 'all' (default), 'project' (only project docs), 'shared' (only shared docs).")),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return. Default: 20.")),
		mcp.WithNumber("context", mcp.Description("Number of lines to show before and after each match. Default: 2.")),
	), handleSearch)

	s.AddTool(mcp.NewTool("kb_commit",
		mcp.WithDescription("Commit pending knowledge base changes. Use after mutations made with no_commit=true. If no message is provided, one is auto-generated from the pending changes."),
		mcp.WithString("message", mcp.Description("Optional commit message. Auto-generates a descriptive message if omitted.")),
	), handleCommit)

	return server.ServeStdio(s)
}

func resolveProject(explicit string) string {
	if explicit != "" {
		return explicit
	}
	kbRoot := config.ResolveKBRoot()
	name, _ := project.Detect(kbRoot)
	return name
}

// validatePath cleans a relative path and rejects traversal/absolute paths.
// After cleaning, ensures the path stays under shared/ or projects/.
func validatePath(relPath string) (string, error) {
	cleanPath := toSlash(filepath.Clean(relPath))
	if strings.HasPrefix(cleanPath, "..") || filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("invalid path: must be relative within KB")
	}
	// After cleaning, ensure path is still under a known KB subdirectory.
	// This catches traversals like "projects/x/../../etc/passwd" → "etc/passwd".
	if !strings.HasPrefix(cleanPath, "shared/") && !strings.HasPrefix(cleanPath, "projects/") {
		return "", fmt.Errorf("invalid path: must be under shared/ or projects/")
	}
	return cleanPath, nil
}

// validateKBPath validates that a path is under shared/ or projects/.
// Equivalent to validatePath after its prefix enforcement was added.
func validateKBPath(relPath string) (string, error) {
	return validatePath(relPath)
}

// validateSlug checks that a slug contains no path separators or traversal sequences.
func validateSlug(slug string) error {
	if strings.Contains(slug, "/") || strings.Contains(slug, "\\") || strings.Contains(slug, "..") {
		return fmt.Errorf("invalid slug %q: must not contain path separators or '..'", slug)
	}
	return nil
}

// resolveScope converts a scope name to a path prefix for git operations.
func resolveScope(kbRoot, scope string) string {
	if scope == "" {
		return ""
	}
	if project.Exists(kbRoot, scope) {
		return "projects/" + scope + "/"
	}
	if shared.Exists(kbRoot, scope) {
		return "shared/" + scope + "/"
	}
	return ""
}

func handleContext(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	kbRoot := config.ResolveKBRoot()
	projectName := resolveProject(request.GetString("project", ""))

	if projectName == "" {
		return mcp.NewToolResultError("Could not detect project. Specify 'project' parameter or ensure cwd is inside a registered project."), nil
	}

	contextPath := project.ContextPath(kbRoot, projectName)
	if !iofs.FileExists(contextPath) {
		return mcp.NewToolResultError(fmt.Sprintf("No context.md found for project %q", projectName)), nil
	}

	content, err := iofs.ReadFile(contextPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error reading context.md: %v", err)), nil
	}

	return mcp.NewToolResultText(content), nil
}

func handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	kbRoot := config.ResolveKBRoot()
	projectName := resolveProject(request.GetString("project", ""))
	includeShared := true
	if v, ok := request.GetArguments()["include_shared"]; ok {
		if b, ok := v.(bool); ok {
			includeShared = b
		}
	}

	var sb strings.Builder

	if projectName != "" {
		info, err := project.Get(kbRoot, projectName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Project %q not found", projectName)), nil
		}

		sb.WriteString(fmt.Sprintf("# Project: %s\n\n", projectName))

		// Context file
		contextPath := project.ContextPath(kbRoot, projectName)
		if iofs.FileExists(contextPath) {
			lines, _ := iofs.CountLines(contextPath)
			sb.WriteString(fmt.Sprintf("- projects/%s/context.md (%d lines)\n", projectName, lines))
		}

		// Additional project files
		for _, f := range info.Files {
			fullPath := filepath.Join(project.Dir(kbRoot, projectName), f)
			lines, _ := iofs.CountLines(fullPath)
			sb.WriteString(fmt.Sprintf("- projects/%s/%s (%d lines)\n", projectName, f, lines))
		}

		// Shared docs
		if includeShared {
			cfg, _ := config.Load(kbRoot)
			effectiveRefs := info.Refs
			var effectiveInline []string
			if cfg != nil {
				effectiveRefs, effectiveInline = config.EffectiveRefsAndInline(cfg, info.Refs, info.Inline)
			}

			linkedSet := make(map[string]bool, len(effectiveRefs)+len(effectiveInline))
			if len(effectiveRefs) > 0 || len(effectiveInline) > 0 {
				sb.WriteString("\n## Linked Shared Docs\n\n")
				for _, slug := range effectiveRefs {
					linkedSet[slug] = true
					sharedInfo, err := shared.Get(kbRoot, slug)
					if err != nil {
						continue
					}
					sb.WriteString(fmt.Sprintf("- %s (%d lines)", sharedInfo.DisplayTitle(), sharedInfo.TotalLines))
					if sharedInfo.Description != "" {
						sb.WriteString(fmt.Sprintf(" — %s", sharedInfo.Description))
					}
					sb.WriteString("\n")
					for _, f := range sharedInfo.Files {
						fullPath := filepath.Join(shared.Dir(kbRoot, slug), f)
						lines, _ := iofs.CountLines(fullPath)
						sb.WriteString(fmt.Sprintf("  - shared/%s/%s (%d lines)\n", slug, f, lines))
					}
				}
				for _, slug := range effectiveInline {
					linkedSet[slug] = true
					sharedInfo, err := shared.Get(kbRoot, slug)
					if err != nil {
						continue
					}
					sb.WriteString(fmt.Sprintf("- %s (%d lines, inlined in context)", sharedInfo.DisplayTitle(), sharedInfo.TotalLines))
					if sharedInfo.Description != "" {
						sb.WriteString(fmt.Sprintf(" — %s", sharedInfo.Description))
					}
					sb.WriteString("\n")
				}
			}

			// Show all other shared docs so they're discoverable
			allShared, err := shared.List(kbRoot)
			if err == nil {
				var other []shared.Info
				for _, d := range allShared {
					if !linkedSet[d.Slug] {
						other = append(other, d)
					}
				}
				if len(other) > 0 {
					sb.WriteString("\n## Other Available Shared Docs\n\n")
					for _, d := range other {
						desc := ""
						if d.Description != "" {
							desc = " — " + d.Description
						}
						sb.WriteString(fmt.Sprintf("- %s (%d lines, %d files)%s\n", d.DisplayTitle(), d.TotalLines, len(d.Files), desc))
						for _, f := range d.Files {
							sb.WriteString(fmt.Sprintf("  - kb_read: shared/%s/%s\n", d.Slug, f))
						}
					}
				}
			}
		}
	} else {
		// List all projects
		sb.WriteString("# All Projects\n\n")
		projects, err := project.List(kbRoot)
		if err == nil {
			for _, p := range projects {
				refInfo := ""
				if len(p.Refs) > 0 {
					refInfo = fmt.Sprintf(" [refs: %s]", strings.Join(p.Refs, ", "))
				}
				sb.WriteString(fmt.Sprintf("- %s (%d lines)%s\n", p.Name, p.ContextLines, refInfo))
			}
		}

		sb.WriteString("\n# Shared Docs\n\n")
		docs, err := shared.List(kbRoot)
		if err == nil {
			for _, d := range docs {
				sb.WriteString(fmt.Sprintf("- %s (%d lines, %d files)\n", d.DisplayTitle(), d.TotalLines, len(d.Files)))
			}
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func handleRead(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	kbRoot := config.ResolveKBRoot()
	relPath := request.GetString("path", "")
	if relPath == "" {
		return mcp.NewToolResultError("'path' parameter is required"), nil
	}

	cleanPath, err := validatePath(relPath)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	absPath := filepath.Join(kbRoot, cleanPath)
	if !iofs.FileExists(absPath) {
		return mcp.NewToolResultError(fmt.Sprintf("File not found: %s", relPath)), nil
	}

	offset := int(request.GetFloat("offset", 0))
	limit := int(request.GetFloat("limit", 0))

	if offset > 0 || limit > 0 {
		lines, totalLines, err := iofs.ReadLines(absPath, offset, limit)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error reading file: %v", err)), nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# %s (lines %d-%d of %d)\n\n", relPath, offset+1, offset+len(lines), totalLines))
		for i, line := range lines {
			sb.WriteString(fmt.Sprintf("%4d: %s\n", offset+i+1, line))
		}
		return mcp.NewToolResultText(sb.String()), nil
	}

	content, err := iofs.ReadFile(absPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error reading file: %v", err)), nil
	}

	// Warn if file is very large
	lines, _ := iofs.CountLines(absPath)
	if lines > 500 {
		header := fmt.Sprintf("# %s (%d lines — consider using offset/limit for large files)\n\n", relPath, lines)
		content = header + content
	}

	return mcp.NewToolResultText(content), nil
}

func handleSearch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	kbRoot := config.ResolveKBRoot()
	query := request.GetString("query", "")
	if query == "" {
		return mcp.NewToolResultError("'query' parameter is required"), nil
	}

	projectName := resolveProject(request.GetString("project", ""))
	scope := request.GetString("scope", "all")
	if scope != "all" && scope != "project" && scope != "shared" {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid scope %q: must be 'all', 'project', or 'shared'", scope)), nil
	}
	maxResults := int(request.GetFloat("max_results", 20))
	contextLines := int(request.GetFloat("context", 2))

	resp, err := search.Search(kbRoot, query, search.Options{
		Project:      projectName,
		Scope:        scope,
		MaxResults:   maxResults,
		ContextLines: contextLines,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Search error: %v", err)), nil
	}

	if resp == nil || (len(resp.Results) == 0 && len(resp.TagMatches) == 0) {
		return mcp.NewToolResultText("No results found."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Search: %q (%d results)\n\n", query, len(resp.Results)))

	currentFile := ""
	currentSection := ""
	lastRenderedLine := 0 // track last rendered line per file to avoid overlap
	for _, r := range resp.Results {
		if r.File != currentFile {
			sb.WriteString(fmt.Sprintf("\n## %s\n", r.File))
			currentFile = r.File
			currentSection = ""
			lastRenderedLine = 0
		}
		// Show section heading if it changed.
		if r.Section != "" && r.Section != currentSection {
			sb.WriteString(fmt.Sprintf("\n> %s\n\n", r.Section))
			currentSection = r.Section
		}
		// Render context-before lines (skip if already rendered by previous result).
		for _, cl := range r.Context {
			if cl.Line < r.Line && cl.Line > lastRenderedLine {
				sb.WriteString(fmt.Sprintf("  %4d  %s\n", cl.Line, cl.Content))
			}
		}
		// Render the matching line (highlighted with marker).
		sb.WriteString(fmt.Sprintf("→ %4d: %s\n", r.Line, r.Content))
		lastRenderedLine = r.Line
		// Render context-after lines.
		for _, cl := range r.Context {
			if cl.Line > r.Line {
				sb.WriteString(fmt.Sprintf("  %4d  %s\n", cl.Line, cl.Content))
				lastRenderedLine = cl.Line
			}
		}
		sb.WriteString("\n")
	}

	// Show tag/title matches for shared docs not already in results.
	if len(resp.TagMatches) > 0 {
		sb.WriteString("\n## Shared docs matching by tag/title\n\n")
		for _, tm := range resp.TagMatches {
			sb.WriteString(fmt.Sprintf("- **%s** (matched %s)\n", tm.Title, tm.Match))
			if sharedInfo, err := shared.Get(kbRoot, tm.Slug); err == nil {
				for _, f := range sharedInfo.Files {
					sb.WriteString(fmt.Sprintf("  - kb_read: shared/%s/%s\n", tm.Slug, f))
				}
			}
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func handleDraft(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	relPath := request.GetString("path", "")
	content := request.GetString("content", "")
	title := request.GetString("title", "")
	description := request.GetString("description", "")

	if relPath == "" {
		return mcp.NewToolResultError("'path' parameter is required"), nil
	}
	if content == "" {
		return mcp.NewToolResultError("'content' parameter is required"), nil
	}

	cleanPath, err := validateKBPath(relPath)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	lines := strings.Count(content, "\n") + 1

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Draft Preview: %s\n\n", cleanPath))

	if title != "" || description != "" {
		sb.WriteString("**Metadata:**\n")
		if title != "" {
			sb.WriteString(fmt.Sprintf("- Title: %s\n", title))
		}
		if description != "" {
			sb.WriteString(fmt.Sprintf("- Description: %s\n", description))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("**Lines:** %d\n\n", lines))
	sb.WriteString("---\n\n")
	sb.WriteString(content)
	sb.WriteString("\n\n---\n\n")
	sb.WriteString("Review the content above. If it looks good, call kb_write with the same parameters to save it.")

	return mcp.NewToolResultText(sb.String()), nil
}

func handleWrite(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	kbRoot := config.ResolveKBRoot()
	relPath := request.GetString("path", "")
	content := request.GetString("content", "")
	title := request.GetString("title", "")
	description := request.GetString("description", "")

	if relPath == "" {
		return mcp.NewToolResultError("'path' parameter is required"), nil
	}
	if content == "" {
		return mcp.NewToolResultError("'content' parameter is required"), nil
	}

	cleanPath, err := validateKBPath(relPath)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	absPath := filepath.Join(kbRoot, cleanPath)

	// Create parent directories
	if err := iofs.EnsureDir(filepath.Dir(absPath)); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error creating directories: %v", err)), nil
	}

	// Write the file
	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error writing file: %v", err)), nil
	}

	if strings.HasPrefix(cleanPath, "shared/") {
		// Update meta.yml with title/description and refresh all inventories.
		parts := strings.SplitN(cleanPath, "/", 3)
		if len(parts) >= 2 {
			if title != "" || description != "" {
				sharedDir := filepath.Join(kbRoot, "shared", parts[1])
				meta := config.LoadMeta(sharedDir)
				if meta == nil {
					meta = &config.Meta{}
				}
				if title != "" {
					meta.Title = title
				}
				if description != "" {
					meta.Description = description
				}
				if err := config.SaveMeta(sharedDir, meta); err != nil {
					// Non-fatal, continue
					fmt.Fprintf(os.Stderr, "Warning: could not write meta.yml: %v\n", err)
				}
			}
			// Content or metadata changed — regenerate refs inventories
			// so KB:REFS blocks reflect updated line counts/descriptions.
			if err := project.UpdateAllRefsInventories(kbRoot); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to update refs inventories: %v\n", err)
			}
		}
	} else if strings.HasPrefix(cleanPath, "projects/") {
		// Re-inject the KB:REFS inventory block if writing a project's context.md.
		parts := strings.SplitN(cleanPath, "/", 3)
		if len(parts) >= 3 && parts[2] == "context.md" {
			if err := project.UpdateRefsInventory(kbRoot, parts[1]); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to update refs inventory for %s: %v\n", parts[1], err)
			}
		}
	}

	lines, _ := iofs.CountLines(absPath)
	if !request.GetBool("no_commit", false) {
		if err := git.CommitAndPush(kbRoot, fmt.Sprintf("write: %s (%d lines)", cleanPath, lines)); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error committing: %v", err)), nil
		}
	}

	return mcp.NewToolResultText(fmt.Sprintf("Wrote %s (%d lines)", cleanPath, lines)), nil
}

func handleGlobalAdd(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	kbRoot := config.ResolveKBRoot()
	slug := request.GetString("shared", "")
	inline := request.GetBool("inline", false)

	if slug == "" {
		return mcp.NewToolResultError("'shared' parameter is required"), nil
	}
	if err := validateSlug(slug); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if !shared.Exists(kbRoot, slug) {
		return mcp.NewToolResultError(fmt.Sprintf("Shared doc %q not found. Create it first with kb_write.", slug)), nil
	}

	if err := project.AddGlobal(kbRoot, slug, inline); errors.Is(err, project.ErrAlreadyGlobal) {
		return mcp.NewToolResultText(fmt.Sprintf("%s is already global", slug)), nil
	} else if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error: %v", err)), nil
	}

	linkType := "ref"
	if inline {
		linkType = "inline"
	}
	if !request.GetBool("no_commit", false) {
		if err := git.CommitAndPush(kbRoot, fmt.Sprintf("global: add %s (%s)", slug, linkType)); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error committing: %v", err)), nil
		}
	}

	return mcp.NewToolResultText(fmt.Sprintf("Marked %s as global (%s) — now available to all projects", slug, linkType)), nil
}

func handleGlobalRemove(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	kbRoot := config.ResolveKBRoot()
	slug := request.GetString("shared", "")

	if slug == "" {
		return mcp.NewToolResultError("'shared' parameter is required"), nil
	}
	if err := validateSlug(slug); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if err := project.RemoveGlobal(kbRoot, slug); errors.Is(err, project.ErrNotGlobal) {
		return mcp.NewToolResultText(fmt.Sprintf("%s is not a global shared doc", slug)), nil
	} else if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error: %v", err)), nil
	}

	if !request.GetBool("no_commit", false) {
		if err := git.CommitAndPush(kbRoot, fmt.Sprintf("global: remove %s", slug)); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error committing: %v", err)), nil
		}
	}

	return mcp.NewToolResultText(fmt.Sprintf("Removed %s from globals", slug)), nil
}

func handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	kbRoot := config.ResolveKBRoot()
	slug := request.GetString("shared", "")
	relPath := request.GetString("path", "")

	if slug == "" && relPath == "" {
		return mcp.NewToolResultError("Specify either 'shared' (to delete an entire shared doc) or 'path' (to delete a specific file)"), nil
	}

	// Delete entire shared doc
	if slug != "" {
		if err := validateSlug(slug); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		dir := shared.Dir(kbRoot, slug)
		if !iofs.DirExists(dir) {
			return mcp.NewToolResultError(fmt.Sprintf("Shared doc %q not found", slug)), nil
		}

		if err := os.RemoveAll(dir); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error removing directory: %v", err)), nil
		}

		// Remove from globals, all project refs, then regenerate inventories once.
		if err := project.RemoveGlobalEntry(kbRoot, slug); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to remove global entry for %s: %v\n", slug, err)
		}
		if err := project.RemoveRefFromAll(kbRoot, slug); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to remove refs for %s: %v\n", slug, err)
		}
		if err := project.UpdateAllRefsInventories(kbRoot); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to update refs inventories: %v\n", err)
		}

		if !request.GetBool("no_commit", false) {
			if err := git.CommitAndPush(kbRoot, fmt.Sprintf("delete: shared/%s", slug)); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Error committing: %v", err)), nil
			}
		}

		return mcp.NewToolResultText(fmt.Sprintf("Deleted shared doc %s (directory, refs, and global entry removed)", slug)), nil
	}

	// Delete a specific file
	cleanPath, err := validateKBPath(relPath)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	absPath := filepath.Join(kbRoot, cleanPath)
	if !iofs.FileExists(absPath) {
		return mcp.NewToolResultError(fmt.Sprintf("File not found: %s", relPath)), nil
	}

	if err := os.Remove(absPath); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error deleting file: %v", err)), nil
	}

	// If deleted file was under shared/, regenerate inventories.
	if strings.HasPrefix(cleanPath, "shared/") {
		if err := project.UpdateAllRefsInventories(kbRoot); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to update refs inventories: %v\n", err)
		}
	}

	if !request.GetBool("no_commit", false) {
		if err := git.CommitAndPush(kbRoot, fmt.Sprintf("delete: %s", cleanPath)); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error committing: %v", err)), nil
		}
	}

	return mcp.NewToolResultText(fmt.Sprintf("Deleted %s", cleanPath)), nil
}

func handleRefAdd(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	kbRoot := config.ResolveKBRoot()
	projectName := resolveProject(request.GetString("project", ""))
	slug := request.GetString("shared", "")
	inline := request.GetBool("inline", false)

	if projectName == "" {
		return mcp.NewToolResultError("Could not detect project. Specify 'project' parameter or ensure cwd is inside a registered project."), nil
	}
	if slug == "" {
		return mcp.NewToolResultError("'shared' parameter is required"), nil
	}
	if err := validateSlug(slug); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if !project.Exists(kbRoot, projectName) {
		return mcp.NewToolResultError(fmt.Sprintf("Project %q not found", projectName)), nil
	}
	if !shared.Exists(kbRoot, slug) {
		return mcp.NewToolResultError(fmt.Sprintf("Shared doc %q not found", slug)), nil
	}

	if err := project.AddRef(kbRoot, projectName, slug, inline); errors.Is(err, project.ErrAlreadyLinked) {
		return mcp.NewToolResultText(fmt.Sprintf("%s is already linked to %s", slug, projectName)), nil
	} else if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error: %v", err)), nil
	}

	linkType := "ref"
	if inline {
		linkType = "inline"
	}
	if !request.GetBool("no_commit", false) {
		if err := git.CommitAndPush(kbRoot, fmt.Sprintf("ref: link %s → %s (%s)", projectName, slug, linkType)); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error committing: %v", err)), nil
		}
	}

	return mcp.NewToolResultText(fmt.Sprintf("Linked %s → %s (%s)", projectName, slug, linkType)), nil
}

func handleRefRemove(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	kbRoot := config.ResolveKBRoot()
	projectName := resolveProject(request.GetString("project", ""))
	slug := request.GetString("shared", "")

	if projectName == "" {
		return mcp.NewToolResultError("Could not detect project. Specify 'project' parameter or ensure cwd is inside a registered project."), nil
	}
	if slug == "" {
		return mcp.NewToolResultError("'shared' parameter is required"), nil
	}
	if err := validateSlug(slug); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if err := project.RemoveRef(kbRoot, projectName, slug); errors.Is(err, project.ErrNotLinked) {
		return mcp.NewToolResultText(fmt.Sprintf("%s is not linked to %s", slug, projectName)), nil
	} else if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error: %v", err)), nil
	}

	if !request.GetBool("no_commit", false) {
		if err := git.CommitAndPush(kbRoot, fmt.Sprintf("ref: unlink %s → %s", projectName, slug)); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error committing: %v", err)), nil
		}
	}

	return mcp.NewToolResultText(fmt.Sprintf("Unlinked %s → %s", projectName, slug)), nil
}

func handleLog(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	kbRoot := config.ResolveKBRoot()
	scope := request.GetString("scope", "")
	maxEntries := int(request.GetFloat("max_entries", 20))

	scopePath := resolveScope(kbRoot, scope)

	entries, err := git.Log(kbRoot, scopePath, maxEntries)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Log error: %v", err)), nil
	}

	if len(entries) == 0 {
		return mcp.NewToolResultText("No history found."), nil
	}

	var sb strings.Builder
	if scope != "" {
		sb.WriteString(fmt.Sprintf("# History: %s (%d entries)\n\n", scope, len(entries)))
	} else {
		sb.WriteString(fmt.Sprintf("# History (%d entries)\n\n", len(entries)))
	}

	for _, entry := range entries {
		sb.WriteString(entry + "\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func handleDiff(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	kbRoot := config.ResolveKBRoot()
	scope := request.GetString("scope", "")

	scopePath := resolveScope(kbRoot, scope)

	result, err := git.Diff(kbRoot, scopePath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Diff error: %v", err)), nil
	}

	return mcp.NewToolResultText(result), nil
}

func handleShow(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	kbRoot := config.ResolveKBRoot()
	ref := request.GetString("ref", "")
	relPath := request.GetString("path", "")

	if ref == "" {
		return mcp.NewToolResultError("'ref' parameter is required"), nil
	}
	if relPath == "" {
		return mcp.NewToolResultError("'path' parameter is required"), nil
	}

	cleanPath, err := validatePath(relPath)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	content, err := git.Show(kbRoot, ref, cleanPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Show error: %v", err)), nil
	}

	return mcp.NewToolResultText(content), nil
}

func handleRevert(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	kbRoot := config.ResolveKBRoot()
	ref := request.GetString("ref", "")
	relPath := request.GetString("path", "")

	if ref == "" {
		return mcp.NewToolResultError("'ref' parameter is required"), nil
	}
	if relPath == "" {
		return mcp.NewToolResultError("'path' parameter is required"), nil
	}

	cleanPath, err := validatePath(relPath)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if err := git.RevertFile(kbRoot, ref, cleanPath); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Revert error: %v", err)), nil
	}

	if !request.GetBool("no_commit", false) {
		if err := git.CommitAndPush(kbRoot, fmt.Sprintf("revert: %s to %s", cleanPath, ref)); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error committing: %v", err)), nil
		}
	}

	return mcp.NewToolResultText(fmt.Sprintf("Reverted %s to %s", cleanPath, ref)), nil
}

func handleCommit(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	kbRoot := config.ResolveKBRoot()

	diff, err := git.Diff(kbRoot, "")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error checking changes: %v", err)), nil
	}
	if diff == "No uncommitted changes." {
		return mcp.NewToolResultText("No uncommitted changes to commit."), nil
	}

	message := request.GetString("message", "")
	if message == "" {
		lines := strings.Split(diff, "\n")
		if len(lines) == 1 {
			parts := strings.SplitN(lines[0], " ", 2)
			if len(parts) == 2 {
				message = fmt.Sprintf("auto: update %s", parts[1])
			} else {
				message = "auto: update"
			}
		} else {
			message = fmt.Sprintf("auto: update %d files", len(lines))
		}
	}

	if err := git.CommitAndPush(kbRoot, message); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error committing: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Committed: %s", message)), nil
}

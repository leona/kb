package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/leona/kb/internal/config"
	"github.com/leona/kb/internal/fs"
	"github.com/leona/kb/internal/shared"
)

// Sentinel errors for ref/global operations.
var (
	ErrAlreadyLinked = errors.New("already linked")
	ErrNotLinked     = errors.New("not linked")
	ErrAlreadyGlobal = errors.New("already global")
	ErrNotGlobal     = errors.New("not global")
)

// Info holds project summary information.
type Info struct {
	Name         string
	Path         string // registered project path on disk
	HasContext   bool
	ContextLines int
	Refs         []string
	Inline       []string // shared doc slugs inlined into context.md
	Files        []string // additional markdown files
}

// List returns info for all projects registered in kb.yml.
func List(kbRoot string) ([]Info, error) {
	cfg, err := config.Load(kbRoot)
	if err != nil {
		return nil, nil
	}

	var projects []Info
	for name, path := range cfg.Projects {
		info, err := Get(kbRoot, name)
		if err != nil {
			continue
		}
		info.Path = path
		projects = append(projects, info)
	}
	return projects, nil
}

// Get returns info for a single project.
func Get(kbRoot, name string) (Info, error) {
	dir := Dir(kbRoot, name)
	if !fs.DirExists(dir) {
		return Info{}, fmt.Errorf("project %q not found", name)
	}

	info := Info{Name: name}

	contextPath := filepath.Join(dir, "context.md")
	if fs.FileExists(contextPath) {
		info.HasContext = true
		lines, err := fs.CountLines(contextPath)
		if err == nil {
			info.ContextLines = lines
		}
	}

	refs, err := config.LoadRefs(dir)
	if err == nil {
		info.Refs = refs.Refs
		info.Inline = refs.Inline
	}

	mdFiles, err := fs.ListMarkdownFiles(dir)
	if err == nil {
		for _, f := range mdFiles {
			if f != "context.md" {
				info.Files = append(info.Files, f)
			}
		}
	}

	return info, nil
}

// Exists returns true if the project directory exists.
func Exists(kbRoot, name string) bool {
	return fs.DirExists(Dir(kbRoot, name))
}

// Dir returns the absolute path to a project's KB directory.
func Dir(kbRoot, name string) string {
	return filepath.Join(kbRoot, "projects", name)
}

// ContextPath returns the absolute path to a project's context.md.
func ContextPath(kbRoot, name string) string {
	return filepath.Join(kbRoot, "projects", name, "context.md")
}

// KBRelPath returns the ~/knowledge-base/projects/<name> style path.
func KBRelPath(kbRoot, name string) string {
	home, _ := os.UserHomeDir()
	dir := Dir(kbRoot, name)
	return strings.ReplaceAll(strings.Replace(dir, home, "~", 1), "\\", "/")
}

// Register creates the project dir, context.md, refs.yml, and registers the
// path in kb.yml. If the project already exists, it only updates the path if
// needed. Returns (created bool, err).
func Register(kbRoot, name, projectPath string) (bool, error) {
	created := !Exists(kbRoot, name)
	dir := Dir(kbRoot, name)

	if created {
		if err := fs.EnsureDir(dir); err != nil {
			return false, fmt.Errorf("creating project dir: %w", err)
		}

		kbRelPath := KBRelPath(kbRoot, name)
		contextContent := fmt.Sprintf("<!-- KB managed: %s/context.md -->\n<!-- Always edit THIS file for project context. Do NOT edit the repo's CLAUDE.md. -->\n\n# %s\n", kbRelPath, name)
		if err := os.WriteFile(filepath.Join(dir, "context.md"), []byte(contextContent), 0644); err != nil {
			return false, fmt.Errorf("creating context.md: %w", err)
		}

		refs := &config.Refs{Refs: []string{}}
		if err := config.SaveRefs(dir, refs); err != nil {
			return false, fmt.Errorf("creating refs.yml: %w", err)
		}
	}

	if projectPath != "" {
		absPath, err := filepath.Abs(projectPath)
		if err != nil {
			return created, fmt.Errorf("resolving path: %w", err)
		}

		cfg, err := config.Load(kbRoot)
		if err != nil {
			return created, err
		}
		if cfg.Projects[name] != absPath {
			cfg.Projects[name] = absPath
			if err := config.Save(kbRoot, cfg); err != nil {
				return created, err
			}
		}
	}

	return created, nil
}

// WriteImportPointers replaces CLAUDE.md (always) and AGENT.md/AGENTS.md (if
// they exist) with @import directives pointing to the KB context.md. Returns
// the list of files written.
func WriteImportPointers(projectDir, kbRoot, name string) ([]string, error) {
	kbContextPath := KBRelPath(kbRoot, name) + "/context.md"
	importDirective := "@" + kbContextPath + "\n"

	var written []string
	for _, mdName := range []string{"CLAUDE.md", "AGENT.md", "AGENTS.md"} {
		mdPath := filepath.Join(projectDir, mdName)
		existing, _ := os.ReadFile(mdPath)

		if mdName == "CLAUDE.md" || len(existing) > 0 {
			if string(existing) != importDirective {
				if err := os.WriteFile(mdPath, []byte(importDirective), 0644); err != nil {
					return written, fmt.Errorf("writing %s: %w", mdName, err)
				}
				written = append(written, mdName)
			}
		}
	}
	return written, nil
}

// Detect finds which project the current directory belongs to.
func Detect(kbRoot string) (string, error) {
	cfg, err := config.Load(kbRoot)
	if err != nil {
		return "", err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting cwd: %w", err)
	}

	name := cfg.DetectProject(cwd)
	if name == "" {
		return "", fmt.Errorf("current directory is not inside any registered project")
	}
	return name, nil
}

// AddRef links a shared doc to a project's refs.yml and regenerates the
// KB:REFS inventory. If inline is true, the doc is added to the inline list
// (embedded in context.md) instead of the refs list. Adding as one type
// removes from the other. Returns ErrAlreadyLinked if already in the target list.
func AddRef(kbRoot, projectName, slug string, inline bool) error {
	dir := Dir(kbRoot, projectName)
	refs, err := config.LoadRefs(dir)
	if err != nil {
		return err
	}
	if inline {
		if containsSlug(refs.Inline, slug) {
			return ErrAlreadyLinked
		}
		refs.Inline = append(refs.Inline, slug)
		refs.Refs = removeSlug(refs.Refs, slug)
	} else {
		if containsSlug(refs.Refs, slug) {
			return ErrAlreadyLinked
		}
		refs.Refs = append(refs.Refs, slug)
		refs.Inline = removeSlug(refs.Inline, slug)
	}
	if err := config.SaveRefs(dir, refs); err != nil {
		return err
	}
	return UpdateRefsInventory(kbRoot, projectName)
}

// RemoveRef unlinks a shared doc from a project's refs.yml (from either refs
// or inline list) and regenerates the inventory. Returns ErrNotLinked if not
// found in either list.
func RemoveRef(kbRoot, projectName, slug string) error {
	dir := Dir(kbRoot, projectName)
	refs, err := config.LoadRefs(dir)
	if err != nil {
		return err
	}
	newRefs := removeSlug(refs.Refs, slug)
	newInline := removeSlug(refs.Inline, slug)
	if len(newRefs) == len(refs.Refs) && len(newInline) == len(refs.Inline) {
		return ErrNotLinked
	}
	refs.Refs = newRefs
	refs.Inline = newInline
	if err := config.SaveRefs(dir, refs); err != nil {
		return err
	}
	return UpdateRefsInventory(kbRoot, projectName)
}

// AddGlobal marks a shared doc as globally available to all projects and
// regenerates all inventories. If inline is true, adds to inline_globals
// (embedded in context.md) instead of globals. Returns ErrAlreadyGlobal if
// already in the target list.
func AddGlobal(kbRoot, slug string, inline bool) error {
	cfg, err := config.Load(kbRoot)
	if err != nil {
		return err
	}
	if inline {
		if containsSlug(cfg.InlineGlobals, slug) {
			return ErrAlreadyGlobal
		}
		cfg.InlineGlobals = append(cfg.InlineGlobals, slug)
		cfg.Globals = removeSlug(cfg.Globals, slug)
	} else {
		if containsSlug(cfg.Globals, slug) {
			return ErrAlreadyGlobal
		}
		cfg.Globals = append(cfg.Globals, slug)
		cfg.InlineGlobals = removeSlug(cfg.InlineGlobals, slug)
	}
	if err := config.Save(kbRoot, cfg); err != nil {
		return err
	}
	return UpdateAllRefsInventories(kbRoot)
}

// RemoveGlobal removes a shared doc from globals (both globals and
// inline_globals) and regenerates all inventories. Returns ErrNotGlobal if
// not found in either list.
func RemoveGlobal(kbRoot, slug string) error {
	cfg, err := config.Load(kbRoot)
	if err != nil {
		return err
	}
	newGlobals := removeSlug(cfg.Globals, slug)
	newInlineGlobals := removeSlug(cfg.InlineGlobals, slug)
	if len(newGlobals) == len(cfg.Globals) && len(newInlineGlobals) == len(cfg.InlineGlobals) {
		return ErrNotGlobal
	}
	cfg.Globals = newGlobals
	cfg.InlineGlobals = newInlineGlobals
	if err := config.Save(kbRoot, cfg); err != nil {
		return err
	}
	return UpdateAllRefsInventories(kbRoot)
}

// RemoveGlobalEntry removes a slug from both globals and inline_globals in
// kb.yml without regenerating inventories. Use this when you'll call
// UpdateAllRefsInventories separately (e.g., during shared doc deletion).
func RemoveGlobalEntry(kbRoot, slug string) error {
	cfg, err := config.Load(kbRoot)
	if err != nil {
		return err
	}
	cfg.Globals = removeSlug(cfg.Globals, slug)
	cfg.InlineGlobals = removeSlug(cfg.InlineGlobals, slug)
	return config.Save(kbRoot, cfg)
}

// RemoveRefFromAll removes a slug from every project's refs.yml (both refs
// and inline lists). Used when deleting a shared doc entirely.
func RemoveRefFromAll(kbRoot, slug string) error {
	projectsDir := filepath.Join(kbRoot, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		projDir := filepath.Join(projectsDir, e.Name())
		refs, err := config.LoadRefs(projDir)
		if err != nil {
			continue
		}
		newRefs := removeSlug(refs.Refs, slug)
		newInline := removeSlug(refs.Inline, slug)
		if len(newRefs) != len(refs.Refs) || len(newInline) != len(refs.Inline) {
			refs.Refs = newRefs
			refs.Inline = newInline
			_ = config.SaveRefs(projDir, refs)
		}
	}
	return nil
}

func containsSlug(slugs []string, slug string) bool {
	for _, s := range slugs {
		if s == slug {
			return true
		}
	}
	return false
}

func removeSlug(slugs []string, slug string) []string {
	var result []string
	for _, s := range slugs {
		if s != slug {
			result = append(result, s)
		}
	}
	return result
}

const refsCommentStart = "<!-- KB:REFS"
const refsCommentEnd = "-->"
const inlineCommentStart = "<!-- KB:INLINE "
const inlineCommentEnd = "<!-- /KB:INLINE "

// UpdateRefsInventory regenerates the <!-- KB:REFS ... --> comment block and
// <!-- KB:INLINE ... --> blocks in a project's context.md.
func UpdateRefsInventory(kbRoot, name string) error {
	contextPath := ContextPath(kbRoot, name)
	if !fs.FileExists(contextPath) {
		return nil
	}

	content, err := fs.ReadFile(contextPath)
	if err != nil {
		return err
	}

	// Strip existing generated blocks.
	content = stripRefsComment(content)
	content = stripInlineBlocks(content)

	// Load project refs and config.
	dir := Dir(kbRoot, name)
	refs, err := config.LoadRefs(dir)
	if err != nil {
		return err
	}
	cfg, _ := config.Load(kbRoot)

	var effectiveRefs []string
	var effectiveInline []string
	if cfg != nil {
		effectiveRefs, effectiveInline = config.EffectiveRefsAndInline(cfg, refs.Refs, refs.Inline)
	} else {
		effectiveRefs = refs.Refs
		effectiveInline = refs.Inline
	}

	// Discover extra project markdown files.
	projDir := Dir(kbRoot, name)
	extraFiles, _ := fs.ListMarkdownFiles(projDir)
	var projectFiles []string
	for _, f := range extraFiles {
		if f != "context.md" {
			projectFiles = append(projectFiles, f)
		}
	}

	var insertBlock strings.Builder

	// Generate KB:REFS block for shared docs and extra project files.
	if len(effectiveRefs) > 0 || len(projectFiles) > 0 {
		insertBlock.WriteString(refsCommentStart + "\n")
		for _, slug := range effectiveRefs {
			sharedInfo, err := shared.Get(kbRoot, slug)
			if err != nil {
				continue
			}
			insertBlock.WriteString(fmt.Sprintf("  - %s (%d lines)", sharedInfo.DisplayTitle(), sharedInfo.TotalLines))
			if sharedInfo.Description != "" {
				insertBlock.WriteString(fmt.Sprintf(" — %s", sharedInfo.Description))
			}
			insertBlock.WriteString("\n")
			for _, f := range sharedInfo.Files {
				insertBlock.WriteString(fmt.Sprintf("    kb_read: shared/%s/%s\n", slug, f))
			}
		}
		for _, f := range projectFiles {
			fullPath := filepath.Join(projDir, f)
			lines, _ := fs.CountLines(fullPath)
			insertBlock.WriteString(fmt.Sprintf("  - %s (%d lines)\n", f, lines))
			insertBlock.WriteString(fmt.Sprintf("    kb_read: projects/%s/%s\n", name, f))
		}
		insertBlock.WriteString("  Use kb_search or kb_read to access these docs.\n")
		insertBlock.WriteString(refsCommentEnd + "\n")
	}

	// Generate KB:INLINE blocks.
	for _, slug := range effectiveInline {
		sharedDir := shared.Dir(kbRoot, slug)
		mdFiles, err := fs.ListMarkdownFiles(sharedDir)
		if err != nil || len(mdFiles) == 0 {
			continue
		}
		insertBlock.WriteString(fmt.Sprintf("%s%s -->\n", inlineCommentStart, slug))
		for i, f := range mdFiles {
			fileContent, err := fs.ReadFile(filepath.Join(sharedDir, f))
			if err != nil {
				continue
			}
			insertBlock.WriteString(strings.TrimRight(fileContent, "\n"))
			insertBlock.WriteString("\n")
			if i < len(mdFiles)-1 {
				insertBlock.WriteString("\n")
			}
		}
		insertBlock.WriteString(fmt.Sprintf("%s%s -->\n", inlineCommentEnd, slug))
	}

	if insertBlock.Len() > 0 {
		content = insertAfterHeader(content, insertBlock.String())
	}

	if err := os.WriteFile(contextPath, []byte(content), 0644); err != nil {
		return err
	}

	return BackupContext(cfg, kbRoot, name)
}

// BackupContext copies context.md to .kb-context.md in the project directory.
// Silently returns nil if cfg is nil, the project path is not configured,
// the directory doesn't exist, or the source context.md doesn't exist.
func BackupContext(cfg *config.Config, kbRoot, name string) error {
	if cfg == nil {
		return nil
	}
	projectPath := cfg.Projects[name]
	if projectPath == "" {
		return nil
	}
	if !fs.DirExists(projectPath) {
		return nil
	}
	src := ContextPath(kbRoot, name)
	if !fs.FileExists(src) {
		return nil
	}
	dst := filepath.Join(projectPath, ".kb-context.md")
	return fs.CopyFile(src, dst)
}

// stripInlineBlocks removes all <!-- KB:INLINE slug -->...<!-- /KB:INLINE slug --> blocks.
func stripInlineBlocks(content string) string {
	for {
		startIdx := strings.Index(content, inlineCommentStart)
		if startIdx == -1 {
			break
		}
		// Find the end of the opening tag line (<!-- KB:INLINE slug -->).
		lineEnd := strings.Index(content[startIdx:], "\n")
		if lineEnd == -1 {
			// Malformed — strip to end.
			content = content[:startIdx]
			break
		}
		// Extract slug from opening tag.
		tagContent := content[startIdx+len(inlineCommentStart) : startIdx+lineEnd]
		tagContent = strings.TrimSuffix(strings.TrimSpace(tagContent), "-->")
		slug := strings.TrimSpace(tagContent)

		// Find the matching closing tag.
		closeTag := fmt.Sprintf("%s%s -->", inlineCommentEnd, slug)
		closeIdx := strings.Index(content[startIdx:], closeTag)
		if closeIdx == -1 {
			// Malformed — strip from start marker to end of file.
			content = content[:startIdx]
			break
		}
		endPos := startIdx + closeIdx + len(closeTag)
		// Also strip trailing newline.
		if endPos < len(content) && content[endPos] == '\n' {
			endPos++
		}
		content = content[:startIdx] + content[endPos:]
	}
	return content
}

// UpdateAllRefsInventories regenerates the refs inventory for every project.
// Called when globals change (affects all projects).
func UpdateAllRefsInventories(kbRoot string) error {
	projects, err := List(kbRoot)
	if err != nil {
		return err
	}
	for _, p := range projects {
		if err := UpdateRefsInventory(kbRoot, p.Name); err != nil {
			return err
		}
	}
	return nil
}

// stripRefsComment removes the <!-- KB:REFS ... --> block from content.
func stripRefsComment(content string) string {
	startIdx := strings.Index(content, refsCommentStart)
	if startIdx == -1 {
		return content
	}
	// Find the closing --> after the start marker.
	rest := content[startIdx+len(refsCommentStart):]
	endIdx := strings.Index(rest, refsCommentEnd)
	if endIdx == -1 {
		// Malformed — strip from start marker to end of file.
		return content[:startIdx]
	}
	endPos := startIdx + len(refsCommentStart) + endIdx + len(refsCommentEnd)
	// Also strip the trailing newline if present.
	if endPos < len(content) && content[endPos] == '\n' {
		endPos++
	}
	return content[:startIdx] + content[endPos:]
}

// insertAfterHeader inserts text after the <!-- KB managed --> header comments
// at the top of context.md, before the first non-comment content.
func insertAfterHeader(content, insert string) string {
	lines := strings.SplitAfter(content, "\n")
	insertIdx := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "<!--") {
			// Keep scanning past header comments.
			insertIdx = i + 1
			continue
		}
		break
	}
	// Insert after the last header comment line.
	before := strings.Join(lines[:insertIdx], "")
	after := strings.Join(lines[insertIdx:], "")
	return before + insert + after
}

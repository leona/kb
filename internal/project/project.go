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
// KB:REFS inventory. Returns ErrAlreadyLinked if already linked.
func AddRef(kbRoot, projectName, slug string) error {
	dir := Dir(kbRoot, projectName)
	refs, err := config.LoadRefs(dir)
	if err != nil {
		return err
	}
	for _, r := range refs.Refs {
		if r == slug {
			return ErrAlreadyLinked
		}
	}
	refs.Refs = append(refs.Refs, slug)
	if err := config.SaveRefs(dir, refs); err != nil {
		return err
	}
	return UpdateRefsInventory(kbRoot, projectName)
}

// RemoveRef unlinks a shared doc from a project's refs.yml and regenerates the
// KB:REFS inventory. Returns ErrNotLinked if not linked.
func RemoveRef(kbRoot, projectName, slug string) error {
	dir := Dir(kbRoot, projectName)
	refs, err := config.LoadRefs(dir)
	if err != nil {
		return err
	}
	found := false
	var newRefs []string
	for _, r := range refs.Refs {
		if r == slug {
			found = true
		} else {
			newRefs = append(newRefs, r)
		}
	}
	if !found {
		return ErrNotLinked
	}
	refs.Refs = newRefs
	if err := config.SaveRefs(dir, refs); err != nil {
		return err
	}
	return UpdateRefsInventory(kbRoot, projectName)
}

// AddGlobal marks a shared doc as globally available to all projects and
// regenerates all KB:REFS inventories. Returns ErrAlreadyGlobal if already global.
func AddGlobal(kbRoot, slug string) error {
	cfg, err := config.Load(kbRoot)
	if err != nil {
		return err
	}
	for _, g := range cfg.Globals {
		if g == slug {
			return ErrAlreadyGlobal
		}
	}
	cfg.Globals = append(cfg.Globals, slug)
	if err := config.Save(kbRoot, cfg); err != nil {
		return err
	}
	return UpdateAllRefsInventories(kbRoot)
}

// RemoveGlobal removes a shared doc from globals and regenerates all KB:REFS
// inventories. Returns ErrNotGlobal if not global.
func RemoveGlobal(kbRoot, slug string) error {
	cfg, err := config.Load(kbRoot)
	if err != nil {
		return err
	}
	found := false
	var newGlobals []string
	for _, g := range cfg.Globals {
		if g == slug {
			found = true
		} else {
			newGlobals = append(newGlobals, g)
		}
	}
	if !found {
		return ErrNotGlobal
	}
	cfg.Globals = newGlobals
	if err := config.Save(kbRoot, cfg); err != nil {
		return err
	}
	return UpdateAllRefsInventories(kbRoot)
}

// RemoveGlobalEntry removes a slug from the globals list in kb.yml without
// regenerating inventories. Use this when you'll call UpdateAllRefsInventories
// separately (e.g., during shared doc deletion).
func RemoveGlobalEntry(kbRoot, slug string) error {
	cfg, err := config.Load(kbRoot)
	if err != nil {
		return err
	}
	var newGlobals []string
	for _, g := range cfg.Globals {
		if g != slug {
			newGlobals = append(newGlobals, g)
		}
	}
	cfg.Globals = newGlobals
	return config.Save(kbRoot, cfg)
}

// RemoveRefFromAll removes a slug from every project's refs.yml.
// Used when deleting a shared doc entirely.
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
		var newRefs []string
		changed := false
		for _, r := range refs.Refs {
			if r == slug {
				changed = true
			} else {
				newRefs = append(newRefs, r)
			}
		}
		if changed {
			refs.Refs = newRefs
			_ = config.SaveRefs(projDir, refs)
		}
	}
	return nil
}

const refsCommentStart = "<!-- KB:REFS"
const refsCommentEnd = "-->"

// UpdateRefsInventory regenerates the <!-- KB:REFS ... --> comment block in
// a project's context.md. This ensures the @import-expanded content includes
// an inventory of linked shared docs visible to agents at session start.
func UpdateRefsInventory(kbRoot, name string) error {
	contextPath := ContextPath(kbRoot, name)
	if !fs.FileExists(contextPath) {
		return nil
	}

	content, err := fs.ReadFile(contextPath)
	if err != nil {
		return err
	}

	// Strip existing KB:REFS block.
	content = stripRefsComment(content)

	// Build effective refs list.
	info, err := Get(kbRoot, name)
	if err != nil {
		return err
	}
	cfg, _ := config.Load(kbRoot)
	var effectiveRefs []string
	if cfg != nil {
		effectiveRefs = config.EffectiveRefs(cfg, info.Refs)
	} else {
		effectiveRefs = info.Refs
	}

	// Generate the new refs comment block.
	if len(effectiveRefs) > 0 {
		var sb strings.Builder
		sb.WriteString(refsCommentStart + "\n")
		for _, slug := range effectiveRefs {
			sharedInfo, err := shared.Get(kbRoot, slug)
			if err != nil {
				continue
			}
			sb.WriteString(fmt.Sprintf("  - %s (%d lines)", sharedInfo.DisplayTitle(), sharedInfo.TotalLines))
			if sharedInfo.Description != "" {
				sb.WriteString(fmt.Sprintf(" — %s", sharedInfo.Description))
			}
			sb.WriteString("\n")
			for _, f := range sharedInfo.Files {
				sb.WriteString(fmt.Sprintf("    kb_read: shared/%s/%s\n", slug, f))
			}
		}
		sb.WriteString("  Use kb_search or kb_read to access these docs.\n")
		sb.WriteString(refsCommentEnd + "\n")

		// Insert after the directive header comments, before the first content.
		content = insertAfterHeader(content, sb.String())
	}

	return os.WriteFile(contextPath, []byte(content), 0644)
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

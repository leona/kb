package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/leona/kb/internal/config"
	"github.com/leona/kb/internal/fs"
)

// Info holds shared doc summary information.
type Info struct {
	Slug        string
	Title       string
	Description string   // 1-2 sentence summary from meta.yml
	Tags        []string // from meta.yml
	Files       []string
	TotalLines  int
	UsedBy      []string // project names that reference this
}

// DisplayTitle returns Title if set, otherwise falls back to Slug.
func (i Info) DisplayTitle() string {
	if i.Title != "" {
		return i.Title
	}
	return i.Slug
}

// List returns info for all shared docs in the KB.
func List(kbRoot string) ([]Info, error) {
	sharedDir := filepath.Join(kbRoot, "shared")
	entries, err := os.ReadDir(sharedDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var docs []Info
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := Get(kbRoot, e.Name())
		if err != nil {
			continue
		}
		docs = append(docs, info)
	}
	return docs, nil
}

// Get returns info for a single shared doc.
func Get(kbRoot, slug string) (Info, error) {
	dir := Dir(kbRoot, slug)
	if !fs.DirExists(dir) {
		return Info{}, fmt.Errorf("shared doc %q not found", slug)
	}

	info := Info{Slug: slug}

	meta := config.LoadMeta(dir)
	if meta != nil {
		info.Title = meta.Title
		info.Description = meta.Description
		info.Tags = meta.Tags
	}

	mdFiles, err := fs.ListMarkdownFiles(dir)
	if err == nil {
		info.Files = mdFiles
		for _, f := range mdFiles {
			lines, err := fs.CountLines(filepath.Join(dir, f))
			if err == nil {
				info.TotalLines += lines
			}
		}
	}

	info.UsedBy = FindUsedBy(kbRoot, slug)

	return info, nil
}

// Exists returns true if the shared doc directory exists.
func Exists(kbRoot, slug string) bool {
	return fs.DirExists(Dir(kbRoot, slug))
}

// Dir returns the absolute path to a shared doc's directory.
func Dir(kbRoot, slug string) string {
	return filepath.Join(kbRoot, "shared", slug)
}

// FindUsedBy scans all projects' refs.yml and globals to find which projects
// effectively reference this slug (via explicit ref or global).
func FindUsedBy(kbRoot, slug string) []string {
	cfg, _ := config.Load(kbRoot)
	isGlobal := false
	if cfg != nil {
		for _, g := range cfg.Globals {
			if g == slug {
				isGlobal = true
				break
			}
		}
	}

	projectsDir := filepath.Join(kbRoot, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil
	}

	var users []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if isGlobal {
			users = append(users, e.Name())
			continue
		}
		refs, err := config.LoadRefs(filepath.Join(projectsDir, e.Name()))
		if err != nil {
			continue
		}
		for _, ref := range refs.Refs {
			if ref == slug {
				users = append(users, e.Name())
				break
			}
		}
	}
	return users
}

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the kb.yml configuration file.
type Config struct {
	Version       int               `yaml:"version"`
	Editor        string            `yaml:"editor,omitempty"`
	Globals       []string          `yaml:"globals,omitempty"`        // shared doc slugs available to all projects
	InlineGlobals []string          `yaml:"inline_globals,omitempty"` // shared doc slugs inlined into all projects' context.md
	Projects      map[string]string `yaml:"projects,omitempty"`       // name → absolute path
}

// Refs represents a project's refs.yml file.
type Refs struct {
	Refs   []string `yaml:"refs"`
	Inline []string `yaml:"inline,omitempty"` // shared doc slugs inlined into context.md
}

// Meta represents a shared doc's meta.yml file.
type Meta struct {
	Title       string   `yaml:"title,omitempty"`
	Description string   `yaml:"description,omitempty"` // 1-2 sentence summary for agent discovery
	Source      string   `yaml:"source,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
	GeneratedAt string   `yaml:"generated_at,omitempty"`
}

// DefaultKBRoot returns the default knowledge base root directory.
func DefaultKBRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	return filepath.Join(home, "knowledge-base")
}

// ResolveKBRoot determines the KB root directory from:
// 1. KB_ROOT environment variable
// 2. Default ~/knowledge-base
func ResolveKBRoot() string {
	if root := os.Getenv("KB_ROOT"); root != "" {
		return expandHome(root)
	}
	return DefaultKBRoot()
}

// Load reads and parses kb.yml from the given KB root.
func Load(kbRoot string) (*Config, error) {
	path := filepath.Join(kbRoot, "kb.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading kb.yml: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing kb.yml: %w", err)
	}

	if cfg.Projects == nil {
		cfg.Projects = make(map[string]string)
	}

	// Expand ~ in project paths
	for name, path := range cfg.Projects {
		cfg.Projects[name] = expandHome(path)
	}

	return &cfg, nil
}

// Save writes the config back to kb.yml.
func Save(kbRoot string, cfg *Config) error {
	path := filepath.Join(kbRoot, "kb.yml")

	// Contract ~ back in project paths for readability
	saveCfg := *cfg
	saveCfg.Projects = make(map[string]string, len(cfg.Projects))
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home directory: %w", err)
	}
	for name, p := range cfg.Projects {
		saveCfg.Projects[name] = contractHome(p, home)
	}

	data, err := yaml.Marshal(&saveCfg)
	if err != nil {
		return fmt.Errorf("marshaling kb.yml: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// LoadRefs reads a project's refs.yml.
func LoadRefs(projectDir string) (*Refs, error) {
	path := filepath.Join(projectDir, "refs.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Refs{}, nil
		}
		return nil, fmt.Errorf("reading refs.yml: %w", err)
	}

	var refs Refs
	if err := yaml.Unmarshal(data, &refs); err != nil {
		return nil, fmt.Errorf("parsing refs.yml: %w", err)
	}
	return &refs, nil
}

// SaveRefs writes a project's refs.yml.
func SaveRefs(projectDir string, refs *Refs) error {
	path := filepath.Join(projectDir, "refs.yml")
	data, err := yaml.Marshal(refs)
	if err != nil {
		return fmt.Errorf("marshaling refs.yml: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// LoadMeta reads a shared doc's meta.yml. Returns nil if not found.
func LoadMeta(sharedDir string) *Meta {
	path := filepath.Join(sharedDir, "meta.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var meta Meta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil
	}
	return &meta
}

// SaveMeta writes a shared doc's meta.yml.
func SaveMeta(sharedDir string, meta *Meta) error {
	path := filepath.Join(sharedDir, "meta.yml")
	data, err := yaml.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshaling meta.yml: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// DetectProject finds which project the given directory belongs to
// by checking if cwd is inside any registered project path.
func (c *Config) DetectProject(cwd string) string {
	cwd = filepath.Clean(expandHome(cwd))
	// Find the longest matching prefix (most specific match)
	bestMatch := ""
	bestName := ""
	for name, projectPath := range c.Projects {
		projectPath = filepath.Clean(expandHome(projectPath))
		if strings.HasPrefix(cwd, projectPath+string(filepath.Separator)) || cwd == projectPath {
			if len(projectPath) > len(bestMatch) {
				bestMatch = projectPath
				bestName = name
			}
		}
	}
	return bestName
}

// EffectiveRefs returns the merged list of a project's refs and global shared
// docs, deduplicated and with globals first.
func EffectiveRefs(cfg *Config, projectRefs []string) []string {
	seen := make(map[string]bool, len(cfg.Globals)+len(projectRefs))
	var result []string
	for _, slug := range cfg.Globals {
		if !seen[slug] {
			seen[slug] = true
			result = append(result, slug)
		}
	}
	for _, slug := range projectRefs {
		if !seen[slug] {
			seen[slug] = true
			result = append(result, slug)
		}
	}
	return result
}

// EffectiveInline returns the merged list of a project's inline slugs and global
// inline shared docs, deduplicated and with globals first.
func EffectiveInline(cfg *Config, projectInline []string) []string {
	seen := make(map[string]bool, len(cfg.InlineGlobals)+len(projectInline))
	var result []string
	for _, slug := range cfg.InlineGlobals {
		if !seen[slug] {
			seen[slug] = true
			result = append(result, slug)
		}
	}
	for _, slug := range projectInline {
		if !seen[slug] {
			seen[slug] = true
			result = append(result, slug)
		}
	}
	return result
}

// EffectiveRefsAndInline computes both effective lists and ensures mutual
// exclusivity: any slug in the inline list is removed from the refs list.
func EffectiveRefsAndInline(cfg *Config, projectRefs, projectInline []string) (refs, inline []string) {
	inline = EffectiveInline(cfg, projectInline)
	allRefs := EffectiveRefs(cfg, projectRefs)

	inlineSet := make(map[string]bool, len(inline))
	for _, slug := range inline {
		inlineSet[slug] = true
	}
	for _, slug := range allRefs {
		if !inlineSet[slug] {
			refs = append(refs, slug)
		}
	}
	return refs, inline
}

// GetEditor returns the configured editor, falling back to $EDITOR, then "vi".
func (c *Config) GetEditor() string {
	if c.Editor != "" {
		return c.Editor
	}
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	return "vi"
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func contractHome(path, home string) string {
	sep := string(filepath.Separator)
	if strings.HasPrefix(path, home+sep) {
		return "~/" + path[len(home)+1:]
	}
	return path
}

package graph

import (
	"sort"

	"github.com/leona/kb/internal/config"
	"github.com/leona/kb/internal/project"
	"github.com/leona/kb/internal/shared"
)

type Node struct {
	ID    string
	Label string
}

type EdgeType int

const (
	Ref EdgeType = iota
	Inline
	Global
	GlobalInline
)

type Edge struct {
	From string // project ID
	To   string // shared doc ID
	Type EdgeType
}

type Graph struct {
	Projects []Node
	Shared   []Node
	Edges    []Edge
}

// Build constructs a graph of all projects and shared docs in the KB.
func Build(kbRoot string) (*Graph, error) {
	cfg, err := config.Load(kbRoot)
	if err != nil {
		return nil, err
	}

	projects, err := project.List(kbRoot)
	if err != nil {
		return nil, err
	}

	sharedDocs, err := shared.List(kbRoot)
	if err != nil {
		return nil, err
	}

	g := &Graph{}

	// Sort projects by name for stable output.
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Name < projects[j].Name
	})

	for _, p := range projects {
		g.Projects = append(g.Projects, Node{ID: p.Name, Label: p.Name})
	}

	// Build sets for globals/inline globals.
	globalSet := make(map[string]bool)
	for _, slug := range cfg.Globals {
		globalSet[slug] = true
	}
	inlineGlobalSet := make(map[string]bool)
	for _, slug := range cfg.InlineGlobals {
		inlineGlobalSet[slug] = true
	}

	// Generate edges: globals apply to all projects.
	for _, p := range projects {
		for _, slug := range cfg.Globals {
			g.Edges = append(g.Edges, Edge{From: p.Name, To: slug, Type: Global})
		}
		for _, slug := range cfg.InlineGlobals {
			g.Edges = append(g.Edges, Edge{From: p.Name, To: slug, Type: GlobalInline})
		}
		// Per-project refs (skip if already covered by global).
		for _, slug := range p.Refs {
			if globalSet[slug] || inlineGlobalSet[slug] {
				continue
			}
			g.Edges = append(g.Edges, Edge{From: p.Name, To: slug, Type: Ref})
		}
		for _, slug := range p.Inline {
			if globalSet[slug] || inlineGlobalSet[slug] {
				continue
			}
			g.Edges = append(g.Edges, Edge{From: p.Name, To: slug, Type: Inline})
		}
	}

	for _, s := range sharedDocs {
		g.Shared = append(g.Shared, Node{ID: s.Slug, Label: s.DisplayTitle()})
	}

	return g, nil
}

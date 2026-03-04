package graph

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	projectStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true) // blue bold
	sharedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))            // yellow
	dimStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))             // gray (also used for ref edges)
	inlineEdgeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))            // green
	globalEdgeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))            // magenta
)

// Render produces a colored tree-style graph string.
func Render(g *Graph) string {
	if len(g.Projects) == 0 && len(g.Shared) == 0 {
		return "No projects or shared docs found.\n"
	}
	if len(g.Projects) == 0 {
		return "No projects found.\n"
	}
	if len(g.Shared) == 0 {
		return "No shared docs found.\n"
	}

	// Build shared doc label map.
	sharedLabels := make(map[string]string)
	for _, n := range g.Shared {
		sharedLabels[n.ID] = n.Label
	}

	// Group edges by project.
	projEdges := make(map[string][]Edge)
	for _, e := range g.Edges {
		projEdges[e.From] = append(projEdges[e.From], e)
	}

	// Find max shared doc label length for dot leader alignment.
	maxLabel := 0
	for _, n := range g.Shared {
		if len(n.Label) > maxLabel {
			maxLabel = len(n.Label)
		}
	}

	var sb strings.Builder

	for i, proj := range g.Projects {
		if i > 0 {
			sb.WriteRune('\n')
		}

		sb.WriteString(projectStyle.Render(proj.Label))
		sb.WriteRune('\n')

		edges := projEdges[proj.ID]
		if len(edges) == 0 {
			sb.WriteString(dimStyle.Render("  (no connections)"))
			sb.WriteRune('\n')
			continue
		}

		// Sort edges by shared doc label.
		sort.Slice(edges, func(a, b int) bool {
			return sharedLabels[edges[a].To] < sharedLabels[edges[b].To]
		})

		for j, e := range edges {
			isLast := j == len(edges)-1

			branch := "├── "
			if isLast {
				branch = "└── "
			}

			label := sharedLabels[e.To]
			tag := edgeTypeTag(e.Type)
			tagStyle := edgeTypeStyle(e.Type)

			dots := strings.Repeat(" ", maxLabel-len(label)+1) + dimStyle.Render(strings.Repeat("·", 2)) + " "

			sb.WriteString(dimStyle.Render("  "+branch) + sharedStyle.Render(label) + dots + tagStyle.Render(tag))
			sb.WriteRune('\n')
		}
	}

	// Orphaned shared docs.
	connected := make(map[string]bool)
	for _, e := range g.Edges {
		connected[e.To] = true
	}
	var orphans []Node
	for _, n := range g.Shared {
		if !connected[n.ID] {
			orphans = append(orphans, n)
		}
	}
	if len(orphans) > 0 {
		sb.WriteRune('\n')
		sb.WriteString(dimStyle.Render("Orphaned shared docs:"))
		sb.WriteRune('\n')
		for _, n := range orphans {
			sb.WriteString(dimStyle.Render("  ○ ") + sharedStyle.Render(n.Label))
			sb.WriteRune('\n')
		}
	}

	// Legend.
	sb.WriteRune('\n')
	sb.WriteString(renderLegend(g))

	return sb.String()
}

func edgeTypeTag(t EdgeType) string {
	switch t {
	case Inline:
		return "inline"
	case Global:
		return "global"
	case GlobalInline:
		return "global+inline"
	default:
		return "ref"
	}
}

func edgeTypeStyle(t EdgeType) lipgloss.Style {
	switch t {
	case Inline:
		return inlineEdgeStyle
	case Global:
		return globalEdgeStyle
	case GlobalInline:
		return inlineEdgeStyle
	default:
		return dimStyle
	}
}

func renderLegend(g *Graph) string {
	hasRef, hasInline, hasGlobal, hasGlobalInline := false, false, false, false
	for _, e := range g.Edges {
		switch e.Type {
		case Ref:
			hasRef = true
		case Inline:
			hasInline = true
		case Global:
			hasGlobal = true
		case GlobalInline:
			hasGlobalInline = true
		}
	}

	desc := lipgloss.NewStyle().Foreground(lipgloss.Color("7"))

	var lines []string
	if hasRef {
		lines = append(lines, dimStyle.Render("  ref            ")+desc.Render("linked via refs.yml, agent reads on demand"))
	}
	if hasInline {
		lines = append(lines, inlineEdgeStyle.Render("  inline         ")+desc.Render("linked via refs.yml, embedded in context.md"))
	}
	if hasGlobal {
		lines = append(lines, globalEdgeStyle.Render("  global         ")+desc.Render("available to all projects, agent reads on demand"))
	}
	if hasGlobalInline {
		lines = append(lines, inlineEdgeStyle.Render("  global+inline  ")+desc.Render("available to all projects, embedded in context.md"))
	}

	if len(lines) == 0 {
		return ""
	}
	return desc.Render("Legend:") + "\n" + strings.Join(lines, "\n") + "\n"
}


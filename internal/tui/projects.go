package tui

import (
	"fmt"
	"strings"

	"github.com/leona/kb/internal/project"
	"github.com/leona/kb/internal/shared"
)

// sharedRefItem implements list.DefaultItem for the shared refs list in project detail.
// linkMode: "" (unlinked), "ref", "inline"
// globalMode: "" (not global), "global", "inline" — when set, item is pinned to top and not toggleable
type sharedRefItem struct {
	info       shared.Info
	linkMode   string
	globalMode string
}

func (i sharedRefItem) Title() string {
	if i.globalMode != "" {
		prefix := "✓"
		if i.globalMode == "inline" {
			prefix = "▪"
		}
		return fmt.Sprintf("%s %s [GLOBAL]", prefix, i.info.Slug)
	}
	switch i.linkMode {
	case "ref":
		return "✓ " + i.info.Slug
	case "inline":
		return "▪ " + i.info.Slug
	default:
		return "  " + i.info.Slug
	}
}
func (i sharedRefItem) FilterValue() string { return i.info.Slug }
func (i sharedRefItem) Description() string {
	if i.globalMode != "" {
		mode := "ref"
		if i.globalMode == "inline" {
			mode = "inline"
		}
		return fmt.Sprintf("global %s — not editable per-project", mode)
	}
	desc := fmt.Sprintf("%d lines  %d files", i.info.TotalLines, len(i.info.Files))
	if i.linkMode == "inline" {
		desc += "  (inline)"
	}
	if len(i.info.UsedBy) > 0 {
		desc += fmt.Sprintf("  used by %s", strings.Join(i.info.UsedBy, ", "))
	}
	return desc
}

// projectItem implements list.DefaultItem for the projects list.
type projectItem struct {
	info project.Info
}

func (i projectItem) Title() string      { return i.info.Name }
func (i projectItem) FilterValue() string { return i.info.Name }
func (i projectItem) Description() string {
	refs := len(i.info.Refs)
	files := len(i.info.Files)
	if i.info.HasContext {
		files++
	}
	return fmt.Sprintf("%d lines  %d files  %d refs", i.info.ContextLines, files, refs)
}

// sharedItem implements list.DefaultItem for the shared docs list.
// globalMode: "" (not global), "global", "inline"
type sharedItem struct {
	info       shared.Info
	globalMode string
}

func (i sharedItem) Title() string {
	switch i.globalMode {
	case "global":
		return "✓ " + i.info.Slug
	case "inline":
		return "▪ " + i.info.Slug
	default:
		return "  " + i.info.Slug
	}
}
func (i sharedItem) FilterValue() string { return i.info.Slug }
func (i sharedItem) Description() string {
	desc := fmt.Sprintf("%d lines  %d files", i.info.TotalLines, len(i.info.Files))
	if i.globalMode == "inline" {
		desc += "  (inline)"
	}
	if len(i.info.UsedBy) > 0 {
		desc += fmt.Sprintf("  used by %s", strings.Join(i.info.UsedBy, ", "))
	}
	return desc
}

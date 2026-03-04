package tui

import (
	"fmt"
	"strings"

	"github.com/leona/kb/internal/project"
	"github.com/leona/kb/internal/shared"
)

// sharedRefItem implements list.DefaultItem for the shared refs list in project detail.
// It shows a ✓ prefix when the shared doc is linked to the current project.
type sharedRefItem struct {
	info   shared.Info
	linked bool
}

func (i sharedRefItem) Title() string {
	if i.linked {
		return "✓ " + i.info.Slug
	}
	return "  " + i.info.Slug
}
func (i sharedRefItem) FilterValue() string { return i.info.Slug }
func (i sharedRefItem) Description() string {
	desc := fmt.Sprintf("%d lines  %d files", i.info.TotalLines, len(i.info.Files))
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
type sharedItem struct {
	info shared.Info
}

func (i sharedItem) Title() string      { return i.info.Slug }
func (i sharedItem) FilterValue() string { return i.info.Slug }
func (i sharedItem) Description() string {
	desc := fmt.Sprintf("%d lines  %d files", i.info.TotalLines, len(i.info.Files))
	if len(i.info.UsedBy) > 0 {
		desc += fmt.Sprintf("  used by %s", strings.Join(i.info.UsedBy, ", "))
	}
	return desc
}

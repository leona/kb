package tui

import "fmt"

// fileItem implements list.DefaultItem for the project detail list.
type fileItem struct {
	name     string // display name
	path     string // absolute path on disk
	isShared bool   // true if this is a shared doc file
	slug     string // shared doc slug (if isShared)
}

func (i fileItem) Title() string      { return i.name }
func (i fileItem) FilterValue() string { return i.name }
func (i fileItem) Description() string {
	if i.isShared {
		return fmt.Sprintf("shared/%s", i.slug)
	}
	return "project file"
}

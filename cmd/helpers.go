package cmd

import (
	"fmt"

	"github.com/leona/kb/internal/project"
	"github.com/leona/kb/internal/shared"
)

// resolveScopePath converts a scope argument (project name or shared doc slug)
// into a relative path prefix for git operations.
func resolveScopePath(kbRoot string, args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	scope := args[0]
	if project.Exists(kbRoot, scope) {
		return "projects/" + scope, nil
	}
	if shared.Exists(kbRoot, scope) {
		return "shared/" + scope, nil
	}
	return "", fmt.Errorf("%q is not a known project or shared doc", scope)
}

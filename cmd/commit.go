package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/leona/kb/internal/config"
	"github.com/leona/kb/internal/git"
	"github.com/spf13/cobra"
)

var commitHook bool

var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Commit uncommitted knowledge base changes",
	Long: `Stages and commits any uncommitted changes in the KB git repo.

With --hook, reads PostToolUse JSON from stdin and only commits if the edited
file is inside the KB directory. Used by the Claude Code auto-commit hook.
Without --hook, commits all pending changes. Used by OpenCode's file_edited hook.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()

		if commitHook {
			return runHookCommit(kbRoot)
		}

		return runManualCommit(kbRoot)
	},
}

func runManualCommit(kbRoot string) error {
	msg := git.GenerateCommitMessage(kbRoot)
	if err := git.AutoCommit(kbRoot, msg); err != nil {
		return fmt.Errorf("commit failed: %w", err)
	}
	fmt.Println("Committed KB changes.")
	return nil
}

type hookInput struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
}

func runHookCommit(kbRoot string) error {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil // silently ignore stdin errors in hook mode
	}

	var input hookInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil
	}

	filePath := input.ToolInput.FilePath
	if filePath == "" {
		return nil
	}

	// Resolve to absolute path
	if !filepath.IsAbs(filePath) {
		cwd, _ := os.Getwd()
		filePath = filepath.Join(cwd, filePath)
	}
	filePath = filepath.Clean(filePath)

	// Check if the file is inside the KB directory
	kbRootClean := filepath.Clean(kbRoot)
	if !strings.HasPrefix(filePath, kbRootClean+string(filepath.Separator)) {
		return nil // not a KB file, skip
	}

	// Generate a commit message from the relative path
	relPath, _ := filepath.Rel(kbRoot, filePath)
	msg := fmt.Sprintf("auto: update %s", relPath)

	return git.AutoCommit(kbRoot, msg)
}

func init() {
	commitCmd.Flags().BoolVar(&commitHook, "hook", false, "Read PostToolUse hook JSON from stdin and filter by KB path")
	rootCmd.AddCommand(commitCmd)
}

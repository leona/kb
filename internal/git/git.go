package git

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"strings"
	"time"

	"github.com/leona/kb/internal/config"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var errLimitReached = errors.New("limit reached")

// Init initializes a new git repository at the given path.
func Init(path string) error {
	_, err := gogit.PlainInit(path, false)
	if err != nil {
		if err == gogit.ErrRepositoryAlreadyExists {
			return nil
		}
		return fmt.Errorf("git init: %w", err)
	}
	return nil
}

// AutoCommit stages all changes and commits with the given message.
// Returns nil if there are no changes to commit.
func AutoCommit(kbRoot, message string) error {
	repo, err := gogit.PlainOpen(kbRoot)
	if err != nil {
		return fmt.Errorf("opening repo: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("getting worktree: %w", err)
	}

	if err := w.AddGlob("."); err != nil {
		return fmt.Errorf("staging changes: %w", err)
	}

	status, err := w.Status()
	if err != nil {
		return fmt.Errorf("getting status: %w", err)
	}

	hasChanges := false
	for _, s := range status {
		if s.Staging != gogit.Unmodified || s.Worktree != gogit.Unmodified {
			hasChanges = true
			break
		}
	}
	if !hasChanges {
		return nil
	}

	_, err = w.Commit(message, &gogit.CommitOptions{
		All: true,
		Author: &object.Signature{
			Name:  "kb",
			Email: "kb@local",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("committing: %w", err)
	}

	return nil
}

// Log returns commit history, optionally scoped to a path.
func Log(kbRoot, scopePath string, maxEntries int) ([]string, error) {
	repo, err := gogit.PlainOpen(kbRoot)
	if err != nil {
		return nil, fmt.Errorf("opening repo: %w", err)
	}

	logOpts := &gogit.LogOptions{}
	if scopePath != "" {
		logOpts.PathFilter = func(path string) bool {
			return strings.HasPrefix(path, scopePath)
		}
	}

	iter, err := repo.Log(logOpts)
	if err != nil {
		return nil, fmt.Errorf("getting log: %w", err)
	}

	var entries []string
	count := 0
	err = iter.ForEach(func(c *object.Commit) error {
		if maxEntries > 0 && count >= maxEntries {
			return errLimitReached
		}
		entry := fmt.Sprintf("%s %s %s",
			c.Hash.String()[:7],
			c.Author.When.Format("2006-01-02 15:04"),
			c.Message,
		)
		entries = append(entries, entry)
		count++
		return nil
	})
	if err != nil && !errors.Is(err, errLimitReached) {
		return nil, err
	}

	return entries, nil
}

// Diff returns uncommitted changes, optionally scoped to a path.
func Diff(kbRoot, scopePath string) (string, error) {
	repo, err := gogit.PlainOpen(kbRoot)
	if err != nil {
		return "", fmt.Errorf("opening repo: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("getting worktree: %w", err)
	}

	status, err := w.Status()
	if err != nil {
		return "", fmt.Errorf("getting status: %w", err)
	}

	var lines []string
	for path, s := range status {
		if scopePath != "" && !strings.HasPrefix(path, scopePath) {
			continue
		}
		var statusChar string
		switch {
		case s.Staging == gogit.Added || s.Worktree == gogit.Untracked:
			statusChar = "A"
		case s.Staging == gogit.Deleted || s.Worktree == gogit.Deleted:
			statusChar = "D"
		case s.Staging == gogit.Modified || s.Worktree == gogit.Modified:
			statusChar = "M"
		default:
			statusChar = "?"
		}
		lines = append(lines, fmt.Sprintf("%s %s", statusChar, path))
	}

	if len(lines) == 0 {
		return "No uncommitted changes.", nil
	}
	return strings.Join(lines, "\n"), nil
}

// Show returns the content of a file at a specific commit.
func Show(kbRoot, ref, filePath string) (string, error) {
	repo, err := gogit.PlainOpen(kbRoot)
	if err != nil {
		return "", fmt.Errorf("opening repo: %w", err)
	}

	hash, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return "", fmt.Errorf("resolving ref %q: %w", ref, err)
	}

	commit, err := repo.CommitObject(*hash)
	if err != nil {
		return "", fmt.Errorf("getting commit: %w", err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return "", fmt.Errorf("getting tree: %w", err)
	}

	file, err := tree.File(filePath)
	if err != nil {
		return "", fmt.Errorf("file %q not found in %s: %w", filePath, ref, err)
	}

	content, err := file.Contents()
	if err != nil {
		return "", fmt.Errorf("reading file contents: %w", err)
	}

	return content, nil
}

// RevertFile restores a file to a previous version without committing.
func RevertFile(kbRoot, ref, filePath string) error {
	content, err := Show(kbRoot, ref, filePath)
	if err != nil {
		return err
	}

	fullPath := filepath.Join(kbRoot, filePath)
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}

var (
	debounceMu    sync.Mutex
	debounceTimer *time.Timer
	debounceRoot  string
	debouncePush  bool
)

// DebouncedCommitAndPush batches rapid mutations into a single commit+push
// after 4 seconds of inactivity. Use for interactive contexts (TUI).
func DebouncedCommitAndPush(kbRoot string, push bool) {
	debounceMu.Lock()
	defer debounceMu.Unlock()
	debounceRoot = kbRoot
	if push {
		debouncePush = true
	}
	if debounceTimer != nil {
		debounceTimer.Stop()
	}
	debounceTimer = time.AfterFunc(4*time.Second, func() {
		flushDebounce()
	})
}

// flushDebounce runs the debounced commit+push synchronously.
func flushDebounce() {
	msg := GenerateCommitMessage(debounceRoot)
	_ = AutoCommit(debounceRoot, msg)
	if debouncePush {
		cmd := exec.Command("git", "-C", debounceRoot, "push")
		_ = cmd.Run()
		debouncePush = false
	}
}

// FlushDebounce fires any pending debounced commit synchronously.
// Call before process exit to avoid losing changes.
func FlushDebounce() {
	debounceMu.Lock()
	defer debounceMu.Unlock()
	if debounceTimer != nil && debounceTimer.Stop() {
		flushDebounce()
	}
}

// PushWithUpstream runs git push -u origin HEAD for first push on fresh repos.
// This runs synchronously since the user needs to see if initial setup succeeded.
func PushWithUpstream(kbRoot string) error {
	cmd := exec.Command("git", "-C", kbRoot, "push", "-u", "origin", "HEAD")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("push failed: %w", err)
	}
	return nil
}

// AddRemote adds or updates the origin remote URL.
func AddRemote(kbRoot, url string) error {
	cmd := exec.Command("git", "-C", kbRoot, "remote", "add", "origin", url)
	if err := cmd.Run(); err != nil {
		// origin may already exist — try set-url instead
		cmd = exec.Command("git", "-C", kbRoot, "remote", "set-url", "origin", url)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("setting remote: %w", err)
		}
	}
	return nil
}

// HasRemote returns true if an origin remote is configured.
func HasRemote(kbRoot string) bool {
	cmd := exec.Command("git", "-C", kbRoot, "remote", "get-url", "origin")
	return cmd.Run() == nil
}

// CommitAndPush loads config, commits, and pushes if auto_push is enabled.
func CommitAndPush(kbRoot, message string) error {
	cfg, _ := config.Load(kbRoot)
	push := cfg != nil && cfg.AutoPush
	return AutoCommitAndPush(kbRoot, message, push)
}

// AutoCommitAndPush commits immediately and optionally pushes in the background.
func AutoCommitAndPush(kbRoot, message string, push bool) error {
	if err := AutoCommit(kbRoot, message); err != nil {
		return err
	}
	if push {
		go func() {
			cmd := exec.Command("git", "-C", kbRoot, "push")
			_ = cmd.Run()
		}()
	}
	return nil
}

// GenerateCommitMessage creates a descriptive commit message based on pending changes.
func GenerateCommitMessage(kbRoot string) string {
	diff, err := Diff(kbRoot, "")
	if err != nil || diff == "No uncommitted changes." {
		return "auto: update"
	}

	lines := strings.Split(diff, "\n")
	if len(lines) == 1 {
		parts := strings.SplitN(lines[0], " ", 2)
		if len(parts) == 2 {
			return fmt.Sprintf("auto: update %s", parts[1])
		}
	}

	return fmt.Sprintf("auto: update %d files", len(lines))
}

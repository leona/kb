package search

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/leona/kb/internal/config"
	iofs "github.com/leona/kb/internal/fs"
	"github.com/leona/kb/internal/shared"
)

// Result represents a single search match.
type Result struct {
	File    string // relative path within KB
	Line    int    // 1-based line number
	Content string // matching line
	Section string // nearest markdown heading above the match
	Context []ContextLine // surrounding lines (before and after)
}

// ContextLine is a non-matching line surrounding a search result.
type ContextLine struct {
	Line    int
	Content string
}

// TagMatchResult represents a shared doc matched by tag or title rather than content.
type TagMatchResult struct {
	Slug  string
	Title string
	Match string // the tag or title that matched
}

// Options controls search behavior.
type Options struct {
	Project      string // scope to project + its shared refs
	Scope        string // "all", "project", "shared"
	MaxResults   int
	ContextLines int // lines of context above and below each match
}

// Response holds both content matches and tag/title matches.
type Response struct {
	Results    []Result
	TagMatches []TagMatchResult // shared docs matched by tag/title
}

// Search performs full-text search across knowledge base markdown files.
// Also matches query terms against shared doc tags and titles.
func Search(kbRoot, query string, opts Options) (*Response, error) {
	if opts.MaxResults == 0 {
		opts.MaxResults = 50
	}
	if opts.Scope == "" {
		opts.Scope = "all"
	}

	// Split query into terms for AND matching
	queryLower := strings.ToLower(query)
	terms := strings.Fields(queryLower)
	if len(terms) == 0 {
		return nil, nil
	}

	var filesToSearch []string

	switch opts.Scope {
	case "project":
		if opts.Project == "" {
			return nil, nil
		}
		filesToSearch = append(filesToSearch, projectFiles(kbRoot, opts.Project)...)
	case "shared":
		if opts.Project != "" {
			filesToSearch = append(filesToSearch, projectSharedFiles(kbRoot, opts.Project)...)
		} else {
			filesToSearch = append(filesToSearch, allSharedFiles(kbRoot)...)
		}
	default: // "all"
		if opts.Project != "" {
			filesToSearch = append(filesToSearch, projectFiles(kbRoot, opts.Project)...)
			filesToSearch = append(filesToSearch, projectSharedFiles(kbRoot, opts.Project)...)
		} else {
			filesToSearch = append(filesToSearch, allFiles(kbRoot)...)
		}
	}

	resp := &Response{}

	// Search file contents.
	for _, relPath := range filesToSearch {
		if len(resp.Results) >= opts.MaxResults {
			break
		}
		absPath := filepath.Join(kbRoot, relPath)
		matches, err := searchFile(absPath, relPath, terms, opts.MaxResults-len(resp.Results), opts.ContextLines)
		if err != nil {
			continue
		}
		resp.Results = append(resp.Results, matches...)
	}

	// Match query against shared doc tags and titles.
	if opts.Scope != "project" {
		fileSet := make(map[string]bool, len(filesToSearch))
		for _, f := range filesToSearch {
			fileSet[f] = true
		}
		resp.TagMatches = findTagMatches(kbRoot, terms, fileSet)
	}

	return resp, nil
}

// findTagMatches checks all shared docs for tag/title matches against query terms.
// Returns matches for docs not already in the searched file set.
func findTagMatches(kbRoot string, terms []string, alreadySearched map[string]bool) []TagMatchResult {
	docs, err := shared.List(kbRoot)
	if err != nil {
		return nil
	}

	var matches []TagMatchResult
	for _, doc := range docs {
		// Check if any of this doc's files are already in the search set.
		anyInSet := false
		for _, f := range doc.Files {
			if alreadySearched[filepath.ToSlash(filepath.Join("shared", doc.Slug, f))] {
				anyInSet = true
				break
			}
		}

		// Match query terms against title and tags.
		var matchedOn string
		titleLower := strings.ToLower(doc.Title)
		slugLower := strings.ToLower(doc.Slug)

		for _, term := range terms {
			if strings.Contains(titleLower, term) {
				matchedOn = "title: " + doc.Title
				break
			}
			if strings.Contains(slugLower, term) {
				matchedOn = "slug: " + doc.Slug
				break
			}
			for _, tag := range doc.Tags {
				if strings.Contains(strings.ToLower(tag), term) {
					matchedOn = "tag: " + tag
					break
				}
			}
			if matchedOn != "" {
				break
			}
		}

		if matchedOn != "" && !anyInSet {
			matches = append(matches, TagMatchResult{
				Slug:  doc.Slug,
				Title: doc.DisplayTitle(),
				Match: matchedOn,
			})
		}
	}
	return matches
}

// matchesAllTerms returns true if lineLower contains every term.
func matchesAllTerms(lineLower string, terms []string) bool {
	for _, t := range terms {
		if !strings.Contains(lineLower, t) {
			return false
		}
	}
	return true
}

func searchFile(absPath, relPath string, terms []string, maxResults, contextLines int) ([]Result, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Read all lines so we can provide context around matches.
	var allLines []string
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Track the current section heading as we scan.
	currentSection := ""
	var results []Result

	for i, line := range allLines {
		// Track markdown headings for section awareness.
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			currentSection = trimmed
		}

		lineLower := strings.ToLower(line)
		if !matchesAllTerms(lineLower, terms) {
			continue
		}

		result := Result{
			File:    relPath,
			Line:    i + 1, // 1-based
			Content: line,
			Section: currentSection,
		}

		// Gather context lines.
		if contextLines > 0 {
			start := i - contextLines
			if start < 0 {
				start = 0
			}
			end := i + contextLines
			if end >= len(allLines) {
				end = len(allLines) - 1
			}
			for j := start; j <= end; j++ {
				if j == i {
					continue // skip the match line itself
				}
				result.Context = append(result.Context, ContextLine{
					Line:    j + 1,
					Content: allLines[j],
				})
			}
		}

		results = append(results, result)
		if len(results) >= maxResults {
			break
		}
	}

	return results, nil
}

func projectFiles(kbRoot, project string) []string {
	dir := filepath.Join(kbRoot, "projects", project)
	files, _ := iofs.ListMarkdownFiles(dir)
	var result []string
	for _, f := range files {
		result = append(result, filepath.ToSlash(filepath.Join("projects", project, f)))
	}
	return result
}

func projectSharedFiles(kbRoot, project string) []string {
	dir := filepath.Join(kbRoot, "projects", project)
	refs, err := config.LoadRefs(dir)
	if err != nil {
		return nil
	}

	cfg, _ := config.Load(kbRoot)
	var slugs []string
	if cfg != nil {
		slugs = config.EffectiveRefs(cfg, refs.Refs)
	} else {
		slugs = refs.Refs
	}

	var result []string
	for _, slug := range slugs {
		sharedDir := filepath.Join(kbRoot, "shared", slug)
		files, _ := iofs.ListMarkdownFiles(sharedDir)
		for _, f := range files {
			result = append(result, filepath.ToSlash(filepath.Join("shared", slug, f)))
		}
	}
	return result
}

func allSharedFiles(kbRoot string) []string {
	sharedDir := filepath.Join(kbRoot, "shared")
	entries, err := os.ReadDir(sharedDir)
	if err != nil {
		return nil
	}
	var result []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		files, _ := iofs.ListMarkdownFiles(filepath.Join(sharedDir, e.Name()))
		for _, f := range files {
			result = append(result, filepath.ToSlash(filepath.Join("shared", e.Name(), f)))
		}
	}
	return result
}

func allFiles(kbRoot string) []string {
	var result []string

	// Project files
	projectsDir := filepath.Join(kbRoot, "projects")
	if entries, err := os.ReadDir(projectsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			files, _ := iofs.ListMarkdownFiles(filepath.Join(projectsDir, e.Name()))
			for _, f := range files {
				result = append(result, filepath.ToSlash(filepath.Join("projects", e.Name(), f)))
			}
		}
	}

	// Shared files
	result = append(result, allSharedFiles(kbRoot)...)

	return result
}

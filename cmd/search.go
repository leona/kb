package cmd

import (
	"fmt"

	"github.com/leona/kb/internal/config"
	"github.com/leona/kb/internal/search"
	"github.com/leona/kb/internal/shared"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search across the knowledge base",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kbRoot := config.ResolveKBRoot()
		query := args[0]

		projectFlag, _ := cmd.Flags().GetString("project")

		resp, err := search.Search(kbRoot, query, search.Options{
			Project:    projectFlag,
			MaxResults: 50,
		})
		if err != nil {
			return err
		}

		if resp == nil || (len(resp.Results) == 0 && len(resp.TagMatches) == 0) {
			fmt.Println("No results found.")
			return nil
		}

		currentFile := ""
		for _, r := range resp.Results {
			if r.File != currentFile {
				if currentFile != "" {
					fmt.Println()
				}
				fmt.Printf("── %s ──\n", r.File)
				currentFile = r.File
			}
			fmt.Printf("  %4d: %s\n", r.Line, r.Content)
		}

		if len(resp.TagMatches) > 0 {
			fmt.Println("\nShared docs matching by tag/title:")
			for _, tm := range resp.TagMatches {
				fmt.Printf("  - %s (matched %s)\n", tm.Title, tm.Match)
				sharedInfo, err := shared.Get(kbRoot, tm.Slug)
				if err == nil {
					for _, f := range sharedInfo.Files {
						fmt.Printf("      kb_read: shared/%s/%s\n", tm.Slug, f)
					}
				}
			}
		}

		fmt.Printf("\n%d result(s) found.\n", len(resp.Results))
		return nil
	},
}

func init() {
	searchCmd.Flags().String("project", "", "Scope search to a project and its shared refs")
	rootCmd.AddCommand(searchCmd)
}

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	buildTime = ""
	gitCommit = ""
)

func SetVersionInfo(v, bt, gc string) {
	version = v
	buildTime = bt
	gitCommit = gc
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("kb %s", version)
		if gitCommit != "" {
			fmt.Printf(" (%s)", gitCommit)
		}
		if buildTime != "" {
			fmt.Printf(" built %s", buildTime)
		}
		fmt.Println()
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

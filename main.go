package main

import "github.com/leona/kb/cmd"

var Version = "dev"
var BuildTime = ""
var GitCommit = ""

func main() {
	cmd.SetVersionInfo(Version, BuildTime, GitCommit)
	cmd.Execute()
}

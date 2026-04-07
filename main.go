package main

import "github.com/docker-x/composed/cmd"

// Set by goreleaser via -ldflags at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetVersion(version, commit, date)
	cmd.Execute()
}

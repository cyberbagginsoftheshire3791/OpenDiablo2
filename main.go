package main

import (
	"log"
	"os"

	"github.com/OpenDiablo2/OpenDiablo2/d2app"
)

// GitBranch is set by the CI build process to the name of the branch
//
//nolint:gochecknoglobals // This is filled in by the build system
var GitBranch = "local"

// GitCommit is set by the CI build process to the commit hash
//
//nolint:gochecknoglobals // This is filled in by the build system
var GitCommit = "build"

func main() {
	log.SetFlags(log.Lshortfile)

	instance := d2app.Create(GitBranch, GitCommit)

	// A loop error used to end the process in silence (M3.4 finding: a
	// playtest script watched the game vanish with no trace). Say why.
	if err := instance.Run(); err != nil {
		log.Printf("OpenDiablo2 exited with error: %v", err)
		os.Exit(1)
	}
}

package main

import (
	"io"
	"log"
	"os"
	"runtime/debug"

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

	// A friend's build writes a log and, on a panic, a stack to disk. A
	// double-clicked Windows exe has its own console freed by ebiten's
	// hideconsole (audit B6; §0(b) measured the exe as console-subsystem, so
	// Windows allocates a console that hideconsole then frees), so stderr -- and
	// a crash's stack -- would otherwise go nowhere. Tee log into the file
	// BEFORE d2app.Create, so d2term's BindLogger wraps the file-inclusive
	// writer (terminal.go) and the in-game console still works too.
	if logFile := d2app.OpenLogFile(); logFile != nil {
		log.SetOutput(io.MultiWriter(os.Stderr, logFile))
		log.Printf("OpenDiablo2 %s (%s) starting; log at %s", GitBranch, GitCommit, d2app.LogFilePath())
	}

	// A panic on the game loop would leave nothing on a double-clicked build.
	// Write the panic and its stack to the tee'd log, then exit non-zero.
	defer func() {
		if r := recover(); r != nil {
			log.Printf("OpenDiablo2 PANIC: %v\n%s", r, debug.Stack())
			os.Exit(1)
		}
	}()

	instance := d2app.Create(GitBranch, GitCommit)

	// A loop error used to end the process in silence (M3.4 finding: a
	// playtest script watched the game vanish with no trace). Say why.
	if err := instance.Run(); err != nil {
		log.Printf("OpenDiablo2 exited with error: %v", err)
		os.Exit(1)
	}
}

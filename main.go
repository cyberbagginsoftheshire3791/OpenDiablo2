package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime/debug"

	"github.com/OpenDiablo2/OpenDiablo2/d2app"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2logfile"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2report"
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
	if logFile := d2logfile.OpenLogFile(); logFile != nil {
		log.SetOutput(io.MultiWriter(os.Stderr, logFile))
		log.Printf("OpenDiablo2 %s (%s) starting; log at %s", GitBranch, GitCommit, d2logfile.LogFilePath())
	}

	// A panic on THIS goroutine -- start-up, before ebiten's loop takes over --
	// would leave nothing on a double-clicked build. Write it and its stack to
	// the tee'd log and a crash folder (d2report), then exit non-zero.
	//
	// THIS NEVER SAW A PANIC IN THE GAME LOOP (Level 1, 5 Oct 2026): ebiten
	// runs Update and Draw on a goroutine of its own, whose panics do not come
	// back here. d2app's crashGuard covers that goroutine (d2app/crash.go).
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			log.Printf("OpenDiablo2 PANIC: %v\n%s", r, stack)
			log.Printf("CRASH report=%s", d2report.HandlePanic(r, "main", stack))
			os.Exit(1)
		}
	}()

	instance := d2app.Create(GitBranch, GitCommit)

	// A loop error used to end the process in silence (M3.4 finding: a
	// playtest script watched the game vanish with no trace). Say why, and
	// report it as a crash: the run did not end the way a player ends it.
	if err := instance.Run(); err != nil {
		log.Printf("OpenDiablo2 exited with error: %v", err)
		log.Printf("CRASH report=%s", d2report.HandlePanic(fmt.Errorf("the game loop ended with an error: %w", err), "main", nil))
		os.Exit(1)
	}

	// The window closed: a clean exit (the run's marker goes).
	d2report.EndRun()
}

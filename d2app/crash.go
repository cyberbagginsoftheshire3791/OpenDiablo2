package d2app

import (
	"log"
	"os"
	"runtime/debug"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2report"
)

// crashGuard is the crash report for ebiten's game goroutine (Level 1, 5 Oct
// 2026). It is deferred at the top of advance (ebiten's Update) and update
// (its Draw).
//
// MAIN'S recover() NEVER SAW THESE. ebiten v2.9 runs Update and Draw on a
// goroutine of its own (internal/ui run.go, runMultiThread: an errgroup), and
// golang.org/x/sync's errgroup does not carry a panic back to Wait -- so a
// panic in the game loop ended the process with the runtime's trace on stderr,
// which a double-clicked build's freed console sends nowhere, and main.go's
// "PANIC" line was never written. Measured by reading both sources at master
// dc91255f; the playtest feedback_test.go forces one and reads the folder.
//
// Here: the panic and its stack to the log, the crash folder (d2report), the
// run's marker told it crashed (so the next launch names the folder), then
// exit 1. A panic on any OTHER goroutine is still uncaught; the runtime's
// fatal-error output (d2report.StartRun, SetCrashOutput) keeps its stack for
// the next launch to report.
func (a *App) crashGuard(where string) {
	r := recover()
	if r == nil {
		return
	}

	stack := debug.Stack()

	log.Printf("OpenDiablo2 PANIC on the %s goroutine: %v\n%s", where, r, stack)

	dir := d2report.HandlePanic(r, where, stack)

	log.Printf("CRASH report=%s", dir)

	os.Exit(1)
}

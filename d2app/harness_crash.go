//go:build harness

package d2app

import (
	"fmt"
	"sync/atomic"
)

// The crash report's own instrument (Level 1, 5 Oct 2026): a console verb, in
// the harness build only and only in a -harness game, that panics on purpose
// -- so a playtest can force the crash a player hits and read the folder it
// leaves (playtest/feedback_test.go). A player's build has no such verb.
//
//	crashtest update     panic on ebiten's game goroutine, in the frame's update
//	crashtest draw       panic in the next frame's draw
//	crashtest goroutine  panic on a goroutine nothing recovers: the runtime's
//	                     fatal error, which the NEXT launch reports
//
// strigoi_run_console runs the verb on the update goroutine, inside advance's
// crashGuard, so "update" is the game loop's own panic.

// harnessCrashOnDraw is a draw-goroutine crash asked for.
//
//nolint:gochecknoglobals // the harness's process-global state
var harnessCrashOnDraw atomic.Bool

func (a *App) harnessTerminalCommands() {
	if !a.harnessEnabled() {
		return
	}

	err := a.terminal.Bind("crashtest", "harness only: panic on purpose (update, draw or goroutine) to test the crash report",
		[]string{"where"}, a.harnessCrashTest)
	if err != nil {
		a.Errorf("binding crashtest: %v", err)
	}
}

func (a *App) harnessCrashTest(args []string) error {
	switch args[0] {
	case "update":
		harnessForcedCrash("update")
	case "draw":
		harnessCrashOnDraw.Store(true)
	case "goroutine":
		go harnessForcedCrash("goroutine")
	default:
		return fmt.Errorf("crashtest: where is update, draw or goroutine, not %q", args[0])
	}

	return nil
}

// harnessForcedCrash is the frame a forced crash's stack.txt must name.
func harnessForcedCrash(where string) {
	panic("crashtest: a crash forced on the " + where + " goroutine")
}

// harnessDrawCrash runs at the end of a frame's draw (harnessDrainDraw).
func harnessDrawCrash() {
	if harnessCrashOnDraw.Load() {
		harnessForcedCrash("draw")
	}
}

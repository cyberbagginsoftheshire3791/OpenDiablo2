//go:build harness

package d2app

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2screen"
)

// THE CONSOLE'S QUIT IS A CLOSE (the M4.6 B5 review, C3; BUG-102). It went
// straight to os.Exit, and everything since his last save was lost; it runs
// the close hook first now (App.closeTheGame: SAVE AND EXIT's save, then the
// unload), and only then ends the process. Here there is no game in play, so
// the hook finds nothing to save -- what is asserted is that it ran, once,
// before the exit.
func TestTheConsolesQuitClosesTheGameFirst(t *testing.T) {
	a := &App{screen: &d2screen.ScreenManager{}, Logger: d2util.NewLogger()}
	a.Logger.SetLevel(d2util.LogLevelNone)

	closedAtExit, exited := false, -1

	defer func(was func(int)) { exitProcess = was }(exitProcess)

	exitProcess = func(code int) {
		closedAtExit, exited = a.closed, code
	}

	if err := a.quitGame(nil); err != nil {
		t.Fatal(err)
	}

	if exited != 0 {
		t.Fatalf("the console's quit ends the process with 0, got %d", exited)
	}

	if !closedAtExit {
		t.Fatal("the console's quit must run the close hook (closeTheGame) before it ends the process")
	}

	// The hook runs once: a second close finds it done.
	if _, ok := a.closeTheGame("again"); ok {
		t.Fatal("the close hook runs once")
	}
}

//go:build harness

package d2app

import (
	"testing"
	"time"

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

// fakeAsker is a screen in play that asks, or not, before the window closes.
type fakeAsker struct {
	asks  bool
	asked int
}

func (f *fakeAsker) AskBeforeClose(time.Time) bool {
	f.asked++

	return f.asks
}

// A WINDOW'S CLOSE IN COMBAT ASKS FIRST (the combat-status review, A2): the
// screen in play is asked, and when it asks the window stays open; a screen
// that does not ask -- out of combat, or not a game -- closes at once, and
// the close hook runs. THE CONTROL: the same App with no game in play closes
// (onWindowClose answers true and the hook ran).
func TestAWindowCloseInCombatAsksFirst(t *testing.T) {
	asker := &fakeAsker{asks: true}
	if !windowCloseAsks(asker, time.Now()) || asker.asked != 1 {
		t.Fatalf("a game in combat asks before the window closes (asked %d)", asker.asked)
	}

	if windowCloseAsks(&fakeAsker{}, time.Now()) {
		t.Fatal("a game out of combat does not ask")
	}

	if windowCloseAsks(struct{}{}, time.Now()) || windowCloseAsks(nil, time.Now()) {
		t.Fatal("a screen that is not a game, or none, does not ask")
	}

	a := &App{screen: &d2screen.ScreenManager{}, Logger: d2util.NewLogger()}
	a.Logger.SetLevel(d2util.LogLevelNone)

	if !a.onWindowClose() || !a.closed {
		t.Fatal("the control: with no game in play the window closes and the hook runs")
	}
}

package d2app

import (
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2gamescreen"
)

// THE CLOSE HOOK (M4.6 B5, the build plan's rule 3: "Closing the window does
// what SAVE AND EXIT does. In a fight it leaves without saving, and you come
// back to your last save.").
//
// Before B5 the window's close button and Alt-F4 ended ebiten's loop and the
// process with it: no OnUnload ran, so neither his .od2 nor his sidecar was
// written, and everything since he last left through the menu was lost. Now
// the renderer holds the close (ebiten.SetWindowClosingHandled), calls
// onWindowClose on the game goroutine, and ends the loop after it. The
// harness's strigoi_quit{graceful: true} calls the same closeTheGame, which is
// how the playtests reach this path: a script cannot press a window's X.
//
// The game's half is d2gamescreen.Game.CloseGame: SaveWorld -- the world file,
// his .od2, his sidecar, in B3's order, the generation in both -- then the
// screen's own unload, under closeLimit. A refused save (a fight) still
// unloads. A hook that has not finished by the limit is left behind and the
// close goes on: B3's order means a save cut off between its files is refused
// TORN at the next load and set aside (rule 7), never half-resumed.

// closeLimit bounds the close hook. A save is tens of milliseconds and the
// unload less; ten seconds is a hung disk, not a slow one.
const closeLimit = 10 * time.Second

// windowCloseHandled is a renderer that can hand its window's close to the App
// (the ebiten renderer's SetCloseHandler).
type windowCloseHandled interface {
	SetCloseHandler(f func())
}

// onWindowClose is the window's close button and Alt-F4.
func (a *App) onWindowClose() {
	a.closeTheGame("the window")
}

// closeTheGame is the close hook: the game in play, if there is one, saved as
// SAVE AND EXIT saves it and unloaded; and nothing after it -- no frame runs
// and no screen is drawn (App.advance, App.update), because the screen it
// unloaded is still the screen manager's current one. It runs once. ok is
// false when there was no game to close.
func (a *App) closeTheGame(why string) (rep d2gamescreen.CloseReport, ok bool) {
	if a.closed {
		return rep, false
	}

	a.closed = true

	game, inGame := a.screen.Current().(*d2gamescreen.Game)
	if !inGame || game == nil {
		a.Infof("CLOSE (%s): no game in play, nothing to save", why)

		return rep, false
	}

	rep = game.CloseGame(closeLimit)

	if rep.TimedOut {
		a.Errorf("CLOSE (%s): the save and unload did not finish in %v; closing without them", why, closeLimit)
	} else {
		a.Infof("CLOSE (%s): %s", why, rep)
	}

	return rep, true
}

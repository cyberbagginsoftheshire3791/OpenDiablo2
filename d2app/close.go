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
// how the playtests reach this path: a script cannot press a window's X. The
// console's quit calls it too (the B5 review, C3).
//
// The game's half is d2gamescreen.Game.CloseGame: a talk and the journal
// ended and the moment after a fight let settle (the B5 review, A1), then
// SaveWorld -- the world file, his .od2, his sidecar, in B3's order, the
// generation in both -- then the screen's own unload, under closeLimit. Only
// his live fight, his death or a network game leaves without saving what he
// played; the unload runs either way. A hook that has not finished by the
// limit is left behind and the close goes on -- but between files, never
// inside one (the B5 review, B1): no file is ever written in place, only
// through a temporary file and a rename, and at the limit no write begins and
// the one in flight is let finish (d2gamescreen's closeGrace). A save cut off
// between its files is refused TORN at the next load, which then resumes the
// world file's .bak -- the last whole save -- when it is his sidecar's moment.

// closeLimit bounds the close hook. A save is tens of milliseconds and the
// unload less; the settle after a fight runs at most four seconds of it
// (d2gamescreen's closeSettleBudget); ten seconds is a hung disk, not a slow
// one.
const closeLimit = 10 * time.Second

// windowCloseHandled is a renderer that can hand its window's close to the App
// (the ebiten renderer's SetCloseHandler).
type windowCloseHandled interface {
	SetCloseHandler(f func() bool)
}

// closeAsker is the game in play as the window's close asks it (the
// combat-status review, A2): d2gamescreen.Game.AskBeforeClose.
type closeAsker interface {
	AskBeforeClose(now time.Time) bool
}

// onWindowClose is the window's close button and Alt-F4. It answers whether
// the window closes: a close in combat asks once ("You are in combat. Close
// again to leave without saving."), and a second close within a few seconds
// leaves without saving (the combat-status review, A2, decided 1 Oct 2026
// on the coordinator's default; Josh can overturn). Only the window asks: the
// console's quit and the harness's graceful quit are a decision already.
func (a *App) onWindowClose() bool {
	if !a.closed && windowCloseAsks(a.screen.Current(), time.Now()) {
		a.Infof("CLOSE (the window): asked, in combat; the game plays on")

		return false
	}

	a.closeTheGame("the window")

	return true
}

// windowCloseAsks is whether the screen in play asks before the window
// closes: only a game, and only in combat (closeAsker).
func windowCloseAsks(screen interface{}, now time.Time) bool {
	game, ok := screen.(closeAsker)

	return ok && game != nil && game.AskBeforeClose(now)
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

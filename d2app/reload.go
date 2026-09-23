package d2app

import (
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2gamescreen"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client/d2clientconnectiontype"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2server"
)

// reloadWaitFrames bounds the wait for the old game's server to let go of its
// port. Its shutdown is a packet on the server's own goroutine, so it takes a
// handful of real milliseconds, not frames of any fixed count.
const reloadWaitFrames = 600

// ReloadGame leaves the game in play and opens a save in a new one (death
// screen v0: "load last save", 23 Sep 2026).
//
// NOT ToCreateGame DIRECTLY. ToCreateGame starts the new game's local server
// at once, while the old game is still the current screen -- and the old
// server holds the port until the old game's OnUnload disconnects from it.
// Measured: "can not connect to the host", and back to the main menu. So this
// goes to the main menu first, and advanceReload opens the save once the menu
// is the settled screen and the port is free.
func (a *App) ReloadGame(filePath string) {
	a.reloadPath, a.reloadFrames = filePath, 0
	a.ToMainMenu()
}

// advanceReload opens a pending save when it can.
func (a *App) advanceReload() {
	if a.reloadPath == "" {
		return
	}

	a.reloadFrames++

	settled := a.screen.Idle()
	if _, onMenu := a.screen.Current().(*d2gamescreen.MainMenu); !settled || !onMenu || !d2server.PortFree() {
		if a.reloadFrames > reloadWaitFrames {
			a.Errorf("reload: gave up waiting for the last game to close; %s not opened", a.reloadPath)
			a.reloadPath = ""
		}

		return
	}

	path := a.reloadPath
	a.reloadPath = ""

	a.ToCreateGame(path, d2clientconnectiontype.Local, "")
}

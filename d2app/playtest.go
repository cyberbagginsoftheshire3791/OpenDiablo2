package d2app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2hero"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapgen"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2gamescreen"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client/d2clientconnectiontype"
)

// THE WORLD EDITOR'S PLAYTEST (P), as the 28 Sep review left it.
//
// P used to set the authored map, go to the hero screens and forget the editor.
// Three things were wrong with that, and each is fixed here:
//
//   - A2: the editor screen, its document, its unsaved changes and its undo
//     history were dropped, and coming back through the menu reopened the file
//     from disk. Now the App holds the screen (ToWorldEditor keeps it in
//     a.editor) and ToMainMenu hands it back when the playtest's game ends.
//   - B3: every later game in the process was built from the playtest's scratch
//     map, because the authored map is a process-wide setting. Now the setting
//     the process had before P is put back when the playtest ends.
//   - B3: P went to character select with the player's REAL heroes, so a
//     playtest spent a real hero's kit and saved over him. Now P starts a
//     throwaway hero, made fresh for this playtest in a temporary folder of its
//     own, and goes straight into the world -- no hero screen, nothing written
//     where the player's heroes live, and the folder removed afterwards.

// playtestHeroName is the throwaway hero every playtest plays.
const playtestHeroName = "Playtest"

// playtestCleanupDelay is how long a finished playtest's hero folder is left
// before it is removed. The game's local server saves the hero on its own
// goroutine as the game shuts down (d2server.GameServer, SavePlayer), a moment
// after the screen has changed, and removing the folder under it would only
// have it made again. [DIAL]
const playtestCleanupDelay = 1500 * time.Millisecond

// playtestRun is one playtest in progress: the editor it came from, the map
// setting the process had before it, and the throwaway hero's folder.
type playtestRun struct {
	editor    *d2gamescreen.Editor
	launchMap string
	saveDir   string
}

// playtestLeftover is a finished playtest's hero folder, waiting to go.
type playtestLeftover struct {
	dir   string
	ended time.Time
}

// ToPlaytest starts a real game on a map the editor has just written: the
// authored map is set to it (d2mapgen.SetAuthoredMap -- how -map reaches a game
// and how the harness swaps a map), a throwaway hero is made for it, and the
// game starts on that hero at once. The editor it came from is kept, and
// ToMainMenu gives it back when the game ends.
func (a *App) ToPlaytest(mapPath string) {
	back := a.editor
	if back == nil {
		a.Error("a playtest was asked for with no editor open")
		return
	}

	dir, save, err := a.playtestHero()
	if err != nil {
		// Still on the editor: say so there, and start nothing.
		a.Errorf("playtest: %v", err)
		back.PlaytestNotStarted(err.Error())

		return
	}

	launchMap, _, _ := d2mapgen.AuthoredMapReport()

	a.playtest = &playtestRun{editor: back, launchMap: launchMap, saveDir: dir}

	if mapPath != "" {
		d2mapgen.SetAuthoredMap(mapPath)
	}

	a.harnessNoteScreen("playtest") // no-op unless built with -tags harness; ToCreateGame notes "game"
	a.ToCreateGame(save, d2clientconnectiontype.Local, "")
}

// endPlaytest hands the editor back, with the map setting the process had
// before the playtest, and queues the throwaway hero's folder for removal.
func (a *App) endPlaytest(why ...string) {
	run := a.playtest
	a.playtest = nil

	d2mapgen.SetAuthoredMap(run.launchMap)

	a.playtestLeftovers = append(a.playtestLeftovers, playtestLeftover{dir: run.saveDir, ended: time.Now()})

	run.editor.ReturnFromPlaytest(strings.Join(why, " "))
	a.screen.SetNextScreen(run.editor)
	a.harnessNoteScreen("world_editor") // no-op unless built with -tags harness
}

// advancePlaytestCleanup removes finished playtests' hero folders, once no
// game can still be writing into them: no playtest running, no screen change
// under way, and playtestCleanupDelay gone by since it ended.
func (a *App) advancePlaytestCleanup() {
	if len(a.playtestLeftovers) == 0 || a.playtest != nil || !a.screen.Idle() {
		return
	}

	kept := a.playtestLeftovers[:0]

	for _, l := range a.playtestLeftovers {
		if time.Since(l.ended) < playtestCleanupDelay {
			kept = append(kept, l)
			continue
		}

		if err := os.RemoveAll(l.dir); err != nil {
			a.Warningf("playtest: could not remove the throwaway hero's folder %s: %v", l.dir, err)
		}
	}

	a.playtestLeftovers = kept
}

// playtestHero makes the throwaway hero a playtest plays: named Playtest, of
// the class a new game offers (d2gamescreen.PinnedHeroClass), saved in a
// temporary folder of his own -- never the player's Saves folder, so the hero
// screen never lists him and nothing he does is written over anyone -- and
// given the default loadout, so the playtest opens in the world rather than on
// the loadout choice.
func (a *App) playtestHero() (dir, save string, err error) {
	dir, err = os.MkdirTemp("", "strigoi-playtest-")
	if err != nil {
		return "", "", fmt.Errorf("a folder for the playtest hero: %w", err)
	}

	fail := func(err error) (string, string, error) {
		_ = os.RemoveAll(dir)
		return "", "", err
	}

	factory, err := d2hero.NewHeroStateFactory(a.asset)
	if err != nil {
		return fail(fmt.Errorf("the hero factory: %w", err))
	}

	class := d2gamescreen.PinnedHeroClass()

	state, err := factory.CreateHeroState(playtestHeroName, class, factory.NewHeroStats(class))
	if err != nil {
		return fail(fmt.Errorf("the playtest hero: %w", err))
	}

	state.FilePath = filepath.Join(dir, "playtest.od2")

	if err := factory.Save(state); err != nil {
		return fail(fmt.Errorf("saving the playtest hero: %w", err))
	}

	data, err := a.asset.LoadFile("/data/strigoi/items.json")
	if err != nil {
		return fail(fmt.Errorf("the item table: %w", err))
	}

	cat, err := d2items.Load(data)
	if err != nil {
		return fail(fmt.Errorf("the item table: %w", err))
	}

	kit, err := cat.NewKit(cat.DefaultLoadout())
	if err != nil {
		return fail(fmt.Errorf("the playtest hero's kit: %w", err))
	}

	if err := d2items.SaveHero(d2items.SidecarPath(state.FilePath), kit, d2items.Extras{}); err != nil {
		return fail(fmt.Errorf("the playtest hero's kit: %w", err))
	}

	return dir, state.FilePath, nil
}

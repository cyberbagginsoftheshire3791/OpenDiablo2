package d2app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
//
// The second 28 Sep review found two gaps, both closed here:
//
//   - The throwaway hero's folder outlived a game that exited during a
//     playtest or within playtestCleanupDelay of one. Now each folder's name
//     carries the process that made it, stale folders -- their process gone --
//     are cleared at start-up (App.clearStalePlaytests), and a process clears
//     its own as it exits (clearOwnPlaytests).
//   - A playtest ended only through ToMainMenu. If the death screen's "load
//     last save" gave up waiting for the old game, the game sat on the REAL
//     main menu with the playtest still running and the scratch map still set.
//     Now that give-up ends the playtest (advanceReload), and so does any other
//     way off the playtest's game (advancePlaytestEnd).

// playtestHeroName is the throwaway hero every playtest plays.
const playtestHeroName = "Playtest"

// playtestCleanupDelay is how long a finished playtest's hero folder is left
// before it is removed. The game's local server saves the hero on its own
// goroutine as the game shuts down (d2server.GameServer, SavePlayer), a moment
// after the screen has changed, and removing the folder under it would only
// have it made again. [DIAL]
const playtestCleanupDelay = 1500 * time.Millisecond

// playtestDirPrefix begins the name of every throwaway hero's folder. The
// process that made it follows, then os.MkdirTemp's random part:
// strigoi-playtest-<pid>-<random>, in the temporary folder. A name with no pid
// in it (strigoi-playtest-<random>) was made by a build from before the second
// 28 Sep review.
const playtestDirPrefix = "strigoi-playtest-"

// playtestLegacyAge is how old a folder whose name carries no process must be
// before start-up clears it. Such a folder can only have been made by an older
// build, which may -- in theory -- still be running a playtest from it, so it is
// given a margin rather than taken at once. [DIAL]
const playtestLegacyAge = 10 * time.Minute

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
// Every way a playtest ends comes here: ToMainMenu, a reload that gave up
// (advanceReload) and any other way off its game (advancePlaytestEnd).
func (a *App) endPlaytest(why ...string) {
	run := a.playtest
	a.playtest = nil

	d2mapgen.SetAuthoredMap(run.launchMap)

	a.playtestLeftovers = append(a.playtestLeftovers, playtestLeftover{dir: run.saveDir, ended: time.Now()})

	run.editor.ReturnFromPlaytest(strings.Join(why, " "))
	a.screen.SetNextScreen(run.editor)
	a.harnessNoteScreen("world_editor") // no-op unless built with -tags harness
}

// advancePlaytestEnd ends a playtest whose game has gone by a way that did not
// end it: a playtest is running, no reload of its hero is pending, no screen
// change is under way -- and the screen is not a game. The ways the second 28
// Sep review found are the death screen's "load last save" giving up (which
// advanceReload now ends itself) and a click on the real main menu while that
// reload waited on it; this is the net under both and any the review did not
// find, so a playtest cannot outlive its game with the scratch map still set.
// Called every frame from advanceOnce, after advanceReload.
func (a *App) advancePlaytestEnd() {
	if a.playtest == nil || a.reloadPath != "" || !a.screen.Idle() {
		return
	}

	if _, inGame := a.screen.Current().(*d2gamescreen.Game); inGame {
		return
	}

	a.endPlaytest("the playtest's game has gone")
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
	// The process's id is in the name, so a later start-up can tell a folder
	// whose game is gone from one a running game is still playing in.
	dir, err = os.MkdirTemp("", playtestDirPrefix+strconv.Itoa(os.Getpid())+"-")
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

// playtestFolderOwner is the process id a throwaway hero folder's name carries
// (strigoi-playtest-<pid>-<random>), or 0 for a name with none: an older
// build's strigoi-playtest-<random>, or not a playtest folder at all.
func playtestFolderOwner(name string) int {
	rest := strings.TrimPrefix(name, playtestDirPrefix)
	if rest == name {
		return 0
	}

	parts := strings.SplitN(rest, "-", 2)
	if len(parts) != 2 || parts[1] == "" {
		return 0
	}

	pid, err := strconv.Atoi(parts[0])
	if err != nil || pid <= 0 {
		return 0
	}

	return pid
}

// playtestFolderStale says whether a throwaway hero folder found at start-up is
// in use -- false -- or may go. It is in use while the process its name carries
// is running: several games share the laptop's temporary folder (the playtest
// suite runs four at once), and one game's start-up must never take another's
// hero from under it. self is the process asking, which has made no folder yet,
// so one carrying its id is an earlier process's whose id was reused. A folder
// whose name carries no process is an older build's and goes once it is
// playtestLegacyAge old.
func playtestFolderStale(name string, modified, now time.Time, self int, alive func(pid int) bool) bool {
	switch owner := playtestFolderOwner(name); {
	case owner == 0:
		return now.Sub(modified) > playtestLegacyAge
	case owner == self:
		return true
	default:
		return !alive(owner)
	}
}

// sweepPlaytestFolders removes every throwaway hero folder in dir that goes
// says may go, and answers the ones it removed and the first error it met.
func sweepPlaytestFolders(dir string, goes func(name string, modified time.Time) bool) (removed []string, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, ent := range entries {
		if !ent.IsDir() || !strings.HasPrefix(ent.Name(), playtestDirPrefix) {
			continue
		}

		info, ierr := ent.Info()
		if ierr != nil {
			continue // gone already
		}

		if !goes(ent.Name(), info.ModTime()) {
			continue
		}

		p := filepath.Join(dir, ent.Name())
		if rerr := os.RemoveAll(p); rerr != nil {
			if err == nil {
				err = rerr
			}

			continue
		}

		removed = append(removed, p)
	}

	return removed, err
}

// clearStalePlaytests clears, at start-up, the throwaway hero folders earlier
// games left in the temporary folder (the second 28 Sep review: a game that
// exited during a playtest, or within playtestCleanupDelay of one, left its
// hero's folder for good). Folders a running game is playing in are kept
// (playtestFolderStale). App.Run's first call.
func (a *App) clearStalePlaytests() {
	self, now := os.Getpid(), time.Now()

	removed, err := sweepPlaytestFolders(os.TempDir(), func(name string, modified time.Time) bool {
		return playtestFolderStale(name, modified, now, self, processAlive)
	})

	for _, p := range removed {
		a.Infof("playtest: cleared a throwaway hero's folder an earlier game left: %s", p)
	}

	if err != nil {
		a.Warningf("playtest: clearing earlier games' throwaway hero folders: %v", err)
	}
}

// clearOwnPlaytests removes this process's own throwaway hero folders -- a
// playtest still running, or one that ended less than playtestCleanupDelay ago
// -- as the process exits. It touches only the file system, so it is safe from
// any goroutine (the harness's quit exits from its own). What it misses -- a
// killed process, or a save the game's server writes after it -- the next
// start-up clears. Called where the App ends the process: the window closing
// (App.Run), the console's quit and the harness's strigoi_quit.
func clearOwnPlaytests() {
	self := os.Getpid()

	_, _ = sweepPlaytestFolders(os.TempDir(), func(name string, _ time.Time) bool {
		return playtestFolderOwner(name) == self
	})
}

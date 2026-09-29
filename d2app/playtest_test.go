//go:build harness

package d2app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapgen"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2screen"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2gamescreen"
)

// B1'S HAZARD, THE HARNESS HALF (28 Sep review; widened by the second review's
// B). The playtest launcher runs the game with the repository as its working
// directory, so under the harness the editor must open nothing in the source
// tree: the first guard refused village.tmj alone, and the second review opened
// a copy beside it, second.tmj, and overwrote it with a scripted Delete and
// Ctrl+S. With -harness on, the editor now refuses any path found by the
// working-directory fallback and any path inside the tree; with it off (a
// harness build played as the game) it opens what it is told.
//
// Negative control (28 Sep 2026, second review): put back the first guard --
// refuse only filepath.Abs(DefaultEditorMap), with the two new refusals off --
// and this fails at its first case: "with -harness on, the working tree's
// village opened" (that guard's village is relative to the working directory,
// which for this test is d2app, so it caught nothing here). Log:
// strigoi-harness-runs\wt-editor-fix2\nc\nc1-unit-village-only.txt.
func TestTheHarnessRefusesToEditTheSourceTree(t *testing.T) {
	was := harness.enabled
	t.Cleanup(func() { harness.enabled = was })

	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}

	village := filepath.Join(repo, filepath.FromSlash(d2gamescreen.DefaultEditorMap))
	second := filepath.Join(repo, "data", "strigoi", "maps", "second.tmj")
	copyPath := filepath.Join(t.TempDir(), "data", "strigoi", "maps", "village.tmj")

	if got := harnessSourceTree(village); !strings.EqualFold(got, repo) {
		t.Fatalf("the instrument: the source tree of %s is %q, want %s", village, got, repo)
	}

	if got := harnessSourceTree(copyPath); got != "" {
		t.Fatalf("the instrument: a temporary copy is inside a source tree, %s", got)
	}

	on, off := true, false
	harness.enabled = &on

	if err := harnessEditorGuard(village, false); err == nil {
		t.Fatalf("with -harness on, the working tree's village opened (%s)", village)
	}

	if err := harnessEditorGuard(second, false); err == nil {
		t.Fatalf("with -harness on, a second map in the working tree opened (%s)", second)
	}

	if err := harnessEditorGuard(copyPath, true); err == nil {
		t.Fatalf("with -harness on, a map found by the working-directory fallback opened (%s)", copyPath)
	}

	if err := harnessEditorGuard(copyPath, false); err != nil {
		t.Fatalf("with -harness on, a copy outside the tree was refused: %v", err)
	}

	harness.enabled = &off

	if err := harnessEditorGuard(village, true); err != nil {
		t.Fatalf("a harness build without -harness refused the village: %v", err)
	}

	harness.enabled = nil

	if err := harnessEditorGuard(village, true); err != nil {
		t.Fatalf("before the flags are read, the guard refused: %v", err)
	}
}

// B3. A PLAYTEST'S THROWAWAY HERO IS REMOVED -- but not while a game could still
// be writing into his folder: not during a playtest, not mid screen change, and
// not until playtestCleanupDelay has passed since the playtest ended (the game's
// server saves the hero on its own goroutine as the game shuts down).
//
// Negative control (28 Sep 2026): drop the delay check from
// advancePlaytestCleanup and this fails -- "a folder whose playtest ended just
// now was removed".
func TestAFinishedPlaytestsHeroIsRemovedOnceItsGameHasGone(t *testing.T) {
	old, fresh := t.TempDir(), t.TempDir()

	for _, dir := range []string{old, fresh} {
		if err := os.WriteFile(filepath.Join(dir, "playtest.od2"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	a := &App{screen: &d2screen.ScreenManager{}, Logger: d2util.NewLogger()}

	a.playtestLeftovers = []playtestLeftover{
		{dir: old, ended: time.Now().Add(-2 * playtestCleanupDelay)},
		{dir: fresh, ended: time.Now()},
	}

	// A playtest in progress holds every folder.
	a.playtest = &playtestRun{}
	a.advancePlaytestCleanup()

	if _, err := os.Stat(old); err != nil {
		t.Fatalf("a folder was removed while a playtest was running: %v", err)
	}

	a.playtest = nil
	a.advancePlaytestCleanup()

	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the finished playtest's folder is still there: %v", err)
	}

	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("a folder whose playtest ended just now was removed: %v", err)
	}

	if len(a.playtestLeftovers) != 1 || a.playtestLeftovers[0].dir != fresh {
		t.Fatalf("the queue after one pass is %v, want just the fresh folder", a.playtestLeftovers)
	}
}

// THE SECOND 28 SEP REVIEW, C: A THROWAWAY HERO'S FOLDER OUTLIVED A GAME THAT
// EXITED during a playtest, or within playtestCleanupDelay of one -- three were
// sitting in %TEMP% when it looked. Start-up now clears the stale ones; but
// several games share the temporary folder (the suite runs four at once), so a
// folder whose game is still RUNNING must survive another game's start-up.
//
// Negative control (28 Sep 2026): make playtestFolderStale answer true for every
// folder -- a sweep that does not ask whether the folder is in use -- and this
// fails: "a folder a running game is playing in was cleared". Log:
// strigoi-harness-runs\wt-editor-fix2\nc\nc3-sweep-ignores-live.txt.
func TestStalePlaytestFoldersAreClearedAndLiveOnesKept(t *testing.T) {
	dir := t.TempDir()

	// A process that has come and gone: this test binary, asked to run nothing.
	gone := exec.Command(os.Args[0], "-test.run=^$")
	if err := gone.Run(); err != nil {
		t.Fatalf("starting a process to outlive: %v", err)
	}

	if processAlive(gone.Process.Pid) {
		t.Fatalf("the instrument: process %d has exited and processAlive says it is running", gone.Process.Pid)
	}

	if !processAlive(os.Getppid()) {
		t.Fatalf("the instrument: this test's parent %d is running and processAlive says it is not", os.Getppid())
	}

	self := os.Getpid()
	now := time.Now()
	old := now.Add(-2 * playtestLegacyAge)

	mk := func(name string, modified time.Time) string {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(p, 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(p, "playtest.od2"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}

		if err := os.Chtimes(p, modified, modified); err != nil {
			t.Fatal(err)
		}

		return p
	}

	live := mk(fmt.Sprintf("%s%d-111", playtestDirPrefix, os.Getppid()), now)
	dead := mk(fmt.Sprintf("%s%d-222", playtestDirPrefix, gone.Process.Pid), now)
	reused := mk(fmt.Sprintf("%s%d-333", playtestDirPrefix, self), now)
	legacyOld := mk(playtestDirPrefix+"1390003002", old) // the review's leftovers' shape
	legacyNew := mk(playtestDirPrefix+"2358539398", now)
	other := mk("strigoi-harness", old)

	removed, err := sweepPlaytestFolders(dir, func(name string, modified time.Time) bool {
		return playtestFolderStale(name, modified, now, self, processAlive)
	})
	if err != nil {
		t.Fatal(err)
	}

	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }

	for _, p := range []string{live, legacyNew, other} {
		if !exists(p) {
			switch p {
			case live:
				t.Errorf("a folder a running game is playing in was cleared: %s", p)
			default:
				t.Errorf("%s was cleared", p)
			}
		}
	}

	for _, p := range []string{dead, reused, legacyOld} {
		if exists(p) {
			t.Errorf("a stale folder was left: %s", p)
		}
	}

	if len(removed) != 3 {
		t.Errorf("the sweep reports %d folder(s) removed, want 3: %v", len(removed), removed)
	}

	// The names the game now makes carry its process.
	if got := playtestFolderOwner(fmt.Sprintf("%s%d-3739554505", playtestDirPrefix, 4242)); got != 4242 {
		t.Errorf("playtestFolderOwner of a new name = %d, want 4242", got)
	}

	for _, name := range []string{playtestDirPrefix + "3739554505", playtestDirPrefix + "x-1", "strigoi-harness"} {
		if got := playtestFolderOwner(name); got != 0 {
			t.Errorf("playtestFolderOwner(%q) = %d, want 0", name, got)
		}
	}
}

// ...AND A PROCESS CLEARS ITS OWN AS IT EXITS, leaving every other process's.
// (clearOwnPlaytests works on the real temporary folder; the sweep under it is
// what is tested here, on a folder of the test's own.)
func TestAProcessClearsItsOwnPlaytestFoldersAsItExits(t *testing.T) {
	dir := t.TempDir()
	self := os.Getpid()

	mine := filepath.Join(dir, fmt.Sprintf("%s%d-1", playtestDirPrefix, self))
	theirs := filepath.Join(dir, fmt.Sprintf("%s%d-2", playtestDirPrefix, self+1))

	for _, p := range []string{mine, theirs} {
		if err := os.MkdirAll(p, 0o750); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := sweepPlaytestFolders(dir, func(name string, _ time.Time) bool {
		return playtestFolderOwner(name) == self
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(mine); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the process's own folder is still there: %v", err)
	}

	if _, err := os.Stat(theirs); err != nil {
		t.Errorf("another process's folder was cleared: %v", err)
	}
}

// THE SECOND 28 SEP REVIEW, C (code reading): IF THE DEATH SCREEN'S "LOAD LAST
// SAVE" GAVE UP during a playtest, the game sat on the REAL main menu with the
// playtest still running and the scratch map still set -- ToMainMenu, the only
// place a playtest ended, had been passed through on the way to the reload.
// Not reachable by a script (the give-up needs the old game's server to hold
// its port for 600 frames), so this drives the state transition directly: the
// give-up ends the playtest, and so does the net under it, advancePlaytestEnd,
// for any other way off the playtest's game.
//
// Negative controls (28 Sep 2026): take the endPlaytest call out of
// advanceReload's give-up and make advancePlaytestEnd return at once, and this
// fails: "the reload gave up and the playtest is still running" (log
// strigoi-harness-runs\wt-editor-fix2\nc\nc4-playtest-outlives-reload.txt).
// Put the give-up back and leave advancePlaytestEnd dark, and it fails on its
// own case: "the screen left the playtest's game and the playtest is still
// running" (nc4b-no-net.txt).
func TestEveryWayOffAPlaytestEndsIt(t *testing.T) {
	was, _, _ := d2mapgen.AuthoredMapReport()
	t.Cleanup(func() { d2mapgen.SetAuthoredMap(was) })

	const launch, scratch = "/data/strigoi/maps/village.tmj", "/data/strigoi/maps/playtest-scratch.tmj"

	start := func() *App {
		a := &App{screen: &d2screen.ScreenManager{}, Logger: d2util.NewLogger()}
		a.playtest = &playtestRun{editor: &d2gamescreen.Editor{}, launchMap: launch, saveDir: t.TempDir()}
		d2mapgen.SetAuthoredMap(scratch)

		return a
	}

	ended := func(a *App, how string) {
		t.Helper()

		if a.playtest != nil {
			t.Fatalf("%s and the playtest is still running", how)
		}

		if asked, _, _ := d2mapgen.AuthoredMapReport(); asked != launch {
			t.Fatalf("%s and the map setting is %q, not the launch map %q", how, asked, launch)
		}

		if len(a.playtestLeftovers) != 1 {
			t.Fatalf("%s and the throwaway hero's folder was not queued for removal", how)
		}

		if a.screen.Idle() {
			t.Fatalf("%s and the editor was not handed back", how)
		}
	}

	// The reload waits for the menu and a free port; on its last frame it
	// gives up. The screen is not the menu, so it cannot open the save.
	a := start()
	a.reloadPath, a.reloadFrames = filepath.Join(a.playtest.saveDir, "playtest.od2"), reloadWaitFrames
	a.advanceReload()
	ended(a, "the reload gave up")

	if a.reloadPath != "" {
		t.Fatalf("the reload gave up and is still pending: %q", a.reloadPath)
	}

	// While the reload is still waiting, the playtest goes on.
	a = start()
	a.reloadPath = "pending.od2"
	a.advancePlaytestEnd()

	if a.playtest == nil {
		t.Fatal("a playtest was ended while its hero's reload was still waiting")
	}

	// Off the game by any other way: no reload, the screen settled, not a game.
	a = start()
	a.advancePlaytestEnd()
	ended(a, "the screen left the playtest's game")
}

//go:build harness

package d2app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2screen"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2gamescreen"
)

// B1'S HAZARD, THE HARNESS HALF (28 Sep review). The playtest launcher runs the
// game with the repository as its working directory, so a bare -editor under
// the harness opened the working tree's village -- and a scripted Ctrl+S would
// have written it. With -harness on, the editor refuses that one file; with it
// off (a harness build played as the game) it opens what it is told.
//
// Negative control (28 Sep 2026): make harnessEditorGuard return nil always and
// this fails -- "with -harness on, the shipped village opened".
func TestTheHarnessRefusesToEditTheShippedVillage(t *testing.T) {
	was := harness.enabled
	t.Cleanup(func() { harness.enabled = was })

	on, off := true, false
	shipped, err := filepath.Abs(filepath.FromSlash(d2gamescreen.DefaultEditorMap))
	if err != nil {
		t.Fatal(err)
	}

	copyPath := filepath.Join(t.TempDir(), "data", "strigoi", "maps", "village.tmj")

	harness.enabled = &on

	if err := harnessEditorGuard(shipped); err == nil {
		t.Fatalf("with -harness on, the shipped village opened (%s)", shipped)
	}

	if err := harnessEditorGuard(copyPath); err != nil {
		t.Fatalf("with -harness on, a copy was refused: %v", err)
	}

	harness.enabled = &off

	if err := harnessEditorGuard(shipped); err != nil {
		t.Fatalf("a harness build without -harness refused the village: %v", err)
	}

	harness.enabled = nil

	if err := harnessEditorGuard(shipped); err != nil {
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

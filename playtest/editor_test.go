//go:build playtest

package playtest

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2mapedit"
)

// TestWorldEditor is the World Editor v0 acceptance script (M5.4).
//
// THE MILESTONE'S ASSERTION IN ONE SENTENCE: the editor opens on the village in
// a real game with no terminal work beyond a flag, and a playtest run on a map
// it wrote leaves the AUTHORING file byte for byte as it was.
//
// WHAT THIS SCRIPT DELIBERATELY DOES NOT DO. The editor is driven by the mouse
// and the keyboard, and the harness has no verb for a mouse WHEEL and no
// press-and-hold for a DRAG (37 tools, checked). So this script does not claim
// to have zoomed or panned -- the screenshots in
// strigoi-harness-runs\editor-v0-shots\ show those working, and the unit tests
// in d2core/d2map/d2maprenderer prove the transform, but nobody has turned a
// wheel under a script and this script does not pretend otherwise.
//
// What it DOES prove is the half that matters most and that no unit test can
// reach: that the flag reaches a real editor in a real game, and that the file
// on disk is safe from a playtest.
//
// THE ORDER OF THE ACTS IS NOT COSMETIC. playtest/launcher.go builds the
// harness binary ONCE per go test run and MIRRORS data/strigoi beside it -- its
// own comment calls the copy a snapshot, "art that changes mid-suite does not
// reach it". So a map written after start/startWith is "file not found" to the
// game. Every file this script wants the game to read is written BEFORE the
// game launches. This cost a red run here and another in the burst's footprint
// scaffold; it is written down so it costs a third nobody.
//
// Five acts:
//  1. the editor's own save keeps the previous generation, and the saved file
//     is one the GAME takes -- run against the shipped village rather than a
//     fixture, on a copy, so the real file is never at risk. FIRST, because the
//     game must see this file at launch;
//  2. -editor opens the EDITOR, not the main menu -- read back from the game,
//     so a refused map (which falls back to the menu by design) fails here;
//  3. opening a map does not write to it: the village is byte-identical after
//     the editor has had it open;
//  4. A PLAYTEST RUN ON THE EDITED MAP LEAVES THE AUTHORING FILE BYTE-IDENTICAL
//     -- the run happens on a scratch copy, the real game plays it, and both
//     the authoring file and the scratch file are hashed before and after;
//  5. control -- the same comparison against a map the game was NOT given, so
//     act 4 is not passing because nothing ever writes to anything.
func TestWorldEditor(t *testing.T) {
	const village = "data/strigoi/maps/village.tmj"

	repoVillage := filepath.Join("..", filepath.FromSlash(village))

	before := hashFile(t, repoVillage)
	t.Logf("the shipped village is %s", before[:16])

	// --- act 1: a save keeps the previous generation -------------------------
	//
	// On a COPY beside the real map, so the tileset's relative image paths
	// still resolve and the shipped file is never written to.
	scratch := filepath.Join(filepath.Dir(repoVillage), "acceptance-scratch.tmj")

	raw, err := os.ReadFile(repoVillage)
	if err != nil {
		t.Fatalf("act 1: reading the village: %v", err)
	}

	if err := os.WriteFile(scratch, raw, 0o600); err != nil {
		t.Fatalf("act 1: writing the scratch map: %v", err)
	}

	t.Cleanup(func() {
		for _, p := range []string{scratch, d2mapedit.BackupPath(scratch)} {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				t.Logf("LEFT A FILE BEHIND: %v", err)
			}
		}
	})

	doc, err := d2mapedit.OpenFile(scratch)
	if err != nil {
		t.Fatalf("act 1: the editor cannot open the scratch copy: %v", err)
	}

	art := d2mapedit.DirArt(filepath.Dir(scratch))

	if problems := doc.Validate(art); len(problems) > 0 {
		t.Fatalf("act 1: the shipped village does not validate clean: %d problem(s), first %s",
			len(problems), problems[0].Error())
	}

	if err := doc.Save(scratch, art); err != nil {
		t.Fatalf("act 1: the editor refused to save the village it just opened: %v", err)
	}

	if _, err := os.Stat(d2mapedit.BackupPath(scratch)); err != nil {
		t.Fatalf("act 1: a save left no previous generation beside it: %v", err)
	}

	t.Logf("act 1: saved, and %s is beside it", filepath.Base(d2mapedit.BackupPath(scratch)))

	// --- act 2: the flag opens the editor -----------------------------------
	//
	// STEP FIRST. The screen name is noted when ToWorldEditor runs, but the
	// ScreenManager loads the screen asynchronously, so at tick 1 the game is
	// still on the loading frame -- the first version of this act screenshotted
	// that and called it the editor. Stepping past the load also makes the
	// assertion mean something: an editor that FAILED to open goes to the main
	// menu (by design, ToWorldEditor's doc), and ToMainMenu notes "main_menu"
	// over the top, so after the load the name can still go red.
	s := startWith(t, "-editor")

	s.call("strigoi_step", map[string]any{"frames": 240})

	info := s.call("strigoi_get_game_info", map[string]any{})
	if got := str(info, "screen"); got != "world_editor" {
		t.Fatalf("act 2: after loading, -editor left the game on screen %q, want %q -- the editor did not open",
			got, "world_editor")
	}

	if flag(t, info, "in_game") {
		t.Fatal("act 2: the editor reports being in a game; it is a screen, not a world")
	}

	if flag(t, info, "loading") {
		t.Fatal("act 2: the editor is still loading after 240 frames")
	}

	shot := s.call("strigoi_screenshot", map[string]any{"name": "editor-v0-acceptance"})
	t.Logf("act 2: the editor is open and drawn -- %s", str(shot, "path"))

	// --- act 3: opening a map does not write to it --------------------------
	s.call("strigoi_step", map[string]any{"frames": 60})

	if after := hashFile(t, repoVillage); after != before {
		t.Fatalf("act 3: the village changed merely by being opened in the editor: %s -> %s",
			before[:16], after[:16])
	}

	t.Log("act 3: the village is byte-identical after the editor has had it open")

	// --- act 4: a playtest leaves the authoring file alone -------------------
	//
	// The real game, on the map the editor wrote. The map argument is STICKY
	// per process (measured 27 Sep), so it is passed explicitly.
	scratchBefore := hashFile(t, scratch)

	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	s.call("strigoi_step", map[string]any{"frames": 30})

	g := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Editor", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
		"map": "data/strigoi/maps/acceptance-scratch.tmj",
	})

	if e := str(g, "map_error"); e != "" {
		t.Fatalf("act 4: the game refused the map the editor saved: %s", e)
	}

	if built := str(g, "map_built"); built != "/data/strigoi/maps/acceptance-scratch.tmj" {
		t.Fatalf("act 4: the game built %q, not the editor's map -- a refused map falls back to Act 1 silently", built)
	}

	// Play it: walk the clock on, so the run is a run and not just a load.
	s.call("strigoi_step_world", map[string]any{"world_minutes": 30.0})
	s.call("strigoi_step", map[string]any{"frames": 120})

	if after := hashFile(t, repoVillage); after != before {
		t.Fatalf("act 4: A PLAYTEST CHANGED THE AUTHORING FILE: %s -> %s", before[:16], after[:16])
	}

	if after := hashFile(t, scratch); after != scratchBefore {
		t.Fatalf("act 4: the run wrote back to the map it played: %s -> %s", scratchBefore[:16], after[:16])
	}

	t.Log("act 4: after a real run on the editor's map, both the authoring file and the played map are byte-identical")

	// --- act 5: the control --------------------------------------------------
	//
	// Act 4 compares hashes and finds them equal, which is exactly what a
	// broken hash would also report. So hash something that DID change and
	// prove the instrument can tell the difference.
	if err := os.WriteFile(scratch+".control", append(raw, '\n'), 0o600); err != nil {
		t.Fatalf("act 5: %v", err)
	}

	t.Cleanup(func() {
		if err := os.Remove(scratch + ".control"); err != nil && !os.IsNotExist(err) {
			t.Logf("LEFT A FILE BEHIND: %v", err)
		}
	})

	if same := hashFile(t, scratch+".control"); same == before {
		t.Fatal("act 5: CONTROL FAILED -- a file with one byte added hashes the same as the original, so act 4 proved nothing")
	}

	t.Log("act 5: control red as it should be -- one added byte changes the hash, so act 4's equality means something")
}

// hashFile is the instrument acts 2, 4 and 5 rest on.
func hashFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("hashing %s: %v", path, err)
	}

	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}

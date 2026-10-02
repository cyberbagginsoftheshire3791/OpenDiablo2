//go:build playtest

package playtest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2mapedit"
)

// TestWorldEditor is the World Editor v0 acceptance script (M5.4), REWRITTEN
// after the 28 Sep 2026 review (B1): the first version could pass on nothing. It
// saved an UNEDITED document through the API instead of pressing Ctrl+S, never
// pressed P (so both of its hash comparisons were equal by construction -- the
// launcher mirrors data/strigoi beside a temporary exe and the game read the
// mirror), and never looked at the screenshot it took. This one drives the
// REAL screen with the mouse and the keyboard, and reads the pixels.
//
// WHERE THE FILES LIVE, AND WHY. The launcher builds the harness game once per
// run into a temporary folder and mirrors data/strigoi beside it; the game
// reads its loose files from its OWN folder, so a file the game must read --
// the map a playtest runs -- has to be there. So the authoring copy is written
// INTO the mirror and handed to -editor by its ABSOLUTE path: the editor reads
// and writes that file, its playtest scratch lands beside it, and the game reads
// both from the same place. The repository's own village is never opened: the
// harness now REFUSES to open it in the editor (act 9 shows the refusal), because
// the game runs with the repository as its working directory and a scripted
// Ctrl+S under a bare -editor would have written it -- and since the second
// review it refuses anything else in the source tree too
// (TestWorldEditorRefusesTheWorkingTree, below).
//
// Two launches, nine acts:
//
//	the refused map (its own launch):
//	 0. a copy whose grass PNG is cut short after its header -- the validator
//	    passes it, the engine refuses it: the editor opens and SAYS SO in the
//	    map area (C), and Ctrl+S and P both refuse it (B2);
//	the authoring copy:
//	 1. -editor opens THAT file (disk_path), clean;
//	 2. the first screen reads (A1): at the fit zoom, with the grid turned off
//	    by G, the map's ink stays inside its own diamond (plus the height of its
//	    tallest art above it), NOT ONE pixel inside it is a hole (the second
//	    review), and every person and the start carry a marker (B5); no name is
//	    drawn until the cursor is on a person, and then his, clear of every mark;
//	 3. scripted zooms to 0.15, 0.5 and 0.25 through the provider's zoom field,
//	    which calls the wheel's own zoomAbout: no holes at any of them; at 0.25
//	    every person is named, whole, inside the map's view, clear of every mark
//	    and every other name (the second review);
//	 4. a house placed by CLICKS -- a palette row, then the greyed Terrain tab
//	    (B4: the house is judged by its own tab), then a map tile the ghost
//	    itself says yes to -- and the house's pixels appear on its footprint;
//	 5. Ctrl+S on the keyboard: the file parses with the ENGINE's parser, holds
//	    the new footprint, names it "peasant-house" (C), keeps a .bak; Ctrl+Z and
//	    Ctrl+Y walk the unsaved mark off and back on (C);
//	 6. an UNSAVED edit (a tree);
//	 7. P on the keyboard: a real game on the edited map, played by a throwaway
//	    hero who is not in the player's Saves (B3);
//	 8. the game ends -- ToMainMenu, where the escape menu's exit goes -- and the
//	    EDITOR comes back with the unsaved tree, the dirty mark and the undo
//	    history (A2); the process's map setting is the launch one again (B3);
//	    the playtest touched neither the authoring copy nor the repository, and
//	    the throwaway hero's folder is gone. Ctrl+S then writes the tree;
//	 9. the menu's own WORLD EDITOR button, which opens the repository's
//	    village, is refused under the harness -- and a normal game started
//	    afterwards is built from the launch map, not the playtest's (B3).
func TestWorldEditor(t *testing.T) {
	if os.Getenv("STRIGOI_HARNESS_ADDR") != "" {
		t.Skip("attached to a running game; this script launches its own with -editor")
	}

	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}

	repoVillage := filepath.Join(repoRoot, "data", "strigoi", "maps", "village.tmj")
	villageBefore := hashFile(t, repoVillage)
	t.Logf("the repository's village is %s", villageBefore[:16])

	raw, err := os.ReadFile(repoVillage)
	if err != nil {
		t.Fatal(err)
	}

	exe, err := harnessBinary(repoRoot)
	if err != nil {
		t.Fatal(err)
	}

	mirror := filepath.Join(filepath.Dir(exe), "data", "strigoi")
	authoring := filepath.Join(mirror, "maps", "editor-accept.tmj")
	scratch := filepath.Join(mirror, "maps", "playtest-scratch.tmj")

	if err := os.WriteFile(authoring, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		for _, p := range []string{authoring, d2mapedit.BackupPath(authoring), scratch} {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				t.Logf("LEFT A FILE BEHIND: %v", err)
			}
		}
	})

	editorRefusesABrokenMap(t, mirror, raw)

	// --- act 1: -editor opens the authoring copy, by its absolute path -------
	s := startWith(t, "-editor="+authoring)
	waitForEditor(t, s)

	st := editorState(s)
	if !samePath(str(st, "disk_path"), authoring) {
		t.Fatalf("act 1: the editor opened %q, want %q", str(st, "disk_path"), authoring)
	}

	if got := str(st, "map_path"); got != "data/strigoi/maps/editor-accept.tmj" {
		t.Fatalf("act 1: the game would read the map as %q, want data/strigoi/maps/editor-accept.tmj", got)
	}

	if flag(t, st, "dirty") || mustNum(t, st, "undo_depth") != 0 {
		t.Fatalf("act 1: a map just opened is dirty or has history: %v", st)
	}

	info := s.call("strigoi_get_game_info", map[string]any{})
	launchMap := str(info, "map_asked")
	t.Logf("act 1: the editor has %s; the process's map setting is %q", str(st, "disk_path"), launchMap)

	// --- act 2: the first screen reads -----------------------------------------
	parkCursor(s)

	fit := mustNum(t, st, "fit_zoom")
	if z := mustNum(t, st, "zoom"); math.Abs(z-fit) > 1e-9 {
		t.Fatalf("act 2: the editor opened at zoom %v, not its fit %v", z, fit)
	}

	// THE GRID OFF (the second 28 Sep review). The grid is drawn along the
	// tile edges -- exactly where seams are -- so with it on the hole count
	// below saw half of them: 228 of 84,552 with the grid on, 449 with it off.
	// G turns it off, the way a hand does, and the provider says it went.
	s.call("strigoi_key", map[string]any{"key": "g"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if st = editorState(s); flag(t, st, "grid") {
		t.Fatal("act 2: G left the grid on, so the seam count below would see only half the seams")
	}

	shot, shotPath := screenshot(t, s, "editor-fit")
	tallest := tallestArt(t, authoring)

	stray, uncovered, inside := inkOutsideTheMap(shot, st, float64(tallest)*fit+2)
	t.Logf("act 2: at the fit zoom %.4f, grid off: %d map pixel(s) outside the diamond (+%d px of art above it), "+
		"%d of %d pixels inside it uncovered -- %s", fit, stray, int(float64(tallest)*fit+2), uncovered, inside, shotPath)

	if stray > 40 {
		t.Errorf("act 2: %d pixels of map art lie outside the map's own diamond at the fit zoom: the art is not "+
			"drawn at the zoom (A1) -- %s", stray, shotPath)
	}

	// NONE. MEASURED 28 Sep (second review, grid off): 449 of 84,552 (0.53%)
	// with the art scaled by the view plus one pixel -- over the half-percent
	// line this used to hold, which the grid had hidden. With each tile sized
	// to its neighbours' floored anchors (MapRenderer.tileArtScale), 0.
	// Negative control: the first fix put back, this act and act 3 count 465,
	// 328, 16 and 0 at the fit zoom, 0.15, 0.5 and 0.25
	// (strigoi-harness-runs\wt-editor-fix2\nc\nc6-pt-seam-pad-only.txt).
	if uncovered > 0 {
		t.Errorf("act 2: %d of %d pixels inside the map's diamond are empty at the fit zoom: the ground has "+
			"seams (%s)", uncovered, inside, shotPath)
	}

	for _, missing := range unmarkedPeople(shot, st) {
		t.Errorf("act 2: no marker where %s stands (B5) -- %s", missing, shotPath)
	}

	// Names at the fit zoom (the second review: "start" was drawn over the
	// smith's mark). Below the label zoom only the person under the cursor is
	// named: none while the cursor is off the map, and the smith once it is on
	// his tile -- his name clear of every mark, whole, inside the map's view.
	if n := checkLabels(t, st, "act 2 (fit zoom, cursor parked)", shotPath); n != 0 {
		t.Errorf("act 2: at the fit zoom %.3f (under the label zoom %v) %d name(s) are drawn with the cursor off "+
			"the map -- %s", fit, st["label_zoom"], n, shotPath)
	}

	smith := personNamed(t, st, "smith")
	sx, sy := tileScreen(st, num(smith, "tile_x")+0.5, num(smith, "tile_y")+0.5)
	s.call("strigoi_move_cursor", map[string]any{"x": int(sx), "y": int(sy)})
	s.call("strigoi_step", map[string]any{"frames": 2})

	_, hoverPath := screenshot(t, s, "editor-fit-hover-smith")
	st = editorState(s)

	if checkLabels(t, st, "act 2 (fit zoom, over the smith)", hoverPath) != 1 ||
		len(list(personNamed(t, st, "smith"), "label")) != 4 {
		t.Errorf("act 2: at the fit zoom, with the cursor on the smith's tile, his name is not the one drawn -- %s",
			hoverPath)
	}

	parkCursor(s)

	// --- act 3: the scripted zoom ---------------------------------------------
	// The seam count at the zooms either side of the one act 4 works at, then
	// 0.25 itself: none at any of them, grid off. (With the art scaled by the
	// view plus one pixel the model counted none at 0.15, 0.25 and 0.5 but
	// 1,222 at 0.077 -- TestTheGroundHasNoSeamsAtTheEditorsZooms.)
	for _, z := range []float64{0.15, 0.5, 0.25} {
		st = sub(s.call("strigoi_set_system_field", map[string]any{"system": "editor", "field": "zoom", "value": z}),
			"state")
		if got := mustNum(t, st, "zoom"); math.Abs(got-z) > 1e-9 {
			t.Fatalf("act 3: zoom reads %v after setting %v", got, z)
		}

		s.call("strigoi_step", map[string]any{"frames": 3})
		zshot, zpath := screenshot(t, s, fmt.Sprintf("editor-%03d", int(math.Round(z*100))))

		_, uncovered, inside = inkOutsideTheMap(zshot, st, float64(tallest)*z+2)
		t.Logf("act 3: at %v, grid off, %d of %d pixels inside the map's diamond uncovered -- %s",
			z, uncovered, inside, zpath)

		if uncovered > 0 {
			t.Errorf("act 3: %d of %d pixels inside the map's diamond are empty at %v: the ground has seams (%s)",
				uncovered, inside, z, zpath)
		}
	}

	before, beforePath := screenshot(t, s, "editor-025")
	st = editorState(s)

	// Names at 0.25 (the second review: the palette cut the headman's name to
	// "headman ..."): every person named, each name whole, inside the map's
	// view, and clear of every mark and every other name.
	if n, want := checkLabels(t, st, "act 3 (0.25)", beforePath), len(list(st, "people")); n != want {
		t.Errorf("act 3: at 0.25 %d of %d people are named -- %s", n, want, beforePath)
	}

	// --- act 4: a house placed by clicks ---------------------------------------
	// Picked in the Buildings tab, then the greyed Terrain tab clicked before
	// the map is: the house is judged by ITS tab, not the one on show (B4,
	// BUG-33 -- until the second review only its unit test said so). Negative
	// control: canPlace asking the tab on show again, and the ghost says no to
	// every tile with Terrain's reason (wt-editor-fix2\nc\nc7-pt-viewed-tab.txt).
	house := pickRow(t, s, "Peasant house")
	pickTab(t, s, "Terrain")

	if st := editorState(s); str(st, "tab") != "Terrain" || str(st, "tool") != "place" || str(st, "picked") == "" {
		t.Fatalf("act 4: after clicking the Terrain tab the editor shows %q, tool %q, holding %q",
			str(st, "tab"), str(st, "tool"), str(st, "picked"))
	}

	x, y := placeByGhost(t, s, authoring, 3)

	st = editorState(s)
	if mustNum(t, st, "undo_depth") != 1 || !flag(t, st, "dirty") {
		t.Fatalf("act 4: after placing, undo depth %v dirty %v (%s)", st["undo_depth"], st["dirty"], str(st, "message"))
	}

	placed := image.Rect(x-2, y-2, x+1, y+1) // the click is the footprint's front tile

	s.call("strigoi_key", map[string]any{"key": "escape"}) // put the house down
	parkCursor(s)

	after, afterPath := screenshot(t, s, "editor-025-house-placed")
	changed, of := changedInside(before, after, editorState(s), placed)
	t.Logf("act 4: %s placed with its front tile at %d,%d; %d of %d pixels on its footprint changed -- %s",
		house, x, y, changed, of, afterPath)

	if of == 0 || changed*3 < of {
		t.Errorf("act 4: only %d of %d pixels on the new house's footprint changed: it is not drawn where it stands",
			changed, of)
	}

	// --- act 5: Ctrl+S, and what is on disk -----------------------------------
	ctrl(s, "s")

	st = editorState(s)
	if flag(t, st, "dirty") || !strings.HasPrefix(str(st, "message"), "saved") {
		t.Fatalf("act 5: Ctrl+S left dirty=%v message %q", st["dirty"], str(st, "message"))
	}

	saved, err := os.ReadFile(authoring)
	if err != nil {
		t.Fatal(err)
	}

	// The hash instrument's own control: the save really changed the file, so
	// the equalities acts 8 and 9 rest on are not a blind instrument agreeing
	// with itself.
	if h := hashFile(t, authoring); h == hex.EncodeToString(func() []byte { s := sha256.Sum256(raw); return s[:] }()) {
		t.Fatal("act 5: the save left the file's hash where it was: the hash instrument cannot see a change")
	}

	if bak, err := os.ReadFile(d2mapedit.BackupPath(authoring)); err != nil || !bytes.Equal(bak, raw) {
		t.Fatalf("act 5: the .bak is not the generation the save replaced (%v)", err)
	}

	m, err := d2maptiled.Parse(saved, ".", func(p string) ([]byte, error) {
		return os.ReadFile(filepath.Join(filepath.Dir(authoring), filepath.FromSlash(path.Clean(p))))
	})
	if err != nil {
		t.Fatalf("act 5: the ENGINE refuses the saved file: %v", err)
	}

	if !hasFootprint(m, placed) {
		t.Fatalf("act 5: the saved map has no structure on %v", placed)
	}

	doc, err := d2mapedit.OpenFile(authoring)
	if err != nil {
		t.Fatal(err)
	}

	if o, ok := doc.StructureOn(x, y); !ok || o.Name != "peasant-house" {
		t.Fatalf("act 5: the saved house is named %q, want the village's own \"peasant-house\"", o.Name)
	}

	ctrl(s, "z")

	if !flag(t, editorState(s), "dirty") {
		t.Fatal("act 5: undoing past the save left the map marked saved")
	}

	ctrl(s, "y")

	if flag(t, editorState(s), "dirty") {
		t.Fatal("act 5: redoing back to the saved map left it marked unsaved")
	}

	t.Logf("act 5: Ctrl+S wrote a file the engine parses, with the house on %v named peasant-house; "+
		"Ctrl+Z / Ctrl+Y walk the unsaved mark off and on", placed)

	// --- act 6: an unsaved edit --------------------------------------------------
	pickTab(t, s, "Props")
	pickRow(t, s, "Tree")
	tx, ty := placeByGhost(t, s, authoring, 1)

	s.call("strigoi_key", map[string]any{"key": "escape"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	st = editorState(s)
	depth, label := mustNum(t, st, "undo_depth"), str(st, "undo_label")

	if !flag(t, st, "dirty") || depth != 2 {
		t.Fatalf("act 6: after the tree, dirty %v depth %v", st["dirty"], depth)
	}

	onDisk := hashFile(t, authoring)
	t.Logf("act 6: a tree on %d,%d, unsaved (%s)", tx, ty, label)

	// --- act 7: P ------------------------------------------------------------------
	s.call("strigoi_key", map[string]any{"key": "p"})

	info = waitForScreen(t, s, "game")
	if !flag(t, info, "playtest") {
		t.Fatalf("act 7: P started a game that is not a playtest: %v", info)
	}

	if got := str(info, "map_built"); got != "/data/strigoi/maps/playtest-scratch.tmj" || str(info, "map_error") != "" {
		t.Fatalf("act 7: the playtest built %q (error %q), not the editor's scratch map", got, str(info, "map_error"))
	}

	if name := str(sub(s.call("strigoi_get_player", map[string]any{}), "state"), "name"); name != "Playtest" {
		t.Fatalf("act 7: the playtest is played by %q, not the throwaway hero", name)
	}

	// P PLAYS THE SHIPPED GAME (the F5 review's B3, decided on its default
	// for Josh to overturn): fog on and the camera at 0.5, as a launch with
	// no switches. Negative control (2 Oct 2026): -fog's default put back to
	// false and this fails (wt-fog5\nc-a2-editor-fog-off.txt).
	s.call("strigoi_step", map[string]any{"frames": 30})

	if f, v := fogState(s), mustNum(t, uiState(s), "view_scale"); !flag(t, f, "enabled") || v != 0.5 {
		t.Fatalf("act 7: P plays the shipped game -- fog on, zoom 0.5; it has fog enabled=%v (off_reason %q) at zoom %v",
			f["enabled"], str(f, "off_reason"), v)
	}

	heroDir := str(info, "playtest_save_dir")
	home, err := testHome(t)
	if err != nil {
		t.Fatal(err)
	}

	realSaves := filepath.Join(home, "OpenDiablo2", "Saves")
	if heroDir == "" || strings.HasPrefix(strings.ToLower(heroDir), strings.ToLower(realSaves)) {
		t.Fatalf("act 7: the playtest hero lives in %q, inside the player's Saves", heroDir)
	}

	if entries, _ := os.ReadDir(realSaves); len(entries) > 0 {
		t.Fatalf("act 7: a playtest wrote %d file(s) into the player's Saves (%s)", len(entries), entries[0].Name())
	}

	s.call("strigoi_step_world", map[string]any{"world_minutes": 30.0})
	s.call("strigoi_step", map[string]any{"frames": 60})
	_, gamePath := screenshot(t, s, "playtest-in-game")

	t.Logf("act 7: P is a real game on %s, played by Playtest from %s -- %s", str(info, "map_built"), heroDir, gamePath)

	// --- act 8: back to the editor ----------------------------------------------
	s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
	info = waitForScreen(t, s, "world_editor")

	st = editorState(s)
	if !flag(t, st, "dirty") || mustNum(t, st, "undo_depth") != depth || str(st, "undo_label") != label {
		t.Fatalf("act 8: back from the playtest the editor has dirty=%v depth=%v last edit %q; "+
			"want the unsaved tree: dirty, depth %v, %q (A2)", st["dirty"], st["undo_depth"], str(st, "undo_label"),
			depth, label)
	}

	if !samePath(str(st, "disk_path"), authoring) || !strings.Contains(str(st, "message"), "back from the playtest") {
		t.Fatalf("act 8: the editor came back on %q saying %q", str(st, "disk_path"), str(st, "message"))
	}

	if flag(t, info, "playtest") || str(info, "map_asked") != launchMap {
		t.Fatalf("act 8: after the playtest the process builds %q (playtest=%v); want the launch map %q again (B3)",
			str(info, "map_asked"), info["playtest"], launchMap)
	}

	if h := hashFile(t, authoring); h != onDisk {
		t.Fatalf("act 8: THE PLAYTEST CHANGED THE AUTHORING COPY: %s -> %s", onDisk[:16], h[:16])
	}

	if h := hashFile(t, repoVillage); h != villageBefore {
		t.Fatalf("act 8: THE REPOSITORY'S VILLAGE CHANGED: %s -> %s", villageBefore[:16], h[:16])
	}

	gone := false

	for i := 0; i < 40 && !gone; i++ {
		time.Sleep(100 * time.Millisecond)
		s.call("strigoi_step", map[string]any{"frames": 5})

		_, err := os.Stat(heroDir)
		gone = os.IsNotExist(err)
	}

	if !gone {
		t.Fatalf("act 8: the throwaway hero's folder %s is still there", heroDir)
	}

	parkCursor(s)
	_, backPath := screenshot(t, s, "editor-back-from-playtest")

	ctrl(s, "s")

	doc, err = d2mapedit.OpenFile(authoring)
	if err != nil {
		t.Fatal(err)
	}

	if doc.WallTile(tx, ty) == 0 {
		t.Fatalf("act 8: Ctrl+S after the playtest wrote no tree on %d,%d -- the edit did not survive", tx, ty)
	}

	t.Logf("act 8: back in the editor with the unsaved tree, depth %v, map setting %q again, the hero's folder gone; "+
		"Ctrl+S then wrote the tree -- %s", depth, launchMap, backPath)

	// --- act 9: the repository's village is refused; the launch map is back ------
	s.call("strigoi_key", map[string]any{"key": "escape"})
	waitForScreen(t, s, "main_menu")

	button := menuButton(t, s, "world_editor")
	s.call("strigoi_click", map[string]any{"x": int(num(button, "x") + num(button, "w")/2),
		"y": int(num(button, "y") + num(button, "h")/2), "button": "left"})
	s.call("strigoi_step", map[string]any{"frames": 240})

	if got := str(s.call("strigoi_get_game_info", map[string]any{}), "screen"); got == "world_editor" {
		t.Fatal("act 9: under the harness the menu's WORLD EDITOR button opened the repository's village")
	}

	if h := hashFile(t, repoVillage); h != villageBefore {
		t.Fatalf("act 9: THE REPOSITORY'S VILLAGE CHANGED: %s -> %s", villageBefore[:16], h[:16])
	}

	g := s.call("strigoi_start_game", map[string]any{
		"hero_name": "After", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})

	if built := str(g, "map_built"); "/"+strings.TrimPrefix(built, "/") != "/"+strings.TrimPrefix(launchMap, "/") {
		t.Fatalf("act 9: the next game was built from %q, not the launch map %q (B3)", built, launchMap)
	}

	t.Logf("act 9: the WORLD EDITOR button's village was refused, and the next game was built from %s", launchMap)
}

// TestWorldEditorRefusesTheWorkingTree is the second 28 Sep review's B, driven
// the way the reviewer drove it. The first guard refused data/strigoi/maps/
// village.tmj alone; the reviewer copied the village to a second map in the SAME
// working-tree folder, launched the harness game with -editor naming it by its
// relative path -- which the editor resolved against the working directory, the
// repository -- and a scripted Delete + Ctrl+S overwrote the working-tree file.
// Under the harness the editor now refuses any map found by that fallback, and
// any map inside the source tree (d2app/harness.go harnessEditorGuard).
//
// The act: a second map in the working tree, opened under the harness by the
// same relative path. It must be refused -- the game goes to the main menu with
// the guard's own reason -- and the file must be as it was.
//
// Negative control (28 Sep 2026): make harnessEditorGuard return nil and this
// goes red -- the editor opens the working tree's file. Log:
// strigoi-harness-runs\wt-editor-fix2\nc\nc2-pt-no-guard.txt.
func TestWorldEditorRefusesTheWorkingTree(t *testing.T) {
	if os.Getenv("STRIGOI_HARNESS_ADDR") != "" {
		t.Skip("attached to a running game; this script launches its own with -editor")
	}

	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(repoRoot, "data", "strigoi", "maps", "village.tmj"))
	if err != nil {
		t.Fatal(err)
	}

	const rel = "data/strigoi/maps/zz-harness-guard-second.tmj"

	second := filepath.Join(repoRoot, filepath.FromSlash(rel))
	if err := os.WriteFile(second, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		for _, p := range []string{second, d2mapedit.BackupPath(second)} {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				t.Logf("LEFT A FILE IN THE WORKING TREE: %v", err)
			}
		}
	})

	before := hashFile(t, second)

	s := startWith(t, "-editor="+rel)

	screen := ""

	for i := 0; i < 60 && screen != "main_menu" && screen != "world_editor"; i++ {
		s.call("strigoi_step", map[string]any{"frames": 10})

		info := s.call("strigoi_get_game_info", map[string]any{})
		if !flag(t, info, "loading") {
			screen = str(info, "screen")
		}
	}

	if screen == "world_editor" {
		t.Fatalf("under the harness -editor=%s opened %q -- the working tree's own file; a scripted Ctrl+S would "+
			"write it", rel, str(editorState(s), "disk_path"))
	}

	if screen != "main_menu" {
		t.Fatalf("under the harness -editor=%s went to %q, not the main menu with the refusal", rel, screen)
	}

	why := ""

	for i := 0; i < 30 && why == ""; i++ {
		if s.callErr("strigoi_get_system_state", map[string]any{"system": "ui"}) == "" {
			why = str(uiState(s), "main_menu_error")
		}

		if why == "" {
			s.call("strigoi_step", map[string]any{"frames": 10})
		}
	}

	if !strings.Contains(why, "never by the working directory") {
		t.Fatalf("the main menu says %q, not the harness's refusal of the working directory", why)
	}

	if h := hashFile(t, second); h != before {
		t.Fatalf("THE WORKING TREE'S FILE CHANGED: %s -> %s", before[:16], h[:16])
	}

	t.Logf("-editor=%s is refused under the harness: %q; the file is as it was", rel, why)
}

// editorRefusesABrokenMap is act 0, a launch of its own: a copy of the village
// whose grass PNG is cut after 40 bytes -- a real signature and size, no
// picture. The validator (which reads the header) passes it and the engine
// (which decodes it) refuses it: the review saved exactly this file.
func editorRefusesABrokenMap(t *testing.T, mirror string, raw []byte) {
	t.Helper()

	tree := filepath.Join(mirror, "zz-editor-bad")
	bad := filepath.Join(tree, "maps", "bad.tmj")

	t.Cleanup(func() {
		if err := os.RemoveAll(tree); err != nil {
			t.Logf("LEFT A FOLDER BEHIND: %v", err)
		}
	})

	// The map's art, beside it the way the village's is: tiles/ under the map,
	// ../structures/ beside its folder.
	for _, dir := range []string{"maps/tiles", "structures"} {
		rel := filepath.FromSlash(dir)
		if err := copyTree(filepath.Join(mirror, rel), filepath.Join(tree, rel)); err != nil {
			t.Fatal(err)
		}
	}

	grass := filepath.Join(tree, "maps", "tiles", "placeholder-grass.png")

	whole, err := os.ReadFile(grass)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(grass, whole[:40], 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(bad, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	badBefore := hashFile(t, bad)

	s := startWith(t, "-editor="+bad)
	defer s.stop()

	waitForEditor(t, s)

	st := editorState(s)
	if str(st, "engine_error") == "" || mustNum(t, st, "problems") != 0 {
		t.Fatalf("act 0: the broken copy was meant to pass the validator and fail the engine: engine %q, %v problem(s)",
			str(st, "engine_error"), st["problems"])
	}

	parkCursor(s)

	shot, shotPath := screenshot(t, s, "editor-refused-map")
	red := countColour(shot, viewRect(st), color.RGBA{R: 0xff, G: 0x50, B: 0x50, A: 0xff}, 8)

	t.Logf("act 0: the engine refuses the copy (%s); %d notice pixel(s) in the map area -- %s",
		str(st, "engine_error"), red, shotPath)

	if red < 500 {
		t.Errorf("act 0: the map area shows %d pixels of the refusal notice: a refused map reads as an empty grid (C) -- %s",
			red, shotPath)
	}

	ctrl(s, "s")

	if msg := str(editorState(s), "message"); !strings.HasPrefix(msg, "NOT SAVED") {
		t.Fatalf("act 0: Ctrl+S on a map the engine refuses said %q (B2)", msg)
	}

	if h := hashFile(t, bad); h != badBefore {
		t.Fatalf("act 0: Ctrl+S wrote a map the engine refuses (B2)")
	}

	s.call("strigoi_key", map[string]any{"key": "p"})
	s.call("strigoi_step", map[string]any{"frames": 60})

	if info := s.call("strigoi_get_game_info", map[string]any{}); str(info, "screen") != "world_editor" ||
		flag(t, info, "in_game") {
		t.Fatalf("act 0: P on a map the engine refuses left the editor: %v (B2)", info)
	}

	if msg := str(editorState(s), "message"); !strings.HasPrefix(msg, "cannot playtest") {
		t.Fatalf("act 0: P on a map the engine refuses said %q (B2)", msg)
	}

	t.Log("act 0: Ctrl+S and P both refuse the copy, and the file is as it was")
}

// ---- the instruments ----------------------------------------------------------

// checkLabels is the names' instrument (the second 28 Sep review's C), read
// from the editor provider's record of the last frame drawn: every name drawn
// lies wholly inside the map's view, is the person's whole name, keeps two
// pixels from every mark and overlaps no other name. It answers how many names
// were drawn. Negative control: the names placed as the second review found
// them, and it reports "start" over the smith's mark at the fit zoom and the
// headman's name drawn as "headman ..." at 0.25
// (strigoi-harness-runs\wt-editor-fix2\nc\nc8-pt-old-names.txt).
func checkLabels(t *testing.T, st map[string]any, act, shot string) int {
	t.Helper()

	view := viewRect(st)

	type box struct {
		who string
		r   image.Rectangle
	}

	var marks, names []box

	for _, v := range list(st, "people") {
		p, _ := v.(map[string]any)

		who := str(p, "name")
		if str(p, "class") == "player_start" {
			who = "start"
		}

		if m := rectOf(list(p, "mark")); !m.Empty() {
			marks = append(marks, box{who, m})
		}

		l := rectOf(list(p, "label"))
		if l.Empty() {
			continue
		}

		if got := str(p, "label_text"); got != who {
			t.Errorf("%s: %s's name is drawn as %q -- %s", act, who, got, shot)
		}

		if !l.In(view) {
			t.Errorf("%s: %s's name %v is not wholly inside the map's view %v -- %s", act, who, l, view, shot)
		}

		names = append(names, box{who, l})
	}

	if len(marks) == 0 {
		t.Errorf("%s: the provider reports no marks at all -- %s", act, shot)
	}

	for i, n := range names {
		// Two pixels round every mark: a name glued to someone else's mark
		// reads as his (the headman's, flipped left at 0.25, began one pixel
		// after the woman's mark in this fix's first run).
		for _, m := range marks {
			if n.r.Overlaps(m.r.Inset(-2)) {
				t.Errorf("%s: %s's name %v lies over or against %s's mark %v -- %s", act, n.who, n.r, m.who, m.r,
					shot)
			}
		}

		for _, o := range names[i+1:] {
			if n.r.Overlaps(o.r) {
				t.Errorf("%s: %s's name %v lies over %s's %v -- %s", act, n.who, n.r, o.who, o.r, shot)
			}
		}
	}

	return len(names)
}

// personNamed is the provider's row for the person whose name holds name, with
// his tile as tile_x and tile_y.
func personNamed(t *testing.T, st map[string]any, name string) map[string]any {
	t.Helper()

	for _, v := range list(st, "people") {
		p, _ := v.(map[string]any)
		if strings.Contains(str(p, "name"), name) {
			if tile := list(p, "tile"); len(tile) == 2 {
				p["tile_x"], p["tile_y"] = tile[0], tile[1]
			}

			return p
		}
	}

	t.Fatalf("no person named %q on the map", name)

	return nil
}

// rectOf reads a provider rectangle, [x0, y0, x1, y1]; empty for anything else.
func rectOf(v []any) image.Rectangle {
	if len(v) != 4 {
		return image.Rectangle{}
	}

	f := func(i int) int { n, _ := v[i].(float64); return int(n) }

	return image.Rect(f(0), f(1), f(2), f(3))
}

// editorState is the editor's harness provider.
func editorState(s *session) map[string]any {
	return sub(s.call("strigoi_get_system_state", map[string]any{"system": "editor"}), "state")
}

// waitForEditor steps until the editor has loaded and registered its provider.
// A map the editor refused sends the game to the main menu, and this fails.
func waitForEditor(t *testing.T, s *session) {
	t.Helper()

	waitForScreen(t, s, "world_editor")

	for i := 0; i < 60; i++ {
		if s.callErr("strigoi_get_system_state", map[string]any{"system": "editor"}) == "" {
			return
		}

		s.call("strigoi_step", map[string]any{"frames": 10})
	}

	t.Fatal("the editor never registered its provider")
}

// waitForScreen steps until the game says it is on screen (and not loading).
func waitForScreen(t *testing.T, s *session, screen string) map[string]any {
	t.Helper()

	var info map[string]any

	for i := 0; i < 90; i++ {
		s.call("strigoi_step", map[string]any{"frames": 20})

		info = s.call("strigoi_get_game_info", map[string]any{})
		if str(info, "screen") == screen && !flag(t, info, "loading") {
			if screen != "game" || flag(t, info, "in_game") {
				return info
			}
		}
	}

	t.Fatalf("the game never reached screen %q: %v", screen, info)

	return nil
}

// parkCursor puts the pointer on the status bar, off the map and the palette,
// so it is in no screenshot's measured area.
func parkCursor(s *session) {
	s.call("strigoi_move_cursor", map[string]any{"x": 700, "y": 590})
	s.call("strigoi_step", map[string]any{"frames": 3})
}

// ctrl presses Ctrl+key on the keyboard, as a hand does: Control held, the key
// tapped, Control released.
func ctrl(s *session, key string) {
	s.call("strigoi_key", map[string]any{"key": "control", "action": "down"})
	s.call("strigoi_key", map[string]any{"key": key})
	s.call("strigoi_key", map[string]any{"key": "control", "action": "up"})
	s.call("strigoi_step", map[string]any{"frames": 3})
}

func screenshot(t *testing.T, s *session, name string) (image.Image, string) {
	t.Helper()

	p := str(s.call("strigoi_screenshot", map[string]any{"name": name}), "path")

	return readPNG(t, p), p
}

// pickTab clicks a palette tab by its title.
func pickTab(t *testing.T, s *session, title string) {
	t.Helper()

	for _, v := range list(editorState(s), "tabs") {
		tab, _ := v.(map[string]any)
		if str(tab, "title") == title {
			s.call("strigoi_click", map[string]any{"x": int(num(tab, "x") + num(tab, "w")/2),
				"y": int(num(tab, "y") + num(tab, "h")/2), "button": "left"})
			s.call("strigoi_step", map[string]any{"frames": 2})

			return
		}
	}

	t.Fatalf("no palette tab %q", title)
}

// pickRow clicks a palette row by its name and checks the editor picked it up.
func pickRow(t *testing.T, s *session, name string) string {
	t.Helper()

	for _, v := range list(editorState(s), "rows") {
		row, _ := v.(map[string]any)
		if str(row, "name") != name {
			continue
		}

		s.call("strigoi_click", map[string]any{"x": int(num(row, "x") + num(row, "w")/2),
			"y": int(num(row, "y") + num(row, "h")/2), "button": "left"})
		s.call("strigoi_step", map[string]any{"frames": 2})

		if st := editorState(s); str(st, "picked") != str(row, "id") || str(st, "tool") != "place" {
			t.Fatalf("clicking the %s row picked %q (tool %s): %s", name, str(st, "picked"), str(st, "tool"),
				str(st, "message"))
		}

		return name
	}

	t.Fatalf("no palette row %q on show", name)

	return ""
}

// placeByGhost finds a tile the held piece may go on and clicks it. Candidates
// are the tiles nearest the view's middle whose side x side footprint, laid
// back from the tile, is clear in the saved document; each is hovered and the
// click is made only where the editor's OWN ghost says yes -- the same question
// the click asks. It answers the tile clicked.
func placeByGhost(t *testing.T, s *session, file string, side int) (int, int) {
	t.Helper()

	doc, err := d2mapedit.OpenFile(file)
	if err != nil {
		t.Fatal(err)
	}

	st := editorState(s)
	view := viewRect(st)
	cx, cy := float64(view.Min.X+view.Max.X)/2, float64(view.Min.Y+view.Max.Y)/2

	type cand struct {
		x, y int
		d    float64
	}

	var cands []cand

	for y := side - 1; y < doc.Size().Y; y++ {
		for x := side - 1; x < doc.Size().X; x++ {
			if !clearFor(doc, x, y, side) {
				continue
			}

			sx, sy := tileScreen(st, float64(x)+0.5, float64(y)+0.5)
			if !image.Pt(int(sx), int(sy)).In(view.Inset(8)) {
				continue
			}

			cands = append(cands, cand{x, y, math.Hypot(sx-cx, sy-cy)})
		}
	}

	for tries := 0; tries < 25 && len(cands) > 0; tries++ {
		best := 0
		for i := range cands {
			if cands[i].d < cands[best].d {
				best = i
			}
		}

		c := cands[best]
		cands = append(cands[:best], cands[best+1:]...)

		sx, sy := tileScreen(st, float64(c.x)+0.5, float64(c.y)+0.5)

		s.call("strigoi_move_cursor", map[string]any{"x": int(sx), "y": int(sy)})
		s.call("strigoi_step", map[string]any{"frames": 2})

		hover := sub(editorState(s), "hover")
		if tile := list(hover, "tile"); len(tile) != 2 || int(tile[0].(float64)) != c.x || int(tile[1].(float64)) != c.y {
			continue
		}

		if ok, _ := hover["can_place"].(bool); !ok {
			continue
		}

		s.call("strigoi_click", map[string]any{"x": int(sx), "y": int(sy), "button": "left"})
		s.call("strigoi_step", map[string]any{"frames": 2})

		return c.x, c.y
	}

	t.Fatalf("the ghost said yes to none of the tiles tried: %s", str(editorState(s), "message"))

	return -1, -1
}

// clearFor is the document's side of the ghost's question: every tile of the
// footprint laid back from x, y on the map, floored, and free of walls,
// structures and people.
func clearFor(doc *d2mapedit.Doc, x, y, side int) bool {
	people := map[image.Point]bool{}

	for _, o := range doc.Objects() {
		if !o.IsStructure() {
			people[o.Tile()] = true
		}
	}

	for ty := y - side + 1; ty <= y; ty++ {
		for tx := x - side + 1; tx <= x; tx++ {
			if tx < 0 || ty < 0 || tx >= doc.Size().X || ty >= doc.Size().Y {
				return false
			}

			if _, on := doc.StructureOn(tx, ty); on || doc.FloorTile(tx, ty) == 0 || doc.WallTile(tx, ty) != 0 ||
				people[image.Pt(tx, ty)] {
				return false
			}
		}
	}

	return true
}

// tileScreen is where world point x, y is on screen, from the provider's
// world_to_screen (origin + x*x_axis + y*y_axis).
func tileScreen(st map[string]any, x, y float64) (float64, float64) {
	w := sub(st, "world_to_screen")
	o, xa, ya := list(w, "origin"), list(w, "x_axis"), list(w, "y_axis")

	f := func(v []any, i int) float64 { n, _ := v[i].(float64); return n }

	return f(o, 0) + x*f(xa, 0) + y*f(ya, 0), f(o, 1) + x*f(xa, 1) + y*f(ya, 1)
}

func viewRect(st map[string]any) image.Rectangle {
	v := list(st, "view")
	f := func(i int) int { n, _ := v[i].(float64); return int(n) }

	return image.Rect(f(0), f(1), f(2), f(3))
}

func list(m map[string]any, key string) []any {
	v, _ := m[key].([]any)
	return v
}

// tallestArt is the tallest piece of art the map's tileset declares, in art
// pixels: how far above its own tile anything on this map can stand.
func tallestArt(t *testing.T, file string) int {
	t.Helper()

	doc, err := d2mapedit.OpenFile(file)
	if err != nil {
		t.Fatal(err)
	}

	tallest := 0
	for _, k := range doc.Kinds() {
		if k.Declared.Y > tallest {
			tallest = k.Declared.Y
		}
	}

	return tallest
}

// editorVoid is the editor's map-view background (edColVoid).
var editorVoid = color.RGBA{R: 0x0a, G: 0x0a, B: 0x10, A: 0xff}

func near(c color.Color, want color.RGBA, tol int) bool {
	r, g, b, _ := c.RGBA()
	d := func(a uint32, w uint8) bool { return math.Abs(float64(int(a>>8)-int(w))) <= float64(tol) }

	return d(r, want.R) && d(g, want.G) && d(b, want.B)
}

// inkOutsideTheMap is act 2's instrument: over the map's view (inset two
// pixels), every pixel that is not the background must lie inside the map's
// diamond on screen -- its corners from the provider -- or ABOVE it by no more
// than above, the height the tallest art can stand. A view whose art is drawn
// at full size on anchors scaled to 0.08 fails it by thousands of pixels. It
// also counts the pixels well inside the diamond that are background: a hole
// in the ground.
//
// A HOLE IS THE BACKGROUND EXACTLY (the second 28 Sep review). The editor fills
// the view with edColVoid and nothing else paints it, so a seam shows that
// colour to the bit: every background pixel of the reviewer's own grid-off
// screenshot at the fit zoom is exact but one (193,724 of 193,725). Within two
// levels, as the ink test below allows, a dark timber in a structure's art
// (0x0b0c0e) counted as one at 0.5.
func inkOutsideTheMap(img image.Image, st map[string]any, above float64) (stray, uncovered, inside int) {
	corners := list(st, "map_corners")
	pt := func(i int) (float64, float64) {
		c, _ := corners[i].([]any)
		x, _ := c[0].(float64)
		y, _ := c[1].(float64)

		return x, y
	}

	topX, topY := pt(0)
	rightX, _ := pt(1)
	_, bottomY := pt(2)
	leftX, midY := pt(3)

	cx, cy := topX, midY
	hw, hh := (rightX-leftX)/2, (bottomY-topY)/2

	const pad = 3.0

	view := viewRect(st).Inset(2)

	for y := view.Min.Y; y < view.Max.Y; y++ {
		for x := view.Min.X; x < view.Max.X; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			void := near(img.At(x, y), editorVoid, 2)

			// Well inside: must be covered.
			if math.Abs(fx-cx)/(hw-pad)+math.Abs(fy-cy)/(hh-pad) <= 1 {
				inside++

				if near(img.At(x, y), editorVoid, 0) {
					uncovered++
				}

				continue
			}

			if void {
				continue
			}

			// Ink: allowed inside the padded diamond, or above it within reach.
			dx := math.Abs(fx-cx) / (hw + pad)
			if dx > 1 {
				stray++
				continue
			}

			half := (hh + pad) * (1 - dx)
			if fy > cy+half || fy < cy-half-above {
				stray++
			}
		}
	}

	return stray, uncovered, inside
}

// unmarkedPeople names every person (and the start) with no pixel of his
// marker's colour within three pixels of where the provider says he is drawn.
func unmarkedPeople(img image.Image, st map[string]any) []string {
	var missing []string

	for _, v := range list(st, "people") {
		p, _ := v.(map[string]any)

		want := color.RGBA{R: 0x60, G: 0xd8, B: 0xff, A: 0xff}
		if str(p, "class") == "player_start" {
			want = color.RGBA{R: 0xff, G: 0xb0, B: 0x30, A: 0xff}
		}

		scr := list(p, "screen")
		if len(scr) != 2 {
			missing = append(missing, str(p, "name")+" (no screen position)")
			continue
		}

		x, _ := scr[0].(float64)
		y, _ := scr[1].(float64)

		if countColour(img, image.Rect(int(x)-3, int(y)-3, int(x)+4, int(y)+4), want, 8) == 0 {
			missing = append(missing, fmt.Sprintf("%s at %d,%d", str(p, "name"), int(x), int(y)))
		}
	}

	return missing
}

func countColour(img image.Image, r image.Rectangle, want color.RGBA, tol int) int {
	n := 0

	r = r.Intersect(img.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if near(img.At(x, y), want, tol) {
				n++
			}
		}
	}

	return n
}

// changedInside counts the pixels inside a footprint's diamond on screen that
// differ between two screenshots.
func changedInside(a, b image.Image, st map[string]any, fp image.Rectangle) (changed, of int) {
	tx, ty := tileScreen(st, float64(fp.Min.X), float64(fp.Min.Y))
	rx, ry := tileScreen(st, float64(fp.Max.X), float64(fp.Min.Y))
	bx, by := tileScreen(st, float64(fp.Max.X), float64(fp.Max.Y))
	lx, ly := tileScreen(st, float64(fp.Min.X), float64(fp.Max.Y))

	cx, cy := (tx+bx)/2, (ry+ly)/2
	hw, hh := (rx-lx)/2-2, (by-ty)/2-2

	for y := int(ty); y <= int(by); y++ {
		for x := int(lx); x <= int(rx); x++ {
			if math.Abs(float64(x)-cx)/hw+math.Abs(float64(y)-cy)/hh > 1 {
				continue
			}

			of++

			ar, ag, ab, _ := a.At(x, y).RGBA()
			br, bg, bb, _ := b.At(x, y).RGBA()

			if ar != br || ag != bg || ab != bb {
				changed++
			}
		}
	}

	return changed, of
}

func hasFootprint(m *d2maptiled.Map, fp image.Rectangle) bool {
	for _, s := range m.Structures {
		if s.Footprint == fp {
			return true
		}
	}

	return false
}

// menuButton waits for the main menu, clicks past its trademark page off every
// button, and answers the named button's rectangle (the "ui" provider).
func menuButton(t *testing.T, s *session, name string) map[string]any {
	t.Helper()

	var ui map[string]any

	for i := 0; i < 60; i++ {
		if s.callErr("strigoi_get_system_state", map[string]any{"system": "ui"}) == "" {
			if ui = uiState(s); str(ui, "screen") == "main_menu" {
				break
			}
		}

		s.call("strigoi_step", map[string]any{"frames": 10})
	}

	for i := 0; i < 10 && str(ui, "main_menu_page") != "main_menu"; i++ {
		s.call("strigoi_click", map[string]any{"x": 700, "y": 150, "button": "left"})
		s.call("strigoi_step", map[string]any{"frames": 10})
		ui = uiState(s)
	}

	b := sub(sub(ui, "main_menu_buttons"), name)
	if len(b) == 0 {
		t.Fatalf("the main menu reports no %s button: %v", name, ui)
	}

	return b
}

// samePath compares two Windows paths as the file system does.
func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// hashFile is the instrument the byte-for-byte acts rest on.
func hashFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("hashing %s: %v", path, err)
	}

	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}

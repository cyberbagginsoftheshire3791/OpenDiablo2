package d2gamescreen

import (
	"bytes"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2mapedit"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2mappalette"
)

// The 28 Sep 2026 review of World Editor v0, the parts of it that are the
// SCREEN's decisions and can be tested without drawing one (this package links
// no ebiten; see editor_test.go). The rendering half -- zoom that scales the art,
// the people markers, the refused-map notice -- is measured on screenshot pixels
// by playtest/editor_test.go and on painted pixels by d2maprenderer's
// draw_scale_test.go. docs/editor.md, "27-28 Sep review and fixes", lists all of
// it.

// docEditor is villageEditor with a DOCUMENT: the shipped village opened from
// the repository, its tileset indexed, an undo stack, the art read from the
// repository and the game's loader stood in for by one that reads the
// repository too. diskPath is where a save would write: a file in a temporary
// folder, never the repository.
func docEditor(t *testing.T) *Editor {
	t.Helper()

	e := villageEditor(t)

	doc, err := d2mapedit.OpenFile(repoFile(DefaultEditorMap))
	if err != nil {
		t.Fatal(err)
	}

	e.doc = doc
	e.stack = d2mapedit.NewStack(doc)
	e.mapPath = DefaultEditorMap
	e.diskPath = filepath.Join(t.TempDir(), "village-copy.tmj")
	e.art = d2mapedit.DirArt(filepath.Join(repoRoot(t), filepath.FromSlash(path.Dir(DefaultEditorMap))))
	e.load = repoLoader(t, nil)
	e.gids = map[string]int{}
	e.refreshGIDs()

	return e
}

// repoRoot is the repository, ABSOLUTE: one test changes the working directory.
func repoRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(repoFile("."))
	if err != nil {
		t.Fatal(err)
	}

	return root
}

// repoLoader reads what the engine's parser asks for from the repository, the
// way AssetManager.LoadFile reads it from the game's folder. cut, when set,
// answers for a path it names instead -- the review's truncated PNG.
func repoLoader(t *testing.T, cut map[string][]byte) d2maptiled.Loader {
	root := repoRoot(t)

	return func(p string) ([]byte, error) {
		clean := strings.TrimPrefix(path.Clean(p), "/")
		if data, ok := cut[clean]; ok {
			return data, nil
		}

		return os.ReadFile(filepath.Join(root, filepath.FromSlash(clean)))
	}
}

// entryNamed is the village palette's entry with the given display name.
func entryNamed(t *testing.T, e *Editor, name string) d2mappalette.Entry {
	t.Helper()

	for _, ent := range e.catalog.Entries() {
		if ent.DisplayName == name {
			return ent
		}
	}

	t.Fatalf("the village palette has no %q", name)

	return d2mappalette.Entry{}
}

func tabIndex(t *testing.T, e *Editor, cat d2mappalette.Category) int {
	t.Helper()

	for i, tab := range e.tabs {
		if tab.Category == cat {
			return i
		}
	}

	t.Fatalf("no %s tab", cat)

	return -1
}

// B4. THE HELD PIECE IS JUDGED BY ITS OWN TAB. The reviewer picked a house,
// clicked the greyed Terrain tab to read why it was greyed, went back to the map
// -- and every click was refused with Terrain's reason.
//
// Negative control (28 Sep 2026): make canPlace ask about e.tabs[e.tab] again
// and this fails -- "holding Peasant house with the Terrain tab on show, a
// clear tile refuses it: No terrain painting in v0 ...".
func TestEditorPlacesTheHeldPieceWhateverTabIsShowing(t *testing.T) {
	e := docEditor(t)
	house := entryNamed(t, e, "Peasant house")

	e.tab = tabIndex(t, e, d2mappalette.CategoryBuildings)
	e.refreshRows()

	// A tile the house may go on, found by the editor's own question.
	x, y, found := -1, -1, false

	for ty := 0; ty < e.doc.Size().Y && !found; ty++ {
		for tx := 0; tx < e.doc.Size().X && !found; tx++ {
			if ok, _ := e.canPlace(house, tx, ty); ok {
				x, y, found = tx, ty, true
			}
		}
	}

	if !found {
		t.Fatal("the village has nowhere a peasant house may go")
	}

	terrain := tabIndex(t, e, d2mappalette.CategoryTerrain)
	if e.tabs[terrain].Available {
		t.Fatal("the Terrain tab is available in v0 -- the test's premise is gone")
	}

	e.tab = terrain
	e.refreshRows()

	if ok, why := e.canPlace(house, x, y); !ok {
		t.Fatalf("holding %s with the Terrain tab on show, a clear tile %d,%d refuses it: %s",
			house.DisplayName, x, y, why)
	}

	// The control the other way: a piece whose OWN tab is unavailable is still
	// refused, whichever tab is on show.
	for _, ent := range e.catalog.InCategory(d2mappalette.CategoryTerrain) {
		e.tab = tabIndex(t, e, d2mappalette.CategoryBuildings)

		if ok, _ := e.canPlace(ent, x, y); ok {
			t.Fatalf("a Terrain piece (%s) was allowed from the Buildings tab; its own tab is not in v0", ent.DisplayName)
		}
	}
}

// C. DUPLICATING AGAINST THE EDGE SAYS "OFF THE MAP". A house duplicated one
// footprint east of a house at the east edge used to be refused as "bare
// ground", because every tile past the edge reads as no floor.
//
// Negative control (28 Sep 2026): take the bounds check out of
// footprintIsClear and this fails -- `the refusal reads "cannot duplicate
// there: 48,20 is bare ground; a structure needs a floor under it"`.
func TestEditorDuplicateAtTheEdgeSaysOffTheMap(t *testing.T) {
	e := docEditor(t)
	w := e.doc.Size().X

	// A peasant house flush with the east edge: anchor x = w, footprint w-3..w.
	cmd, err := e.doc.PlaceStructure(17, w, 22)
	if err != nil {
		t.Fatal(err)
	}

	if err := e.stack.Do(cmd); err != nil {
		t.Fatal(err)
	}

	placed, ok := e.doc.StructureOn(w-1, 21)
	if !ok {
		t.Fatal("the house at the east edge is not there")
	}

	e.selected = placed.ID
	depth := e.stack.Depth()

	e.duplicateSelected()

	if !strings.Contains(e.message, "off the") || strings.Contains(e.message, "bare ground") {
		t.Fatalf("the refusal reads %q, want it to say the copy is off the map", e.message)
	}

	if e.stack.Depth() != depth {
		t.Fatal("a refused duplicate joined the history")
	}
}

// B2. SAVE AND PLAYTEST ASK THE ENGINE. The reviewer's village copy with a tile
// PNG cut short after its header: the validator (which reads the header) had
// nothing to say, Ctrl+S wrote it, and the game refused it and built Act 1.
// Here the game's loader hands the parser the truncated PNG while the validator
// reads the real one's header -- exactly that split.
//
// Negative control (28 Sep 2026): make Editor.engine answer an Engine that
// accepts everything and this fails -- "Ctrl+S wrote a map the engine refuses"
// -- and P starts a playtest on it.
func TestEditorSaveAndPlaytestAskTheEngine(t *testing.T) {
	e := docEditor(t)
	nav := &playtestNavigator{}
	e.navigator = nav

	grass := "data/strigoi/maps/tiles/placeholder-grass.png"

	whole, err := os.ReadFile(repoFile(grass))
	if err != nil {
		t.Fatal(err)
	}

	e.load = repoLoader(t, map[string][]byte{grass: whole[:40]})

	if problems := e.doc.Validate(e.art); len(problems) != 0 {
		t.Fatalf("the validator was meant to be satisfied (it reads the header): %v", problems[0])
	}

	e.save()

	if !strings.HasPrefix(e.message, "NOT SAVED") {
		t.Fatalf("Ctrl+S said %q", e.message)
	}

	if _, err := os.Stat(e.diskPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Ctrl+S wrote a map the engine refuses")
	}

	e.playtest()

	if !strings.HasPrefix(e.message, "cannot playtest") {
		t.Fatalf("P said %q", e.message)
	}

	if nav.played != "" {
		t.Fatalf("P started a playtest on %s, a map the engine refuses", nav.played)
	}

	// The control: the whole PNG back, and both go through -- to the file the
	// editor opened, not anywhere else.
	e.load = repoLoader(t, nil)
	e.save()

	if _, err := os.Stat(e.diskPath); err != nil {
		t.Fatalf("the intact map was not saved (%q): %v", e.message, err)
	}
}

// B1'S HAZARD. THE SAVE TARGET IS FIXED WHEN THE MAP OPENS. The harness runs
// the game with the repository as its working directory, and the editor used to
// save to its RELATIVE path at the moment of saving -- so a script's Ctrl+S
// under a bare -editor wrote the shipped village. Now the file is resolved to
// an absolute path at open and a save goes there whatever the working directory
// has become.
//
// Negative control (28 Sep 2026): make save write to
// editorDiskPath(e.mapPath) -- the relative path, as before -- and this fails:
// "the save went to the working directory's data/strigoi/maps/village.tmj".
func TestEditorSavesToTheFileItOpenedWhateverTheWorkingDirectory(t *testing.T) {
	e := docEditor(t)

	// A decoy tree where the relative path would land.
	elsewhere := t.TempDir()
	decoy := filepath.Join(elsewhere, filepath.FromSlash(DefaultEditorMap))

	if err := os.MkdirAll(filepath.Dir(decoy), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(decoy, []byte("decoy"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(elsewhere)

	e.save()

	if got, _ := os.ReadFile(decoy); !bytes.Equal(got, []byte("decoy")) {
		t.Fatalf("the save went to the working directory's %s (%q)", DefaultEditorMap, e.message)
	}

	if _, err := os.Stat(e.diskPath); err != nil {
		t.Fatalf("the file the editor opened was not saved (%q): %v", e.message, err)
	}
}

// editorResolve: a map under one of the game's folders is read by the game by
// its path below that folder; an absolute path outside all of them is refused
// (the game could not load it); a relative one keeps the meaning it always had.
func TestEditorResolve(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "data", "strigoi", "maps", "copy.tmj")

	disk, asset, got, err := editorResolve(inside, []string{t.TempDir(), root})
	if err != nil {
		t.Fatal(err)
	}

	if disk != inside || asset != "data/strigoi/maps/copy.tmj" || got != root {
		t.Fatalf("editorResolve(%s) = %s, %s, %s", inside, disk, asset, got)
	}

	outside := filepath.Join(t.TempDir(), "maps", "copy.tmj")
	if _, _, _, err := editorResolve(outside, []string{root}); err == nil {
		t.Fatalf("editorResolve(%s) opened a map outside the game's folders", outside)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	disk, asset, got, err = editorResolve("", nil)
	if err != nil {
		t.Fatal(err)
	}

	if want := filepath.Join(cwd, filepath.FromSlash(DefaultEditorMap)); disk != want || asset != DefaultEditorMap || got != cwd {
		t.Fatalf("the default map resolves to %s, %s, %s; want %s, %s, %s", disk, asset, got, want, DefaultEditorMap, cwd)
	}

	if !filepath.IsAbs(disk) {
		t.Fatalf("the save target %s is not absolute", disk)
	}
}

// playtestNavigator records what P asked for.
type playtestNavigator struct {
	menuNavigator
	played string
}

func (n *playtestNavigator) ToPlaytest(mapPath string) { n.played = mapPath }

package d2gamescreen

import (
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2mapedit"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2mappalette"
)

// These tests cover the World Editor's PURE arithmetic -- the parts that decide
// what a click means and where a row is. The package links zero ebiten and has
// to stay that way (CI is Linux with no display and a test binary that links
// ebiten panics there), so nothing here builds a screen: the screen's decisions
// were written as package-level functions and small methods precisely so they
// could be tested without one.

// repoFile is a path in the repository from this package's directory.
func repoFile(rel string) string {
	return filepath.Join("..", "..", filepath.FromSlash(rel))
}

// The editor is the only screen that writes files, so it needs the path on
// DISK and not the one the loader takes. Both come out of one normalisation:
// forward slashes, cleaned, no leading slash.
//
// Negative control (28 Sep 2026): drop the strings.TrimPrefix from
// editorAssetPath and this fails --
//
//	editorAssetPath("data/strigoi/maps/village.tmj") = "/data/strigoi/maps/village.tmj",
//	want "data/strigoi/maps/village.tmj"
func TestEditorDiskPathIsTheAssetPathOnDisk(t *testing.T) {
	cases := []struct{ in, want string }{
		{"data/strigoi/maps/village.tmj", "data/strigoi/maps/village.tmj"},
		{"/data/strigoi/maps/village.tmj", "data/strigoi/maps/village.tmj"},
		{"data\\strigoi\\maps\\village.tmj", "data/strigoi/maps/village.tmj"},
		{"./data/strigoi/maps/../maps/village.tmj", "data/strigoi/maps/village.tmj"},
		{"data//strigoi///maps/village.tmj", "data/strigoi/maps/village.tmj"},
	}

	for _, c := range cases {
		if got := editorAssetPath(c.in); got != c.want {
			t.Fatalf("editorAssetPath(%q) = %q, want %q", c.in, got, c.want)
		}

		if got := editorDiskPath(c.in); got != filepath.FromSlash(c.want) {
			t.Fatalf("editorDiskPath(%q) = %q, want %q", c.in, got, filepath.FromSlash(c.want))
		}
	}

	// The default map must be openable by the disk path the editor builds from
	// it, or -editor with no argument opens nothing.
	if _, err := os.Stat(repoFile(editorDiskPath(DefaultEditorMap))); err != nil {
		t.Fatalf("the default editor map is not where editorDiskPath says: %v", err)
	}
}

// A structure's anchor is its footprint's bottom corner and the footprint runs
// BACK from it, Max exclusive (d2maptiled/tiled.go:1013-1014), so the anchor
// tile is not itself covered. For the tile a designer clicked to be the
// footprint's front tile, the anchor is one past it -- which is what
// editorStructureAnchor returns.
//
// This is asserted against the SHIPPED VILLAGE rather than against a comment:
// every structure in it is read back, its front tile worked out from its
// footprint, and the anchor recomputed from that front tile.
//
// Negative control (28 Sep 2026): make editorStructureAnchor return clickX,
// clickY and this fails --
//
//	structure 3 (peasant-house) footprint (27,18)-(30,21): clicking its front
//	tile 29,20 gives anchor 29,20, want 30,21
func TestEditorStructureAnchorIsOnePastTheClick(t *testing.T) {
	doc, err := d2mapedit.OpenFile(repoFile(DefaultEditorMap))
	if err != nil {
		t.Fatal(err)
	}

	structures := doc.Structures()
	if len(structures) == 0 {
		t.Fatal("the village has no structures to check the anchor against")
	}

	for _, o := range structures {
		fp := o.Footprint

		// The front tile is the last tile the footprint covers on both axes.
		clickX, clickY := fp.Max.X-1, fp.Max.Y-1

		ax, ay := editorStructureAnchor(clickX, clickY)
		if ax != fp.Max.X || ay != fp.Max.Y {
			t.Fatalf("structure %d (%s) footprint %v: clicking its front tile %d,%d gives anchor %d,%d, want %d,%d",
				o.ID, o.Name, fp, clickX, clickY, ax, ay, fp.Max.X, fp.Max.Y)
		}

		if got := editorFootprintAt(ax, ay, image.Pt(fp.Dx(), fp.Dy())); got != fp {
			t.Fatalf("structure %d: editorFootprintAt(%d,%d) = %v, want the shipped %v", o.ID, ax, ay, got, fp)
		}

		if !image.Pt(clickX, clickY).In(fp) {
			t.Fatalf("structure %d: the clicked tile %d,%d is not inside its own footprint %v", o.ID, clickX, clickY, fp)
		}

		if image.Pt(ax, ay).In(fp) {
			t.Fatalf("structure %d: the anchor %d,%d is inside the footprint %v; Max is exclusive", o.ID, ax, ay, fp)
		}

		// And the document agrees about which tile the structure stands on.
		if s, ok := doc.StructureOn(clickX, clickY); !ok || s.ID != o.ID {
			t.Fatalf("structure %d: the document does not put it on its own front tile %d,%d", o.ID, clickX, clickY)
		}
	}
}

// The palette column's hit test and its drawing read the same four numbers, so
// a click lands on the row the user is looking at. This walks the real village
// palette: every tab row, every visible entry row, the gaps, and the column's
// left edge.
//
// Negative control (28 Sep 2026): change editorPaletteHit's row index from
// rowTop + (y-rowsTop)/edRowH to (y-rowsTop)/edRowH and this fails --
//
//	scrolled to 4, y=168 (row 4) hit tab -1 row 0, want 4
func TestEditorPaletteHitFindsTheTabAndTheRow(t *testing.T) {
	e := villageEditor(t)

	tabsTop, rowsTop := e.paletteTabsTop(), e.paletteRowsTop()
	visible := e.paletteVisibleRows()

	if visible <= 0 {
		t.Fatalf("the palette has room for %d rows", visible)
	}

	// Left of the column is never a palette hit, whatever the y.
	for _, y := range []int{tabsTop, rowsTop, rowsTop + edRowH} {
		if tab, row := editorPaletteHit(editorScreenW-edPaletteW-1, y,
			tabsTop, len(e.tabs), rowsTop, 0, visible, len(e.rows)); tab != -1 || row != -1 {
			t.Fatalf("a point left of the palette at y=%d hit tab %d row %d", y, tab, row)
		}
	}

	// Every tab row, hit at its top, its middle and its last pixel.
	for i := range e.tabs {
		for _, dy := range []int{0, edTabH / 2, edTabH - 1} {
			tab, row := editorPaletteHit(editorScreenW-1, tabsTop+i*edTabH+dy,
				tabsTop, len(e.tabs), rowsTop, 0, visible, len(e.rows))
			if tab != i || row != -1 {
				t.Fatalf("tab %d at +%d hit tab %d row %d", i, dy, tab, row)
			}
		}
	}

	// The band between the tabs and the rows belongs to neither.
	if gap := rowsTop - (tabsTop + len(e.tabs)*edTabH); gap > 0 {
		y := tabsTop + len(e.tabs)*edTabH + gap/2
		if tab, row := editorPaletteHit(editorScreenW-1, y,
			tabsTop, len(e.tabs), rowsTop, 0, visible, len(e.rows)); tab != -1 || row != -1 {
			t.Fatalf("the reason block at y=%d hit tab %d row %d", y, tab, row)
		}
	}

	// Every visible row, unscrolled and then scrolled.
	for _, top := range []int{0, 4} {
		for i := 0; i < visible; i++ {
			y := rowsTop + i*edRowH + edRowH/2

			tab, row := editorPaletteHit(editorScreenW-1, y,
				tabsTop, len(e.tabs), rowsTop, top, visible, 1000)
			if tab != -1 || row != top+i {
				t.Fatalf("scrolled to %d, y=%d (row %d) hit tab %d row %d, want %d",
					top, y, top+i, tab, row, top+i)
			}
		}
	}

	// A row index past the end of the list is not a hit.
	if _, row := editorPaletteHit(editorScreenW-1, rowsTop+edRowH/2,
		tabsTop, len(e.tabs), rowsTop, 0, visible, 0); row != -1 {
		t.Fatalf("an empty list answered row %d", row)
	}

	// And below the last drawn row is not a hit either.
	if tab, row := editorPaletteHit(editorScreenW-1, rowsTop+visible*edRowH,
		tabsTop, len(e.tabs), rowsTop, 0, visible, 1000); tab != -1 || row != -1 {
		t.Fatalf("below the last drawn row hit tab %d row %d", tab, row)
	}
}

// Tab cycles the palette's tabs, wraps at the end, and refills the list each
// time -- the whole point being that the three tabs with nothing in them are
// still reachable, with their reasons, rather than hidden.
//
// Negative control (28 Sep 2026): drop the "% len(e.tabs)" from cycleTab and
// this fails --
//
//	panic: runtime error: index out of range [5] with length 5
//
// and with the wrap put back but rowTop left alone, the second half of the test
// fails: "a tab change left the list scrolled to 3".
func TestEditorTabCyclingWrapsAndRefillsTheList(t *testing.T) {
	e := villageEditor(t)

	if len(e.tabs) < 2 {
		t.Fatalf("the palette has %d tabs", len(e.tabs))
	}

	seen := map[d2mappalette.Category]bool{}

	for i := 0; i < len(e.tabs); i++ {
		if e.tab != i {
			t.Fatalf("after %d cycles the tab is %d, want %d", i, e.tab, i)
		}

		cat := e.tabs[e.tab].Category
		seen[cat] = true

		// The rows on show are this tab's rows and no others.
		for _, r := range e.rows {
			if r.Category != cat {
				t.Fatalf("tab %s is showing a %s entry (%s)", cat, r.Category, r.DisplayName)
			}
		}

		if e.tabs[e.tab].Count != len(e.rows) {
			t.Fatalf("tab %s says it holds %d and the list has %d", cat, e.tabs[e.tab].Count, len(e.rows))
		}

		e.cycleTab()
	}

	if e.tab != 0 {
		t.Fatalf("cycling every tab ended on %d, want back at 0", e.tab)
	}

	if len(seen) != len(e.tabs) {
		t.Fatalf("cycling reached %d of %d tabs", len(seen), len(e.tabs))
	}

	// A tab change puts the list back at the top: the old offset would point
	// past the end of a shorter tab.
	e.rowTop = 3
	e.cycleTab()

	if e.rowTop != 0 {
		t.Fatalf("a tab change left the list scrolled to %d", e.rowTop)
	}
}

// The scroll offset never leaves the list: not past the last screenful, not
// below zero, and always zero when everything fits.
//
// Negative control (28 Sep 2026): drop the "if top > last" clamp from
// editorClampTop and this fails --
//
//	editorClampTop(20, 18, 11) = 20, want 7
func TestEditorClampTopKeepsTheListOnScreen(t *testing.T) {
	cases := []struct {
		top, count, visible, want int
	}{
		{0, 18, 11, 0},
		{20, 18, 11, 7}, // no further than the last screenful
		{7, 18, 11, 7},  // the last screenful is reachable
		{-3, 18, 11, 0}, // and never above the first
		{5, 4, 11, 0},   // a list that fits does not scroll
		{0, 0, 11, 0},   // nor an empty one
		{3, 18, 0, 3},   // a column with no room keeps what it was given
	}

	for _, c := range cases {
		if got := editorClampTop(c.top, c.count, c.visible); got != c.want {
			t.Fatalf("editorClampTop(%d, %d, %d) = %d, want %d", c.top, c.count, c.visible, got, c.want)
		}
	}
}

// villageEditor is an Editor with everything the pure arithmetic needs and
// NOTHING that needs a renderer: the real village's catalog and tab strip, and
// no map engine, no widgets and no asset manager. Building the whole screen
// here would link ebiten and the package must not.
func villageEditor(t *testing.T) *Editor {
	t.Helper()

	root := os.DirFS(repoFile("."))

	catalog := d2mappalette.NewCatalog()
	if err := catalog.ReadMap(root, DefaultEditorMap); err != nil {
		t.Fatal(err)
	}

	if catalog.Len() == 0 {
		t.Fatal("the village palette is empty")
	}

	e := &Editor{catalog: catalog, tabs: catalog.Tabs(), reasonFor: -1}
	e.refreshRows()

	return e
}

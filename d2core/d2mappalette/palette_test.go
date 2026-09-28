package d2mappalette

import (
	"image"
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// TestSquareFootprintForInvertsTheLoaderFormula checks the derivation both ways:
// every width a square footprint produces must come back as that footprint, and
// a width no square footprint produces must be refused rather than rounded.
func TestSquareFootprintForInvertsTheLoaderFormula(t *testing.T) {
	for n := 1; n <= MaxFootprint; n++ {
		fp := image.Pt(n, n)
		w := StructureWidthFor(fp)

		got, ok := SquareFootprintFor(w)
		if !ok || got != fp {
			t.Errorf("a %dx%d footprint wants %d px; SquareFootprintFor(%d) = %v, %v", n, n, w, w, got, ok)
		}
	}

	for _, w := range []int{0, -160, 1, 80, 159, 161, 240, 400, 481, 1000, 2720, 16 * TileWidth * 2} {
		if got, ok := SquareFootprintFor(w); ok {
			t.Errorf("SquareFootprintFor(%d) = %v; no square footprint has that width", w, got)
		}
	}

	// The four installed structures and the three legal Dealu modules, by the
	// width of their real renders.
	for _, c := range []struct {
		w    int
		want int
	}{{160, 1}, {320, 2}, {480, 3}, {960, 6}} {
		if got, ok := SquareFootprintFor(c.w); !ok || got.X != c.want {
			t.Errorf("SquareFootprintFor(%d) = %v, %v; want %dx%d", c.w, got, ok, c.want, c.want)
		}
	}
}

// TestLegalLayersAndPrimary pins the size-only layer rules and the preference
// order, with the sizes of real art in the tree.
func TestLegalLayersAndPrimary(t *testing.T) {
	cases := []struct {
		name    string
		w, h    int
		want    LayerSet
		primary d2maptiled.Layer
	}{
		{"a floor diamond is floor and wall", TileWidth, TileHeight,
			SetFloor | SetWall | SetStructure, d2maptiled.LayerFloor},
		{"the village fence", 160, 130, SetWall | SetStructure, d2maptiled.LayerWall},
		{"the church tower", 160, 330, SetWall | SetStructure, d2maptiled.LayerWall},
		{"the village well render", 160, 256, SetWall | SetStructure, d2maptiled.LayerWall},
		{"one pixel over a wall", 160, MaxWallHeight + 1, SetStructure, d2maptiled.LayerStructure},
		// 768 is a legal structure and far too tall for a wall, so the set is a
		// single bit and the palette must place it as a structure.
		{"the tallest structure", 160, MaxStructureHeight, SetStructure, d2maptiled.LayerStructure},
		{"one pixel over a structure", 160, MaxStructureHeight + 1, 0, d2maptiled.LayerFloor},
		{"the peasant house", 480, 448, SetStructure, d2maptiled.LayerStructure},
		{"the Dealu church", 960, 768, SetStructure, d2maptiled.LayerStructure},
		{"a width no footprint fits", 500, 448, 0, d2maptiled.LayerFloor},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := LegalLayers(c.w, c.h)
			if got != c.want {
				t.Errorf("LegalLayers(%d,%d) = %v (%d), want %v (%d)", c.w, c.h, got, got, c.want, c.want)
			}

			if got.Empty() != (c.want == 0) {
				t.Errorf("Empty() = %v on %v", got.Empty(), got)
			}

			if p := got.Primary(); p != c.primary {
				t.Errorf("Primary() = %v, want %v", p, c.primary)
			}
		})
	}

	// A 160x80 image is legal on all three layers by size, and the preference
	// order must put it on the floor. This is the one place the order is
	// load-bearing: get it wrong and every ground tile in the village is filed
	// as a prop instead of terrain.
	if p := (SetFloor | SetWall | SetStructure).Primary(); p != d2maptiled.LayerFloor {
		t.Errorf("Primary on all three layers = %v, want the floor", p)
	}

	if s := (SetFloor | SetStructure).String(); s != "floor+structure" {
		t.Errorf("LayerSet.String() = %q", s)
	}

	if s := LayerSet(0).String(); s != "no layer" {
		t.Errorf("empty LayerSet.String() = %q", s)
	}
}

// TestHumanNameReadsRealPaths runs the naming rule over the real paths in the
// tree. There is no title on any asset anywhere, so this derivation is the only
// thing between the user and a filename.
func TestHumanNameReadsRealPaths(t *testing.T) {
	cases := map[string]string{
		"maps/tiles/placeholder-grass.png":         "Grass",
		"maps/tiles/placeholder-church-tower.png":  "Church tower",
		"maps/tiles/placeholder-ditch.png":         "Ditch",
		"maps/tiles/placeholder-grass-dark.png":    "Grass dark",
		"structures/peasant-house/intact.png":      "Peasant house",
		"structures/burned-house/cold-ruin.png":    "Burned house (cold ruin)",
		"structures/village-well/intact.png":       "Village well",
		"structures/village-hearth/unlit.png":      "Village hearth (unlit)",
		"renders/structures/church-intact-960.png": "Church intact 960",
	}

	for in, want := range cases {
		if got := HumanName(in); got != want {
			t.Errorf("HumanName(%q) = %q, want %q", in, got, want)
		}
	}

	// A backslash path is normalised rather than producing one long "name":
	// the loader refuses backslashes outright (tiled.go:773), and a palette that
	// mangled the name would hide which file was at fault.
	if got := HumanName(`structures\village-well\intact.png`); got != "Village well" {
		t.Errorf("HumanName on a backslash path = %q", got)
	}
}

// TestCategoryOfFollowsItsWrittenRule pins the tab grouping, including the part
// that matters most: a floor tile is Terrain, so it lands in the tab that is not
// in v0, and a multi-tile structure is a Building whatever it is called.
func TestCategoryOfFollowsItsWrittenRule(t *testing.T) {
	one := image.Pt(1, 1)

	cases := []struct {
		layer d2maptiled.Layer
		fp    image.Point
		id    string
		name  string
		want  Category
	}{
		{d2maptiled.LayerFloor, one, "village-placeholder#0", "Grass", CategoryTerrain},
		{d2maptiled.LayerFloor, one, "village-placeholder#5", "Ditch", CategoryTerrain},
		{d2maptiled.LayerWall, one, "village-placeholder#7", "House", CategoryBuildings},
		{d2maptiled.LayerWall, one, "village-placeholder#8", "Smithy", CategoryBuildings},
		{d2maptiled.LayerWall, one, "village-placeholder#10", "Church tower", CategoryBuildings},
		// Boundary kit is construction, not dressing.
		{d2maptiled.LayerWall, one, "village-placeholder#6", "Fence", CategoryBuildings},
		{d2maptiled.LayerWall, one, "village-placeholder#12", "Tree", CategoryProps},
		{d2maptiled.LayerWall, one, "village-placeholder#13", "Grave", CategoryProps},
		{d2maptiled.LayerWall, one, "village-well/intact", "Village well", CategoryProps},
		{d2maptiled.LayerStructure, image.Pt(3, 3), "peasant-house/intact", "Peasant house", CategoryBuildings},
		// A 3x1 wall section is a Building on the footprint rule alone, before
		// the keyword table is even consulted.
		{d2maptiled.LayerStructure, image.Pt(3, 1), "dealu/wall/intact", "Dealu monastery wall", CategoryBuildings},
		// The well that earned the prop-words-first ordering: its name carries
		// "monastery", and it is still a well.
		{d2maptiled.LayerStructure, image.Pt(2, 2), "dealu-monastery-v1/well/intact", "Dealu monastery well", CategoryProps},
		// Nothing either table knows, covering nine tiles: a building by default.
		{d2maptiled.LayerStructure, image.Pt(3, 3), "some/thing", "Unnamed block", CategoryBuildings},
		{d2maptiled.LayerWall, one, "some/thing", "Unnamed post", CategoryProps},
	}

	for _, c := range cases {
		if got := CategoryOf(c.layer, c.fp, c.id, c.name); got != c.want {
			t.Errorf("CategoryOf(%v, %v, %q, %q) = %q, want %q", c.layer, c.fp, c.id, c.name, got, c.want)
		}
	}
}

// TestUnavailableTabsCarryAReason is the editor's contract: it shows all five
// tabs, and the three it cannot fill must each say why in words a user can read,
// with a citation a reader can check. An empty reason would be the editor
// silently pretending a feature is coming.
func TestUnavailableTabsCarryAReason(t *testing.T) {
	tabs := Tabs()
	if len(tabs) != 5 {
		t.Fatalf("%d tabs, want 5", len(tabs))
	}

	unavailable := 0

	for _, tab := range tabs {
		if tab.Title == "" {
			t.Errorf("tab %q has no title", tab.Category)
		}

		if tab.Available {
			if tab.Why != "" {
				t.Errorf("tab %q is available and still carries a reason: %q", tab.Category, tab.Why)
			}

			continue
		}

		unavailable++

		switch {
		case len(tab.Why) < 120:
			t.Errorf("tab %q has a %d-character reason; that is not an explanation: %q", tab.Category, len(tab.Why), tab.Why)
		case !strings.Contains(tab.Why, ".go:"):
			t.Errorf("tab %q gives a reason with no file:line to check it against: %q", tab.Category, tab.Why)
		}
	}

	if unavailable != 3 {
		t.Errorf("%d unavailable tabs, want 3 (creatures, people, terrain)", unavailable)
	}

	// The Creatures reason must name BOTH walls, because the loader's object
	// switch is the easy one and the spawner is the one that makes an authored
	// creature useless.
	for _, want := range []string{"player_start", "gameSpawner.Spawn", "harness", "inert"} {
		if !strings.Contains(WhyNoCreatures, want) {
			t.Errorf("WhyNoCreatures does not mention %q: %s", want, WhyNoCreatures)
		}
	}

	for _, want := range []string{"monstat", "warriv1", "kashya", "charsi", "akara", "dialogue.json"} {
		if !strings.Contains(WhyNoPeople, want) {
			t.Errorf("WhyNoPeople does not mention %q: %s", want, WhyNoPeople)
		}
	}

	// The Terrain reason has to say what painting WOULD be, not just that it is
	// missing.
	for _, want := range []string{"floor layer", "x+y*width"} {
		if !strings.Contains(WhyNoTerrain, want) {
			t.Errorf("WhyNoTerrain does not say what painting would be (%q missing): %s", want, WhyNoTerrain)
		}
	}
}

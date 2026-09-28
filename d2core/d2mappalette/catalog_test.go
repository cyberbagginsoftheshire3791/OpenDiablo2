package d2mappalette

import (
	"errors"
	"image"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// strigoiFS is data/strigoi, the way the game reads it: the map lives at
// maps/village.tmj and its "../structures/..." tile paths resolve to
// structures/... by path.Join, exactly as tiled.go:777 resolves them.
func strigoiFS(t *testing.T) string {
	t.Helper()

	root := filepath.Join("..", "..", "data", "strigoi")
	if _, err := os.Stat(filepath.Join(root, "maps", "village.tmj")); err != nil {
		t.Fatalf("the shipped village is not where it should be: %v", err)
	}

	return root
}

// TestShippedVillageCatalog reads the real data/strigoi/maps/village.tmj and its
// real PNGs. It is the test that would notice the map being edited underneath
// this package.
func TestShippedVillageCatalog(t *testing.T) {
	root := strigoiFS(t)
	fsys := os.DirFS(root)

	c := NewCatalog()
	if err := c.ReadMap(fsys, "maps/village.tmj"); err != nil {
		t.Fatalf("reading the shipped village: %v", err)
	}

	// The village's one embedded tileset holds 18 tiles, and every one of them
	// becomes an entry -- including the six floors, which are catalogued for a
	// Terrain tab that has no tool yet.
	if c.Len() != 18 {
		t.Errorf("%d entries, want the village tileset's 18", c.Len())
	}

	// The note the loader throws away. If this stops saying "PLACEHOLDER" the
	// tiles stop being Preview, which is a decision somebody should make on
	// purpose rather than discover.
	if !strings.Contains(c.MapNote(), "PLACEHOLDER") {
		t.Errorf("the village's note no longer says its art is placeholder: %q", c.MapNote())
	}

	// Every tile in the map's own tiles/ folder is a placeholder-*.png, and the
	// note agrees, so every one of those is Preview. The four art-repo renders
	// it points at (../structures/...) have no placeholder in their path, and
	// with no art manifest to hand they fall to the map note -- also Preview,
	// with the note quoted, which is the honest answer when nothing better
	// exists.
	for _, e := range c.Entries() {
		if e.Status != StatusPreview {
			t.Errorf("%s (%s) is %q; every tile of this map has a placeholder signal", e.ID, e.ImagePath, e.Status)
		}

		if e.StatusWhy == "" || e.DisplayName == "" || e.ImagePath == "" {
			t.Errorf("%s is missing a name, a path or a reason: %+v", e.ID, e)
		}

		if e.PixelWidth <= 0 || e.PixelHeight <= 0 {
			t.Errorf("%s was not measured: %dx%d", e.ID, e.PixelWidth, e.PixelHeight)
		}
	}

	// Every one of the 18 is placeable: the shipped village loads, so nothing in
	// its tileset may be greyed out. This is the strongest single assertion in
	// the file -- it ties the palette's verdict to a map the engine demonstrably
	// accepts (d2maptiled/village_test.go).
	if n := len(c.Refused()); n != 0 {
		for _, e := range c.Refused() {
			t.Errorf("the palette refuses %s (%s), but the shipped village loads it: %v",
				e.ID, e.ImagePath, e.Problems)
		}

		t.Fatalf("%d refused entries in a map the engine accepts", n)
	}

	// Spot-checks on real tiles, by the ids the loader itself uses.
	want := map[string]struct {
		name        string
		layer       d2maptiled.Layer
		fp          image.Point
		w, h        int
		blocked     bool
		blocksSight bool
		cat         Category
	}{
		"village-placeholder#0":  {"Grass", d2maptiled.LayerFloor, image.Pt(1, 1), 160, 80, false, false, CategoryTerrain},
		"village-placeholder#5":  {"Ditch", d2maptiled.LayerFloor, image.Pt(1, 1), 160, 80, true, false, CategoryTerrain},
		"village-placeholder#6":  {"Fence", d2maptiled.LayerWall, image.Pt(1, 1), 160, 130, true, false, CategoryBuildings},
		"village-placeholder#9":  {"Church", d2maptiled.LayerWall, image.Pt(1, 1), 160, 240, true, true, CategoryBuildings},
		"village-placeholder#12": {"Tree", d2maptiled.LayerWall, image.Pt(1, 1), 160, 230, true, true, CategoryProps},
		"village-placeholder#13": {"Grave", d2maptiled.LayerWall, image.Pt(1, 1), 160, 130, false, false, CategoryProps},
		"village-placeholder#14": {"Village well", d2maptiled.LayerWall, image.Pt(1, 1), 160, 256, true, false, CategoryProps},
		"village-placeholder#16": {"Peasant house", d2maptiled.LayerStructure, image.Pt(3, 3), 480, 448, true, true, CategoryBuildings},
		"village-placeholder#17": {"Burned house (cold ruin)", d2maptiled.LayerStructure, image.Pt(3, 3), 480, 448, true, true, CategoryBuildings},
	}

	for id, w := range want {
		e, ok := c.ByID(id)
		if !ok {
			t.Errorf("no entry %s", id)

			continue
		}

		if e.DisplayName != w.name {
			t.Errorf("%s is called %q, want %q", id, e.DisplayName, w.name)
		}

		if e.Layer != w.layer {
			t.Errorf("%s is on %v, want %v", id, e.Layer, w.layer)
		}

		if e.Footprint != w.fp {
			t.Errorf("%s footprint %v, want %v", id, e.Footprint, w.fp)
		}

		if e.PixelWidth != w.w || e.PixelHeight != w.h {
			t.Errorf("%s measures %dx%d, want %dx%d", id, e.PixelWidth, e.PixelHeight, w.w, w.h)
		}

		if e.Blocked != w.blocked || e.BlocksSight != w.blocksSight {
			t.Errorf("%s blocked=%v sight=%v, want %v and %v", id, e.Blocked, e.BlocksSight, w.blocked, w.blocksSight)
		}

		if e.Category != w.cat {
			t.Errorf("%s is in %q, want %q", id, e.Category, w.cat)
		}
	}

	// The two structure tiles are the only ones the engine will take as tile
	// objects, and no floor or wall tile may be offered as one: kind() refuses
	// both mix-ups (tiled.go:689-694).
	for _, e := range c.Entries() {
		if e.Layer == d2maptiled.LayerStructure && e.Layers != SetStructure {
			t.Errorf("%s is a structure and its layer set is %v; a footprinted tile is a tile object and nothing else", e.ID, e.Layers)
		}

		if e.Layer != d2maptiled.LayerStructure && e.Layers.Has(d2maptiled.LayerStructure) {
			t.Errorf("%s has no footprint and is offered as a structure (%v)", e.ID, e.Layers)
		}
	}

	// Tab counts: 18 tiles split six ground, and the rest between buildings and
	// props. Creatures and People have nothing and say why.
	counts := map[Category]int{}
	for _, tab := range c.Tabs() {
		counts[tab.Category] = tab.Count

		if !tab.Available && tab.Why == "" {
			t.Errorf("the %s tab is unavailable and gives no reason", tab.Category)
		}
	}

	if counts[CategoryTerrain] != 6 {
		t.Errorf("%d terrain entries, want the village's six floor tiles", counts[CategoryTerrain])
	}

	if counts[CategoryCreatures] != 0 || counts[CategoryPeople] != 0 {
		t.Errorf("creatures %d, people %d; neither can be placed at all",
			counts[CategoryCreatures], counts[CategoryPeople])
	}

	if total := counts[CategoryBuildings] + counts[CategoryProps] + counts[CategoryTerrain]; total != 18 {
		t.Errorf("the tabs hold %d of 18 entries; something is in no tab", total)
	}
}

// TestKindsUsedMatchesTheLoadersOwnCount is a differential assertion on the
// MaxKinds budget: the palette counts the distinct (gid, layer) kinds the village
// spends, and the real loader builds exactly one Kind per such pair
// (tiled.go:653). If the two ever disagree, the palette's "240 left" is wrong and
// a user could be offered a tile the 256-kind cap will refuse.
func TestKindsUsedMatchesTheLoadersOwnCount(t *testing.T) {
	root := strigoiFS(t)
	fsys := os.DirFS(root)

	c := NewCatalog()
	if err := c.ReadMap(fsys, "maps/village.tmj"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, "maps", "village.tmj"))
	if err != nil {
		t.Fatal(err)
	}

	m, err := d2maptiled.Parse(data, "/maps", func(p string) ([]byte, error) {
		rel := strings.TrimPrefix(path.Clean(p), "/")

		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, errors.New("no such file: " + p)
		}

		return b, nil
	})
	if err != nil {
		t.Fatalf("the shipped village is refused by the loader: %v", err)
	}

	if c.KindsUsed() != len(m.Kinds) {
		t.Errorf("the palette counts %d kinds used, the loader built %d", c.KindsUsed(), len(m.Kinds))
	}

	if c.KindsFree() != MaxKinds-len(m.Kinds) {
		t.Errorf("KindsFree() = %d, want %d", c.KindsFree(), MaxKinds-len(m.Kinds))
	}

	if c.KindsUsed() == 0 {
		t.Error("zero kinds counted; the counter is measuring nothing")
	}
}

// TestReadStructureDirDerivesTheFootprintsTheReadmeDeclares reads the real
// data/strigoi/structures, where NOTHING declares a footprint, and checks the
// derivation against the footprints data/strigoi/structures/README.md's table
// declares by hand. Two independent records of the same fact: the README was
// written by a person and the derivation is the loader's formula inverted, so
// agreement is worth something.
func TestReadStructureDirDerivesTheFootprintsTheReadmeDeclares(t *testing.T) {
	root := strigoiFS(t)
	fsys := os.DirFS(root)

	c := NewCatalog()
	if err := c.ReadStructureDir(fsys, "structures", nil); err != nil {
		t.Fatalf("reading the installed structures: %v", err)
	}

	// README.md's table: peasant-house 3x3 structure, burned-house 3x3
	// structure, village-well 1x1 wall tile, village-hearth 1x1 wall tile.
	want := map[string]struct {
		name  string
		layer d2maptiled.Layer
		fp    image.Point
		w, h  int
	}{
		"peasant-house/intact":   {"Peasant house", d2maptiled.LayerStructure, image.Pt(3, 3), 480, 448},
		"burned-house/cold-ruin": {"Burned house (cold ruin)", d2maptiled.LayerStructure, image.Pt(3, 3), 480, 448},
		"village-well/intact":    {"Village well", d2maptiled.LayerWall, image.Pt(1, 1), 160, 256},
		"village-hearth/unlit":   {"Village hearth (unlit)", d2maptiled.LayerWall, image.Pt(1, 1), 160, 128},
	}

	if c.Len() != len(want) {
		t.Errorf("%d entries, want %d; data/strigoi/structures has changed", c.Len(), len(want))
	}

	for id, w := range want {
		e, ok := c.ByID(id)
		if !ok {
			t.Errorf("no entry %q; got %v", id, ids(c))

			continue
		}

		if e.DisplayName != w.name {
			t.Errorf("%s is called %q, want %q", id, e.DisplayName, w.name)
		}

		if e.Layer != w.layer {
			t.Errorf("%s is on %v, want %v (README.md's \"Used as\" column)", id, e.Layer, w.layer)
		}

		if e.PixelWidth != w.w || e.PixelHeight != w.h {
			t.Errorf("%s measures %dx%d, want %dx%d", id, e.PixelWidth, e.PixelHeight, w.w, w.h)
		}

		if e.Footprint != w.fp {
			t.Errorf("%s footprint %v, want %v (README.md's Footprint column)", id, e.Footprint, w.fp)
		}

		if !e.Placeable() {
			t.Errorf("%s is refused: %v", id, e.Problems)
		}

		// With no art manifest passed, nothing says either way -- and the
		// package must say Unknown rather than assume the renders are finished.
		if e.Status != StatusUnknown {
			t.Errorf("%s is %q with no art manifest to hand; want unknown (why: %s)", id, e.Status, e.StatusWhy)
		}

		// A derived footprint must SAY it was derived. This is the line between
		// a measurement and a guess presented as one.
		if e.Layer == d2maptiled.LayerStructure && !strings.Contains(e.FootprintWhy, "derived") {
			t.Errorf("%s footprint %v does not say where it came from: %q", id, e.Footprint, e.FootprintWhy)
		}
	}
}

// TestReadStructureDirPicksUpAnArtManifest checks the one art-repo key this
// package reads. The art repository is not part of the engine, so the manifests
// are synthesised here with their real contents.
func TestReadStructureDirPicksUpAnArtManifest(t *testing.T) {
	art := fstest.MapFS{
		"structures/peasant-house.json": &fstest.MapFile{Data: []byte(
			`{"id":"peasant-house","version":1,"footprint_tiles":[3,3],"integration":"art-ready"}`)},
		"structures/village-well.json": &fstest.MapFile{Data: []byte(
			`{"id":"village-well","version":1,"integration":"art-ready"}`)},
		// A manifest with an integration value that says more than art-ready.
		"structures/barn.json": &fstest.MapFile{Data: []byte(
			`{"id":"barn","integration":"art-ready; not installed"}`)},
		// Junk must be skipped, not fatal: the art repo is optional.
		"structures/broken.json": &fstest.MapFile{Data: []byte(`{ not json`)},
	}

	game := fstest.MapFS{
		"structures/peasant-house/intact.png": pngFile(t, 480, 448),
		"structures/village-well/intact.png":  pngFile(t, 160, 256),
		"structures/barn/intact.png":          pngFile(t, 480, 448),
		"structures/mystery/intact.png":       pngFile(t, 160, 200),
	}

	c := NewCatalog()
	if err := c.ReadStructureDir(game, "structures", art); err != nil {
		t.Fatal(err)
	}

	want := map[string]Status{
		"peasant-house/intact": StatusApproved,
		"village-well/intact":  StatusApproved,
		"barn/intact":          StatusPreview,
		"mystery/intact":       StatusUnknown,
	}

	for id, status := range want {
		e, ok := c.ByID(id)
		if !ok {
			t.Errorf("no entry %q; got %v", id, ids(c))

			continue
		}

		if e.Status != status {
			t.Errorf("%s is %q, want %q (why: %s)", id, e.Status, status, e.StatusWhy)
		}
	}
}

// TestRefusedArtIsGreyedOutWithTheLoadersWords is the palette's whole purpose:
// bad art must be catalogued AND refused, with a reason, rather than dropped
// silently or offered.
func TestRefusedArtIsGreyedOutWithTheLoadersWords(t *testing.T) {
	game := fstest.MapFS{
		// A legal 3x3 structure, to prove the good case is not refused too.
		"structures/good-house/intact.png": pngFile(t, 480, 448),
		// A width no square footprint fits.
		"structures/odd-width/intact.png": pngFile(t, 500, 448),
		// 160 wide but taller than a structure may be, so no layer takes it.
		"structures/too-tall/intact.png": pngFile(t, 160, 900),
	}

	c := NewCatalog()
	if err := c.ReadStructureDir(game, "structures", nil); err != nil {
		t.Fatal(err)
	}

	good, ok := c.ByID("good-house/intact")
	if !ok || !good.Placeable() {
		t.Fatalf("the legal house is refused: %+v", good)
	}

	for _, id := range []string{"odd-width/intact", "too-tall/intact"} {
		e, ok := c.ByID(id)
		if !ok {
			t.Fatalf("no entry %q", id)
		}

		if e.Placeable() {
			t.Errorf("%s (%dx%d) is offered as placeable", id, e.PixelWidth, e.PixelHeight)
		}

		if len(e.Problems) == 0 {
			t.Errorf("%s is refused and gives no reason", id)
		}

		if !strings.Contains(e.Why(), "cannot be placed") {
			t.Errorf("%s: Why() does not say it cannot be placed: %s", id, e.Why())
		}

		if !e.Layers.Empty() {
			t.Errorf("%s fits no layer but its layer set is %v", id, e.Layers)
		}
	}

	// The odd width gets the useful message, not "it has no footprint".
	odd, _ := c.ByID("odd-width/intact")
	if !strings.Contains(odd.Problems[0], "no square footprint fits") {
		t.Errorf("a 500-wide structure's reason is unhelpful: %q", odd.Problems[0])
	}

	// The too-tall one is explained as a wall, because 160 wide is what a wall
	// is, and that is the advice the artist needs.
	tall, _ := c.ByID("too-tall/intact")
	if !strings.Contains(strings.Join(tall.Problems, " "), "wall art is 160x900") {
		t.Errorf("a 160x900 image's reason does not name the wall rule: %v", tall.Problems)
	}
}

// TestReadMapRefusesAnExternalTileset: the loader refuses the whole map for an
// external tileset (tiled.go:515), so the palette must not quietly offer an empty
// list as though the map were fine.
func TestReadMapRefusesAnExternalTileset(t *testing.T) {
	fsys := fstest.MapFS{
		"maps/m.tmj": &fstest.MapFile{Data: []byte(
			`{"tilesets":[{"firstgid":1,"name":"ext","source":"village.tsx"}],"layers":[]}`)},
	}

	err := NewCatalog().ReadMap(fsys, "maps/m.tmj")
	if err == nil {
		t.Fatal("an external tileset was accepted")
	}

	if !strings.Contains(err.Error(), "external") {
		t.Errorf("the error does not say the tileset is external: %v", err)
	}
}

// TestReadMapRejectsATileItCannotMeasure: a tileset that points at art that is
// not there is an error, not an entry with a zero size. A palette that invented
// 0x0 would then say the tile is refused for the wrong reason.
func TestReadMapRejectsATileItCannotMeasure(t *testing.T) {
	fsys := fstest.MapFS{
		"maps/m.tmj": &fstest.MapFile{Data: []byte(
			`{"tilesets":[{"firstgid":1,"name":"t","tiles":[{"id":0,"image":"tiles/gone.png"}]}],"layers":[]}`)},
	}

	err := NewCatalog().ReadMap(fsys, "maps/m.tmj")
	if err == nil {
		t.Fatal("a tile whose art is missing was accepted")
	}
}

func ids(c *Catalog) []string {
	out := make([]string, 0, c.Len())
	for _, e := range c.Entries() {
		out = append(out, e.ID)
	}

	return out
}

func pngFile(t *testing.T, w, h int) *fstest.MapFile {
	t.Helper()

	return &fstest.MapFile{Data: synthPNG(t, w, h)}
}

// TestKindsCountATileUsedOnTwoLayersTwice closes a coverage gap a negative
// control found.
//
// The palette's kind counter keys on (gid, LAYER), because the loader's kind()
// cache does (tiled.go:653) -- one tile image used on two layers is two Kinds. The
// shipped village happens to use no gid on more than one layer, so
// TestKindsUsedMatchesTheLoadersOwnCount could not tell the right key from a
// gid-only one: dropping the layer from the key left it green. This map uses one
// grass tile on the floor AND on the walls, and both counts must say two.
func TestKindsCountATileUsedOnTwoLayersTwice(t *testing.T) {
	const tmj = `{
      "type":"map","orientation":"isometric","infinite":false,
      "width":4,"height":3,"tilewidth":160,"tileheight":80,
      "layers":[
        {"type":"tilelayer","name":"floor","width":4,"height":3,"data":[1,1,1,1,1,1,1,1,1,1,1,1]},
        {"type":"tilelayer","name":"walls","width":4,"height":3,"data":[1,0,0,0,0,0,0,0,0,0,0,0]},
        {"type":"objectgroup","name":"objects","objects":[
          {"id":1,"type":"player_start","x":200,"y":200}]}
      ],
      "tilesets":[{"firstgid":1,"name":"t","tilewidth":160,"tileheight":80,"tilecount":1,
        "tiles":[{"id":0,"image":"tiles/grass.png"}]}]
    }`

	grass := synthPNG(t, TileWidth, TileHeight)

	fsys := fstest.MapFS{
		"maps/m.tmj":           &fstest.MapFile{Data: []byte(tmj)},
		"maps/tiles/grass.png": &fstest.MapFile{Data: grass},
	}

	c := NewCatalog()
	if err := c.ReadMap(fsys, "maps/m.tmj"); err != nil {
		t.Fatal(err)
	}

	m, err := d2maptiled.Parse([]byte(tmj), "/maps", func(p string) ([]byte, error) {
		if p == "/maps/tiles/grass.png" {
			return grass, nil
		}

		return nil, errors.New("no such file: " + p)
	})
	if err != nil {
		t.Fatalf("the two-layer map is refused by the loader: %v", err)
	}

	if len(m.Kinds) != 2 {
		t.Fatalf("the loader built %d kinds from one tile on two layers, want 2", len(m.Kinds))
	}

	if c.KindsUsed() != 2 {
		t.Errorf("the palette counts %d kinds, the loader built %d; one image on two layers is two kinds",
			c.KindsUsed(), len(m.Kinds))
	}

	// One TILE, so one palette entry: the entry is the art, and the kind budget
	// is a separate count.
	if c.Len() != 1 {
		t.Errorf("%d entries from a one-tile tileset, want 1", c.Len())
	}
}

// TestTheSameArtIsJudgedByWhatItsSourceClaims pins the tri-state a negative
// control and then a red test both had to teach this package.
//
// One 160x80 PNG. As a tileset tile with no footprint_w/footprint_h it may go on
// the floor or the walls and NOT on the objects layer, because the tile has
// answered the footprint question and kind() refuses it as a tile object
// (tiled.go:690). As a bare PNG on disk nothing has answered, so the same art may
// also be a 1x1 structure -- (1+1)*80 is 160, which is the only square footprint
// its width allows. The art is identical; the claim about it is not.
func TestTheSameArtIsJudgedByWhatItsSourceClaims(t *testing.T) {
	art := synthPNG(t, TileWidth, TileHeight)

	tileset := fstest.MapFS{
		"maps/m.tmj": &fstest.MapFile{Data: []byte(`{
          "type":"map","orientation":"isometric","width":1,"height":1,"tilewidth":160,"tileheight":80,
          "layers":[],
          "tilesets":[{"firstgid":1,"name":"t","tiles":[{"id":0,"image":"tiles/a.png"}]}]}`)},
		"maps/tiles/a.png": &fstest.MapFile{Data: art},
	}

	disk := fstest.MapFS{"structures/a/intact.png": &fstest.MapFile{Data: art}}

	fromTileset := NewCatalog()
	if err := fromTileset.ReadMap(tileset, "maps/m.tmj"); err != nil {
		t.Fatal(err)
	}

	fromDisk := NewCatalog()
	if err := fromDisk.ReadStructureDir(disk, "structures", nil); err != nil {
		t.Fatal(err)
	}

	tileEntry, ok := fromTileset.ByID("t#0")
	if !ok {
		t.Fatalf("no tileset entry; got %v", ids(fromTileset))
	}

	diskEntry, ok := fromDisk.ByID("a/intact")
	if !ok {
		t.Fatalf("no disk entry; got %v", ids(fromDisk))
	}

	if tileEntry.Layers != SetFloor|SetWall {
		t.Errorf("a tileset tile with no footprint is offered on %v; it may only go on a tile layer (tiled.go:692)",
			tileEntry.Layers)
	}

	if diskEntry.Layers != SetFloor|SetWall|SetStructure {
		t.Errorf("a bare 160x80 PNG on disk is offered on %v; nothing has ruled out a 1x1 structure",
			diskEntry.Layers)
	}

	// Both still default to the floor, and both are placeable.
	for _, e := range []Entry{tileEntry, diskEntry} {
		if e.Layer != d2maptiled.LayerFloor || !e.Placeable() {
			t.Errorf("%s: layer %v placeable %v, want the floor and placeable (%v)",
				e.ID, e.Layer, e.Placeable(), e.Problems)
		}
	}
}

// TestAStructureThatSaysBlockedFalseIsRefused. The loader refuses such a map
// outright (tiled.go:887) even though it would force Blocked true anyway
// (tiled.go:890), and that ordering is a trap: an earlier draft of this package
// read the FORCED value and its check could never fire. Both halves are asserted
// here, and the loader is asked the same question.
func TestAStructureThatSaysBlockedFalseIsRefused(t *testing.T) {
	const tmj = `{
      "type":"map","orientation":"isometric","infinite":false,
      "width":4,"height":4,"tilewidth":160,"tileheight":80,
      "layers":[
        {"type":"tilelayer","name":"floor","width":4,"height":4,
         "data":[1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1]},
        {"type":"objectgroup","name":"objects","objects":[
          {"id":1,"type":"player_start","x":280,"y":280},
          {"id":2,"type":"structure","gid":2,"x":240,"y":240}]}
      ],
      "tilesets":[{"firstgid":1,"name":"t","tilewidth":160,"tileheight":80,"tilecount":2,
        "tiles":[
          {"id":0,"image":"tiles/grass.png"},
          {"id":1,"image":"tiles/house.png","properties":[
            {"name":"footprint_w","type":"int","value":3},
            {"name":"footprint_h","type":"int","value":3},
            {"name":"blocked","type":"bool","value":false}]}]}]}`

	grass, house := synthPNG(t, 160, 80), synthPNG(t, 480, 448)

	if _, err := d2maptiled.Parse([]byte(tmj), "/maps", func(p string) ([]byte, error) {
		switch p {
		case "/maps/tiles/grass.png":
			return grass, nil
		case "/maps/tiles/house.png":
			return house, nil
		}

		return nil, errors.New("no such file: " + p)
	}); err == nil {
		t.Error("the loader accepted a structure with blocked=false; tiled.go:887 says it refuses one")
	}

	c := NewCatalog()
	if err := c.ReadMap(fstest.MapFS{
		"maps/m.tmj":           &fstest.MapFile{Data: []byte(tmj)},
		"maps/tiles/grass.png": &fstest.MapFile{Data: grass},
		"maps/tiles/house.png": &fstest.MapFile{Data: house},
	}, "maps/m.tmj"); err != nil {
		t.Fatal(err)
	}

	e, ok := c.ByID("t#1")
	if !ok {
		t.Fatalf("no entry for the house; got %v", ids(c))
	}

	if e.Placeable() {
		t.Error("the palette offers a structure the loader refuses for blocked=false")
	}

	if !strings.Contains(strings.Join(e.Problems, " | "), "cannot be blocked=false") {
		t.Errorf("the reason does not name the refusal: %v", e.Problems)
	}

	// And the value the palette REPORTS is the one the engine will hold: forced
	// true, because the footprint is solid whatever the property says.
	if !e.Blocked {
		t.Error("the palette reports Blocked false; the engine forces it true (tiled.go:890)")
	}
}

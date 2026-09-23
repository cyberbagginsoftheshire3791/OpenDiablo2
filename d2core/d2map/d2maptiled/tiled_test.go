package d2maptiled

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// ---- fixtures ---------------------------------------------------------------

// pngOf returns a w x h PNG filled with c.
func pngOf(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = c.R, c.G, c.B, c.A
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

var (
	grass = color.RGBA{R: 40, G: 120, B: 40, A: 255}
	mud   = color.RGBA{R: 110, G: 80, B: 40, A: 255}
	house = color.RGBA{R: 90, G: 60, B: 30, A: 255}
)

// fixture is a small valid map as a mutable tree, so each negative case can
// break exactly one thing.
type fixture struct {
	m     map[string]any
	files map[string][]byte
	reads []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	// 4x3 map. gid 1 grass, 2 mud (blocked, sight explicitly false), 3 house
	// (blocked, wall art 160x200).
	floor := []any{
		1, 1, 1, 1,
		1, 2, 1, 1,
		1, 1, 1, 0,
	}
	walls := []any{
		0, 0, 3, 0,
		0, 0, 0, 0,
		0, 0, 0, 0,
	}

	f := &fixture{files: map[string][]byte{
		"/maps/tiles/grass.png": pngOf(t, 160, 80, grass),
		"/maps/tiles/mud.png":   pngOf(t, 160, 80, mud),
		"/maps/tiles/house.png": pngOf(t, 160, 200, house),
	}}

	f.m = map[string]any{
		"type": "map", "orientation": "isometric",
		"width": 4, "height": 3, "tilewidth": 160, "tileheight": 80, "infinite": false,
		"layers": []any{
			map[string]any{"type": "tilelayer", "name": "floor", "width": 4, "height": 3, "data": floor},
			map[string]any{"type": "tilelayer", "name": "walls", "width": 4, "height": 3, "data": walls},
			map[string]any{"type": "objectgroup", "name": "objects", "objects": []any{
				map[string]any{"id": 1, "type": "player_start", "x": 80.0 * 0.5, "y": 80.0 * 2.5},
				map[string]any{"id": 2, "type": "npc", "x": 80.0 * 3.2, "y": 80.0 * 1.6, "properties": []any{
					map[string]any{"name": "monstat", "type": "string", "value": "warriv1"},
				}},
			}},
		},
		"tilesets": []any{
			map[string]any{"firstgid": 1, "name": "village", "tilewidth": 160, "tileheight": 200, "tilecount": 3, "tiles": []any{
				map[string]any{"id": 0, "image": "tiles/grass.png"},
				map[string]any{"id": 1, "image": "tiles/mud.png", "properties": []any{
					map[string]any{"name": "blocked", "type": "bool", "value": true},
					map[string]any{"name": "blocks_sight", "type": "bool", "value": false},
				}},
				map[string]any{"id": 2, "image": "tiles/house.png", "properties": []any{
					map[string]any{"name": "blocked", "type": "bool", "value": true},
				}},
			}},
		},
	}

	return f
}

func (f *fixture) layer(i int) map[string]any {
	return f.m["layers"].([]any)[i].(map[string]any)
}

func (f *fixture) objects() []any {
	return f.layer(2)["objects"].([]any)
}

func (f *fixture) tile(i int) map[string]any {
	return f.m["tilesets"].([]any)[0].(map[string]any)["tiles"].([]any)[i].(map[string]any)
}

func (f *fixture) parse(t *testing.T) (*Map, error) {
	t.Helper()

	data, err := json.Marshal(f.m)
	if err != nil {
		t.Fatal(err)
	}

	return Parse(data, "/maps", func(p string) ([]byte, error) {
		f.reads = append(f.reads, p)

		b, ok := f.files[p]
		if !ok {
			return nil, errors.New("no such file")
		}

		return b, nil
	})
}

// ---- the positive case ---------------------------------------------------------

func TestParseReadsTheMap(t *testing.T) {
	f := newFixture(t)

	m, err := f.parse(t)
	if err != nil {
		t.Fatalf("valid map refused: %v", err)
	}

	if m.Width != 4 || m.Height != 3 {
		t.Fatalf("size %dx%d, want 4x3", m.Width, m.Height)
	}

	// grass floor, mud floor, house wall: three kinds.
	if len(m.Kinds) != 3 {
		t.Fatalf("%d kinds, want 3: %+v", len(m.Kinds), m.Kinds)
	}

	if c := m.At(1, 1); c.Floor < 0 || m.Kinds[c.Floor].Name != "village#1" {
		t.Fatalf("cell 1,1 = %+v, want the mud floor", c)
	}

	if c := m.At(3, 2); c.Floor != -1 || c.Wall != -1 {
		t.Fatalf("cell 3,2 = %+v, want empty (gid 0)", c)
	}

	wall := m.At(2, 0).Wall
	if wall < 0 || m.Kinds[wall].Layer != LayerWall || m.Kinds[wall].Pixels.Bounds().Dy() != 200 {
		t.Fatalf("cell 2,0 wall = %d, want the 200-tall house on the wall layer", wall)
	}

	// Blocking: grass open; mud blocked but seen across (explicit false);
	// house blocked and sight-blocking (the default follows blocked); a tile
	// with no floor blocked but not sight-blocking.
	for _, c := range []struct {
		x, y           int
		blocked, sight bool
	}{
		{0, 0, false, false},
		{1, 1, true, false},
		{2, 0, true, true},
		{3, 2, true, false},
	} {
		if got := m.Blocked(c.x, c.y); got != c.blocked {
			t.Errorf("Blocked(%d,%d) = %v, want %v", c.x, c.y, got, c.blocked)
		}

		if got := m.BlocksSight(c.x, c.y); got != c.sight {
			t.Errorf("BlocksSight(%d,%d) = %v, want %v", c.x, c.y, got, c.sight)
		}
	}

	// Object pixels divide by the TILE HEIGHT on both axes.
	if m.StartX != 0.5 || m.StartY != 2.5 {
		t.Fatalf("start %.2f,%.2f, want 0.5,2.5", m.StartX, m.StartY)
	}

	if len(m.NPCs) != 1 || m.NPCs[0].Monstat != "warriv1" || m.NPCs[0].X != 3.2 || m.NPCs[0].Y != 1.6 {
		t.Fatalf("npcs %+v, want warriv1 at 3.2,1.6", m.NPCs)
	}

	// The art arrives as the colour it was painted.
	if px := m.Kinds[m.At(0, 0).Floor].Pixels.RGBAAt(80, 40); px != grass {
		t.Fatalf("grass pixel %v, want %v", px, grass)
	}

	// Each image is read once, however many tiles use it.
	counts := map[string]int{}
	for _, r := range f.reads {
		counts[r]++
	}

	for p, n := range counts {
		if n != 1 {
			t.Errorf("%s read %d times", p, n)
		}
	}
}

// The same Tiled tile on both layers is two kinds, because the renderer draws
// a floor and a wall by different rules.
func TestSameTileOnBothLayersIsTwoKinds(t *testing.T) {
	f := newFixture(t)
	f.files["/maps/tiles/grass.png"] = pngOf(t, 160, 80, grass)
	f.layer(1)["data"] = []any{1, 0, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0}

	m, err := f.parse(t)
	if err != nil {
		t.Fatal(err)
	}

	c := m.At(0, 0)
	if c.Floor < 0 || c.Wall < 0 || c.Floor == c.Wall {
		t.Fatalf("cell 0,0 = %+v, want distinct floor and wall kinds", c)
	}

	if m.Kinds[c.Wall].Layer != LayerWall || m.Kinds[c.Floor].Layer != LayerFloor {
		t.Fatalf("layers wrong: %+v", m.Kinds)
	}
}

// A sprite-sheet tileset is cut by columns, margin and spacing, on both axes.
func TestSheetTilesetIsCut(t *testing.T) {
	f := newFixture(t)

	// A 2x2 sheet, margin 2, spacing 4: grass, mud / house-brown, grass.
	colours := [2][2]color.RGBA{{grass, mud}, {house, grass}}
	const margin, spacing = 2, 4

	sheet := image.NewRGBA(image.Rect(0, 0, 2*margin+2*160+spacing, 2*margin+2*80+spacing))
	for y := 0; y < sheet.Bounds().Dy(); y++ {
		for x := 0; x < sheet.Bounds().Dx(); x++ {
			col, row := 0, 0
			if x >= margin+160+spacing/2 {
				col = 1
			}

			if y >= margin+80+spacing/2 {
				row = 1
			}

			sheet.SetRGBA(x, y, colours[row][col])
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, sheet); err != nil {
		t.Fatal(err)
	}

	f.files["/maps/sheet.png"] = buf.Bytes()
	f.m["tilesets"] = []any{map[string]any{
		"firstgid": 1, "name": "sheet", "image": "sheet.png", "columns": 2, "margin": margin, "spacing": spacing,
		"tilewidth": 160, "tileheight": 80, "tilecount": 4,
	}}
	f.layer(0)["data"] = []any{1, 1, 1, 1, 1, 2, 1, 1, 3, 1, 1, 4}
	f.layer(1)["data"] = []any{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

	m, err := f.parse(t)
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		x, y int
		want color.RGBA
	}{{0, 0, grass}, {1, 1, mud}, {0, 2, house}, {3, 2, grass}} {
		px := m.Kinds[m.At(c.x, c.y).Floor].Pixels
		for _, p := range [][2]int{{0, 0}, {159, 79}} {
			if got := px.RGBAAt(p[0], p[1]); got != c.want {
				t.Fatalf("cell %d,%d pixel %v is %v, want %v", c.x, c.y, p, got, c.want)
			}
		}
	}
}

// Two tilesets: a gid resolves to the one with the largest firstgid not
// above it, and a gid past the last tileset's tiles is in none.
func TestTwoTilesets(t *testing.T) {
	f := newFixture(t)
	f.files["/maps/tiles/moss.png"] = pngOf(t, 160, 80, color.RGBA{R: 20, G: 90, B: 60, A: 255})
	f.m["tilesets"] = append(f.m["tilesets"].([]any), map[string]any{
		"firstgid": 4, "name": "moss", "tilewidth": 160, "tileheight": 80, "tilecount": 1, "tiles": []any{
			map[string]any{"id": 0, "image": "tiles/moss.png"},
		},
	})
	f.layer(0)["data"].([]any)[0] = 4

	m, err := f.parse(t)
	if err != nil {
		t.Fatal(err)
	}

	if k := m.Kinds[m.At(0, 0).Floor]; k.Name != "moss#0" {
		t.Fatalf("gid 4 resolved to %s, want moss#0", k.Name)
	}

	if k := m.Kinds[m.At(2, 0).Wall]; k.Name != "village#2" {
		t.Fatalf("gid 3 resolved to %s, want village#2", k.Name)
	}

	f.layer(0)["data"].([]any)[0] = 5

	if _, err := f.parse(t); err == nil || !strings.Contains(err.Error(), "in no tileset") {
		t.Fatalf("gid 5, past the last tileset: %v", err)
	}
}

// Two tiles cut from the same image read it once.
func TestSharedImageReadOnce(t *testing.T) {
	f := newFixture(t)
	tiles := f.m["tilesets"].([]any)[0].(map[string]any)["tiles"].([]any)
	f.m["tilesets"].([]any)[0].(map[string]any)["tiles"] = append(tiles, map[string]any{
		"id": 3, "image": "tiles/grass.png", "properties": []any{
			map[string]any{"name": "blocked", "type": "bool", "value": true},
		},
	})
	f.m["tilesets"].([]any)[0].(map[string]any)["tilecount"] = 4
	f.layer(0)["data"].([]any)[1] = 4

	m, err := f.parse(t)
	if err != nil {
		t.Fatal(err)
	}

	if !m.Blocked(1, 0) || m.Blocked(0, 0) {
		t.Fatal("the two grass tiles are not told apart by their properties")
	}

	n := 0
	for _, r := range f.reads {
		if r == "/maps/tiles/grass.png" {
			n++
		}
	}

	if n != 1 {
		t.Fatalf("grass.png read %d times for two tiles; want once", n)
	}
}

// withBarn adds a 2x2 structure to the fixture's tileset (gid 4, art 320x200)
// and, when at is non-nil, places it with its bottom corner at tile at.
func withBarn(t *testing.T, f *fixture, at *[2]float64) {
	t.Helper()

	f.files["/maps/tiles/barn.png"] = pngOf(t, 320, 200, house)
	ts := f.m["tilesets"].([]any)[0].(map[string]any)
	ts["tilecount"] = 4
	ts["tiles"] = append(ts["tiles"].([]any), map[string]any{
		"id": 3, "image": "tiles/barn.png", "properties": []any{
			map[string]any{"name": "footprint_w", "type": "int", "value": 2},
			map[string]any{"name": "footprint_h", "type": "int", "value": 2},
		},
	})

	if at != nil {
		f.layer(2)["objects"] = append(f.objects(), map[string]any{
			"id": 20, "gid": 4, "x": at[0] * 80, "y": at[1] * 80, "width": 320.0, "height": 200.0,
		})
	}
}

func TestAStructureStandsOnItsFootprint(t *testing.T) {
	f := newFixture(t)
	withBarn(t, f, &[2]float64{2, 2}) // bottom corner at 2,2: tiles 0..1 x 0..1

	m, err := f.parse(t)
	if err != nil {
		t.Fatal(err)
	}

	if len(m.Structures) != 1 {
		t.Fatalf("%d structures, want 1", len(m.Structures))
	}

	s := m.Structures[0]
	if s.Footprint != image.Rect(0, 0, 2, 2) || s.Front() != image.Pt(1, 1) {
		t.Fatalf("footprint %v front %v; want (0,0)-(2,2), front 1,1", s.Footprint, s.Front())
	}

	k := m.Kinds[s.Kind]
	if k.Layer != LayerStructure || k.Footprint != image.Pt(2, 2) || k.Pixels.Bounds().Dx() != 320 {
		t.Fatalf("kind %+v", k)
	}

	// Grass at 0,0 was open; under the barn it is solid and opaque.
	if !m.Blocked(0, 0) || !m.BlocksSight(0, 0) || m.StructureOn(0, 0) != 0 || m.StructureOn(1, 1) != 0 {
		t.Fatalf("0,0 under the barn: blocked %v sight %v cell %+v", m.Blocked(0, 0), m.BlocksSight(0, 0), m.At(0, 0))
	}

	if m.StructureOn(2, 2) != -1 || m.StructureOn(0, 2) != -1 {
		t.Fatal("tiles beyond the footprint report a structure")
	}

	// Control: beside it, the grass is still open.
	if m.Blocked(0, 2) {
		t.Fatal("0,2 beside the barn is blocked")
	}
}

// A person listed BEFORE a structure in the file is still checked against it.
func TestAPersonUnderAStructureListedLaterIsRefused(t *testing.T) {
	f := newFixture(t)
	o := f.objects()[1].(map[string]any)
	o["x"], o["y"] = 80.0*0.5, 80.0*0.5
	withBarn(t, f, &[2]float64{2, 2})

	if _, err := f.parse(t); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("an npc under a house: %v", err)
	}
}

// ---- every refusal, one broken thing at a time ---------------------------------

func TestParseRefuses(t *testing.T) {
	cases := []struct {
		name    string
		breakIt func(t *testing.T, f *fixture)
		want    string
	}{
		{"orthogonal", func(t *testing.T, f *fixture) { f.m["orientation"] = "orthogonal" }, "isometric"},
		{"tile size", func(t *testing.T, f *fixture) { f.m["tilewidth"] = 64 }, "160x80"},
		{"infinite", func(t *testing.T, f *fixture) { f.m["infinite"] = true }, "infinite"},
		{"zero size", func(t *testing.T, f *fixture) { f.m["width"] = 0 }, "each side"},
		{"external tileset", func(t *testing.T, f *fixture) {
			f.m["tilesets"] = []any{map[string]any{"firstgid": 1, "source": "village.tsj"}}
		}, "embed"},
		{"base64 layer", func(t *testing.T, f *fixture) { f.layer(0)["encoding"] = "base64" }, "CSV"},
		{"compressed layer", func(t *testing.T, f *fixture) { f.layer(0)["compression"] = "zlib" }, "compressed"},
		{"layer size", func(t *testing.T, f *fixture) { f.layer(1)["width"] = 3 }, "3x3 on a 4x3"},
		{"short data", func(t *testing.T, f *fixture) { f.layer(0)["data"] = []any{1, 1} }, "holds 2 tiles"},
		{"flipped gid", func(t *testing.T, f *fixture) {
			f.layer(0)["data"].([]any)[0] = uint32(0x80000001)
		}, "flipped"},
		{"unknown gid", func(t *testing.T, f *fixture) { f.layer(0)["data"].([]any)[0] = 9 }, "in no tileset"},
		{"floor art size", func(t *testing.T, f *fixture) { f.files["/maps/tiles/grass.png"] = pngOf(t, 160, 81, grass) }, "exactly 160x80"},
		{"wall art width", func(t *testing.T, f *fixture) { f.files["/maps/tiles/house.png"] = pngOf(t, 150, 200, house) }, "160 wide"},
		{"wall art short", func(t *testing.T, f *fixture) { f.files["/maps/tiles/house.png"] = pngOf(t, 160, 60, house) }, "160 wide"},
		{"missing image", func(t *testing.T, f *fixture) { delete(f.files, "/maps/tiles/mud.png") }, "loading /maps/tiles/mud.png"},
		{"not a png", func(t *testing.T, f *fixture) { f.files["/maps/tiles/mud.png"] = []byte("nope") }, "decoding"},
		{"backslash path", func(t *testing.T, f *fixture) { f.tile(0)["image"] = `tiles\grass.png` }, "backslashes"},
		{"typo property", func(t *testing.T, f *fixture) {
			f.tile(0)["properties"] = []any{map[string]any{"name": "blockd", "type": "bool", "value": true}}
		}, "unknown tile property \"blockd\""},
		{"string bool", func(t *testing.T, f *fixture) {
			f.tile(0)["properties"] = []any{map[string]any{"name": "blocked", "type": "string", "value": "true"}}
		}, "must be a bool"},
		{"unknown layer", func(t *testing.T, f *fixture) { f.layer(1)["name"] = "roofs" }, "\"roofs\""},
		{"group layer", func(t *testing.T, f *fixture) { f.layer(1)["type"] = "group" }, "not one the game reads"},
		{"duplicate layer", func(t *testing.T, f *fixture) { f.layer(0)["name"] = "walls" }, "two layers"},
		{"no floor", func(t *testing.T, f *fixture) { f.m["layers"] = f.m["layers"].([]any)[1:] }, "no tile layer named \"floor\""},
		{"no objects layer", func(t *testing.T, f *fixture) {
			f.m["layers"] = f.m["layers"].([]any)[:2]
		}, "no object layer"},
		{"no start", func(t *testing.T, f *fixture) {
			f.layer(2)["objects"] = f.objects()[1:]
		}, "0 player_start"},
		{"two starts", func(t *testing.T, f *fixture) {
			f.layer(2)["objects"] = append(f.objects(), map[string]any{"id": 9, "type": "player_start", "x": 40.0, "y": 40.0})
		}, "2 player_start"},
		{"npc without monstat", func(t *testing.T, f *fixture) {
			delete(f.objects()[1].(map[string]any), "properties")
		}, "no monstat"},
		{"npc stray property", func(t *testing.T, f *fixture) {
			f.objects()[1].(map[string]any)["properties"] = []any{map[string]any{"name": "mood", "type": "string", "value": "sour"}}
		}, "unknown property \"mood\""},
		{"start on blocked", func(t *testing.T, f *fixture) {
			o := f.objects()[0].(map[string]any)
			o["x"], o["y"] = 80.0*1.5, 80.0*1.5
		}, "blocked or has no floor"},
		{"npc on the void", func(t *testing.T, f *fixture) {
			o := f.objects()[1].(map[string]any)
			o["x"], o["y"] = 80.0*3.5, 80.0*2.5
		}, "blocked or has no floor"},
		{"start off map", func(t *testing.T, f *fixture) {
			f.objects()[0].(map[string]any)["x"] = 80.0 * 9
		}, "off the 4x3 map"},
		{"unknown class", func(t *testing.T, f *fixture) { f.objects()[1].(map[string]any)["type"] = "chest" }, "class \"chest\""},
		{"no class", func(t *testing.T, f *fixture) { f.objects()[1].(map[string]any)["type"] = "" }, "has no class"},
		{"type and class disagree", func(t *testing.T, f *fixture) {
			f.objects()[1].(map[string]any)["class"] = "player_start"
		}, "says both"},
		{"hidden layer", func(t *testing.T, f *fixture) { f.layer(1)["visible"] = false }, "hidden"},
		{"translucent layer", func(t *testing.T, f *fixture) { f.layer(0)["opacity"] = 0.5 }, "opacity"},
		{"offset layer", func(t *testing.T, f *fixture) { f.layer(1)["offsetx"] = 16 }, "offset"},
		{"parallax layer", func(t *testing.T, f *fixture) { f.layer(0)["parallaxx"] = 0.8 }, "parallax"},
		{"tinted layer", func(t *testing.T, f *fixture) { f.layer(0)["tintcolor"] = "#ff0000" }, "tinted"},
		{"tileset offset", func(t *testing.T, f *fixture) {
			f.m["tilesets"].([]any)[0].(map[string]any)["tileoffset"] = map[string]any{"x": 0, "y": 8}
		}, "drawing offset"},
		{"grid render size", func(t *testing.T, f *fixture) {
			f.m["tilesets"].([]any)[0].(map[string]any)["tilerendersize"] = "grid"
		}, "\"grid\" size"},
		{"collision shapes", func(t *testing.T, f *fixture) {
			f.tile(2)["objectgroup"] = map[string]any{"type": "objectgroup", "objects": []any{}}
		}, "Tile Collision Editor"},
		{"animated tile", func(t *testing.T, f *fixture) {
			f.tile(0)["animation"] = []any{map[string]any{"tileid": 0, "duration": 100}}
		}, "animated"},
		{"sub-rectangle", func(t *testing.T, f *fixture) {
			tl := f.tile(0)
			tl["imagewidth"], tl["imageheight"], tl["width"], tl["height"] = 160, 80, 80, 80
		}, "part of its image"},
		{"tile object", func(t *testing.T, f *fixture) { f.objects()[1].(map[string]any)["gid"] = 1 }, "tile object"},
		{"structure off the grid", func(t *testing.T, f *fixture) { withBarn(t, f, &[2]float64{2.3, 2}) }, "off the tile grid"},
		{"structure over a wall", func(t *testing.T, f *fixture) { withBarn(t, f, &[2]float64{3, 2}) }, "overlaps a wall"},
		{"structure over bare ground", func(t *testing.T, f *fixture) { withBarn(t, f, &[2]float64{4, 3}) }, "bare ground"},
		{"structure off the map", func(t *testing.T, f *fixture) { withBarn(t, f, &[2]float64{5, 2}) }, "reaches off"},
		{"structures overlapping", func(t *testing.T, f *fixture) {
			withBarn(t, f, &[2]float64{2, 2})
			f.layer(2)["objects"] = append(f.objects(), map[string]any{"id": 21, "gid": 4, "x": 80.0 * 2, "y": 80.0 * 3})
		}, "overlaps another structure"},
		{"plain tile as a tile object", func(t *testing.T, f *fixture) {
			f.layer(2)["objects"] = append(f.objects(), map[string]any{"id": 21, "gid": 1, "x": 80.0, "y": 80.0})
		}, "only structures are tile objects"},
		{"structure on a tile layer", func(t *testing.T, f *fixture) {
			withBarn(t, f, nil)
			f.layer(1)["data"].([]any)[4] = 4
		}, "place it as a tile object"},
		{"structure art too narrow", func(t *testing.T, f *fixture) {
			withBarn(t, f, &[2]float64{2, 2})
			f.files["/maps/tiles/barn.png"] = pngOf(t, 240, 200, house)
		}, "wants 320 wide"},
		{"structure with one footprint side", func(t *testing.T, f *fixture) {
			withBarn(t, f, &[2]float64{2, 2})
			ts := f.m["tilesets"].([]any)[0].(map[string]any)
			tl := ts["tiles"].([]any)[3].(map[string]any)
			tl["properties"] = tl["properties"].([]any)[:1]
		}, "both footprint_w and footprint_h"},
		{"structure not square", func(t *testing.T, f *fixture) {
			withBarn(t, f, &[2]float64{2, 2})
			ts := f.m["tilesets"].([]any)[0].(map[string]any)
			tl := ts["tiles"].([]any)[3].(map[string]any)
			tl["properties"].([]any)[1].(map[string]any)["value"] = 1
			f.files["/maps/tiles/barn.png"] = pngOf(t, 240, 200, house)
		}, "not square"},
		{"structure not blocked", func(t *testing.T, f *fixture) {
			withBarn(t, f, &[2]float64{2, 2})
			ts := f.m["tilesets"].([]any)[0].(map[string]any)
			tl := ts["tiles"].([]any)[3].(map[string]any)
			tl["properties"] = append(tl["properties"].([]any), map[string]any{"name": "blocked", "type": "bool", "value": false})
		}, "blocked=false"},
		{"structure stretched", func(t *testing.T, f *fixture) {
			withBarn(t, f, &[2]float64{2, 2})
			f.objects()[len(f.objects())-1].(map[string]any)["width"] = 300.0
		}, "stretches"},
		{"objects aligned top-left", func(t *testing.T, f *fixture) {
			f.m["tilesets"].([]any)[0].(map[string]any)["objectalignment"] = "topleft"
		}, "Object Alignment"},
		{"structure flipped", func(t *testing.T, f *fixture) {
			withBarn(t, f, nil)
			f.layer(2)["objects"] = append(f.objects(), map[string]any{"id": 21, "gid": uint32(0x80000004), "x": 160.0, "y": 160.0})
		}, "flipped"},
		{"inside without area", func(t *testing.T, f *fixture) {
			f.layer(2)["objects"] = append(f.objects(), map[string]any{"id": 8, "type": "inside", "x": 80.0, "y": 80.0, "point": true})
		}, "no area"},
		{"inside off the map", func(t *testing.T, f *fixture) {
			f.layer(2)["objects"] = append(f.objects(), map[string]any{"id": 8, "type": "inside", "x": 80.0, "y": 80.0, "width": 800.0, "height": 80.0})
		}, "reaches off"},
		{"inside rotated", func(t *testing.T, f *fixture) {
			f.layer(2)["objects"] = append(f.objects(), map[string]any{"id": 8, "type": "inside", "x": 0.0, "y": 0.0, "width": 80.0, "height": 80.0, "rotation": 45.0})
		}, "rotated"},
		{"inside ellipse", func(t *testing.T, f *fixture) {
			f.layer(2)["objects"] = append(f.objects(), map[string]any{"id": 8, "type": "inside", "x": 0.0, "y": 0.0, "width": 80.0, "height": 80.0, "ellipse": true})
		}, "ellipse"},
		{"inside polygon", func(t *testing.T, f *fixture) {
			f.layer(2)["objects"] = append(f.objects(), map[string]any{"id": 8, "type": "inside", "x": 0.0, "y": 0.0,
				"polygon": []any{map[string]any{"x": 0, "y": 0}, map[string]any{"x": 80, "y": 0}, map[string]any{"x": 0, "y": 80}}})
		}, "polygon"},
		{"inside with a property", func(t *testing.T, f *fixture) {
			f.layer(2)["objects"] = append(f.objects(), map[string]any{"id": 8, "type": "inside", "x": 0.0, "y": 0.0, "width": 80.0, "height": 80.0,
				"properties": []any{map[string]any{"name": "safe", "type": "bool", "value": true}}})
		}, "takes no properties"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			c.breakIt(t, f)

			_, err := f.parse(t)
			if err == nil {
				t.Fatalf("broken map (%s) was accepted", c.name)
			}

			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not say %q", err, c.want)
			}
		})
	}
}

// An inside rectangle is read as every tile it touches.
func TestInsideAreas(t *testing.T) {
	f := newFixture(t)
	f.layer(2)["objects"] = append(f.objects(), map[string]any{
		"id": 7, "type": "inside", "x": 80.0, "y": 0.0, "width": 80.0 * 2.5, "height": 80.0,
	})

	m, err := f.parse(t)
	if err != nil {
		t.Fatal(err)
	}

	if len(m.Inside) != 1 || m.Inside[0].Min.X != 1 || m.Inside[0].Min.Y != 0 || m.Inside[0].Max.X != 4 || m.Inside[0].Max.Y != 1 {
		t.Fatalf("inside %v, want [(1,0)-(4,1)]", m.Inside)
	}

	for _, c := range []struct {
		x, y int
		want bool
	}{{1, 0, true}, {3, 0, true}, {0, 0, false}, {1, 1, false}} {
		if got := m.IsInside(c.x, c.y); got != c.want {
			t.Errorf("IsInside(%d,%d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}

	// And a map without one has none.
	g := newFixture(t)

	plain, err := g.parse(t)
	if err != nil {
		t.Fatal(err)
	}

	if len(plain.Inside) != 0 || plain.IsInside(1, 0) {
		t.Fatal("a map with no inside object reports inside ground")
	}
}

// Tiled 1.9+ may write "class" where older versions wrote "type"; both read.
func TestClassReadsLikeType(t *testing.T) {
	f := newFixture(t)
	o := f.objects()[0].(map[string]any)
	o["class"], o["type"] = "player_start", ""

	if _, err := f.parse(t); err != nil {
		t.Fatalf("class-only player_start refused: %v", err)
	}
}

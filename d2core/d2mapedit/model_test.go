package d2mapedit

import (
	"image"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// THE READ MODEL IS MEASURED AGAINST THE LOADER, NOT AGAINST A LIST OF
// EXPECTED NUMBERS.
//
// Every rule in this package is a copy of a rule in d2maptiled, and a copy is a
// thing that drifts. So the document's own answers are compared with the
// loader's, over the whole shipped village -- all 2304 tiles, twice, plus every
// structure, npc, inside area and kind. If either side changes its mind about
// anything, this goes red, and nobody has to have remembered to update a
// number.
func TestTheReadModelAgreesWithTheLoaderAboutTheVillage(t *testing.T) {
	data := villageBytes(t)
	d := openVillage(t)
	m := parseVillage(t, data)

	if got, want := d.Size(), image.Pt(m.Width, m.Height); got != want {
		t.Fatalf("size %v, the loader says %v", got, want)
	}

	if got, want := d.SoundEnv(), m.SoundEnv; got != want {
		t.Errorf("sound_env %d, the loader says %d", got, want)
	}

	if got, want := d.DisplayName(), m.DisplayName; got != want {
		t.Errorf("display_name %q, the loader says %q", got, want)
	}

	start, ok := d.Start()
	if !ok {
		t.Fatal("no single player_start")
	}

	if start.X != m.StartX || start.Y != m.StartY {
		t.Errorf("start %.2f,%.2f, the loader says %.2f,%.2f", start.X, start.Y, m.StartX, m.StartY)
	}

	// Blocked and BlocksSight over every tile: the two rules the editor's warn
	// and the engine's walk both hang off, and the two the brief calls out as
	// INDEPENDENT of each other.
	blockedDiff, sightDiff := 0, 0

	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			if d.Blocked(x, y) != m.Blocked(x, y) && blockedDiff < 5 {
				blockedDiff++

				t.Errorf("tile %d,%d: Blocked %v, the loader says %v", x, y, d.Blocked(x, y), m.Blocked(x, y))
			}

			if d.BlocksSight(x, y) != m.BlocksSight(x, y) && sightDiff < 5 {
				sightDiff++

				t.Errorf("tile %d,%d: BlocksSight %v, the loader says %v", x, y, d.BlocksSight(x, y), m.BlocksSight(x, y))
			}

			if d.Inside(x, y) != m.IsInside(x, y) {
				t.Fatalf("tile %d,%d: Inside %v, the loader says %v", x, y, d.Inside(x, y), m.IsInside(x, y))
			}
		}
	}

	// The structures, footprint for footprint and in the same order.
	structures := d.Structures()
	if len(structures) != len(m.Structures) {
		t.Fatalf("%d structures, the loader says %d", len(structures), len(m.Structures))
	}

	for i, st := range structures {
		if st.Footprint != m.Structures[i].Footprint {
			t.Errorf("structure %d (object %d) footprint %s, the loader says %s",
				i, st.ID, st.Footprint, m.Structures[i].Footprint)
		}

		k, found := d.Kind(st.GID)
		if !found {
			t.Fatalf("structure %d has gid %d, which is in no tileset", i, st.GID)
		}

		lk := m.Kinds[m.Structures[i].Kind]
		if k.Name != lk.Name || k.Footprint != lk.Footprint || k.Blocked != lk.Blocked || k.BlocksSight != lk.BlocksSight {
			t.Errorf("structure %d kind %+v, the loader says name %q footprint %v blocked %v sight %v",
				i, k, lk.Name, lk.Footprint, lk.Blocked, lk.BlocksSight)
		}
	}

	// The npcs, name for name and place for place.
	npcs := map[string]image.Point{}

	for _, o := range d.Objects() {
		if !o.IsStructure() && o.Class == ClassNPC {
			npcs[o.Monstat] = o.Tile()
		}
	}

	if len(npcs) != len(m.NPCs) {
		t.Errorf("%d npcs, the loader says %d", len(npcs), len(m.NPCs))
	}

	for _, n := range m.NPCs {
		at, found := npcs[n.Monstat]
		if !found {
			t.Errorf("the loader reads an npc %q the document does not", n.Monstat)

			continue
		}

		if want := image.Pt(int(n.X), int(n.Y)); at != want {
			t.Errorf("npc %q at %v, the loader says %v", n.Monstat, at, want)
		}
	}

	// The inside areas.
	var inside []image.Rectangle

	for _, o := range d.Objects() {
		if !o.IsStructure() && o.Class == ClassInside {
			inside = append(inside, o.Rect)
		}
	}

	if len(inside) != len(m.Inside) {
		t.Fatalf("%d inside areas, the loader says %d", len(inside), len(m.Inside))
	}

	for i := range inside {
		if inside[i] != m.Inside[i] {
			t.Errorf("inside area %d is %s, the loader says %s", i, inside[i], m.Inside[i])
		}
	}
}

// Every kind the village PLACES must come out of the document with the same
// blocked, blocks_sight and footprint the loader gives it. The loader only
// builds a Kind for a tile that is actually placed, so this walks the map and
// compares the pair it finds on each tile.
func TestEveryPlacedKindAgreesWithTheLoader(t *testing.T) {
	data := villageBytes(t)
	d := openVillage(t)
	m := parseVillage(t, data)

	checked := 0

	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			cell := m.At(x, y)

			for _, pair := range []struct {
				gid   int
				index int
				layer string
			}{
				{d.FloorTile(x, y), cell.Floor, LayerFloor},
				{d.WallTile(x, y), cell.Wall, LayerWalls},
			} {
				if pair.gid == 0 || pair.index < 0 {
					if (pair.gid == 0) != (pair.index < 0) {
						t.Fatalf("tile %d,%d %s: the document says gid %d, the loader says kind %d",
							x, y, pair.layer, pair.gid, pair.index)
					}

					continue
				}

				k, ok := d.Kind(pair.gid)
				if !ok {
					t.Fatalf("tile %d,%d %s: gid %d is in no tileset", x, y, pair.layer, pair.gid)
				}

				lk := m.Kinds[pair.index]
				if k.Name != lk.Name || k.Blocked != lk.Blocked || k.BlocksSight != lk.BlocksSight || k.Footprint != lk.Footprint {
					t.Fatalf("tile %d,%d %s: %q blocked=%v sight=%v fp=%v; the loader says %q blocked=%v sight=%v fp=%v",
						x, y, pair.layer, k.Name, k.Blocked, k.BlocksSight, k.Footprint,
						lk.Name, lk.Blocked, lk.BlocksSight, lk.Footprint)
				}

				checked++
			}
		}
	}

	if checked < 2000 {
		t.Errorf("only %d placed tiles were compared; the village has 2304 floor tiles alone, so this test is not looking at the map", checked)
	}
}

// Where a kind may be placed comes from checkArt read backwards, so it is
// checked against checkArt's own answer: build a one-tile map for each kind and
// each layer, and see whether the loader takes it.
func TestLegalLayersMatchesWhatTheLoaderWillTake(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		w, h      int
		footprint image.Point
		want      Legal
	}{
		{"a floor tile", tileW, tileH, image.Point{}, Legal{Floor: true, Wall: true}},
		{"a tall wall", tileW, maxWallHeight, image.Point{}, Legal{Wall: true}},
		{"a wall one pixel too tall", tileW, maxWallHeight + 1, image.Point{}, Legal{}},
		{"art one pixel too wide", tileW + 1, tileH, image.Point{}, Legal{}},
		{"art one pixel short", tileW, tileH - 1, image.Point{}, Legal{}},
		{"a 2x2 structure", 4 * artUnit, 160, image.Pt(2, 2), Legal{Structure: true}},
		{"a 2x2 structure of the wrong width", 3 * artUnit, 160, image.Pt(2, 2), Legal{}},
		{"a structure one pixel too tall", 4 * artUnit, maxStructureHeight + 1, image.Pt(2, 2), Legal{}},
		{"a structure at its tallest", 4 * artUnit, maxStructureHeight, image.Pt(2, 2), Legal{Structure: true}},
	} {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := LegalLayers(tc.footprint, tc.w, tc.h); got != tc.want {
				t.Fatalf("LegalLayers(%v, %d, %d) = %+v, want %+v", tc.footprint, tc.w, tc.h, got, tc.want)
			}

			// And the loader's own answer for each layer it claims.
			for _, layer := range []string{LayerFloor, LayerWalls, ClassStructure} {
				takes := loaderTakes(t, tc.footprint, tc.w, tc.h, layer)

				if want := tc.want.On(layer); takes != want {
					t.Errorf("%s: LegalLayers says %v, the loader %s it",
						layer, want, map[bool]string{true: "takes", false: "refuses"}[takes])
				}
			}
		})
	}
}

// loaderTakes builds a 4x4 map that places one piece of art of the given size
// on the given layer and reports whether d2maptiled.Parse accepts it.
func loaderTakes(t *testing.T, footprint image.Point, w, h int, layer string) bool {
	t.Helper()

	m, files := fixture(t)

	// A plain floor everywhere, no walls, no structures, so the only thing
	// under test is the one tile being placed.
	m.layer(LayerWalls)["data"] = fill(16, 0)
	m.layer(LayerObjects)["objects"] = []any{m.object(1)}

	files["test.png"] = pngOf(t, w, h)
	m.tileset()["tiles"] = append(m.tileset()["tiles"].([]any),
		tmj{"id": 3, "image": "test.png", "imagewidth": w, "imageheight": h})

	if footprint != (image.Point{}) {
		m.setProp(3, "footprint_w", "int", footprint.X)
		m.setProp(3, "footprint_h", "int", footprint.Y)
	}

	switch layer {
	case ClassStructure:
		// Bottom corner at 4,4 so a footprint up to 4x4 fits the 4x4 map.
		m.addObject(tmj{"id": 9, "name": "t", "type": ClassStructure,
			"x": 4 * tileH, "y": 4 * tileH, "width": w, "height": h,
			"rotation": 0, "visible": true, "point": false, "gid": 4})
	default:
		// 3,3 is away from the start at 0,0, so a blocked tile here cannot
		// refuse the map for a different reason.
		m.setTile(layer, 3, 3, 4)
	}

	_, err := d2maptiled.Parse(m.bytes(t), "", files.loader())

	return err == nil
}

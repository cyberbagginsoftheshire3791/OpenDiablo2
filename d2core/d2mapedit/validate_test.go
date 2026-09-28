package d2mapedit

import (
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// THE VALIDATOR IS MEASURED AGAINST THE LOADER, ONE BROKEN MAP AT A TIME.
//
// A validator that is a hand-copy of somebody else's rules is only as good as
// the copy. So this test does not check that Validate produces a particular
// message: it builds a map that works, breaks ONE rule, and asserts that the
// loader refuses it AND the editor refuses it. Both halves matter:
//
//   - The loader's refusal is this test's own positive control. A mutation that
//     does not actually break the map would otherwise pass forever while
//     proving nothing, which is exactly how a validator ends up with rules
//     nobody has ever seen fire.
//   - The editor's refusal is the thing under test. "Refuses" means either Open
//     would not take the file at all or Validate reported at least one problem;
//     both stop a save.
func TestTheLoaderAndTheValidatorAgree(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		bad  func(m tmj, files art)
		// rule, when set, is the Problem the editor is expected to raise. It is
		// checked when Open accepts the file, so a rule cannot be satisfied by
		// some unrelated complaint.
		rule Rule
	}{
		// ---- the map's own properties (tiled.go:322-340) ----
		{"an unknown map property", func(m tmj, _ art) {
			m["properties"] = append(m["properties"].([]any), tmj{"name": "format", "type": "int", "value": 2})
		}, RuleMapProperty},
		{"sound_env below one", func(m tmj, _ art) {
			m["properties"].([]any)[0].(tmj)["value"] = 0
		}, RuleMapProperty},
		{"sound_env as a string", func(m tmj, _ art) {
			p := m["properties"].([]any)[0].(tmj)
			p["type"], p["value"] = "string", "one"
		}, RuleMapProperty},
		{"display_name as an int", func(m tmj, _ art) {
			p := m["properties"].([]any)[1].(tmj)
			p["type"], p["value"] = "int", 3
		}, RuleMapProperty},

		// ---- tile properties (tiled.go:835-900) ----
		{"an unknown tile property", func(m tmj, _ art) { m.setProp(0, "slippery", "bool", true) }, RuleTileProperty},
		{"blocked as a string", func(m tmj, _ art) {
			m.setTile(LayerWalls, 3, 0, 2) // the kind must be PLACED to be read
			m.setProp(1, "blocked", "string", "yes")
		}, RuleTileProperty},
		{"a footprint of zero", func(m tmj, _ art) { m.setProp(2, "footprint_w", "int", 0) }, RuleTileProperty},
		{"a footprint past sixteen", func(m tmj, _ art) { m.setProp(2, "footprint_w", "int", maxFootprint+1) }, RuleTileProperty},
		{"a footprint that is not square", func(m tmj, _ art) { m.setProp(2, "footprint_h", "int", 3) }, RuleTileProperty},
		{"half a footprint", func(m tmj, _ art) { m.dropProp(2, "footprint_h") }, RuleTileProperty},
		{"a structure that says it is not solid", func(m tmj, _ art) { m.setProp(2, "blocked", "bool", false) }, RuleTileProperty},

		// ---- art (tiled.go:807-833, :773) ----
		{"floor art of the wrong height", func(m tmj, files art) {
			files["floor.png"] = pngOf(t, tileW, tileH+1)
			m.tile(0)["imageheight"] = tileH + 1
		}, RuleArtSize},
		{"floor art of the wrong width", func(m tmj, files art) {
			files["floor.png"] = pngOf(t, tileW+1, tileH)
			m.tile(0)["imagewidth"] = tileW + 1
		}, RuleArtSize},
		{"wall art too tall", func(m tmj, files art) {
			m.setTile(LayerWalls, 3, 0, 2)
			files["wall.png"] = pngOf(t, tileW, maxWallHeight+1)
			m.tile(1)["imageheight"] = maxWallHeight + 1
		}, RuleArtSize},
		{"structure art of the wrong width", func(m tmj, files art) {
			files["house.png"] = pngOf(t, 3*artUnit, 160)
			m.tile(2)["imagewidth"] = 3 * artUnit
			m.object(3)["width"] = 3 * artUnit
		}, RuleArtSize},
		{"structure art too tall", func(m tmj, files art) {
			files["house.png"] = pngOf(t, 4*artUnit, maxStructureHeight+1)
			m.tile(2)["imageheight"] = maxStructureHeight + 1
			m.object(3)["height"] = maxStructureHeight + 1
		}, RuleArtSize},
		{"a backslash in an image path", func(m tmj, files art) {
			files[`sub\floor.png`] = files["floor.png"]
			m.tile(0)["image"] = `sub\floor.png`
		}, RuleArtPath},
		{"art that is not there", func(m tmj, _ art) { m.tile(0)["image"] = "gone.png" }, RuleArtMissing},

		// ---- the tile layers ----
		{"a tile id in no tileset", func(m tmj, _ art) { m.setTile(LayerFloor, 1, 1, 99) }, RuleUnknownTile},
		{"a flipped tile", func(m tmj, _ art) { m.setTile(LayerFloor, 1, 1, 1|0x80000000) }, RuleTileFlipped},
		{"a structure painted onto the floor", func(m tmj, _ art) { m.setTile(LayerFloor, 1, 1, 3) }, RuleWrongLayer},
		{"a hidden layer", func(m tmj, _ art) { m.layer(LayerWalls)["visible"] = false }, RuleLayerSet},
		{"a half-transparent layer", func(m tmj, _ art) { m.layer(LayerWalls)["opacity"] = 0.5 }, RuleLayerSet},
		{"an offset layer", func(m tmj, _ art) { m.layer(LayerWalls)["offsetx"] = 8 }, RuleLayerSet},
		{"a tinted layer", func(m tmj, _ art) { m.layer(LayerWalls)["tintcolor"] = "#ff0000" }, RuleLayerSet},
		{"a layer the game does not read", func(m tmj, _ art) {
			m["layers"] = append(m["layers"].([]any), tmj{"id": 9, "name": "notes", "type": "tilelayer",
				"visible": true, "opacity": 1, "width": 4, "height": 4, "x": 0, "y": 0, "data": fill(16, 0)})
		}, RuleMapShape},

		// ---- the tileset (tiled.go:437-467) ----
		{"objects aligned anywhere but the bottom", func(m tmj, _ art) { m.tileset()["objectalignment"] = "top" }, RuleTilesetSet},
		{"a tileset drawing offset", func(m tmj, _ art) { m.tileset()["tileoffset"] = tmj{"x": 0, "y": -16} }, RuleTilesetSet},
		{"a tile with collision shapes", func(m tmj, _ art) {
			m.tile(1)["objectgroup"] = tmj{"draworder": "index", "objects": []any{}}
		}, RuleTilesetSet},
		{"an animated tile", func(m tmj, _ art) {
			m.tile(1)["animation"] = []any{tmj{"duration": 100, "tileid": 0}}
		}, RuleTilesetSet},
		{"a tile cut from part of its image", func(m tmj, _ art) {
			tile := m.tile(0)
			tile["x"], tile["y"], tile["width"], tile["height"] = 0, 0, tileW/2, tileH
		}, RuleTilesetSet},

		// ---- the objects (tiled.go:902-1098) ----
		{"two player_starts", func(m tmj, _ art) {
			m.addObject(tmj{"id": 9, "name": "another start", "type": ClassPlayerStart,
				"x": 2.5 * tileH, "y": 0.5 * tileH, "width": 0, "height": 0,
				"rotation": 0, "visible": true, "point": true})
		}, RuleStart},
		{"no player_start", func(m tmj, _ art) {
			l := m.layer(LayerObjects)
			l["objects"] = m.objects()[1:]
		}, RuleStart},
		{"a player_start with a property", func(m tmj, _ art) {
			m.object(1)["properties"] = []any{tmj{"name": "monstat", "type": "string", "value": "x"}}
		}, RuleObjectProps},
		{"a start on a blocked tile", func(m tmj, _ art) { m.setTile(LayerWalls, 0, 0, 2) }, RuleStandable},
		{"a start off the map", func(m tmj, _ art) { m.object(1)["x"] = 9 * tileH }, RuleOffMap},
		{"an npc with no monstat", func(m tmj, _ art) { delete(m.object(4), "properties") }, RuleNPC},
		{"an npc with an unknown property", func(m tmj, _ art) {
			m.object(4)["properties"] = []any{tmj{"name": "mood", "type": "string", "value": "grim"}}
		}, RuleNPC},
		{"an npc whose monstat is a number", func(m tmj, _ art) {
			m.object(4)["properties"] = []any{tmj{"name": "monstat", "type": "int", "value": 4}}
		}, RuleNPC},
		{"an npc on a blocked tile", func(m tmj, _ art) { m.setTile(LayerWalls, 1, 0, 2) }, RuleStandable},
		{"an object with no class", func(m tmj, _ art) { delete(m.object(4), "type") }, RuleObjectClass},
		{"an object that disagrees with itself", func(m tmj, _ art) { m.object(4)["class"] = ClassInside }, RuleObjectClass},
		{"an object with a class the game does not read", func(m tmj, _ art) { m.object(4)["type"] = "chicken" }, RuleObjectClass},
		{"an inside area with no area", func(m tmj, _ art) { m.object(2)["width"] = 0 }, RuleInside},
		{"an inside ellipse", func(m tmj, _ art) { m.object(2)["ellipse"] = true }, RuleInside},
		{"a rotated inside area", func(m tmj, _ art) { m.object(2)["rotation"] = 15 }, RuleInside},
		{"an inside polygon", func(m tmj, _ art) {
			m.object(2)["polygon"] = []any{tmj{"x": 0, "y": 0}, tmj{"x": 80, "y": 0}, tmj{"x": 0, "y": 80}}
		}, RuleInside},
		{"an inside area with a property", func(m tmj, _ art) {
			m.object(2)["properties"] = []any{tmj{"name": "monstat", "type": "string", "value": "x"}}
		}, RuleObjectProps},
		{"an inside area off the map", func(m tmj, _ art) { m.object(2)["width"] = 9 * tileH }, RuleOffMap},

		// ---- the structures (tiled.go:977-1042) ----
		{"a structure with a property", func(m tmj, _ art) {
			m.object(3)["properties"] = []any{tmj{"name": "blocked", "type": "bool", "value": true}}
		}, RuleObjectProps},
		{"a rotated structure", func(m tmj, _ art) { m.object(3)["rotation"] = 90 }, RuleObjectProps},
		{"a structure off the tile grid", func(m tmj, _ art) { m.object(3)["x"] = 4*tileH + 40 }, RuleOffGrid},
		{"a structure stretched in the editor", func(m tmj, _ art) { m.object(3)["width"] = 5 * artUnit }, RuleObjectSize},
		{"a structure reaching off the map", func(m tmj, _ art) { m.object(3)["x"] = 1 * tileH }, RuleOffMap},
		{"two structures on one tile", func(m tmj, _ art) {
			m.addObject(tmj{"id": 9, "name": "another house", "type": ClassStructure,
				"x": 3 * tileH, "y": 3 * tileH, "width": 4 * artUnit, "height": 160,
				"rotation": 0, "visible": true, "point": false, "gid": 3})
		}, RuleOverlap},
		{"a structure over a wall", func(m tmj, _ art) { m.setTile(LayerWalls, 3, 3, 2) }, RuleOverlap},
		{"a structure over bare ground", func(m tmj, _ art) { m.setTile(LayerFloor, 3, 3, 0) }, RuleBareGround},
		{"a flipped structure", func(m tmj, _ art) { m.object(3)["gid"] = 3 | 0x80000000 }, RuleTileFlipped},
		{"a tile object that is not a structure", func(m tmj, _ art) { m.object(3)["gid"] = 1 }, RuleFootprint},
		{"a tile object with a class the game does not read", func(m tmj, _ art) { m.object(3)["type"] = "house" }, RuleObjectClass},
		{"a structure whose tile is in no tileset", func(m tmj, _ art) { m.object(3)["gid"] = 99 }, RuleUnknownTile},
	}

	// THE BASELINE. If the fixture itself were refused, every case below would
	// "agree" for the wrong reason.
	t.Run("the fixture is a map the game takes", func(t *testing.T) {
		t.Parallel()

		m, files := fixture(t)
		data := m.bytes(t)

		if _, err := d2maptiled.Parse(data, "", files.loader()); err != nil {
			t.Fatalf("the loader refuses the unbroken fixture: %v", err)
		}

		d, err := Open(data)
		if err != nil {
			t.Fatalf("the editor will not open the unbroken fixture: %v", err)
		}

		if problems := d.Validate(files.size()); len(problems) > 0 {
			t.Fatalf("the validator complains about the unbroken fixture: %v", problems)
		}
	})

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m, files := fixture(t)
			tc.bad(m, files)
			data := m.bytes(t)

			// The positive control: the mutation must really break the map.
			_, perr := d2maptiled.Parse(data, "", files.loader())
			if perr == nil {
				t.Fatal("the loader ACCEPTS this map, so this case proves nothing about the validator; fix the case")
			}

			want := refusals[tc.name]
			if want == "" {
				t.Fatalf("this case does not say what the loader should refuse it for; add it to refusals (it said: %v)", perr)
			}

			if !strings.Contains(perr.Error(), want) {
				t.Fatalf("this case meant to break %q but the loader refused it for something else: %v", want, perr)
			}

			d, err := Open(data)
			if err != nil {
				return // Open refused it outright, which also stops a save
			}

			problems := d.Validate(files.size())
			if len(problems) == 0 {
				t.Fatal("the game refuses this map and the editor would have saved it")
			}

			if tc.rule == "" {
				return
			}

			for _, p := range problems {
				if p.Rule == tc.rule {
					return
				}
			}

			t.Errorf("want a %q problem, got %v", tc.rule, problems)
		})
	}
}

// refusals is what the LOADER must say about each case above -- MEASURED, by
// running every case and copying the message d2maptiled.Parse actually produced
// (27 Sep 2026). Requiring it is the second half of this test's control: a case
// that breaks the map in some OTHER way than it meant to would otherwise pass,
// and "the validator agrees" would be green over a rule nobody has ever seen
// fire. Two of these cases did exactly that before this map was added -- a wall
// placed under the house was refused for overlapping it, not for its art.
var refusals = map[string]string{
	"a backslash in an image path":                      "uses backslashes",
	"a flipped structure":                               "structure (object 3) is flipped",
	"a flipped tile":                                    "is flipped or rotated",
	"a footprint of zero":                               "\"footprint_w\" must be an int from 1 to 16",
	"a footprint past sixteen":                          "\"footprint_w\" must be an int from 1 to 16",
	"a footprint that is not square":                    "is not square",
	"a half-transparent layer":                          "has opacity 0.5",
	"a hidden layer":                                    "is hidden in the editor",
	"a layer the game does not read":                    "is not one the game reads",
	"a player_start with a property":                    "player_start (object 1) takes no properties",
	"a rotated inside area":                             "is rotated; the game reads it unrotated",
	"a rotated structure":                               "takes no properties and no rotation",
	"a start off the map":                               "is off the 4x4 map",
	"a start on a blocked tile":                         "player_start (object 1) stands on tile 0,0",
	"a structure off the tile grid":                     "off the tile grid",
	"a structure over a wall":                           "overlaps a wall",
	"a structure over bare ground":                      "stands over bare ground",
	"a structure painted onto the floor":                "placed on the floor layer",
	"a structure reaching off the map":                  "reaches off the 4x4 map",
	"a structure stretched in the editor":               "is sized 400x160 in the editor",
	"a structure that says it is not solid":             "cannot be blocked=false",
	"a structure whose tile is in no tileset":           "tile id 99 is in no tileset",
	"a structure with a property":                       "takes no properties and no rotation",
	"a tile cut from part of its image":                 "uses part of its image",
	"a tile id in no tileset":                           "tile id 99 is in no tileset",
	"a tile object that is not a structure":             "has no footprint_w/footprint_h",
	"a tile object with a class the game does not read": "is a tile object with class \"house\"",
	"a tile with collision shapes":                      "has collision shapes",
	"a tileset drawing offset":                          "has a drawing offset",
	"a tinted layer":                                    "is tinted #ff0000",
	"an animated tile":                                  "is animated",
	"an inside area off the map":                        "inside (object 2) reaches off the 4x4 map",
	"an inside area with a property":                    "inside (object 2) takes no properties",
	"an inside area with no area":                       "has no area",
	"an inside ellipse":                                 "is an ellipse",
	"an inside polygon":                                 "is a polygon",
	"an npc on a blocked tile":                          "npc warriv1 (object 4) stands on tile 1,0",
	"an npc whose monstat is a number":                  "monstat must be a string",
	"an npc with an unknown property":                   "unknown property \"mood\"",
	"an npc with no monstat":                            "has no monstat property",
	"an object with a class the game does not read":     "has class \"chicken\"",
	"an object that disagrees with itself":              "says both",
	"an object with no class":                           "has no class",
	"an offset layer":                                   "is offset by 8,0",
	"an unknown map property":                           "unknown map property \"format\"",
	"an unknown tile property":                          "unknown tile property \"slippery\"",
	"art that is not there":                             "loading gone.png",
	"blocked as a string":                               "property \"blocked\" must be a bool",
	"display_name as an int":                            "\"display_name\" must be a string",
	"floor art of the wrong height":                     "floor art is 160x81",
	"floor art of the wrong width":                      "floor art is 161x80",
	"half a footprint":                                  "needs both footprint_w and footprint_h",
	"no player_start":                                   "holds 0 player_start objects",
	"objects aligned anywhere but the bottom":           "aligns objects \"top\"",
	"sound_env as a string":                             "\"sound_env\" must be an int of at least 1",
	"sound_env below one":                               "\"sound_env\" must be an int of at least 1",
	"structure art of the wrong width":                  "structure art is 240x160",
	"structure art too tall":                            "structure art is 320x769",
	"two player_starts":                                 "holds 2 player_start objects",
	"two structures on one tile":                        "overlaps another structure",
	"wall art too tall":                                 "wall art is 160x513",
}

// The shipped village must validate CLEAN, or "place a house" starts from a red
// map and nobody can tell their mistake from the one that was already there.
func TestTheShippedVillageValidatesClean(t *testing.T) {
	if problems := openVillage(t).Validate(DirArt(villageDir())); len(problems) > 0 {
		for _, p := range problems {
			t.Errorf("%v", p)
		}
	}
}

// THE BOUNDS THE LOADER KEEPS TO ITSELF. maxWallHeight, maxStructureHeight,
// maxSide and the footprint range are unexported in d2maptiled, so this package
// repeats them -- and a repeated constant is a constant that drifts. Each one is
// measured here against a real Parse, one tile inside the bound and one tile
// past it.
func TestTheLoaderAgreesAboutEveryBound(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		build func(m tmj, files art, over bool)
	}{
		{"the tallest wall", func(m tmj, files art, over bool) {
			h := maxWallHeight
			if over {
				h++
			}

			m.setTile(LayerWalls, 3, 0, 2)
			files["wall.png"] = pngOf(t, tileW, h)
			m.tile(1)["imageheight"] = h
		}},
		{"the tallest structure", func(m tmj, files art, over bool) {
			h := maxStructureHeight
			if over {
				h++
			}

			files["house.png"] = pngOf(t, 4*artUnit, h)
			m.tile(2)["imageheight"] = h
			m.object(3)["height"] = h
		}},
		{"the largest footprint", func(m tmj, files art, over bool) {
			fp := maxFootprint
			if over {
				fp++
			}

			side := 2 * maxFootprint
			grow(m, files, side)
			m.setProp(2, "footprint_w", "int", fp)
			m.setProp(2, "footprint_h", "int", fp)
			files["house.png"] = pngOf(t, 2*fp*artUnit, 160)
			m.tile(2)["imagewidth"] = 2 * fp * artUnit
			m.object(3)["width"] = 2 * fp * artUnit

			// The far corner, so the footprint never reaches the start at 0,0.
			m.object(3)["x"] = float64(side) * tileH
			m.object(3)["y"] = float64(side) * tileH
		}},
		{"the widest map", func(m tmj, files art, over bool) {
			w := maxSide
			if over {
				w++
			}

			// One row tall, so the widest legal map is 1024 tiles rather than a
			// million: the bound under test is the SIDE.
			flatten(m, w)
		}},
	} {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, over := range []bool{false, true} {
				m, files := fixture(t)
				tc.build(m, files, over)
				data := m.bytes(t)

				_, perr := d2maptiled.Parse(data, "", files.loader())
				refused := perr != nil

				if refused != over {
					t.Fatalf("over=%v: the loader %s it (%v); this package's bound is wrong",
						over, map[bool]string{true: "refuses", false: "accepts"}[refused], perr)
				}

				d, oerr := Open(data)
				if oerr != nil {
					if !over {
						t.Fatalf("the editor will not open a map the game takes: %v", oerr)
					}

					continue
				}

				problems := d.Validate(files.size())
				if (len(problems) > 0) != over {
					t.Errorf("over=%v: the validator reports %v", over, problems)
				}
			}
		})
	}
}

// grow makes the fixture side x side, keeping the floor everywhere, the walls
// empty, the start at 0,0 and the inside area inside.
func grow(m tmj, _ art, side int) {
	m["width"], m["height"] = side, side

	for _, name := range []string{LayerFloor, LayerWalls} {
		l := m.layer(name)
		l["width"], l["height"] = side, side

		gid := 1
		if name == LayerWalls {
			gid = 0
		}

		l["data"] = fill(side*side, gid)
	}

	// The npc would otherwise sit inside the bigger house's footprint.
	m.layer(LayerObjects)["objects"] = []any{m.object(1), m.object(2), m.object(3)}
}

// flatten makes the fixture w x 1: one row of floor, the start on it, no walls,
// no structures, no inside area.
func flatten(m tmj, w int) {
	m["width"], m["height"] = w, 1

	for _, name := range []string{LayerFloor, LayerWalls} {
		l := m.layer(name)
		l["width"], l["height"] = w, 1

		gid := 1
		if name == LayerWalls {
			gid = 0
		}

		l["data"] = fill(w, gid)
	}

	m.layer(LayerObjects)["objects"] = []any{m.object(1)}
}

// Validate with nothing to read the art with is a PROBLEM, not a quiet pass:
// half of what the game refuses is art sizes.
func TestValidateWithNoArtSaysSo(t *testing.T) {
	problems := openVillage(t).Validate(nil)
	if len(problems) == 0 {
		t.Fatal("Validate(nil) reported nothing, so a caller with no art reader gets a green light")
	}

	for _, p := range problems {
		if p.Rule == RuleArtUnchecked {
			return
		}
	}

	t.Errorf("want a %q problem, got %v", RuleArtUnchecked, problems)
}

func TestPNGSizeReadsTheHeaderAndNothingElse(t *testing.T) {
	t.Parallel()

	real := pngOf(t, 480, 448)

	w, h, err := PNGSize(real)
	if err != nil || w != 480 || h != 448 {
		t.Fatalf("PNGSize = %d,%d,%v; want 480,448,nil", w, h, err)
	}

	// The first 24 bytes are enough, which is the whole point.
	if w, h, err := PNGSize(real[:pngHeaderLen]); err != nil || w != 480 || h != 448 {
		t.Errorf("PNGSize of the first %d bytes = %d,%d,%v", pngHeaderLen, w, h, err)
	}

	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"too short", real[:20], "first 24"},
		{"not a png", []byte("GIF89a...............   "), "signature"},
		{"no IHDR", append(append([]byte{}, real[:12]...), []byte("IDAT............")...), "IHDR"},
		{"a zero size", zeroSizePNG(real), "0x448"},
	} {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := PNGSize(tc.data)
			if err == nil {
				t.Fatalf("PNGSize accepted %s", tc.name)
			}

			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func zeroSizePNG(real []byte) []byte {
	out := append([]byte{}, real[:pngHeaderLen]...)
	out[16], out[17], out[18], out[19] = 0, 0, 0, 0

	return out
}

// MaxKinds: the map may place at most 256 distinct tiles, because each one
// becomes one sequence number under a single reserved style and a sequence is a
// byte (tiled.go:109-112, :657). The loader counts one kind per gid PER LAYER,
// so the validator counts the same way.
func TestTheKindCapIsWhereTheLoaderPutsIt(t *testing.T) {
	t.Parallel()

	for _, n := range []int{d2maptiled.MaxKinds, d2maptiled.MaxKinds + 1} {
		n := n
		over := n > d2maptiled.MaxKinds

		t.Run(map[bool]string{true: "one too many", false: "as many as it may"}[over], func(t *testing.T) {
			t.Parallel()

			m, files := manyKinds(t, n)
			data := m.bytes(t)

			_, perr := d2maptiled.Parse(data, "", files.loader())
			if (perr != nil) != over {
				t.Fatalf("%d kinds: the loader says %v; MaxKinds is %d", n, perr, d2maptiled.MaxKinds)
			}

			problems := mustOpen(t, data).Validate(files.size())
			if (len(problems) > 0) != over {
				t.Fatalf("%d kinds: the validator reports %v", n, problems)
			}

			if !over {
				return
			}

			for _, p := range problems {
				if p.Rule == RuleKindCount {
					return
				}
			}

			t.Errorf("want a %q problem, got %v", RuleKindCount, problems)
		})
	}
}

// manyKinds is a 17x17 map whose floor places n distinct tiles, all drawn from
// one 160x80 image so the fixture stays small.
func manyKinds(t *testing.T, n int) (tmj, art) {
	t.Helper()

	const side = 17 // 289 tiles, room for more than 256 distinct ones

	m, files := fixture(t)
	m["width"], m["height"] = side, side

	tiles := make([]any, 0, n)
	for id := 0; id < n; id++ {
		tiles = append(tiles, tmj{"id": id, "image": "floor.png", "imagewidth": tileW, "imageheight": tileH})
	}

	ts := m.tileset()
	ts["tiles"], ts["tilecount"] = tiles, n

	floor := make([]any, side*side)
	for i := range floor {
		gid := 1
		if i < n {
			gid = i + 1
		}

		floor[i] = gid
	}

	// The start stands on the last tile, which is always gid 1.
	start := m.object(1)
	start["x"], start["y"] = (side-0.5)*tileH, (side-0.5)*tileH

	f := m.layer(LayerFloor)
	f["width"], f["height"], f["data"] = side, side, floor

	w := m.layer(LayerWalls)
	w["width"], w["height"], w["data"] = side, side, fill(side*side, 0)

	m.layer(LayerObjects)["objects"] = []any{start}

	return m, files
}

// A sprite-sheet tileset (one image, cut into cells) is the other kind Tiled
// writes, and the loader takes both (tiled.go:29-30, :751-768). The cell has to
// be inside the sheet.
func TestASpriteSheetTilesetWorksAndItsCellsMustBeInIt(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		tilecount int
		place     int // the gid put on the floor
		over      bool
	}{
		{"a cell in the sheet", 4, 1, false},
		{"the last cell in the sheet", 4, 4, false},
		{"a cell off the bottom of the sheet", 6, 5, true},
	} {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m, files := sheetMap(t, tc.tilecount, tc.place)
			data := m.bytes(t)

			_, perr := d2maptiled.Parse(data, "", files.loader())
			if (perr != nil) != tc.over {
				t.Fatalf("the loader says %v", perr)
			}

			d := mustOpen(t, data)

			k, ok := d.Kind(tc.place)
			if !ok {
				t.Fatalf("gid %d is in no tileset", tc.place)
			}

			if !k.Sheet || k.Image != "sheet.png" {
				t.Errorf("gid %d came out as %+v; it is a cell of a sheet", tc.place, k)
			}

			problems := d.Validate(files.size())
			if (len(problems) > 0) != tc.over {
				t.Fatalf("the validator reports %v", problems)
			}

			if !tc.over {
				return
			}

			for _, p := range problems {
				if p.Rule == RuleArtSheetCell {
					return
				}
			}

			t.Errorf("want a %q problem, got %v", RuleArtSheetCell, problems)
		})
	}
}

// sheetMap is a 4x4 map whose only tileset is a 2x2 sheet of 160x80 cells, with
// gid `place` on one floor tile and gid 1 everywhere else.
func sheetMap(t *testing.T, tilecount, place int) (tmj, art) {
	t.Helper()

	m, files := fixture(t)
	files["sheet.png"] = pngOf(t, 2*tileW, 2*tileH)

	m["tilesets"] = []any{tmj{
		"columns": 2, "firstgid": 1, "margin": 0, "name": "sheet",
		"objectalignment": "bottom", "spacing": 0, "tilecount": tilecount,
		"image": "sheet.png", "imagewidth": 2 * tileW, "imageheight": 2 * tileH,
		"tilewidth": tileW, "tileheight": tileH,
	}}

	m.layer(LayerWalls)["data"] = fill(16, 0)
	m.layer(LayerObjects)["objects"] = []any{m.object(1)}
	m.setTile(LayerFloor, 3, 3, place)

	return m, files
}

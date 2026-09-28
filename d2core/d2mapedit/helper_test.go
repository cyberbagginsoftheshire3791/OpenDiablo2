package d2mapedit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// Every fixture in this file is SYNTHESIZED IN CODE (Article V): a PNG is
// png.Encode over a blank image, and a map is a Go literal. Nothing new lands
// under testdata/, and the only file on disk these tests read is the shipped
// data/strigoi/maps/village.tmj, which the loader's own tests already read.

// pngOf is a w x h PNG -- real bytes, with a real IHDR, because the validator
// reads the header and the loader decodes the whole thing.
func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatalf("encoding a %dx%d png: %v", w, h, err)
	}

	return buf.Bytes()
}

// art is the synthetic map's art, by the path the .tmj writes.
type art map[string][]byte

// loader is the art as d2maptiled.Parse wants it. The synthetic maps are parsed
// with dir "", so path.Join("", rel) is rel and one map serves both.
func (a art) loader() d2maptiled.Loader {
	return func(p string) ([]byte, error) {
		data, ok := a[path.Clean(p)]
		if !ok {
			return nil, fmt.Errorf("no such file %q", p)
		}

		return data, nil
	}
}

// size is the art as Validate wants it.
func (a art) size() Art {
	return func(rel string) (int, int, error) {
		data, ok := a[path.Clean(rel)]
		if !ok {
			return 0, 0, fmt.Errorf("no such file %q", rel)
		}

		return PNGSize(data)
	}
}

// tmj is a map under construction, as the plain JSON tree the file holds.
type tmj map[string]any

// fixture is a valid map and its art: 4x4 tiles, a floor everywhere, a wall
// tile in the palette, one 2x2 structure standing in the far corner, a
// player_start, an npc and an inside rectangle. It PARSES and it VALIDATES
// CLEAN, which TestTheLoaderAndTheValidatorAgree checks before it breaks
// anything.
func fixture(t *testing.T) (tmj, art) {
	t.Helper()

	files := art{
		"floor.png": pngOf(t, tileW, tileH),
		"wall.png":  pngOf(t, tileW, 120),
		"house.png": pngOf(t, 4*artUnit, 160), // a 2x2 footprint: (2+2)*80
	}

	m := tmj{
		"compressionlevel": -1,
		"height":           4,
		"infinite":         false,
		"nextlayerid":      4,
		"nextobjectid":     5,
		"orientation":      "isometric",
		"renderorder":      "right-down",
		"tiledversion":     "1.10.2",
		"tileheight":       tileH,
		"tilewidth":        tileW,
		"type":             "map",
		"version":          "1.10",
		"width":            4,
		"properties": []any{
			tmj{"name": "sound_env", "type": "int", "value": 1},
			tmj{"name": "display_name", "type": "string", "value": "Test"},
		},
		"tilesets": []any{
			tmj{
				"columns": 0, "firstgid": 1, "margin": 0, "name": "t",
				"objectalignment": "bottom", "spacing": 0, "tilecount": 3,
				"tileheight": 160, "tilewidth": 320,
				"tiles": []any{
					tmj{"id": 0, "image": "floor.png", "imagewidth": tileW, "imageheight": tileH},
					tmj{"id": 1, "image": "wall.png", "imagewidth": tileW, "imageheight": 120,
						"properties": []any{
							tmj{"name": "blocked", "type": "bool", "value": true},
							tmj{"name": "blocks_sight", "type": "bool", "value": false},
						}},
					tmj{"id": 2, "image": "house.png", "imagewidth": 4 * artUnit, "imageheight": 160,
						"properties": []any{
							tmj{"name": "footprint_w", "type": "int", "value": 2},
							tmj{"name": "footprint_h", "type": "int", "value": 2},
						}},
				},
			},
		},
		"layers": []any{
			tmj{"id": 1, "name": LayerFloor, "type": "tilelayer", "visible": true, "opacity": 1,
				"width": 4, "height": 4, "x": 0, "y": 0, "data": fill(16, 1)},
			tmj{"id": 2, "name": LayerWalls, "type": "tilelayer", "visible": true, "opacity": 1,
				"width": 4, "height": 4, "x": 0, "y": 0, "data": fill(16, 0)},
			tmj{"id": 3, "name": LayerObjects, "type": "objectgroup", "draworder": "topdown",
				"visible": true, "opacity": 1, "x": 0, "y": 0, "objects": []any{
					tmj{"id": 1, "name": "start", "type": ClassPlayerStart,
						"x": 0.5 * tileH, "y": 0.5 * tileH, "width": 0, "height": 0,
						"rotation": 0, "visible": true, "point": true},
					tmj{"id": 2, "name": "the yard", "type": ClassInside,
						"x": 0, "y": 0, "width": 2 * tileH, "height": 2 * tileH,
						"rotation": 0, "visible": true, "point": false},
					tmj{"id": 3, "name": "house", "type": ClassStructure,
						"x": 4 * tileH, "y": 4 * tileH, "width": 4 * artUnit, "height": 160,
						"rotation": 0, "visible": true, "point": false, "gid": 3},
					tmj{"id": 4, "name": "a villager", "type": ClassNPC,
						"x": 1.5 * tileH, "y": 0.5 * tileH, "width": 0, "height": 0,
						"rotation": 0, "visible": true, "point": true,
						"properties": []any{tmj{"name": "monstat", "type": "string", "value": "warriv1"}}},
				}},
		},
	}

	return m, files
}

func fill(n, gid int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = gid
	}

	return out
}

func (m tmj) bytes(t *testing.T) []byte {
	t.Helper()

	data, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		t.Fatalf("encoding the test map: %v", err)
	}

	return append(data, '\n')
}

// ---- reaching into a map under construction --------------------------------
//
// These take no *testing.T: a mutation in a table knows the layer and the tile
// it is reaching for are in the fixture, and a panic in a test is a failure
// that names its own line.

func (m tmj) layer(name string) tmj {
	for _, raw := range m["layers"].([]any) {
		l := raw.(tmj)
		if l["name"] == name {
			return l
		}
	}

	panic("the test map has no layer " + name)
}

// setTile writes a gid into a tile layer at x, y.
func (m tmj) setTile(layer string, x, y, gid int) {
	m.layer(layer)["data"].([]any)[x+y*m["width"].(int)] = gid
}

func (m tmj) objects() []any {
	return m.layer(LayerObjects)["objects"].([]any)
}

func (m tmj) object(id int) tmj {
	for _, raw := range m.objects() {
		o := raw.(tmj)
		if o["id"] == id {
			return o
		}
	}

	panic("the test map has no that object")
}

func (m tmj) addObject(o tmj) {
	l := m.layer(LayerObjects)
	l["objects"] = append(l["objects"].([]any), o)
}

// tile is one tileset tile by its local id.
func (m tmj) tile(local int) tmj {
	for _, raw := range m.tileset()["tiles"].([]any) {
		tile := raw.(tmj)
		if tile["id"] == local {
			return tile
		}
	}

	panic("the test map has no such tile")
}

func (m tmj) tileset() tmj {
	return m["tilesets"].([]any)[0].(tmj)
}

// setProp adds or replaces a property on a tileset tile.
func (m tmj) setProp(local int, name, typ string, value any) {
	tile := m.tile(local)
	props, _ := tile["properties"].([]any)

	for _, raw := range props {
		p := raw.(tmj)
		if p["name"] == name {
			p["type"], p["value"] = typ, value

			return
		}
	}

	tile["properties"] = append(props, tmj{"name": name, "type": typ, "value": value})
}

// dropProp removes a property from a tileset tile.
func (m tmj) dropProp(local int, name string) {
	tile := m.tile(local)
	props, _ := tile["properties"].([]any)
	kept := []any{}

	for _, raw := range props {
		if raw.(tmj)["name"] != name {
			kept = append(kept, raw)
		}
	}

	tile["properties"] = kept
}

// ---- the shipped village --------------------------------------------------

// villageDir is where the shipped map lives, from this package's directory.
func villageDir() string {
	return filepath.Join("..", "..", "data", "strigoi", "maps")
}

func villageBytes(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(villageDir(), "village.tmj"))
	if err != nil {
		t.Fatalf("reading the shipped village: %v", err)
	}

	return data
}

// villageLoader reads the village's art the way the game does: the map's own
// folder is /data/strigoi/maps, so "../structures/..." resolves beside it.
// Copied from d2maptiled's own village test so the two agree about the paths.
func villageLoader(t *testing.T) (string, d2maptiled.Loader) {
	t.Helper()

	strigoi := filepath.Dir(villageDir())

	return "/data/strigoi/maps", func(p string) ([]byte, error) {
		rel := strings.TrimPrefix(path.Clean(p), "/data/strigoi")

		return os.ReadFile(filepath.Join(strigoi, filepath.FromSlash(rel)))
	}
}

// openVillage opens the shipped village as a document.
func openVillage(t *testing.T) *Doc {
	t.Helper()

	d, err := Open(villageBytes(t))
	if err != nil {
		t.Fatalf("the shipped village will not open: %v", err)
	}

	return d
}

// parseVillage loads the shipped village through the real loader.
func parseVillage(t *testing.T, data []byte) *d2maptiled.Map {
	t.Helper()

	dir, load := villageLoader(t)

	m, err := d2maptiled.Parse(data, dir, load)
	if err != nil {
		t.Fatalf("the loader refuses the village: %v", err)
	}

	return m
}

// parseFixture runs a map under construction through the real loader.
func parseFixture(t *testing.T, m tmj, files art) (*d2maptiled.Map, error) {
	t.Helper()

	return d2maptiled.Parse(m.bytes(t), "", files.loader())
}

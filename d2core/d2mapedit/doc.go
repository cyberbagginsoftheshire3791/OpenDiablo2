// Package d2mapedit is the World Editor's authoring document: the .tmj a
// designer opens, the edits that change it, the validator that reproduces the
// game's refusals before anything is written, the undo stack, and the save.
// Pure Go -- no ebiten, no renderer, no asset manager -- so its tests run
// anywhere and the screen is written on top of it.
//
// # The .tmj is the authoring file
//
// There is no second format. The editor opens data/strigoi/maps/village.tmj,
// edits it, and writes it back; Tiled must still be able to open the result,
// and so must the game. Everything follows from that:
//
//   - A Doc holds the WHOLE parsed JSON tree, in the order the file wrote it,
//     so nextlayerid, nextobjectid, compressionlevel, tiledversion, draworder,
//     a tile object's width and height and anything a later Tiled adds survive
//     an open and a save untouched (json.go).
//   - The typed read model is a VIEW over that tree, rebuilt after every edit,
//     so the two cannot drift.
//   - Editor-only data -- the groups -- goes in the one place a .tmj has for
//     it: the "note" map property, which the loader reads and ignores
//     (tiled.go:325). Nothing else in a .tmj is free. The loader refuses an
//     unknown map property (tiled.go:335) and refuses ANY property on a
//     player_start (:932), an npc (:1084), an inside area (:1048) or a
//     structure (:987), so there is no per-object place to hide a group id.
//
// # Why the validator is the important part
//
// A map the game refuses does not stop the game. d2mapgen builds Diablo II's
// Act 1 instead and logs the reason (d2mapgen/authored.go:34-38), so a
// designer who saves a map with two player_starts gets a world his file has
// nothing to do with. Validate reproduces the loader's refusals -- naming the
// same rules -- and Save REFUSES to write a document that would be refused.
//
// # What Open refuses and what Validate reports
//
// Open refuses only what the read model cannot be built over: not JSON, not an
// isometric 160x80 finite map of a sane size, no "floor" tile layer of the
// right length in plain CSV, no "objects" layer, an external tileset. An
// editor must be able to OPEN a broken map in order to fix it, so everything
// else a hand-edit can get wrong -- an unknown object class, a property of the
// wrong type, a house over bare ground -- opens fine and comes back from
// Validate as a Problem.
package d2mapedit

import (
	"fmt"
	"image"
	"os"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// Doc is one authoring document: a .tmj's whole JSON tree and the typed read
// model over it.
type Doc struct {
	tree *jsonObject
	m    model
	// path is where the document was opened from, "" when it was opened from
	// bytes. Save takes its own path and does not read this.
	path string
}

// Open parses a .tmj into a document.
func Open(data []byte) (*Doc, error) {
	tree, err := parseTree(data)
	if err != nil {
		return nil, fmt.Errorf("not a Tiled JSON map: %w", err)
	}

	d := &Doc{tree: tree}

	if err := d.checkShape(); err != nil {
		return nil, err
	}

	d.derive()

	return d, nil
}

// OpenFile reads and opens a .tmj.
//
// A file that cannot be parsed is NOT moved aside here, unlike the kit screen's
// (d2game/d2gamescreen/kit.go:60-62): a kit can be chosen again and a map is
// somebody's week of work, so the caller decides, and SetAside is what it
// calls when the designer says yes.
func OpenFile(path string) (*Doc, error) {
	data, err := os.ReadFile(path) // nolint:gosec // the designer names the file
	if err != nil {
		return nil, err
	}

	d, err := Open(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	d.path = path

	return d, nil
}

// Path is where the document was opened from, "" when it came from bytes.
func (d *Doc) Path() string {
	return d.path
}

// Bytes renders the document, byte for byte the way Tiled and
// tools/villagemap write a .tmj: json.MarshalIndent with one space, the file's
// own key order, and a closing newline. An untouched document round-trips to
// the bytes it was opened from.
func (d *Doc) Bytes() ([]byte, error) {
	return writeTree(d.tree)
}

// checkShape refuses a tree the read model cannot be built over. See the
// package comment for where this line is drawn, and Validate for the rest.
func (d *Doc) checkShape() error {
	if typ := fieldString(d.tree, "type"); typ != "" && typ != "map" {
		return fmt.Errorf("type is %q, want a Tiled map (\"map\")", typ)
	}

	if o := fieldString(d.tree, "orientation"); o != "isometric" {
		return fmt.Errorf("orientation is %q; the engine draws isometric maps only", o)
	}

	tw, th := fieldInt(d.tree, "tilewidth"), fieldInt(d.tree, "tileheight")
	if tw != d2maptiled.TileWidth || th != d2maptiled.TileHeight {
		return fmt.Errorf("tiles are %dx%d; the engine's tile is %dx%d",
			tw, th, d2maptiled.TileWidth, d2maptiled.TileHeight)
	}

	if fieldBool(d.tree, "infinite") {
		return errInfinite
	}

	w, h := fieldInt(d.tree, "width"), fieldInt(d.tree, "height")
	if w <= 0 || h <= 0 || w > maxSide || h > maxSide {
		return fmt.Errorf("map is %dx%d tiles; each side must be 1..%d", w, h, maxSide)
	}

	for _, name := range []string{LayerFloor, LayerWalls} {
		l, ok := findLayer(d.tree, name)
		if !ok {
			if name == LayerWalls {
				continue
			}

			return fmt.Errorf("no tile layer named %q", name)
		}

		if err := checkTileLayer(l, w, h); err != nil {
			return err
		}
	}

	if _, ok := findLayer(d.tree, LayerObjects); !ok {
		return fmt.Errorf("no object layer named %q; it must hold the player_start", LayerObjects)
	}

	for _, raw := range fieldArray(d.tree, "tilesets") {
		ts, ok := asObject(raw)
		if !ok {
			return fmt.Errorf("a tileset is not a JSON object")
		}

		if src := fieldString(ts, "source"); src != "" {
			return fmt.Errorf("tileset %q is external; embed it in the map (Map > Embed Tileset)", src)
		}
	}

	return nil
}

var errInfinite = fmt.Errorf("the map is infinite; untick Map Properties > Infinite so it has a fixed size")

func checkTileLayer(l *jsonObject, w, h int) error {
	name := fieldString(l, "name")

	if typ := fieldString(l, "type"); typ != "tilelayer" {
		return fmt.Errorf("layer %q is a %s, want a tile layer", name, typ)
	}

	if enc := fieldString(l, "encoding"); enc != "" && enc != "csv" {
		return fmt.Errorf("layer %q is stored as %s; set Map Properties > Tile Layer Format to CSV", name, enc)
	}

	if c := fieldString(l, "compression"); c != "" {
		return fmt.Errorf("layer %q is compressed (%s); set Tile Layer Format to CSV", name, c)
	}

	data, ok := asArray(mustGet(l, "data"))
	if !ok {
		return fmt.Errorf("layer %q: data is not a list of tile ids", name)
	}

	if len(data) != w*h {
		return fmt.Errorf("layer %q holds %d tiles, want %d", name, len(data), w*h)
	}

	return nil
}

func mustGet(o *jsonObject, k string) any {
	v, _ := o.Get(k)

	return v
}

// ---- the typed read model -------------------------------------------------

// Size is the map's size in tiles.
func (d *Doc) Size() image.Point {
	return image.Pt(d.m.width, d.m.height)
}

// TileSize is the map's tile size in pixels, which the engine fixes at 160x80.
func (d *Doc) TileSize() image.Point {
	return image.Pt(d.m.tileWidth, d.m.tileHeight)
}

// Kinds is the tileset's tiles, the editor's palette.
func (d *Doc) Kinds() []Kind {
	out := make([]Kind, len(d.m.kinds))
	copy(out, d.m.kinds)

	return out
}

// Kind is the tile with this global id.
func (d *Doc) Kind(gid int) (Kind, bool) {
	return d.m.kind(gid)
}

// Objects is everything on the "objects" layer, in file order.
func (d *Doc) Objects() []Object {
	out := make([]Object, len(d.m.objects))
	copy(out, d.m.objects)

	return out
}

// Object is the object with this id.
func (d *Doc) Object(id int) (Object, bool) {
	if i := d.objectIndex(id); i >= 0 {
		return d.m.objects[i], true
	}

	return Object{}, false
}

func (d *Doc) objectIndex(id int) int {
	for i := range d.m.objects {
		if d.m.objects[i].ID == id {
			return i
		}
	}

	return -1
}

// Start is the map's player_start, if it has exactly the one the game wants.
func (d *Doc) Start() (Object, bool) {
	found := Object{}
	n := 0

	for _, o := range d.m.objects {
		if !o.IsStructure() && o.Class == ClassPlayerStart {
			found, n = o, n+1
		}
	}

	return found, n == 1
}

// Structures is every tile object, which is every structure.
func (d *Doc) Structures() []Object {
	var out []Object

	for _, o := range d.m.objects {
		if o.IsStructure() {
			out = append(out, o)
		}
	}

	return out
}

// StructureOn is the structure standing on tile x, y, if one does. When two
// overlap -- which the loader refuses (tiled.go:1027) and Validate reports --
// it is the first in file order, the one the loader would have kept.
func (d *Doc) StructureOn(x, y int) (Object, bool) {
	if !d.m.onMap(x, y) || len(d.m.cover) == 0 {
		return Object{}, false
	}

	mark := d.m.cover[d.m.at(x, y)]
	if mark == 0 {
		return Object{}, false
	}

	return d.m.objects[mark-1], true
}

// FloorTile is the global tile id on the floor layer at x, y; 0 for bare
// ground. Off the map is 0.
func (d *Doc) FloorTile(x, y int) int {
	if !d.m.onMap(x, y) {
		return 0
	}

	return d.m.floor[d.m.at(x, y)]
}

// WallTile is the global tile id on the walls layer at x, y; 0 for none. A map
// with no walls layer answers 0 everywhere.
func (d *Doc) WallTile(x, y int) int {
	if !d.m.onMap(x, y) || d.m.walls == nil {
		return 0
	}

	return d.m.walls[d.m.at(x, y)]
}

// HasWalls reports whether the map has a "walls" tile layer, which the loader
// treats as optional.
func (d *Doc) HasWalls() bool {
	return d.m.walls != nil
}

// SoundEnv is the map's sound_env property, 0 when it gives none.
func (d *Doc) SoundEnv() int {
	return d.m.soundEnv
}

// DisplayName is the map's display_name property.
func (d *Doc) DisplayName() string {
	return d.m.displayName
}

// NextObjectID is the id Tiled would give the next object it created.
func (d *Doc) NextObjectID() int {
	return fieldInt(d.tree, "nextobjectid")
}

// Inside reports whether tile x, y is in any inside area (tiled.go:222-231).
func (d *Doc) Inside(x, y int) bool {
	p := image.Pt(x, y)

	for _, o := range d.m.objects {
		if !o.IsStructure() && o.Class == ClassInside && p.In(o.Rect) {
			return true
		}
	}

	return false
}

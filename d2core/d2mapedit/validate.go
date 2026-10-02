package d2mapedit

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// THE VALIDATOR REPRODUCES THE LOADER'S REFUSALS BEFORE ANYTHING IS SAVED.
//
// This is the most valuable thing in the package, and the reason is not
// tidiness. A map the game refuses does NOT stop the game: d2mapgen logs the
// reason and silently builds Diablo II's Act 1 instead
// (d2core/d2map/d2mapgen/authored.go:34-38, "A map that fails to load is
// refused WHOLE and the generated world is built instead"). So a designer who
// saves a map with two player_starts does not get an error in front of him -- he
// gets somebody else's town, and spends an hour wondering why his fence moved.
//
// Every rule below names the same thing the loader's own message names, and
// every rule cites the line it came from. The test
// TestTheLoaderAndTheValidatorAgree builds a working map, breaks it one rule at
// a time, and asserts that d2maptiled.Parse and Validate refuse the same files:
// the validator is not trusted to be a copy, it is measured against the
// original.

// Rule is a short name for one of the loader's refusals, for a UI that wants to
// group or filter problems. The message is what a person reads.
type Rule string

// The rules, named after the thing that has to change.
const (
	RuleArtMissing   Rule = "art-missing"
	RuleArtSize      Rule = "art-size"
	RuleArtDeclared  Rule = "art-declared"
	RuleArtPath      Rule = "art-path"
	RuleArtSheetCell Rule = "art-sheet-cell"
	RuleFootprint    Rule = "footprint"
	RuleTileProperty Rule = "tile-property"
	RuleTileLayer    Rule = "tile-layer"
	RuleTileFlipped  Rule = "tile-flipped"
	RuleUnknownTile  Rule = "unknown-tile"
	RuleKindCount    Rule = "kind-count"
	RuleWrongLayer   Rule = "wrong-layer"
	RuleMapProperty  Rule = "map-property"
	RuleMapShape     Rule = "map-shape"
	RuleLayerSet     Rule = "layer-setting"
	RuleTilesetSet   Rule = "tileset-setting"
	RuleObjectClass  Rule = "object-class"
	RuleObjectProps  Rule = "object-properties"
	RuleStart        Rule = "player-start"
	RuleStandable    Rule = "standable"
	RuleNPC          Rule = "npc"
	RuleInside       Rule = "inside"
	RuleOffGrid      Rule = "off-grid"
	RuleObjectSize   Rule = "object-size"
	RuleOffMap       Rule = "off-map"
	RuleOverlap      Rule = "overlap"
	RuleBareGround   Rule = "bare-ground"
	RuleArtUnchecked Rule = "art-unchecked"
	RuleDuplicateID  Rule = "duplicate-object-id"
	RuleNextObjectID Rule = "next-object-id"
)

// Problem is one thing the game would refuse.
type Problem struct {
	Rule Rule
	// Msg names the same rule the loader's own message names.
	Msg string
	// Object is the object id a problem is about, 0 when it is not about one.
	Object int
	// Tile is the tile a problem is about, or an empty point when it is not
	// about one. HasTile says which.
	Tile    image.Point
	HasTile bool
	// Kind is the "<tileset>#<id>" the problem is about, "" when it is not
	// about one.
	Kind string
}

func (p Problem) Error() string {
	var b strings.Builder

	b.WriteString(string(p.Rule))
	b.WriteString(": ")

	if p.Object != 0 {
		fmt.Fprintf(&b, "object %d: ", p.Object)
	}

	if p.Kind != "" {
		fmt.Fprintf(&b, "%s: ", p.Kind)
	}

	if p.HasTile {
		fmt.Fprintf(&b, "tile %d,%d: ", p.Tile.X, p.Tile.Y)
	}

	b.WriteString(p.Msg)

	return b.String()
}

// RefusedError is what Save returns rather than writing a file the game would
// throw away.
type RefusedError struct {
	Problems []Problem
}

func (e *RefusedError) Error() string {
	lines := make([]string, 0, len(e.Problems)+1)
	lines = append(lines, fmt.Sprintf("the game would refuse this map (%d problems):", len(e.Problems)))

	for _, p := range e.Problems {
		lines = append(lines, "  "+p.Error())
	}

	return strings.Join(lines, "\n")
}

// Art reads the size of one piece of art the map refers to. rel is the path
// EXACTLY as the .tmj writes it; the loader resolves it with
// path.Join(dir-of-the-tmj, rel) (tiled.go:777).
type Art func(rel string) (w, h int, err error)

// DirArt reads art out of the directory a .tmj lives in, resolving the path the
// way the loader does and reading only the PNG header -- there is no reason to
// decode a 480x448 house to learn it is 480x448.
func DirArt(dir string) Art {
	return func(rel string) (int, int, error) {
		full := filepath.Join(dir, filepath.FromSlash(path.Clean(rel)))

		f, err := os.Open(full) // nolint:gosec // the map names its own art
		if err != nil {
			return 0, 0, err
		}

		defer f.Close()

		head := make([]byte, pngHeaderLen)
		if _, err := io.ReadFull(f, head); err != nil {
			return 0, 0, fmt.Errorf("%s: %w", full, err)
		}

		w, h, err := PNGSize(head)
		if err != nil {
			return 0, 0, fmt.Errorf("%s: %w", full, err)
		}

		return w, h, nil
	}
}

// pngHeaderLen is how much of a PNG has to be read to learn its size: the
// 8-byte signature, the IHDR chunk's 4-byte length and 4-byte type, then the
// width and height as big-endian uint32s.
const pngHeaderLen = 24

// PNGSize reads a PNG's width and height out of its IHDR chunk. Only the first
// 24 bytes are needed, which is why the validator can check every piece of art
// on a map without decoding an image.
func PNGSize(data []byte) (w, h int, err error) {
	const signature = "\x89PNG\r\n\x1a\n"

	if len(data) < pngHeaderLen {
		return 0, 0, fmt.Errorf("only %d bytes; a PNG's size is in its first %d", len(data), pngHeaderLen)
	}

	if string(data[:len(signature)]) != signature {
		return 0, 0, errors.New("not a PNG (the 8-byte signature is wrong)")
	}

	if string(data[12:16]) != "IHDR" {
		return 0, 0, fmt.Errorf("a PNG's first chunk must be IHDR, this one is %q", data[12:16])
	}

	w = int(binary.BigEndian.Uint32(data[16:20]))
	h = int(binary.BigEndian.Uint32(data[20:24]))

	if w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("the PNG says it is %dx%d", w, h)
	}

	return w, h, nil
}

// Validate answers "would the game take this file?" -- ALL of it, not the first
// refusal, because a designer fixing one thing at a time is a designer saving
// eleven times.
//
// art is required. Passing nil is a problem in itself rather than a silent skip:
// the art rules are half the loader's refusals, and a validator that quietly
// stops checking them is worse than none.
func (d *Doc) Validate(art Art) []Problem {
	v := &validation{doc: d, art: art, sizes: map[string]image.Point{}}

	if art == nil {
		v.add(Problem{Rule: RuleArtUnchecked, Msg: "no way to read the map's art was given, so the art rules -- half of what the game refuses -- were not checked"})
	}

	v.mapShape()
	v.mapProperties()
	v.layerSettings()
	v.tilesetSettings()
	v.kinds()
	v.tileLayers()
	v.objects()

	sort.SliceStable(v.problems, func(a, b int) bool {
		return v.problems[a].Rule < v.problems[b].Rule
	})

	return v.problems
}

type validation struct {
	doc      *Doc
	art      Art
	problems []Problem
	sizes    map[string]image.Point
	// used is every (gid, layer) pair actually placed, which is what the loader
	// counts against MaxKinds (tiled.go:652-658: one kind per gid PER LAYER).
	used map[kindUse]bool
}

type kindUse struct {
	gid   int
	layer string
}

func (v *validation) add(p Problem) {
	v.problems = append(v.problems, p)
}

func (v *validation) tile(p Problem, x, y int) {
	p.Tile, p.HasTile = image.Pt(x, y), true
	v.add(p)
}

// size is the art's real size, from its PNG header, read once per path.
func (v *validation) size(rel string) (image.Point, bool) {
	if v.art == nil {
		return image.Point{}, false
	}

	if sz, ok := v.sizes[rel]; ok {
		return sz, sz != image.Point{}
	}

	w, h, err := v.art(rel)
	if err != nil {
		v.sizes[rel] = image.Point{}
		v.add(Problem{Rule: RuleArtMissing, Msg: fmt.Sprintf("the art %q cannot be read: %v", rel, err)})

		return image.Point{}, false
	}

	v.sizes[rel] = image.Pt(w, h)

	return image.Pt(w, h), true
}

// ---- the map itself -------------------------------------------------------

func (v *validation) mapShape() {
	d := v.doc

	if typ := fieldString(d.tree, "type"); typ != "" && typ != "map" {
		v.add(Problem{Rule: RuleMapShape, Msg: fmt.Sprintf("type is %q, want a Tiled map (\"map\")", typ)})
	}

	if o := fieldString(d.tree, "orientation"); o != "isometric" {
		v.add(Problem{Rule: RuleMapShape, Msg: fmt.Sprintf("orientation is %q; the engine draws isometric maps only", o)})
	}

	if d.m.tileWidth != tileW || d.m.tileHeight != tileH {
		v.add(Problem{Rule: RuleMapShape, Msg: fmt.Sprintf("tiles are %dx%d; the engine's tile is %dx%d", d.m.tileWidth, d.m.tileHeight, tileW, tileH)})
	}

	if fieldBool(d.tree, "infinite") {
		v.add(Problem{Rule: RuleMapShape, Msg: "the map is infinite; untick Map Properties > Infinite"})
	}

	if d.m.width <= 0 || d.m.height <= 0 || d.m.width > maxSide || d.m.height > maxSide {
		v.add(Problem{Rule: RuleMapShape, Msg: fmt.Sprintf("map is %dx%d tiles; each side must be 1..%d", d.m.width, d.m.height, maxSide)})
	}

	if next := d.NextObjectID(); next <= 0 {
		v.add(Problem{Rule: RuleNextObjectID, Msg: "\"nextobjectid\" is missing or not a positive number; Tiled needs it to hand out the next object's id"})
	}

	// The loader reads exactly three layers and refuses any other
	// (tiled.go:556-565), and refuses two layers with one name (:546-548).
	seen := map[string]bool{}

	for _, raw := range fieldArray(d.tree, "layers") {
		l, ok := asObject(raw)
		if !ok {
			v.add(Problem{Rule: RuleMapShape, Msg: "a layer is not a JSON object"})

			continue
		}

		name, typ := fieldString(l, "name"), fieldString(l, "type")

		if seen[name] {
			v.add(Problem{Rule: RuleMapShape, Msg: fmt.Sprintf("two layers are named %q", name)})
		}

		seen[name] = true

		switch {
		case typ == "tilelayer" && (name == LayerFloor || name == LayerWalls):
		case typ == "objectgroup" && name == LayerObjects:
		default:
			v.add(Problem{Rule: RuleMapShape, Msg: fmt.Sprintf("layer %q (%s) is not one the game reads; use tile layers %q and %q and an object layer %q",
				name, typ, LayerFloor, LayerWalls, LayerObjects)})
		}
	}

	if !seen[LayerFloor] {
		v.add(Problem{Rule: RuleMapShape, Msg: fmt.Sprintf("no tile layer named %q", LayerFloor)})
	}

	if !seen[LayerObjects] {
		v.add(Problem{Rule: RuleMapShape, Msg: fmt.Sprintf("no object layer named %q; it must hold the player_start", LayerObjects)})
	}
}

// mapProperties is tiled.go:322-340: three names, and any other is a refusal,
// so "a new format key cannot simply be added".
func (v *validation) mapProperties() {
	for _, raw := range fieldArray(v.doc.tree, "properties") {
		p, ok := asObject(raw)
		if !ok {
			v.add(Problem{Rule: RuleMapProperty, Msg: "a map property is not a JSON object"})

			continue
		}

		name, typ := fieldString(p, "name"), fieldString(p, "type")
		value, _ := p.Get("value")

		switch name {
		case "note":
			// Read and IGNORED by the loader, type and all (tiled.go:325).
		case "sound_env":
			n, isInt := asInt(value)
			if typ != "int" || !isInt || n < 1 {
				v.add(Problem{Rule: RuleMapProperty, Msg: "map property \"sound_env\" must be an int of at least 1 (a row of soundenviron.txt)"})
			}
		case "display_name":
			if _, isString := asString(value); typ != "string" || !isString {
				v.add(Problem{Rule: RuleMapProperty, Msg: "map property \"display_name\" must be a string"})
			}
		default:
			v.add(Problem{Rule: RuleMapProperty, Msg: fmt.Sprintf("unknown map property %q; the game reads \"sound_env\" and \"display_name\" (and ignores \"note\")", name)})
		}
	}
}

// layerSettings is tmjLayer.checkEditorOnly (tiled.go:419-434): settings the
// game would silently ignore are refused rather than ignored.
func (v *validation) layerSettings() {
	for _, raw := range fieldArray(v.doc.tree, "layers") {
		l, ok := asObject(raw)
		if !ok {
			continue
		}

		name := fieldString(l, "name")

		if vis, has := l.Get("visible"); has {
			if b, _ := asBool(vis); !b {
				v.add(Problem{Rule: RuleLayerSet, Msg: fmt.Sprintf("layer %q is hidden in the editor, and the game would draw it anyway; show it or delete it", name)})
			}
		}

		if op, has := l.Get("opacity"); has {
			if f, _ := asFloat(op); f != 1 {
				v.add(Problem{Rule: RuleLayerSet, Msg: fmt.Sprintf("layer %q has opacity %v; the game draws layers opaque", name, f)})
			}
		}

		if fieldFloat(l, "offsetx") != 0 || fieldFloat(l, "offsety") != 0 {
			v.add(Problem{Rule: RuleLayerSet, Msg: fmt.Sprintf("layer %q is offset by %v,%v; the game draws it unshifted",
				name, fieldFloat(l, "offsetx"), fieldFloat(l, "offsety"))})
		}

		for _, key := range []string{"parallaxx", "parallaxy"} {
			if px, has := l.Get(key); has {
				if f, _ := asFloat(px); f != 1 {
					v.add(Problem{Rule: RuleLayerSet, Msg: fmt.Sprintf("layer %q has parallax; the game has none", name)})
				}
			}
		}

		if tint := fieldString(l, "tintcolor"); tint != "" {
			v.add(Problem{Rule: RuleLayerSet, Msg: fmt.Sprintf("layer %q is tinted %s; the game draws it untinted", name, tint)})
		}
	}
}

// tilesetSettings is tmjTileset.checkEditorOnly (tiled.go:437-467).
func (v *validation) tilesetSettings() {
	for _, raw := range fieldArray(v.doc.tree, "tilesets") {
		ts, ok := asObject(raw)
		if !ok {
			v.add(Problem{Rule: RuleTilesetSet, Msg: "a tileset is not a JSON object"})

			continue
		}

		name := fieldString(ts, "name")

		if src := fieldString(ts, "source"); src != "" {
			v.add(Problem{Rule: RuleTilesetSet, Msg: fmt.Sprintf("tileset %q is external (%s); embed it in the map (Map > Embed Tileset)", name, src)})
		}

		if fieldInt(ts, "firstgid") < 1 {
			v.add(Problem{Rule: RuleTilesetSet, Msg: fmt.Sprintf("tileset %q has firstgid %d", name, fieldInt(ts, "firstgid"))})
		}

		if off, has := ts.Get("tileoffset"); has {
			if o, ok := asObject(off); ok && (fieldInt(o, "x") != 0 || fieldInt(o, "y") != 0) {
				v.add(Problem{Rule: RuleTilesetSet, Msg: fmt.Sprintf("tileset %q has a drawing offset; the game draws tiles where they stand", name)})
			}
		}

		if s := fieldString(ts, "tilerendersize"); s != "" && s != "tile" {
			v.add(Problem{Rule: RuleTilesetSet, Msg: fmt.Sprintf("tileset %q renders at %q size; the game draws art at its own size", name, s)})
		}

		if s := fieldString(ts, "fillmode"); s != "" && s != "stretch" {
			v.add(Problem{Rule: RuleTilesetSet, Msg: fmt.Sprintf("tileset %q uses fill mode %q", name, s)})
		}

		if s := fieldString(ts, "objectalignment"); s != "" && s != "unspecified" && s != "bottom" {
			v.add(Problem{Rule: RuleTilesetSet, Msg: fmt.Sprintf("tileset %q aligns objects %q; structures need \"bottom\" (Tileset > Object Alignment)", name, s)})
		}

		if fieldString(ts, "image") != "" && (fieldInt(ts, "columns") <= 0 || fieldInt(ts, "tilewidth") <= 0 || fieldInt(ts, "tileheight") <= 0) {
			v.add(Problem{Rule: RuleTilesetSet, Msg: fmt.Sprintf("tileset %q is a sprite sheet with no columns or tile size", name)})
		}

		for _, tileRaw := range fieldArray(ts, "tiles") {
			tile, ok := asObject(tileRaw)
			if !ok {
				continue
			}

			kind := fmt.Sprintf("%s#%d", name, fieldInt(tile, "id"))

			if og, has := tile.Get("objectgroup"); has && og != nil {
				v.add(Problem{Rule: RuleTilesetSet, Kind: kind, Msg: "has collision shapes from the Tile Collision Editor; the game reads the \"blocked\" property instead"})
			}

			if an, has := tile.Get("animation"); has && an != nil {
				v.add(Problem{Rule: RuleTilesetSet, Kind: kind, Msg: "is animated; the game draws one frame"})
			}

			if w := fieldInt(tile, "width"); w > 0 &&
				(fieldInt(tile, "x") != 0 || fieldInt(tile, "y") != 0 ||
					w != fieldInt(tile, "imagewidth") || fieldInt(tile, "height") != fieldInt(tile, "imageheight")) {
				v.add(Problem{Rule: RuleTilesetSet, Kind: kind, Msg: "uses part of its image; the game draws the whole image"})
			}
		}
	}
}

// ---- the kinds ------------------------------------------------------------

func (v *validation) kinds() {
	for _, k := range v.doc.m.kinds {
		if err := k.PropertyError(); err != nil {
			v.add(Problem{Rule: RuleTileProperty, Kind: k.Name, Msg: err.Error()})

			continue
		}

		if strings.Contains(k.Image, `\`) {
			v.add(Problem{Rule: RuleArtPath, Kind: k.Name,
				Msg: fmt.Sprintf("image path %q uses backslashes; Tiled writes forward slashes on every system", k.Image)})

			continue
		}

		if k.Image == "" {
			v.add(Problem{Rule: RuleArtMissing, Kind: k.Name, Msg: "has no image"})

			continue
		}

		onDisk, ok := v.size(k.Image)
		if !ok {
			continue
		}

		if k.Sheet {
			if !k.Cell.In(image.Rect(0, 0, onDisk.X, onDisk.Y)) {
				v.add(Problem{Rule: RuleArtSheetCell, Kind: k.Name,
					Msg: fmt.Sprintf("its cell %s lies outside its sheet %s (%dx%d)", k.Cell, k.Image, onDisk.X, onDisk.Y)})
			}

			continue
		}

		if k.Declared != (image.Point{}) && k.Declared != onDisk {
			v.add(Problem{Rule: RuleArtDeclared, Kind: k.Name,
				Msg: fmt.Sprintf("the map says its art is %dx%d and %s is %dx%d; Tiled draws the map's number and the game reads the file's, so they must agree",
					k.Declared.X, k.Declared.Y, k.Image, onDisk.X, onDisk.Y)})
		}
	}
}

// artSizeOf is the size the loader would give a kind's art: the whole image for
// a collection tile, the tileset's cell for a sheet tile (tiled.go:737-769).
func (v *validation) artSizeOf(k Kind) (image.Point, bool) {
	if k.Sheet {
		return k.Declared, k.Declared != image.Point{}
	}

	return v.size(k.Image)
}

// checkArtFor is checkArt (tiled.go:807-833) for one placement.
func (v *validation) checkArtFor(k Kind, layer string, where Problem) {
	size, ok := v.artSizeOf(k)
	if !ok {
		return
	}

	w, h := size.X, size.Y

	switch layer {
	case ClassStructure:
		want := (k.Footprint.X + k.Footprint.Y) * artUnit
		if w != want || h < tileH || h > maxStructureHeight {
			where.Rule, where.Msg = RuleArtSize, fmt.Sprintf(
				"structure art is %dx%d; a %dx%d footprint wants %d wide and %d..%d tall",
				w, h, k.Footprint.X, k.Footprint.Y, want, tileH, maxStructureHeight)
			v.add(where)
		}
	case LayerFloor:
		if w != tileW || h != tileH {
			where.Rule, where.Msg = RuleArtSize, fmt.Sprintf("floor art is %dx%d, want exactly %dx%d", w, h, tileW, tileH)
			v.add(where)
		}
	case LayerWalls:
		if w != tileW || h < tileH || h > maxWallHeight {
			where.Rule, where.Msg = RuleArtSize, fmt.Sprintf("wall art is %dx%d, want %d wide and %d..%d tall", w, h, tileW, tileH, maxWallHeight)
			v.add(where)
		}
	}
}

// ---- the tile layers ------------------------------------------------------

// gidFlipMask covers Tiled's flip and rotation flags in the top bits of a gid
// (tiled.go:124).
const gidFlipMask = 0xF0000000

func (v *validation) tileLayers() {
	v.used = map[kindUse]bool{}

	for _, name := range []string{LayerFloor, LayerWalls} {
		layer, ok := findLayer(v.doc.tree, name)
		if !ok {
			continue
		}

		if enc := fieldString(layer, "encoding"); enc != "" && enc != "csv" {
			v.add(Problem{Rule: RuleTileLayer, Msg: fmt.Sprintf("layer %q is stored as %s; set Map Properties > Tile Layer Format to CSV", name, enc)})
		}

		if c := fieldString(layer, "compression"); c != "" {
			v.add(Problem{Rule: RuleTileLayer, Msg: fmt.Sprintf("layer %q is compressed (%s); set Tile Layer Format to CSV", name, c)})
		}

		if lw, lh := fieldInt(layer, "width"), fieldInt(layer, "height"); lw != v.doc.m.width || lh != v.doc.m.height {
			v.add(Problem{Rule: RuleTileLayer, Msg: fmt.Sprintf("layer %q is %dx%d on a %dx%d map", name, lw, lh, v.doc.m.width, v.doc.m.height)})
		}

		data := fieldArray(layer, "data")
		if len(data) != v.doc.m.width*v.doc.m.height {
			v.add(Problem{Rule: RuleTileLayer, Msg: fmt.Sprintf("layer %q holds %d tiles, want %d", name, len(data), v.doc.m.width*v.doc.m.height)})
		}

		for i, raw := range data {
			x, y := i%v.doc.m.width, i/v.doc.m.width

			gid, isInt := asInt(raw)
			if !isInt {
				v.tile(Problem{Rule: RuleTileLayer, Msg: fmt.Sprintf("layer %q: %v is not a tile id", name, raw)}, x, y)

				continue
			}

			if gid == 0 {
				continue
			}

			if uint32(gid)&gidFlipMask != 0 {
				v.tile(Problem{Rule: RuleTileFlipped, Msg: fmt.Sprintf("layer %q tile is flipped or rotated; the game draws art as it is", name)}, x, y)

				continue
			}

			v.placeKind(gid, name, x, y)
		}
	}

	// The structures' kinds count too: one kind per gid per layer, and a
	// structure is its own layer (tiled.go:652-658, :690).
	for _, o := range v.doc.m.objects {
		if o.IsStructure() {
			v.used[kindUse{o.GID, ClassStructure}] = true
		}
	}

	if len(v.used) > d2maptiled.MaxKinds {
		v.add(Problem{Rule: RuleKindCount, Msg: fmt.Sprintf(
			"the map places %d distinct tiles (counting one tile used on two layers twice, as the game does); the most it may use is %d",
			len(v.used), d2maptiled.MaxKinds)})
	}
}

// placeKind checks one tile placed on one layer.
func (v *validation) placeKind(gid int, layer string, x, y int) {
	v.used[kindUse{gid, layer}] = true

	k, ok := v.doc.Kind(gid)
	if !ok {
		v.tile(Problem{Rule: RuleUnknownTile, Msg: fmt.Sprintf("tile id %d is in no tileset", gid)}, x, y)

		return
	}

	if k.PropertyError() != nil {
		return // already reported once, against the kind
	}

	if k.Footprint != (image.Point{}) {
		v.tile(Problem{Rule: RuleWrongLayer, Kind: k.Name, Msg: fmt.Sprintf(
			"is a structure (it has a footprint) placed on the %s layer; place it as a tile object on the objects layer", layer)}, x, y)

		return
	}

	// Fog of war F4: height is the ground's (tiled.go's parser.kind).
	if k.Height != 0 && layer != LayerFloor {
		v.tile(Problem{Rule: RuleWrongLayer, Kind: k.Name, Msg: fmt.Sprintf(
			"carries \"height\", a floor tile's property, but is placed on the %s layer; put it on the floor layer", layer)}, x, y)

		return
	}

	// The raid's R3a: a building is a wall or a structure (tiled.go's
	// parser.kind).
	if k.Building && layer == LayerFloor {
		v.tile(Problem{Rule: RuleWrongLayer, Kind: k.Name, Msg: "carries \"building\" but is placed on the floor layer; a building is a wall or a structure -- put it on the walls layer"}, x, y)

		return
	}

	p := Problem{Kind: k.Name, Tile: image.Pt(x, y), HasTile: true}
	v.checkArtFor(k, layer, p)
}

// ---- the objects ----------------------------------------------------------

func (v *validation) objects() {
	starts := 0
	ids := map[int]bool{}

	var vc villageCheck

	for _, o := range v.doc.m.objects {
		if ids[o.ID] {
			v.add(Problem{Rule: RuleDuplicateID, Object: o.ID, Msg: "two objects share this id"})
		}

		ids[o.ID] = true

		if o.IsStructure() {
			v.structure(o)

			continue
		}

		// An object that names both a "type" and a "class" and disagrees with
		// itself is refused; the loader will not guess which the designer meant
		// (tiled.go:924-926). Tile objects are exempt: structure() reads
		// whichever is set and does not compare them (tiled.go:978-981).
		if raw, found := v.doc.rawObject(o.ID); found {
			cls, typ := fieldString(raw, "class"), fieldString(raw, "type")
			if cls != "" && typ != "" && cls != typ {
				v.add(Problem{Rule: RuleObjectClass, Object: o.ID,
					Msg: fmt.Sprintf("says both %q and %q", typ, cls)})
			}
		}

		switch o.Class {
		case ClassPlayerStart:
			starts++

			if len(o.props) > 0 {
				v.add(Problem{Rule: RuleObjectProps, Object: o.ID, Msg: "player_start takes no properties"})
			}

			v.standable("player_start", o)
		case ClassNPC:
			if o.monstatErr != nil {
				v.add(Problem{Rule: RuleNPC, Object: o.ID, Msg: fmt.Sprintf("npc: %v", o.monstatErr)})
			}

			v.standable("npc "+o.Monstat, o)
			vc.npcs = append(vc.npcs, o)
		case ClassInside:
			v.inside(o)
		case ClassHousehold:
			v.household(o, &vc)
		case ClassHotar:
			v.hotar(o, &vc)
		case ClassWatchPost:
			v.watchPost(o)
		case "":
			v.add(Problem{Rule: RuleObjectClass, Object: o.ID,
				Msg: fmt.Sprintf("object %q has no class; set it to %s", o.Name, d2maptiled.ObjectClassesOr)})
		default:
			v.add(Problem{Rule: RuleObjectClass, Object: o.ID,
				Msg: fmt.Sprintf("has class %q; the game reads %s", o.Class, d2maptiled.ObjectClassesAnd)})
		}
	}

	if starts != 1 {
		v.add(Problem{Rule: RuleStart, Msg: fmt.Sprintf("the objects layer holds %d player_start objects, want exactly 1", starts)})
	}

	// The raid's R3a: the loader's village pass, once the whole layer is read.
	v.village(&vc)
}

// standable is tiled.go:1103-1114: a person off the map or on a tile nobody can
// stand on would be put there anyway, stuck, and the map would look right in
// the editor.
func (v *validation) standable(what string, o Object) {
	t := o.Tile()

	if o.X < 0 || o.Y < 0 || !v.doc.m.onMap(t.X, t.Y) {
		v.add(Problem{Rule: RuleOffMap, Object: o.ID, Msg: fmt.Sprintf("%s at tile %.2f,%.2f is off the %dx%d map",
			what, o.X, o.Y, v.doc.m.width, v.doc.m.height)})

		return
	}

	if v.doc.Blocked(t.X, t.Y) {
		p := Problem{Rule: RuleStandable, Object: o.ID, Msg: fmt.Sprintf("%s stands on a tile which is blocked or has no floor", what)}
		v.tile(p, t.X, t.Y)
	}
}

// inside is tiled.go:1047-1077.
func (v *validation) inside(o Object) {
	if len(o.props) > 0 {
		v.add(Problem{Rule: RuleObjectProps, Object: o.ID, Msg: "inside takes no properties"})
	}

	switch {
	case o.Rotation != 0:
		v.add(Problem{Rule: RuleInside, Object: o.ID, Msg: "inside is rotated; the game reads it unrotated -- draw it square to the tiles"})
	case o.Ellipse:
		v.add(Problem{Rule: RuleInside, Object: o.ID, Msg: "inside is an ellipse; the game reads rectangles only"})
	}

	if raw, ok := v.doc.rawObject(o.ID); ok {
		for _, key := range []string{"polygon", "polyline"} {
			if val, has := raw.Get(key); has && val != nil {
				v.add(Problem{Rule: RuleInside, Object: o.ID, Msg: "inside is a polygon; the game reads rectangles only"})
			}
		}
	}

	if o.W <= 0 || o.H <= 0 {
		v.add(Problem{Rule: RuleInside, Object: o.ID, Msg: "inside has no area; draw it as a rectangle"})

		return
	}

	if !o.Rect.In(v.doc.bounds()) {
		v.add(Problem{Rule: RuleOffMap, Object: o.ID, Msg: fmt.Sprintf("inside %s reaches off the %dx%d map",
			o.Rect, v.doc.m.width, v.doc.m.height)})
	}
}

// structure is tiled.go:977-1042, in the loader's own order.
func (v *validation) structure(o Object) {
	if o.Class != "" && o.Class != ClassStructure {
		v.add(Problem{Rule: RuleObjectClass, Object: o.ID, Msg: fmt.Sprintf(
			"is a tile object with class %q; tile objects are structures -- people are point objects", o.Class)})
	}

	if len(o.props) > 0 || o.Rotation != 0 {
		v.add(Problem{Rule: RuleObjectProps, Object: o.ID, Msg: "structure takes no properties and no rotation"})
	}

	if uint32(o.GID)&gidFlipMask != 0 {
		v.add(Problem{Rule: RuleTileFlipped, Object: o.ID, Msg: "structure is flipped; the game draws art as it is"})

		return
	}

	k, ok := v.doc.Kind(o.GID)
	if !ok {
		v.add(Problem{Rule: RuleUnknownTile, Object: o.ID, Msg: fmt.Sprintf("tile id %d is in no tileset", o.GID)})

		return
	}

	if k.PropertyError() != nil {
		return // reported against the kind
	}

	if k.Footprint == (image.Point{}) {
		v.add(Problem{Rule: RuleFootprint, Object: o.ID, Kind: k.Name,
			Msg: "is placed as a tile object but has no footprint_w/footprint_h; only structures are tile objects"})

		return
	}

	v.checkArtFor(k, ClassStructure, Problem{Object: o.ID, Kind: k.Name})

	if o.X != float64(int(o.X)) || o.Y != float64(int(o.Y)) {
		v.add(Problem{Rule: RuleOffGrid, Object: o.ID, Msg: fmt.Sprintf(
			"structure at %.2f,%.2f is off the tile grid; turn on snapping", o.X, o.Y)})
	}

	// The editor size against the art, in the PIXELS the file stores rather
	// than the tiles the read model reports, because that is what the loader
	// compares (tiled.go:1008-1011).
	if size, known := v.artSizeOf(k); known {
		if raw, found := v.doc.rawObject(o.ID); found {
			pw, ph := fieldFloat(raw, "width"), fieldFloat(raw, "height")

			if (pw != 0 || ph != 0) && (int(pw) != size.X || int(ph) != size.Y) {
				v.add(Problem{Rule: RuleObjectSize, Object: o.ID, Msg: fmt.Sprintf(
					"structure is sized %.0fx%.0f in the editor but its art is %dx%d; reset its size (Tiled stretches tile objects, the game does not)",
					pw, ph, size.X, size.Y)})
			}
		}
	}

	if !o.Footprint.In(v.doc.bounds()) {
		v.add(Problem{Rule: RuleOffMap, Object: o.ID, Msg: fmt.Sprintf("structure %s reaches off the %dx%d map",
			o.Footprint, v.doc.m.width, v.doc.m.height)})

		return
	}

	// What the footprint stands on, tile by tile: another structure, a wall, or
	// bare ground (tiled.go:1022-1037).
	for y := o.Footprint.Min.Y; y < o.Footprint.Max.Y; y++ {
		for x := o.Footprint.Min.X; x < o.Footprint.Max.X; x++ {
			if other, on := v.doc.StructureOn(x, y); on && other.ID != o.ID {
				v.tile(Problem{Rule: RuleOverlap, Object: o.ID,
					Msg: fmt.Sprintf("structure overlaps structure %d", other.ID)}, x, y)
			}

			if v.doc.WallTile(x, y) != 0 {
				v.tile(Problem{Rule: RuleOverlap, Object: o.ID, Msg: "structure overlaps a wall"}, x, y)
			}

			if v.doc.FloorTile(x, y) == 0 {
				v.tile(Problem{Rule: RuleBareGround, Object: o.ID, Msg: "structure stands over bare ground"}, x, y)
			}
		}
	}
}

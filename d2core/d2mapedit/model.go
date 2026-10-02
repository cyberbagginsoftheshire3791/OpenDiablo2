package d2mapedit

import (
	"errors"
	"fmt"
	"image"
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// The loader's own bounds. d2maptiled keeps them unexported (tiled.go:112-120,
// :141, :845-847), and the validator's whole job is to answer the loader's
// question before the loader is asked, so they are repeated here -- and
// TestTheLoaderAgreesAboutEveryBound measures every one of them against a real
// d2maptiled.Parse rather than trusting this block.
const (
	maxWallHeight      = 512
	maxStructureHeight = 768
	maxSide            = 1024
	maxFootprint       = 16
	// artUnit is the width one tile of a structure's footprint adds to its
	// art: (footprint_w + footprint_h) * artUnit (tiled.go:811).
	artUnit = d2maptiled.TileWidth / 2
	// tileW and tileH are the engine's only tile size, from the loader so the
	// two cannot disagree (tiled.go:106-107).
	tileW = d2maptiled.TileWidth
	tileH = d2maptiled.TileHeight
)

// The object classes the loader reads, and nothing else (tiled.go:931-963).
// A structure is a TILE object, whose class is empty or "structure"
// (tiled.go:983).
const (
	ClassPlayerStart = "player_start"
	ClassNPC         = "npc"
	ClassInside      = "inside"
	ClassStructure   = "structure"
)

// The layer names the loader reads (tiled.go:556-565).
const (
	LayerFloor   = "floor"
	LayerWalls   = "walls"
	LayerObjects = "objects"
)

// Legal says which layers a kind may be placed on, by the loader's art and
// footprint rules. A kind with a footprint is a structure and NOTHING else
// (tiled.go:689-694); a kind without one is a floor or a wall according to the
// size of its art (tiled.go:807-833).
type Legal struct {
	Floor, Wall, Structure bool
}

// Any reports whether the kind may be placed at all.
func (l Legal) Any() bool {
	return l.Floor || l.Wall || l.Structure
}

// On reports whether the kind may be placed on the named layer ("floor",
// "walls") or as a structure ("structure").
func (l Legal) On(layer string) bool {
	switch layer {
	case LayerFloor:
		return l.Floor
	case LayerWalls:
		return l.Wall
	case ClassStructure:
		return l.Structure
	}

	return false
}

// LegalLayers works out where art of this size and footprint may go. It is the
// palette's rule and the validator's rule, from one place: checkArt
// (tiled.go:807-833) read backwards.
func LegalLayers(footprint image.Point, w, h int) Legal {
	if footprint != (image.Point{}) {
		want := (footprint.X + footprint.Y) * artUnit

		return Legal{Structure: w == want && h >= d2maptiled.TileHeight && h <= maxStructureHeight}
	}

	return Legal{
		Floor: w == d2maptiled.TileWidth && h == d2maptiled.TileHeight,
		Wall:  w == d2maptiled.TileWidth && h >= d2maptiled.TileHeight && h <= maxWallHeight,
	}
}

// Kind is one tileset tile as the editor's palette sees it.
type Kind struct {
	// GID is the global tile id: what a tile layer's data and a tile object's
	// "gid" hold, and what the edit operations take.
	GID int
	// Name is "<tileset>#<local id>", the name the loader puts in its
	// messages (tiled.go:679).
	Name    string
	Tileset string
	LocalID int
	// Image is the path exactly as the .tmj writes it, relative to the
	// directory the map is in (tiled.go:777).
	Image string
	// Sheet is true when Image is a sprite sheet this tile is one cell of,
	// rather than the tile's own image.
	Sheet bool
	// Cell is the tile's rectangle in the sheet (Sheet only).
	Cell image.Rectangle
	// Declared is the art size the .tmj says this tile's image has. It is
	// what Tiled draws with, and the editor's palette uses it; the loader
	// reads the PNG instead, so the validator checks the two agree.
	Declared    image.Point
	Footprint   image.Point
	Blocked     bool
	BlocksSight bool
	// Height is a floor tile's ground height and SightRadius a tower's sight
	// (fog of war F4; tiled.go's Kind.Height and Kind.SightRadius).
	Height      int
	SightRadius int
	// Building is whether a household's door may be beside it (the raid's
	// R3a; tiled.go's Kind.Building).
	Building bool
	// Legal is worked out from Declared, so it is advisory: Validate settles
	// it from the PNG's own header.
	Legal Legal

	propErr error
}

// PropertyError is what the loader would say about this tile's properties, or
// nil. Validate reports it too; it is here so a palette can grey the tile out.
func (k Kind) PropertyError() error {
	return k.propErr
}

// Object is one object on the "objects" layer.
type Object struct {
	ID   int
	Name string
	// Class is what the file says the object is, from whichever of "class" and
	// "type" it uses. Empty on a structure that names no class, which the
	// loader allows (tiled.go:983).
	Class string
	// GID is non-zero exactly for a TILE object, which the loader reads as a
	// structure and nothing else (tiled.go:908, :984).
	GID int
	// X, Y are in WORLD TILES: the pixels the file stores divided by the map's
	// tile height, on both axes, which is how the loader reads them
	// (tiled.go:928). They are not whole numbers for a person standing in the
	// middle of his tile.
	X, Y float64
	// W, H are the object's size in world tiles, 0 for a point.
	W, H float64
	// Monstat is an npc's monstats record (tiled.go:1079-1098).
	Monstat string
	// Household is the household an npc names, "" for none (the raid's R3a;
	// d2maptiled's NPC.Household).
	Household string
	// Members, Incense, Stakes and Church are a household's properties, and
	// Post a watch post's (the raid's R3a; d2maptiled/households.go).
	Members         []string
	Incense, Stakes int
	Church          bool
	Post            string
	// Footprint is a structure's footprint in whole tiles, Max exclusive,
	// laid back from its bottom corner (tiled.go:1013-1014).
	Footprint image.Rectangle
	// Rect is an inside area in whole tiles, Max exclusive (tiled.go:1067).
	Rect     image.Rectangle
	Rotation float64
	Ellipse  bool
	Point    bool

	monstatErr error
	// propErr is what the loader would say about a household's or a watch
	// post's properties, or nil (the raid's R3a).
	propErr error
	props   []any
}

// IsStructure reports whether the object is a tile object, which is the only
// thing the loader reads as a structure.
func (o Object) IsStructure() bool {
	return o.GID != 0
}

// Tile is the whole tile the object stands in, the way the engine places a
// person (tiled.go:1104).
func (o Object) Tile() image.Point {
	return image.Pt(int(o.X), int(o.Y))
}

// model is the typed read model, rebuilt from the tree after every edit so the
// two can never disagree.
type model struct {
	width, height         int
	tileWidth, tileHeight int

	floor []int // gids, row-major; 0 is no tile
	walls []int // nil when the map has no "walls" layer

	kinds   []Kind
	byGID   map[int]int
	objects []Object

	// cover is the structure standing on each tile as its index in objects
	// PLUS ONE, 0 for none -- the loader's own convention (tiled.go:173-176).
	cover []int

	soundEnv    int
	displayName string
	note        string
	groups      []Group
	groupsErr   error

	// classKey is "type" or "class", whichever the file already uses for its
	// objects, so a placed structure is written the way its neighbours are.
	classKey string
}

func (m *model) at(x, y int) int {
	return x + y*m.width
}

func (m *model) onMap(x, y int) bool {
	return x >= 0 && y >= 0 && x < m.width && y < m.height
}

// derive rebuilds the READ MODEL, WHOLE, from the tree, after every edit. That
// is deliberately the blunt choice: it makes it impossible for the model and the
// tree to disagree, which is the bug class that would otherwise eat this package
// (an edit that updates one and not the other, found weeks later as a house
// drawn where the file does not have one). The cost is a rebuild per edit --
// 16 kinds, 13 objects and a Width*Height coverage map for the village. If a
// brush dragged across a 1024x1024 map ever stutters, the fix is an incremental
// path for the tile layers only, measured, not a second copy of the model.
//
// IT ALSO CANNOT FAIL: Open has
// already refused a tree the model could not be built over, and everything a
// hand-edit can get wrong inside that shape -- an unknown class, a property of
// the wrong type, an off-grid structure -- is carried as a value the validator
// reports, not as an error that would leave the document unopenable.
func (d *Doc) derive() {
	m := model{
		width:      fieldInt(d.tree, "width"),
		height:     fieldInt(d.tree, "height"),
		tileWidth:  fieldInt(d.tree, "tilewidth"),
		tileHeight: fieldInt(d.tree, "tileheight"),
		byGID:      map[int]int{},
		classKey:   "type",
	}

	m.deriveKinds(d.tree)
	m.deriveTiles(d.tree)
	m.deriveObjects(d.tree)
	m.deriveProperties(d.tree)

	d.m = m
}

func (m *model) deriveKinds(tree *jsonObject) {
	for _, raw := range fieldArray(tree, "tilesets") {
		ts, ok := asObject(raw)
		if !ok {
			continue
		}

		first := fieldInt(ts, "firstgid")
		name := fieldString(ts, "name")
		props := map[int][]any{}
		declared := map[int]image.Point{}
		images := map[int]string{}
		locals := []int{}

		for _, tileRaw := range fieldArray(ts, "tiles") {
			tile, ok := asObject(tileRaw)
			if !ok {
				continue
			}

			id := fieldInt(tile, "id")
			locals = append(locals, id)
			props[id] = fieldArray(tile, "properties")
			images[id] = fieldString(tile, "image")
			declared[id] = image.Pt(fieldInt(tile, "imagewidth"), fieldInt(tile, "imageheight"))
		}

		sheet := fieldString(ts, "image")
		if sheet != "" {
			locals = locals[:0]
			for id := 0; id < fieldInt(ts, "tilecount"); id++ {
				locals = append(locals, id)
			}
		}

		for _, id := range locals {
			k := Kind{
				GID:     first + id,
				Name:    fmt.Sprintf("%s#%d", name, id),
				Tileset: name,
				LocalID: id,
				Image:   images[id],
			}

			if sheet != "" {
				k.Sheet = true
				k.Image = sheet
				k.Declared = image.Pt(fieldInt(ts, "tilewidth"), fieldInt(ts, "tileheight"))
				k.Cell = sheetCell(ts, id)
			} else {
				k.Declared = declared[id]
			}

			p, err := parseKindProps(props[id])
			k.Blocked, k.BlocksSight, k.Footprint, k.propErr = p.blocked, p.blocksSight, p.footprint, err
			k.Height, k.SightRadius, k.Building = p.height, p.sightRadius, p.building
			k.Legal = LegalLayers(k.Footprint, k.Declared.X, k.Declared.Y)

			if _, clash := m.byGID[k.GID]; !clash {
				m.byGID[k.GID] = len(m.kinds)
				m.kinds = append(m.kinds, k)
			}
		}
	}
}

// sheetCell is the rectangle a sheet tileset cuts a tile from (tiled.go:756-759).
func sheetCell(ts *jsonObject, local int) image.Rectangle {
	cols := fieldInt(ts, "columns")
	if cols <= 0 {
		return image.Rectangle{}
	}

	tw, th := fieldInt(ts, "tilewidth"), fieldInt(ts, "tileheight")
	margin, spacing := fieldInt(ts, "margin"), fieldInt(ts, "spacing")
	col, row := local%cols, local/cols
	x := margin + col*(tw+spacing)
	y := margin + row*(th+spacing)

	return image.Rect(x, y, x+tw, y+th)
}

// kindProps is what the loader reads off a tileset tile (tiled.go:835-900).
type kindProps struct {
	blocked     bool
	blocksSight bool
	footprint   image.Point
	height      int  // fog of war F4: a floor tile's ground height
	sightRadius int  // fog of war F4: a tower's sight
	building    bool // the raid's R3a: a household's door may be beside it
}

// parseKindProps is the loader's Kind.properties, to the letter: the same four
// names, the same refusals, the same defaults. It returns what the loader would
// have built AND what it would have said, so the palette and the validator
// read one implementation.
func parseKindProps(props []any) (kindProps, error) {
	var (
		out                               kindProps
		sightSet, blockedSet, buildingSet bool
	)

	for _, raw := range props {
		p, ok := asObject(raw)
		if !ok {
			return out, errors.New("a tile property is not a JSON object")
		}

		name := fieldString(p, "name")
		typ := fieldString(p, "type")
		value, _ := p.Get("value")

		switch name {
		case "blocked", "blocks_sight":
			v, isBool := asBool(value)
			if typ != "bool" || !isBool {
				return out, fmt.Errorf("property %q must be a bool", name)
			}

			if name == "blocked" {
				out.blocked, blockedSet = v, true
			} else {
				out.blocksSight, sightSet = v, true
			}
		case "footprint_w", "footprint_h":
			n, isInt := asInt(value)
			if typ != "int" || !isInt || n < 1 || n > maxFootprint {
				return out, fmt.Errorf("property %q must be an int from 1 to %d", name, maxFootprint)
			}

			if name == "footprint_w" {
				out.footprint.X = n
			} else {
				out.footprint.Y = n
			}
		case "height":
			n, isInt := asInt(value)
			if typ != "int" || !isInt || n < 0 || n > d2maptiled.MaxHeight {
				return out, fmt.Errorf("property \"height\" must be an int from 0 to %d", d2maptiled.MaxHeight)
			}

			out.height = n
		case "sight_radius":
			n, isInt := asInt(value)
			if typ != "int" || !isInt || n < 1 || n > d2maptiled.MaxSightRadius {
				return out, fmt.Errorf("property \"sight_radius\" must be an int from 1 to %d", d2maptiled.MaxSightRadius)
			}

			out.sightRadius = n
		case "building":
			v, isBool := asBool(value)
			if typ != "bool" || !isBool {
				return out, errors.New("property \"building\" must be a bool")
			}

			out.building, buildingSet = v, true
		default:
			return out, fmt.Errorf("unknown tile property %q; the game reads %s", name, d2maptiled.TilePropertyNames)
		}
	}

	if (out.footprint.X == 0) != (out.footprint.Y == 0) {
		return out, errors.New("a structure needs both footprint_w and footprint_h")
	}

	// Raised sight (fog of war F4; tiled.go's Kind.properties, to the letter).
	switch {
	case out.sightRadius != 0 && out.footprint == (image.Point{}):
		return out, errors.New("\"sight_radius\" makes a structure a tower; this tile has no footprint_w/footprint_h")
	case out.sightRadius != 0 && out.footprint != image.Pt(1, 1):
		return out, fmt.Errorf("a tower is 1x1 for now (it sees from its one tile); this one is %dx%d", out.footprint.X, out.footprint.Y)
	case out.height != 0 && out.footprint != (image.Point{}):
		return out, errors.New("\"height\" is the ground's, a floor tile's property; a structure cannot carry it")
	}

	if out.footprint != (image.Point{}) {
		if out.footprint.X != out.footprint.Y {
			return out, fmt.Errorf("a %dx%d footprint is not square; structures are square for now", out.footprint.X, out.footprint.Y)
		}

		if blockedSet && !out.blocked {
			return out, errors.New("a structure cannot be blocked=false; its footprint is solid")
		}

		out.blocked = true
	}

	// The raid's R3a (tiled.go's Kind.properties): a structure is a building
	// unless it says otherwise or is a tower; a building is solid.
	if !buildingSet {
		out.building = out.footprint != (image.Point{}) && out.sightRadius == 0
	}

	if out.building && !out.blocked {
		return out, errors.New("\"building\" marks a solid building; this tile is not blocked")
	}

	if !sightSet {
		out.blocksSight = out.blocked || out.footprint != (image.Point{})
	}

	return out, nil
}

func (m *model) deriveTiles(tree *jsonObject) {
	m.floor = make([]int, m.width*m.height)

	for _, raw := range fieldArray(tree, "layers") {
		l, ok := asObject(raw)
		if !ok {
			continue
		}

		if fieldString(l, "type") != "tilelayer" {
			continue
		}

		into := m.floor

		switch fieldString(l, "name") {
		case LayerFloor:
		case LayerWalls:
			m.walls = make([]int, m.width*m.height)
			into = m.walls
		default:
			continue
		}

		for i, cell := range fieldArray(l, "data") {
			if i >= len(into) {
				break
			}

			gid, _ := asInt(cell)
			into[i] = gid
		}
	}
}

func (m *model) deriveObjects(tree *jsonObject) {
	layer, ok := findLayer(tree, LayerObjects)
	if !ok {
		return
	}

	raws := fieldArray(layer, "objects")

	// Which key the file uses for an object's class. Tiled 1.9 renamed "type"
	// to "class" and still reads both; tools/villagemap wrote "type", which is
	// what village.tmj holds. A placed structure follows its neighbours.
	for _, raw := range raws {
		if o, ok := asObject(raw); ok {
			if _, has := o.Get("class"); has {
				m.classKey = "class"

				break
			}
		}
	}

	m.cover = make([]int, m.width*m.height)

	th := float64(m.tileHeight)
	if th == 0 {
		th = 1
	}

	for _, raw := range raws {
		o, ok := asObject(raw)
		if !ok {
			continue
		}

		obj := Object{
			ID:       fieldInt(o, "id"),
			Name:     fieldString(o, "name"),
			GID:      fieldInt(o, "gid"),
			X:        fieldFloat(o, "x") / th,
			Y:        fieldFloat(o, "y") / th,
			W:        fieldFloat(o, "width") / th,
			H:        fieldFloat(o, "height") / th,
			Rotation: fieldFloat(o, "rotation"),
			Ellipse:  fieldBool(o, "ellipse"),
			Point:    fieldBool(o, "point"),
			props:    fieldArray(o, "properties"),
		}

		obj.Class = fieldString(o, "class")
		if obj.Class == "" {
			obj.Class = fieldString(o, "type")
		}

		switch {
		case obj.IsStructure():
			if k, found := m.kind(obj.GID); found {
				obj.Footprint = footprintAt(k.Footprint, obj.X, obj.Y)
			}
		case obj.Class == ClassNPC:
			obj.Monstat, obj.Household, obj.monstatErr = npcProperties(obj.props)
		case obj.Class == ClassHousehold:
			obj.propErr = householdProperties(&obj)
		case obj.Class == ClassWatchPost:
			obj.Post, obj.propErr = postProperty(obj.props)
		case obj.Class == ClassInside:
			obj.Rect = insideRect(obj.X, obj.Y, obj.W, obj.H)
		}

		index := len(m.objects)
		m.objects = append(m.objects, obj)

		// First structure on a tile wins, because the loader refuses the
		// second one (tiled.go:1027-1028) and the validator says so.
		if obj.IsStructure() {
			m.markCover(obj.Footprint, index+1)
		}
	}
}

func (m *model) markCover(r image.Rectangle, mark int) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if !m.onMap(x, y) || m.cover[m.at(x, y)] != 0 {
				continue
			}

			m.cover[m.at(x, y)] = mark
		}
	}
}

// footprintAt lays a footprint back from the bottom corner a tile object sits
// on (tiled.go:1001, :1013-1014), rounding the way the loader rounds.
func footprintAt(fp image.Point, x, y float64) image.Rectangle {
	fx, fy := int(math.Round(x)), int(math.Round(y))

	return image.Rect(fx-fp.X, fy-fp.Y, fx, fy)
}

// insideRect is every tile an inside rectangle touches (tiled.go:1067-1070).
func insideRect(x, y, w, h float64) image.Rectangle {
	return image.Rect(
		int(math.Floor(x)), int(math.Floor(y)),
		int(math.Ceil(x+w)), int(math.Ceil(y+h)),
	)
}

func (m *model) deriveProperties(tree *jsonObject) {
	for _, raw := range fieldArray(tree, "properties") {
		p, ok := asObject(raw)
		if !ok {
			continue
		}

		switch fieldString(p, "name") {
		case "note":
			m.note, m.groups, m.groupsErr = decodeNote(fieldString(p, "value"))
		case "sound_env":
			m.soundEnv = fieldInt(p, "value")
		case "display_name":
			m.displayName = fieldString(p, "value")
		}
	}
}

func (m *model) kind(gid int) (Kind, bool) {
	i, ok := m.byGID[gid]
	if !ok {
		return Kind{}, false
	}

	return m.kinds[i], true
}

func findLayer(tree *jsonObject, name string) (*jsonObject, bool) {
	for _, raw := range fieldArray(tree, "layers") {
		l, ok := asObject(raw)
		if !ok {
			continue
		}

		if fieldString(l, "name") == name {
			return l, true
		}
	}

	return nil, false
}

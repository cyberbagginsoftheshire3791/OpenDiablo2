package d2mapedit

import (
	"errors"
	"fmt"
	"image"
	"path"
	"strings"
)

// THE EDIT OPERATIONS.
//
// Each one returns a Cmd and changes NOTHING until the command is run, so the
// caller -- normally a Stack -- decides whether the edit joins the history. A
// command built from one state and run against a later one is the caller's
// problem to avoid; run them promptly, which is what a screen does anyway.
//
// WHAT THEY REFUSE AND WHAT THEY ALLOW. An operation refuses only what it
// cannot represent: a tile id no tileset holds, a kind placed on a layer its
// art can never be legal on, a tile off the map. It does NOT refuse a house
// that overlaps another house, a start on a blocked tile or a village sealed by
// a fence -- an editor whose every intermediate state must be valid is an
// editor nobody can use. Validate answers that question, and Save asks it.

// PlaceStructure puts a structure on the map, its footprint's BOTTOM CORNER at
// x, y in whole tiles -- where Tiled draws a tile object and where the game
// stands it (tiled.go:1013-1014). The footprint runs back from there:
// image.Rect(x-w, y-h, x, y).
//
// The object takes the next id from "nextobjectid" and bumps it, the way Tiled
// does, so Tiled and the editor never hand out the same id twice.
func (d *Doc) PlaceStructure(gid, x, y int) (Cmd, error) {
	k, ok := d.Kind(gid)
	if !ok {
		return nil, fmt.Errorf("no tile with id %d in this map's tilesets", gid)
	}

	if k.PropertyError() != nil {
		return nil, fmt.Errorf("%s: %w", k.Name, k.PropertyError())
	}

	if k.Footprint == (image.Point{}) {
		return nil, fmt.Errorf("%s has no footprint_w/footprint_h, so it is not a structure; place it on the floor or walls layer", k.Name)
	}

	r := footprintAt(k.Footprint, float64(x), float64(y))
	if !r.In(d.bounds()) {
		return nil, fmt.Errorf("a %dx%d structure with its bottom corner at %d,%d reaches off the %dx%d map",
			k.Footprint.X, k.Footprint.Y, x, y, d.m.width, d.m.height)
	}

	return &placeStructureCmd{gid: gid, x: x, y: y}, nil
}

func (d *Doc) bounds() image.Rectangle {
	return image.Rect(0, 0, d.m.width, d.m.height)
}

type placeStructureCmd struct {
	gid  int
	x, y int
	// id is the object id allocated the first time the command runs, so a redo
	// puts the same object back rather than eating another id.
	id int
}

func (c *placeStructureCmd) Label() string {
	return fmt.Sprintf("place structure %d at %d,%d", c.gid, c.x, c.y)
}

func (c *placeStructureCmd) Do(d *Doc) error {
	layer, ok := findLayer(d.tree, LayerObjects)
	if !ok {
		return errNoObjectLayer
	}

	k, found := d.Kind(c.gid)
	if !found {
		return fmt.Errorf("no tile with id %d in this map's tilesets", c.gid)
	}

	if c.id == 0 {
		next := d.NextObjectID()
		if next <= 0 {
			return errors.New("the map's \"nextobjectid\" is missing or not a positive number, so a new object cannot be given an id Tiled will respect")
		}

		c.id = next
	}

	if _, clash := d.Object(c.id); clash {
		return fmt.Errorf("object %d already exists", c.id)
	}

	th := float64(d.m.tileHeight)

	o := newJSONObject()
	o.Set("id", num(c.id))
	o.Set("name", StructureName(k))
	o.Set(d.m.classKey, ClassStructure)
	o.Set("x", fnum(float64(c.x)*th))
	o.Set("y", fnum(float64(c.y)*th))

	// The loader refuses a tile object whose editor size disagrees with its art
	// (tiled.go:1008-1011). Tiled writes the art's size; so do we, from what the
	// .tmj declares, and Validate checks that declaration against the PNG.
	o.Set("width", num(k.Declared.X))
	o.Set("height", num(k.Declared.Y))
	o.Set("rotation", num(0))
	o.Set("visible", true)
	o.Set("point", false)
	o.Set("gid", num(c.gid))

	layer.Set("objects", append(fieldArray(layer, "objects"), o))
	d.tree.Set("nextobjectid", num(maxInt(d.NextObjectID(), c.id+1)))
	d.derive()

	return nil
}

func (c *placeStructureCmd) Undo(d *Doc) error {
	if err := d.removeObject(c.id); err != nil {
		return err
	}

	// The id goes back ONLY if this placement was the last one to take one, so
	// an undone placement does not leak an id and an out-of-order undo cannot
	// hand the same id out twice. An id is cheap; a duplicate is not.
	if d.NextObjectID() == c.id+1 {
		d.tree.Set("nextobjectid", num(c.id))
	}

	d.derive()

	return nil
}

var errNoObjectLayer = fmt.Errorf("the map has no %q layer", LayerObjects)

// StructureName is the name a placed structure is written with: its KIND, the
// way the village's own records name theirs -- "peasant-house",
// "burned-house" -- and the way tools/villagemap writes them.
//
// It is read off the art's path, because that is where the kind is: a
// structure's picture lives at structures/<kind>/<state>.png, so the directory
// is the kind and the file is its state. Art that does not follow that shape
// (a tile under tiles/) is named for its file, without the extension. Only a
// kind with no art at all falls back to the loader's "<tileset>#<id>" -- which
// is what EVERY placed structure was called until the 28 Sep review
// ("village-placeholder#16" beside the village's "peasant-house").
func StructureName(k Kind) string {
	img := strings.ReplaceAll(k.Image, "\\", "/")
	if strings.TrimSpace(img) == "" {
		return k.Name
	}

	img = path.Clean(img)

	if dir := path.Base(path.Dir(img)); dir != "." && dir != ".." && dir != "/" && dir != "tiles" {
		return dir
	}

	return strings.TrimSuffix(path.Base(img), path.Ext(img))
}

// MoveObject moves any object to toX, toY in WORLD TILES -- the unit the read
// model reports and the unit the loader divides pixels into (tiled.go:928).
// Fractions are kept: a villager standing in the middle of his tile is moved a
// whole tile by adding 1, not by snapping him to a corner.
func (d *Doc) MoveObject(id int, toX, toY float64) (Cmd, error) {
	o, ok := d.Object(id)
	if !ok {
		return nil, fmt.Errorf("no object %d", id)
	}

	return &moveObjectCmd{id: id, toX: toX, toY: toY, what: o.Class}, nil
}

type moveObjectCmd struct {
	id           int
	toX, toY     float64
	what         string
	fromX, fromY float64
	captured     bool
}

func (c *moveObjectCmd) Label() string {
	what := c.what
	if what == "" {
		what = "object"
	}

	return fmt.Sprintf("move %s %d to %g,%g", what, c.id, c.toX, c.toY)
}

func (c *moveObjectCmd) Do(d *Doc) error {
	o, ok := d.rawObject(c.id)
	if !ok {
		return fmt.Errorf("no object %d", c.id)
	}

	th := float64(d.m.tileHeight)

	if !c.captured {
		c.fromX, c.fromY = fieldFloat(o, "x")/th, fieldFloat(o, "y")/th
		c.captured = true
	}

	o.Set("x", fnum(c.toX*th))
	o.Set("y", fnum(c.toY*th))
	d.derive()

	return nil
}

func (c *moveObjectCmd) Undo(d *Doc) error {
	o, ok := d.rawObject(c.id)
	if !ok {
		return fmt.Errorf("no object %d", c.id)
	}

	th := float64(d.m.tileHeight)
	o.Set("x", fnum(c.fromX*th))
	o.Set("y", fnum(c.fromY*th))
	d.derive()

	return nil
}

// DeleteObject removes an object. The whole JSON object is kept by the command,
// so an undo puts back everything it had -- its name, its properties, the keys
// this package has never heard of -- and not a reconstruction of it.
func (d *Doc) DeleteObject(id int) (Cmd, error) {
	o, ok := d.Object(id)
	if !ok {
		return nil, fmt.Errorf("no object %d", id)
	}

	what := o.Class
	if what == "" && o.IsStructure() {
		what = ClassStructure
	}

	return &deleteObjectCmd{id: id, what: what}, nil
}

type deleteObjectCmd struct {
	id    int
	what  string
	had   *jsonObject
	index int
}

func (c *deleteObjectCmd) Label() string {
	what := c.what
	if what == "" {
		what = "object"
	}

	return fmt.Sprintf("delete %s %d", what, c.id)
}

func (c *deleteObjectCmd) Do(d *Doc) error {
	raw, ok := d.rawObject(c.id)
	if !ok {
		return fmt.Errorf("no object %d", c.id)
	}

	c.had = raw.Clone()
	c.index = d.rawObjectIndex(c.id)

	if err := d.removeObject(c.id); err != nil {
		return err
	}

	d.derive()

	return nil
}

func (c *deleteObjectCmd) Undo(d *Doc) error {
	layer, ok := findLayer(d.tree, LayerObjects)
	if !ok {
		return errNoObjectLayer
	}

	objects := fieldArray(layer, "objects")
	at := c.index

	if at < 0 || at > len(objects) {
		at = len(objects)
	}

	// Back where it was, because a structure's file order decides which of two
	// overlapping ones the loader keeps (tiled.go:1027) and which the editor
	// draws on top.
	restored := make([]any, 0, len(objects)+1)
	restored = append(restored, objects[:at]...)
	restored = append(restored, c.had.Clone())
	restored = append(restored, objects[at:]...)

	layer.Set("objects", restored)
	d.derive()

	return nil
}

// SetFloorTile puts a tile id on the floor layer at x, y. 0 clears it, leaving
// bare ground, which the loader reads as blocked (tiled.go:242-244).
func (d *Doc) SetFloorTile(x, y, gid int) (Cmd, error) {
	return d.setTile(LayerFloor, x, y, gid)
}

// SetWallTile puts a tile id on the walls layer at x, y. 0 clears it.
//
// A map with no "walls" layer is refused rather than given one: the loader
// treats the layer as optional, and inventing a layer with an id out of
// "nextlayerid" behind the designer's back is the kind of surprise this package
// is meant not to spring. Add the layer in Tiled.
func (d *Doc) SetWallTile(x, y, gid int) (Cmd, error) {
	if !d.HasWalls() {
		return nil, fmt.Errorf("this map has no %q tile layer; add one in Tiled", LayerWalls)
	}

	return d.setTile(LayerWalls, x, y, gid)
}

func (d *Doc) setTile(layerName string, x, y, gid int) (Cmd, error) {
	if !d.m.onMap(x, y) {
		return nil, fmt.Errorf("tile %d,%d is off the %dx%d map", x, y, d.m.width, d.m.height)
	}

	if gid != 0 {
		k, ok := d.Kind(gid)
		if !ok {
			return nil, fmt.Errorf("no tile with id %d in this map's tilesets", gid)
		}

		if k.PropertyError() != nil {
			return nil, fmt.Errorf("%s: %w", k.Name, k.PropertyError())
		}

		if k.Footprint != (image.Point{}) {
			return nil, fmt.Errorf("%s is a structure (it has a footprint); place it as a tile object with PlaceStructure, not on the %s layer", k.Name, layerName)
		}

		if !k.Legal.On(layerName) {
			return nil, fmt.Errorf("%s's art is %dx%d, which the game will not take on the %s layer: %s",
				k.Name, k.Declared.X, k.Declared.Y, layerName, artRule(layerName))
		}
	}

	return &setTileCmd{layer: layerName, x: x, y: y, gid: gid}, nil
}

type setTileCmd struct {
	layer    string
	x, y     int
	gid      int
	had      int
	captured bool
}

func (c *setTileCmd) Label() string {
	if c.gid == 0 {
		return fmt.Sprintf("clear %s tile %d,%d", c.layer, c.x, c.y)
	}

	return fmt.Sprintf("set %s tile %d,%d to %d", c.layer, c.x, c.y, c.gid)
}

func (c *setTileCmd) Do(d *Doc) error {
	if !c.captured {
		switch c.layer {
		case LayerWalls:
			c.had = d.WallTile(c.x, c.y)
		default:
			c.had = d.FloorTile(c.x, c.y)
		}

		c.captured = true
	}

	return d.writeTile(c.layer, c.x, c.y, c.gid)
}

func (c *setTileCmd) Undo(d *Doc) error {
	return d.writeTile(c.layer, c.x, c.y, c.had)
}

func (d *Doc) writeTile(layerName string, x, y, gid int) error {
	layer, ok := findLayer(d.tree, layerName)
	if !ok {
		return fmt.Errorf("the map has no %q layer", layerName)
	}

	data := fieldArray(layer, "data")
	i := d.m.at(x, y)

	if i < 0 || i >= len(data) {
		return fmt.Errorf("tile %d,%d is off the %dx%d map", x, y, d.m.width, d.m.height)
	}

	data[i] = num(gid)
	layer.Set("data", data)
	d.derive()

	return nil
}

// ---- helpers over the tree ------------------------------------------------

func (d *Doc) rawObject(id int) (*jsonObject, bool) {
	layer, ok := findLayer(d.tree, LayerObjects)
	if !ok {
		return nil, false
	}

	for _, raw := range fieldArray(layer, "objects") {
		o, ok := asObject(raw)
		if ok && fieldInt(o, "id") == id {
			return o, true
		}
	}

	return nil, false
}

func (d *Doc) rawObjectIndex(id int) int {
	layer, ok := findLayer(d.tree, LayerObjects)
	if !ok {
		return -1
	}

	for i, raw := range fieldArray(layer, "objects") {
		if o, ok := asObject(raw); ok && fieldInt(o, "id") == id {
			return i
		}
	}

	return -1
}

func (d *Doc) removeObject(id int) error {
	layer, ok := findLayer(d.tree, LayerObjects)
	if !ok {
		return errNoObjectLayer
	}

	objects := fieldArray(layer, "objects")
	kept := make([]any, 0, len(objects))
	found := false

	for _, raw := range objects {
		if o, ok := asObject(raw); ok && fieldInt(o, "id") == id {
			found = true

			continue
		}

		kept = append(kept, raw)
	}

	if !found {
		return fmt.Errorf("no object %d", id)
	}

	layer.Set("objects", kept)

	return nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}

	return b
}

// artRule says, in the loader's own words, what art a layer takes
// (tiled.go:807-833).
func artRule(layer string) string {
	if layer == LayerWalls {
		return fmt.Sprintf("wall art is %d wide and %d..%d tall",
			tileW, tileH, maxWallHeight)
	}

	return fmt.Sprintf("floor art is exactly %dx%d", tileW, tileH)
}

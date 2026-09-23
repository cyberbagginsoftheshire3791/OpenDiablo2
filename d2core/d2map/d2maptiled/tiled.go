// Package d2maptiled reads a map authored in Tiled (https://www.mapeditor.org),
// saved as JSON (.tmj), into a plain description the map generator can build.
//
// This is the M5.4 authoring path (Plan Phase 5, "own the assets"): the
// village slice's region is drawn in an editor anyone can download, with tile
// art that is ours, instead of being carved out of Diablo II's DS1 stamps.
// Nothing here knows about the map engine, the renderer or the MPQs -- the
// package reads bytes and returns data, so its tests run anywhere (the gate's
// headless-safety check covers it like every other tested package).
//
// # What a map must look like
//
// Deliberately narrow, and every departure is REFUSED with a message naming
// the thing to change, never guessed at. A map that loads wrong is worse than
// a map that does not load, because the designer is looking at something the
// game is not playing.
//
//   - Orientation isometric, tiles 160x80 (Diablo II's floor diamond, which
//     the renderer, the camera and every sub-tile rule assume), finite size.
//   - Tile layers named "floor" (required) and "walls" (optional), stored as
//     plain JSON arrays -- Tiled's CSV layer format. Base64 and compression
//     are refused, as are group and image layers.
//   - One object layer named "objects" holding exactly one "player_start"
//     and any number of "npc" objects, each npc with a string property
//     "monstat" naming the monstats record it stands in for.
//   - Tilesets EMBEDDED in the map (Tiled: "Embed tileset"). Either kind
//     works: a single sprite sheet, or a collection of images.
//   - Floor art exactly 160x80. Wall art 160 wide and at least 80 tall, drawn
//     so its bottom 80 pixels are the floor diamond it stands on; it is drawn
//     interleaved with the people walking around it, like D2's own walls.
//   - Tile properties, both bool, both optional: "blocked" (nobody walks
//     here) and "blocks_sight" (nothing is seen through here). blocks_sight
//     DEFAULTS TO blocked -- a wall you cannot walk through is a wall you
//     cannot see through -- and is set false for a low fence or a well.
//     Any other property name is refused, so a typo cannot silently do
//     nothing.
//   - Flipped or rotated tiles are refused (the renderer draws art as-is).
//   - Editor settings the game would silently ignore are refused rather than
//     ignored: hidden, translucent, offset, parallax or tinted layers; tileset
//     tile offsets and render-size modes; collision shapes drawn in Tiled's
//     Tile Collision Editor (use the "blocked" property); tile animations;
//     tiles cut from a sub-rectangle of their image; and TILE objects, which
//     Tiled anchors at their bottom corner rather than where they appear to
//     stand -- place people with point objects.
//
// # Coordinates
//
// Tiled draws isometric tile (x, y) where Diablo II does, so tile coordinates
// carry over unchanged. Object positions in an isometric Tiled map are in
// pixels measured along the two tile axes with the map's TILE HEIGHT as the
// unit on both, so an object at (400, 240) on this map stands on tile
// (5, 3). The object's x, y is used whatever its shape (point or rectangle;
// tile objects are refused, above); a point object is the natural choice.
// The player starts in the TILE his start stands in -- the engine places him
// by whole tiles (d2mapgen).
//
// Known limit: the map is read by each process that builds a world. A
// networked client in another process builds from its own -map setting, and
// nothing yet checks that the two agree (the GenerateMap packet carries only
// the region). Single-player, the one way Strigoi is played, reads one file.
package d2maptiled

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/png" // tile art is PNG; registered for image.Decode
	"path"
	"sort"
	"strings"
)

const (
	// TileWidth and TileHeight are the only tile size the engine draws.
	TileWidth  = 160
	TileHeight = 80

	// MaxKinds is how many distinct tile images one map may use: each one
	// becomes one sequence number under a single reserved style, and a
	// sequence is a byte.
	MaxKinds = 256

	// maxSide bounds the map so a corrupt file cannot ask for gigabytes.
	maxSide = 1024

	// maxWallHeight bounds a wall image. The renderer culls rows about 530
	// pixels below the screen, so art taller than that would pop into view at
	// the bottom edge; 512 keeps every wall inside the margin.
	maxWallHeight = 512

	// gidFlipMask covers Tiled's flip and rotation flags in the top bits of
	// every gid.
	gidFlipMask = 0xF0000000
)

// Layer says which of the two tile layers a kind was placed on.
type Layer int

// The two tile layers.
const (
	LayerFloor Layer = iota
	LayerWall
)

func (l Layer) String() string {
	if l == LayerWall {
		return "walls"
	}

	return "floor"
}

// Kind is one tile image as the map uses it: the same Tiled tile placed on
// both layers is two kinds, because a floor and a wall are drawn differently.
type Kind struct {
	// Name is "<tileset>#<tile id>", for messages and for the harness.
	Name        string
	Layer       Layer
	Blocked     bool
	BlocksSight bool
	// Pixels is the tile's art, premultiplied RGBA with its origin at 0,0.
	Pixels *image.RGBA
}

// Cell is one map tile: an index into Map.Kinds for each layer, -1 for none.
type Cell struct {
	Floor, Wall int
}

// NPC is one person the map places.
type NPC struct {
	Monstat string
	// X, Y in world tiles.
	X, Y float64
}

// Map is a parsed, validated authored map.
type Map struct {
	Width, Height  int
	Kinds          []Kind
	Cells          []Cell // row-major, Width*Height
	StartX, StartY float64
	NPCs           []NPC
}

// At returns the cell at x, y. The caller keeps x, y on the map.
func (m *Map) At(x, y int) Cell {
	return m.Cells[x+y*m.Width]
}

// Blocked reports whether nobody may walk on x, y: a tile with no floor, or
// with a floor or wall marked blocked.
func (m *Map) Blocked(x, y int) bool {
	c := m.At(x, y)
	if c.Floor < 0 {
		return true
	}

	return m.Kinds[c.Floor].Blocked || (c.Wall >= 0 && m.Kinds[c.Wall].Blocked)
}

// BlocksSight reports whether a line of sight stops at x, y. A tile with no
// floor does not: there is nothing there to stop it.
func (m *Map) BlocksSight(x, y int) bool {
	c := m.At(x, y)

	return (c.Floor >= 0 && m.Kinds[c.Floor].BlocksSight) || (c.Wall >= 0 && m.Kinds[c.Wall].BlocksSight)
}

// Loader reads a file the map refers to, by the path the map resolves it to.
type Loader func(path string) ([]byte, error)

// Parse reads a .tmj. dir is the directory the map lives in, which is what
// the image paths inside it are relative to; load reads those images.
func Parse(data []byte, dir string, load Loader) (*Map, error) {
	var raw tmj
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("not a Tiled JSON map: %w", err)
	}

	if err := raw.checkShape(); err != nil {
		return nil, err
	}

	p := &parser{
		raw:    &raw,
		dir:    dir,
		load:   load,
		kinds:  map[kindKey]int{},
		images: map[string]*image.RGBA{},
		out: &Map{
			Width:  raw.Width,
			Height: raw.Height,
		},
	}

	if err := p.tilesets(); err != nil {
		return nil, err
	}

	if err := p.layers(); err != nil {
		return nil, err
	}

	return p.out, nil
}

// ---- the raw Tiled JSON --------------------------------------------------

type tmj struct {
	Type        string       `json:"type"`
	Orientation string       `json:"orientation"`
	Width       int          `json:"width"`
	Height      int          `json:"height"`
	TileWidth   int          `json:"tilewidth"`
	TileHeight  int          `json:"tileheight"`
	Infinite    bool         `json:"infinite"`
	Layers      []tmjLayer   `json:"layers"`
	Tilesets    []tmjTileset `json:"tilesets"`
}

type tmjLayer struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Visible     *bool           `json:"visible"`
	Opacity     *float64        `json:"opacity"`
	OffsetX     float64         `json:"offsetx"`
	OffsetY     float64         `json:"offsety"`
	ParallaxX   *float64        `json:"parallaxx"`
	ParallaxY   *float64        `json:"parallaxy"`
	TintColor   string          `json:"tintcolor"`
	Width       int             `json:"width"`
	Height      int             `json:"height"`
	Encoding    string          `json:"encoding"`
	Compression string          `json:"compression"`
	Data        json.RawMessage `json:"data"`
	Objects     []tmjObject     `json:"objects"`
}

type tmjObject struct {
	ID         int           `json:"id"`
	Name       string        `json:"name"`
	Type       string        `json:"type"`
	Class      string        `json:"class"`
	GID        uint32        `json:"gid"`
	X          float64       `json:"x"`
	Y          float64       `json:"y"`
	Properties []tmjProperty `json:"properties"`
}

type tmjProperty struct {
	Name  string          `json:"name"`
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

type tmjTileset struct {
	FirstGID   int       `json:"firstgid"`
	Source     string    `json:"source"`
	Name       string    `json:"name"`
	TileWidth  int       `json:"tilewidth"`
	TileHeight int       `json:"tileheight"`
	TileCount  int       `json:"tilecount"`
	Columns    int       `json:"columns"`
	Margin     int       `json:"margin"`
	Spacing    int       `json:"spacing"`
	Image      string    `json:"image"`
	Tiles      []tmjTile `json:"tiles"`
	TileOffset *struct {
		X int `json:"x"`
		Y int `json:"y"`
	} `json:"tileoffset"`
	TileRenderSize string `json:"tilerendersize"`
	FillMode       string `json:"fillmode"`
}

type tmjTile struct {
	ID          int             `json:"id"`
	Image       string          `json:"image"`
	ImageWidth  int             `json:"imagewidth"`
	ImageHeight int             `json:"imageheight"`
	X           int             `json:"x"`
	Y           int             `json:"y"`
	Width       int             `json:"width"`
	Height      int             `json:"height"`
	ObjectGroup json.RawMessage `json:"objectgroup"`
	Animation   json.RawMessage `json:"animation"`
	Properties  []tmjProperty   `json:"properties"`
}

// checkEditorOnly refuses a layer setting the game would ignore.
func (l *tmjLayer) checkEditorOnly() error {
	switch {
	case l.Visible != nil && !*l.Visible:
		return fmt.Errorf("layer %q is hidden in the editor, and the game would draw it anyway; show it or delete it", l.Name)
	case l.Opacity != nil && *l.Opacity != 1:
		return fmt.Errorf("layer %q has opacity %v; the game draws layers opaque", l.Name, *l.Opacity)
	case l.OffsetX != 0 || l.OffsetY != 0:
		return fmt.Errorf("layer %q is offset by %v,%v; the game draws it unshifted", l.Name, l.OffsetX, l.OffsetY)
	case (l.ParallaxX != nil && *l.ParallaxX != 1) || (l.ParallaxY != nil && *l.ParallaxY != 1):
		return fmt.Errorf("layer %q has parallax; the game has none", l.Name)
	case l.TintColor != "":
		return fmt.Errorf("layer %q is tinted %s; the game draws it untinted", l.Name, l.TintColor)
	}

	return nil
}

// checkEditorOnly refuses a tileset setting the game would ignore.
func (ts *tmjTileset) checkEditorOnly() error {
	switch {
	case ts.TileOffset != nil && (ts.TileOffset.X != 0 || ts.TileOffset.Y != 0):
		return fmt.Errorf("tileset %q has a drawing offset; the game draws tiles where they stand", ts.Name)
	case ts.TileRenderSize != "" && ts.TileRenderSize != "tile":
		return fmt.Errorf("tileset %q renders at %q size; the game draws art at its own size", ts.Name, ts.TileRenderSize)
	case ts.FillMode != "" && ts.FillMode != "stretch":
		return fmt.Errorf("tileset %q uses fill mode %q", ts.Name, ts.FillMode)
	}

	for i := range ts.Tiles {
		tile := &ts.Tiles[i]
		name := fmt.Sprintf("%s#%d", ts.Name, tile.ID)

		switch {
		case len(tile.ObjectGroup) > 0 && string(tile.ObjectGroup) != "null":
			return fmt.Errorf("%s has collision shapes from the Tile Collision Editor; the game reads the \"blocked\" property instead", name)
		case len(tile.Animation) > 0 && string(tile.Animation) != "null":
			return fmt.Errorf("%s is animated; the game draws one frame", name)
		case tile.Width > 0 && (tile.X != 0 || tile.Y != 0 || tile.Width != tile.ImageWidth || tile.Height != tile.ImageHeight):
			return fmt.Errorf("%s uses part of its image; the game draws the whole image", name)
		}
	}

	return nil
}

func (raw *tmj) checkShape() error {
	if raw.Type != "" && raw.Type != "map" {
		return fmt.Errorf("type is %q, want a Tiled map (\"map\")", raw.Type)
	}

	if raw.Orientation != "isometric" {
		return fmt.Errorf("orientation is %q; the engine draws isometric maps only (Map > Map Properties > Orientation)", raw.Orientation)
	}

	if raw.TileWidth != TileWidth || raw.TileHeight != TileHeight {
		return fmt.Errorf("tiles are %dx%d; the engine's tile is %dx%d", raw.TileWidth, raw.TileHeight, TileWidth, TileHeight)
	}

	if raw.Infinite {
		return errors.New("the map is infinite; untick Map Properties > Infinite so it has a fixed size")
	}

	if raw.Width <= 0 || raw.Height <= 0 || raw.Width > maxSide || raw.Height > maxSide {
		return fmt.Errorf("map is %dx%d tiles; each side must be 1..%d", raw.Width, raw.Height, maxSide)
	}

	return nil
}

// ---- parsing ---------------------------------------------------------------

type kindKey struct {
	gid   int
	layer Layer
}

type parser struct {
	raw    *tmj
	dir    string
	load   Loader
	sets   []tmjTileset // sorted by FirstGID
	kinds  map[kindKey]int
	images map[string]*image.RGBA // decoded source images by resolved path
	out    *Map
}

func (p *parser) tilesets() error {
	for i := range p.raw.Tilesets {
		ts := p.raw.Tilesets[i]

		if ts.Source != "" {
			return fmt.Errorf("tileset %q is external (%s); embed it in the map (Map > Embed Tileset)", ts.Source, ts.Source)
		}

		if ts.FirstGID < 1 {
			return fmt.Errorf("tileset %q has firstgid %d", ts.Name, ts.FirstGID)
		}

		if err := ts.checkEditorOnly(); err != nil {
			return err
		}

		if ts.Image != "" && (ts.Columns <= 0 || ts.TileWidth <= 0 || ts.TileHeight <= 0) {
			return fmt.Errorf("tileset %q is a sprite sheet with no columns or tile size", ts.Name)
		}

		p.sets = append(p.sets, ts)
	}

	sort.Slice(p.sets, func(a, b int) bool { return p.sets[a].FirstGID < p.sets[b].FirstGID })

	return nil
}

func (p *parser) layers() error {
	seen := map[string]bool{}
	haveFloor := false
	var objects *tmjLayer

	for i := range p.raw.Layers {
		l := &p.raw.Layers[i]

		if seen[l.Name] {
			return fmt.Errorf("two layers are named %q", l.Name)
		}

		if err := l.checkEditorOnly(); err != nil {
			return err
		}

		seen[l.Name] = true

		switch {
		case l.Type == "tilelayer" && l.Name == "floor":
			haveFloor = true
		case l.Type == "tilelayer" && l.Name == "walls":
		case l.Type == "objectgroup" && l.Name == "objects":
			objects = l
			continue
		default:
			return fmt.Errorf("layer %q (%s) is not one the game reads; use tile layers \"floor\" and \"walls\" and an object layer \"objects\"", l.Name, l.Type)
		}
	}

	if !haveFloor {
		return errors.New("no tile layer named \"floor\"")
	}

	p.out.Cells = make([]Cell, p.out.Width*p.out.Height)
	for i := range p.out.Cells {
		p.out.Cells[i] = Cell{Floor: -1, Wall: -1}
	}

	// Tile layers in file order: the floor must be read before the walls are
	// checked against it, and file order is how the editor stacks them.
	for i := range p.raw.Layers {
		l := &p.raw.Layers[i]
		if l.Type != "tilelayer" {
			continue
		}

		layer := LayerFloor
		if l.Name == "walls" {
			layer = LayerWall
		}

		if err := p.tileLayer(l, layer); err != nil {
			return err
		}
	}

	if objects == nil {
		return errors.New("no object layer named \"objects\"; it must hold the player_start")
	}

	return p.objectLayer(objects)
}

func (p *parser) tileLayer(l *tmjLayer, layer Layer) error {
	if l.Encoding != "" && l.Encoding != "csv" {
		return fmt.Errorf("layer %q is stored as %s; set Map Properties > Tile Layer Format to CSV", l.Name, l.Encoding)
	}

	if l.Compression != "" {
		return fmt.Errorf("layer %q is compressed (%s); set Tile Layer Format to CSV", l.Name, l.Compression)
	}

	if l.Width != p.out.Width || l.Height != p.out.Height {
		return fmt.Errorf("layer %q is %dx%d on a %dx%d map", l.Name, l.Width, l.Height, p.out.Width, p.out.Height)
	}

	var gids []uint32
	if err := json.Unmarshal(l.Data, &gids); err != nil {
		return fmt.Errorf("layer %q: data is not a list of tile ids: %w", l.Name, err)
	}

	if len(gids) != p.out.Width*p.out.Height {
		return fmt.Errorf("layer %q holds %d tiles, want %d", l.Name, len(gids), p.out.Width*p.out.Height)
	}

	for i, gid := range gids {
		if gid == 0 {
			continue
		}

		x, y := i%p.out.Width, i/p.out.Width

		if gid&gidFlipMask != 0 {
			return fmt.Errorf("layer %q tile %d,%d is flipped or rotated; the game draws art as it is", l.Name, x, y)
		}

		kind, err := p.kind(int(gid), layer)
		if err != nil {
			return fmt.Errorf("layer %q tile %d,%d: %w", l.Name, x, y, err)
		}

		if layer == LayerWall {
			p.out.Cells[i].Wall = kind
		} else {
			p.out.Cells[i].Floor = kind
		}
	}

	return nil
}

// kind returns the index of the kind for gid on layer, building it the first
// time it is seen.
func (p *parser) kind(gid int, layer Layer) (int, error) {
	if k, ok := p.kinds[kindKey{gid, layer}]; ok {
		return k, nil
	}

	if len(p.out.Kinds) == MaxKinds {
		return 0, fmt.Errorf("the map uses more than %d distinct tiles", MaxKinds)
	}

	ts, local, err := p.tilesetOf(gid)
	if err != nil {
		return 0, err
	}

	var tile *tmjTile

	for i := range ts.Tiles {
		if ts.Tiles[i].ID == local {
			tile = &ts.Tiles[i]
		}
	}

	pixels, err := p.art(ts, tile, local)
	if err != nil {
		return 0, err
	}

	name := fmt.Sprintf("%s#%d", ts.Name, local)

	if err := checkArt(pixels, layer); err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}

	k := Kind{Name: name, Layer: layer, Pixels: pixels}

	if tile != nil {
		if err := k.properties(tile.Properties); err != nil {
			return 0, fmt.Errorf("%s: %w", name, err)
		}
	}

	p.out.Kinds = append(p.out.Kinds, k)
	p.kinds[kindKey{gid, layer}] = len(p.out.Kinds) - 1

	return len(p.out.Kinds) - 1, nil
}

func (p *parser) tilesetOf(gid int) (*tmjTileset, int, error) {
	for i := len(p.sets) - 1; i >= 0; i-- {
		ts := &p.sets[i]
		if gid < ts.FirstGID {
			continue
		}

		local := gid - ts.FirstGID

		if ts.Image != "" {
			if ts.TileCount > 0 && local >= ts.TileCount {
				break
			}

			return ts, local, nil
		}

		for j := range ts.Tiles {
			if ts.Tiles[j].ID == local {
				return ts, local, nil
			}
		}

		break
	}

	return nil, 0, fmt.Errorf("tile id %d is in no tileset", gid)
}

// art returns the pixels for one tile: its own image in a collection, or its
// cell of the sheet.
func (p *parser) art(ts *tmjTileset, tile *tmjTile, local int) (*image.RGBA, error) {
	if ts.Image == "" {
		if tile == nil || tile.Image == "" {
			return nil, fmt.Errorf("%s#%d has no image", ts.Name, local)
		}

		img, err := p.image(tile.Image)
		if err != nil {
			return nil, err
		}

		return img, nil
	}

	sheet, err := p.image(ts.Image)
	if err != nil {
		return nil, err
	}

	col, row := local%ts.Columns, local/ts.Columns
	x := ts.Margin + col*(ts.TileWidth+ts.Spacing)
	y := ts.Margin + row*(ts.TileHeight+ts.Spacing)
	cell := image.Rect(x, y, x+ts.TileWidth, y+ts.TileHeight)

	if !cell.In(sheet.Bounds()) {
		return nil, fmt.Errorf("%s#%d lies outside its sheet %s", ts.Name, local, ts.Image)
	}

	out := image.NewRGBA(image.Rect(0, 0, ts.TileWidth, ts.TileHeight))
	draw.Draw(out, out.Bounds(), sheet, cell.Min, draw.Src)

	return out, nil
}

// image loads and decodes one source image, once.
func (p *parser) image(rel string) (*image.RGBA, error) {
	if strings.Contains(rel, `\`) {
		return nil, fmt.Errorf("image path %q uses backslashes; Tiled writes forward slashes on every system", rel)
	}

	full := path.Join(p.dir, rel)

	if img, ok := p.images[full]; ok {
		return img, nil
	}

	data, err := p.load(full)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", full, err)
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", full, err)
	}

	b := src.Bounds()
	if b.Empty() {
		return nil, fmt.Errorf("%s has no pixels", full)
	}

	// Onto a zeroed RGBA: normalises the origin and premultiplies, which the
	// renderer's surfaces require (the same reason as d2asset's PNG path).
	img := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(img, img.Bounds(), src, b.Min, draw.Src)
	p.images[full] = img

	return img, nil
}

func checkArt(img *image.RGBA, layer Layer) error {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()

	if layer == LayerFloor {
		if w != TileWidth || h != TileHeight {
			return fmt.Errorf("floor art is %dx%d, want exactly %dx%d", w, h, TileWidth, TileHeight)
		}

		return nil
	}

	if w != TileWidth || h < TileHeight || h > maxWallHeight {
		return fmt.Errorf("wall art is %dx%d, want %d wide and %d..%d tall", w, h, TileWidth, TileHeight, maxWallHeight)
	}

	return nil
}

func (k *Kind) properties(props []tmjProperty) error {
	sightSet := false

	for _, prop := range props {
		var v bool

		switch prop.Name {
		case "blocked", "blocks_sight":
		default:
			return fmt.Errorf("unknown tile property %q; the game reads \"blocked\" and \"blocks_sight\"", prop.Name)
		}

		if prop.Type != "bool" || json.Unmarshal(prop.Value, &v) != nil {
			return fmt.Errorf("property %q must be a bool", prop.Name)
		}

		if prop.Name == "blocked" {
			k.Blocked = v
		} else {
			k.BlocksSight = v
			sightSet = true
		}
	}

	if !sightSet {
		k.BlocksSight = k.Blocked
	}

	return nil
}

func (p *parser) objectLayer(l *tmjLayer) error {
	starts := 0

	for i := range l.Objects {
		o := &l.Objects[i]

		kind := o.Class
		if kind == "" {
			kind = o.Type
		} else if o.Type != "" && o.Type != o.Class {
			return fmt.Errorf("object %d says both %q and %q", o.ID, o.Type, o.Class)
		}

		if o.GID != 0 {
			return fmt.Errorf("object %d is a tile object, which Tiled anchors at its bottom corner, not where it seems to stand; use a point object", o.ID)
		}

		x, y := o.X/float64(p.raw.TileHeight), o.Y/float64(p.raw.TileHeight)

		switch kind {
		case "player_start":
			if len(o.Properties) > 0 {
				return fmt.Errorf("player_start (object %d) takes no properties", o.ID)
			}

			if err := p.standable("player_start", o.ID, x, y); err != nil {
				return err
			}

			starts++
			p.out.StartX, p.out.StartY = x, y
		case "npc":
			monstat, err := npcMonstat(o)
			if err != nil {
				return err
			}

			if err := p.standable("npc "+monstat, o.ID, x, y); err != nil {
				return err
			}

			p.out.NPCs = append(p.out.NPCs, NPC{Monstat: monstat, X: x, Y: y})
		case "":
			return fmt.Errorf("object %d (%q) has no class; set it to player_start or npc", o.ID, o.Name)
		default:
			return fmt.Errorf("object %d has class %q; the game reads player_start and npc", o.ID, kind)
		}
	}

	if starts != 1 {
		return fmt.Errorf("the objects layer holds %d player_start objects, want exactly 1", starts)
	}

	return nil
}

func npcMonstat(o *tmjObject) (string, error) {
	monstat := ""

	for _, prop := range o.Properties {
		if prop.Name != "monstat" {
			return "", fmt.Errorf("npc (object %d): unknown property %q; an npc takes \"monstat\"", o.ID, prop.Name)
		}

		if prop.Type != "string" || json.Unmarshal(prop.Value, &monstat) != nil {
			return "", fmt.Errorf("npc (object %d): monstat must be a string", o.ID)
		}
	}

	monstat = strings.TrimSpace(monstat)
	if monstat == "" {
		return "", fmt.Errorf("npc (object %d) has no monstat property naming who stands here", o.ID)
	}

	return monstat, nil
}

// standable refuses a person placed off the map or on a tile nobody can stand
// on: the engine would put him there anyway, stuck, and the map would look
// right in the editor.
func (p *parser) standable(what string, id int, x, y float64) error {
	tx, ty := int(x), int(y)

	if x < 0 || y < 0 || tx >= p.out.Width || ty >= p.out.Height {
		return fmt.Errorf("%s (object %d) at tile %.2f,%.2f is off the %dx%d map", what, id, x, y, p.out.Width, p.out.Height)
	}

	if p.out.Blocked(tx, ty) {
		return fmt.Errorf("%s (object %d) stands on tile %d,%d, which is blocked or has no floor", what, id, tx, ty)
	}

	return nil
}

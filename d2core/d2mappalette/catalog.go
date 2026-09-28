package d2mappalette

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// ErrNotPNG is returned by [PNGSize] for bytes that are not a PNG, or are too
// short to hold an IHDR.
var ErrNotPNG = errors.New("d2mappalette: not a PNG with an IHDR header")

// pngMagic is the eight-byte PNG signature.
var pngMagic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// PNGSize reads a PNG's pixel size out of its IHDR header without decoding it.
//
// A PNG is an 8-byte signature, then a 4-byte chunk length, then the 4-byte
// chunk type, and the IHDR chunk must come first: so the width is the
// big-endian uint32 at byte 16 and the height the one at byte 20. That is 24
// bytes read instead of a full inflate-and-unfilter of every candidate image,
// which matters when the palette measures a directory of them to open a tab.
//
// The engine's loader decodes the image for real (tiled.go:788), so this is
// a cheap pre-read and not a replacement for it; the two must agree, and
// TestPNGSizeAgreesWithTheImageDecoder holds them to it.
func PNGSize(data []byte) (w, h int, err error) {
	const ihdrAt = 8

	if len(data) < 24 || string(data[:8]) != string(pngMagic) {
		return 0, 0, ErrNotPNG
	}

	if string(data[ihdrAt+4:ihdrAt+8]) != "IHDR" {
		return 0, 0, ErrNotPNG
	}

	w = int(binary.BigEndian.Uint32(data[16:20]))
	h = int(binary.BigEndian.Uint32(data[20:24]))

	if w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("%w: IHDR says %dx%d", ErrNotPNG, w, h)
	}

	return w, h, nil
}

// stateWords are filename stems that name a STATE of a thing rather than the
// thing: "peasant-house/intact.png" is a peasant house, not an "intact". Any
// other stem is kept in brackets -- "Burned house (cold ruin)".
var stateWords = map[string]bool{"intact": true, "default": true, "base": true}

// groupDirs are directory names that are a filing cabinet rather than the name
// of a thing, so [HumanName] does not call a tile a "Tiles".
var groupDirs = map[string]bool{
	"": true, ".": true, "/": true, "tiles": true, "structures": true,
	"maps": true, "renders": true, "art": true, "data": true, "strigoi": true,
}

// HumanName turns an art path into human words.
//
// NOTHING IN THIS PROJECT CARRIES A TITLE. A tileset tile has an image path and
// nothing else (d2maptiled's tmjTile has no name field); a structure PNG on disk
// has a folder and a filename. So a display name has to be derived, and the
// derivation is written down here rather than buried:
//
//   - drop the directory and the extension, and drop a leading "placeholder-";
//   - if the file sits in a folder that names a thing rather than a category,
//     the FOLDER is the name, and the filename follows in brackets unless it is
//     a plain state word: "village-hearth/unlit.png" -> "Village hearth
//     (unlit)", "peasant-house/intact.png" -> "Peasant house";
//   - otherwise the filename is the name: "tiles/placeholder-church-tower.png"
//     -> "Church tower";
//   - hyphens and underscores become spaces, and the first letter is
//     capitalised. Nothing else is title-cased, because "Church Tower" reads
//     like a proper noun and this is a label.
func HumanName(p string) string {
	p = path.Clean(strings.ReplaceAll(p, "\\", "/"))
	base := strings.TrimSuffix(path.Base(p), path.Ext(p))
	base = trimPlaceholderPrefix(base)
	dir := strings.ToLower(path.Base(path.Dir(p)))

	if !groupDirs[dir] {
		name := words(path.Base(path.Dir(p)))
		if !stateWords[strings.ToLower(base)] && base != "" {
			name += " (" + strings.ToLower(words(base)) + ")"
		}

		return name
	}

	return words(base)
}

func trimPlaceholderPrefix(s string) string {
	if len(s) >= len(placeholderWord)+1 && strings.EqualFold(s[:len(placeholderWord)], placeholderWord) {
		if s[len(placeholderWord)] == '-' || s[len(placeholderWord)] == '_' {
			return s[len(placeholderWord)+1:]
		}
	}

	return s
}

// words replaces separators with spaces and capitalises the first letter.
func words(s string) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '-' || r == '_' {
			return ' '
		}

		return r
	}, s))

	if s == "" {
		return ""
	}

	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])

	return string(r)
}

// footprintClaim is what the SOURCE of a piece of art said about the footprint
// question, which is not the same as what the art's size allows.
//
// This distinction was removed once, on the reasoning that CheckArt already
// enforces it, and TestShippedVillageCatalog went red: every 160x80 grass tile
// started being offered as a 1x1 structure. CheckArt enforces the DECLARED
// footprint rules, but it cannot know whether a zero footprint means "this tile
// says it is not a structure" or "nobody has said". Those two want different
// answers, so the source has to say which it is.
type footprintClaim int

const (
	// claimNone: nothing declared either way. A bare PNG in
	// data/strigoi/structures is this -- the folder holds images and a README --
	// so every layer its size allows is fair game, including a square structure
	// footprint derived from its width.
	claimNone footprintClaim = iota

	// claimNotAStructure: the source answered and said no footprint. A tileset
	// tile with no footprint_w/footprint_h is this, and kind() will only take it
	// on a tile layer (tiled.go:692) -- offering it as a tile object would
	// produce a map the loader refuses.
	claimNotAStructure

	// claimFootprint: the source declared a footprint, so it is a tile object or
	// nothing (tiled.go:690).
	claimFootprint
)

// shape is everything needed to decide layers, footprint and problems for one
// piece of art. It is the single place those three are worked out, so a tileset
// tile, a bare PNG and a manifest module cannot drift apart.
type shape struct {
	// declared is the footprint a tile property or a manifest declares, and the
	// zero Point when nothing does. It is what the floor and wall checks see,
	// because those refuse a DECLARED footprint (tiled.go:692) and know nothing
	// about a derived one.
	declared image.Point
	// structure is the footprint to judge a structure by: declared when
	// something declared one, otherwise derived from the art's width by
	// [SquareFootprintFor], and the zero Point when no square footprint fits.
	structure image.Point
	claim     footprintClaim
	w, h      int
}

// layers is every layer the engine will actually take this art on.
//
// A DECLARED footprint settles it: the loader takes such a tile as a tile object
// or not at all (tiled.go:690-694). Otherwise the answer is the size-only one,
// which is exactly what [LegalLayers] computes -- so this asks it rather than
// keeping a second copy of the same three tests, and strikes the structure bit
// off when the source has already said the art is not one.
func (s shape) layers() LayerSet {
	switch s.claim {
	case claimFootprint:
		if len(CheckArt(d2maptiled.LayerStructure, s.declared, s.w, s.h)) == 0 {
			return SetStructure
		}

		return 0
	case claimNotAStructure:
		return LegalLayers(s.w, s.h) &^ SetStructure
	}

	return LegalLayers(s.w, s.h)
}

func (s shape) footprintFor(l d2maptiled.Layer) image.Point {
	if l == d2maptiled.LayerStructure {
		return s.structure
	}

	return s.declared
}

// intended is the layer to explain a refusal against, when no layer will take
// the art: whatever the author most likely meant.
func (s shape) intended() d2maptiled.Layer {
	switch {
	case s.declared != image.Point{}:
		return d2maptiled.LayerStructure
	case s.w == TileWidth:
		return d2maptiled.LayerWall
	}

	return d2maptiled.LayerStructure
}

// resolve fills in an entry's layer, footprint and problems.
func (e *Entry) resolve(s shape, footprintWhy string) {
	e.PixelWidth, e.PixelHeight = s.w, s.h
	e.Layers = s.layers()

	if !e.Layers.Empty() {
		e.Layer = e.Layers.Primary()

		if e.Layer == d2maptiled.LayerStructure {
			e.Footprint, e.FootprintWhy = s.structure, footprintWhy
		} else {
			e.Footprint = image.Pt(1, 1)
			e.FootprintWhy = "one tile: a tile on the " + e.Layer.String() + " layer covers exactly one"
		}

		return
	}

	e.Layer = s.intended()
	e.Footprint, e.FootprintWhy = s.structure, footprintWhy

	if e.Footprint == (image.Point{}) {
		e.Footprint = image.Pt(1, 1)
	}

	e.Problems = CheckArt(e.Layer, s.footprintFor(e.Layer), s.w, s.h)

	// A width that is not a multiple of TileWidth fits no square footprint at
	// all, and CheckArt would only be able to say "it has no footprint". Say the
	// useful thing instead.
	if e.Layer == d2maptiled.LayerStructure && s.declared == (image.Point{}) && s.structure == (image.Point{}) {
		e.Problems = []string{fmt.Sprintf(
			"art is %d wide, and no square footprint fits: a structure's width is (w+h)*%d, so for a "+
				"square footprint it must be a multiple of %d from %d to %d (tiled.go:811, tiled.go:880)",
			s.w, TileHeight, TileWidth, TileWidth, TileWidth*MaxFootprint)}
	}
}

// Catalog is what the palette can offer, built up from one or more sources.
//
// A Catalog is not a live view of anything: the readers measure the files once
// and the Catalog then answers from what they measured. The editor rebuilds it
// when the user changes maps or drops new art in.
//
// IDs are unique within a Catalog and the FIRST entry to claim one wins: a later
// reader offering the same ID is ignored rather than overwriting it. The three
// readers use disjoint ID shapes ("<tileset>#<id>", "<dir>/<file>",
// "<manifest>/<module>/<state>"), so this is a guard and not a policy anyone
// should be relying on.
type Catalog struct {
	entries []Entry
	byID    map[string]int

	// kindsUsed is how many distinct (gid, layer) kinds the map already spends
	// out of MaxKinds, counted the way the loader counts them.
	kindsUsed int
	mapFrom   string
	mapNote   string
}

// NewCatalog returns an empty Catalog.
func NewCatalog() *Catalog { return &Catalog{byID: map[string]int{}} }

// Entries is every entry, in tab order then alphabetical.
func (c *Catalog) Entries() []Entry {
	out := make([]Entry, len(c.entries))
	copy(out, c.entries)
	sortEntries(out)

	return out
}

// Len is how many entries the catalog holds.
func (c *Catalog) Len() int { return len(c.entries) }

// ByID returns one entry.
func (c *Catalog) ByID(id string) (Entry, bool) {
	i, ok := c.byID[id]
	if !ok {
		return Entry{}, false
	}

	return c.entries[i], true
}

// InCategory is the entries of one tab, in display order.
func (c *Catalog) InCategory(cat Category) []Entry {
	var out []Entry

	for _, e := range c.entries {
		if e.Category == cat {
			out = append(out, e)
		}
	}

	sortEntries(out)

	return out
}

// Placeable is every entry the engine will accept as it stands: what the editor
// may actually let the user drop on the map.
func (c *Catalog) Placeable() []Entry {
	var out []Entry

	for _, e := range c.entries {
		if e.Placeable() {
			out = append(out, e)
		}
	}

	sortEntries(out)

	return out
}

// Refused is every entry the engine would refuse, which the editor greys out
// with [Entry.Why] in the tooltip.
func (c *Catalog) Refused() []Entry {
	var out []Entry

	for _, e := range c.entries {
		if !e.Placeable() {
			out = append(out, e)
		}
	}

	sortEntries(out)

	return out
}

// Tabs is the tab strip with this catalog's counts filled in.
func (c *Catalog) Tabs() []Tab {
	out := Tabs()

	for i := range out {
		out[i].Count = len(c.InCategory(out[i].Category))
	}

	return out
}

// KindsUsed is how many distinct (tile image, layer) kinds the map read by
// [Catalog.ReadMap] already spends, and KindsFree is how many of the loader's
// MaxKinds are left. Placing art of a kind the map does not yet use costs one;
// a map at the cap is refused outright (tiled.go:657-658), so a palette that
// cannot see this offers a tile that will not load.
func (c *Catalog) KindsUsed() int { return c.kindsUsed }

// KindsFree is MaxKinds minus KindsUsed, floored at zero.
func (c *Catalog) KindsFree() int {
	if c.kindsUsed > MaxKinds {
		return 0
	}

	return MaxKinds - c.kindsUsed
}

// MapNote is the "note" property of the map [Catalog.ReadMap] read, verbatim,
// and "" when there was none. The loader reads this property and discards it
// (tiled.go:325), so this is the only place in the engine it survives.
func (c *Catalog) MapNote() string { return c.mapNote }

func (c *Catalog) add(e Entry) {
	if _, clash := c.byID[e.ID]; clash {
		return
	}

	c.byID[e.ID] = len(c.entries)
	c.entries = append(c.entries, e)
}

// ---- the .tmj reader -------------------------------------------------------

type tmjProperty struct {
	Name  string          `json:"name"`
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

type tmjTile struct {
	ID         int           `json:"id"`
	Image      string        `json:"image"`
	Properties []tmjProperty `json:"properties"`
}

type tmjTileset struct {
	FirstGID int       `json:"firstgid"`
	Name     string    `json:"name"`
	Source   string    `json:"source"`
	Tiles    []tmjTile `json:"tiles"`
}

type tmjObject struct {
	GID uint32 `json:"gid"`
}

type tmjLayer struct {
	Type    string          `json:"type"`
	Name    string          `json:"name"`
	Data    json.RawMessage `json:"data"`
	Objects []tmjObject     `json:"objects"`
}

type tmjMap struct {
	Properties []tmjProperty `json:"properties"`
	Layers     []tmjLayer    `json:"layers"`
	Tilesets   []tmjTileset  `json:"tilesets"`
}

// ReadMap catalogues a Tiled .tmj map's embedded tilesets.
//
// tmjPath is a slash path inside fsys. Tile image paths are resolved the way
// the loader resolves them, path.Join(dir-of-the-tmj, rel) (tiled.go:777), so
// the village's "../structures/village-well/intact.png" lands on
// "structures/village-well/intact.png" for an fsys rooted at data/strigoi --
// exactly as the game reads it.
//
// It reads the map's own "note" property, which the loader throws away
// (tiled.go:325), and feeds it to [DeriveStatus] as a map-wide signal. It also
// counts the distinct (gid, layer) kinds the map already spends against
// MaxKinds; see [Catalog.KindsUsed].
//
// An EXTERNAL tileset (one with a "source" instead of embedded tiles) is
// skipped, with an error returned naming it, because the loader refuses the map
// outright for one (tiled.go:515) and there is nothing for the palette to offer.
func (c *Catalog) ReadMap(fsys fs.FS, tmjPath string) error {
	tmjPath = path.Clean(tmjPath)

	data, err := fs.ReadFile(fsys, tmjPath)
	if err != nil {
		return fmt.Errorf("d2mappalette: reading %s: %w", tmjPath, err)
	}

	var raw tmjMap
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("d2mappalette: %s is not a Tiled JSON map: %w", tmjPath, err)
	}

	dir := path.Dir(tmjPath)
	c.mapFrom = tmjPath
	c.mapNote = mapNote(raw.Properties)
	c.kindsUsed = countKinds(raw)

	for i := range raw.Tilesets {
		ts := &raw.Tilesets[i]
		if ts.Source != "" {
			return fmt.Errorf("d2mappalette: tileset %q in %s is external (%s); the loader refuses the map for it (tiled.go:515)",
				ts.Name, tmjPath, ts.Source)
		}

		for j := range ts.Tiles {
			e, err := c.tileEntry(fsys, dir, ts, &ts.Tiles[j])
			if err != nil {
				return err
			}

			c.add(e)
		}
	}

	return nil
}

func mapNote(props []tmjProperty) string {
	for _, p := range props {
		if p.Name != "note" {
			continue
		}

		var s string
		if json.Unmarshal(p.Value, &s) == nil {
			return s
		}
	}

	return ""
}

// countKinds counts the distinct (gid, layer) pairs the map spends, the way
// kind() keys its cache (tiled.go:653). A tile used on both the floor and the
// walls layer is TWO kinds, which is the loader's own arithmetic.
func countKinds(raw tmjMap) int {
	type key struct {
		gid   uint32
		layer d2maptiled.Layer
	}

	seen := map[key]bool{}

	for i := range raw.Layers {
		l := &raw.Layers[i]

		switch l.Type {
		case "tilelayer":
			layer := d2maptiled.LayerFloor
			if l.Name != "floor" {
				layer = d2maptiled.LayerWall
			}

			var gids []uint32
			if json.Unmarshal(l.Data, &gids) != nil {
				// Base64 or compressed: the loader refuses the map
				// (tiled.go:607, tiled.go:615-618), so there is nothing to
				// count and nothing to hide.
				continue
			}

			for _, g := range gids {
				if g != 0 {
					seen[key{g, layer}] = true
				}
			}
		case "objectgroup":
			for _, o := range l.Objects {
				if o.GID != 0 {
					seen[key{o.GID, d2maptiled.LayerStructure}] = true
				}
			}
		}
	}

	return len(seen)
}

// tileEntry builds one Entry from one embedded tileset tile.
func (c *Catalog) tileEntry(fsys fs.FS, dir string, ts *tmjTileset, t *tmjTile) (Entry, error) {
	img := path.Join(dir, strings.ReplaceAll(t.Image, "\\", "/"))

	w, h, err := readPNGSize(fsys, img)
	if err != nil {
		return Entry{}, fmt.Errorf("d2mappalette: tile %s#%d: %w", ts.Name, t.ID, err)
	}

	declared, blocked, sight, blockedFalse, err := tileProperties(t.Properties)
	if err != nil {
		return Entry{}, fmt.Errorf("d2mappalette: tile %s#%d: %w", ts.Name, t.ID, err)
	}

	s := shape{declared: declared, structure: declared, claim: claimFootprint, w: w, h: h}

	why := fmt.Sprintf("declared by the tile's footprint_w/footprint_h as %dx%d", declared.X, declared.Y)

	if declared == (image.Point{}) {
		// The tile ANSWERED and said it is not a structure, so the loader will
		// only take it on a tile layer (tiled.go:692).
		s.claim, why = claimNotAStructure, ""
	}

	e := Entry{
		// The loader's own name for a kind (tiled.go:679), so a palette entry
		// and an engine error message identify the same thing.
		ID:          fmt.Sprintf("%s#%d", ts.Name, t.ID),
		DisplayName: HumanName(img),
		ImagePath:   img,
		Blocked:     blocked,
		BlocksSight: sight,
		Source:      Source{Kind: "tileset", From: c.mapFrom, Detail: fmt.Sprintf("%s#%d", ts.Name, t.ID)},
	}

	e.resolve(s, why)

	// blocked=false on a structure is refused outright (tiled.go:887). The
	// engine forces Blocked true anyway (tiled.go:890), which is why the
	// EXPLICIT false has to be carried out of tileProperties separately: an
	// earlier draft checked !blocked here and could never fire, because
	// tileProperties had already forced it true.
	if declared != (image.Point{}) && blockedFalse {
		e.Problems = append(e.Problems, "a structure cannot be blocked=false; its footprint is solid (tiled.go:887)")
	}

	e.Category = CategoryOf(e.Layer, e.Footprint, e.ID, e.DisplayName)
	e.Status, e.StatusWhy, e.Signals = DeriveStatus(Signals{
		Path:        img,
		MapNote:     c.mapNote,
		MapNoteFrom: c.mapFrom,
	})

	return e, nil
}

// tileProperties reads a tile's properties the way Kind.properties does
// (tiled.go:835-899), including its two defaults: blocked is false unless said,
// and blocks_sight defaults to blocked-or-has-a-footprint (tiled.go:895-897). A
// structure's blocked is forced true (tiled.go:890), which is reported as the
// engine's own value while the refusal of an explicit blocked=false is kept as a
// problem by the caller.
func tileProperties(props []tmjProperty) (fp image.Point, blocked, sight, blockedFalse bool, err error) {
	sightSet := false

	for _, p := range props {
		switch p.Name {
		case "footprint_w", "footprint_h":
			var n int
			if p.Type != "int" || json.Unmarshal(p.Value, &n) != nil {
				return fp, false, false, false, fmt.Errorf("property %q must be an int from 1 to %d (tiled.go:845)", p.Name, MaxFootprint)
			}

			if p.Name == "footprint_w" {
				fp.X = n
			} else {
				fp.Y = n
			}
		case "blocked":
			var v bool
			if p.Type != "bool" || json.Unmarshal(p.Value, &v) != nil {
				return fp, false, false, false, errors.New("property \"blocked\" must be a bool")
			}

			blocked = v
			blockedFalse = !v
		case "blocks_sight":
			var v bool
			if p.Type != "bool" || json.Unmarshal(p.Value, &v) != nil {
				return fp, false, false, false, errors.New("property \"blocks_sight\" must be a bool")
			}

			sight, sightSet = v, true
		default:
			return fp, false, false, false, fmt.Errorf("unknown tile property %q; the game reads \"blocked\", \"blocks_sight\", \"footprint_w\" and \"footprint_h\"", p.Name)
		}
	}

	if (fp.X == 0) != (fp.Y == 0) {
		return fp, false, false, false, errors.New("a structure needs both footprint_w and footprint_h")
	}

	if fp != (image.Point{}) {
		blocked = true
	}

	if !sightSet {
		sight = blocked || fp != (image.Point{})
	}

	return fp, blocked, sight, blockedFalse, nil
}

// ---- the on-disk structure reader ------------------------------------------

// ReadStructureDir catalogues every PNG under dir, one entry each.
//
// This is data/strigoi/structures: art-repo renders copied into the game so the
// authored map can stand them up. NOTHING THERE DECLARES A FOOTPRINT -- the
// folder holds PNGs and a README, and the README's table is Markdown -- so the
// footprint is DERIVED from the art's width by [SquareFootprintFor], which is
// the loader's own formula inverted and therefore exact about what the engine
// will accept. It is not exact about what the artist meant: a 480-wide image
// drawn as a 1x5 barn is offered here as a 3x3, and [Entry.FootprintWhy] says
// the footprint was derived so the user can see that.
//
// Blocked and BlocksSight on these entries are the engine's defaults for a tile
// with NO properties: false and false for a wall, and forced true for a
// structure (tiled.go:890, tiled.go:895-897). They are what the engine would
// believe if the piece were dropped in a map without properties; the editor
// writes the properties it wants when it places one.
//
// If artFS is non-nil it is searched for a strigoi-art structure manifest
// ("<name>.json" beside a "structures" directory) to pick up an "integration"
// value; pass nil when the art repository is not on hand, and the entries fall
// back to whatever the path and the map note say.
func (c *Catalog) ReadStructureDir(fsys fs.FS, dir string, artFS fs.FS) error {
	dir = path.Clean(dir)

	integrations := map[string]Signals{}
	if artFS != nil {
		integrations = readArtIntegrations(artFS)
	}

	return fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("d2mappalette: walking %s: %w", dir, err)
		}

		if d.IsDir() || !strings.EqualFold(path.Ext(p), ".png") {
			return nil
		}

		w, h, err := readPNGSize(fsys, p)
		if err != nil {
			return fmt.Errorf("d2mappalette: %w", err)
		}

		rel := strings.TrimPrefix(strings.TrimPrefix(p, dir), "/")

		s := shape{claim: claimNone, w: w, h: h}
		why := ""

		if fp, ok := SquareFootprintFor(w); ok {
			s.structure = fp
			why = fmt.Sprintf("derived from the art's width: %d px is (%d+%d)*%d, and a structure's "+
				"footprint must be square, so %dx%d is the only one the engine accepts (tiled.go:811, tiled.go:880). "+
				"Nothing on disk declares it", w, fp.X, fp.Y, TileHeight, fp.X, fp.Y)
		}

		e := Entry{
			ID:          strings.TrimSuffix(rel, path.Ext(rel)),
			DisplayName: HumanName(p),
			ImagePath:   p,
			Source:      Source{Kind: "disk", From: p},
		}

		e.resolve(s, why)

		if e.Layer == d2maptiled.LayerStructure {
			e.Blocked, e.BlocksSight = true, true
		}

		e.Category = CategoryOf(e.Layer, e.Footprint, e.ID, e.DisplayName)

		sig := integrations[path.Base(path.Dir(p))]
		sig.Path = p
		sig.MapNote, sig.MapNoteFrom = c.mapNote, c.mapFrom
		e.Status, e.StatusWhy, e.Signals = DeriveStatus(sig)

		c.add(e)

		return nil
	})
}

// readArtIntegrations reads every structures/*.json in a strigoi-art tree for
// its "integration" value, keyed by the manifest's id. Missing or unreadable
// files are silently skipped: the art repository is optional, and a palette
// that refuses to open because it is not mounted is worse than one that says
// "unknown".
func readArtIntegrations(artFS fs.FS) map[string]Signals {
	out := map[string]Signals{}

	names, err := fs.Glob(artFS, "structures/*.json")
	if err != nil {
		return out
	}

	for _, n := range names {
		data, err := fs.ReadFile(artFS, n)
		if err != nil {
			continue
		}

		var m struct {
			ID          string `json:"id"`
			Integration string `json:"integration"`
		}

		if json.Unmarshal(data, &m) != nil || m.ID == "" {
			continue
		}

		out[m.ID] = Signals{Integration: m.Integration, IntegrationFrom: n}
	}

	return out
}

// ---- the module-manifest reader --------------------------------------------

type moduleManifest struct {
	ID      string `json:"id"`
	Modules []struct {
		ID          string            `json:"id"`
		Footprint   []int             `json:"footprint_tiles"`
		Canvas      []int             `json:"world_canvas_pixels"`
		States      []string          `json:"states"`
		Outputs     map[string]string `json:"outputs"`
		Integration string            `json:"integration"`
	} `json:"modules"`
}

// ReadModules catalogues a strigoi-art module manifest: one entry per module per
// state, each pointing at the render that state names.
//
// This is how the Dealu monastery reaches the palette. The monastery is ART
// ONLY -- grep the engine for "dealu" and there are zero hits at HEAD d1d07434
// -- so nothing of it is installed, and its six modules are judged here purely
// on whether their renders satisfy the engine's rules. Three do (church 6x6 at
// 960x768, gate 3x3 at 480x576, well 2x2 at 320x384) and three do not (cells
// 3x7 at 1120x720, refectory 3x6 at 960x640, wall 3x1 at 480x384: each is
// non-square AND the wrong width for what it declares). The refusals are on the
// entries, in the loader's words.
//
// THE MANIFEST'S DECLARED FOOTPRINT IS USED AS DECLARED. It would be easy to
// "fix" the cells by deriving 7x7 from its 1120-pixel width, and it would be a
// lie: the art is three tiles deep. A declared footprint that the width
// contradicts is reported as the conflict it is.
//
// manifestPath and every "outputs" path are slash paths inside fsys, and the
// outputs are resolved from the ROOT of fsys, not from the manifest's directory,
// because that is how strigoi-art writes them ("renders/structures/..."). Point
// fsys at the art repository's root. The manifest's free-text keys --
// "walkability", "rectangular_canvas_note", "historical_status" -- are not read;
// [Signals] records why.
func (c *Catalog) ReadModules(fsys fs.FS, manifestPath string) error {
	manifestPath = path.Clean(manifestPath)

	data, err := fs.ReadFile(fsys, manifestPath)
	if err != nil {
		return fmt.Errorf("d2mappalette: reading %s: %w", manifestPath, err)
	}

	var m moduleManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("d2mappalette: %s is not a module manifest: %w", manifestPath, err)
	}

	if m.ID == "" {
		return fmt.Errorf("d2mappalette: %s has no \"id\"", manifestPath)
	}

	for i := range m.Modules {
		mod := &m.Modules[i]

		states := mod.States
		if len(states) == 0 {
			states = sortedKeys(mod.Outputs)
		}

		for _, state := range states {
			out, ok := mod.Outputs[state]
			if !ok || out == "" {
				continue
			}

			c.add(c.moduleEntry(fsys, manifestPath, m.ID, mod.ID, state, out,
				declaredFootprint(mod.Footprint), mod.Canvas, mod.Integration))
		}
	}

	return nil
}

func declaredFootprint(fp []int) image.Point {
	if len(fp) != 2 {
		return image.Point{}
	}

	return image.Pt(fp[0], fp[1])
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}

func (c *Catalog) moduleEntry(fsys fs.FS, manifest, manifestID, modID, state, art string,
	declared image.Point, canvas []int, integration string) Entry {
	e := Entry{
		ID:          manifestID + "/" + modID + "/" + state,
		DisplayName: moduleName(manifestID, modID, state),
		ImagePath:   art,
		Source:      Source{Kind: "manifest", From: manifest, Detail: modID + " " + state},
	}

	var extra []string

	w, h, err := readPNGSize(fsys, art)
	if err != nil {
		// No render to measure. The declared canvas is reported so the table is
		// still informative, and the missing file is a problem in its own right:
		// you cannot place art that is not there.
		if len(canvas) == 2 {
			w, h = canvas[0], canvas[1]
		}

		extra = append(extra, fmt.Sprintf("its art %s could not be read (%v), so the size above is the "+
			"manifest's declaration and not a measurement", art, err))
	} else if len(canvas) == 2 && (canvas[0] != w || canvas[1] != h) {
		extra = append(extra, fmt.Sprintf("the manifest declares world_canvas_pixels %dx%d but %s measures %dx%d",
			canvas[0], canvas[1], art, w, h))
	}

	why := "no footprint declared"
	if declared != (image.Point{}) {
		why = fmt.Sprintf("declared by %s as %dx%d", manifest, declared.X, declared.Y)
	}

	e.resolve(shape{declared: declared, structure: declared, claim: claimFootprint, w: w, h: h}, why)

	// A module is a structure, so the engine forces it solid (tiled.go:890) and
	// defaults its sight the same way (tiled.go:895-897).
	e.Blocked, e.BlocksSight = true, true
	e.Problems = append(e.Problems, extra...)
	e.Category = CategoryOf(e.Layer, e.Footprint, e.ID, e.DisplayName)

	e.Status, e.StatusWhy, e.Signals = DeriveStatus(Signals{
		Path:            art,
		Integration:     integration,
		IntegrationFrom: manifest,
	})

	return e
}

// moduleName reads "dealu-monastery-v1" + "church" + "intact" as "Dealu
// monastery church", and the gate's two states as "Dealu monastery gate (open)"
// and "... (closed)". The trailing -vN of a manifest id is dropped: a version
// is not part of a thing's name.
func moduleName(manifestID, modID, state string) string {
	base := manifestID
	if i := strings.LastIndex(base, "-v"); i > 0 && isDigits(base[i+2:]) {
		base = base[:i]
	}

	name := words(base) + " " + strings.ToLower(words(modID))
	if !stateWords[strings.ToLower(state)] {
		name += " (" + strings.ToLower(words(state)) + ")"
	}

	return name
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}

	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}

// readPNGSize reads just enough of a PNG to learn its size.
func readPNGSize(fsys fs.FS, p string) (w, h int, err error) {
	f, err := fsys.Open(p)
	if err != nil {
		return 0, 0, fmt.Errorf("reading %s: %w", p, err)
	}
	defer f.Close()

	head := make([]byte, 24)

	n, err := io.ReadFull(f, head)
	if n < 24 {
		if err == nil {
			err = ErrNotPNG
		}

		return 0, 0, fmt.Errorf("reading %s: %w", p, err)
	}

	w, h, err = PNGSize(head)
	if err != nil {
		return 0, 0, fmt.Errorf("reading %s: %w", p, err)
	}

	return w, h, nil
}

// Package d2mappalette is the model behind the map editor's asset palette: the
// honest answer to "what can I put on this map, what is it called, what does it
// look like, and is the art finished?"
//
// It is a LEAF package. No ebiten, no d2mapedit, no world, no RNG: it reads
// bytes through an fs.FS and returns a list of [Entry]. It does not decode or
// scale a single image -- see THUMBNAILS below -- so it can be unit-tested on a
// Linux CI box with no display.
//
// # What it is for
//
// The editor has to grey out a piece a user cannot place, instead of letting
// him place it and get a map the engine refuses at load. So every Entry carries
// [Entry.Problems]: the list of rules in d2core/d2map/d2maptiled the art
// breaks, empty when the art will load. The rules are not restated from memory
// here; they are transcribed with citations in [CheckArt] and cross-checked
// against the real loader by TestPaletteAgreesWithTheEngine, which builds
// synthetic maps and asserts that this package says "placeable" exactly when
// d2maptiled.Parse accepts them.
//
// # Status: there is no approval field in the engine
//
// A search of the tree finds no "approved", "approval" or review field on any
// asset -- d2records, d2asset and d2maptiled have nothing of the kind, and the
// one machine-readable "not finished" record in the whole repository is the
// knownUnanchored allowlist in d2core/d2asset/png_sheet_anchor_test.go:47,
// which names sheet folders drawn off their foot point. This package therefore
// INVENTS NOTHING. [Status] is derived from the signals that actually exist,
// each one recorded in [Entry.Signals] so the editor can show its work:
//
//   - a "placeholder" element anywhere in the art's path (every one of the 14
//     tiles in data/strigoi/maps/tiles is placeholder-*.png, and the hero has a
//     hero/placeholder folder);
//   - the map's own "note" property, which the loader reads and THROWS AWAY
//     (d2maptiled/tiled.go:325 is an empty `case "note":`, and Map has no Note
//     field) -- so this package parses the .tmj itself to see it;
//   - an "integration" value from a strigoi-art structure manifest, which is
//     the only key in that repository this package will read as data.
//
// See [DeriveStatus] for the precedence and for what was deliberately NOT
// parsed. Nothing here guesses: an asset with no signal at all is
// [StatusUnknown], and says so.
//
// # Category
//
// [Category] is the one field on an Entry that is a CONVENTION rather than a
// measurement. The engine has no notion of a building or a prop; it has a
// layer, a footprint and two booleans. Category exists because the editor shows
// tabs, and it is derived by [CategoryOf] from the layer, the footprint and a
// small documented keyword table. Three of the five categories are not
// implemented at all in v0 and say why -- see [Tabs].
//
// # Thumbnails
//
// This package NEVER decodes, scales or draws an image. [Entry.ImagePath],
// [Entry.PixelWidth] and [Entry.PixelHeight] are all the editor screen needs to
// load the PNG with ebiten and letterbox it into a thumbnail box of whatever
// size it likes; the true pixel size is given so the screen can preserve the
// aspect ratio rather than squash a 160x330 church tower into a square. Sizes
// come from the PNG's IHDR header ([PNGSize]), read as eight bytes at offsets
// 16 and 20, because decoding every candidate image to learn its width would
// make opening the palette cost a full image decode per asset for nothing.
package d2mappalette

import (
	"fmt"
	"image"
	"sort"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// The engine's bounds on art, transcribed from d2core/d2map/d2maptiled.
//
// TileWidth, TileHeight and MaxKinds are taken from the loader's own exported
// constants rather than copied, so they cannot drift. MaxWallHeight and
// MaxStructureHeight are the loader's UNEXPORTED maxWallHeight (tiled.go:120)
// and maxStructureHeight (tiled.go:141); they are copied because Go leaves no
// choice, and TestHeightBoundsMatchTheLoader pins each one by handing the real
// d2maptiled.Parse art one pixel either side of it.
const (
	// TileWidth and TileHeight are the only tile size the engine draws
	// (tiled.go:106-107).
	TileWidth  = d2maptiled.TileWidth
	TileHeight = d2maptiled.TileHeight

	// MaxWallHeight is the tallest wall art the renderer keeps inside its cull
	// margin (tiled.go:120).
	MaxWallHeight = 512

	// MaxStructureHeight is the tallest structure art (tiled.go:141).
	MaxStructureHeight = 768

	// MaxFootprint is the largest footprint side a structure tile may declare;
	// the loader's footprint_w/footprint_h property must be 1..16
	// (tiled.go:845-846).
	MaxFootprint = 16

	// MaxKinds is how many distinct (tile image, layer) pairs one map may use
	// (tiled.go:112). A palette that cannot see this cap will happily offer a
	// 257th tile the loader then refuses.
	MaxKinds = d2maptiled.MaxKinds
)

// LayerSet is a set of the layers the engine would accept one piece of art on.
//
// It is a SET and not a single layer on purpose. A 160x80 image with no
// footprint is legal as a floor and as a wall; a 160x256 image is legal as a
// wall and as a 1x1 structure; a 160x600 image is too tall for a wall
// (MaxWallHeight) yet fits a 1x1 structure. Collapsing that to one answer would
// make the palette lie about half the village's art.
type LayerSet uint8

// The three layer bits, in d2maptiled.Layer order.
const (
	SetFloor LayerSet = 1 << iota
	SetWall
	SetStructure
)

// LayerBit is the LayerSet holding just l.
func LayerBit(l d2maptiled.Layer) LayerSet { return 1 << uint(l) }

// Has reports whether the set holds l.
func (s LayerSet) Has(l d2maptiled.Layer) bool { return s&LayerBit(l) != 0 }

// Empty reports whether no layer in the engine will take this art.
func (s LayerSet) Empty() bool { return s == 0 }

// Primary is the layer the palette should default to placing this art on.
//
// FLOOR FIRST, then wall, then structure -- lowest layer wins. The floor rule is
// the strictest the loader has (exactly TileWidth x TileHeight, tiled.go:822),
// so art that satisfies it was drawn as a floor diamond, and a 160x80 image
// legal as both should be offered as ground rather than as a very short wall.
// A 160x256 image fails the floor rule and is offered as a wall even though a
// 1x1 structure would also take it, because a wall tile is the simpler
// placement and it is how data/strigoi/maps/village.tmj actually stands the
// well and the hearth up (tiles 14 and 15, on the walls layer).
//
// It returns LayerFloor on the empty set; callers check Empty first, and a
// refused [Entry] carries its intended layer instead.
func (s LayerSet) Primary() d2maptiled.Layer {
	switch {
	case s.Has(d2maptiled.LayerFloor):
		return d2maptiled.LayerFloor
	case s.Has(d2maptiled.LayerWall):
		return d2maptiled.LayerWall
	case s.Has(d2maptiled.LayerStructure):
		return d2maptiled.LayerStructure
	}

	return d2maptiled.LayerFloor
}

// String lists the set in the loader's own layer names.
func (s LayerSet) String() string {
	if s == 0 {
		return "no layer"
	}

	names := make([]string, 0, 3)

	for _, l := range []d2maptiled.Layer{d2maptiled.LayerFloor, d2maptiled.LayerWall, d2maptiled.LayerStructure} {
		if s.Has(l) {
			names = append(names, l.String())
		}
	}

	return strings.Join(names, "+")
}

// SquareFootprintFor inverts the loader's structure width formula.
//
// checkArt (tiled.go:811) wants a structure's art exactly (fw+fh)*TileWidth/2
// wide, and Kind.properties (tiled.go:880) refuses a footprint that is not
// square. For a square n x n footprint that is 2n*80 = 160n, so exactly one
// square footprint can fit any given width, and it is width/160. A width that
// is not a multiple of TileWidth, or that works out below 1 or above
// MaxFootprint, fits no square footprint at all.
//
// This is arithmetic on the engine's own rule, not a guess about the art. It is
// how a bare PNG on disk gets a footprint when nothing declares one -- and
// [Entry.FootprintWhy] says so to the user, because a 480-wide image the artist
// meant as a 1x5 barn will be offered here as a 3x3.
func SquareFootprintFor(width int) (image.Point, bool) {
	if width <= 0 || width%TileWidth != 0 {
		return image.Point{}, false
	}

	n := width / TileWidth
	if n < 1 || n > MaxFootprint {
		return image.Point{}, false
	}

	return image.Pt(n, n), true
}

// StructureWidthFor is the loader's own formula (tiled.go:811): the pixel width
// a footprint demands.
func StructureWidthFor(fp image.Point) int { return (fp.X + fp.Y) * TileWidth / 2 }

// LegalLayers reports every layer the engine would accept a w x h image on,
// judging by size alone -- what a bare PNG on disk can tell you.
//
// A tile's own properties narrow this further and the narrowing is absolute:
// kind() (tiled.go:689-694) refuses a footprinted tile anywhere but as a tile
// object, and refuses an unfootprinted one AS a tile object. [Entry] intersects
// the two.
func LegalLayers(w, h int) LayerSet {
	var out LayerSet

	if w == TileWidth && h == TileHeight {
		out |= SetFloor
	}

	if w == TileWidth && h >= TileHeight && h <= MaxWallHeight {
		out |= SetWall
	}

	if _, ok := SquareFootprintFor(w); ok && h >= TileHeight && h <= MaxStructureHeight {
		out |= SetStructure
	}

	return out
}

// CheckArt returns every reason the engine would refuse this art on this layer,
// in the loader's own words, or nil when it will load it.
//
// It is the palette's copy of checkArt (tiled.go:807-833) plus the footprint
// rules from Kind.properties (tiled.go:835-899), and it is only worth having
// because TestPaletteAgreesWithTheEngine holds it against the real
// d2maptiled.Parse on both sides of every bound.
//
// footprint is the zero Point for a floor or wall tile.
func CheckArt(layer d2maptiled.Layer, footprint image.Point, w, h int) []string {
	var why []string

	switch layer {
	case d2maptiled.LayerStructure:
		if footprint == (image.Point{}) {
			return []string{"a tile object with no footprint_w/footprint_h is not a structure; only structures are tile objects (tiled.go:690)"}
		}

		if footprint.X < 1 || footprint.X > MaxFootprint || footprint.Y < 1 || footprint.Y > MaxFootprint {
			why = append(why, fmt.Sprintf("footprint %dx%d is outside 1..%d; footprint_w and footprint_h must be an int from 1 to %d (tiled.go:845)",
				footprint.X, footprint.Y, MaxFootprint, MaxFootprint))
		}

		if footprint.X != footprint.Y {
			why = append(why, fmt.Sprintf("a %dx%d footprint is not square; structures are square for now (tiled.go:880)",
				footprint.X, footprint.Y))
		}

		if want := StructureWidthFor(footprint); w != want {
			why = append(why, fmt.Sprintf("structure art is %d wide; a %dx%d footprint wants exactly %d ((w+h)*%d) (tiled.go:811-814)",
				w, footprint.X, footprint.Y, want, TileHeight))
		}

		if h < TileHeight || h > MaxStructureHeight {
			why = append(why, fmt.Sprintf("structure art is %d tall; want %d..%d (tiled.go:812)", h, TileHeight, MaxStructureHeight))
		}
	case d2maptiled.LayerFloor:
		if footprint != (image.Point{}) {
			why = append(why, fmt.Sprintf("it has a %dx%d footprint, so it is a structure and belongs on the objects layer as a tile object, not on the floor layer (tiled.go:692)",
				footprint.X, footprint.Y))
		}

		if w != TileWidth || h != TileHeight {
			why = append(why, fmt.Sprintf("floor art is %dx%d, want exactly %dx%d (tiled.go:822)", w, h, TileWidth, TileHeight))
		}
	case d2maptiled.LayerWall:
		if footprint != (image.Point{}) {
			why = append(why, fmt.Sprintf("it has a %dx%d footprint, so it is a structure and belongs on the objects layer as a tile object, not on the walls layer (tiled.go:692)",
				footprint.X, footprint.Y))
		}

		if w != TileWidth || h < TileHeight || h > MaxWallHeight {
			why = append(why, fmt.Sprintf("wall art is %dx%d, want %d wide and %d..%d tall (tiled.go:829)",
				w, h, TileWidth, TileHeight, MaxWallHeight))
		}
	default:
		why = append(why, fmt.Sprintf("layer %d is not one the loader knows", int(layer)))
	}

	return why
}

// Source says where an Entry came from, so a reason can name a real file.
type Source struct {
	// Kind is "tileset", "disk" or "manifest".
	Kind string
	// From is the file the entry was read out of: the .tmj, the PNG, or the
	// module manifest.
	From string
	// Detail is the tile id, the module id, or "".
	Detail string
}

func (s Source) String() string {
	if s.Detail == "" {
		return s.Kind + " " + s.From
	}

	return s.Kind + " " + s.From + " (" + s.Detail + ")"
}

// Entry is one thing the palette can offer.
//
// An Entry is a claim about a PNG the palette has actually measured. Nothing on
// it is inferred from a filename except DisplayName and Category, and both say
// so in their own documentation.
type Entry struct {
	// ID is stable across runs and unique within a Catalog: the tileset's
	// "<tileset>#<tile id>" the loader itself uses for Kind.Name
	// (tiled.go:679), or "<dir>/<file>" for a PNG on disk, or
	// "<manifest id>/<module id>/<state>" for a module.
	ID string

	// DisplayName is human words, DERIVED FROM THE PATH by [HumanName] because
	// no asset in this project carries a title anywhere. "Church tower", not
	// "placeholder-church-tower.png".
	DisplayName string

	// ImagePath is the art, slash-separated, resolved the way the engine
	// resolves it: path.Join(dir-of-the-map, the tileset's relative path)
	// (tiled.go:777). Backslashes are refused by the loader (tiled.go:773) so
	// this is always forward slashes.
	ImagePath string

	// PixelWidth and PixelHeight are the PNG's real size, read from its IHDR
	// rather than from whatever the .tmj declares. The editor letterboxes a
	// thumbnail with these; this package never decodes the image.
	PixelWidth, PixelHeight int

	// Footprint is the size in tiles the piece occupies. (1,1) for a floor or
	// wall tile -- those always cover exactly one tile.
	Footprint image.Point

	// FootprintWhy says where Footprint came from: declared by a tile
	// property, declared by a manifest, or derived from the art's width.
	FootprintWhy string

	// Layer is the layer the palette places it on by default; Layers is every
	// layer the engine would accept. Layers is empty when the art fits nowhere,
	// and then Layer is meaningless and Problems is not empty.
	Layer  d2maptiled.Layer
	Layers LayerSet

	// Blocked and BlocksSight are what the engine will believe about this
	// piece, defaults applied the way Kind.properties does: blocked is false
	// unless said, blocks_sight defaults to blocked-or-has-a-footprint
	// (tiled.go:895-897), and a structure's blocked is FORCED true whatever it
	// says (tiled.go:887-890). A structure may still declare
	// blocks_sight=false; only blocked=false is refused.
	Blocked     bool
	BlocksSight bool

	// Category is the editor's tab. A convention, not an engine fact: see
	// [CategoryOf].
	Category Category

	// Status is approved / preview / unknown, and StatusWhy is one sentence the
	// editor can put in front of the user. Signals lists every signal that was
	// found, including the ones the precedence rule did not use.
	Status    Status
	StatusWhy string
	Signals   []string

	// Problems is empty when the engine will load this art on Layer, and
	// otherwise holds the loader's own refusals. The palette greys out an entry
	// with problems rather than letting a user build a map that will not open.
	Problems []string

	// Source is the file this entry was read out of.
	Source Source
}

// Placeable reports whether the engine will accept this art as it stands.
func (e Entry) Placeable() bool { return len(e.Problems) == 0 && !e.Layers.Empty() }

// Why is one line an editor can show: the status sentence, plus the first
// refusal when the piece cannot be placed.
func (e Entry) Why() string {
	if e.Placeable() {
		return e.StatusWhy
	}

	if len(e.Problems) == 0 {
		return e.StatusWhy + " It fits no layer the engine has."
	}

	return e.StatusWhy + " It cannot be placed: " + e.Problems[0]
}

// sortEntries puts a catalog in a stable order: category, then name, then ID.
func sortEntries(es []Entry) {
	sort.SliceStable(es, func(i, j int) bool {
		switch {
		case es[i].Category != es[j].Category:
			return categoryOrder(es[i].Category) < categoryOrder(es[j].Category)
		case es[i].DisplayName != es[j].DisplayName:
			return es[i].DisplayName < es[j].DisplayName
		}

		return es[i].ID < es[j].ID
	})
}

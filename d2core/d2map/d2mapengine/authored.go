package d2mapengine

import (
	"image"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2ds1"
)

// AuthoredStyle is the tile style every AUTHORED tile carries -- a tile built
// from a Tiled map (M5.4, d2maptiled) rather than from a DS1 stamp. Its art is
// not in any DT1; it is a PNG the map brought with it, handed to the renderer
// through AuthoredImages.
//
// Why 250 cannot collide: a DS1 stores a tile's style in six bits (d2ds1
// styleBitmask), so every Diablo II tile has a style of 0..63. The renderer's
// image cache is keyed on style, sequence and type, so a reserved style is
// the whole of the separation -- there is no second table to keep in step.
const AuthoredStyle byte = 250

// AuthoredKey names one authored tile image: its sequence under
// AuthoredStyle (the d2maptiled kind index), the tile type it is drawn as,
// and -- for a structure cut into strips -- which strip (the tile's
// RandomIndex; 0 for everything else).
type AuthoredKey struct {
	Sequence byte
	Type     d2enum.TileType
	Index    byte
}

// The tile types authored walls are drawn as. All three are upper walls, so
// the renderer draws them interleaved with the entities around them
// (renderPass3) -- none is a roof, a special or a lower wall.
const (
	// AuthoredWallType is a one-tile wall (a fence, a tree, the well).
	AuthoredWallType = d2enum.TilePillarsColumnsAndStandaloneObjects

	// AuthoredStripLeft and AuthoredStripRight are the 80-pixel strips a
	// STRUCTURE is cut into, one per tile along its two front faces, the way
	// Diablo II cuts its own buildings: drawn tile by tile, each strip is
	// ordered against the people beside it the way a one-tile wall is. A
	// left-face strip covers the left half of its tile's diamond, a
	// right-face strip the right half.
	AuthoredStripLeft  = d2enum.TileLeftWall
	AuthoredStripRight = d2enum.TileRightWall

	// AuthoredStripWidth is one strip: half a tile's width.
	AuthoredStripWidth = 80
)

// AuthoredWallLeft is how far left of its tile's top corner an authored
// wall image is drawn: a left-face strip over the diamond's left half, a
// right-face strip over its right half, anything else centred on the corner
// by its own width (-80 for a one-tile wall, as a DT1 wall hangs).
func AuthoredWallLeft(t d2enum.TileType, width int) float64 {
	switch t {
	case AuthoredStripLeft:
		return -AuthoredStripWidth
	case AuthoredStripRight:
		return 0
	}

	return -float64(width) / 2
}

// IsAuthoredTile reports whether a tile's art comes from AuthoredImages rather
// than from the DT1s. The renderer uses it to keep its DT1 cache builder off
// these tiles, which would otherwise look them up, find nothing, and log an
// error per tile.
func IsAuthoredTile(t *d2ds1.Tile) bool {
	return t.Style == AuthoredStyle
}

// SetAuthored records what an authored map brought beyond its tiles: the art
// for every authored tile and the start position the map named. ResetMap
// clears both, so a generated world after an authored one carries neither.
func (m *MapEngine) SetAuthored(images map[AuthoredKey]*image.RGBA, startX, startY float64) {
	m.authoredImages = images
	m.authoredStart = &[2]float64{startX, startY}
}

// SetInside records an authored map's "inside" areas, in whole tiles (Max
// exclusive): the ground the night does not arrive on (d2maptiled; the
// spawner places arrivals outside them). ResetMap clears them.
func (m *MapEngine) SetInside(areas []image.Rectangle) {
	m.authoredInside = append([]image.Rectangle(nil), areas...)
}

// Inside reports whether tile x, y lies in an authored inside area. Always
// false on a generated map, which has none.
func (m *MapEngine) Inside(x, y int) bool {
	p := image.Pt(x, y)
	for _, r := range m.authoredInside {
		if p.In(r) {
			return true
		}
	}

	return false
}

// AuthoredImages returns the authored tile art, nil on a generated map.
func (m *MapEngine) AuthoredImages() map[AuthoredKey]*image.RGBA {
	return m.authoredImages
}

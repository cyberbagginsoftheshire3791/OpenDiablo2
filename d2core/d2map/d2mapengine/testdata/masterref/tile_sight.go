package masterref

import (
	"image"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2geom"
)

// FOG OF WAR, F1 (1 Oct 2026; claude/fog-of-war-build-plan.md §2.3). Fog asks
// a different question from checkLos: not "can this point see that point" but
// "can an eye see THAT TILE" -- and a tile that blocks sight is still seen, by
// its face. So the walk is tile-stepped, reads one cell per tile it crosses,
// and never reads the destination. The walk itself is d2geom.TileLineClear
// since F4 (2 Oct 2026), so fog's own cached grid walks the same code.

// TileSightClear reports whether the straight line from the point (fx, fy), in
// world tiles, to the CENTRE of tile (tx, ty) crosses no tile that blocks
// sight STRICTLY BETWEEN the two: the eye's own tile and the destination are
// never read, so a wall is seen by its face and only what stands behind it is
// hidden. cells is how many tiles the walk read (the fog's cost counter).
//
// It is a grid traversal (Amanatides & Woo; d2geom.TileLineClear, where the
// corner rule is): every tile the segment passes through, in order, and
// nothing it does not. A tile "blocks sight" when its centre subtile does
// under the map's ONE sight rule, sightBlocked -- the shipped SightRule that
// the notice model's checkLos obeys too, so the beasts' eyes and the player's
// fog never disagree about what is opaque. On the authored village a
// blocks_sight tile marks all 25 of its subtiles, so its centre answers for
// it. A tile off the map blocks, as it does for checkLos.
//
// checkLos, LineOfSight and the notice model are untouched by it.
func (m *MapEngine) TileSightClear(fx, fy float64, tx, ty int) (clear bool, cells int) {
	return d2geom.TileLineClear(fx, fy, tx, ty, m.tileBlocksSight)
}

// TileBlocksSight is whether tile (tx, ty) stops a line of sight (fog of war
// F4): its centre subtile under the map's sight rule, and every tile off the
// map. Fog reads it once a map into a grid of its own, and walks that grid
// with the same d2geom.TileLineClear TileSightClear walks.
func (m *MapEngine) TileBlocksSight(tx, ty int) bool { return m.tileBlocksSight(tx, ty) }

// tileBlocksSight is whether tile (x, y) stops the fog's ray: its centre
// subtile under the map's sight rule, and every tile off the map.
func (m *MapEngine) tileBlocksSight(x, y int) bool {
	centre := subtilesPerTile / 2 //nolint:gomnd // the middle subtile of five

	flags := m.SubTileAt(x*subtilesPerTile+centre, y*subtilesPerTile+centre)

	return flags == nil || m.sightBlocked(flags)
}

// SetStructures records an authored map's structure footprints, in whole tiles
// (Max exclusive). LayAuthoredMap passes them; ResetMap clears them. Fog of war
// reads them so a house seen by any one tile is shown whole: its art stands on
// its front tiles, which its own footprint hides from an eye behind it, so
// without them a house seen from behind or the side would not be drawn at all.
func (m *MapEngine) SetStructures(footprints []image.Rectangle) {
	m.structures = append([]image.Rectangle(nil), footprints...)
}

// Structures are the map's structure footprints (none on a generated map).
func (m *MapEngine) Structures() []image.Rectangle {
	return append([]image.Rectangle(nil), m.structures...)
}

// RAISED SIGHT (fog of war F4, 1 Oct 2026; Josh's ruling 4: talents,
// structures, gear and height all raise sight). The engine keeps an authored
// map's ground heights and towers for fog to read; nothing else reads them --
// v1's height raises sight only (it does not see over blockers, move anyone or
// change the draw), and a tower is an eye, not a garrison (Q6).

// SetHeights records an authored map's ground height per tile, in levels,
// row-major over the map's size (nil, or a slice of the wrong length, is flat
// ground). LayAuthoredMap passes them; ResetMap clears them.
func (m *MapEngine) SetHeights(heights []uint8) {
	if len(heights) != m.size.Width*m.size.Height {
		m.heights = nil

		return
	}

	m.heights = append([]uint8(nil), heights...)
}

// HeightAt is the ground's height at tile (tx, ty) in levels: 0 off the map,
// on a generated map, and on flat ground.
func (m *MapEngine) HeightAt(tx, ty int) int {
	if m.heights == nil || tx < 0 || ty < 0 || tx >= m.size.Width || ty >= m.size.Height {
		return 0
	}

	return int(m.heights[ty*m.size.Width+tx])
}

// SetTowers records an authored map's towers: each one's footprint, in whole
// tiles (Max exclusive), and its sight radius in tiles, one for one.
// LayAuthoredMap passes them; ResetMap clears them.
func (m *MapEngine) SetTowers(footprints []image.Rectangle, sight []float64) {
	n := len(footprints)
	if len(sight) < n {
		n = len(sight)
	}

	m.towers = append([]image.Rectangle(nil), footprints[:n]...)
	m.towerSight = append([]float64(nil), sight[:n]...)
}

// TowerSights are the map's towers, footprints and sight radii one for one
// (none on a generated map). Fog reads them when it sizes itself to the map:
// every standing tower is an eye.
func (m *MapEngine) TowerSights() (footprints []image.Rectangle, sight []float64) {
	return append([]image.Rectangle(nil), m.towers...), append([]float64(nil), m.towerSight...)
}

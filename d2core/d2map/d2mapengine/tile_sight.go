package d2mapengine

import "math"

// FOG OF WAR, F1 (1 Oct 2026; claude/fog-of-war-build-plan.md §2.3). Fog asks
// a different question from checkLos: not "can this point see that point" but
// "can an eye see THAT TILE" -- and a tile that blocks sight is still seen, by
// its face. So the walk is tile-stepped, reads one cell per tile it crosses,
// and never reads the destination.

// tileSightCellCap bounds the walk: a ray from one corner of the largest map
// to the other crosses fewer tiles than its width plus its height, so a walk
// that reaches this many has gone wrong (NaN coordinates) and answers blocked.
const tileSightCellCap = 1 << 16

// tieEpsilon is how close the walk's two next boundary crossings must be to
// count as one: the ray passes through a tile corner.
const tieEpsilon = 1e-9

// TileSightClear reports whether the straight line from the point (fx, fy), in
// world tiles, to the CENTRE of tile (tx, ty) crosses no tile that blocks
// sight STRICTLY BETWEEN the two: the eye's own tile and the destination are
// never read, so a wall is seen by its face and only what stands behind it is
// hidden. cells is how many tiles the walk read (the fog's cost counter).
//
// It is a grid traversal (Amanatides & Woo): every tile the segment passes
// through, in order, and nothing it does not. A tile "blocks sight" when its
// centre subtile does under the map's ONE sight rule, sightBlocked -- the
// shipped SightRule that the notice model's checkLos obeys too, so the beasts'
// eyes and the player's fog never disagree about what is opaque. On the
// authored village a blocks_sight tile marks all 25 of its subtiles, so its
// centre answers for it. A tile off the map blocks, as it does for checkLos.
//
// WHERE THE RAY PASSES EXACTLY THROUGH A CORNER it steps diagonally, and is
// stopped only if BOTH tiles beside the corner block: a diagonal run of wall
// tiles touching at their corners is opaque, and one post beside the diagonal
// is not. Without this, whichever axis the tie broke toward would decide, and
// the answer would differ from the reverse ray's.
//
// checkLos, LineOfSight and the notice model are untouched by it.
func (m *MapEngine) TileSightClear(fx, fy float64, tx, ty int) (clear bool, cells int) {
	x, y := int(math.Floor(fx)), int(math.Floor(fy))
	if x == tx && y == ty {
		return true, 0
	}

	dx := float64(tx) + 0.5 - fx
	dy := float64(ty) + 0.5 - fy

	stepX, tMaxX, tDeltaX := axisWalk(fx, dx)
	stepY, tMaxY, tDeltaY := axisWalk(fy, dy)

	for cells < tileSightCellCap {
		switch {
		case math.Abs(tMaxX-tMaxY) <= tieEpsilon:
			// Through a corner: the two tiles beside it, then the diagonal one.
			sideX, sideY := m.tileBlocksSight(x+stepX, y), m.tileBlocksSight(x, y+stepY)
			cells += 2

			if sideX && sideY {
				return false, cells
			}

			x += stepX
			y += stepY
			tMaxX += tDeltaX
			tMaxY += tDeltaY
		case tMaxX < tMaxY:
			x += stepX
			tMaxX += tDeltaX
		default:
			y += stepY
			tMaxY += tDeltaY
		}

		if x == tx && y == ty {
			return true, cells
		}

		cells++

		if m.tileBlocksSight(x, y) {
			return false, cells
		}
	}

	return false, cells
}

// axisWalk is one axis of the grid traversal from p along d: the step (+1, -1
// or 0), the ray parameter at which it first crosses a tile boundary, and the
// parameter between crossings. The segment runs over parameters 0..1.
func axisWalk(p, d float64) (step int, tMax, tDelta float64) {
	switch {
	case d > 0:
		return 1, (math.Floor(p) + 1 - p) / d, 1 / d
	case d < 0:
		return -1, (p - math.Floor(p)) / -d, 1 / -d
	default:
		return 0, math.Inf(1), math.Inf(1)
	}
}

// tileBlocksSight is whether tile (x, y) stops the fog's ray: its centre
// subtile under the map's sight rule, and every tile off the map.
func (m *MapEngine) tileBlocksSight(x, y int) bool {
	centre := subtilesPerTile / 2 //nolint:gomnd // the middle subtile of five

	flags := m.SubTileAt(x*subtilesPerTile+centre, y*subtilesPerTile+centre)

	return flags == nil || m.sightBlocked(flags)
}
